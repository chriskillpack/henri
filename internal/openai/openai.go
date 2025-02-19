package openai

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/chriskillpack/henri/describer"

	oagc "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const model = "text-embedding-3-small"

type openai struct {
	oac   *oagc.Client
	model string
}

var (
	_ describer.Describer = &openai{}

	rl *rateLimiter // For requests to the OpenAI API

	// This map has dual purposes, first is to define which models are used
	// and two the size of the embedding vectors we wish
	modelDimensions = map[string]int{
		"text-embedding-3-small": 512,
	}
)

func Init(httpClient *http.Client) *openai {
	if _, ok := modelDimensions[model]; !ok {
		panic("Unrecognized model")
	}

	rl = newRateLimiter(20, time.Minute)

	return &openai{
		oac: oagc.NewClient(
			option.WithHTTPClient(httpClient),
		),
		model: model,
	}
}

func (o *openai) Name() string { return "openai" }

func (o *openai) Model() string { return model }

func (o *openai) DescribeImage(ctx context.Context, image []byte) (string, error) {
	panic("not implemented for privacy reasons")
}

func (o *openai) IsHealthy() bool {
	// TODO
	return true
}

func (o *openai) Embeddings(ctx context.Context, description string) ([]float32, error) {
	// Rate limit use of the OpenAI API
	if err := rl.Acquire(ctx); err != nil {
		return nil, err
	}

	enp := oagc.EmbeddingNewParams{
		Input:      oagc.F(oagc.EmbeddingNewParamsInputUnion(oagc.EmbeddingNewParamsInputArrayOfStrings{description})),
		Model:      oagc.F(oagc.EmbeddingModel(o.model)),
		Dimensions: oagc.Int(int64(modelDimensions[o.model])),
	}
	resp, err := o.oac.Embeddings.New(ctx, enp)
	if err != nil {
		return nil, err
	}
	if resp.Data[0].Object != oagc.EmbeddingObjectEmbedding {
		return nil, fmt.Errorf("unexpected object type %q", resp.Data[0].Object)
	}

	// Convert the float64 embedding vector to float32
	embs := make([]float32, len(resp.Data[0].Embedding))
	for i, em := range resp.Data[0].Embedding {
		embs[i] = float32(em)
	}

	return embs, nil
}
