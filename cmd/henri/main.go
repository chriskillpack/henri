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

func run(ctx context.Context, mode AppMode, h *henri.Henri) error {
	if mode == AppModeDescribe && h.Name() == "openai" {
		return fmt.Errorf("for privacy reasons OpenAI cannot be used for describing")
	}

	defer h.DB.Close()

	if mode == AppModeScan {
		if len(os.Args) < 2 {
			return fmt.Errorf("missing library path to scan")
		}
		photos, mtimes, err := findJpegFiles(os.Args[2])
		if err != nil {
			return err
		}
		fmt.Printf("Found %d images on disk\n", len(photos))

		const batchSize = 100
		added, err := h.DB.InsertImagePaths(ctx, photos, mtimes, batchSize)
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

	fmt.Printf("%t\n", *openAI)

	hio := henri.InitOptions{
		LlamaServer:  *llamaServer,
		LlamaSeed:    *llamaSeed,
		OllamaServer: *ollamaServer,
		OpenAI:       *openAI,
		HttpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		DbPath: *dbPath,
	}

	sigch := make(chan os.Signal, 2)
	signal.Notify(sigch, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	go sighandler(sigch, cancel)

	h, err := henri.Init(ctx, hio)
	if err != nil {
		log.Fatal(err)
	}

	if err := run(ctx, modeinfo.mode, h); err != nil {
		log.Fatal(err)
	}
}
