package llm

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrMissingAPIKey = errors.New("LLM_API_KEY is not set in environment")
	ErrMissingModel  = errors.New("model is not specified and no default model configured")
	ErrRequestFailed = errors.New("LLM request failed")
	ErrEmptyResponse = errors.New("received empty response from LLM")
)

// RequestError preserves provider HTTP retry facts without exposing request
// URLs or credentials. It still unwraps to ErrRequestFailed for compatibility.
type RequestError struct {
	StatusCode int
	RetryAfter time.Duration
	Detail     string
}

// TransportError represents a request that failed before the provider returned
// an HTTP response. It deliberately carries no URL or underlying network error
// so credentials and endpoint details cannot leak into journals or UI events.
// Because no response stream began, the agent runner may safely retry it within
// the same turn.
type TransportError struct{}

func (e *TransportError) Error() string { return ErrRequestFailed.Error() + ": transport error" }

func (e *TransportError) Unwrap() error { return ErrRequestFailed }

func (e *RequestError) Error() string {
	if e == nil {
		return ErrRequestFailed.Error()
	}
	if e.Detail != "" {
		return fmt.Sprintf("%s: status %d: %s", ErrRequestFailed, e.StatusCode, e.Detail)
	}
	return fmt.Sprintf("%s: status %d", ErrRequestFailed, e.StatusCode)
}

func (e *RequestError) Unwrap() error { return ErrRequestFailed }

func RequestStatus(err error) (int, bool) {
	var requestError *RequestError
	if !errors.As(err, &requestError) || requestError == nil || requestError.StatusCode <= 0 {
		return 0, false
	}
	return requestError.StatusCode, true
}

func RequestRetryAfter(err error) (time.Duration, bool) {
	var requestError *RequestError
	if !errors.As(err, &requestError) || requestError == nil || requestError.RetryAfter <= 0 {
		return 0, false
	}
	return requestError.RetryAfter, true
}

func IsTransientRequestError(err error) bool {
	var transportError *TransportError
	if errors.As(err, &transportError) {
		return true
	}
	status, ok := RequestStatus(err)
	return ok && (status == 429 || status >= 500)
}
