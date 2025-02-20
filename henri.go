package henri

import (
	"context"
	"fmt"
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

	HttpClient *http.Client // if nil uses http.DefaultClient
	DbPath     string       // if present, initialize the database
}

type Henri struct {
	DB *DB
	describer.Describer
}

func Init(ctx context.Context, hio InitOptions) (*Henri, error) {
	h := &Henri{}

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
		return nil, fmt.Errorf("no backend selected")
	case 1:
		// no-op
	default:
		return nil, fmt.Errorf("multiple backends selected, only one allowed")
	}

	if hio.OpenAI {
		h.Describer = openai.Init(httpClient)
	} else if hio.LlamaServer != "" {
		h.Describer = llama.Init(hio.LlamaServer, hio.LlamaSeed, httpClient)
	} else if hio.OllamaServer != "" {
		h.Describer = ollama.Init("llava", hio.OllamaServer, httpClient)
	}

	if hio.DbPath != "" {
		var err error
		h.DB, err = NewDB(ctx, hio.DbPath)
		if err != nil {
			return nil, err
		}
	}

	return h, nil
}
