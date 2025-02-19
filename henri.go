package henri

import (
	"fmt"
	"net/http"

	"github.com/chriskillpack/henri/describer"
	"github.com/chriskillpack/henri/internal/llama"
	"github.com/chriskillpack/henri/internal/ollama"
	"github.com/chriskillpack/henri/internal/openai"
)

type InitOptions struct {
	LlamaServer string
	LlamaSeed   int

	OllamaServer string

	OpenAI bool

	HttpClient *http.Client // if nil uses http.DefaultClient
}

type Henri struct {
	describer.Describer
}

func Init(hio InitOptions) (*Henri, error) {
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

	return h, nil
}
