package credentialpool

import (
	"encoding/json"
	"fmt"
)

const redactedSecret = "[redacted]"

type RedactedCredential struct {
	ID             string            `json:"id"`
	Provider       string            `json:"provider"`
	Label          string            `json:"label,omitempty"`
	HasSecret      bool              `json:"has_secret"`
	RedactedSecret string            `json:"secret,omitempty"`
	BaseURL        string            `json:"base_url,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func (c ProviderCredential) Redacted() RedactedCredential {
	redacted := RedactedCredential{
		ID:        c.ID,
		Provider:  c.Provider,
		Label:     c.Label,
		HasSecret: c.Secret != "",
		BaseURL:   c.BaseURL,
		Metadata:  cloneMetadata(c.Metadata),
	}
	if c.Secret != "" {
		redacted.RedactedSecret = RedactSecret(c.Secret)
	}
	return redacted
}

func (c ProviderCredential) String() string {
	secret := ""
	if c.Secret != "" {
		secret = " secret=" + RedactSecret(c.Secret)
	}
	return fmt.Sprintf(
		"credentialpool.ProviderCredential{id:%q provider:%q label:%q%s}",
		c.ID,
		c.Provider,
		c.Label,
		secret,
	)
}

func (c ProviderCredential) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Redacted())
}

func RedactSecret(secret string) string {
	if secret == "" {
		return ""
	}
	return redactedSecret
}
