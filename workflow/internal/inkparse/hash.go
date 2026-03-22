package inkparse

import (
	"crypto/sha256"
	"encoding/hex"
)

// SourceHash returns the hex-encoded SHA-256 of text.
func SourceHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
