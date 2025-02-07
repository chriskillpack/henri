package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/chriskillpack/henri"
)

var (
	libraryPath  = flag.String("library", "", "Path to photos library")
	dbPath       = flag.String("db", "./henri.db", "Path to database")
	llamaServer  = flag.String("llama", "", "Address of running llama server, typically http://localhost:8080")
	llamaSeed    = flag.Int("seed", 385480504, "Random seed to llama")
	ollamaServer = flag.String("ollama", "", "Address of running ollama server, typically http://localhost:11434")

	lameduck bool
)

func findJpegFiles(root string) ([]string, []time.Time, error) {
	var photos []string
	var mtimes []time.Time

	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" {
			photos = append(photos, path)
			mtimes = append(mtimes, info.ModTime())
		}

		return nil
	})

	return photos, mtimes, err
}

func run(ctx context.Context, h *henri.Henri, dbpath string) error {
	// Is the server healthy?
	if !h.Describer.IsHealthy() {
		return fmt.Errorf("server is not responding")
	}

	db, err := henri.NewDB(ctx, dbpath)
	if err != nil {
		return err
	}
	defer db.Close()

	var photos []string
	var mtimes []time.Time
	if *libraryPath != "" {
		photos, mtimes, err = findJpegFiles(*libraryPath)
		if err != nil {
			return err
		}
		fmt.Printf("Found %d images on disk\n", len(photos))
		const batchSize = 100
		added, err := db.InsertImagePaths(ctx, photos, mtimes, batchSize)
		if err != nil {
			return err
		}
		fmt.Printf("Added %d new images\n", added)
		return nil
	}

	images, err := db.ImagesToDescribe(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%d images to process\nClassifying with %s\n", len(images), h.Describer.Name())

	errcnt := 0
out:
	for i := 0; i < len(images) && !lameduck; i++ {
		if errcnt >= 5 {
			fmt.Println("Too many errors, exiting")
			return err
		}

		select {
		case <-ctx.Done():
			break out
		default:
		}
		img := images[i]
		_, fname := filepath.Split(img.Path)

		fmt.Printf("Processing %d/%d <%d: %s> ", i, len(images), img.Id, fname)
		now := time.Now()

		imgdata, err := os.ReadFile(img.Path)
		if err != nil {
			// Skip missing file errors
			if _, ok := err.(*fs.PathError); ok {
				fmt.Printf("file error, skipping: %s\n", err)
				err = db.UpdateImageAttempted(ctx, img.Id, h.Describer.Name(), now)
				if err != nil {
					fmt.Printf("error updating image attempt: %s\n", err)
					errcnt++
				}
				continue
			}
			return err
		}

		img.Description, err = h.Describer.DescribeImage(ctx, imgdata)
		if err != nil {
			db.UpdateImageAttempted(ctx, img.Id, h.Describer.Name(), now) // ignore error, already in an error state

			// Allow up to 5 errors before bailing
			errcnt++
			if errcnt == 5 {
				fmt.Println()
				continue
			}
		} else {
			end := time.Now()
			img.ProcessedAt = now
			db.UpdateImage(ctx, img, h.Describer.Name())
			fmt.Printf("okay, %d secs", int(end.Sub(now).Seconds()))
		}
		fmt.Println()
	}

	return nil
}

func sighandler(ch chan os.Signal, cancel context.CancelFunc) {
	for {
		<-ch
		if lameduck {
			// Already in lame duck, hard stop
			fmt.Println("Exiting")
			cancel()
			return
		} else {
			fmt.Println("SIGINT received, stopping...")
			lameduck = true
		}
	}
}

func main() {
	flag.Parse()
	hio := henri.InitOptions{
		LlamaServer:  *llamaServer,
		LlamaSeed:    *llamaSeed,
		OllamaServer: *ollamaServer,
	}
	var (
		h   *henri.Henri
		err error
	)
	if h, err = henri.Init(hio); err != nil {
		log.Fatal(err)
	}

	sigch := make(chan os.Signal, 2)
	signal.Notify(sigch, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	go sighandler(sigch, cancel)

	if err := run(ctx, h, *dbPath); err != nil {
		log.Fatal(err)
	}
}
