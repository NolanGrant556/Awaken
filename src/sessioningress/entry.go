package sessioningress

import "encoding/json"

// Entry is the transcript object sent to session ingress.
//
// UUID is the append-chain identifier. Fields can carry transcript-specific
// top-level properties while preserving UUID as the stable local head value.
type Entry struct {
	UUID    string         `json:"uuid"`
	Type    string         `json:"type,omitempty"`
	Payload any            `json:"payload,omitempty"`
	Fields  map[string]any `json:"-"`
}

func (e Entry) MarshalJSON() ([]byte, error) {
	if len(e.Fields) == 0 {
		return json.Marshal(struct {
			UUID    string `json:"uuid"`
			Type    string `json:"type,omitempty"`
			Payload any    `json:"payload,omitempty"`
		}{
			UUID:    e.UUID,
			Type:    e.Type,
			Payload: e.Payload,
		})
	}

	out := make(map[string]any, len(e.Fields)+3)
	for key, value := range e.Fields {
		out[key] = value
	}
	out["uuid"] = e.UUID
	if e.Type != "" {
		out["type"] = e.Type
	}
	if e.Payload != nil {
		out["payload"] = e.Payload
	}
	return json.Marshal(out)
}
