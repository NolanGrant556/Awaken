package llm

import (
	"bytes"
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const NativeCompactionCapabilityOpenAIResponsesV1 = "openai_responses_compact_v1"

var ErrNativeCompactionUnavailable = errors.New("native compaction unavailable")

type NativeCompactionCapability struct {
	ID                 string
	ProviderID         string
	BaseURL            string
	ProtocolFamily     Protocol
	EndpointContractID string
	Models             map[string]struct{}
}

var nativeCompactionCapabilities = []NativeCompactionCapability{
	{
		ID:         NativeCompactionCapabilityOpenAIResponsesV1,
		ProviderID: "openai", BaseURL: "https://api.openai.com/v1",
		ProtocolFamily:     ProtocolOpenAIResponses,
		EndpointContractID: EndpointContractOpenAIResponses,
		Models:             stringSet("gpt-5.2", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-6-astra"),
	},
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[strings.TrimSpace(value)] = struct{}{}
	}
	return result
}

func ResolveNativeCompactionCapability(cfg Config, model string) (NativeCompactionCapability, bool) {
	providerID := normalizeProviderID(cfg.ProviderID)
	baseURL := normalizeCapabilityBaseURL(cfg.BaseURL)
	protocol := resolveProtocol(cfg)
	model = strings.TrimSpace(model)
	for _, capability := range nativeCompactionCapabilities {
		if capability.ProviderID != providerID ||
			normalizeCapabilityBaseURL(capability.BaseURL) != baseURL ||
			capability.ProtocolFamily != protocol {
			continue
		}
		if _, ok := capability.Models[model]; !ok {
			continue
		}
		if endpoint := strings.TrimSpace(cfg.InferenceEndpoint); endpoint != "" &&
			normalizeCapabilityEndpoint(endpoint) != "https://api.openai.com/v1/responses" {
			continue
		}
		return capability, true
	}
	return NativeCompactionCapability{}, false
}

func normalizeCapabilityEndpoint(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

type openAIResponsesCompactRequest struct {
	Model          string `json:"model"`
	Instructions   string `json:"instructions,omitempty"`
	Input          []any  `json:"input"`
	PromptCacheKey string `json:"prompt_cache_key,omitempty"`
}

type openAIResponsesCompactedResponse struct {
	ID     string                `json:"id"`
	Object string                `json:"object"`
	Output []json.RawMessage     `json:"output"`
	Usage  *openAIResponsesUsage `json:"usage,omitempty"`
}

func prepareOpenAIResponsesCompactRequest(cfg Config, req Request) (PreparedRequest, error) {
	model := resolveModel(cfg, req)
	capability, ok := ResolveNativeCompactionCapability(cfg, model)
	if !ok {
		return PreparedRequest{}, fmt.Errorf("%w: provider=%s protocol=%s model=%s", ErrNativeCompactionUnavailable, normalizeProviderID(cfg.ProviderID), resolveProtocol(cfg), model)
	}
	input, err := buildOpenAIResponsesInput(req)
	if err != nil {
		return PreparedRequest{}, err
	}
	input = prependCompatibleNativeCompactionInput(cfg, req, input)
	_, affinityKey := promptCacheMetadata(cfg, req)
	body := openAIResponsesCompactRequest{
		Model: model, Instructions: req.SystemPrompt, Input: input, PromptCacheKey: affinityKey,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("failed to marshal native compaction request: %w", err)
	}
	prepared := newPreparedRequest(req, ProtocolOpenAIResponses, strings.TrimRight(capability.BaseURL, "/")+"/responses/compact", encoded, false)
	prepared.Operation = PreparedOperationNativeCompaction
	prepared.Cache = PreparedPromptCache{
		CapabilityID: capability.ID, EndpointContract: capability.EndpointContractID,
		Mode: PromptCacheAffinity, KeyApplied: affinityKey != "",
	}
	return prepared, nil
}

func (t *openAIResponsesTransport) invokeNativeCompactPrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest) (*Response, error) {
	if prepared.Operation != PreparedOperationNativeCompaction || prepared.Protocol != ProtocolOpenAIResponses {
		return nil, fmt.Errorf("invalid native compaction prepared request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, prepared.Endpoint, bytes.NewReader(prepared.Body))
	if err != nil {
		return nil, requestBuildError(err)
	}
	applyAuthHeaders(httpReq, cfg, ProtocolOpenAIResponses)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return nil, requestFailedError(httpResp)
	}
	var payload openAIResponsesCompactedResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode native compaction response: %w", err)
	}
	if payload.Object != "response.compaction" {
		return nil, fmt.Errorf("%w: unexpected object %q", ErrNativeCompactionUnavailable, payload.Object)
	}
	if err := validateNativeCompactionOutput(payload.Output); err != nil {
		return nil, err
	}
	model := modelFromNativeCompactBody(prepared.Body)
	state := &NativeCompactionState{
		ProviderID: normalizeProviderID(cfg.ProviderID), ProfileID: strings.TrimSpace(cfg.ProfileID),
		CredentialRevision: effectiveCredentialRevision(cfg.CredentialRevision), ModelID: model,
		ProtocolFamily: ProtocolOpenAIResponses, EndpointContractID: prepared.Cache.EndpointContract,
		Output: cloneRawMessages(payload.Output),
	}
	state.Digest = nativeCompactionOutputDigest(state.Output)
	return &Response{
		Usage: normalizeOpenAIResponsesUsage(payload.Usage), IsFinished: true,
		ProviderRequestID:    firstNonEmpty(providerRequestID(httpResp), payload.ID),
		RateLimitObservation: providerRateLimitObservation(httpResp), NativeCompactionState: state,
	}, nil
}

func validateNativeCompactionOutput(output []json.RawMessage) error {
	if len(output) < 2 {
		return fmt.Errorf("%w: empty output", ErrNativeCompactionUnavailable)
	}
	compactionCount := 0
	for index, raw := range output {
		var header struct {
			Type             string `json:"type"`
			Role             string `json:"role"`
			EncryptedContent string `json:"encrypted_content"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return fmt.Errorf("%w: invalid output item %d", ErrNativeCompactionUnavailable, index)
		}
		if header.Type == "compaction" {
			compactionCount++
			if strings.TrimSpace(header.EncryptedContent) == "" || index != len(output)-1 {
				return fmt.Errorf("%w: invalid compaction item", ErrNativeCompactionUnavailable)
			}
		} else if header.Type != "message" || header.Role != "user" || index == len(output)-1 {
			return fmt.Errorf("%w: output item %d is not a user message", ErrNativeCompactionUnavailable, index)
		}
	}
	if compactionCount != 1 {
		return fmt.Errorf("%w: expected one compaction item, got %d", ErrNativeCompactionUnavailable, compactionCount)
	}
	return nil
}

func prependCompatibleNativeCompactionInput(cfg Config, req Request, input []any) []any {
	state := req.NativeCompactionState
	if !nativeCompactionStateCompatible(cfg, req, state) {
		return input
	}
	result := make([]any, 0, len(state.Output)+len(input))
	for _, raw := range state.Output {
		var item any
		if json.Unmarshal(raw, &item) == nil {
			result = append(result, item)
		}
	}
	return append(result, input...)
}

func nativeCompactionStateCompatible(cfg Config, req Request, state *NativeCompactionState) bool {
	if state == nil || state.Digest == "" || state.Digest != nativeCompactionOutputDigest(state.Output) {
		return false
	}
	model := resolveModel(cfg, req)
	capability, ok := ResolveNativeCompactionCapability(cfg, model)
	return ok &&
		state.ProviderID == normalizeProviderID(cfg.ProviderID) &&
		state.ProfileID == strings.TrimSpace(cfg.ProfileID) &&
		state.CredentialRevision == effectiveCredentialRevision(cfg.CredentialRevision) &&
		state.ModelID == model && state.ProtocolFamily == ProtocolOpenAIResponses &&
		state.EndpointContractID == capability.EndpointContractID
}

func effectiveCredentialRevision(value uint64) uint64 {
	if value == 0 {
		return 1
	}
	return value
}

func nativeCompactionOutputDigest(output []json.RawMessage) string {
	hash := sha256.New()
	for _, raw := range output {
		canonical, ok := canonicalRawMessage(raw)
		if !ok {
			return ""
		}
		hash.Write(canonical)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, len(values))
	for index, value := range values {
		if canonical, ok := canonicalRawMessage(value); ok {
			result[index] = append(json.RawMessage(nil), canonical...)
		}
	}
	return result
}

func canonicalRawMessage(raw json.RawMessage) ([]byte, bool) {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	encoded, err := json.Marshal(value)
	return encoded, err == nil
}

func modelFromNativeCompactBody(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &payload)
	return strings.TrimSpace(payload.Model)
}
