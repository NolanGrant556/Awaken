package lspdiagnostics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "Error"
	SeverityWarning Severity = "Warning"
	SeverityInfo    Severity = "Info"
	SeverityHint    Severity = "Hint"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type PublishDiagnostics struct {
	URI         string          `json:"uri"`
	Diagnostics []LSPDiagnostic `json:"diagnostics"`
	Version     *int            `json:"version,omitempty"`
}

type LSPDiagnostic struct {
	Range    Range  `json:"range"`
	Severity *int   `json:"severity,omitempty"`
	Code     any    `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

type Diagnostic struct {
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
	Range    Range    `json:"range"`
	Source   string   `json:"source,omitempty"`
	Code     string   `json:"code,omitempty"`
}

type DiagnosticFile struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func MapLSPSeverity(lspSeverity *int) Severity {
	if lspSeverity == nil {
		return SeverityError
	}

	switch *lspSeverity {
	case 1:
		return SeverityError
	case 2:
		return SeverityWarning
	case 3:
		return SeverityInfo
	case 4:
		return SeverityHint
	default:
		return SeverityError
	}
}

func URIToPath(rawURI string) string {
	if !strings.HasPrefix(rawURI, "file://") {
		return rawURI
	}

	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "file" {
		return rawURI
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		return rawURI
	}
	if parsed.Path == "" {
		return rawURI
	}
	return parsed.Path
}

func FormatDiagnostics(params PublishDiagnostics) []DiagnosticFile {
	diagnostics := make([]Diagnostic, 0, len(params.Diagnostics))
	for _, diag := range params.Diagnostics {
		diagnostics = append(diagnostics, Diagnostic{
			Message:  diag.Message,
			Severity: MapLSPSeverity(diag.Severity),
			Range:    diag.Range,
			Source:   diag.Source,
			Code:     normalizeCode(diag.Code),
		})
	}

	return []DiagnosticFile{{
		URI:         URIToPath(params.URI),
		Diagnostics: diagnostics,
	}}
}

func PublishDiagnosticsFromPayload(payload any) (PublishDiagnostics, bool) {
	switch value := payload.(type) {
	case PublishDiagnostics:
		return validatePublishDiagnostics(value)
	case *PublishDiagnostics:
		if value == nil {
			return PublishDiagnostics{}, false
		}
		return validatePublishDiagnostics(*value)
	case []byte:
		return decodePublishDiagnostics(value)
	case string:
		return decodePublishDiagnostics([]byte(value))
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return PublishDiagnostics{}, false
		}
		return decodePublishDiagnostics(data)
	}
}

func decodePublishDiagnostics(data []byte) (PublishDiagnostics, bool) {
	var params PublishDiagnostics
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&params); err != nil {
		return PublishDiagnostics{}, false
	}
	return validatePublishDiagnostics(params)
}

func validatePublishDiagnostics(params PublishDiagnostics) (PublishDiagnostics, bool) {
	if strings.TrimSpace(params.URI) == "" || params.Diagnostics == nil {
		return PublishDiagnostics{}, false
	}
	return params, true
}

func normalizeCode(code any) string {
	if code == nil {
		return ""
	}

	switch value := code.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case fmt.Stringer:
		return value.String()
	default:
		return fmt.Sprint(value)
	}
}
