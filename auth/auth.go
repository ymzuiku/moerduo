package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ymzuiku/listening-first/db"
)

type contextKey string

const userIDKey contextKey = "user_id"

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-me"
	}
	return []byte(s)
}

// IssueToken creates a JWT for the given user ID, valid for 30 days.
func IssueToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(30 * 24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

// ParseToken validates a JWT and returns the user ID.
func ParseToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	sub, _ := claims.GetSubject()
	if sub == "" {
		return "", fmt.Errorf("missing sub claim")
	}
	return sub, nil
}

// UserIDFromContext extracts user ID set by AuthMiddleware.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

// AuthMiddleware requires a valid Bearer token. Sets user_id in context.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		userID, err := ParseToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, 401)
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ── OAuth Token Verification ─────────────────────────────────────────────

// Google OAuth client IDs (both iOS and Web are accepted)
var googleClientIDs = []string{
	"555010677252-b0a30lbgplpgkqvhp0a5e2u7ckrvee3t.apps.googleusercontent.com",  // Web
	"555010677252-e2cbci6okaoj34naqoa81reiq1nqfod5.apps.googleusercontent.com", // iOS
}

// VerifyGoogleIDToken verifies a Google ID token by calling Google's tokeninfo endpoint.
func VerifyGoogleIDToken(idToken string) (email, sub, name string, err error) {
	resp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + idToken)
	if err != nil {
		return "", "", "", fmt.Errorf("google verify: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", "", fmt.Errorf("google verify: status %d", resp.StatusCode)
	}
	var info struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
		Name  string `json:"name"`
		Aud   string `json:"aud"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", "", err
	}
	// Verify audience matches one of our client IDs
	validAud := false
	for _, cid := range googleClientIDs {
		if info.Aud == cid {
			validAud = true
			break
		}
	}
	if !validAud {
		return "", "", "", fmt.Errorf("google verify: invalid audience %s", info.Aud)
	}
	return info.Email, info.Sub, info.Name, nil
}

// HandleLogin handles POST /api/login with {"provider":"apple|google","id_token":"..."}
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		IDToken  string `json:"id_token"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, 400)
		return
	}

	var email, sub, name string
	var err error

	switch req.Provider {
	case "google":
		email, sub, name, err = VerifyGoogleIDToken(req.IDToken)
	case "apple":
		email, sub, err = VerifyAppleIDToken(req.IDToken)
		if req.Name != "" {
			name = req.Name // Apple only sends name on first sign-in
		}
	default:
		http.Error(w, `{"error":"unsupported provider"}`, 400)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 401)
		return
	}

	user, err := db.FindOrCreateUser(req.Provider, sub, email, name)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	token, err := IssueToken(user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"token": token,
		"user":  user,
	})
}
