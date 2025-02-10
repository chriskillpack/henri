package henri

import (
	"fmt"
	"net/http"

	"github.com/chriskillpack/henri/describer"
	"github.com/chriskillpack/henri/internal/llama"
	"github.com/chriskillpack/henri/internal/ollama"
)

type InitOptions struct {
	LlamaServer string
	LlamaSeed   int

	OllamaServer string

	httpClient *http.Client // if nil uses http.DefaultClient
}

type Henri struct {
	describer.Describer
}

func Init(hio InitOptions) (*Henri, error) {
	h := &Henri{}

	if hio.LlamaServer == "" && hio.OllamaServer == "" {
		return nil, fmt.Errorf("needs llama or ollama server")
	}
	if hio.LlamaServer != "" && hio.OllamaServer != "" {
		return nil, fmt.Errorf("cannot use both llama and ollama together")
	}

	httpClient := hio.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	if hio.LlamaServer != "" {
		h.Describer = llama.Init(hio.LlamaServer, hio.LlamaSeed, httpClient)
	}
	if hio.OllamaServer != "" {
		h.Describer = ollama.Init(hio.OllamaServer, httpClient)
	}

	return h, nil
}
