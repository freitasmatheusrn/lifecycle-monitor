package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/google/uuid"
)

// GenerateRefreshToken creates a cryptographically secure random token.
// Returns the plain token (to send to client) and should be hashed before storage.
func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashToken creates a SHA-256 hash of the token for secure storage.
// Only the hash is stored in Redis, not the plain token.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GenerateFamilyID creates a new UUID for token family tracking.
// All tokens in a rotation chain share the same family ID.
func GenerateFamilyID() string {
	return uuid.New().String()
}
