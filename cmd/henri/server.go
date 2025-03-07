//go:generate tailwindcss --input static/in.css --output static/tailwind.css

package main

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net"
	"net/http"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
	"golang.org/x/sync/errgroup"
)

var (
	//go:embed tmpl/*.html
	tmplFS embed.FS

	//go:embed static
	staticFS embed.FS

	indexTmpl *template.Template
)

type Server struct {
	hs     *http.Server
	d      describer.Describer
	db     *henri.DB
	logger *log.Logger
}

func init() {
	indexTmpl = template.Must(template.ParseFS(tmplFS, "tmpl/index.html"))
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

	/*
		// Test that we can retrieve all the Embeddings (with associated Images for a model)
		batchC, errC := db.EmbeddingsForModel(context.TODO(), d.Model(), 0)
		ok := true
		for ok {
			select {
			case err := <-errC:
				fmt.Printf("had an error %q", err)
				ok = false
				break
			case batch := <-batchC:
				for i, emb := range batch.Embeds[:min(len(batch.Embeds), 10)] {
					fmt.Printf("%d: %d, %d, %s\n", i, emb.Id, emb.ImageId, emb.Image.Description[0:min(len(emb.Image.Description), 30)])
				}
				fmt.Printf("Last seen: %d\n", batch.LastIDSeen)
				ok = !batch.Done
			}
		}
	*/

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
		if err := s.runQuery(req.Context(), query); err != nil {
			s.logger.Printf("runQuery error - %s\n", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) serveRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		indexTmpl.Execute(w, nil)
	}
}

func (s *Server) runQuery(ctx context.Context, query string) error {
	var (
		queryvec []float32
		eids     []int
	)

	g, _ := errgroup.WithContext(ctx)
	// Compute the embeddings for this query
	g.Go(func() error {
		var err error
		queryvec, err = s.d.Embeddings(ctx, query)
		return err
	})

	// Concurrently retrieve the embedding for this model
	g.Go(func() error {
		batchCh, errCh := s.db.EmbeddingsForModel(ctx, s.d.Model(), 0)

		for {
			select {
			case err := <-errCh:
				return err

			case batch, ok := <-batchCh:
				if !ok {
					// Done
					break
				}

				for _, emb := range batch.Embeds {
					emb = emb
				}
			}
		}

		var err error
		eids, err = s.db.EmbeddingIdsForModel(ctx, s.d.Model())
		return err
	})
	if err := g.Wait(); err != nil {
		return err
	}

	_ = queryvec
	_ = eids

	return nil
}
