package toolsummary

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type toolCategory string

const (
	categoryOther  toolCategory = "other"
	categorySearch toolCategory = "search"
	categoryRead   toolCategory = "read"
	categoryList   toolCategory = "list"
	categoryEdit   toolCategory = "edit"
	categoryShell  toolCategory = "shell"
	categoryWeb    toolCategory = "web"
	categoryCalc   toolCategory = "calc"
)

type toolDescription struct {
	category     toolCategory
	label        string
	failureLabel string
	action       string
	noun         string
	isTests      bool
}

func (s DeterministicSummarizer) Summarize(batch Batch) Summary {
	opts := batch.Options.normalized()
	tools := batch.Tools
	if len(tools) == 0 {
		return Summary{Status: StatusUnknown}
	}

	errorCount, partialCount := 0, 0
	descriptions := make([]toolDescription, 0, len(tools))
	for _, tool := range tools {
		status := normalizedStatus(tool)
		if status == StatusError || status == StatusCanceled {
			errorCount++
		}
		if status == StatusPartial {
			partialCount++
		}
		descriptions = append(descriptions, describeTool(tool, opts))
	}

	status := StatusSuccess
	if errorCount == len(tools) {
		status = StatusError
	} else if errorCount > 0 || partialCount > 0 {
		status = StatusPartial
	}

	label := ""
	if len(tools) == 1 {
		label = descriptions[0].label
		switch normalizedStatus(tools[0]) {
		case StatusError, StatusCanceled:
			label = descriptions[0].failureLabel
		case StatusPartial:
			label = "Partially " + lowerFirst(descriptions[0].label)
		}
	} else {
		label = summarizeMultiple(descriptions, errorCount, partialCount)
	}
	if !opts.DisableRedaction {
		label = redactString(label)
	}

	return Summary{
		Label:        truncate(label, opts.MaxLabelChars),
		Status:       status,
		ToolCount:    len(tools),
		ErrorCount:   errorCount,
		PartialCount: partialCount,
	}
}

func normalizedStatus(tool ToolUse) ToolStatus {
	switch strings.ToLower(strings.TrimSpace(string(tool.Status))) {
	case string(StatusError), "failed", "failure":
		return StatusError
	case string(StatusPartial), "partial_failure", "partial-failure":
		return StatusPartial
	case string(StatusCanceled), "cancelled":
		return StatusCanceled
	case string(StatusSuccess), "ok", "completed", "":
		if strings.TrimSpace(tool.Error) != "" {
			return StatusError
		}
		return StatusSuccess
	default:
		if strings.TrimSpace(tool.Error) != "" {
			return StatusError
		}
		return StatusSuccess
	}
}

func summarizeMultiple(descriptions []toolDescription, errorCount int, partialCount int) string {
	category := descriptions[0].category
	sameCategory := true
	for _, desc := range descriptions[1:] {
		if desc.category != category {
			sameCategory = false
			break
		}
	}

	base := fmt.Sprintf("Used %d tools", len(descriptions))
	if sameCategory {
		base = aggregateCategory(descriptions)
	}

	if errorCount == len(descriptions) {
		return aggregateFailure(descriptions)
	}
	if errorCount > 0 {
		return fmt.Sprintf("%s with %d %s", base, errorCount, pluralize("error", errorCount))
	}
	if partialCount > 0 {
		return "Partially " + lowerFirst(base)
	}
	return base
}

func aggregateCategory(descriptions []toolDescription) string {
	count := len(descriptions)
	switch descriptions[0].category {
	case categorySearch:
		return fmt.Sprintf("Searched %d %s", count, pluralize("location", count))
	case categoryRead:
		return fmt.Sprintf("Read %d %s", count, pluralize("file", count))
	case categoryList:
		return fmt.Sprintf("Listed %d %s", count, pluralize("location", count))
	case categoryEdit:
		action := descriptions[0].action
		sameAction := true
		for _, desc := range descriptions[1:] {
			if desc.action != action {
				sameAction = false
				break
			}
		}
		if sameAction && action != "" {
			return fmt.Sprintf("%s %d %s", title(action), count, pluralize("file", count))
		}
		return fmt.Sprintf("Changed %d %s", count, pluralize("file", count))
	case categoryShell:
		allTests, anyFailing := true, false
		for _, desc := range descriptions {
			allTests = allTests && desc.isTests
			anyFailing = anyFailing || desc.label == "Ran failing tests"
		}
		if allTests && anyFailing {
			return "Ran failing tests"
		}
		if allTests {
			return "Ran tests"
		}
		return fmt.Sprintf("Ran %d %s", count, pluralize("command", count))
	case categoryWeb:
		return "Searched web"
	case categoryCalc:
		return fmt.Sprintf("Calculated %d %s", count, pluralize("result", count))
	default:
		return fmt.Sprintf("Used %d tools", count)
	}
}

func aggregateFailure(descriptions []toolDescription) string {
	count := len(descriptions)
	category := descriptions[0].category
	sameCategory := true
	for _, desc := range descriptions[1:] {
		if desc.category != category {
			sameCategory = false
			break
		}
	}
	if !sameCategory {
		return fmt.Sprintf("Failed %d tools", count)
	}
	switch category {
	case categorySearch:
		return fmt.Sprintf("Failed %d searches", count)
	case categoryRead:
		return fmt.Sprintf("Failed to read %d %s", count, pluralize("file", count))
	case categoryList:
		return fmt.Sprintf("Failed to list %d %s", count, pluralize("location", count))
	case categoryEdit:
		return fmt.Sprintf("Failed to change %d %s", count, pluralize("file", count))
	case categoryShell:
		allTests := true
		for _, desc := range descriptions {
			allTests = allTests && desc.isTests
		}
		if allTests {
			return "Ran failing tests"
		}
		return fmt.Sprintf("Failed %d %s", count, pluralize("command", count))
	default:
		return fmt.Sprintf("Failed %d tools", count)
	}
}

func describeTool(tool ToolUse, opts Options) toolDescription {
	name := canonicalToolName(tool.Name)
	input := objectFromValue(tool.Input)

	switch {
	case isSearchTool(name):
		return describeSearch(input, opts)
	case isReadTool(name):
		return describeRead(input, opts)
	case isListTool(name):
		return describeList(input, opts)
	case isEditTool(name):
		return describeEdit(tool, input, opts)
	case isShellTool(name):
		return describeShell(tool, input)
	case isWebTool(name):
		return describeWeb(input)
	case name == "calculator":
		return toolDescription{
			category:     categoryCalc,
			label:        "Calculated result",
			failureLabel: "Calculation failed",
		}
	default:
		noun := firstString(input, "path", "file", "query", "command", "name")
		if noun != "" {
			noun = displayNoun(noun, opts)
		} else {
			noun = readableToolName(tool.Name)
		}
		return toolDescription{
			category:     categoryOther,
			label:        "Used " + noun,
			failureLabel: readableToolName(tool.Name) + " failed",
			noun:         noun,
		}
	}
}

func describeSearch(input map[string]any, opts Options) toolDescription {
	location := firstString(input, "root", "path", "directory", "dir", "cwd", "workdir")
	query := firstString(input, "query", "pattern")
	label := "Searched files"
	failure := "Search failed"
	if location != "" {
		location = displayDirectory(location, opts)
		label = "Searched in " + location
		failure = "Search failed in " + location
	} else if query != "" {
		query = cleanPhrase(query)
		label = "Searched for " + query
		failure = "Search failed for " + query
	}
	return toolDescription{
		category:     categorySearch,
		label:        label,
		failureLabel: failure,
		noun:         location,
	}
}

func describeRead(input map[string]any, opts Options) toolDescription {
	path := firstString(input, "path", "file", "filename", "file_path", "filepath")
	noun := displayNoun(path, opts)
	if noun == "" {
		noun = "file"
	}
	return toolDescription{
		category:     categoryRead,
		label:        "Read " + noun,
		failureLabel: "Failed to read " + noun,
		noun:         noun,
	}
}

func describeList(input map[string]any, opts Options) toolDescription {
	root := firstString(input, "root", "path", "directory", "dir", "cwd", "workdir")
	if root == "" {
		root = firstString(input, "pattern")
	}
	noun := displayDirectory(root, opts)
	if noun == "" || noun == "./" {
		noun = "files"
	}
	return toolDescription{
		category:     categoryList,
		label:        "Listed " + noun,
		failureLabel: "Failed to list " + noun,
		noun:         noun,
	}
}

func describeEdit(tool ToolUse, input map[string]any, opts Options) toolDescription {
	targets := patchTargets(firstString(input, "patch"))
	if len(targets) == 0 {
		path := firstString(input, "path", "file", "filename", "file_path", "filepath")
		if path != "" {
			targets = append(targets, patchTarget{Action: "updated", Path: path})
		}
	}
	if len(targets) == 0 {
		targets = patchTargets(StringifyValue(tool.Input, Options{MaxValueChars: 10000, WorkspaceRoot: opts.WorkspaceRoot}))
	}

	action := "updated"
	noun := "files"
	if len(targets) == 1 {
		action = targets[0].Action
		noun = displayNoun(targets[0].Path, opts)
	} else if len(targets) > 1 {
		action = targets[0].Action
		sameAction := true
		for _, target := range targets[1:] {
			if target.Action != action {
				sameAction = false
				break
			}
		}
		if !sameAction {
			action = "changed"
		}
		noun = fmt.Sprintf("%d files", len(targets))
	}
	if noun == "" {
		noun = "files"
	}

	label := title(action) + " " + noun
	return toolDescription{
		category:     categoryEdit,
		label:        label,
		failureLabel: "Failed to " + action + " " + noun,
		action:       action,
		noun:         noun,
	}
}

func describeShell(tool ToolUse, input map[string]any) toolDescription {
	command := firstString(input, "command", "cmd")
	if command == "" {
		if raw, ok := tool.Input.(string); ok {
			command = raw
		}
	}

	if isTestCommand(command) {
		label := "Ran tests"
		if normalizedStatus(tool) == StatusError || outputLooksLikeFailingTests(tool.Output) || strings.TrimSpace(tool.Error) != "" {
			label = "Ran failing tests"
		}
		return toolDescription{
			category:     categoryShell,
			label:        label,
			failureLabel: label,
			isTests:      true,
		}
	}

	noun := commandSubject(command)
	if noun == "" {
		noun = "command"
	}
	return toolDescription{
		category:     categoryShell,
		label:        "Ran " + noun,
		failureLabel: "Command failed",
		noun:         noun,
	}
}

func describeWeb(input map[string]any) toolDescription {
	query := cleanPhrase(firstString(input, "query", "q"))
	if query == "" {
		return toolDescription{
			category:     categoryWeb,
			label:        "Searched web",
			failureLabel: "Web search failed",
		}
	}
	return toolDescription{
		category:     categoryWeb,
		label:        "Searched web for " + query,
		failureLabel: "Web search failed for " + query,
		noun:         query,
	}
}

func canonicalToolName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func isSearchTool(name string) bool {
	return name == "grep_search" || name == "search" || name == "ripgrep" || name == "rg" || strings.Contains(name, "grep")
}

func isReadTool(name string) bool {
	return name == "file_read" || name == "read_file" || name == "read" || name == "view_file" || strings.Contains(name, "read")
}

func isListTool(name string) bool {
	return name == "list_files" || name == "ls" || strings.Contains(name, "list")
}

func isEditTool(name string) bool {
	return name == "apply_patch" || name == "write_file" || name == "edit_file" || name == "replace_file" || strings.Contains(name, "patch") || strings.Contains(name, "edit")
}

func isShellTool(name string) bool {
	return name == "shell" || name == "bash" || name == "exec" || strings.Contains(name, "command")
}

func isWebTool(name string) bool {
	return name == "web_search" || name == "web" || strings.Contains(name, "web")
}

func objectFromValue(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = val
		}
		return out
	case string:
		var decoded map[string]any
		if json.Unmarshal([]byte(typed), &decoded) == nil {
			return decoded
		}
		return map[string]any{}
	case []byte:
		var decoded map[string]any
		if json.Unmarshal(typed, &decoded) == nil {
			return decoded
		}
		return map[string]any{}
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			return map[string]any{}
		}
		var decoded map[string]any
		if json.Unmarshal(bytes, &decoded) != nil {
			return map[string]any{}
		}
		return decoded
	}
}

func firstString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		for actualKey, value := range input {
			if strings.EqualFold(actualKey, key) {
				if str := valueToString(value); str != "" {
					return str
				}
			}
		}
	}
	return ""
}

func valueToString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func displayNoun(path string, opts Options) string {
	path = cleanPath(path, opts)
	if path == "" {
		return ""
	}
	if strings.HasSuffix(path, "/") {
		return displayDirectory(path, opts)
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return path
	}
	if strings.EqualFold(base, "README.md") {
		return "README"
	}
	return base
}

func displayDirectory(path string, opts Options) string {
	path = cleanPath(path, opts)
	if path == "" {
		return ""
	}
	if path == "." || path == "./" {
		return "./"
	}
	path = strings.TrimSuffix(path, "/")
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return "./"
	}
	return base + "/"
}

func cleanPath(path string, opts Options) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, `"'`)
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "" {
		return ""
	}
	if opts.WorkspaceRoot != "" && filepath.IsAbs(path) {
		if rel, err := filepath.Rel(opts.WorkspaceRoot, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
	}
	return path
}

func cleanPhrase(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	return value
}

type patchTarget struct {
	Action string
	Path   string
}

func patchTargets(patch string) []patchTarget {
	lines := strings.Split(patch, "\n")
	targets := make([]patchTarget, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			targets = append(targets, patchTarget{Action: "created", Path: strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))})
		case strings.HasPrefix(line, "*** Update File: "):
			targets = append(targets, patchTarget{Action: "updated", Path: strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))})
		case strings.HasPrefix(line, "*** Delete File: "):
			targets = append(targets, patchTarget{Action: "deleted", Path: strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))})
		case strings.HasPrefix(line, "*** Move to: "):
			targets = append(targets, patchTarget{Action: "moved", Path: strings.TrimSpace(strings.TrimPrefix(line, "*** Move to: "))})
		}
	}
	return targets
}

func isTestCommand(command string) bool {
	command = strings.ToLower(strings.Join(strings.Fields(command), " "))
	if command == "" {
		return false
	}
	testPatterns := []string{
		"go test",
		"npm test",
		"npm run test",
		"pnpm test",
		"pnpm run test",
		"yarn test",
		"pytest",
		"cargo test",
		"swift test",
		"xcodebuild test",
		"make test",
	}
	for _, pattern := range testPatterns {
		if strings.HasPrefix(command, pattern) || strings.Contains(command, " "+pattern+" ") {
			return true
		}
	}
	return false
}

func outputLooksLikeFailingTests(output any) bool {
	text := strings.ToLower(StringifyValue(output, Options{MaxValueChars: 2000}))
	return strings.Contains(text, "fail") || strings.Contains(text, "failed") || strings.Contains(text, "error")
}

func commandSubject(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	first := filepath.Base(fields[0])
	if len(fields) >= 3 && (first == "npm" || first == "pnpm" || first == "yarn") && fields[1] == "run" {
		return fields[2]
	}
	if len(fields) >= 2 && first == "git" {
		return "git " + fields[1]
	}
	if len(fields) >= 2 && first == "go" {
		return "go " + fields[1]
	}
	return first
}

func readableToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tool"
	}
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	return name
}

func title(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func pluralize(noun string, count int) string {
	if count == 1 {
		return noun
	}
	if strings.HasSuffix(noun, "s") {
		return noun
	}
	return noun + "s"
}
