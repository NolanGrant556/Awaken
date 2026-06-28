package providererrors

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Reason string

const (
	ReasonAuth                  Reason = "auth"
	ReasonAuthPermanent         Reason = "auth_permanent"
	ReasonBilling               Reason = "billing"
	ReasonRateLimit             Reason = "rate_limit"
	ReasonOverloaded            Reason = "overloaded"
	ReasonServerError           Reason = "server_error"
	ReasonTimeout               Reason = "timeout"
	ReasonContextOverflow       Reason = "context_overflow"
	ReasonPayloadTooLarge       Reason = "payload_too_large"
	ReasonImageTooLarge         Reason = "image_too_large"
	ReasonModelNotFound         Reason = "model_not_found"
	ReasonProviderPolicyBlocked Reason = "provider_policy_blocked"
	ReasonFormatError           Reason = "format_error"
	ReasonThinkingSignature     Reason = "thinking_signature"
	ReasonLongContextTier       Reason = "long_context_tier"
	ReasonUnknown               Reason = "unknown"
)

type ClassificationInput struct {
	StatusCode    int
	Provider      string
	Model         string
	Message       string
	ErrorCode     string
	Body          string
	ApproxTokens  int
	ContextLength int
	MessageCount  int
	ErrorType     string
}

type ClassifiedError struct {
	Reason                 Reason            `json:"reason"`
	Retryable              bool              `json:"retryable"`
	ShouldCompress         bool              `json:"should_compress"`
	ShouldRotateCredential bool              `json:"should_rotate_credential"`
	ShouldFallback         bool              `json:"should_fallback"`
	Message                string            `json:"message"`
	Metadata               map[string]string `json:"metadata,omitempty"`
}

func Classify(input ClassificationInput) ClassifiedError {
	ctx := newClassificationContext(input)

	if hasAny(ctx.searchText, providerPolicyBlockedPatterns) {
		return ctx.result(ReasonProviderPolicyBlocked, false, false, false, false, "provider_specific")
	}
	if input.StatusCode == 400 && strings.Contains(ctx.searchText, "signature") && strings.Contains(ctx.searchText, "thinking") {
		return ctx.result(ReasonThinkingSignature, true, false, false, false, "provider_specific")
	}
	if input.StatusCode == 429 && strings.Contains(ctx.searchText, "extra usage") && (strings.Contains(ctx.searchText, "long context") || strings.Contains(ctx.searchText, "long-context")) {
		return ctx.result(ReasonLongContextTier, true, true, false, false, "provider_specific")
	}

	if input.StatusCode > 0 {
		if classified, ok := ctx.classifyByStatus(); ok {
			return classified
		}
	}

	if input.StatusCode == 0 && input.ErrorType == "RateLimitError" {
		return ctx.result(ReasonRateLimit, true, false, true, true, "error_type")
	}

	if ctx.errorCode != "" {
		if classified, ok := ctx.classifyByErrorCode(ctx.errorCode); ok {
			return classified
		}
	}
	for _, code := range ctx.bodyCodes {
		if classified, ok := ctx.classifyByErrorCode(code); ok {
			return classified
		}
	}

	if classified, ok := ctx.classifyByMessage(); ok {
		return classified
	}

	if hasAny(ctx.searchText, sslTransientPatterns) || isSSLErrorType(ctx.input.ErrorType) {
		return ctx.result(ReasonTimeout, true, false, false, false, "ssl_tls_transient")
	}

	if hasAny(ctx.searchText, serverDisconnectPatterns) {
		if ctx.isLargeDisconnectContext() {
			return ctx.result(ReasonContextOverflow, true, true, false, false, "server_disconnect_large_context")
		}
		return ctx.result(ReasonTimeout, true, false, false, false, "server_disconnect")
	}

	if isTransportErrorType(input.ErrorType) {
		return ctx.result(ReasonTimeout, true, false, false, false, "transport_type")
	}

	return ctx.result(ReasonUnknown, false, false, false, false, "unknown")
}

type classificationContext struct {
	input        ClassificationInput
	errorCode    string
	bodyCodes    []string
	bodyMessages []string
	searchText   string
	message      string
}

func newClassificationContext(input ClassificationInput) classificationContext {
	bodyMessages, bodyCodes := extractBodySignals(input.Body)
	messageParts := make([]string, 0, 4+len(bodyMessages))
	messageParts = appendIfPresent(messageParts, input.Message)
	messageParts = appendIfPresent(messageParts, input.Body)
	messageParts = append(messageParts, bodyMessages...)
	messageParts = appendIfPresent(messageParts, input.ErrorCode)
	messageParts = appendIfPresent(messageParts, input.ErrorType)

	message := strings.TrimSpace(input.Message)
	if message == "" && len(bodyMessages) > 0 {
		message = strings.TrimSpace(bodyMessages[0])
	}
	if message == "" {
		message = strings.TrimSpace(input.Body)
	}

	code := strings.TrimSpace(input.ErrorCode)
	if code == "" && len(bodyCodes) > 0 {
		code = bodyCodes[0]
	}

	return classificationContext{
		input:        input,
		errorCode:    strings.ToLower(code),
		bodyCodes:    lowerNonEmpty(bodyCodes),
		bodyMessages: bodyMessages,
		searchText:   strings.ToLower(strings.Join(messageParts, " ")),
		message:      truncate(message, 500),
	}
}

func (ctx classificationContext) classifyByStatus() (ClassifiedError, bool) {
	switch ctx.input.StatusCode {
	case 401:
		return ctx.result(ReasonAuth, false, false, true, true, "http_status"), true
	case 403:
		if hasAny(ctx.searchText, billingPatterns) || strings.Contains(ctx.searchText, "key limit exceeded") || strings.Contains(ctx.searchText, "spending limit") {
			return ctx.result(ReasonBilling, false, false, true, true, "http_status"), true
		}
		return ctx.result(ReasonAuth, false, false, true, true, "http_status"), true
	case 402:
		if hasUsageLimit(ctx.searchText) && hasAny(ctx.searchText, usageLimitTransientSignals) {
			return ctx.result(ReasonRateLimit, true, false, true, true, "http_status"), true
		}
		return ctx.result(ReasonBilling, false, false, true, true, "http_status"), true
	case 404:
		if hasAny(ctx.searchText, modelNotFoundPatterns) {
			return ctx.result(ReasonModelNotFound, false, false, false, true, "http_status"), true
		}
		return ctx.result(ReasonUnknown, false, false, false, false, "http_status"), true
	case 413:
		return ctx.result(ReasonPayloadTooLarge, true, true, false, false, "http_status"), true
	case 429:
		return ctx.result(ReasonRateLimit, true, false, true, true, "http_status"), true
	case 400:
		return ctx.classifyBadRequest(), true
	case 500, 502:
		return ctx.result(ReasonServerError, true, false, false, false, "http_status"), true
	case 503, 529:
		return ctx.result(ReasonOverloaded, true, false, false, false, "http_status"), true
	}

	if ctx.input.StatusCode >= 400 && ctx.input.StatusCode < 500 {
		return ctx.result(ReasonFormatError, false, false, false, true, "http_status"), true
	}
	if ctx.input.StatusCode >= 500 && ctx.input.StatusCode < 600 {
		return ctx.result(ReasonServerError, true, false, false, false, "http_status"), true
	}
	return ClassifiedError{}, false
}

func (ctx classificationContext) classifyBadRequest() ClassifiedError {
	if hasAny(ctx.searchText, imageTooLargePatterns) {
		return ctx.result(ReasonImageTooLarge, true, false, false, false, "http_status")
	}
	if ctx.hasErrorCode("image_too_large") {
		return ctx.result(ReasonImageTooLarge, true, false, false, false, "http_status")
	}
	if ctx.hasErrorCode("payload_too_large", "request_too_large") {
		return ctx.result(ReasonPayloadTooLarge, true, true, false, false, "http_status")
	}
	if ctx.hasErrorCode("context_length_exceeded", "max_tokens_exceeded", "context_overflow") {
		return ctx.result(ReasonContextOverflow, true, true, false, false, "http_status")
	}
	if hasAny(ctx.searchText, contextOverflowPatterns) {
		return ctx.result(ReasonContextOverflow, true, true, false, false, "http_status")
	}
	if ctx.hasErrorCode("model_not_found", "model_not_available", "invalid_model") {
		return ctx.result(ReasonModelNotFound, false, false, false, true, "http_status")
	}
	if hasAny(ctx.searchText, modelNotFoundPatterns) {
		return ctx.result(ReasonModelNotFound, false, false, false, true, "http_status")
	}
	if ctx.hasErrorCode("resource_exhausted", "throttled", "rate_limit_exceeded", "too_many_requests") {
		return ctx.result(ReasonRateLimit, true, false, true, true, "http_status")
	}
	if hasAny(ctx.searchText, rateLimitPatterns) {
		return ctx.result(ReasonRateLimit, true, false, true, true, "http_status")
	}
	if ctx.hasErrorCode("insufficient_quota", "billing_not_active", "payment_required", "credits_exhausted") {
		return ctx.result(ReasonBilling, false, false, true, true, "http_status")
	}
	if hasAny(ctx.searchText, billingPatterns) {
		return ctx.result(ReasonBilling, false, false, true, true, "http_status")
	}
	if ctx.isGenericLargeBadRequest() {
		return ctx.result(ReasonContextOverflow, true, true, false, false, "http_status")
	}
	return ctx.result(ReasonFormatError, false, false, false, true, "http_status")
}

func (ctx classificationContext) classifyByErrorCode(code string) (ClassifiedError, bool) {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "auth_permanent":
		return ctx.result(ReasonAuthPermanent, false, false, false, true, "error_code"), true
	case "invalid_api_key", "unauthorized", "authentication_error", "permission_denied":
		return ctx.result(ReasonAuth, false, false, true, true, "error_code"), true
	case "resource_exhausted", "throttled", "rate_limit_exceeded", "too_many_requests":
		return ctx.result(ReasonRateLimit, true, false, true, true, "error_code"), true
	case "insufficient_quota", "billing_not_active", "payment_required", "credits_exhausted":
		return ctx.result(ReasonBilling, false, false, true, true, "error_code"), true
	case "model_not_found", "model_not_available", "invalid_model":
		return ctx.result(ReasonModelNotFound, false, false, false, true, "error_code"), true
	case "context_length_exceeded", "max_tokens_exceeded", "context_overflow":
		return ctx.result(ReasonContextOverflow, true, true, false, false, "error_code"), true
	case "payload_too_large", "request_too_large":
		return ctx.result(ReasonPayloadTooLarge, true, true, false, false, "error_code"), true
	case "image_too_large":
		return ctx.result(ReasonImageTooLarge, true, false, false, false, "error_code"), true
	}
	return ClassifiedError{}, false
}

func (ctx classificationContext) classifyByMessage() (ClassifiedError, bool) {
	if hasAny(ctx.searchText, payloadTooLargePatterns) {
		return ctx.result(ReasonPayloadTooLarge, true, true, false, false, "message_pattern"), true
	}
	if hasAny(ctx.searchText, imageTooLargePatterns) {
		return ctx.result(ReasonImageTooLarge, true, false, false, false, "message_pattern"), true
	}
	if hasUsageLimit(ctx.searchText) {
		if hasAny(ctx.searchText, usageLimitTransientSignals) {
			return ctx.result(ReasonRateLimit, true, false, true, true, "message_pattern"), true
		}
		return ctx.result(ReasonBilling, false, false, true, true, "message_pattern"), true
	}
	if hasAny(ctx.searchText, billingPatterns) {
		return ctx.result(ReasonBilling, false, false, true, true, "message_pattern"), true
	}
	if hasAny(ctx.searchText, rateLimitPatterns) {
		return ctx.result(ReasonRateLimit, true, false, true, true, "message_pattern"), true
	}
	if hasAny(ctx.searchText, contextOverflowPatterns) {
		return ctx.result(ReasonContextOverflow, true, true, false, false, "message_pattern"), true
	}
	if hasAny(ctx.searchText, authPermanentPatterns) {
		return ctx.result(ReasonAuthPermanent, false, false, false, true, "message_pattern"), true
	}
	if hasAny(ctx.searchText, authPatterns) {
		return ctx.result(ReasonAuth, false, false, true, true, "message_pattern"), true
	}
	if hasAny(ctx.searchText, modelNotFoundPatterns) {
		return ctx.result(ReasonModelNotFound, false, false, false, true, "message_pattern"), true
	}
	return ClassifiedError{}, false
}

func (ctx classificationContext) result(reason Reason, retryable bool, compress bool, rotate bool, fallback bool, matchedBy string) ClassifiedError {
	metadata := map[string]string{}
	if matchedBy != "" {
		metadata["matched_by"] = matchedBy
	}
	if ctx.input.StatusCode > 0 {
		metadata["status_code"] = strconv.Itoa(ctx.input.StatusCode)
	}
	if ctx.errorCode != "" {
		metadata["error_code"] = ctx.errorCode
	}
	if provider := strings.TrimSpace(ctx.input.Provider); provider != "" {
		metadata["provider"] = provider
	}
	if model := strings.TrimSpace(ctx.input.Model); model != "" {
		metadata["model"] = model
	}
	if errorType := strings.TrimSpace(ctx.input.ErrorType); errorType != "" {
		metadata["error_type"] = errorType
	}

	return ClassifiedError{
		Reason:                 reason,
		Retryable:              retryable,
		ShouldCompress:         compress,
		ShouldRotateCredential: rotate,
		ShouldFallback:         fallback,
		Message:                userMessage(reason, ctx.input.StatusCode),
		Metadata:               metadata,
	}
}

func userMessage(reason Reason, statusCode int) string {
	switch reason {
	case ReasonAuth:
		return "Provider authentication failed. Check the configured API key or token."
	case ReasonAuthPermanent:
		return "Provider credentials were revoked or permanently disabled."
	case ReasonBilling:
		return "Provider quota or billing limit has been reached."
	case ReasonRateLimit:
		return "Provider rate limit reached. Retry later."
	case ReasonOverloaded:
		return "Provider is temporarily overloaded. Retry later."
	case ReasonServerError:
		return "Provider request failed with a server error."
	case ReasonTimeout:
		return "Provider request timed out or the network connection was interrupted."
	case ReasonContextOverflow:
		return "Prompt exceeds the model context window. Compress or shorten the conversation."
	case ReasonPayloadTooLarge:
		return "Provider request payload is too large. Compress or reduce the request size."
	case ReasonImageTooLarge:
		return "An attached image exceeds the provider size limit."
	case ReasonModelNotFound:
		return "Selected model is unavailable for this provider."
	case ReasonProviderPolicyBlocked:
		return "Provider blocked the request because of account or policy restrictions."
	case ReasonFormatError:
		return "Provider rejected the request as invalid."
	case ReasonThinkingSignature:
		return "Provider rejected stale thinking or signature blocks. Retry after rebuilding the request."
	case ReasonLongContextTier:
		return "Long-context usage is unavailable for this account. Retry with a shorter context or enable the required tier."
	default:
		if statusCode > 0 {
			return fmt.Sprintf("Provider request failed with status %d.", statusCode)
		}
		return "Provider request failed for an unknown reason."
	}
}

func (ctx classificationContext) isGenericLargeBadRequest() bool {
	msg := strings.TrimSpace(strings.ToLower(ctx.message))
	generic := msg == "" || msg == "error" || len(msg) < 30
	return generic && (ctx.input.MessageCount > 80 ||
		ctx.input.ApproxTokens > 80000 ||
		(ctx.input.ContextLength > 0 && ctx.input.ApproxTokens > int(float64(ctx.input.ContextLength)*0.4)))
}

func (ctx classificationContext) isLargeDisconnectContext() bool {
	return ctx.input.MessageCount > 200 ||
		ctx.input.ApproxTokens > 120000 ||
		(ctx.input.ContextLength > 0 && ctx.input.ApproxTokens > int(float64(ctx.input.ContextLength)*0.6))
}

func (ctx classificationContext) hasErrorCode(codes ...string) bool {
	for _, code := range codes {
		if ctx.errorCode == code {
			return true
		}
		for _, bodyCode := range ctx.bodyCodes {
			if bodyCode == code {
				return true
			}
		}
	}
	return false
}

func extractBodySignals(body string) ([]string, []string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return []string{body}, nil
	}

	var messages []string
	var codes []string
	walkJSON(decoded, &messages, &codes)
	return messages, codes
}

func walkJSON(value any, messages *[]string, codes *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lowerKey := strings.ToLower(key)
			if lowerKey == "message" {
				if msg, ok := stringValue(child); ok {
					*messages = append(*messages, msg)
				}
			}
			if lowerKey == "code" || lowerKey == "error_code" || lowerKey == "type" {
				if code, ok := stringValue(child); ok {
					*codes = append(*codes, code)
				}
			}
			if lowerKey == "raw" {
				if raw, ok := child.(string); ok && strings.TrimSpace(raw) != "" {
					var nested any
					if err := json.Unmarshal([]byte(raw), &nested); err == nil {
						walkJSON(nested, messages, codes)
					} else {
						*messages = append(*messages, raw)
					}
				}
			}
			walkJSON(child, messages, codes)
		}
	case []any:
		for _, child := range typed {
			walkJSON(child, messages, codes)
		}
	}
}

func stringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), strings.TrimSpace(typed) != ""
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%.0f", typed)), true
	case bool:
		return strconv.FormatBool(typed), true
	default:
		return "", false
	}
}

func appendIfPresent(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	return append(values, value)
}

func lowerNonEmpty(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func hasUsageLimit(text string) bool {
	return hasAny(text, usageLimitPatterns)
}

func hasAny(text string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func isTransportErrorType(errorType string) bool {
	normalized := strings.TrimSpace(errorType)
	if normalized == "" {
		return false
	}
	if transportErrorTypes[normalized] {
		return true
	}
	lower := strings.ToLower(normalized)
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection") ||
		strings.Contains(lower, "transport") ||
		strings.Contains(lower, "network")
}

func isSSLErrorType(errorType string) bool {
	normalized := strings.TrimSpace(errorType)
	lower := strings.ToLower(normalized)
	return lower == "sslerror" || strings.HasPrefix(lower, "ssl") || strings.Contains(lower, ".ssl")
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

var billingPatterns = []string{
	"insufficient credits",
	"insufficient_quota",
	"insufficient balance",
	"credit balance",
	"credits have been exhausted",
	"top up your credits",
	"payment required",
	"billing hard limit",
	"exceeded your current quota",
	"account is deactivated",
	"plan does not include",
}

var rateLimitPatterns = []string{
	"rate limit",
	"rate_limit",
	"too many requests",
	"throttled",
	"requests per minute",
	"tokens per minute",
	"requests per day",
	"try again in",
	"please retry after",
	"resource_exhausted",
	"rate increased too quickly",
	"throttlingexception",
	"too many concurrent requests",
	"servicequotaexceededexception",
}

var usageLimitPatterns = []string{
	"usage limit",
	"quota",
	"limit exceeded",
	"key limit exceeded",
}

var usageLimitTransientSignals = []string{
	"try again",
	"retry",
	"resets at",
	"reset in",
	"wait",
	"requests remaining",
	"periodic",
	"window",
}

var payloadTooLargePatterns = []string{
	"request entity too large",
	"payload too large",
	"error code: 413",
}

var imageTooLargePatterns = []string{
	"image exceeds",
	"image too large",
	"image_too_large",
	"image size exceeds",
}

var contextOverflowPatterns = []string{
	"context length",
	"context size",
	"maximum context",
	"token limit",
	"too many tokens",
	"reduce the length",
	"exceeds the limit",
	"context window",
	"prompt is too long",
	"prompt exceeds max length",
	"max_tokens",
	"maximum number of tokens",
	"exceeds the max_model_len",
	"max_model_len",
	"prompt length",
	"input is too long",
	"maximum model length",
	"context length exceeded",
	"truncating input",
	"slot context",
	"n_ctx_slot",
	"超过最大长度",
	"上下文长度",
	"max input token",
	"input token",
	"exceeds the maximum number of input tokens",
}

var modelNotFoundPatterns = []string{
	"is not a valid model",
	"invalid model",
	"model not found",
	"model_not_found",
	"does not exist",
	"no such model",
	"unknown model",
	"unsupported model",
}

var providerPolicyBlockedPatterns = []string{
	"no endpoints available matching your guardrail",
	"no endpoints available matching your data policy",
	"no endpoints found matching your data policy",
	"openrouter.ai/settings/privacy",
}

var authPermanentPatterns = []string{
	"credential revoked",
	"key revoked",
	"account is deactivated",
}

var authPatterns = []string{
	"invalid api key",
	"invalid_api_key",
	"authentication",
	"unauthorized",
	"forbidden",
	"invalid token",
	"token expired",
	"token revoked",
	"access denied",
}

var serverDisconnectPatterns = []string{
	"server disconnected",
	"peer closed connection",
	"connection reset by peer",
	"connection was closed",
	"network connection lost",
	"unexpected eof",
	"incomplete chunked read",
}

var sslTransientPatterns = []string{
	"bad record mac",
	"ssl alert",
	"tls alert",
	"ssl handshake failure",
	"tlsv1 alert",
	"sslv3 alert",
	"bad_record_mac",
	"ssl_alert",
	"tls_alert",
	"tls_alert_internal_error",
	"[ssl:",
}

var transportErrorTypes = map[string]bool{
	"ReadTimeout":             true,
	"ConnectTimeout":          true,
	"PoolTimeout":             true,
	"ConnectError":            true,
	"RemoteProtocolError":     true,
	"ConnectionError":         true,
	"ConnectionResetError":    true,
	"ConnectionAbortedError":  true,
	"BrokenPipeError":         true,
	"TimeoutError":            true,
	"ReadError":               true,
	"ServerDisconnectedError": true,
	"SSLError":                true,
	"SSLZeroReturnError":      true,
	"SSLWantReadError":        true,
	"SSLWantWriteError":       true,
	"SSLEOFError":             true,
	"SSLSyscallError":         true,
	"APIConnectionError":      true,
	"APITimeoutError":         true,
}
