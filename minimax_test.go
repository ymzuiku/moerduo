package listeningfirst

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateImage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/image_generation" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing Authorization header")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"image_urls": []string{"https://example.com/img.png"},
			},
		})
	}))
	defer srv.Close()

	orig := minimaxBaseURL
	defer func() { setMinimaxBaseURL(orig) }()
	setMinimaxBaseURL(srv.URL)

	t.Setenv("MINIMAX_API_KEY", "test-key")

	result, err := GenerateImage(context.Background(), GenerateImageRequest{
		Prompt: "a cat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ImageURLs) != 1 || result.ImageURLs[0] != "https://example.com/img.png" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGenerateImage_Defaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["aspect_ratio"] != "1:1" {
			t.Fatalf("expected default aspect_ratio 1:1, got %v", body["aspect_ratio"])
		}
		if body["n"] != float64(1) {
			t.Fatalf("expected default n=1, got %v", body["n"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"image_urls": []string{"https://example.com/img.png"}},
		})
	}))
	defer srv.Close()

	orig := minimaxBaseURL
	defer func() { setMinimaxBaseURL(orig) }()
	setMinimaxBaseURL(srv.URL)
	t.Setenv("MINIMAX_API_KEY", "test-key")

	_, err := GenerateImage(context.Background(), GenerateImageRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateImage_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	orig := minimaxBaseURL
	defer func() { setMinimaxBaseURL(orig) }()
	setMinimaxBaseURL(srv.URL)
	t.Setenv("MINIMAX_API_KEY", "test-key")

	_, err := GenerateImage(context.Background(), GenerateImageRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTextToSpeech_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/t2a_v2" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"audio": "deadbeef"},
		})
	}))
	defer srv.Close()

	orig := minimaxBaseURL
	defer func() { setMinimaxBaseURL(orig) }()
	setMinimaxBaseURL(srv.URL)
	t.Setenv("MINIMAX_API_KEY", "test-key")

	result, err := TextToSpeech(context.Background(), TextToSpeechRequest{
		Text: "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AudioHex != "deadbeef" {
		t.Fatalf("unexpected audio: %s", result.AudioHex)
	}
}

func TestTextToSpeech_Defaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		vs := body["voice_setting"].(map[string]any)
		if vs["voice_id"] != "English_expressive_narrator" {
			t.Fatalf("unexpected voice_id: %v", vs["voice_id"])
		}
		as := body["audio_setting"].(map[string]any)
		if as["format"] != "mp3" {
			t.Fatalf("unexpected format: %v", as["format"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"audio": "aabb"},
		})
	}))
	defer srv.Close()

	orig := minimaxBaseURL
	defer func() { setMinimaxBaseURL(orig) }()
	setMinimaxBaseURL(srv.URL)
	t.Setenv("MINIMAX_API_KEY", "test-key")

	_, err := TextToSpeech(context.Background(), TextToSpeechRequest{Text: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTextToSpeech_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	orig := minimaxBaseURL
	defer func() { setMinimaxBaseURL(orig) }()
	setMinimaxBaseURL(srv.URL)
	t.Setenv("MINIMAX_API_KEY", "test-key")

	_, err := TextToSpeech(context.Background(), TextToSpeechRequest{Text: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
}
