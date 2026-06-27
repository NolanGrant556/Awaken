package mcpoauth

import "encoding/json"

const redactedSecret = "<redacted>"

// Secret stores sensitive OAuth values while keeping display output redacted.
type Secret string

func NewSecret(value string) Secret {
	return Secret(value)
}

func (s Secret) Value() string {
	return string(s)
}

func (s Secret) IsZero() bool {
	return s == ""
}

func (s Secret) Redacted() string {
	if s.IsZero() {
		return ""
	}
	return redactedSecret
}

func (s Secret) String() string {
	return s.Redacted()
}

func (s Secret) GoString() string {
	return s.Redacted()
}

func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Value())
}

func (s *Secret) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = Secret(value)
	return nil
}
