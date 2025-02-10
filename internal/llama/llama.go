package llama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/chriskillpack/henri/describer"
)

const (
	promptPreamble = `This is a conversation between User and Llama, a friendly chatbot. Llama is helpful, kind, honest, good at writing, and never fails to answer any requests immediately and with precision.

User:`
	promptSuffix = `
Llama:`

	imagePreamble = `A chat between a curious human and an artificial intelligence assistant. The assistant gives helpful, detailed, and polite answers to the human's questions.
USER:`
	imageSuffix = `
ASSISTANT:`
)

type jsonmap map[string]any

// These were lifted from the web inspector for the server UI
var defaultparams = jsonmap{
	"n_predict":         400,
	"n_probs":           0,
	"temperature":       0.7,
	"stop":              []string{"</s>", "Llama:", "User:"},
	"repeat_last_n":     256,
	"repeat_penalty":    1.18,
	"top_k":             40,
	"top_p":             0.5,
	"tfs_z":             1,
	"typical_p":         1,
	"presence_penalty":  0,
	"frequency_penalty": 0,
	"mirostat":          0,
	"mirostat_tau":      5,
	"mirostat_eta":      0.1,
	"grammar":           "",
	"slot_id":           -1,
	"cache_prompt":      true,
}

type llama struct {
	srvAddr string
	seed    int

	client *http.Client
}

var _ describer.Describer = &llama{}

func Init(srvAddr string, seed int, httpClient *http.Client) *llama {
	return &llama{
		srvAddr: srvAddr,
		seed:    seed,
		client:  httpClient,
	}
}

/*
func prompt(ctx context.Context, query string) (string, error) {
	// Prompt doesn't have to be a streaming request, just kicking the tires of that code path
	return l.sendRequest(ctx, queryPrompt(query), true, map[string]any{})
}
*/

func (l *llama) Name() string { return "llama" }

func (l *llama) IsHealthy() bool {
	resp, err := http.Get(l.srvAddr)
	if err != nil {
		return false
	}

	return resp.StatusCode == http.StatusOK
}

func (l *llama) DescribeImage(ctx context.Context, image []byte) (string, error) {
	imb64 := base64.StdEncoding.EncodeToString(image)
	return l.sendRequest(ctx, imagePreamble+"[img-10]please describe this image in detail"+imageSuffix, false, jsonmap{
		"image_data": []jsonmap{
			{
				"data": imb64, "id": 10,
			},
		},
	})
}

func (l *llama) Embeddings(ctx context.Context, description string) ([]float32, error) {
	panic("Not implemented for llama")
}

// Use this with a text prompt
func queryPrompt(prompt string) string {
	return promptPreamble + prompt + promptSuffix
}

func (l *llama) sendRequest(ctx context.Context, prompt string, stream bool, keys jsonmap) (string, error) {
	data := maps.Clone(defaultparams)
	maps.Copy(data, keys)
	data["prompt"] = prompt
	data["stream"] = stream
	data["seed"] = l.seed

	buf := bytes.NewBuffer(make([]byte, 0, 2_000_000)) // The buffer will be resized by Encode
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(&data)
	if err != nil {
		return "", err
	}
	br := bytes.NewReader(buf.Bytes())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.srvAddr+"/completion", br)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	content := new(bytes.Buffer)
	respbody := struct {
		Content string
		Stop    bool
	}{}

	lr := bufio.NewScanner(resp.Body)
	for !respbody.Stop {
		// Read in one line
		if !lr.Scan() {
			return "", lr.Err()
		}
		line := lr.Text()
		// TODO: Is there a way to eliminate this check? The empty line appears after a JSON body
		if len(line) == 0 {
			continue
		}
		if stream {
			var found bool
			line, found = strings.CutPrefix(line, "data: ")
			if !found {
				return "", fmt.Errorf("missing `data: ` prefix")
			}
		}

		dec := json.NewDecoder(bytes.NewBufferString(line))
		if err := dec.Decode(&respbody); err != nil {
			return "", err
		}
		content.WriteString(respbody.Content)
	}

	return strings.TrimLeft(content.String(), " "), nil
}
