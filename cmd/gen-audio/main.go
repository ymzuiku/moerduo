package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"time"

	"github.com/joho/godotenv"
	lf "github.com/ymzuiku/listening-first"
)

type Book struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Paragraphs []string `json:"paragraphs"`
}

func main() {
	godotenv.Load()

	raw, err := os.ReadFile("data/books/robinson-crusoe.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read book:", err)
		os.Exit(1)
	}
	var book Book
	json.Unmarshal(raw, &book)

	outDir := "data/audio/robinson-crusoe"
	os.MkdirAll(outDir, 0755)

	count := 10
	if count > len(book.Paragraphs) {
		count = len(book.Paragraphs)
	}

	for i := 0; i < count; i++ {
		outFile := fmt.Sprintf("%s/%04d.mp3", outDir, i)
		if info, err := os.Stat(outFile); err == nil && info.Size() > 0 {
			fmt.Printf("[%d/%d] skip (exists): %s\n", i+1, count, outFile)
			continue
		}

		fmt.Printf("[%d/%d] generating: %s\n", i+1, count, book.Paragraphs[i][:min(60, len(book.Paragraphs[i]))])
		audio, err := lf.TextToSpeech(context.Background(), lf.TextToSpeechRequest{
			Text:  book.Paragraphs[i],
			Speed: 0.9,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "TTS error sentence %d: %v\n", i, err)
			continue
		}

		if err := os.WriteFile(outFile, audio, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			continue
		}
		fmt.Printf("[%d/%d] saved: %s (%d bytes)\n", i+1, count, outFile, len(audio))
		time.Sleep(2 * time.Second)
	}
	fmt.Println("Done!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
