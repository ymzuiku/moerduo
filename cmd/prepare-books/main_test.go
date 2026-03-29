package main

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello  world", "hello world"},
		{"  a\n\tb  ", "a b"},
		{"already clean", "already clean"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeWhitespace(tt.in)
		if got != tt.want {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitSentences(t *testing.T) {
	text := "I was born in York. My father was a merchant. He had a large estate."

	// Request 2 sentences
	got := splitSentences(text, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences, got %d: %v", len(got), got)
	}
	if got[0] != "I was born in York." {
		t.Errorf("sentence[0] = %q", got[0])
	}
	if got[1] != "My father was a merchant." {
		t.Errorf("sentence[1] = %q", got[1])
	}

	// Request more than available
	got = splitSentences(text, 10)
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %v", len(got), got)
	}
}

func TestSplitSentences_QuotesAndExclamation(t *testing.T) {
	text := `He said "Stop!" Then he ran away. It was over.`
	got := splitSentences(text, 10)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 sentences, got %d: %v", len(got), got)
	}
}

func TestSplitSentencesIntoGroups_ShortSentences(t *testing.T) {
	// All sentences ≤15 words — no AI call should happen
	sentences := []string{
		"I was born in York.",
		"My father was a merchant.",
	}
	got := splitSentencesIntoGroups(sentences)
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(got), got)
	}
	if got[0] != sentences[0] || got[1] != sentences[1] {
		t.Errorf("short sentences should pass through unchanged, got %v", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate = %q", got)
	}
}

// Integration test: calls real MiniMax API. Skip in short mode or if no API key.
func TestAiSplitOne_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("MINIMAX_API_KEY") == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}

	sentence := "I was born in the year 1632, in the city of York, of a good family, though not of that country, my father being a foreigner of Bremen, who settled first at Hull."
	groups := aiSplitOne(sentence)
	if len(groups) < 2 {
		t.Fatalf("expected multiple groups, got %d: %v", len(groups), groups)
	}

	// Verify concatenation matches original
	joined := strings.Join(groups, " ")
	if joined != sentence {
		t.Errorf("concatenation mismatch:\n  want: %s\n  got:  %s", sentence, joined)
	}

	// Verify each group is within word count range
	for i, g := range groups {
		words := len(strings.Fields(g))
		if words < 3 || words > 20 {
			t.Errorf("group[%d] has %d words (out of reasonable range): %q", i, words, g)
		}
	}
}
