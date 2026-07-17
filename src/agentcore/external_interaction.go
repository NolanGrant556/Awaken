package agentcore

import (
	"encoding/json"
	"net/url"
	"strings"

	"awaken/src/tool"
)

// ExternalInteractionResolution is the Desktop/Main answer to a pending
// interaction_required wait. It must never carry device codes, tokens, secrets,
// or raw CLI stdout/stderr.
type ExternalInteractionResolution string

const (
	ExternalInteractionCompleted ExternalInteractionResolution = "completed"
	ExternalInteractionCancelled ExternalInteractionResolution = "cancelled"
	ExternalInteractionExpired   ExternalInteractionResolution = "expired"
	ExternalInteractionFailed    ExternalInteractionResolution = "failed"
)

// ExternalInteractionView is the safe, public projection shown by
// ExternalActionGate. Sensitive fields are intentionally absent.
type ExternalInteractionView struct {
	InteractionID   string   `json:"interactionId"`
	Title           string   `json:"title,omitempty"`
	SafeMessage     string   `json:"safeMessage,omitempty"`
	VerificationURL string   `json:"verificationUrl,omitempty"`
	QRImageURL      string   `json:"qrImageUrl,omitempty"`
	Provider        string   `json:"provider,omitempty"`
	ToolCallID      string   `json:"toolCallId,omitempty"`
	ToolName        string   `json:"toolName,omitempty"`
	Recoveries      []string `json:"recoveries,omitempty"`
	RequiresReplay  bool     `json:"requiresReplay,omitempty"`
}

// DetectCLIAuthInteraction inspects a Bash/process outcome for official CLI
// authorization signals. It returns a safe view plus whether the original call
// should be replayed once after host verification.
//
// Raw stdout/stderr, device codes, tokens and secrets are never copied into the
// returned view.
func DetectCLIAuthInteraction(toolName string, outcome tool.ExecutionOutcome) (ExternalInteractionView, bool) {
	name := strings.TrimSpace(strings.ToLower(toolName))
	if name != "bash" && name != "shell" {
		return ExternalInteractionView{}, false
	}

	payload := parseBashPayload(outcome.Content)
	if payload == nil {
		payload = parseBashPayload(outcome.Summary)
	}
	stdout := stringField(payload, "stdout")
	stderr := stringField(payload, "stderr")
	exitCode := intField(payload, "exit_code")
	if outcome.ExitCode != nil && exitCode == 0 {
		exitCode = *outcome.ExitCode
	}

	envelope := firstJSONObject(stdout, stderr)
	if envelope == nil {
		return ExternalInteractionView{}, false
	}

	// DingTalk DWS host-owned auth: exit 4 + authorizationUrl.
	if exitCode == 4 {
		authURL := opaqueHTTPSURL(stringField(envelope, "data.authorizationUrl"), stringMapField(envelope, "data", "authorizationUrl"))
		if authURL == "" {
			authURL = opaqueHTTPSURL(stringField(envelope, "authorizationUrl"), stringField(envelope, "authorization_url"))
		}
		if authURL != "" {
			return ExternalInteractionView{
				Title:           "钉钉授权",
				SafeMessage:     "完成钉钉授权后，Awaken 会验证状态并继续原任务",
				VerificationURL: authURL,
				Provider:        "dingtalk-workspace",
				Recoveries:      []string{"open_url", "confirm_done", "cancel"},
				RequiresReplay:  true,
			}, true
		}
	}

	// Lark: login initiation with verification_url, or missing_scopes guidance.
	verificationURL := opaqueHTTPSURL(
		stringField(envelope, "verification_url"),
		stringField(envelope, "verification_uri_complete"),
		stringField(envelope, "verification_uri"),
	)
	consoleURL := opaqueHTTPSURL(stringField(envelope, "console_url"))
	missingScopes := stringSliceField(envelope, "missing_scopes")
	hint := sanitizeHint(stringField(envelope, "hint"))

	if verificationURL != "" {
		// Login initiation (--no-wait) returns a URL for the user; do not replay
		// the initiation command. Business calls that lack scopes still replay.
		return ExternalInteractionView{
			Title:           "飞书授权",
			SafeMessage:     firstNonEmpty(hint, "请在浏览器或飞书中完成授权"),
			VerificationURL: verificationURL,
			Provider:        "lark-cli",
			Recoveries:      []string{"open_url", "confirm_done", "cancel"},
			RequiresReplay:  false,
		}, true
	}

	if len(missingScopes) > 0 || consoleURL != "" {
		message := firstNonEmpty(hint, "飞书权限不足，请完成授权或联系管理员")
		if consoleURL != "" && strings.Contains(strings.ToLower(hint+message), "bot") {
			return ExternalInteractionView{
				Title:           "飞书管理员授权",
				SafeMessage:     message,
				VerificationURL: consoleURL,
				Provider:        "lark-cli",
				Recoveries:      []string{"open_url", "confirm_done", "cancel"},
				RequiresReplay:  true,
			}, true
		}
		if consoleURL != "" {
			return ExternalInteractionView{
				Title:           "飞书授权",
				SafeMessage:     message,
				VerificationURL: consoleURL,
				Provider:        "lark-cli",
				Recoveries:      []string{"open_url", "confirm_done", "cancel"},
				RequiresReplay:  true,
			}, true
		}
	}

	return ExternalInteractionView{}, false
}

func parseBashPayload(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return payload
}

func firstJSONObject(parts ...string) map[string]any {
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if obj := parseBashPayload(part); obj != nil {
			return obj
		}
		// CLI may emit a single JSON line among other text.
		for _, line := range strings.Split(part, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "{") {
				if obj := parseBashPayload(line); obj != nil {
					return obj
				}
			}
		}
	}
	return nil
}

func opaqueHTTPSURL(candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		if !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
			continue
		}
		// Keep the URL opaque: do not re-encode or rebuild.
		return candidate
	}
	return ""
}

func stringField(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if strings.Contains(key, ".") {
		parts := strings.Split(key, ".")
		current := any(payload)
		for _, part := range parts {
			object, ok := current.(map[string]any)
			if !ok {
				return ""
			}
			current = object[part]
		}
		text, _ := current.(string)
		return strings.TrimSpace(text)
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func stringMapField(payload map[string]any, objectKey, fieldKey string) string {
	if payload == nil {
		return ""
	}
	object, _ := payload[objectKey].(map[string]any)
	return stringField(object, fieldKey)
}

func stringSliceField(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	raw, ok := payload[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, _ := item.(string)
		text = strings.TrimSpace(text)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func intField(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	switch value := payload[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	default:
		return 0
	}
}

func sanitizeHint(hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	lower := strings.ToLower(hint)
	banned := []string{"device_code", "access_token", "refresh_token", "client_secret", "authorization:"}
	for _, token := range banned {
		if strings.Contains(lower, token) {
			return "请根据官方授权页完成操作"
		}
	}
	if len(hint) > 240 {
		return hint[:240]
	}
	return hint
}
