package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const (
	tokenKindVerify = "verify"
	tokenKindReset  = "reset"
	verifyTokenTTL  = 24 * time.Hour
	resetTokenTTL   = time.Hour
)

// newToken returns a URL-safe random token (32 bytes) and the sha256 hex hash
// that gets stored. Only the hash ever touches the database.
func newToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
