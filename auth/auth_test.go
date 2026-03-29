package auth

import (
	"testing"
)

func TestIssueAndParseToken(t *testing.T) {
	token, err := IssueToken("user123")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := ParseToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if uid != "user123" {
		t.Fatalf("expected user123, got %s", uid)
	}
}

func TestParseTokenInvalid(t *testing.T) {
	_, err := ParseToken("garbage")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}
