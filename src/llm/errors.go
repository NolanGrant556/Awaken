package llm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrMissingAPIKey = errors.New("LLM_API_KEY is not set in environment")
	ErrMissingModel  = errors.New("model is not specified and no default model configured")
	ErrRequestFailed = errors.New("LLM request failed")
	ErrEmptyResponse = errors.New("received empty response from LLM")
)

type ErrorKind string

const (
	ErrorKindUnknown         ErrorKind = "unknown"
	ErrorKindRateLimit       ErrorKind = "rate_limit"
	ErrorKindServerError     ErrorKind = "server_error"
	ErrorKindServerOverload  ErrorKind = "server_overload"
	ErrorKindAuthentication  ErrorKind = "authentication"
	ErrorKindContextOverflow ErrorKind = "context_overflow"
)

type RequestError struct {
	StatusCode int
	Kind       ErrorKind
}

func (e *RequestError) Error() string {
	if e == nil {
		return ErrRequestFailed.Error()
	}
	kind := e.Kind
	if kind == "" {
		kind = ErrorKindUnknown
	}
	if e.StatusCode > 0 {
		if kind != ErrorKindUnknown {
			return fmt.Sprintf("%v: status %d (%s)", ErrRequestFailed, e.StatusCode, kind)
		}
		return fmt.Sprintf("%v: status %d", ErrRequestFailed, e.StatusCode)
	}
	if kind != ErrorKindUnknown {
		return fmt.Sprintf("%v: %s", ErrRequestFailed, kind)
	}
	return ErrRequestFailed.Error()
}

func (e *RequestError) Unwrap() error {
	return ErrRequestFailed
}

func (e *RequestError) Retryable() bool {
	if e == nil {
		return false
	}
	return isRetryableErrorKind(e.Kind)
}

func ClassifyHTTPError(statusCode int, responseBody []byte) ErrorKind {
	body := strings.ToLower(string(responseBody))
	if isContextOverflowBody(body) {
		return ErrorKindContextOverflow
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return ErrorKindAuthentication
	}
	if statusCode == 529 || strings.Contains(body, "overloaded_error") {
		return ErrorKindServerOverload
	}
	if statusCode == http.StatusTooManyRequests {
		return ErrorKindRateLimit
	}
	switch statusCode {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ErrorKindServerError
	default:
		return ErrorKindUnknown
	}
}

func isContextOverflowBody(body string) bool {
	if body == "" {
		return false
	}
	patterns := []string{
		"context_length_exceeded",
		"model_context_window_exceeded",
		"prompt is too long",
		"maximum context length",
		"context length exceeded",
		"exceed context limit",
		"exceeds context limit",
		"context window exceeded",
		"context overflow",
		"input length and `max_tokens` exceed context limit",
	}
	for _, pattern := range patterns {
		if strings.Contains(body, pattern) {
			return true
		}
	}
	return false
}
