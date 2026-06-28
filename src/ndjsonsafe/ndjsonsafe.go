package ndjsonsafe

import (
	"bytes"
	"encoding/json"
)

var (
	lineSeparator        = []byte("\u2028")
	paragraphSeparator   = []byte("\u2029")
	escapedLineSeparator = []byte(`\u2028`)
	escapedParagraphSep  = []byte(`\u2029`)
)

// Marshal serializes v as compact JSON and escapes JavaScript line terminators
// that can break one-message-per-line receivers.
func Marshal(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return EscapeLineTerminators(data), nil
}

// Stringify serializes v as an NDJSON-safe JSON string.
func Stringify(v any) (string, error) {
	data, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// EscapeLineTerminators replaces raw U+2028 and U+2029 UTF-8 sequences in JSON
// bytes with their equivalent JSON escape forms.
func EscapeLineTerminators(jsonBytes []byte) []byte {
	jsonBytes = bytes.ReplaceAll(jsonBytes, lineSeparator, escapedLineSeparator)
	jsonBytes = bytes.ReplaceAll(jsonBytes, paragraphSeparator, escapedParagraphSep)
	return jsonBytes
}
