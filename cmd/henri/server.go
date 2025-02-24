//go:generate tailwindcss --input static/in.css --output static/tailwind.css

package main

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/chriskillpack/henri"
	"github.com/chriskillpack/henri/describer"
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
		cherr    = make(chan error, 2)
		wg       sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		queryvec, err = s.d.Embeddings(ctx, query)
		cherr <- err
	}()

	go func() {
		defer wg.Done()
		var err error
		eids, err = s.db.EmbeddingIdsForModel(ctx, s.d.Model())
		cherr <- err
	}()
	wg.Wait()
	close(cherr)

	for err := range cherr {
		if err != nil {
			return err
		}
	}
	_ = queryvec
	_ = eids

	return nil
}
