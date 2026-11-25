package oidc

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMintVerifiesAgainstJWKS checks a minted token verifies with the key the
// JWKS publishes and carries the requested claims.
func TestMintVerifiesAgainstJWKS(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()
	iss, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = iss.Handler()

	body, _ := json.Marshal(TokenRequest{Subject: "alice", Email: "alice@example.com", Groups: []string{"devs"}, Audience: "kaddie-ui"})
	resp, err := http.Post(srv.URL+TokenPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatal(err)
	}

	keysResp, err := http.Get(srv.URL + "/keys")
	if err != nil {
		t.Fatal(err)
	}
	defer keysResp.Body.Close()
	var jwks struct {
		Keys []struct{ N, E string } `json:"keys"`
	}
	if err := json.NewDecoder(keysResp.Body).Decode(&jwks); err != nil {
		t.Fatal(err)
	}
	n, _ := base64.RawURLEncoding.DecodeString(jwks.Keys[0].N)
	e, _ := base64.RawURLEncoding.DecodeString(jwks.Keys[0].E)
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}

	parts := strings.Split(tr.IDToken, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature does not verify against the JWKS: %v", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	_ = json.Unmarshal(raw, &claims)
	if claims["iss"] != srv.URL || claims["aud"] != "kaddie-ui" || claims["email"] != "alice@example.com" {
		t.Fatalf("claims = %v", claims)
	}
}

func TestMintRequiresSubjectAndAudience(t *testing.T) {
	iss, err := New("https://issuer.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iss.Mint(TokenRequest{Subject: "a"}); err == nil {
		t.Error("minted a token without an audience")
	}
}
