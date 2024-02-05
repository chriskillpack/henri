package henri

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"strings"
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
	"seed":              llamaSeed,
}

var (
	llamaSeed    int
	llamaSrvAddr string
)

func Init(srvAddr string, seed int) {
	llamaSrvAddr = srvAddr
	llamaSeed = seed
}

func Prompt(ctx context.Context, query string) (string, error) {
	return sendRequest(ctx, queryPrompt("tell me a story"), map[string]any{})
}

func IsHealthy() bool {
	resp, err := http.Get(llamaSrvAddr)
	if err != nil {
		return false
	}

	return resp.StatusCode == http.StatusOK
}

func DescribeImage(ctx context.Context, image []byte) (string, error) {
	imb64 := base64.StdEncoding.EncodeToString(image)
	return sendRequest(ctx, imagePreamble+"[img-10]please describe this image in detail"+imageSuffix, jsonmap{
		"image_data": []jsonmap{
			{
				"data": imb64, "id": 10,
			},
		},
	})
}

// Use this with a text prompt
func queryPrompt(prompt string) string {
	return promptPreamble + prompt + promptSuffix
}

func sendRequest(ctx context.Context, prompt string, keys jsonmap) (string, error) {
	data := maps.Clone(defaultparams)
	maps.Copy(data, keys)
	data["prompt"] = prompt

	buf := bytes.NewBuffer(make([]byte, 0, 2_000_000)) // The buffer will be resized by Encode
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(&data)
	if err != nil {
		return "", err
	}
	br := bytes.NewReader(buf.Bytes())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, llamaSrvAddr+"/completion", br)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	respbody := struct {
		Content string
	}{}
	if err := dec.Decode(&respbody); err != nil {
		return "", err
	}

	return strings.TrimLeft(respbody.Content, " "), nil
}
