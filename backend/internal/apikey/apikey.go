// Package apikey generates, hashes and parses workspace API keys
// ("le_live_" + 32 random bytes, base64url). Only the SHA-256 is stored.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	Prefix       = "le_live_"
	randomBytes  = 32
	displayChars = 6 // chars after Prefix kept for display ("le_live_AbC123…")

	ScopeRead  = "read"
	ScopeWrite = "write"
)

var encodedLen = base64.RawURLEncoding.EncodedLen(randomBytes)

// Key is a freshly generated key. Plaintext must be shown once and discarded.
type Key struct {
	Plaintext string
	Display   string
	Hash      string
}

func Generate() (Key, error) {
	b := make([]byte, randomBytes)
	if _, err := rand.Read(b); err != nil {
		return Key{}, fmt.Errorf("generate api key: %w", err)
	}
	pt := Prefix + base64.RawURLEncoding.EncodeToString(b)
	return Key{Plaintext: pt, Display: pt[:len(Prefix)+displayChars], Hash: Hash(pt)}, nil
}

// Hash is the hex SHA-256 of the key; keys are high-entropy so no salt/KDF is needed.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// LooksLikeKey is a cheap shape check that rejects garbage before a DB lookup.
func LooksLikeKey(s string) bool {
	if len(s) != len(Prefix)+encodedLen || !strings.HasPrefix(s, Prefix) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(s[len(Prefix):])
	return err == nil
}

// Bearer extracts the token from an "Authorization: Bearer <token>" header.
func Bearer(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// NormalizeScopes validates scopes; empty means read-only and write implies read.
func NormalizeScopes(in []string) ([]string, error) {
	read, write := len(in) == 0, false
	for _, s := range in {
		switch s {
		case ScopeRead:
			read = true
		case ScopeWrite:
			read, write = true, true
		default:
			return nil, errors.New("invalid scope: " + s)
		}
	}
	out := []string{}
	if read {
		out = append(out, ScopeRead)
	}
	if write {
		out = append(out, ScopeWrite)
	}
	return out, nil
}

func HasScope(scopes []string, want string) bool { return slices.Contains(scopes, want) }
