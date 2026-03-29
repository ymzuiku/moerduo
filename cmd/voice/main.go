package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	lf "github.com/ymzuiku/listening-first"
)

func main() {
	godotenv.Load()

	text := strings.Join(os.Args[1:], " ")
	if text == "" {
		fmt.Fprintln(os.Stderr, "usage: voice <text>")
		os.Exit(1)
	}

	audio, err := lf.TextToSpeech(context.Background(), lf.TextToSpeechRequest{
		Text: text,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	outFile := "output.mp3"
	if err := os.WriteFile(outFile, audio, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write file error:", err)
		os.Exit(1)
	}
	fmt.Println("saved:", outFile)
}
