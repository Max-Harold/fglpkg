// Package oauth implements the auth-code + PKCE login flow against the
// fglpkg consumer registry. It registers a one-off public client via DCR
// (RFC 7591), runs the browser hop on a loopback port, and returns the
// resulting access + refresh tokens. The caller (typically the credentials
// package via cli) is responsible for persisting them.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
)

// GenerateVerifier returns a high-entropy PKCE code_verifier per RFC 7636 §4.1.
// 32 random bytes yield a 43-character base64url string, well within the
// allowed range [43, 128].
func GenerateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random for verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ChallengeFor computes the S256 code_challenge for a code_verifier per
// RFC 7636 §4.2: BASE64URL(SHA256(ASCII(verifier))).
func ChallengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GenerateState returns a random URL-safe state value used to detect CSRF on
// the /callback hop.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random for state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// verifierRe matches the unreserved character set permitted by RFC 7636 §4.1.
var verifierRe = regexp.MustCompile(`^[A-Za-z0-9\-._~]{43,128}$`)

// ValidVerifier reports whether v conforms to RFC 7636 §4.1. Used by tests
// and as a defensive check before sending a verifier on the wire.
func ValidVerifier(v string) bool {
	return verifierRe.MatchString(v)
}
