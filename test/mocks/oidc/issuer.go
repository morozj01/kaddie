// Package oidc is a minimal mock OpenID Connect issuer for tests (UI spec
// §12). It serves discovery and a JWKS, and mints RS256 ID tokens for any
// claims a test asks for through a test-only endpoint. It is never shipped.
//
// The kube-apiserver of the e2e kind cluster trusts it (test/infra), so a
// token from it authenticates to the cluster with the configured username
// and group prefixes.
package oidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"
)

// TokenPath is the test-only endpoint that mints an ID token. It takes a
// JSON TokenRequest and returns a TokenResponse.
const TokenPath = "/mock/token"

// TokenRequest describes the ID token a test wants.
type TokenRequest struct {
	Subject  string   `json:"sub"`
	Email    string   `json:"email,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Audience string   `json:"aud"`
	// TTLSeconds defaults to one hour.
	TTLSeconds int `json:"ttlSeconds,omitempty"`
}

// TokenResponse carries the minted token.
type TokenResponse struct {
	IDToken string `json:"id_token"`
}

// Issuer signs tokens for one issuer URL with one RSA key.
type Issuer struct {
	URL   string
	key   *rsa.PrivateKey
	keyID string
	now   func() time.Time
}

// New returns an issuer for url with a fresh 2048-bit RSA key.
func New(url string) (*Issuer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(key.N.Bytes())
	return &Issuer{URL: url, key: key, keyID: b64(sum[:8]), now: time.Now}, nil
}

// Mint signs an ID token for req.
func (i *Issuer) Mint(req TokenRequest) (string, error) {
	if req.Subject == "" || req.Audience == "" {
		return "", errors.New("sub and aud are required")
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	now := i.now()
	claims := map[string]any{
		"iss": i.URL,
		"sub": req.Subject,
		"aud": req.Audience,
		"iat": now.Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	if req.Email != "" {
		claims["email"] = req.Email
		claims["email_verified"] = true
	}
	if len(req.Groups) > 0 {
		claims["groups"] = req.Groups
	}
	return i.sign(claims)
}

func (i *Issuer) sign(claims map[string]any) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": i.keyID})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64(header) + "." + b64(payload)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, i.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + b64(sig), nil
}

// Handler serves discovery, the JWKS and the test-only token endpoint.
func (i *Issuer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                i.URL,
			"jwks_uri":                              i.URL + "/keys",
			"authorization_endpoint":                i.URL + "/authorize",
			"token_endpoint":                        i.URL + "/token",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": i.keyID,
			"n":   b64(i.key.N.Bytes()),
			"e":   b64(big.NewInt(int64(i.key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST "+TokenPath, func(w http.ResponseWriter, r *http.Request) {
		var req TokenRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		tok, err := i.Mint(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, TokenResponse{IDToken: tok})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
