package edit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/filemutation"
	"awaken/src/tools/internal/textfile"
)

const ToolName = "Edit"

type Tool struct{}

type params struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (t *Tool) Name() string { return ToolName }
func (t *Tool) Description() string {
	return "Atomically replace an exact string in one previously Read text file. The host rejects stale files; replace_all is required when the old string occurs more than once."
}
func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "file_path", Description: "Existing text file. Relative paths resolve from the primary workspace folder; explicit absolute paths are honored unless they match a protected mutation shape.", Type: "string", Required: true},
		{Name: "old_string", Description: "Exact text to replace.", Type: "string", Required: true},
		{Name: "new_string", Description: "Replacement text.", Type: "string", Required: true},
		{Name: "replace_all", Description: "Replace every match. Without it, old_string must be unique.", Type: "boolean"},
	}
}
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Edit", Description: t.Description(),
		Purpose:      "Perform one host-versioned, atomic text replacement while preserving encoding, BOM, newline style, and file mode.",
		WhenToUse:    []string{"After Read has established the current file snapshot", "For precise local replacement in one existing text file"},
		WhenNotToUse: []string{"Use Write for a new file or intentional whole-file replacement", "Use trash_file for deletion"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectUpdate, Resources: []string{"one text file"}, Description: "Atomically replaces text in a writable file.", Reversibility: tool.ReversibilityConditional}},
		Examples:     []tool.ToolExample{{Scenario: "Rename one heading", Input: map[string]any{"file_path": "README.md", "old_string": "Old", "new_string": "New"}, Outcome: "Returns match count, structured diff, and refreshed file summary."}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "file_not_read", Cause: "The current thread has not Read this existing file", Retryable: true, Recovery: []string{"read_the_file"}},
			{Code: "file_changed_since_read", Cause: "The file changed outside this thread after Read", Retryable: true, Recovery: []string{"read_the_file_again"}},
			{Code: "edit_match_not_unique", Cause: "Multiple matches require replace_all", Retryable: true},
			{Code: "unsupported_text_encoding", Cause: "The file is binary or uses an unknown encoding", Retryable: false},
		},
		OutputSchema: tool.ToolParam{Type: "object", Description: "Refreshed file snapshot, replacement count, and structured diff details.", AdditionalProperties: tool.BoolPtr(true)},
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
	return tool.ToolActivity{Stage: "started", Title: "Editing file", Detail: p.FilePath}
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
		return fail(started, "edit_invalid_input", err.Error(), tool.ErrorCategoryValidation, false), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	path, err := permissionContext.ResolveMutationPathChecked(p.FilePath)
	if err != nil {
		return tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied).WithTiming(started, time.Now().UTC()), nil
	}
	state, ok := tool.ToolSessionStateFromContext(ctx)
	if !ok {
		return fail(started, "file_not_read", "Edit requires this thread to Read the file first.", tool.ErrorCategoryDomain, true), nil
	}
	expected, ok := state.File(path)
	if !ok {
		return fail(started, "file_not_read", "Edit requires this thread to Read the file first.", tool.ErrorCategoryDomain, true), nil
	}

	var outcome tool.ExecutionOutcome
	err = state.WithPathLockContext(ctx, path, func(canonical string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		beforeBytes, info, _, loadErr := filemutation.LoadExisting(canonical, expected)
		if loadErr != nil {
			if loadErr.Error() == "file_changed_since_read" {
				outcome = fail(started, "file_changed_since_read", "The file changed after Read; Read it again before editing.", tool.ErrorCategoryDomain, true)
				return nil
			}
			outcome = fail(started, "edit_read_failed", loadErr.Error(), tool.ErrorCategoryTool, true)
			return nil
		}
		document, decodeErr := textfile.Decode(beforeBytes)
		if decodeErr != nil {
			outcome = fail(started, "unsupported_text_encoding", decodeErr.Error(), tool.ErrorCategoryUnsupported, false)
			return nil
		}
		oldString := textfile.AdaptNewlines(p.OldString, document.Newline)
		newString := textfile.AdaptNewlines(p.NewString, document.Newline)
		if oldString == newString {
			outcome = fail(started, "edit_invalid_input", "old_string and new_string must differ", tool.ErrorCategoryValidation, false)
			return nil
		}
		matchCount := strings.Count(document.Text, oldString)
		switch {
		case matchCount == 0:
			outcome = fail(started, "edit_match_not_found", "old_string was not found in the current file", tool.ErrorCategoryDomain, true)
			return nil
		case matchCount > 1 && !p.ReplaceAll:
			outcome = fail(started, "edit_match_not_unique", fmt.Sprintf("old_string occurs %d times; provide more context or set replace_all", matchCount), tool.ErrorCategoryDomain, true)
			outcome.Details = map[string]any{"match_count": matchCount}
			return nil
		}
		afterText := strings.Replace(document.Text, oldString, newString, 1)
		replacements := 1
		if p.ReplaceAll {
			afterText = strings.ReplaceAll(document.Text, oldString, newString)
			replacements = matchCount
		}
		afterBytes, encodeErr := textfile.Encode(afterText, document.Encoding)
		if encodeErr != nil {
			outcome = fail(started, "unsupported_text_encoding", encodeErr.Error(), tool.ErrorCategoryUnsupported, false)
			return nil
		}
		if writeErr := textfile.WriteAtomic(canonical, afterBytes, info.Mode()); writeErr != nil {
			outcome = fail(started, "edit_write_failed", writeErr.Error(), tool.ErrorCategoryPersistence, true)
			return nil
		}
		refreshed, refreshErr := filemutation.RefreshedSnapshot(canonical, expected.MimeType)
		if refreshErr != nil {
			return refreshErr
		}
		state.RecordFile(refreshed)
		diff := structuredDiff(canonical, document.Text, afterText, expected.SHA256, refreshed.SHA256, replacements)
		payload, marshalErr := json.MarshalIndent(diff, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		outcome = tool.SucceededOutcome(string(payload))
		outcome.AffectedPaths = []string{relativePath(canonical, permissionContext.WorkspaceRootOrCWD())}
		outcome.PatchSummary = fmt.Sprintf("updated 1 file with %d replacement(s)", replacements)
		outcome.Summary = fmt.Sprintf("Edited %s with %d replacement(s).", filepath.Base(canonical), replacements)
		outcome.Details = map[string]any{"file": filemutation.Details(refreshed), "replacement_count": replacements, "diff": diff}
		outcome.Changes = []tool.ResourceChange{{Kind: "file", Action: "updated", Resource: outcome.AffectedPaths[0], Summary: outcome.Summary, Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional}}
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
		return params{}, fmt.Errorf("invalid Edit input: %w", err)
	}
	if strings.TrimSpace(p.FilePath) == "" {
		return params{}, fmt.Errorf("file_path is required")
	}
	if p.OldString == "" {
		return params{}, fmt.Errorf("old_string must not be empty")
	}
	return p, nil
}

func structuredDiff(path, before, after, beforeSHA, afterSHA string, replacements int) map[string]any {
	previewBefore := before
	previewAfter := after
	const limit = 24000
	if len(previewBefore) > limit {
		previewBefore = previewBefore[:limit] + "\n[truncated]"
	}
	if len(previewAfter) > limit {
		previewAfter = previewAfter[:limit] + "\n[truncated]"
	}
	return map[string]any{
		"file_path": path, "before_sha256": beforeSHA, "after_sha256": afterSHA,
		"replacement_count": replacements,
		"hunks":             []map[string]any{{"old_text": previewBefore, "new_text": previewAfter}},
	}
}

func relativePath(path, root string) string {
	if relative, err := filepath.Rel(root, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}

func fail(started time.Time, code, message string, category tool.ErrorCategory, retryable bool) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, code, category, retryable)
	if retryable {
		outcome.RecoveryOptions = []string{"read_the_file_again", "retry_with_current_content"}
	}
	return outcome.WithTiming(started, time.Now().UTC())
}

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
