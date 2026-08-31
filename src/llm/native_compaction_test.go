package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestPrepareNativeCompactionRequiresExactOfficialCapability(t *testing.T) {
	base := Config{
		APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.6-sol",
		ProviderID: "openai", ProfileID: "profile-openai", CredentialRevision: 4,
		Protocol: ProtocolOpenAIResponses,
	}
	request := Request{
		NativeCompaction: true, Purpose: RequestPurposeNativeCompaction,
		SystemPrompt: "system", Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "hello"}},
	}
	prepared, err := PrepareRequest(base, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Operation != PreparedOperationNativeCompaction || prepared.Endpoint != "https://api.openai.com/v1/responses/compact" {
		t.Fatalf("unexpected native route: %+v", prepared)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "gpt-5.6-sol" || body["instructions"] != "system" || body["prompt_cache_key"] == "" {
		t.Fatalf("unexpected native body: %s", prepared.Body)
	}

	tests := []Config{
		{APIKey: "test", BaseURL: "https://proxy.example/v1", Model: "gpt-5.6-sol", ProviderID: "openai", Protocol: ProtocolOpenAIResponses},
		{APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.6-sol", ProviderID: "custom", Protocol: ProtocolOpenAIResponses},
		{APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.6-sol", ProviderID: "openai", Protocol: ProtocolOpenAIChat},
		{APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-custom", ProviderID: "openai", Protocol: ProtocolOpenAIResponses},
		{APIKey: "test", BaseURL: "https://api.openai.com/v1", InferenceEndpoint: "https://gateway.example/responses", Model: "gpt-5.6-sol", ProviderID: "openai", Protocol: ProtocolOpenAIResponses},
	}
	for _, cfg := range tests {
		if _, err := PrepareRequest(cfg, request, false); err == nil || !strings.Contains(err.Error(), ErrNativeCompactionUnavailable.Error()) {
			t.Fatalf("inexact identity must fail closed: cfg=%+v err=%v", cfg, err)
		}
	}
}

func TestInvokeNativeCompactionReturnsOpaqueOutputAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses/compact" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		w.Header().Set("x-request-id", "req_native_1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_compact_1", "object": "response.compaction",
			"output": []any{
				map[string]any{"id": "msg_1", "type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hello"}}},
				map[string]any{"id": "cmp_1", "type": "compaction", "encrypted_content": "opaque-ciphertext"},
			},
			"usage": map[string]any{"input_tokens": 120, "output_tokens": 30, "total_tokens": 150},
		})
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	originalClient := httpClient
	httpClient = &http.Client{Transport: rewriteHostTransport{target: target, base: http.DefaultTransport}}
	defer func() { httpClient = originalClient }()

	cfg := Config{
		APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.6-sol",
		ProviderID: "openai", ProfileID: "profile-openai", CredentialRevision: 4,
		Protocol: ProtocolOpenAIResponses,
	}
	prepared, err := PrepareRequest(cfg, Request{
		NativeCompaction: true, Purpose: RequestPurposeNativeCompaction,
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "hello"}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	response, err := InvokePrepared(t.Context(), cfg, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if response.ProviderRequestID != "req_native_1" || response.NativeCompactionState == nil {
		t.Fatalf("missing native state: %+v", response)
	}
	state := response.NativeCompactionState
	if len(state.Output) != 2 || state.Digest == "" || state.CredentialRevision != 4 || state.ModelID != "gpt-5.6-sol" {
		t.Fatalf("unexpected native state: %+v", state)
	}
	if response.Usage == nil || response.Usage.InputTokens == nil || *response.Usage.InputTokens != 120 {
		t.Fatalf("unexpected native usage: %+v", response.Usage)
	}
}

func TestCompatibleNativeCompactionOutputIsReplayedOnlyForExactIdentity(t *testing.T) {
	cfg := Config{
		APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.6-sol",
		ProviderID: "openai", ProfileID: "profile-openai", CredentialRevision: 4,
		Protocol: ProtocolOpenAIResponses,
	}
	state := &NativeCompactionState{
		ProviderID: "openai", ProfileID: "profile-openai", CredentialRevision: 4,
		ModelID: "gpt-5.6-sol", ProtocolFamily: ProtocolOpenAIResponses,
		EndpointContractID: EndpointContractOpenAIResponses,
		Output: []json.RawMessage{
			json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"old user"}]}`),
			json.RawMessage(`{"type":"compaction","encrypted_content":"opaque"}`),
		},
	}
	state.Digest = nativeCompactionOutputDigest(state.Output)
	request := Request{
		Messages:              []convcontext.Message{{Role: convcontext.RoleUser, Content: "new user"}},
		NativeCompactionState: state,
	}
	prepared, err := PrepareRequest(cfg, request, false)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Input []map[string]any `json:"input"`
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Input) != 3 || body.Input[1]["type"] != "compaction" {
		t.Fatalf("compatible native output was not prepended: %s", prepared.Body)
	}

	mismatched := cfg
	mismatched.CredentialRevision = 5
	prepared, err = PrepareRequest(mismatched, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Input) != 1 {
		t.Fatalf("incompatible native output must be discarded: %s", prepared.Body)
	}

	state.Digest = strings.Repeat("0", 64)
	prepared, err = PrepareRequest(cfg, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Input) != 1 {
		t.Fatalf("digest mismatch must be discarded: %s", prepared.Body)
	}
}

func TestNativeCompactionRejectsMalformedOutput(t *testing.T) {
	user := json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}`)
	compact := json.RawMessage(`{"type":"compaction","encrypted_content":"opaque"}`)
	tests := []struct {
		name   string
		output []json.RawMessage
	}{
		{name: "empty"},
		{name: "compaction only", output: []json.RawMessage{compact}},
		{name: "multiple compaction items", output: []json.RawMessage{user, compact, compact}},
		{name: "compaction not last", output: []json.RawMessage{compact, user}},
		{name: "non-user prefix", output: []json.RawMessage{json.RawMessage(`{"type":"function_call","name":"Read"}`), compact}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateNativeCompactionOutput(test.output); err == nil {
				t.Fatalf("malformed native output was accepted: %s", test.name)
			}
		})
	}
}
