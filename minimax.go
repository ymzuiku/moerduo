package listeningfirst

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

var minimaxBaseURL = "https://api.minimax.io"

func setMinimaxBaseURL(url string) { minimaxBaseURL = url }

func minimaxAuthHeader() string {
	return "Bearer " + os.Getenv("MINIMAX_API_KEY")
}

// ── Image Generation ──────────────────────────────────────────────────────

// GenerateImageRequest holds parameters for image generation.
type GenerateImageRequest struct {
	Prompt          string `json:"prompt"`
	AspectRatio     string `json:"aspect_ratio,omitempty"`
	N               int    `json:"n,omitempty"`
	PromptOptimizer bool   `json:"prompt_optimizer,omitempty"`
}

// GenerateImageResult holds the generated image URLs.
type GenerateImageResult struct {
	ImageURLs []string
}

// GenerateImage calls the MiniMax image generation API.
func GenerateImage(ctx context.Context, req GenerateImageRequest) (*GenerateImageResult, error) {
	if req.N <= 0 {
		req.N = 1
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "1:1"
	}

	body, _ := json.Marshal(map[string]any{
		"model":            "image-01",
		"prompt":           req.Prompt,
		"aspect_ratio":     req.AspectRatio,
		"response_format":  "url",
		"n":                req.N,
		"prompt_optimizer": req.PromptOptimizer,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		minimaxBaseURL+"/v1/image_generation", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", minimaxAuthHeader())

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("MiniMax image API error: status %d, body: %s", resp.StatusCode, buf.String())
	}

	var result struct {
		Data struct {
			ImageURLs []string `json:"image_urls"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode image response: %w", err)
	}
	return &GenerateImageResult{ImageURLs: result.Data.ImageURLs}, nil
}

// ── Text-to-Speech ────────────────────────────────────────────────────────

// TextToSpeechRequest holds parameters for TTS generation.
type TextToSpeechRequest struct {
	Text    string  `json:"text"`
	VoiceID string  `json:"voice_id,omitempty"`
	Format  string  `json:"format,omitempty"`
	Speed   float64 `json:"speed,omitempty"`
}

// TextToSpeechResult holds the generated audio as hex-encoded data.
type TextToSpeechResult struct {
	AudioHex string
}

// TextToSpeech calls the MiniMax TTS API and returns hex-encoded audio.
func TextToSpeech(ctx context.Context, req TextToSpeechRequest) (*TextToSpeechResult, error) {
	if req.VoiceID == "" {
		req.VoiceID = "English_expressive_narrator"
	}
	if req.Format == "" {
		req.Format = "mp3"
	}
	if req.Speed == 0 {
		req.Speed = 1.0
	}

	body, _ := json.Marshal(map[string]any{
		"model":  "speech-2.8-hd",
		"text":   req.Text,
		"stream": false,
		"voice_setting": map[string]any{
			"voice_id": req.VoiceID,
			"speed":    req.Speed,
			"vol":      1,
			"pitch":    0,
		},
		"audio_setting": map[string]any{
			"sample_rate": 32000,
			"bitrate":     128000,
			"format":      req.Format,
			"channel":     1,
		},
		"output_format": "hex",
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		minimaxBaseURL+"/v1/t2a_v2", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", minimaxAuthHeader())

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("MiniMax TTS API error: status %d, body: %s", resp.StatusCode, buf.String())
	}

	var result struct {
		Data struct {
			Audio string `json:"audio"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode TTS response: %w", err)
	}
	return &TextToSpeechResult{AudioHex: result.Data.Audio}, nil
}
