package henri

import (
	"context"
	"errors"
	"net/http"

	"github.com/chriskillpack/henri/describer"
	"github.com/chriskillpack/henri/internal/llama"
	"github.com/chriskillpack/henri/internal/ollama"
	"github.com/chriskillpack/henri/internal/openai"
)

type InitOptions struct {
	LlamaServer  string // Address of Llama server
	LlamaSeed    int    // Seed to use with LLama (legacy behavior)
	OllamaServer string // Address of Ollama Server
	OpenAI       bool   // Should use OpenAI API platform

	// Setting this to true removes the requirement that at least one backend
	// is specified. Certain app modes may not require a backend.
	NoSpecifiedBackendsOK bool

	HttpClient *http.Client // if nil uses http.DefaultClient
	DbPath     string       // if present, initialize the database
}

type Henri struct {
	DB *DB
	describer.Describer
}

func Init(ctx context.Context, hio InitOptions) (*Henri, error) {
	if hio.DbPath == "" {
		return nil, errors.New("no database specified")
	}

	h := &Henri{}

	var err error
	if h.DB, err = NewDB(ctx, hio.DbPath); err != nil {
		return nil, err
	}

	httpClient := hio.HttpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	var n int
	if hio.OpenAI {
		n++
	}
	if hio.LlamaServer != "" {
		n++
	}
	if hio.OllamaServer != "" {
		n++
	}
	switch n {
	case 0:
		if !hio.NoSpecifiedBackendsOK {
			return nil, errors.New("no backend selected")
		}
	case 1:
		// no-op
	default:
		return nil, errors.New("multiple backends selected, only one allowed")
	}

	if hio.OpenAI {
		h.Describer = openai.Init(httpClient)
	} else if hio.LlamaServer != "" {
		h.Describer = llama.Init(hio.LlamaServer, hio.LlamaSeed, httpClient)
	} else if hio.OllamaServer != "" {
		h.Describer = ollama.Init("llava", hio.OllamaServer, httpClient)
	}

	return h, nil
}
