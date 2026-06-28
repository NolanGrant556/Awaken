package structuredio

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNDJSONSafeMarshalEscapesJavaScriptLineTerminators(t *testing.T) {
	value := map[string]string{
		"text": "alpha\u2028beta\u2029gamma",
	}

	got, err := NDJSONSafeMarshal(value)
	if err != nil {
		t.Fatalf("NDJSONSafeMarshal returned error: %v", err)
	}

	if strings.Contains(string(got), "\n") {
		t.Fatalf("NDJSONSafeMarshal returned a newline: %q", string(got))
	}
	if strings.Contains(string(got), "\u2028") || strings.Contains(string(got), "\u2029") {
		t.Fatalf("NDJSONSafeMarshal returned raw JavaScript line terminators: %q", string(got))
	}
	if !strings.Contains(string(got), `\u2028`) || !strings.Contains(string(got), `\u2029`) {
		t.Fatalf("NDJSONSafeMarshal did not escape U+2028/U+2029: %q", string(got))
	}

	var decoded map[string]string
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("marshal output is not valid JSON: %v", err)
	}
	if decoded["text"] != value["text"] {
		t.Fatalf("decoded value changed: want %q got %q", value["text"], decoded["text"])
	}
}
