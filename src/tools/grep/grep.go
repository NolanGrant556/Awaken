package grep

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/rgexec"
)

const (
	ToolName         = "Grep"
	defaultHeadLimit = 100
	maxHeadLimit     = 2000
)

type Tool struct{}
type params struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	Type       string `json:"type,omitempty"`
	OutputMode string `json:"output_mode,omitempty"`
	After      int    `json:"-A,omitempty"`
	Before     int    `json:"-B,omitempty"`
	Context    int    `json:"-C,omitempty"`
	LineNumber *bool  `json:"-n,omitempty"`
	IgnoreCase bool   `json:"-i,omitempty"`
	HeadLimit  int    `json:"head_limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	Multiline  bool   `json:"multiline,omitempty"`
}

type contentRecord struct {
	Path  string
	Line  int
	Text  string
	Match bool
	Order int
}

func (t *Tool) Name() string { return ToolName }
func (t *Tool) Description() string {
	return "Search file contents with the run-pinned ripgrep executable (hidden and ignored files included). Supports content/files_with_matches/count modes, contexts, line numbers, case folding, offsets, limits, and multiline regex."
}
func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "pattern", Description: "ripgrep regular expression.", Type: "string", Required: true},
		{Name: "path", Description: "Readable file or directory. Defaults to thread cwd.", Type: "string"},
		{Name: "glob", Description: "Additional file glob.", Type: "string"},
		{Name: "type", Description: "ripgrep file type, for example go or ts.", Type: "string"},
		{Name: "output_mode", Description: "content (default), files_with_matches, or count.", Type: "string"},
		{Name: "-A", Description: "Lines after each match.", Type: "integer"},
		{Name: "-B", Description: "Lines before each match.", Type: "integer"},
		{Name: "-C", Description: "Lines before and after each match.", Type: "integer"},
		{Name: "-n", Description: "Include line numbers; defaults true for content.", Type: "boolean"},
		{Name: "-i", Description: "Case-insensitive search.", Type: "boolean"},
		{Name: "head_limit", Description: "Maximum returned rows, default 100 and maximum 2000.", Type: "integer"},
		{Name: "offset", Description: "Skip this many stable result rows.", Type: "integer"},
		{Name: "multiline", Description: "Enable multiline matching.", Type: "boolean"},
	}
}
func (t *Tool) IsReadOnly() bool { return true }
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Grep", Description: t.Description(), Purpose: "Perform real ripgrep searches with stable host-side projection and explicit truncation.",
		WhenToUse:    []string{"Finding symbols, text, regex matches, or candidate files", "Counting matches or listing files that contain matches"},
		WhenNotToUse: []string{"Use Glob for path-only patterns", "Use Read for a known file"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"file contents"}, Description: "Reads files through rg without mutation.", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:     []tool.ToolExample{{Scenario: "Find Go declarations", Input: map[string]any{"pattern": "^func ", "path": "src", "type": "go", "output_mode": "content", "head_limit": 100}, Outcome: "Returns stable path/line/content rows and total match count."}},
		FailureModes: []tool.ToolFailureMode{{Code: tool.ErrorCodeNeedsRuntime, Cause: "The pinned run has no rg executable", Retryable: true}, {Code: "grep_invalid_pattern", Cause: "rg rejected the regex", Retryable: false}},
		OutputSchema: tool.ToolParam{Type: "object", Description: "Ripgrep rows, counts, offsets, truncation, and runtime snapshot details.", AdditionalProperties: tool.BoolPtr(true)},
		Annotations:  tool.ToolAnnotations{ReadOnly: true, CapabilityClass: tool.CapabilityArtifactRead, TrustedOrigin: tool.TrustedOriginBuiltin},
	})
}

func (t *Tool) CheckPermission(_ context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	p, err := parseParams(input)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if strings.TrimSpace(p.Path) != "" {
		if _, err := permissionContext.ResolveReadablePathChecked(p.Path); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermission()
}

func (t *Tool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return outcome.Content, fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *Tool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	p, err := parseParams(input)
	if err != nil {
		return tool.FailedOutcome(err.Error(), "grep_invalid_input", tool.ErrorCategoryValidation, false).WithTiming(started, time.Now().UTC()), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	state, _ := tool.ToolSessionStateFromContext(ctx)
	root := permissionContext.WorkspaceRootOrCWD()
	if state != nil {
		root = state.CWD(root)
	}
	root, err = permissionContext.ResolveReadablePathChecked(root)
	if err != nil {
		return tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied).WithTiming(started, time.Now().UTC()), nil
	}
	target := "."
	if strings.TrimSpace(p.Path) != "" {
		target, err = permissionContext.ResolveReadablePathChecked(p.Path)
		if err != nil {
			return tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied).WithTiming(started, time.Now().UTC()), nil
		}
	}
	runtime, err := rgexec.Resolve(ctx)
	if err != nil {
		return needsRuntime(started, err.Error()), nil
	}
	if p.OutputMode == "content" {
		return executeContent(ctx, started, p, root, target, runtime), nil
	}
	return executeSummaryMode(ctx, started, p, root, target, runtime), nil
}

func baseArgs(p params) []string {
	args := []string{"--hidden", "--no-ignore", "--color", "never"}
	if p.Glob != "" {
		args = append(args, "--glob", p.Glob)
	}
	if p.Type != "" {
		args = append(args, "--type", p.Type)
	}
	if p.IgnoreCase {
		args = append(args, "--ignore-case")
	}
	if p.Multiline {
		args = append(args, "--multiline")
	}
	return args
}

func executeContent(ctx context.Context, started time.Time, p params, root, target string, runtime rgexec.Runtime) tool.ExecutionOutcome {
	args := append(baseArgs(p), "--json")
	if p.Context > 0 {
		args = append(args, "-C", strconv.Itoa(p.Context))
	} else {
		if p.Before > 0 {
			args = append(args, "-B", strconv.Itoa(p.Before))
		}
		if p.After > 0 {
			args = append(args, "-A", strconv.Itoa(p.After))
		}
	}
	args = append(args, "--", p.Pattern, target)
	command := runtime.Command(ctx, root, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return tool.FailedOutcome(err.Error(), "grep_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return tool.FailedOutcome(err.Error(), "grep_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
	}
	var records []contentRecord
	matchCount := 0
	order := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
			} `json:"data"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil || (event.Type != "match" && event.Type != "context") {
			continue
		}
		if event.Type == "match" {
			matchCount++
		}
		path := stableDisplayPath(event.Data.Path.Text, root)
		for index, line := range strings.Split(strings.TrimSuffix(event.Data.Lines.Text, "\n"), "\n") {
			records = append(records, contentRecord{Path: path, Line: event.Data.LineNumber + index, Text: strings.TrimSuffix(line, "\r"), Match: event.Type == "match", Order: order})
			order++
		}
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	if scanErr != nil {
		return tool.FailedOutcome(scanErr.Error(), "grep_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
	}
	if waitErr != nil && command.ProcessState != nil && command.ProcessState.ExitCode() != 1 {
		return tool.FailedOutcome(strings.TrimSpace(stderr.String()), "grep_invalid_pattern", tool.ErrorCategoryValidation, false).WithTiming(started, time.Now().UTC())
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Path != records[j].Path {
			return records[i].Path < records[j].Path
		}
		if records[i].Line != records[j].Line {
			return records[i].Line < records[j].Line
		}
		return records[i].Order < records[j].Order
	})
	return grepOutcome(started, p, root, records, matchCount, runtime.SnapshotID)
}

func executeSummaryMode(ctx context.Context, started time.Time, p params, root, target string, runtime rgexec.Runtime) tool.ExecutionOutcome {
	args := baseArgs(p)
	if p.OutputMode == "files_with_matches" {
		args = append(args, "--files-with-matches")
	} else {
		args = append(args, "--count-matches")
	}
	args = append(args, "--", p.Pattern, target)
	command := runtime.Command(ctx, root, args...)
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*os.PathError); ok {
			return tool.FailedOutcome(exit.Error(), "grep_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
		}
		if command.ProcessState == nil || command.ProcessState.ExitCode() != 1 {
			return tool.FailedOutcome(err.Error(), "grep_invalid_pattern", tool.ErrorCategoryValidation, false).WithTiming(started, time.Now().UTC())
		}
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	for i := range lines {
		lines[i] = stableDisplayPath(lines[i], root)
	}
	sort.Strings(lines)
	total := len(lines)
	start, end := pageBounds(total, p.Offset, p.HeadLimit)
	selected := append([]string(nil), lines[start:end]...)
	content := strings.Join(selected, "\n")
	if content == "" {
		content = "No matches found."
	}
	outcome := tool.SucceededOutcome(content).WithTiming(started, time.Now().UTC())
	outcome.Truncated = end < total
	outcome.Summary = fmt.Sprintf("Grep returned %d of %d %s row(s).", len(selected), total, p.OutputMode)
	outcome.Details = map[string]any{"output_mode": p.OutputMode, "rows": selected, "offset": p.Offset, "head_limit": p.HeadLimit, "total": total, "truncated": outcome.Truncated, "environment_snapshot_id": runtime.SnapshotID}
	return outcome
}

func grepOutcome(started time.Time, p params, root string, records []contentRecord, matchCount int, snapshotID string) tool.ExecutionOutcome {
	start, end := pageBounds(len(records), p.Offset, p.HeadLimit)
	includeLine := p.LineNumber == nil || *p.LineNumber
	rows := make([]string, 0, end-start)
	for _, record := range records[start:end] {
		separator := ":"
		if !record.Match {
			separator = "-"
		}
		if includeLine {
			rows = append(rows, fmt.Sprintf("%s%s%d%s%s", record.Path, separator, record.Line, separator, record.Text))
		} else {
			rows = append(rows, fmt.Sprintf("%s%s%s", record.Path, separator, record.Text))
		}
	}
	content := strings.Join(rows, "\n")
	if content == "" {
		content = "No matches found."
	}
	outcome := tool.SucceededOutcome(content).WithTiming(started, time.Now().UTC())
	outcome.Truncated = end < len(records)
	outcome.Summary = fmt.Sprintf("Grep found %d match event(s); returned %d row(s).", matchCount, len(rows))
	outcome.Details = map[string]any{"output_mode": "content", "path": root, "match_count": matchCount, "returned_rows": len(rows), "total_rows": len(records), "offset": p.Offset, "head_limit": p.HeadLimit, "truncated": outcome.Truncated, "environment_snapshot_id": snapshotID}
	return outcome
}

func parseParams(input string) (params, error) {
	var p params
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(input)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return params{}, fmt.Errorf("invalid Grep input: %w", err)
	}
	if p.Pattern == "" {
		return params{}, fmt.Errorf("pattern is required")
	}
	if p.OutputMode == "" {
		p.OutputMode = "content"
	}
	if p.OutputMode != "content" && p.OutputMode != "files_with_matches" && p.OutputMode != "count" {
		return params{}, fmt.Errorf("output_mode must be content, files_with_matches, or count")
	}
	if p.After < 0 || p.Before < 0 || p.Context < 0 || p.Offset < 0 {
		return params{}, fmt.Errorf("context and offset values must be non-negative")
	}
	if p.HeadLimit == 0 {
		p.HeadLimit = defaultHeadLimit
	}
	if p.HeadLimit < 1 || p.HeadLimit > maxHeadLimit {
		return params{}, fmt.Errorf("head_limit must be between 1 and %d", maxHeadLimit)
	}
	return p, nil
}

func pageBounds(total, offset, limit int) (int, int) {
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return start, end
}

func stableDisplayPath(path, root string) string {
	path = strings.TrimSpace(path)
	if colon := strings.LastIndex(path, ":"); colon > 1 {
		if _, err := strconv.Atoi(path[colon+1:]); err == nil {
			path = path[:colon] + ":" + path[colon+1:]
			return filepath.ToSlash(path)
		}
	}
	if filepath.IsAbs(path) {
		if relative, err := filepath.Rel(root, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(relative)
		}
	}
	return filepath.ToSlash(strings.TrimPrefix(path, "."+string(filepath.Separator)))
}

func needsRuntime(started time.Time, message string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, tool.ErrorCodeNeedsRuntime, tool.ErrorCategoryRuntime, true)
	outcome.Status = tool.OutcomeNeedsRuntime
	outcome.RecoveryOptions = []string{"install_ripgrep", "refresh_run_environment"}
	return outcome.WithTiming(started, time.Now().UTC())
}

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
