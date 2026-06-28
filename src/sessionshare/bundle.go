package sessionshare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const BundleVersion = "awaken.session_share.bundle.v1"

var ErrUnsafePublicBundle = errors.New("sessionshare: public bundle contains unredacted secrets")

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityOrg     Visibility = "org"
	VisibilityPublic  Visibility = "public"
)

type Action string

const (
	ActionCopy   Action = "copy"
	ActionExport Action = "export"
	ActionShare  Action = "share"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

type BuildInput struct {
	SessionID string    `json:"session_id,omitempty"`
	Title     string    `json:"title,omitempty"`
	Messages  []Message `json:"messages"`
}

type Options struct {
	Visibility         Visibility `json:"visibility"`
	Action             Action     `json:"action"`
	IncludeToolOutputs bool       `json:"include_tool_outputs"`
	RedactSecrets      bool       `json:"redact_secrets"`
	RedactPaths        bool       `json:"redact_paths"`
	MaxToolOutputChars int        `json:"max_tool_output_chars"`
	SummaryChars       int        `json:"summary_chars"`
	PathRoots          []string   `json:"path_roots,omitempty"`
}

type Message struct {
	ID         string            `json:"id,omitempty"`
	Role       Role              `json:"role"`
	Content    string            `json:"content,omitempty"`
	ToolName   string            `json:"tool_name,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Bundle struct {
	Version    string          `json:"version"`
	SessionID  string          `json:"session_id,omitempty"`
	Title      string          `json:"title,omitempty"`
	Visibility Visibility      `json:"visibility"`
	Messages   []BundleMessage `json:"messages"`
	Manifest   Manifest        `json:"manifest"`
	Privacy    PrivacyReport   `json:"privacy"`
	Plan       ActionPlan      `json:"plan"`
}

type BundleMessage struct {
	Sequence   int               `json:"sequence"`
	ID         string            `json:"id,omitempty"`
	Role       Role              `json:"role"`
	Content    string            `json:"content,omitempty"`
	ToolName   string            `json:"tool_name,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	ToolOutput *ToolOutput       `json:"tool_output,omitempty"`
}

type ToolOutputMode string

const (
	ToolOutputInline     ToolOutputMode = "inline"
	ToolOutputSummarized ToolOutputMode = "summarized"
	ToolOutputOmitted    ToolOutputMode = "omitted"
)

type ToolOutput struct {
	Mode          ToolOutputMode `json:"mode"`
	Handle        string         `json:"handle,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	ContentHash   string         `json:"content_hash,omitempty"`
	OriginalChars int            `json:"original_chars"`
	IncludedChars int            `json:"included_chars"`
}

type Manifest struct {
	Version       string        `json:"version"`
	ContentHash   string        `json:"content_hash"`
	MessageCounts MessageCounts `json:"message_counts"`
	OmittedCounts OmittedCounts `json:"omitted_counts"`
}

type MessageCounts struct {
	Total                 int            `json:"total"`
	ByRole                map[string]int `json:"by_role"`
	ToolOutputs           int            `json:"tool_outputs"`
	ToolOutputsIncluded   int            `json:"tool_outputs_included"`
	ToolOutputsOmitted    int            `json:"tool_outputs_omitted"`
	ToolOutputsSummarized int            `json:"tool_outputs_summarized"`
}

type OmittedCounts struct {
	ToolOutputs               int `json:"tool_outputs"`
	ToolOutputChars           int `json:"tool_output_chars"`
	SummarizedToolOutputs     int `json:"summarized_tool_outputs"`
	SummarizedToolOutputChars int `json:"summarized_tool_output_chars"`
	Redactions                int `json:"redactions"`
}

type PrivacyReport struct {
	Findings          []PrivacyFinding `json:"findings,omitempty"`
	Warnings          []string         `json:"warnings,omitempty"`
	Blocked           bool             `json:"blocked"`
	BlockReasons      []string         `json:"block_reasons,omitempty"`
	SecretFindings    int              `json:"secret_findings"`
	PathFindings      int              `json:"path_findings"`
	RedactedFindings  int              `json:"redacted_findings"`
	UnredactedSecrets int              `json:"unredacted_secrets"`
	UnredactedPaths   int              `json:"unredacted_paths"`
}

type PrivacyFinding struct {
	Kind         string `json:"kind"`
	MessageIndex int    `json:"message_index"`
	Scope        string `json:"scope"`
	Redacted     bool   `json:"redacted"`
	Severity     string `json:"severity"`
}

type ActionPlan struct {
	Action            Action      `json:"action"`
	Visibility        Visibility  `json:"visibility"`
	Allowed           bool        `json:"allowed"`
	RequiresUpload    bool        `json:"requires_upload"`
	Destination       string      `json:"destination"`
	ContentType       string      `json:"content_type"`
	SuggestedFilename string      `json:"suggested_filename,omitempty"`
	Steps             []string    `json:"steps"`
	BlockReasons      []string    `json:"block_reasons,omitempty"`
	Upload            *UploadPlan `json:"upload,omitempty"`
}

type UploadPlan struct {
	DryRun      bool       `json:"dry_run"`
	Method      string     `json:"method,omitempty"`
	Visibility  Visibility `json:"visibility,omitempty"`
	ContentHash string     `json:"content_hash,omitempty"`
}

func DefaultOptions() Options {
	return Options{
		Visibility:         VisibilityPrivate,
		Action:             ActionExport,
		IncludeToolOutputs: true,
		RedactSecrets:      true,
		RedactPaths:        true,
		MaxToolOutputChars: 4096,
		SummaryChars:       400,
	}
}

func Build(messages []Message, opts Options) (Bundle, error) {
	return BuildBundle(BuildInput{Messages: messages}, opts)
}

func BuildBundle(input BuildInput, opts Options) (Bundle, error) {
	opts = normalizeOptions(opts)

	redactor := newRedactor(opts)
	counts := MessageCounts{ByRole: map[string]int{}}
	omitted := OmittedCounts{}
	bundleMessages := make([]BundleMessage, 0, len(input.Messages))

	for i, msg := range input.Messages {
		messageIndex := i + 1
		role := normalizeRole(msg.Role)
		isTool := role == RoleTool || msg.ToolName != "" || msg.ToolCallID != ""
		originalChars := utf8.RuneCountInString(msg.Content)

		counts.Total++
		counts.ByRole[string(role)]++
		if isTool {
			counts.ToolOutputs++
		}

		bundled := BundleMessage{
			Sequence:   messageIndex,
			ID:         msg.ID,
			Role:       role,
			Content:    msg.Content,
			ToolName:   msg.ToolName,
			ToolCallID: msg.ToolCallID,
			Fields:     redactor.processStringMap(msg.Fields, "fields", messageIndex),
			Metadata:   redactor.processStringMap(msg.Metadata, "metadata", messageIndex),
		}

		if isTool && !opts.IncludeToolOutputs {
			bundled.Content = ""
			bundled.ToolOutput = &ToolOutput{
				Mode:          ToolOutputOmitted,
				Summary:       "Tool output omitted by bundle policy.",
				OriginalChars: originalChars,
			}
			counts.ToolOutputsOmitted++
			omitted.ToolOutputs++
			omitted.ToolOutputChars += originalChars
			bundleMessages = append(bundleMessages, bundled)
			continue
		}

		bundled.Content = redactor.processText(msg.Content, "content", messageIndex)

		if isTool {
			counts.ToolOutputsIncluded++
			includedChars := utf8.RuneCountInString(bundled.Content)
			if opts.MaxToolOutputChars > 0 && originalChars > opts.MaxToolOutputChars {
				summary := summarizeToolOutput(bundled.Content, opts.SummaryChars, originalChars)
				outputHash := hashString(bundled.Content)
				summaryChars := utf8.RuneCountInString(summary)
				bundled.Content = ""
				bundled.ToolOutput = &ToolOutput{
					Mode:          ToolOutputSummarized,
					Handle:        "tool-result:" + outputHash[:16],
					Summary:       summary,
					ContentHash:   outputHash,
					OriginalChars: originalChars,
					IncludedChars: summaryChars,
				}
				counts.ToolOutputsSummarized++
				omitted.SummarizedToolOutputs++
				if originalChars > summaryChars {
					omitted.SummarizedToolOutputChars += originalChars - summaryChars
				}
			} else {
				bundled.ToolOutput = &ToolOutput{
					Mode:          ToolOutputInline,
					OriginalChars: originalChars,
					IncludedChars: includedChars,
				}
			}
		}

		bundleMessages = append(bundleMessages, bundled)
	}

	privacy := redactor.report()
	omitted.Redactions = privacy.RedactedFindings
	privacy = finalizePrivacyReport(privacy, opts.Visibility)

	manifest := Manifest{
		Version:       BundleVersion,
		MessageCounts: counts,
		OmittedCounts: omitted,
	}
	manifest.ContentHash = contentHash(input.SessionID, input.Title, opts.Visibility, bundleMessages, manifest)

	bundle := Bundle{
		Version:    BundleVersion,
		SessionID:  input.SessionID,
		Title:      input.Title,
		Visibility: opts.Visibility,
		Messages:   bundleMessages,
		Manifest:   manifest,
		Privacy:    privacy,
	}
	bundle.Plan = buildActionPlan(opts.Action, opts.Visibility, manifest.ContentHash, privacy)

	if privacy.Blocked {
		return bundle, ErrUnsafePublicBundle
	}
	return bundle, nil
}

func (b Bundle) Bytes() ([]byte, error) {
	return json.MarshalIndent(b, "", "  ")
}

func normalizeOptions(opts Options) Options {
	if opts.Visibility == "" {
		opts.Visibility = VisibilityPrivate
	}
	if opts.Action == "" {
		opts.Action = ActionExport
	}
	if opts.MaxToolOutputChars == 0 {
		opts.MaxToolOutputChars = 4096
	}
	if opts.SummaryChars == 0 {
		opts.SummaryChars = 400
	}
	return opts
}

func normalizeRole(role Role) Role {
	switch role {
	case RoleUser, RoleAssistant, RoleTool, RoleSystem:
		return role
	case "":
		return RoleUser
	default:
		return role
	}
}

func finalizePrivacyReport(report PrivacyReport, visibility Visibility) PrivacyReport {
	if report.UnredactedSecrets > 0 {
		if visibility == VisibilityPublic {
			report.Blocked = true
			report.BlockReasons = append(report.BlockReasons, "public bundles cannot include unredacted secrets")
		} else {
			report.Warnings = append(report.Warnings, fmt.Sprintf("bundle contains %d unredacted secret finding(s)", report.UnredactedSecrets))
		}
	}
	if report.UnredactedPaths > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("bundle contains %d unredacted absolute path finding(s)", report.UnredactedPaths))
	}
	return report
}

func buildActionPlan(action Action, visibility Visibility, contentHash string, privacy PrivacyReport) ActionPlan {
	shortHash := contentHash
	if len(shortHash) > 12 {
		shortHash = shortHash[:12]
	}

	plan := ActionPlan{
		Action:      action,
		Visibility:  visibility,
		Allowed:     !privacy.Blocked,
		ContentType: "application/vnd.awaken.session-share+json",
	}

	switch action {
	case ActionCopy:
		plan.Destination = "clipboard"
		plan.Steps = []string{"serialize bundle", "copy bundle JSON to clipboard"}
	case ActionShare:
		plan.RequiresUpload = true
		plan.Destination = "remote-share"
		plan.SuggestedFilename = "awaken-session-" + shortHash + ".json"
		plan.Steps = []string{"serialize bundle", "request upload target", "upload bundle JSON", "return share URL"}
		plan.Upload = &UploadPlan{
			DryRun:      true,
			Method:      "POST",
			Visibility:  visibility,
			ContentHash: contentHash,
		}
	default:
		plan.Action = ActionExport
		plan.Destination = "file"
		plan.SuggestedFilename = "awaken-session-" + shortHash + ".json"
		plan.Steps = []string{"serialize bundle", "write bundle JSON to selected file"}
	}

	if privacy.Blocked {
		plan.Allowed = false
		plan.BlockReasons = append([]string(nil), privacy.BlockReasons...)
	}
	return plan
}

func contentHash(sessionID, title string, visibility Visibility, messages []BundleMessage, manifest Manifest) string {
	manifest.ContentHash = ""
	payload := struct {
		Version    string          `json:"version"`
		SessionID  string          `json:"session_id,omitempty"`
		Title      string          `json:"title,omitempty"`
		Visibility Visibility      `json:"visibility"`
		Messages   []BundleMessage `json:"messages"`
		Manifest   Manifest        `json:"manifest"`
	}{
		Version:    BundleVersion,
		SessionID:  sessionID,
		Title:      title,
		Visibility: visibility,
		Messages:   messages,
		Manifest:   manifest,
	}
	data, _ := json.Marshal(payload)
	return hashBytes(data)
}

func hashString(value string) string {
	return hashBytes([]byte(value))
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func summarizeToolOutput(content string, maxChars int, originalChars int) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Sprintf("Original tool output was %d characters; no preview content.", originalChars)
	}
	if maxChars <= 0 {
		maxChars = 400
	}
	if utf8.RuneCountInString(trimmed) <= maxChars {
		return fmt.Sprintf("Original tool output was %d characters. Preview:\n%s", originalChars, trimmed)
	}

	runes := []rune(trimmed)
	headChars := maxChars / 2
	tailChars := maxChars - headChars
	if headChars < 1 {
		headChars = 1
	}
	if tailChars < 1 {
		tailChars = 1
	}

	excerpt := string(runes[:headChars]) + "\n...[summary omitted]...\n" + string(runes[len(runes)-tailChars:])
	return fmt.Sprintf("Original tool output was %d characters. Preview:\n%s", originalChars, excerpt)
}
