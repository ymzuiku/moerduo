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

	prompt := strings.Join(os.Args[1:], " ")
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "usage: image <prompt>")
		os.Exit(1)
	}

	result, err := lf.GenerateImage(context.Background(), lf.GenerateImageRequest{
		Prompt: prompt,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	for _, url := range result.ImageURLs {
		fmt.Println(url)
	}
}
