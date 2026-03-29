package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	lf "github.com/ymzuiku/listening-first"
	"github.com/ymzuiku/listening-first/auth"
	"github.com/ymzuiku/listening-first/db"
)

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func checkBalance(w http.ResponseWriter, userID string) bool {
	user, err := db.GetUser(userID)
	if err != nil || user == nil {
		jsonError(w, "user not found", 404)
		return false
	}
	if user.BalanceCoins < db.MinBalanceCoins {
		jsonError(w, "insufficient balance", 402)
		return false
	}
	return true
}

type Book struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Cover string `json:"cover"`
}

type BookDetail struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Cover      string   `json:"cover"`
	Paragraphs []string `json:"paragraphs"`
}

func buildClientZip() []byte {
	cmd := exec.Command("zip", "-r", "-", ".",
		"-x", "*.DS_Store",
		"-x", "audio/*",
		"-x", "books/*",
		"-x", "covers/*",
		"-x", "data.js",
	)
	cmd.Dir = "client"
	out, err := cmd.Output()
	if err != nil {
		log.Printf("buildClientZip error: %v", err)
		return nil
	}
	return out
}

func main() {
	godotenv.Load()

	if err := db.Open(); err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer db.Close()

	port := "20300"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	dataDir := "data/books"

	mux := http.NewServeMux()

	// ── Public routes ────────────────────────────────────────────────

	// POST /api/login
	mux.HandleFunc("POST /api/login", auth.HandleLogin)

	// GET /api/books
	mux.HandleFunc("GET /api/books", func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(dataDir)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var books []Book
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dataDir, e.Name()))
			if err != nil {
				continue
			}
			var bd BookDetail
			if json.Unmarshal(raw, &bd) == nil {
				books = append(books, Book{ID: bd.ID, Title: bd.Title, Cover: bd.Cover})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(books)
	})

	// GET /api/books/{id}
	mux.HandleFunc("GET /api/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		raw, err := os.ReadFile(filepath.Join(dataDir, id+".json"))
		if err != nil {
			http.Error(w, "book not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	})

	// Client zip endpoints
	mux.HandleFunc("GET /client/version", func(w http.ResponseWriter, r *http.Request) {
		zip := buildClientZip()
		if zip == nil {
			http.Error(w, "failed to build client zip", 500)
			return
		}
		h := sha256.Sum256(zip)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"hash": fmt.Sprintf("%x", h[:8])})
	})

	mux.HandleFunc("GET /client", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client" {
			http.FileServer(http.Dir("client")).ServeHTTP(w, r)
			return
		}
		zip := buildClientZip()
		if zip == nil {
			http.Error(w, "failed to build client zip", 500)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zip)
	})

	// ── Authenticated routes ─────────────────────────────────────────

	// GET /api/me — get current user info
	mux.Handle("GET /api/me", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := db.GetUser(auth.UserIDFromContext(r.Context()))
		if err != nil || user == nil {
			http.Error(w, `{"error":"user not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":              user.ID,
			"email":           user.Email,
			"name":            user.Name,
			"balance_coins":   user.DisplayBalance(),
		})
	})))

	// POST /api/paragraph-explain — AI paragraph explanation (cached)
	mux.Handle("POST /api/paragraph-explain", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req lf.ExplainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "bad request", 400)
			return
		}
		cached, err := db.GetExplanation(req.BookID, req.ParagraphIndex)
		if err != nil {
			log.Printf("db.GetExplanation error: %v", err)
		}
		if cached != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached)
			return
		}

		userID := auth.UserIDFromContext(r.Context())
		if !checkBalance(w, userID) {
			return
		}
		result, err := lf.ExplainPhrase(r.Context(), req)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if err := db.DeductBalance(userID, 1, "paragraph_explain"); err != nil {
			log.Printf("deduct balance: %v", err)
		}
		if err := db.SaveExplanation(req.BookID, req.ParagraphIndex, req.ParagraphText, result.Explanation); err != nil {
			log.Printf("db.SaveExplanation error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})))

	// POST /api/phrase-translate — translate phrases in context (cached per phrase)
	mux.Handle("POST /api/phrase-translate", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req lf.PhraseTranslateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		userID := auth.UserIDFromContext(r.Context())
		if !checkBalance(w, userID) {
			return
		}
		translations := make([]string, len(req.Phrases))
		var uncachedIdx []int

		// Check cache first
		for i, phrase := range req.Phrases {
			tr, err := db.GetPhraseTranslation(phrase, req.ContextParagraph)
			if err != nil {
				log.Printf("db.GetPhraseTranslation error: %v", err)
			}
			if tr != "" {
				translations[i] = tr
			} else {
				uncachedIdx = append(uncachedIdx, i)
			}
		}

		// If all cached, return immediately
		if len(uncachedIdx) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string][]string{"translations": translations})
			return
		}

		// Call AI for uncached phrases
		uncachedPhrases := make([]string, len(uncachedIdx))
		for i, idx := range uncachedIdx {
			uncachedPhrases[i] = req.Phrases[idx]
		}

		result, err := lf.TranslatePhrases(r.Context(), lf.PhraseTranslateRequest{
			Phrases:          uncachedPhrases,
			ContextParagraph: req.ContextParagraph,
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Charge per uncached phrase: 1 coin each
		costCoins := len(uncachedIdx)
		if err := db.DeductBalance(userID, costCoins, "phrase_translate"); err != nil {
			log.Printf("deduct balance: %v", err)
		}

		// Merge and cache
		for i, idx := range uncachedIdx {
			if i < len(result.Translations) {
				translations[idx] = result.Translations[i]
				db.SavePhraseTranslation(req.Phrases[idx], req.ContextParagraph, result.Translations[i])
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]string{"translations": translations})
	})))

	// POST /api/word-translate — translate a single word in context (cached)
	mux.Handle("POST /api/word-translate", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Word            string `json:"word"`
			ContextSentence string `json:"context_sentence"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		word := strings.ToLower(strings.TrimSpace(req.Word))
		cached, err := db.GetWordTranslation(word)
		if err != nil {
			log.Printf("db.GetWordTranslation error: %v", err)
		}
		if cached != "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"translation": cached})
			return
		}

		userID := auth.UserIDFromContext(r.Context())
		if !checkBalance(w, userID) {
			return
		}
		translation, err := lf.TranslateWord(r.Context(), word, req.ContextSentence)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		if err := db.DeductBalance(userID, 1, "word_translate"); err != nil {
			log.Printf("deduct balance: %v", err)
		}
		db.SaveWordTranslation(word, translation)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"translation": translation})
	})))

	// POST /api/word-pronounce — TTS for a single word (returns MP3)
	mux.Handle("POST /api/word-pronounce", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Word string `json:"word"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		userID := auth.UserIDFromContext(r.Context())
		if !checkBalance(w, userID) {
			return
		}
		mp3, err := lf.TextToSpeech(r.Context(), lf.TextToSpeechRequest{
			Text:  req.Word,
			Model: "tts-1-hd",
			Voice: "nova",
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		if err := db.DeductBalance(userID, 1, "word_pronounce"); err != nil {
			log.Printf("deduct balance: %v", err)
		}

		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(mp3)
	})))

	// ── User Books ───────────────────────────────────────────────────

	// POST /api/user-books — create a book
	mux.Handle("POST /api/user-books", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		userID := auth.UserIDFromContext(r.Context())
		book, err := db.CreateUserBook(userID, req.Title)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(book)
	})))

	// GET /api/user-books — list user's books
	mux.Handle("GET /api/user-books", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserIDFromContext(r.Context())
		books, err := db.GetUserBooks(userID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(books)
	})))

	// GET /api/user-books/{id} — get single user book
	mux.Handle("GET /api/user-books/{id}", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bookID := r.PathValue("id")
		book, err := db.GetUserBook(bookID)
		if err != nil || book == nil {
			jsonError(w, "book not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(book)
	})))

	// PUT /api/user-books/{id} — update title, cover, paragraphs
	mux.Handle("PUT /api/user-books/{id}", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title      string `json:"title"`
			Cover      string `json:"cover"`
			Paragraphs string `json:"paragraphs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "bad request", 400)
			return
		}
		bookID := r.PathValue("id")
		userID := auth.UserIDFromContext(r.Context())
		book, err := db.GetUserBook(bookID)
		if err != nil || book == nil {
			jsonError(w, "book not found", 404)
			return
		}
		if book.UserID != userID {
			jsonError(w, "forbidden", 403)
			return
		}
		title := req.Title
		if title == "" {
			title = book.Title
		}
		cover := req.Cover
		if cover == "" {
			cover = book.Cover
		}
		paragraphs := req.Paragraphs
		if paragraphs == "" {
			paragraphs = book.Paragraphs
		}
		if err := db.UpdateUserBook(bookID, title, cover, paragraphs); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})))

	// POST /api/user-books/{id}/cover — generate cover image
	mux.Handle("POST /api/user-books/{id}/cover", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		userID := auth.UserIDFromContext(r.Context())
		if !checkBalance(w, userID) {
			return
		}
		result, err := lf.GenerateImage(r.Context(), lf.GenerateImageRequest{Prompt: req.Prompt})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Image generation: 4 coins
		if err := db.DeductBalance(userID, 4, "cover_generate"); err != nil {
			log.Printf("deduct balance: %v", err)
		}

		coverURL := ""
		if len(result.ImageURLs) > 0 {
			coverURL = result.ImageURLs[0]
		}

		bookID := r.PathValue("id")
		if _, err := db.DB.Exec("UPDATE user_books SET cover = $1 WHERE id = $2", coverURL, bookID); err != nil {
			log.Printf("update cover: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"cover": coverURL})
	})))

	// POST /api/user-books/{id}/publish — make book public (irreversible)
	mux.Handle("POST /api/user-books/{id}/publish", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bookID := r.PathValue("id")
		userID := auth.UserIDFromContext(r.Context())

		book, err := db.GetUserBook(bookID)
		if err != nil || book == nil {
			http.Error(w, `{"error":"book not found"}`, 404)
			return
		}
		if book.UserID != userID {
			http.Error(w, `{"error":"forbidden"}`, 403)
			return
		}
		if book.IsPublic {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"is_public": true})
			return
		}

		if err := db.PublishBook(bookID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"is_public": true})
	})))

	// GET /api/public-books — list all public user books
	mux.HandleFunc("GET /api/public-books", func(w http.ResponseWriter, r *http.Request) {
		books, err := db.GetPublicBooks()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if books == nil {
			books = []db.UserBook{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(books)
	})

	// Serve covers and audio
	mux.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir("data"))))

	// Serve client static files
	mux.Handle("/", http.FileServer(http.Dir("client")))

	// Wrap with CORS and request logging
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		log.Printf("%s %s (from %s)", r.Method, r.URL.Path, r.RemoteAddr)
		mux.ServeHTTP(w, r)
	})

	log.Printf("Server listening on 0.0.0.0:%s", port)
	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, handler))
}
