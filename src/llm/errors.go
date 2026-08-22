package llm

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMissingAPIKey       = errors.New("LLM_API_KEY is not set in environment")
	ErrMissingModel        = errors.New("model is not specified and no default model configured")
	ErrRequestFailed       = errors.New("LLM request failed")
	ErrEmptyResponse       = errors.New("received empty response from LLM")
	ErrUnsupportedProtocol = errors.New("unsupported LLM protocol")
)

// RequestError preserves provider HTTP retry facts without exposing request
// URLs or credentials. It still unwraps to ErrRequestFailed for compatibility.
type RequestError struct {
	StatusCode int
	RetryAfter time.Duration
	Detail     string
}

// TransportError represents a request that failed before the provider returned
// an HTTP response. Error() stays generic so journals never leak URLs or
// hosts. Reason is a classified, URL-free phrase for reconnect UI.
// Because no response stream began, the agent runner may retry it while the
// run stays active.
type TransportError struct {
	Reason string
}

func (e *TransportError) Error() string { return ErrRequestFailed.Error() + ": transport error" }

func (e *TransportError) Unwrap() error { return ErrRequestFailed }

// StreamDisconnectedError is a body/read failure after the provider accepted
// the request. It is retryable only when the runner has not started a tool
// stream. Idle timeouts are not this type.
type StreamDisconnectedError struct {
	Reason string
}

func (e *StreamDisconnectedError) Error() string {
	if e == nil || e.Reason == "" {
		return ErrRequestFailed.Error() + ": stream disconnected before completion"
	}
	return fmt.Sprintf("%s: stream disconnected before completion: %s", ErrRequestFailed, e.Reason)
}

func (e *StreamDisconnectedError) Unwrap() error { return ErrRequestFailed }

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
	if IsConnectionError(err) {
		return true
	}
	var streamError *StreamDisconnectedError
	if errors.As(err, &streamError) {
		return true
	}
	status, ok := RequestStatus(err)
	return ok && (status == 429 || status >= 500)
}

// IsConnectionError reports a failure that happened before any HTTP response.
// The runner keeps the turn alive and waits for the network instead of ending
// the run.
func IsConnectionError(err error) bool {
	var transportError *TransportError
	return errors.As(err, &transportError)
}

// SafeRetryDetail returns a URL-free phrase for reconnect UI and diagnostics.
func SafeRetryDetail(err error) string {
	var transportError *TransportError
	if errors.As(err, &transportError) {
		if transportError != nil && strings.TrimSpace(transportError.Reason) != "" {
			return transportError.Reason
		}
		return "error sending request"
	}
	var streamError *StreamDisconnectedError
	if errors.As(err, &streamError) {
		if streamError != nil && strings.TrimSpace(streamError.Reason) != "" {
			return streamError.Reason
		}
		return "stream eof"
	}
	if status, ok := RequestStatus(err); ok {
		return fmt.Sprintf("status %d", status)
	}
	return "connection failed"
}

func sanitizeNetworkReason(err error) string {
	if err == nil {
		return "error sending request"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "handshake") && strings.Contains(text, "eof"):
		return "tls handshake eof"
	case strings.Contains(text, "tls") && strings.Contains(text, "eof"):
		return "tls handshake eof"
	case strings.Contains(text, "error sending request"), strings.Contains(text, "sendrequest"):
		return "error sending request"
	case strings.Contains(text, "connection refused"):
		return "connection refused"
	case strings.Contains(text, "connection reset"):
		return "connection reset"
	case strings.Contains(text, "broken pipe"):
		return "broken pipe"
	case strings.Contains(text, "network is unreachable"), strings.Contains(text, "no route to host"):
		return "network unreachable"
	case strings.Contains(text, "i/o timeout"):
		return "connection timeout"
	case strings.Contains(text, "no such host"), strings.Contains(text, "server misbehaving"):
		return "name resolution failed"
	case strings.Contains(text, "unexpected eof"), strings.Contains(text, "eof"):
		return "stream eof"
	default:
		return "stream disconnected before completion"
	}
}
