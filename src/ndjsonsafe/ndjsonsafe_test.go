package ndjsonsafe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalEscapesLineTerminatorsInMapString(t *testing.T) {
	value := map[string]string{
		"message": "before\u2028middle\u2029after",
	}

	data, err := Marshal(value)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	got := string(data)
	if strings.Contains(got, "\u2028") || strings.Contains(got, "\u2029") {
		t.Fatalf("expected raw line terminators to be escaped, got %q", got)
	}
	if !strings.Contains(got, `\u2028`) || !strings.Contains(got, `\u2029`) {
		t.Fatalf("expected escaped line terminators, got %q", got)
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("expected compact one-line JSON, got %q", got)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded["message"] != value["message"] {
		t.Fatalf("round trip changed value: want %q, got %q", value["message"], decoded["message"])
	}
}

func TestStringifyNestedStructPreservesUnicode(t *testing.T) {
	type nested struct {
		Detail string `json:"detail"`
	}
	type payload struct {
		Name   string `json:"name"`
		Nested nested `json:"nested"`
	}

	got, err := Stringify(payload{
		Name:   "你好",
		Nested: nested{Detail: "left\u2028right"},
	})
	if err != nil {
		t.Fatalf("Stringify returned error: %v", err)
	}

	if !strings.Contains(got, "你好") {
		t.Fatalf("expected ordinary Unicode to be preserved, got %q", got)
	}
	if strings.Contains(got, "\u2028") {
		t.Fatalf("expected raw U+2028 to be escaped, got %q", got)
	}
	if !strings.Contains(got, `\u2028`) {
		t.Fatalf("expected escaped U+2028, got %q", got)
	}

	var decoded payload
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded.Name != "你好" || decoded.Nested.Detail != "left\u2028right" {
		t.Fatalf("round trip changed value: %+v", decoded)
	}
}

func TestMarshalLeavesOrdinaryNewlinesToEncodingJSON(t *testing.T) {
	value := map[string]string{
		"message": "line one\nline two\rline three",
	}

	data, err := Marshal(value)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	got := string(data)
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("expected ordinary newline and carriage return to stay JSON-escaped, got %q", got)
	}
	if !strings.Contains(got, `\n`) || !strings.Contains(got, `\r`) {
		t.Fatalf("expected encoding/json newline escapes, got %q", got)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded["message"] != value["message"] {
		t.Fatalf("round trip changed value: want %q, got %q", value["message"], decoded["message"])
	}
}

func TestEscapeLineTerminatorsEscapesRawJSONBytes(t *testing.T) {
	raw := []byte(`{"message":"a` + "\u2028" + `b` + "\u2029" + `c","newline":"x\ny"}`)

	escaped := EscapeLineTerminators(raw)
	got := string(escaped)

	want := `{"message":"a\u2028b\u2029c","newline":"x\ny"}`
	if got != want {
		t.Fatalf("unexpected escaped JSON:\nwant %q\ngot  %q", want, got)
	}
	if strings.Contains(got, "\u2028") || strings.Contains(got, "\u2029") {
		t.Fatalf("expected no raw line terminators, got %q", got)
	}

	var decoded map[string]string
	if err := json.Unmarshal(escaped, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded["message"] != "a\u2028b\u2029c" || decoded["newline"] != "x\ny" {
		t.Fatalf("round trip changed value: %+v", decoded)
	}
}

func TestMarshalReturnsErrorForInvalidValue(t *testing.T) {
	data, err := Marshal(map[string]any{
		"bad": make(chan int),
	})
	if err == nil {
		t.Fatalf("expected Marshal error")
	}
	if data != nil {
		t.Fatalf("expected nil data on error, got %q", string(data))
	}

	text, err := Stringify(map[string]any{
		"bad": make(chan int),
	})
	if err == nil {
		t.Fatalf("expected Stringify error")
	}
	if text != "" {
		t.Fatalf("expected empty string on error, got %q", text)
	}
}
