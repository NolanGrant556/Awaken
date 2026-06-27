package bridgetoken

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDecodeJWTExpiry(t *testing.T) {
	token := testJWT(t, map[string]any{
		"exp": int64(1700000000),
		"sub": "session",
	})

	payload, ok := DecodeJWTPayload(token)
	if !ok {
		t.Fatal("expected payload to decode")
	}
	if payload["sub"] != "session" {
		t.Fatalf("expected sub=session, got %v", payload["sub"])
	}

	expiry, ok := DecodeJWTExpiry(token)
	if !ok {
		t.Fatal("expected expiry to decode")
	}
	if expiry != 1700000000 {
		t.Fatalf("expected expiry 1700000000, got %d", expiry)
	}
}

func TestDecodeJWTExpiryStripsSessionIngressPrefix(t *testing.T) {
	token := sessionIngressTokenPrefix + testJWT(t, map[string]any{
		"exp": int64(1700001234),
	})

	expiry, ok := DecodeJWTExpiry(token)
	if !ok {
		t.Fatal("expected prefixed token expiry to decode")
	}
	if expiry != 1700001234 {
		t.Fatalf("expected expiry 1700001234, got %d", expiry)
	}
}

func TestDecodeJWTMalformedReturnsFalse(t *testing.T) {
	tokens := []string{
		"",
		"header.payload",
		"header..signature",
		"header.not-base64-url.signature",
		testJWT(t, map[string]any{"sub": "missing-exp"}),
	}

	for _, token := range tokens {
		if _, ok := DecodeJWTExpiry(token); ok {
			t.Fatalf("expected DecodeJWTExpiry(%q) to return false", token)
		}
	}

	if _, ok := DecodeJWTPayload("header.not-base64-url.signature"); ok {
		t.Fatal("expected malformed payload to return false")
	}
}

func testJWT(t *testing.T, payload map[string]any) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	return base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(payloadBytes) + ".signature"
}
