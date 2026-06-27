package structuredio

import (
	"encoding/json"
	"fmt"
)

type EnvelopeType string

const (
	EnvelopeUser            EnvelopeType = "user"
	EnvelopeAssistant       EnvelopeType = "assistant"
	EnvelopeControlRequest  EnvelopeType = "control_request"
	EnvelopeControlResponse EnvelopeType = "control_response"
	EnvelopeStreamEvent     EnvelopeType = "stream_event"
	EnvelopeKeepAlive       EnvelopeType = "keep_alive"
	EnvelopeError           EnvelopeType = "error"
)

type Diagnostic struct {
	Line    string
	Message string
}

func (d Diagnostic) Error() string {
	return d.Message
}

type Envelope struct {
	Type   EnvelopeType
	Raw    json.RawMessage
	Fields map[string]json.RawMessage
}

func ParseEnvelope(line []byte) (Envelope, *Diagnostic) {
	raw := append(json.RawMessage(nil), line...)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Envelope{}, &Diagnostic{
			Line:    string(line),
			Message: fmt.Sprintf("invalid structured IO JSON: %v", err),
		}
	}

	typeRaw, ok := fields["type"]
	if !ok {
		return Envelope{}, &Diagnostic{
			Line:    string(line),
			Message: "invalid structured IO envelope: missing type",
		}
	}

	var typ string
	if err := json.Unmarshal(typeRaw, &typ); err != nil || typ == "" {
		return Envelope{}, &Diagnostic{
			Line:    string(line),
			Message: "invalid structured IO envelope: type must be a non-empty string",
		}
	}

	return Envelope{
		Type:   EnvelopeType(typ),
		Raw:    raw,
		Fields: fields,
	}, nil
}

func (e Envelope) IsKeepAlive() bool {
	return e.Type == EnvelopeKeepAlive
}

type ControlRequest struct {
	Type      EnvelopeType    `json:"type"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
}

type ControlResponse struct {
	Type     EnvelopeType           `json:"type"`
	Response ControlResponsePayload `json:"response"`
	Raw      json.RawMessage        `json:"-"`
}

type ControlResponsePayload struct {
	RequestID string          `json:"request_id"`
	Subtype   string          `json:"subtype,omitempty"`
	Response  json.RawMessage `json:"response,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func ParseControlResponse(raw []byte) (ControlResponse, error) {
	var response ControlResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return ControlResponse{}, err
	}
	response.Raw = append(json.RawMessage(nil), raw...)
	return response, nil
}

func (e Envelope) ControlResponse() (ControlResponse, error) {
	if e.Type != EnvelopeControlResponse {
		return ControlResponse{}, fmt.Errorf("envelope type %q is not %q", e.Type, EnvelopeControlResponse)
	}
	return ParseControlResponse(e.Raw)
}
