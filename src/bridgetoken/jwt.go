package bridgetoken

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
)

const sessionIngressTokenPrefix = "sk-ant-si-"

func DecodeJWTPayload(token string) (map[string]any, bool) {
	jwt := strings.TrimPrefix(token, sessionIngressTokenPrefix)
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || parts[1] == "" {
		return nil, false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, false
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(payloadBytes))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, false
	}
	if payload == nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return payload, true
}

func DecodeJWTExpiry(token string) (int64, bool) {
	payload, ok := DecodeJWTPayload(token)
	if !ok {
		return 0, false
	}

	exp, ok := payload["exp"].(json.Number)
	if !ok {
		return 0, false
	}
	value, err := exp.Int64()
	if err != nil {
		return 0, false
	}
	return value, true
}
