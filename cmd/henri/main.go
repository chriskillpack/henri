package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png" // see imageDimensions()
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
)

type AppMode int

const (
	AppModeScan AppMode = iota
	AppModeDescribe
	AppModeEmbeddings
	AppModeQuery
	AppModeServer
)

type modeArgInfo struct {
	mode    AppMode
	addArgs int // number of additional required & non-flag arguments after the mode parameter
}

var (
	dbPath       = flag.String("db", "./henri.db", "Path to database")
	llamaServer  = flag.String("llama", "", "Address of running llama server, typically http://localhost:8080")
	llamaSeed    = flag.Int("seed", 385480504, "Random seed to llama")
	ollamaServer = flag.String("ollama", "", "Address of running ollama server, typically http://localhost:11434")
	openAI       = flag.Bool("openai", false, "Use OpenAI (only embedding and search)")
	count        = flag.Int("count", -1, "Number of items to process, defaul is no limit")

	modeArgs = map[string]modeArgInfo{
		"scan":       {AppModeScan, 1},
		"sc":         {AppModeScan, 1},
		"describe":   {AppModeDescribe, 0},
		"d":          {AppModeDescribe, 0},
		"embeddings": {AppModeEmbeddings, 0},
		"e":          {AppModeEmbeddings, 0},
		"query":      {AppModeQuery, 1},
		"q":          {AppModeQuery, 1},
		"server":     {AppModeServer, 0},
		"s":          {AppModeServer, 0},
	}

	lameduck bool
)

// Walk the filesystem from root finding all supported image files.
func findAndInsertImageFiles(ctx context.Context, root string, db *henri.DB) (int, error) {
	var (
		results []henri.ImagePath
		nn      int
	)

	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return filepath.SkipAll
		default:
			if lameduck {
				return filepath.SkipAll
			}
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" {
			result := henri.ImagePath{
				Path:    path,
				Modtime: info.ModTime(),
			}

			// Retrieve the image dimensions
			var (
				w, h int
			)
			w, h, err = imageDimensions(path)
			if err != nil {
				return fmt.Errorf("error reading image dimensions of %s - %s", path, err)
			}
			result.Width = w
			result.Height = h

			results = append(results, result)
			if len(results) == 200 {
				// Write this batch to the DB
				n, err := db.InsertImagePaths(ctx, results, len(results))
				if err != nil {
					return err
				}
				nn += n
				results = results[:0]
			}
		}

		return nil
	})

	if len(results) > 0 {
		n, err := db.InsertImagePaths(ctx, results, len(results))
		if err != nil {
			return 0, err
		}
		nn += n
	}

	return nn, err
}

// Retrieve the dimensions of the images
// Most of the time this will be JPEGs but in my photo library I found at least two PNGs that had a JPEG extension.
// Those should still be included.
func imageDimensions(imgPath string) (w int, h int, err error) {
	var f *os.File

	f, err = os.Open(imgPath)
	if err != nil {
		return
	}
	defer f.Close()

	var img image.Image
	img, _, err = image.Decode(f)
	if err != nil {
		return
	}

	bounds := img.Bounds()
	w = bounds.Max.X
	h = bounds.Max.Y

	return
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

func run(ctx context.Context, mode AppMode, h *henri.Henri) error {
	if mode == AppModeDescribe && h.Name() == "openai" {
		return fmt.Errorf("for privacy reasons OpenAI cannot be used for describing")
	}

	defer h.DB.Close()

	if mode == AppModeScan {
		if len(os.Args) < 2 {
			return fmt.Errorf("missing library path to scan")
		}
		imagecount, err := findAndInsertImageFiles(ctx, os.Args[2], h.DB)
		if err != nil {
			return err
		}
		fmt.Printf("Added %d new images\n", imagecount)
		return nil
	}

	// All functionality from this point on requires the LLM server. Check if
	// it is healthy.
	if !h.Describer.IsHealthy() {
		return fmt.Errorf("server is not responding")
	}

	if mode == AppModeQuery {
		if len(os.Args) < 2 {
			return fmt.Errorf("missing query string")
		}

		// Issue query
		if err := runQuery(os.Args[2], h.Describer, h.DB); err != nil {
			return err
		}

		return nil
	}

	var (
		images []*henri.Image
		workFn func(context.Context, describer.Describer, *henri.Image, *henri.DB) error
		err    error
	)

	switch mode {
	case AppModeDescribe:
		images, err = h.DB.ImagesToDescribe(ctx)
		workFn = describeImageFn
	case AppModeEmbeddings:
		images, err = h.DB.DescribedImagesMissingEmbeddings(ctx, h.Describer.Model())
		workFn = calcEmbeddingFn
	}
	if err != nil {
		return err
	}

	if *count > -1 {
		images = images[:min(len(images), *count)]
	}
	fmt.Printf("%d images to process\n", len(images))
	if len(images) > 0 {
		fmt.Printf("Using describer %s model %s\n", h.Describer.Name(), h.Describer.Model())
	}

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

		if err = workFn(ctx, h.Describer, img, h.DB); err != nil {
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
			fmt.Println("Stopping...")
			lameduck = true
		}
	}
}

func printUsageAndExit() {
	w := flag.CommandLine.Output()
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  henri scan, sc <library_path>    Recursively scan library_path for JPEG files")
	fmt.Fprintln(w, "  henri describe, d                Generate textual descriptions for images")
	fmt.Fprintln(w, "  henri embeddings, e              Generate embeddings from image descriptions")
	fmt.Fprintln(w, "  henri query, q <query>           Search embeddings using the query")
	fmt.Fprintln(w, "  henri server, s                  Start a web server on port 8080, override with PORT env var")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")

	flag.CommandLine.PrintDefaults()
	os.Exit(0)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("henri: ")

	flag.Usage = printUsageAndExit

	// First argument is the mode and required
	if len(os.Args) < 2 {
		flag.Usage()
	}

	modeinfo, ok := modeArgs[strings.ToLower(os.Args[1])]
	if !ok {
		fmt.Fprintf(os.Stderr, "unrecognized mode: %s\n", os.Args[1])
		flag.Usage()
	}

	// Parse command line args after the mode
	if err := flag.CommandLine.Parse(os.Args[2+modeinfo.addArgs:]); err != nil {
		log.Fatal(err)
	}

	hio := henri.InitOptions{
		DbPath:                *dbPath,
		LlamaServer:           *llamaServer,
		LlamaSeed:             *llamaSeed,
		OllamaServer:          *ollamaServer,
		OpenAI:                *openAI,
		NoSpecifiedBackendsOK: modeinfo.mode == AppModeScan,
		HttpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	sigch := make(chan os.Signal, 2)
	signal.Notify(sigch, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	go sighandler(sigch, cancel)

	h, err := henri.Init(ctx, hio)
	if err != nil {
		log.Fatal(err)
	}

	if modeinfo.mode == AppModeServer {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		srv := NewServer(h.Describer, h.DB, port)

		go func() {
			if err := srv.Start(); err != nil {
				log.Fatalf("Server failed to start - %s", err)
			}
		}()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Error at server shutdown - %s", err)
			}
		}()
		wg.Wait()
		os.Exit(0)
	}

	if err := run(ctx, modeinfo.mode, h); err != nil {
		log.Fatal(err)
	}
}
