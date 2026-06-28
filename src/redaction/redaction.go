package redaction

import "regexp"

const replacement = "[REDACTED]"

type Redactor struct{}

var defaultRedactor = Redactor{}

var (
	privateKeyRE   = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	authHeaderRE   = regexp.MustCompile(`(?im)(\bAuthorization\s*[:=]\s*)[^\r\n]+`)
	apiKeyHeaderRE = regexp.MustCompile(`(?im)(\b(?:x-api-key|api-key)\s*[:=]\s*)[^\s,\r\n]+`)
	bearerTokenRE  = regexp.MustCompile(`(?i)(\bBearer\s+)[A-Za-z0-9._~+/=-]{12,}`)
	urlUserinfoRE  = regexp.MustCompile(`(?i)((?:https?|wss?|ftp|postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp)://[^:\s/@]+:)([^@\s/]+)(@)`)
	queryParamRE   = regexp.MustCompile(`(?i)([?&](?:access_token|refresh_token|id_token|token|api_key|apikey|client_secret|password|passwd|secret|signature|x-amz-signature)=)([^&#\s]+)`)
	jsonFieldRE    = regexp.MustCompile(`(?i)("(?:[A-Za-z0-9_-]+_)?(?:api_key|apikey|access_token|refresh_token|id_token|auth_token|client_secret|password|passwd|secret|token|private_key|authorization|auth|aws_secret_access_key)"\s*:\s*")([^"\\\r\n]*)(")`)
	assignmentRE   = regexp.MustCompile(`(?i)(\b[A-Za-z0-9_.-]*(?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|auth[_-]?token|client[_-]?secret|secret[_-]?key|aws[_-]?secret[_-]?access[_-]?key|password|passwd|token|secret)\b\s*[:=]\s*['"]?)([^'"\s&;,]+)(['"]?)`)
	prefixTokenRE  = regexp.MustCompile(`(^|[^A-Za-z0-9_-])((?:sk-[A-Za-z0-9_-]{10,}|sk_(?:live|test)_[A-Za-z0-9]{10,}|rk_live_[A-Za-z0-9]{10,}|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|gh[ousr]_[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{20,}|AIza[A-Za-z0-9_-]{30,}|hf_[A-Za-z0-9]{20,}|npm_[A-Za-z0-9]{20,}|pypi-[A-Za-z0-9_-]{20,}))(?:([^A-Za-z0-9_-])|$)`)
)

func NewRedactor() Redactor {
	return Redactor{}
}

func RedactString(value string) string {
	return defaultRedactor.RedactString(value)
}

func (Redactor) RedactString(value string) string {
	if value == "" {
		return ""
	}

	value = privateKeyRE.ReplaceAllString(value, replacement)
	value = authHeaderRE.ReplaceAllString(value, "${1}"+replacement)
	value = apiKeyHeaderRE.ReplaceAllString(value, "${1}"+replacement)
	value = bearerTokenRE.ReplaceAllString(value, "${1}"+replacement)
	value = urlUserinfoRE.ReplaceAllString(value, "${1}"+replacement+"${3}")
	value = queryParamRE.ReplaceAllString(value, "${1}"+replacement)
	value = jsonFieldRE.ReplaceAllString(value, "${1}"+replacement+"${3}")
	value = assignmentRE.ReplaceAllString(value, "${1}"+replacement+"${3}")
	value = prefixTokenRE.ReplaceAllString(value, "${1}"+replacement+"${3}")
	return value
}
