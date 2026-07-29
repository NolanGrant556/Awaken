package llm

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
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
	if cfg.APIKey == "" {
		return PreparedRequest{}, ErrMissingAPIKey
	}
	if resolveModel(cfg, req) == "" {
		return PreparedRequest{}, ErrMissingModel
	}
	switch resolveProtocol(cfg) {
	case ProtocolAnthropicMessages:
		return prepareAnthropicRequest(cfg, req, stream)
	default:
		return prepareOpenAIRequest(cfg, req, stream)
	}
}

func InvokePrepared(ctx stdctx.Context, cfg Config, prepared PreparedRequest) (*Response, error) {
	if prepared.Stream {
		return nil, fmt.Errorf("prepared request is streaming")
	}
	switch prepared.Protocol {
	case ProtocolAnthropicMessages:
		return newAnthropicTransport().invokePrepared(ctx, cfg, prepared)
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
	switch prepared.Protocol {
	case ProtocolAnthropicMessages:
		return newAnthropicTransport().streamPrepared(ctx, cfg, prepared, observer)
	case ProtocolOpenAIChat:
		return newOpenAITransport().streamPrepared(ctx, cfg, prepared, observer)
	case ProtocolMiniMaxChat:
		return newOpenAITransport().streamPrepared(ctx, cfg, prepared, observer)
	default:
		return nil, fmt.Errorf("unsupported prepared protocol %q", prepared.Protocol)
	}
}
