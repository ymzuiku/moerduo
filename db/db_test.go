package db

import (
	"os"
	"testing"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("DATABASE_URL", "sqlite://"+tmpFile)
	if err := Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { Close() })
}

func TestMigrate(t *testing.T) {
	setupTestDB(t)
	for _, table := range []string{"users", "paragraph_explanations", "phrase_translations", "word_translations", "user_books", "usage_logs"} {
		var name string
		err := DB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=$1", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestFindOrCreateUser(t *testing.T) {
	setupTestDB(t)

	u1, err := FindOrCreateUser("apple", "sub123", "test@example.com", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID == "" || u1.Email != "test@example.com" || u1.BalanceCoins != 0 {
		t.Fatalf("unexpected user: %+v", u1)
	}

	// Same provider+sub should return same user
	u2, err := FindOrCreateUser("apple", "sub123", "test@example.com", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("expected same user, got %s vs %s", u1.ID, u2.ID)
	}
}

func TestDisplayBalance(t *testing.T) {
	u := &User{BalanceCoins: 5}
	if u.DisplayBalance() != 0 {
		t.Fatalf("expected 0 for balance < 10, got %d", u.DisplayBalance())
	}
	u.BalanceCoins = 10
	if u.DisplayBalance() != 10 {
		t.Fatalf("expected 10, got %d", u.DisplayBalance())
	}
	u.BalanceCoins = 100
	if u.DisplayBalance() != 100 {
		t.Fatalf("expected 100, got %d", u.DisplayBalance())
	}
}

func TestDeductBalance(t *testing.T) {
	setupTestDB(t)

	u, _ := FindOrCreateUser("google", "g1", "a@b.com", "A")
	// Should fail with 0 balance (< MinBalanceCoins)
	if err := DeductBalance(u.ID, 1, "test"); err == nil {
		t.Fatal("expected insufficient balance error")
	}

	// Add balance and try again
	AddBalance(u.ID, 500)
	if err := DeductBalance(u.ID, 10, "word_translate"); err != nil {
		t.Fatalf("deduct failed: %v", err)
	}

	got, _ := GetUser(u.ID)
	if got.BalanceCoins != 490 {
		t.Fatalf("expected 490, got %d", got.BalanceCoins)
	}
}

func TestRewardBookOwner(t *testing.T) {
	setupTestDB(t)

	owner, _ := FindOrCreateUser("apple", "owner1", "owner@x.com", "Owner")
	// 35% of 100 = 35
	if err := RewardBookOwner(owner.ID, 100); err != nil {
		t.Fatal(err)
	}
	got, _ := GetUser(owner.ID)
	if got.BalanceCoins != 35 {
		t.Fatalf("expected 35, got %d", got.BalanceCoins)
	}
}

func TestWordTranslation(t *testing.T) {
	setupTestDB(t)

	tr, _ := GetWordTranslation("hello")
	if tr != "" {
		t.Fatal("expected empty")
	}

	SaveWordTranslation("hello", "你好")
	tr, _ = GetWordTranslation("hello")
	if tr != "你好" {
		t.Fatalf("expected 你好, got %s", tr)
	}

	// Upsert
	SaveWordTranslation("hello", "你好啊")
	tr, _ = GetWordTranslation("hello")
	if tr != "你好啊" {
		t.Fatalf("expected 你好啊, got %s", tr)
	}
}

func TestPhraseTranslation(t *testing.T) {
	setupTestDB(t)

	SavePhraseTranslation("good morning", "I said good morning to him.", "早上好")
	tr, _ := GetPhraseTranslation("good morning", "I said good morning to him.")
	if tr != "早上好" {
		t.Fatalf("expected 早上好, got %s", tr)
	}
}

func TestUserBooksAndPublish(t *testing.T) {
	setupTestDB(t)

	u, _ := FindOrCreateUser("apple", "s1", "x@y.com", "X")
	b, err := CreateUserBook(u.ID, "My Book")
	if err != nil {
		t.Fatal(err)
	}
	if b.IsPublic {
		t.Fatal("new book should not be public")
	}

	// Publish
	if err := PublishBook(b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := GetUserBook(b.ID)
	if !got.IsPublic {
		t.Fatal("book should be public after PublishBook")
	}

	// Public books list
	pubs, _ := GetPublicBooks()
	if len(pubs) != 1 || pubs[0].ID != b.ID {
		t.Fatalf("expected 1 public book, got %+v", pubs)
	}
}
