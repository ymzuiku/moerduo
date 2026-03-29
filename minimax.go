package listeningfirst

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// ── Image Generation (OpenAI DALL-E) ─────────────────────────────────────

type GenerateImageRequest struct {
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
	N      int    `json:"n,omitempty"`
}

type GenerateImageResult struct {
	ImageURLs []string
}

func GenerateImage(ctx context.Context, req GenerateImageRequest) (*GenerateImageResult, error) {
	apiKey := openaiKey()
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_KEY not set")
	}
	if req.N <= 0 {
		req.N = 1
	}
	if req.Size == "" {
		req.Size = "1024x1024"
	}

	body, _ := json.Marshal(map[string]any{
		"model":           "dall-e-2",
		"prompt":          req.Prompt,
		"size":            req.Size,
		"n":               req.N,
		"response_format": "url",
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openaiBaseURL+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("OpenAI image API error: status %d, body: %s", resp.StatusCode, buf.String())
	}

	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode image response: %w", err)
	}
	urls := make([]string, len(result.Data))
	for i, d := range result.Data {
		urls[i] = d.URL
	}
	return &GenerateImageResult{ImageURLs: urls}, nil
}

// ── Text-to-Speech (OpenAI) ─────────────────────────────────────────────

type TextToSpeechRequest struct {
	Text         string  `json:"text"`
	Model        string  `json:"model,omitempty"`
	Voice        string  `json:"voice,omitempty"`
	Speed        float64 `json:"speed,omitempty"`
	Instructions string  `json:"instructions,omitempty"`
}

func TextToSpeech(ctx context.Context, req TextToSpeechRequest) ([]byte, error) {
	apiKey := os.Getenv("OPENAI_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_KEY not set")
	}
	if req.Voice == "" {
		req.Voice = "nova"
	}
	if req.Model == "" {
		req.Model = "tts-1-hd"
	}
	if req.Speed == 0 {
		req.Speed = 1.0
	}

	params := map[string]any{
		"model":           req.Model,
		"input":           req.Text,
		"voice":           req.Voice,
		"speed":           req.Speed,
		"response_format": "mp3",
	}
	if req.Instructions != "" && req.Model == "gpt-4o-mini-tts" {
		params["instructions"] = req.Instructions
	}
	body, _ := json.Marshal(params)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openaiBaseURL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI TTS API error: status %d, body: %s", resp.StatusCode, buf.String())
	}

	return buf.Bytes(), nil
}
