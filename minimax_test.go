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
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"url": "https://example.com/img.png"},
			},
		})
	}))
	defer srv.Close()

	orig := openaiBaseURL
	defer func() { openaiBaseURL = orig }()
	openaiBaseURL = srv.URL
	t.Setenv("OPENAI_KEY", "test-key")

	result, err := GenerateImage(context.Background(), GenerateImageRequest{Prompt: "a cat"})
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
		if body["size"] != "1024x1024" {
			t.Fatalf("expected default size 1024x1024, got %v", body["size"])
		}
		if body["n"] != float64(1) {
			t.Fatalf("expected default n=1, got %v", body["n"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"url": "https://example.com/img.png"}},
		})
	}))
	defer srv.Close()

	orig := openaiBaseURL
	defer func() { openaiBaseURL = orig }()
	openaiBaseURL = srv.URL
	t.Setenv("OPENAI_KEY", "test-key")

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

	orig := openaiBaseURL
	defer func() { openaiBaseURL = orig }()
	openaiBaseURL = srv.URL
	t.Setenv("OPENAI_KEY", "test-key")

	_, err := GenerateImage(context.Background(), GenerateImageRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTextToSpeech_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte("fake-mp3-data"))
	}))
	defer srv.Close()

	orig := openaiBaseURL
	defer func() { openaiBaseURL = orig }()
	openaiBaseURL = srv.URL
	t.Setenv("OPENAI_KEY", "test-key")

	audio, err := TextToSpeech(context.Background(), TextToSpeechRequest{Text: "Hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(audio) != "fake-mp3-data" {
		t.Fatalf("unexpected audio: %s", string(audio))
	}
}

func TestTextToSpeech_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	orig := openaiBaseURL
	defer func() { openaiBaseURL = orig }()
	openaiBaseURL = srv.URL
	t.Setenv("OPENAI_KEY", "test-key")

	_, err := TextToSpeech(context.Background(), TextToSpeechRequest{Text: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
}
