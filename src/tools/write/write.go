package write

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/tool"
	"awaken/src/tools/internal/filemutation"
	"awaken/src/tools/internal/textfile"
)

const ToolName = "Write"

type Tool struct{}
type params struct {
	FilePath string  `json:"file_path"`
	Content  *string `json:"content"`
}

func (t *Tool) Name() string { return ToolName }
func (t *Tool) Description() string {
	return "Atomically create or replace one text file. New files are UTF-8; replacing an existing file requires a current Read and preserves its encoding, BOM, newline style, and permissions. Write never publishes an Artifact automatically."
}
func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "file_path", Description: "Target text file within writable roots.", Type: "string", Required: true},
		{Name: "content", Description: "Complete text content. Empty content is allowed.", Type: "string", Required: true},
	}
}
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Write", Description: t.Description(),
		Purpose:      "Atomically create or replace one text file under host-owned stale-read protection without publishing an Artifact.",
		WhenToUse:    []string{"Creating a new UTF-8 source, Markdown, JSON, CSV, or text file", "Replacing the complete content of an existing file after Read"},
		WhenNotToUse: []string{"Use Edit for a precise replacement", "Call artifact_publish separately when the file is a user deliverable", "Use trash_file for deletion"},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectCreate, Resources: []string{"one text file"}, Description: "Creates a new UTF-8 file.", Reversibility: tool.ReversibilityConditional},
			{Kind: tool.SideEffectUpdate, Resources: []string{"one text file"}, Description: "Atomically replaces a previously read text file.", Reversibility: tool.ReversibilityConditional},
		},
		Examples: []tool.ToolExample{{Scenario: "Create a report", Input: map[string]any{"file_path": "report.md", "content": "# Report\n"}, Outcome: "Creates UTF-8 text and returns its file summary."}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "file_not_read", Cause: "An existing target was not Read by this thread", Retryable: true},
			{Code: "file_changed_since_read", Cause: "The target changed after Read", Retryable: true},
			{Code: "unsupported_text_encoding", Cause: "The existing target is binary or has unknown encoding", Retryable: false},
		},
		OutputSchema: tool.ToolParam{Type: "object", Description: "Created or updated file snapshot, encoding, and newline details.", AdditionalProperties: tool.BoolPtr(true)},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, CapabilityClass: tool.CapabilityArtifactWrite, TrustedOrigin: tool.TrustedOriginBuiltin, FileBridgeScope: tool.FileBridgeTaskScopeWrite},
	})
}

func (t *Tool) CheckPermission(_ context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	p, err := parseParams(input)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if _, err := permissionContext.ResolveMutationPathChecked(p.FilePath); err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermission()
}

func (t *Tool) DescribeActivity(input string) tool.ToolActivity {
	p, _ := parseParams(input)
	return tool.ToolActivity{Stage: "started", Title: "Writing file", Detail: p.FilePath}
}

func (t *Tool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return outcome.Content, errors.New(outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *Tool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	p, err := parseParams(input)
	if err != nil {
		return failure(started, "write_invalid_input", err.Error(), tool.ErrorCategoryValidation, false), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	path, err := permissionContext.ResolveMutationPathChecked(p.FilePath)
	if err != nil {
		return tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied).WithTiming(started, time.Now().UTC()), nil
	}
	state, hasState := tool.ToolSessionStateFromContext(ctx)
	if !hasState {
		state = tool.NewToolSessionState(permissionContext.WorkspaceRootOrCWD())
	}
	var outcome tool.ExecutionOutcome
	err = state.WithPathLock(path, func(canonical string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, statErr := os.Lstat(canonical)
		action := "created"
		encoding := textfile.EncodingUTF8
		newline := "\n"
		mode := os.FileMode(0o644)
		mimeType := "text/plain; charset=utf-8"
		if statErr == nil {
			action = "overwritten"
			expected, read := state.File(canonical)
			if !read {
				outcome = failure(started, "file_not_read", "Write requires this thread to Read an existing file before replacing it.", tool.ErrorCategoryDomain, true)
				return nil
			}
			beforeBytes, existingInfo, _, loadErr := filemutation.LoadExisting(canonical, expected)
			if loadErr != nil {
				if loadErr.Error() == "file_changed_since_read" {
					outcome = failure(started, "file_changed_since_read", "The file changed after Read; Read it again before writing.", tool.ErrorCategoryDomain, true)
					return nil
				}
				outcome = failure(started, "write_read_failed", loadErr.Error(), tool.ErrorCategoryTool, true)
				return nil
			}
			document, decodeErr := textfile.Decode(beforeBytes)
			if decodeErr != nil {
				outcome = failure(started, "unsupported_text_encoding", decodeErr.Error(), tool.ErrorCategoryUnsupported, false)
				return nil
			}
			encoding, newline, mode, mimeType = document.Encoding, document.Newline, existingInfo.Mode(), expected.MimeType
		} else if !os.IsNotExist(statErr) {
			outcome = failure(started, "write_stat_failed", statErr.Error(), tool.ErrorCategoryTool, true)
			return nil
		} else {
			// Re-check after acquiring the path lock closes the create/replace race.
			if info != nil {
				outcome = failure(started, "write_target_invalid", "target is not a regular file", tool.ErrorCategoryValidation, false)
				return nil
			}
		}
		content := textfile.AdaptNewlines(*p.Content, newline)
		encoded, encodeErr := textfile.Encode(content, encoding)
		if encodeErr != nil {
			outcome = failure(started, "unsupported_text_encoding", encodeErr.Error(), tool.ErrorCategoryUnsupported, false)
			return nil
		}
		if writeErr := textfile.WriteAtomic(canonical, encoded, mode); writeErr != nil {
			outcome = failure(started, "write_failed", writeErr.Error(), tool.ErrorCategoryPersistence, true)
			return nil
		}
		refreshed, refreshErr := filemutation.RefreshedSnapshot(canonical, mimeType)
		if refreshErr != nil {
			return refreshErr
		}
		state.RecordFile(refreshed)
		relative := relativePath(canonical, permissionContext.WorkspaceRootOrCWD())
		result := map[string]any{"file_path": relative, "action": action, "bytes": refreshed.Size, "sha256": refreshed.SHA256, "encoding": encoding, "newline": newlineLabel(newline)}
		payload, marshalErr := json.MarshalIndent(result, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		outcome = tool.SucceededOutcome(string(payload))
		outcome.AffectedPaths = []string{relative}
		outcome.PatchSummary = fmt.Sprintf("%s 1 file", action)
		verb := "Created"
		if action == "overwritten" {
			verb = "Overwrote"
		}
		outcome.Summary = fmt.Sprintf("%s %s.", verb, relative)
		outcome.Details = map[string]any{"file": filemutation.Details(refreshed), "action": action, "encoding": encoding, "newline": newlineLabel(newline), "artifact_published": false}
		outcome.Changes = []tool.ResourceChange{{Kind: "file", Action: action, Resource: relative, Summary: outcome.Summary, Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional}}
		return nil
	})
	if err != nil {
		return tool.ExecutionOutcome{}, err
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func parseParams(input string) (params, error) {
	var p params
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(input)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return params{}, fmt.Errorf("invalid Write input: %w", err)
	}
	if strings.TrimSpace(p.FilePath) == "" {
		return params{}, fmt.Errorf("file_path is required")
	}
	if p.Content == nil {
		return params{}, fmt.Errorf("content is required")
	}
	if !utf8.ValidString(*p.Content) {
		return params{}, fmt.Errorf("content must be valid UTF-8")
	}
	return p, nil
}

func relativePath(path, root string) string {
	if relative, err := filepath.Rel(root, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}

func newlineLabel(newline string) string {
	switch newline {
	case "\r\n":
		return "crlf"
	case "\r":
		return "cr"
	default:
		return "lf"
	}
}

func failure(started time.Time, code, message string, category tool.ErrorCategory, retryable bool) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, code, category, retryable)
	if retryable {
		outcome.RecoveryOptions = []string{"read_the_file_again", "retry_with_current_content"}
	}
	return outcome.WithTiming(started, time.Now().UTC())
}

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
