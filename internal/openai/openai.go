package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/chriskillpack/henri/describer"
)

const model = "text-embedding-3-small"

type openai struct {
	client *http.Client
	model  string
}

var (
	_ describer.Describer = &openai{}

	openai_key string // This will be populated in init()

	rl *rateLimiter // For requests to the OpenAI API

	// This map has dual purposes, first is to define which models are used
	// and two the size of the embedding vectors we wish
	modelDimensions = map[string]int{
		"text-embedding-3-small": 512,
	}
)

func init() {
	if openai_key == "" {
		openai_key = os.Getenv("OPENAI_API_KEY")
		if openai_key == "" {
			panic("OPENAI_API_KEY is not set")
		}
	}
}

func Init(httpClient *http.Client) *openai {
	if _, ok := modelDimensions[model]; !ok {
		panic("Unrecognized model")
	}

	rl = newRateLimiter(10, time.Minute)

	return &openai{client: httpClient, model: model}
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
	reqData := struct {
		Input      string `json:"input"`
		Model      string `json:"model"`
		Dimensions int    `json:"dimensions"`
	}{
		Input:      description,
		Model:      o.model,
		Dimensions: modelDimensions[o.model],
	}

	type embedding struct {
		Object string    `json:"object"`
		Vector []float32 `json:"embedding"`
	}

	type usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	}

	respData := struct {
		Object    string      `json:"object"`
		Embedding []embedding `json:"data"`
		Model     string      `json:"model"`
		Usage     usage       `json:"usage"`
	}{}

	// Rate limit use of the OpenAI API
	if err := rl.Acquire(ctx); err != nil {
		return nil, err
	}

	if err := o.sendRequest(ctx, "https://api.openai.com/v1/embeddings", reqData, &respData); err != nil {
		return nil, err
	}
	if respData.Embedding[0].Object != "embedding" {
		return nil, fmt.Errorf("unexpected object type %q", respData.Embedding[0].Object)
	}

	return respData.Embedding[0].Vector, nil
}

func (o *openai) sendRequest(ctx context.Context, path string, reqData, respData any) error {
	data, err := json.Marshal(reqData)
	if err != nil {
		return err
	}
	reqBody := bytes.NewReader(data)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openai_key)

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("bad response %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(respBody, respData); err != nil {
		return err
	}

	return nil
}
