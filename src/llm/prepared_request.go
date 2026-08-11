package llm

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

func newPreparedRequest(req Request, protocol Protocol, endpoint string, body []byte, stream bool) PreparedRequest {
	digest := sha256.Sum256(body)
	return PreparedRequest{
		RequestID: req.RequestID, Protocol: protocol, Endpoint: endpoint,
		Body: append([]byte(nil), body...), RequestShapeDigest: hex.EncodeToString(digest[:]),
		Stream: stream, StreamIdleTimeout: req.StreamIdleTimeout,
	}
}

// PrepareRequest performs provider protocol lowering exactly once.
func PrepareRequest(cfg Config, req Request, stream bool) (PreparedRequest, error) {
	if apiKeyRequired(cfg) && cfg.APIKey == "" {
		return PreparedRequest{}, ErrMissingAPIKey
	}
	if resolveModel(cfg, req) == "" {
		return PreparedRequest{}, ErrMissingModel
	}
	switch resolveProtocol(cfg) {
	case ProtocolAnthropicMessages:
		return prepareAnthropicRequest(cfg, req, stream)
	case ProtocolOpenAIResponses:
		return prepareOpenAIResponsesRequest(cfg, req, stream)
	case ProtocolOpenAIChat, ProtocolMiniMaxChat:
		return prepareOpenAIRequest(cfg, req, stream)
	default:
		return PreparedRequest{}, fmt.Errorf("%w: %q", ErrUnsupportedProtocol, cfg.Protocol)
	}
}

func InvokePrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest) (*Response, error) {
	if prepared.Stream {
		return nil, fmt.Errorf("prepared request is streaming")
	}
	switch prepared.Protocol {
	case ProtocolAnthropicMessages:
		return newAnthropicTransport().invokePrepared(ctx, cfg, prepared)
	case ProtocolOpenAIResponses:
		return newOpenAIResponsesTransport().invokePrepared(ctx, cfg, prepared)
	case ProtocolOpenAIChat:
		return newOpenAITransport().invokePrepared(ctx, cfg, prepared)
	case ProtocolMiniMaxChat:
		return newOpenAITransport().invokePrepared(ctx, cfg, prepared)
	default:
		return nil, fmt.Errorf("unsupported prepared protocol %q", prepared.Protocol)
	}
}

func StreamPreparedWithObserver(ctx stdctx.Context, cfg Config, prepared PreparedRequest, observer StreamObserver) (*Response, error) {
	if !prepared.Stream {
		return nil, fmt.Errorf("prepared request is not streaming")
	}
	originalOnEvent := observer.OnEvent
	terminalEventSent := false
	observer.OnEvent = func(event StreamEvent) {
		if event.Type == StreamEventCompleted || event.Type == StreamEventFailed {
			terminalEventSent = true
		}
		if originalOnEvent != nil {
			originalOnEvent(event)
		}
	}

	var response *Response
	var err error
	switch prepared.Protocol {
	case ProtocolAnthropicMessages:
		response, err = newAnthropicTransport().streamPrepared(ctx, cfg, prepared, observer)
	case ProtocolOpenAIResponses:
		response, err = newOpenAIResponsesTransport().streamPrepared(ctx, cfg, prepared, observer)
	case ProtocolOpenAIChat:
		response, err = newOpenAITransport().streamPrepared(ctx, cfg, prepared, observer)
	case ProtocolMiniMaxChat:
		response, err = newOpenAITransport().streamPrepared(ctx, cfg, prepared, observer)
	default:
		err = fmt.Errorf("%w: prepared protocol %q", ErrUnsupportedProtocol, prepared.Protocol)
	}
	if !terminalEventSent && originalOnEvent != nil {
		if err != nil {
			originalOnEvent(StreamEvent{Type: StreamEventFailed, ErrorCode: streamFailureCode(err)})
		} else {
			originalOnEvent(StreamEvent{Type: StreamEventCompleted, Usage: response.Usage, StopReason: response.StopReason})
		}
	}
	return response, err
}

func streamFailureCode(err error) string {
	if status, ok := RequestStatus(err); ok {
		return fmt.Sprintf("http_%d", status)
	}
	switch {
	case errors.Is(err, ErrEmptyResponse):
		return "empty_response"
	case errors.Is(err, ErrUnsupportedProtocol):
		return "unsupported_protocol"
	case errors.Is(err, stdctx.Canceled):
		return "canceled"
	case errors.Is(err, stdctx.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "stream_error"
	}
}
