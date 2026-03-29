package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/oklog/ulid/v2"
	listeningfirst "github.com/ymzuiku/listening-first"
)

var (
	DB           *sql.DB
	dbDriverName string
)

func NewULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

func Open() error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "sqlite://./listening-first.db"
	}

	driver, dsn := parseURL(url)

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	DB = db
	dbDriverName = driver
	log.Printf("database connected: %s", driver)

	if err := migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}

func parseURL(url string) (driver, dsn string) {
	if strings.HasPrefix(url, "sqlite") {
		return "sqlite3", strings.TrimPrefix(url, "sqlite://")
	}
	if strings.HasPrefix(url, "postgres") || strings.HasPrefix(url, "postgresql") {
		return "pgx", url
	}
	return "sqlite3", url
}

func migrate() error {
	tables := sqliteTables
	if dbDriverName == "pgx" {
		tables = postgresTables
	}
	for _, ddl := range tables {
		if _, err := DB.Exec(ddl); err != nil {
			return fmt.Errorf("migrate: %w\nSQL: %s", err, ddl)
		}
	}
	return nil
}

var sqliteTables = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL,
		provider_sub TEXT NOT NULL,
		balance_coins INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(provider, provider_sub)
	)`,
	`CREATE TABLE IF NOT EXISTS paragraph_explanations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		book_id TEXT NOT NULL,
		paragraph_index INTEGER NOT NULL,
		paragraph_text TEXT NOT NULL,
		explanation TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(book_id, paragraph_index)
	)`,
	`CREATE TABLE IF NOT EXISTS phrase_translations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		phrase TEXT NOT NULL,
		context_paragraph TEXT NOT NULL,
		translation TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(phrase, context_paragraph)
	)`,
	`CREATE TABLE IF NOT EXISTS word_translations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		word TEXT NOT NULL UNIQUE,
		translation TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS user_books (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id),
		title TEXT NOT NULL,
		cover TEXT NOT NULL DEFAULT '',
		paragraphs TEXT NOT NULL DEFAULT '[]',
		is_public INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS usage_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT NOT NULL REFERENCES users(id),
		action TEXT NOT NULL,
		cost_coins INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
}

var postgresTables = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL,
		provider_sub TEXT NOT NULL,
		balance_coins INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(provider, provider_sub)
	)`,
	`CREATE TABLE IF NOT EXISTS paragraph_explanations (
		id SERIAL PRIMARY KEY,
		book_id TEXT NOT NULL,
		paragraph_index INTEGER NOT NULL,
		paragraph_text TEXT NOT NULL,
		explanation TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(book_id, paragraph_index)
	)`,
	`CREATE TABLE IF NOT EXISTS phrase_translations (
		id SERIAL PRIMARY KEY,
		phrase TEXT NOT NULL,
		context_paragraph TEXT NOT NULL,
		translation TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(phrase, context_paragraph)
	)`,
	`CREATE TABLE IF NOT EXISTS word_translations (
		id SERIAL PRIMARY KEY,
		word TEXT NOT NULL UNIQUE,
		translation TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS user_books (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id),
		title TEXT NOT NULL,
		cover TEXT NOT NULL DEFAULT '',
		paragraphs TEXT NOT NULL DEFAULT '[]',
		is_public INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS usage_logs (
		id SERIAL PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id),
		action TEXT NOT NULL,
		cost_coins INTEGER NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`,
}

// ── Users ────────────────────────────────────────────────────────────────

// MinBalanceCoins is the minimum balance required to use AI features.
// Below this threshold, balance is displayed as 0 and requests are rejected.
const MinBalanceCoins = 10

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Provider    string `json:"-"`
	ProviderSub string `json:"-"`
	// BalanceCoins is the raw DB value. Use DisplayBalance() for user-facing value.
	BalanceCoins int `json:"balance_coins"`
}

// DisplayBalance returns 0 if balance < MinBalanceCoins, otherwise the real balance.
func (u *User) DisplayBalance() int {
	if u.BalanceCoins < MinBalanceCoins {
		return 0
	}
	return u.BalanceCoins
}

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.ProviderSub, &u.BalanceCoins)
	return &u, err
}

const userCols = "id, email, name, provider, provider_sub, balance_coins"

// FindOrCreateUser finds user by provider+sub, or creates one with 0 balance.
func FindOrCreateUser(provider, providerSub, email, name string) (*User, error) {
	u, err := scanUser(DB.QueryRow(
		"SELECT "+userCols+" FROM users WHERE provider = $1 AND provider_sub = $2",
		provider, providerSub,
	))
	if err == nil {
		return u, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	u = &User{
		ID:          NewULID(),
		Email:       email,
		Name:        name,
		Provider:    provider,
		ProviderSub: providerSub,
	}
	_, err = DB.Exec(
		"INSERT INTO users (id, email, name, provider, provider_sub, balance_coins) VALUES ($1, $2, $3, $4, $5, $6)",
		u.ID, u.Email, u.Name, u.Provider, u.ProviderSub, u.BalanceCoins,
	)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUser(id string) (*User, error) {
	u, err := scanUser(DB.QueryRow("SELECT "+userCols+" FROM users WHERE id = $1", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// DeductBalance subtracts coins from user balance. Rejects if balance < MinBalanceCoins.
func DeductBalance(userID string, costCoins int, action string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var balance int
	if err := tx.QueryRow("SELECT balance_coins FROM users WHERE id = $1", userID).Scan(&balance); err != nil {
		return err
	}
	if balance < MinBalanceCoins {
		return fmt.Errorf("insufficient balance: %d coins (minimum %d)", balance, MinBalanceCoins)
	}
	if balance-costCoins < 0 {
		return fmt.Errorf("insufficient balance: %d coins, need %d", balance, costCoins)
	}

	if _, err := tx.Exec("UPDATE users SET balance_coins = balance_coins - $1 WHERE id = $2", costCoins, userID); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO usage_logs (user_id, action, cost_coins) VALUES ($1, $2, $3)", userID, action, costCoins); err != nil {
		return err
	}
	return tx.Commit()
}

// AddBalance adds coins to user balance.
func AddBalance(userID string, coins int) error {
	_, err := DB.Exec("UPDATE users SET balance_coins = balance_coins + $1 WHERE id = $2", coins, userID)
	return err
}

// RewardBookOwner gives 35% of costCoins to the book owner.
func RewardBookOwner(bookOwnerID string, costCoins int) error {
	reward := costCoins * 35 / 100
	if reward <= 0 {
		return nil
	}
	return AddBalance(bookOwnerID, reward)
}

// ── Paragraph Explanations ───────────────────────────────────────────────

func GetExplanation(bookID string, paragraphIndex int) (*listeningfirst.ExplainResult, error) {
	var explanation string
	err := DB.QueryRow(
		"SELECT explanation FROM paragraph_explanations WHERE book_id = $1 AND paragraph_index = $2",
		bookID, paragraphIndex,
	).Scan(&explanation)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &listeningfirst.ExplainResult{Explanation: explanation}, nil
}

func SaveExplanation(bookID string, paragraphIndex int, paragraphText, explanation string) error {
	_, err := DB.Exec(
		"INSERT INTO paragraph_explanations (book_id, paragraph_index, paragraph_text, explanation) VALUES ($1, $2, $3, $4) ON CONFLICT(book_id, paragraph_index) DO UPDATE SET explanation = $4",
		bookID, paragraphIndex, paragraphText, explanation,
	)
	return err
}

// ── Phrase Translations ──────────────────────────────────────────────────

func GetPhraseTranslation(phrase, contextParagraph string) (string, error) {
	var translation string
	err := DB.QueryRow(
		"SELECT translation FROM phrase_translations WHERE phrase = $1 AND context_paragraph = $2",
		phrase, contextParagraph,
	).Scan(&translation)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return translation, err
}

func SavePhraseTranslation(phrase, contextParagraph, translation string) error {
	_, err := DB.Exec(
		"INSERT INTO phrase_translations (phrase, context_paragraph, translation) VALUES ($1, $2, $3) ON CONFLICT(phrase, context_paragraph) DO UPDATE SET translation = $3",
		phrase, contextParagraph, translation,
	)
	return err
}

// ── Word Translations ────────────────────────────────────────────────────

func GetWordTranslation(word string) (string, error) {
	var translation string
	err := DB.QueryRow(
		"SELECT translation FROM word_translations WHERE word = $1", word,
	).Scan(&translation)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return translation, err
}

func SaveWordTranslation(word, translation string) error {
	_, err := DB.Exec(
		"INSERT INTO word_translations (word, translation) VALUES ($1, $2) ON CONFLICT(word) DO UPDATE SET translation = $2",
		word, translation,
	)
	return err
}

// ── User Books ───────────────────────────────────────────────────────────

type UserBook struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Title      string `json:"title"`
	Cover      string `json:"cover"`
	Paragraphs string `json:"paragraphs"`
	IsPublic   bool   `json:"is_public"`
}

func scanUserBook(row interface{ Scan(...any) error }) (*UserBook, error) {
	var b UserBook
	var isPublic int
	err := row.Scan(&b.ID, &b.UserID, &b.Title, &b.Cover, &b.Paragraphs, &isPublic)
	b.IsPublic = isPublic != 0
	return &b, err
}

func CreateUserBook(userID, title string) (*UserBook, error) {
	b := &UserBook{
		ID:         NewULID(),
		UserID:     userID,
		Title:      title,
		Paragraphs: "[]",
	}
	_, err := DB.Exec(
		"INSERT INTO user_books (id, user_id, title, paragraphs) VALUES ($1, $2, $3, $4)",
		b.ID, b.UserID, b.Title, b.Paragraphs,
	)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func GetUserBook(id string) (*UserBook, error) {
	b, err := scanUserBook(DB.QueryRow(
		"SELECT id, user_id, title, cover, paragraphs, is_public FROM user_books WHERE id = $1", id,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

func GetUserBooks(userID string) ([]UserBook, error) {
	rows, err := DB.Query(
		"SELECT id, user_id, title, cover, paragraphs, is_public FROM user_books WHERE user_id = $1", userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []UserBook
	for rows.Next() {
		b, err := scanUserBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, *b)
	}
	return books, rows.Err()
}

// GetPublicBooks returns all public user books (from any user).
func GetPublicBooks() ([]UserBook, error) {
	rows, err := DB.Query(
		"SELECT id, user_id, title, cover, paragraphs, is_public FROM user_books WHERE is_public = 1",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []UserBook
	for rows.Next() {
		b, err := scanUserBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, *b)
	}
	return books, rows.Err()
}

// PublishBook sets is_public=1. Once public, cannot be unpublished.
func PublishBook(bookID string) error {
	_, err := DB.Exec("UPDATE user_books SET is_public = 1 WHERE id = $1", bookID)
	return err
}

func UpdateUserBook(id, title, cover, paragraphs string) error {
	_, err := DB.Exec(
		"UPDATE user_books SET title = $1, cover = $2, paragraphs = $3 WHERE id = $4",
		title, cover, paragraphs, id,
	)
	return err
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
