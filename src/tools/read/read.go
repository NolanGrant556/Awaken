package read

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/textfile"
)

const (
	ToolName             = "Read"
	defaultLineLimit     = 2000
	maxLineLimit         = 2000
	defaultPDFLastPage   = 5
	maxPDFPages          = 20
	maxModelMediaPayload = 20 * 1024 * 1024
)

type Tool struct{}

type params struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Pages    string `json:"pages,omitempty"`
}

func (t *Tool) Name() string { return ToolName }

func (t *Tool) Description() string {
	return "Read a text file, supported image, or PDF. A relative path resolves against the primary folder; an absolute path is read wherever it points, including outside the task's folders. Text uses 1-based offset/limit; images and PDFs return model-visible media when the selected model supports it."
}

func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "file_path", Description: "Absolute path or path relative to the workspace.", Type: "string", Required: true},
		{Name: "offset", Description: "1-based first text line. Defaults to 1.", Type: "integer"},
		{Name: "limit", Description: "Maximum text lines. Defaults to and is capped at 2000.", Type: "integer"},
		{Name: "pages", Description: "PDF pages, for example 1-5 or 1,3,7-9. Defaults to 1-5 and allows at most 20 pages.", Type: "string"},
	}
}

func (t *Tool) IsReadOnly() bool { return true }

func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        ToolName,
		DisplayName: "Read",
		Description: t.Description(),
		Purpose:     "Read bounded text or send supported file media to the selected model without persisting inline blobs.",
		WhenToUse: []string{
			"Inspecting source code, configuration, Markdown, logs, images, or PDFs",
			"Before editing or overwriting an existing file with Edit or Write",
		},
		WhenNotToUse: []string{
			"Office and notebook binaries are not parsed by Read",
			"Use Glob to discover paths and Grep to search file contents",
		},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"task file"}, Description: "Reads file bytes and metadata only.", Reversibility: tool.ReversibilityNotApplicable}},
		Examples: []tool.ToolExample{
			{Scenario: "Read text", Input: map[string]any{"file_path": "README.md", "offset": 1, "limit": 200}, Outcome: "Returns numbered text lines and a host snapshot."},
			{Scenario: "Read PDF pages", Input: map[string]any{"file_path": "report.pdf", "pages": "1-5"}, Outcome: "Returns a native PDF or rendered page images according to model capability."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "read_invalid_input", Cause: "Path, text range, or PDF page range is invalid", Retryable: false},
			{Code: "unsupported_media", Cause: "The file type or selected model capability is unsupported", Retryable: false},
			{Code: "read_media_too_large", Cause: "The model media payload exceeds 20 MiB", Retryable: false},
			{Code: tool.ErrorCodeNeedsRuntime, Cause: "Poppler is required for PDF rendering but unavailable", Retryable: true},
		},
		OutputSchema: tool.ToolParam{Type: "object", Description: "File snapshot, range, route, and media summary details.", AdditionalProperties: tool.BoolPtr(true)},
		Annotations:  tool.ToolAnnotations{ReadOnly: true, CapabilityClass: tool.CapabilityArtifactRead, TrustedOrigin: tool.TrustedOriginBuiltin},
	})
}

func (t *Tool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	p, err := parseParams(input)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if _, err := permissionContext.ResolveReadablePathChecked(p.FilePath); err != nil {
		return tool.DenyPermission(err.Error())
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
		return failure(started, "read_invalid_input", err.Error(), tool.ErrorCategoryValidation, false), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	path := permissionContext.ResolvePath(p.FilePath)
	if tool.HasPermissionContext(ctx) {
		path, err = permissionContext.ResolveReadablePathChecked(p.FilePath)
		if err != nil {
			return failure(started, "read_path_unavailable", err.Error(), tool.ErrorCategoryPolicy, false), nil
		}
	}
	data, info, err := readRegularFile(ctx, path)
	if err != nil {
		return failure(started, "read_failed", err.Error(), tool.ErrorCategoryTool, true), nil
	}
	mimeType, kind := detectFileKind(path, data)
	snapshot := snapshotFor(path, mimeType, data, info)
	state, hasState := tool.ToolSessionStateFromContext(ctx)
	if hasState {
		if previous, ok := state.File(path); ok && sameSnapshot(previous, snapshot) {
			state.RecordFile(snapshot)
			outcome := tool.SucceededOutcome("file_unchanged")
			outcome.Details = readDetails(snapshot, "file_unchanged")
			outcome.Summary = fmt.Sprintf("%s has not changed since this thread last read it.", filepath.Base(path))
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
	}

	var outcome tool.ExecutionOutcome
	switch kind {
	case "text":
		outcome, err = readText(p, path, data)
	case "image":
		outcome, err = readImage(ctx, path, mimeType, data)
	case "pdf":
		outcome, err = readPDF(ctx, p, path, data)
	default:
		outcome = unsupportedOutcome(
			fmt.Sprintf("unsupported_media: Read does not parse %s files", mimeType),
			"unsupported_media",
			[]string{"use_an_appropriate_skill", "use_bash_with_an_installed_converter"},
		)
	}
	if err != nil {
		return failure(started, "read_failed", err.Error(), tool.ErrorCategoryTool, true), nil
	}
	outcome.Details = mergeDetails(outcome.Details, readDetails(snapshot, kind))
	if !outcome.IsError() && hasState {
		state.RecordFile(snapshot)
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func parseParams(input string) (params, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return params{}, fmt.Errorf("file_path is required")
	}
	if !strings.HasPrefix(input, "{") {
		return params{FilePath: input}, nil
	}
	var p params
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return params{}, fmt.Errorf("invalid Read input: %w", err)
	}
	if strings.TrimSpace(p.FilePath) == "" {
		return params{}, fmt.Errorf("file_path is required")
	}
	if p.Offset < 0 || p.Limit < 0 || p.Limit > maxLineLimit {
		return params{}, fmt.Errorf("offset must be >= 1 and limit must be between 1 and %d", maxLineLimit)
	}
	return p, nil
}

func readRegularFile(ctx context.Context, path string) ([]byte, os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return data, info, nil
}

func detectFileKind(path string, data []byte) (string, string) {
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return "application/pdf", "pdf"
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png", "image"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg", "image"
	}
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return "image/gif", "image"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp", "image"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".docx" || ext == ".xlsx" || ext == ".pptx" || ext == ".doc" || ext == ".xls" || ext == ".ppt" || ext == ".ipynb" {
		return "application/octet-stream", "unsupported"
	}
	if _, err := textfile.Decode(data); err == nil {
		return "text/plain; charset=utf-8", "text"
	}
	return "application/octet-stream", "unsupported"
}

func snapshotFor(path, mimeType string, data []byte, info os.FileInfo) tool.FileReadState {
	sum := sha256.Sum256(data)
	return tool.FileReadState{
		CanonicalPath: filepath.Clean(path),
		SHA256:        hex.EncodeToString(sum[:]),
		Size:          int64(len(data)),
		ModTime:       info.ModTime().UTC(),
		Mode:          uint32(info.Mode()),
		MimeType:      mimeType,
	}
}

func sameSnapshot(left, right tool.FileReadState) bool {
	return left.CanonicalPath == right.CanonicalPath && left.SHA256 == right.SHA256 && left.Size == right.Size && left.ModTime.Equal(right.ModTime) && left.Mode == right.Mode && left.MimeType == right.MimeType
}

func readText(p params, path string, data []byte) (tool.ExecutionOutcome, error) {
	document, err := textfile.Decode(data)
	if err != nil {
		return tool.FailedOutcome(err.Error(), "unsupported_text_encoding", tool.ErrorCategoryUnsupported, false), nil
	}
	text := document.Text
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	offset := p.Offset
	if offset == 0 {
		offset = 1
	}
	limit := p.Limit
	if limit == 0 {
		limit = defaultLineLimit
	}
	if offset > len(lines)+1 {
		return tool.FailedOutcome(fmt.Sprintf("offset %d is beyond end of file (%d lines)", offset, len(lines)), "read_offset_out_of_range", tool.ErrorCategoryValidation, false), nil
	}
	start := offset - 1
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	width := len(strconv.Itoa(end))
	if width < 1 {
		width = 1
	}
	var builder strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&builder, "%*d\t%s", width, i+1, strings.TrimSuffix(lines[i], "\r"))
		if i+1 < end {
			builder.WriteByte('\n')
		}
	}
	if start == end {
		builder.WriteString("[empty file]")
	}
	outcome := tool.SucceededOutcome(builder.String())
	outcome.Truncated = end < len(lines)
	outcome.Summary = fmt.Sprintf("Read lines %d-%d of %s.", offset, end, filepath.Base(path))
	outcome.Details = map[string]any{"type": "text", "offset": offset, "limit": limit, "returned_lines": end - start, "total_lines": len(lines), "truncated": outcome.Truncated}
	return outcome, nil
}

func readImage(ctx context.Context, path, mimeType string, data []byte) (tool.ExecutionOutcome, error) {
	if !tool.ModelCapabilitiesFromContext(ctx).SupportsVision {
		return unsupportedOutcome("unsupported_media: the selected model does not support vision", "model_vision_unsupported", []string{"select_a_vision_capable_model"}), nil
	}
	if len(data) > maxModelMediaPayload {
		return tool.FailedOutcome("Image exceeds the 20 MiB model media limit.", "read_media_too_large", tool.ErrorCategoryValidation, false), nil
	}
	outcome := tool.SucceededOutcome(fmt.Sprintf("image: %s (%s, %d bytes)", filepath.Base(path), mimeType, len(data)))
	outcome.ModelContent = []tool.ModelContentPart{{Type: "image", MimeType: mimeType, DisplayName: filepath.Base(path), ByteSize: int64(len(data)), DataBase64: base64.StdEncoding.EncodeToString(data)}}
	outcome.Summary = fmt.Sprintf("Read image %s.", filepath.Base(path))
	return outcome, nil
}

func readPDF(ctx context.Context, p params, path string, data []byte) (tool.ExecutionOutcome, error) {
	pages, err := parsePages(p.Pages)
	if err != nil {
		return tool.FailedOutcome(err.Error(), "read_invalid_pages", tool.ErrorCategoryValidation, false), nil
	}
	capabilities := tool.ModelCapabilitiesFromContext(ctx)
	if capabilities.SupportsNativePDF {
		if len(data) > maxModelMediaPayload {
			return tool.FailedOutcome("PDF exceeds the 20 MiB model media limit.", "read_media_too_large", tool.ErrorCategoryValidation, false), nil
		}
		outcome := tool.SucceededOutcome(fmt.Sprintf("document: %s (%d bytes)", filepath.Base(path), len(data)))
		outcome.ModelContent = []tool.ModelContentPart{{Type: "file", MimeType: "application/pdf", DisplayName: filepath.Base(path), ByteSize: int64(len(data)), DataBase64: base64.StdEncoding.EncodeToString(data)}}
		outcome.Details = map[string]any{"type": "document", "route": "native_pdf", "requested_pages": pages}
		outcome.Summary = fmt.Sprintf("Read PDF %s with the model's native PDF capability.", filepath.Base(path))
		return outcome, nil
	}
	if !capabilities.SupportsVision {
		return unsupportedOutcome("unsupported_media: the selected model supports neither native PDF nor vision fallback", "model_pdf_unsupported", []string{"select_a_native_pdf_or_vision_capable_model"}), nil
	}
	parts, err := renderPDFPages(ctx, path, pages)
	if err != nil {
		if errorsIsRuntimeMissing(err) {
			outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeNeedsRuntime, tool.ErrorCategoryRuntime, true)
			outcome.Status = tool.OutcomeNeedsRuntime
			outcome.RecoveryOptions = []string{"install_poppler", "select_a_native_pdf_model"}
			return outcome, nil
		}
		return tool.FailedOutcome(err.Error(), "pdf_render_failed", tool.ErrorCategoryRuntime, true), nil
	}
	total := int64(0)
	for _, part := range parts {
		total += part.ByteSize
	}
	if total > maxModelMediaPayload {
		return tool.FailedOutcome("Rendered PDF pages exceed the 20 MiB model media limit.", "read_media_too_large", tool.ErrorCategoryValidation, false), nil
	}
	outcome := tool.SucceededOutcome(fmt.Sprintf("document parts: %s pages %s", filepath.Base(path), formatPages(pages)))
	outcome.ModelContent = parts
	outcome.Details = map[string]any{"type": "parts", "route": "poppler", "pages": pages, "total_media_bytes": total}
	outcome.Summary = fmt.Sprintf("Rendered %d page(s) from %s for the vision model.", len(parts), filepath.Base(path))
	return outcome, nil
}

func parsePages(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fmt.Sprintf("1-%d", defaultPDFLastPage)
	}
	seen := make(map[int]bool)
	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, fmt.Errorf("invalid empty PDF page token")
		}
		bounds := strings.Split(token, "-")
		if len(bounds) > 2 {
			return nil, fmt.Errorf("invalid PDF page range %q", token)
		}
		first, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil || first < 1 {
			return nil, fmt.Errorf("invalid PDF page %q", token)
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || last < first {
				return nil, fmt.Errorf("invalid PDF page range %q", token)
			}
		}
		for page := first; page <= last; page++ {
			seen[page] = true
			if len(seen) > maxPDFPages {
				return nil, fmt.Errorf("a Read call may request at most %d PDF pages", maxPDFPages)
			}
		}
	}
	pages := make([]int, 0, len(seen))
	for page := range seen {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	return pages, nil
}

type runtimeMissingError struct{ name string }

func (e runtimeMissingError) Error() string {
	return fmt.Sprintf("required runtime %s is unavailable", e.name)
}
func errorsIsRuntimeMissing(err error) bool { _, ok := err.(runtimeMissingError); return ok }

func renderPDFPages(ctx context.Context, path string, pages []int) ([]tool.ModelContentPart, error) {
	executable := ""
	environment, ok := tool.ExecutionEnvironmentSnapshotFromContext(ctx)
	if ok {
		if observed, found := environment.Executable("pdftoppm"); found && strings.TrimSpace(observed.Path) != "" {
			executable = observed.Path
		}
	} else if discovered, err := exec.LookPath("pdftoppm"); err == nil {
		executable = discovered
	}
	if executable == "" {
		return nil, runtimeMissingError{name: "pdftoppm (Poppler)"}
	}
	tempDir, err := os.MkdirTemp("", "awaken-read-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	parts := make([]tool.ModelContentPart, 0, len(pages))
	for _, page := range pages {
		prefix := filepath.Join(tempDir, fmt.Sprintf("page-%03d", page))
		command := exec.CommandContext(ctx, executable, "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile", "-png", "-r", "144", path, prefix)
		if environment, ok := tool.ExecutionEnvironmentSnapshotFromContext(ctx); ok {
			command.Env = replacePath(os.Environ(), environment.PinnedTransportPATH())
		}
		output, err := command.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("Poppler failed for page %d: %v: %s", page, err, strings.TrimSpace(string(output)))
		}
		pageData, err := os.ReadFile(prefix + ".png")
		if err != nil {
			return nil, fmt.Errorf("cannot read rendered PDF page %d: %w", page, err)
		}
		parts = append(parts, tool.ModelContentPart{Type: "image", MimeType: "image/png", DisplayName: fmt.Sprintf("%s page %d", filepath.Base(path), page), ByteSize: int64(len(pageData)), Page: page, DataBase64: base64.StdEncoding.EncodeToString(pageData)})
	}
	return parts, nil
}

func replacePath(environment []string, pathValue string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.HasPrefix(strings.ToUpper(entry), "PATH=") {
			continue
		}
		result = append(result, entry)
	}
	if strings.TrimSpace(pathValue) != "" {
		result = append(result, "PATH="+pathValue)
	}
	return result
}

func formatPages(pages []int) string {
	values := make([]string, len(pages))
	for i, page := range pages {
		values[i] = strconv.Itoa(page)
	}
	return strings.Join(values, ",")
}

func readDetails(snapshot tool.FileReadState, resultType string) map[string]any {
	return map[string]any{
		"type": resultType, "file_path": snapshot.CanonicalPath, "mime_type": snapshot.MimeType,
		"sha256": snapshot.SHA256, "size": snapshot.Size, "mtime": snapshot.ModTime.Format(time.RFC3339Nano), "mode": snapshot.Mode,
	}
}

func mergeDetails(left, right map[string]any) map[string]any {
	result := make(map[string]any, len(left)+len(right))
	for key, value := range left {
		result[key] = value
	}
	for key, value := range right {
		result[key] = value
	}
	return result
}

func failure(started time.Time, code, message string, category tool.ErrorCategory, retryable bool) tool.ExecutionOutcome {
	return tool.FailedOutcome(message, code, category, retryable).WithTiming(started, time.Now().UTC())
}

func unsupportedOutcome(message, code string, recovery []string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, code, tool.ErrorCategoryUnsupported, false)
	outcome.Status = tool.OutcomeUnsupported
	outcome.RecoveryOptions = append([]string(nil), recovery...)
	return outcome
}

func init() {
	tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin)
}
