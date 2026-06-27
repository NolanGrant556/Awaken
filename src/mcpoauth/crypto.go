package mcpoauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const randomBytesLength = 32

func GenerateCodeVerifier() (string, error) {
	return randomBase64URL(randomBytesLength)
}

func GenerateState() (string, error) {
	return randomBase64URL(randomBytesLength)
}

func CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomBase64URL(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
