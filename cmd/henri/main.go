package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
)

var (
	libraryPath    = flag.String("library", "", "Path to photos library")
	dbPath         = flag.String("db", "./henri.db", "Path to database")
	llamaServer    = flag.String("llama", "", "Address of running llama server, typically http://localhost:8080")
	llamaSeed      = flag.Int("seed", 385480504, "Random seed to llama")
	ollamaServer   = flag.String("ollama", "", "Address of running ollama server, typically http://localhost:11434")
	openAI         = flag.Bool("openai", false, "Use OpenAI")
	calcEmbeddings = flag.Bool("embeddings", false, "Specify to compute missing description embeddings")
	query          = flag.String("query", "", "Search query")
	count          = flag.Int("count", -1, "Number of items to process")

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

func describeImageFn(ctx context.Context, d describer.Describer, img *henri.Image, db *henri.DB) error {
	now := time.Now()

	imgdata, err := os.ReadFile(img.Path)
	if err != nil {
		// Skip missing file errors
		if _, ok := err.(*fs.PathError); ok {
			fmt.Printf("file error, skipping: %s\n", err)
			err = db.UpdateImageAttempted(ctx, img.Id, d.Model(), d.Name(), now)
			if err != nil {
				fmt.Printf("error updating image attempt: %s\n", err)
				return err
			}
			return nil
		}
		return err
	}

	img.Description, err = d.DescribeImage(ctx, imgdata)
	if err != nil {
		db.UpdateImageAttempted(ctx, img.Id, d.Model(), d.Name(), now) // ignore error, already in an error state
		return err
	} else {
		img.ProcessedAt.Time = now
		img.ProcessedAt.Valid = true // TODO - this feels error prone, is there a better way?
		db.UpdateImage(ctx, img, d.Model(), d.Name())
	}

	return nil
}

func calcEmbeddingFn(ctx context.Context, d describer.Describer, img *henri.Image, db *henri.DB) error {
	vector, err := d.Embeddings(ctx, img.Description)
	if err != nil {
		return err
	}
	_, err = db.CreateEmbedding(ctx, vector, d.Model(), img, time.Now())
	if err != nil {
		return err
	}

	return nil
}

func run(ctx context.Context, h *henri.Henri, dbpath string) error {
	if h.Name() == "openai" && !*calcEmbeddings && *libraryPath == "" {
		return fmt.Errorf("for privacy reasons OpenAI cannot be used for describing")
	}

	db, err := henri.NewDB(ctx, dbpath)
	if err != nil {
		return err
	}
	defer db.Close()

	if *libraryPath != "" {
		photos, mtimes, err := findJpegFiles(*libraryPath)
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

	// All functionality from this point on requires the LLM server. Check if
	// it is healthy.
	if !h.Describer.IsHealthy() {
		return fmt.Errorf("server is not responding")
	}

	if *query != "" {
		// Issue query
		if err := runQuery(*query, h.Describer, db); err != nil {
			return err
		}

		return nil
	}

	var (
		images []*henri.Image
		workFn func(context.Context, describer.Describer, *henri.Image, *henri.DB) error
	)

	if *calcEmbeddings {
		images, err = db.DescribedImagesMissingEmbeddings(ctx, h.Describer.Model())
		workFn = calcEmbeddingFn
	} else {
		// Assume image describe mode
		images, err = db.ImagesToDescribe(ctx)
		workFn = describeImageFn
	}

	if err != nil {
		fmt.Println(err)
		return err
	}

	if *count > -1 {
		images = images[:min(len(images), *count)]
	}
	fmt.Printf("%d images to process\nUsing describer %s model %s\n", len(images), h.Describer.Name(), h.Describer.Model())

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

		if err = workFn(ctx, h.Describer, img, db); err != nil {
			errcnt++
			fmt.Println()
			continue
		}
		end := time.Now()
		fmt.Printf("okay, %d secs\n", int(end.Sub(now).Seconds()))
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

	if *calcEmbeddings && *query != "" {
		// Query has to act alone
		flag.Usage()
		os.Exit(1)
	}

	hio := henri.InitOptions{
		LlamaServer:  *llamaServer,
		LlamaSeed:    *llamaSeed,
		OllamaServer: *ollamaServer,
		OpenAI:       *openAI,
		HttpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
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
