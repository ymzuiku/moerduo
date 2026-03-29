package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Book struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Cover        string   `json:"cover"`
	Paragraphs   []string `json:"paragraphs"`
	Translations []string `json:"translations,omitempty"`
}

func main() {
	godotenv.Load()
	os.MkdirAll("data/books", 0755)

	// Robinson Crusoe
	fmt.Println("Downloading Robinson Crusoe...")
	rc := downloadText("https://www.gutenberg.org/files/521/521-0.txt")
	rc = extractGutenberg(rc)
	if idx := strings.Index(rc, "I was born"); idx >= 0 {
		rc = rc[idx:]
	}
	rcText := normalizeWhitespace(rc)
	rcSentences := splitSentences(rcText, 10)
	rcParagraphs := splitSentencesIntoGroups(rcSentences)
	rcTranslations := aiTranslateBatch(rcParagraphs)
	writeBook("data/books/robinson-crusoe.json", Book{
		ID:           "robinson-crusoe",
		Title:        "Robinson Crusoe",
		Cover:        "covers/robinson-crusoe.jpg",
		Paragraphs:   rcParagraphs,
		Translations: rcTranslations,
	})
	fmt.Printf("Robinson Crusoe: %d paragraphs\n", len(rcParagraphs))

	// The Little Prince
	fmt.Println("Downloading The Little Prince...")
	lp := downloadText("https://archive.org/download/TheLittlePrince-English/littleprince_djvu.txt")
	if idx := strings.Index(lp, "Once when I was six years old"); idx >= 0 {
		lp = lp[idx:]
	}
	lpText := normalizeWhitespace(lp)
	lpSentences := splitSentences(lpText, 10)
	lpParagraphs := splitSentencesIntoGroups(lpSentences)
	lpTranslations := aiTranslateBatch(lpParagraphs)
	writeBook("data/books/the-little-prince.json", Book{
		ID:           "the-little-prince",
		Title:        "The Little Prince",
		Cover:        "covers/the-little-prince.jpg",
		Paragraphs:   lpParagraphs,
		Translations: lpTranslations,
	})
	fmt.Printf("The Little Prince: %d paragraphs\n", len(lpParagraphs))
}

func normalizeWhitespace(text string) string {
	ws := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(ws.ReplaceAllString(text, " "))
}

// splitSentences extracts the first N sentences from text (split by period + space + uppercase).
func splitSentences(text string, n int) []string {
	sentenceEnd := regexp.MustCompile(`[.!?]["']?\s+(?:[A-Z"'])`)
	var sentences []string
	for i := 0; i < n; i++ {
		loc := sentenceEnd.FindStringIndex(text)
		if loc == nil {
			s := strings.TrimSpace(text)
			if s != "" {
				sentences = append(sentences, s)
			}
			break
		}
		// Include up to the punctuation mark
		end := loc[0] + 1 // include the period
		for end < loc[1]-1 && text[end] == '"' || text[end] == '\'' {
			end++
		}
		sentences = append(sentences, strings.TrimSpace(text[:end]))
		text = strings.TrimSpace(text[end:])
	}
	return sentences
}

// splitSentencesIntoGroups takes each sentence and splits long ones into meaning groups via AI.
func splitSentencesIntoGroups(sentences []string) []string {
	var all []string
	for i, s := range sentences {
		words := len(strings.Fields(s))
		if words <= 15 {
			// Short enough, keep as-is
			fmt.Printf("[%d] keep (%dw): %s\n", i, words, truncate(s, 60))
			all = append(all, s)
		} else {
			// Call AI to split into meaning groups
			fmt.Printf("[%d] splitting (%dw): %s\n", i, words, truncate(s, 60))
			groups := aiSplitOne(s)
			if len(groups) == 0 {
				all = append(all, s) // fallback: keep whole
			} else {
				for _, g := range groups {
					fmt.Printf("  -> (%dw) %s\n", len(strings.Fields(g)), truncate(g, 60))
				}
				all = append(all, groups...)
			}
		}
	}
	return all
}

// aiTranslateBatch translates all paragraphs to Chinese via MiniMax M2.7 in one call.
func aiTranslateBatch(paragraphs []string) []string {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		return nil
	}

	fmt.Printf("Translating %d paragraphs...\n", len(paragraphs))

	numbered := ""
	for i, p := range paragraphs {
		numbered += fmt.Sprintf("%d. %s\n", i+1, p)
	}

	prompt := fmt.Sprintf(`你是一个英译中翻译工具。把下面的英文片段逐条翻译成中文。

要求：
1. 保持编号和顺序完全一致
2. 翻译要自然流畅，符合中文表达习惯
3. 只输出一个 JSON 数组（字符串数组），不要输出任何其他内容
4. 数组长度必须和输入条数一致（%d条）

原文：
%s`, len(paragraphs), numbered)

	body, _ := json.Marshal(map[string]any{
		"model":      "MiniMax-M2.7",
		"max_tokens": 16000,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequest(http.MethodPost,
		"https://api.minimax.io/anthropic/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2024-06-01")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Translate error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "  Translate error: status %d, %s\n", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
		return nil
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil
	}

	var content string
	for _, b := range result.Content {
		if b.Type == "text" && b.Text != "" {
			content = b.Text
			break
		}
	}
	content = strings.TrimSpace(content)

	if idx := strings.Index(content, "["); idx >= 0 {
		content = content[idx:]
	}
	if idx := strings.LastIndex(content, "]"); idx >= 0 {
		content = content[:idx+1]
	}

	var translations []string
	if err := json.Unmarshal([]byte(content), &translations); err != nil {
		fmt.Fprintf(os.Stderr, "  Translate parse error: %v\n", err)
		return nil
	}

	if len(translations) != len(paragraphs) {
		fmt.Fprintf(os.Stderr, "  Translate count mismatch: got %d, want %d\n", len(translations), len(paragraphs))
	}

	for i, t := range translations {
		fmt.Printf("  [%d] %s\n", i, t)
	}
	return translations
}

// aiSplitOne calls MiniMax M2.7 to split one sentence into meaning groups of 8-15 words.
func aiSplitOne(sentence string) []string {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		return nil
	}

	prompt := fmt.Sprintf(`你是一个英语分句工具。把下面的英文句子按意群拆分成多个片段。

要求：
1. 每个片段 8-15 个单词
2. 在自然停顿处断开（逗号、分号、关系从句等）
3. 所有片段拼接起来必须和原句完全一致，一个字符都不能多也不能少
4. 只输出一个 JSON 数组，不要输出任何其他内容

原句：%s`, sentence)

	body, _ := json.Marshal(map[string]any{
		"model":      "MiniMax-M2.7",
		"max_tokens": 16000,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequest(http.MethodPost,
		"https://api.minimax.io/anthropic/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2024-06-01")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  AI error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "  AI error: status %d, %s\n", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
		return nil
	}

	var result struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			Thinking string `json:"thinking,omitempty"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil
	}

	// Get text block content
	var content string
	for _, b := range result.Content {
		if b.Type == "text" && b.Text != "" {
			content = b.Text
			break
		}
	}
	content = strings.TrimSpace(content)

	// Extract JSON array
	if idx := strings.Index(content, "["); idx >= 0 {
		content = content[idx:]
	}
	if idx := strings.LastIndex(content, "]"); idx >= 0 {
		content = content[:idx+1]
	}

	var segments []string
	if err := json.Unmarshal([]byte(content), &segments); err != nil {
		fmt.Fprintf(os.Stderr, "  AI parse error: %v\n", err)
		return nil
	}

	var filtered []string
	for _, s := range segments {
		s = strings.TrimSpace(s)
		if s != "" {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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
