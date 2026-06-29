package titlegen

import (
	stdctx "context"
	"os"
	"strings"
	"testing"

	"awaken/src/llm"
)

func TestLLMGeneratorRealSmoke(t *testing.T) {
	if os.Getenv("AWAKEN_TITLEGEN_SMOKE") == "" {
		t.Skip("skipping real-LLM smoke; set AWAKEN_TITLEGEN_SMOKE=1 to run")
	}
	cfg := llm.LoadConfig()
	if cfg.APIKey == "" {
		t.Skip("skipping smoke: no LLM_API_KEY in .env")
	}
	gen := NewLLMGenerator(LLMGeneratorOptions{Config: cfg})
	if gen == nil {
		t.Fatal("expected non-nil generator from real config")
	}
	prompt := BuildPrompt("How do I fix the login callback loop in the auth middleware?", "The session cookie was being cleared on each redirect; guard the callback handler with a single-use nonce.", Config{})

	title, err := gen.GenerateTitle(stdctx.Background(), prompt)
	if err != nil {
		t.Fatalf("real LLM call failed: %v", err)
	}
	cleaned := CleanTitle(title, DefaultMaxTitleRunes)
	t.Logf("raw=%q cleaned=%q", title, cleaned)
	if strings.TrimSpace(cleaned) == "" {
		t.Fatalf("expected non-empty cleaned title, got %q", cleaned)
	}
	// The prompt-based fallback would just echo "How do I fix the login callback
	// loop in the auth middleware?" (truncated). A real LLM summary should be
	// materially different and short.
	if cleaned == "How do I fix the login callback loop in the auth middleware?" {
		t.Fatalf("output matches prompt-fallback; LLM likely not invoked: %q", cleaned)
	}
}
