package describer

import "context"

// Describer describes an image used a specific LLM.
type Describer interface {
	// Name returns the name of the backing LLM, e.g. "llama" or "ollama"
	Name() string

	// Returns the model identifier, e.g. llava-7b, llama-13b, gpt-4o-mini
	Model() string

	// DescribeImage returns a string contains an English description of the
	// provided image. The image data should be the full contents of a JPEG file
	// including the header. The provided ctx is used as a parent context for
	// the request to the LLM server.
	DescribeImage(ctx context.Context, image []byte) (string, error)

	// Embeddings returns the embeddings vector for the given text.
	Embeddings(ctx context.Context, description string) ([]float32, error)

	// IsHealthy returns whether the LLM server is healthy.
	IsHealthy() bool
}
