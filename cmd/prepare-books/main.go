package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type Book struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Sentences []string `json:"sentences"`
}

func main() {
	os.MkdirAll("data/books", 0755)

	// Robinson Crusoe from Project Gutenberg
	fmt.Println("Downloading Robinson Crusoe...")
	rc := downloadText("https://www.gutenberg.org/files/521/521-0.txt")
	rc = extractGutenberg(rc)
	rcSentences := splitSentences(rc)
	writeBook("data/books/robinson-crusoe.json", Book{
		ID:        "robinson-crusoe",
		Title:     "Robinson Crusoe",
		Sentences: rcSentences,
	})
	fmt.Printf("Robinson Crusoe: %d sentences\n", len(rcSentences))

	// The Little Prince from Internet Archive
	fmt.Println("Downloading The Little Prince...")
	lp := downloadText("https://archive.org/download/TheLittlePrince-English/littleprince_djvu.txt")
	// Remove the intro/metadata before the story
	if idx := strings.Index(lp, "Once when I was six years old"); idx >= 0 {
		lp = lp[idx:]
	}
	lpSentences := splitSentences(lp)
	writeBook("data/books/the-little-prince.json", Book{
		ID:        "the-little-prince",
		Title:     "The Little Prince",
		Sentences: lpSentences,
	})
	fmt.Printf("The Little Prince: %d sentences\n", len(lpSentences))
}

func downloadText(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "download error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func extractGutenberg(text string) string {
	start := strings.Index(text, "*** START OF THE PROJECT GUTENBERG EBOOK")
	if start >= 0 {
		nl := strings.Index(text[start:], "\n")
		if nl >= 0 {
			text = text[start+nl+1:]
		}
	}
	end := strings.Index(text, "*** END OF THE PROJECT GUTENBERG EBOOK")
	if end >= 0 {
		text = text[:end]
	}
	return strings.TrimSpace(text)
}

var sentenceEnd = regexp.MustCompile(`([.!?]["']?)\s+`)

func splitSentences(text string) []string {
	// Normalize whitespace
	ws := regexp.MustCompile(`\s+`)
	text = ws.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	parts := sentenceEnd.Split(text, -1)
	delimiters := sentenceEnd.FindAllStringSubmatch(text, -1)

	var sentences []string
	for i, part := range parts {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		// Re-attach the delimiter
		if i < len(delimiters) {
			s += delimiters[i][1]
		}
		s = strings.TrimSpace(s)
		if len(s) > 3 {
			sentences = append(sentences, s)
		}
	}
	return sentences
}

func writeBook(path string, book Book) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(book)
}
