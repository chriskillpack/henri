//go:generate tailwindcss --input static/in.css --output static/tailwind.css

package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
	"golang.org/x/sync/errgroup"
)

var (
	//go:embed tmpl/*.html
	tmplFS embed.FS

	//go:embed static
	staticFS embed.FS

	indexTmpl   *template.Template
	resultsTmpl *template.Template
)

type Server struct {
	hs     *http.Server
	d      describer.Describer
	db     *henri.DB
	logger *log.Logger
}

func init() {
	indexTmpl = template.Must(template.ParseFS(tmplFS, "tmpl/index.html"))
	resultsTmpl = template.Must(template.ParseFS(tmplFS, "tmpl/_results.html"))
}

func NewServer(d describer.Describer, db *henri.DB, port string) *Server {
	srv := &Server{
		d:      d,
		db:     db,
		logger: log.Default(),
	}

	srv.hs = &http.Server{
		Addr:    net.JoinHostPort("0.0.0.0", port),
		Handler: srv.serveHandler(),
	}

	return srv
}

func (s *Server) Start() error {
	return s.hs.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.hs.Shutdown(ctx)
}

func (s *Server) serveHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	mux.Handle("GET /search", s.serveSearch())
	mux.Handle("GET /image/{id}", s.serveImage())
	mux.Handle("GET /", s.serveRoot())

	return mux
}

func (s *Server) serveSearch() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		qvals := req.URL.Query()["q"]
		if len(qvals) == 0 {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		query := qvals[0]
		s.logger.Printf("query - %q\n", query)
		topk, err := s.runQuery(req.Context(), query, 5)
		if err != nil {
			s.logger.Printf("runQuery error - %s\n", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		type searchresult struct {
			Description   []string
			Score         float32
			ImageURL      string
			ImageCSSClass string
		}
		results := struct {
			Results []searchresult
		}{Results: make([]searchresult, 5)}
		for i, es := range topk.GetTopK() {
			w, h, err := imageDimensions(es.embed.Image.Path)
			if err != nil {
				continue
			}

			results.Results[i].Description = splitByNewline(es.embed.Image.Description)
			results.Results[i].Score = es.score
			results.Results[i].ImageURL = fmt.Sprintf("/image/%d", es.embed.ImageId)

			cssClass := "img-landscape"
			if h > w {
				cssClass = "img-portrait"
			}
			results.Results[i].ImageCSSClass = cssClass
		}
		resultsTmpl.Execute(w, results)
	}
}

func (s *Server) serveImage() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ids := req.PathValue("id")
		if len(ids) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		id, err := strconv.Atoi(ids)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		img, err := s.db.GetImage(req.Context(), id)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// s.logger.Printf("img id %d, path - %s\n", img.Id, img.Path)

		// Load the image off disk and return it
		data, err := os.ReadFile(img.Path)
		if err != nil {
			s.logger.Printf("Failed to read %s\n", img.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", http.DetectContentType(data))
		w.Write(data)
	}
}
func (s *Server) serveRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		indexTmpl.Execute(w, nil)
	}
}

func (s *Server) runQuery(ctx context.Context, query string, k int) (*TopKTracker, error) {
	g, _ := errgroup.WithContext(ctx)

	var (
		batch    henri.EmbeddingBatch
		batchCh  <-chan henri.EmbeddingBatch
		errCh    <-chan error
		queryvec []float32
		ok       bool
	)

	// Compute the embeddings for this query
	g.Go(func() error {
		var err error
		queryvec, err = s.d.Embeddings(ctx, query)
		return err
	})

	// Concurrently retrieve the first batch of embeddings for this model
	g.Go(func() error {
		batchCh, errCh = s.db.EmbeddingsForModel(ctx, s.d.Model(), 0)

		select {
		case err := <-errCh:
			return err

		case batch, ok = <-batchCh:
		}

		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("query error - %w", err)
	}

	// With the data collected we can start scoring. While the first batch is
	// being scored, concurrently the next batch will be fetched.
	topk := NewTopKTracker(k)
	for ok {
		// Fetch the next batch concurrently while computing scores for the current batch
		var nb henri.EmbeddingBatch
		g.Go(func() error {
			select {
			case err := <-errCh:
				return err
			case nb, ok = <-batchCh:
			}
			return nil
		})
		g.Go(func() error {
			for _, emb := range batch.Embeds {
				score, err := computeCosineSimilarity(queryvec, emb.Vector)
				if err != nil {
					return err
				}

				topk.ProcessItem(emb, score)
			}
			return nil
		})
		err := g.Wait()
		if err != nil {
			return nil, fmt.Errorf("scoring batches - %w", err)
		}

		// Intermediate batches will have batch.Done=false,ok=true
		// Final batch will have batch.Done=true,ok=true
		// One past the final batch will have batch.Done=false,ok=false,
		// because the batch channel will have been closed. The closed channel
		// also returns a zero value batch which has batch.Done=false.
		// Terminating condition for the loop is ok=false

		// Move the new batch over (TODO - should this all be pointers?)
		batch = nb
	}

	return topk, nil
}

// Splits s into separate substrings by newline character. Each substring is
// trimmed for whitespace and the results returned in a slice.
func splitByNewline(s string) []string {
	var sections []string
	for p := range strings.SplitSeq(s, "\n") {
		if p != "" {
			sections = append(sections, strings.TrimSpace(p))
		}
	}

	return sections
}

// Returns the dimensions of the JPEG at imgPath.
// TODO - this should be stored in the DB as part of injestion.
