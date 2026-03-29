package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appleJWKSURL = "https://appleid.apple.com/auth/keys"
	appleIssuer  = "https://appleid.apple.com"
	appleBundleID = "com.ymzuiku.moerduo"
)

// Cached Apple JWKS keys
var (
	appleKeys     map[string]*rsa.PublicKey
	appleKeysMu   sync.RWMutex
	appleKeysTime time.Time
)

type appleJWKS struct {
	Keys []appleJWK `json:"keys"`
}

type appleJWK struct {
	KTY string `json:"kty"`
	KID string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func fetchAppleKeys() (map[string]*rsa.PublicKey, error) {
	appleKeysMu.RLock()
	if appleKeys != nil && time.Since(appleKeysTime) < 24*time.Hour {
		defer appleKeysMu.RUnlock()
		return appleKeys, nil
	}
	appleKeysMu.RUnlock()

	resp, err := http.Get(appleJWKSURL)
	if err != nil {
		return nil, fmt.Errorf("fetch apple JWKS: %w", err)
	}
	defer resp.Body.Close()

	var jwks appleJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode apple JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.KTY != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nBytes)
		e := 0
		for _, b := range eBytes {
			e = e<<8 + int(b)
		}
		keys[k.KID] = &rsa.PublicKey{N: n, E: e}
	}

	appleKeysMu.Lock()
	appleKeys = keys
	appleKeysTime = time.Now()
	appleKeysMu.Unlock()

	return keys, nil
}

// VerifyAppleIDToken verifies an Apple identity token using Apple's JWKS public keys.
func VerifyAppleIDToken(idToken string) (email, sub string, err error) {
	keys, err := fetchAppleKeys()
	if err != nil {
		return "", "", err
	}

	token, err := jwt.Parse(idToken, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}
		key, exists := keys[kid]
		if !exists {
			return nil, fmt.Errorf("unknown kid: %s", kid)
		}
		return key, nil
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(appleIssuer),
		jwt.WithAudience(appleBundleID),
	)
	if err != nil {
		return "", "", fmt.Errorf("verify apple token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", "", fmt.Errorf("invalid apple token")
	}

	subVal, _ := claims["sub"].(string)
	emailVal, _ := claims["email"].(string)
	if subVal == "" {
		return "", "", fmt.Errorf("missing sub in apple token")
	}

	return emailVal, subVal, nil
}
