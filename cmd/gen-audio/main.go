package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	lf "github.com/ymzuiku/listening-first"
)

type Book struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Sentences []string `json:"sentences"`
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
	if count > len(book.Sentences) {
		count = len(book.Sentences)
	}

	for i := 0; i < count; i++ {
		outFile := fmt.Sprintf("%s/%04d.mp3", outDir, i)
		if _, err := os.Stat(outFile); err == nil {
			fmt.Printf("[%d/%d] skip (exists): %s\n", i+1, count, outFile)
			continue
		}

		fmt.Printf("[%d/%d] generating: %s\n", i+1, count, book.Sentences[i][:min(60, len(book.Sentences[i]))])
		result, err := lf.TextToSpeech(context.Background(), lf.TextToSpeechRequest{
			Text:  book.Sentences[i],
			Speed: 0.9,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "TTS error sentence %d: %v\n", i, err)
			continue
		}

		audio, err := hex.DecodeString(result.AudioHex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "decode hex error sentence %d: %v\n", i, err)
			continue
		}

		if err := os.WriteFile(outFile, audio, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			continue
		}
		fmt.Printf("[%d/%d] saved: %s (%d bytes)\n", i+1, count, outFile, len(audio))
	}
	fmt.Println("Done!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
