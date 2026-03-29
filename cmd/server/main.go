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
)

type Book struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Cover string `json:"cover"`
}

type BookDetail struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Cover     string   `json:"cover"`
	Sentences []string `json:"sentences"`
}

func buildClientZip() []byte {
	cmd := exec.Command("zip", "-r", "-", ".", "-x", "*.DS_Store")
	cmd.Dir = "client"
	out, err := cmd.Output()
	if err != nil {
		log.Printf("buildClientZip error: %v", err)
		return nil
	}
	return out
}

func main() {
	port := "20300"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	dataDir := "data/books"

	mux := http.NewServeMux()

	// GET /api/books — list all books
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

	// GET /api/books/{id} — get book detail with sentences
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

	// GET /client/version — return hash of client zip
	// GET /client — serve client zip for AppShell download
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
		// Exact match only — don't intercept /client/foo static files
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

	// Serve covers and audio
	mux.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir("data"))))

	// Serve client static files
	mux.Handle("/", http.FileServer(http.Dir("client")))

	// Wrap with CORS and request logging
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
