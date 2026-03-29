package listeningfirst

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

var openaiBaseURL = "https://api.openai.com"

func openaiKey() string { return os.Getenv("OPENAI_KEY") }

// ── Chat Completion Helper ───────────────────────────────────────────────

func chatCompletion(ctx context.Context, model, prompt string, maxTokens int) (string, error) {
	apiKey := openaiKey()
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_KEY not set")
	}

	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": maxTokens,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openaiBaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return "", fmt.Errorf("OpenAI API error: status %d, body: %s", resp.StatusCode, buf.String())
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return result.Choices[0].Message.Content, nil
}

// ── Paragraph Explanation ────────────────────────────────────────────────

type ExplainRequest struct {
	BookID         string `json:"book_id"`
	ParagraphIndex int    `json:"paragraph_index"`
	ParagraphText  string `json:"paragraph_text"`
}

type ExplainResult struct {
	Explanation string `json:"explanation"`
}

func ExplainPhrase(ctx context.Context, req ExplainRequest) (*ExplainResult, error) {
	prompt := fmt.Sprintf(`用户正在听英文小说。请用中文简单解释这段英文的含义，要：
1. 解释大意（1-2句）
2. 列出2-3个重点词汇或短语的中文意思

英文原文: "%s"

回复格式：
大意：...
重点词汇：...`, req.ParagraphText)

	content, err := chatCompletion(ctx, "gpt-4o-mini", prompt, 500)
	if err != nil {
		return nil, err
	}
	return &ExplainResult{Explanation: content}, nil
}

// ── Phrase Translation (per-phrase in context) ───────────────────────────

type PhraseTranslateRequest struct {
	Phrases          []string `json:"phrases"`
	ContextParagraph string   `json:"context_paragraph"`
}

type PhraseTranslateResult struct {
	Translations []string `json:"translations"`
}

// TranslatePhrases translates multiple phrases given the full paragraph context.
// Returns one Chinese translation per phrase.
func TranslatePhrases(ctx context.Context, req PhraseTranslateRequest) (*PhraseTranslateResult, error) {
	phrasesJSON, _ := json.Marshal(req.Phrases)
	prompt := fmt.Sprintf(`你是一个英语学习助手。用户正在阅读以下英文段落：

"%s"

请翻译以下短语为中文，翻译必须符合上下文语境，简洁自然。每个短语一行翻译，不要编号，不要额外解释。

短语列表（JSON数组）：
%s

请按相同顺序返回翻译，每行一个，共%d行。`, req.ContextParagraph, string(phrasesJSON), len(req.Phrases))

	content, err := chatCompletion(ctx, "gpt-4o-mini", prompt, 1000)
	if err != nil {
		return nil, err
	}

	lines := splitNonEmpty(content)
	// Pad or trim to match input length
	for len(lines) < len(req.Phrases) {
		lines = append(lines, "")
	}
	return &PhraseTranslateResult{Translations: lines[:len(req.Phrases)]}, nil
}

// ── Word Translation ─────────────────────────────────────────────────────

func TranslateWord(ctx context.Context, word, contextSentence string) (string, error) {
	prompt := fmt.Sprintf(`翻译英文单词 "%s" 为中文。
上下文句子: "%s"
要求：只返回中文翻译，简洁（1-5个字），符合上下文语境。不要任何额外解释。`, word, contextSentence)

	return chatCompletion(ctx, "gpt-4o-mini", prompt, 50)
}

// ── helpers ──────────────────────────────────────────────────────────────

func splitNonEmpty(s string) []string {
	var result []string
	for _, line := range bytes.Split([]byte(s), []byte("\n")) {
		t := bytes.TrimSpace(line)
		if len(t) > 0 {
			result = append(result, string(t))
		}
	}
	return result
}
