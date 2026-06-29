package titlegen

import (
	stdctx "context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"awaken/src/llm"
)

func TestNewLLMGeneratorReturnsNilWhenAPIKeyMissing(t *testing.T) {
	if g := NewLLMGenerator(LLMGeneratorOptions{Config: llm.Config{Model: "gpt"}}); g != nil {
		t.Fatalf("expected nil generator when APIKey is empty, got %T", g)
	}
}

func TestLLMGeneratorReturnsContent(t *testing.T) {
	withNoProxy(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAIChatResponse(w, "Debug login failure")
	}))
	defer server.Close()

	gen := NewLLMGenerator(LLMGeneratorOptions{
		Config: llm.Config{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			Model:      "test-model",
			AuthScheme: "bearer",
		},
	})
	if gen == nil {
		t.Fatal("expected non-nil generator when APIKey is set")
	}

	title, err := gen.GenerateTitle(stdctx.Background(), Prompt{
		SystemPrompt: "system instruction",
		UserPrompt:   "User: hello\n\nAssistant: hi",
	})
	if err != nil {
		t.Fatalf("expected title, got error: %v", err)
	}
	if title != "Debug login failure" {
		t.Fatalf("expected generated title, got %q", title)
	}
}

func TestLLMGeneratorSendsSystemAndUserPrompts(t *testing.T) {
	withNoProxy(t)
	var captured openaiChatRequestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &captured)
		writeOpenAIChatResponse(w, "Some Title")
	}))
	defer server.Close()

	gen := NewLLMGenerator(LLMGeneratorOptions{
		Config: llm.Config{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			Model:      "test-model",
			AuthScheme: "bearer",
		},
		MaxTokens: 42,
	})

	_, err := gen.GenerateTitle(stdctx.Background(), Prompt{
		SystemPrompt: "title system prompt",
		UserPrompt:   "User: question\n\nAssistant: answer",
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if captured.Model != "test-model" {
		t.Fatalf("expected model in request, got %q", captured.Model)
	}
	if captured.MaxTokens != 42 {
		t.Fatalf("expected max_tokens=%d, got %d", 42, captured.MaxTokens)
	}
	if captured.Stream {
		t.Fatalf("expected non-streaming request, got stream=%v", captured.Stream)
	}
	if len(captured.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" || captured.Messages[0].Content != "title system prompt" {
		t.Fatalf("unexpected system message: %+v", captured.Messages[0])
	}
	if captured.Messages[1].Role != "user" || captured.Messages[1].Content != "User: question\n\nAssistant: answer" {
		t.Fatalf("unexpected user message: %+v", captured.Messages[1])
	}
}

func TestLLMGeneratorHTTPErrorReturnsError(t *testing.T) {
	withNoProxy(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	gen := NewLLMGenerator(LLMGeneratorOptions{
		Config: llm.Config{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			Model:      "test-model",
			AuthScheme: "bearer",
		},
	})

	_, err := gen.GenerateTitle(stdctx.Background(), Prompt{
		SystemPrompt: "sys",
		UserPrompt:   "usr",
	})
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

func TestLLMGeneratorEmptyContentReturnsError(t *testing.T) {
	withNoProxy(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAIChatResponse(w, "")
	}))
	defer server.Close()

	gen := NewLLMGenerator(LLMGeneratorOptions{
		Config: llm.Config{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			Model:      "test-model",
			AuthScheme: "bearer",
		},
	})

	_, err := gen.GenerateTitle(stdctx.Background(), Prompt{
		SystemPrompt: "sys",
		UserPrompt:   "usr",
	})
	if err == nil {
		t.Fatal("expected error on empty content")
	}
}

func TestLLMGeneratorWhitespaceContentReturnsError(t *testing.T) {
	withNoProxy(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAIChatResponse(w, "   \n  ")
	}))
	defer server.Close()

	gen := NewLLMGenerator(LLMGeneratorOptions{
		Config: llm.Config{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			Model:      "test-model",
			AuthScheme: "bearer",
		},
	})

	_, err := gen.GenerateTitle(stdctx.Background(), Prompt{
		SystemPrompt: "sys",
		UserPrompt:   "usr",
	})
	if err == nil {
		t.Fatal("expected error on whitespace-only content")
	}
}

func TestLLMGeneratorFeedsMaybeGenerateAndCleansTitle(t *testing.T) {
	withNoProxy(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAIChatResponse(w, "Title: \"Fix login flow.\"")
	}))
	defer server.Close()

	gen := NewLLMGenerator(LLMGeneratorOptions{
		Config: llm.Config{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			Model:      "test-model",
			AuthScheme: "bearer",
		},
	})

	result := MaybeGenerate(stdctx.Background(), MaybeGenerateRequest{
		SessionID: "session-1",
		Messages:  firstExchangeMessages(),
		Generator: gen,
	})
	if !result.Generated || result.Title != "Fix login flow" {
		t.Fatalf("expected clean title from LLM, got %+v", result)
	}
}

type openaiChatRequestBody struct {
	Model     string             `json:"model"`
	Messages  []openaiChatMessage `json:"messages"`
	Stream    bool               `json:"stream,omitempty"`
	MaxTokens int                `json:"max_tokens,omitempty"`
}

type openaiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func writeOpenAIChatResponse(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"content": content,
				},
			},
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func withNoProxy(t *testing.T) {
	t.Helper()
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("no_proxy", "127.0.0.1,localhost")
}
