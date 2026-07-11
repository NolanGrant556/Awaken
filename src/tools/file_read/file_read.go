package fileread

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/tool"
	"awaken/src/tools/internal/filesafe"
)

const (
	defaultMaxBytes = 256 * 1024
	maxBytesLimit   = 1024 * 1024
	maxLineLimit    = 5000
)

type FileReadTool struct{}

type fileReadParams struct {
	Path     string `json:"path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
	MaxBytes int    `json:"max_bytes"`
}

func (t *FileReadTool) Name() string {
	return "read_file"
}

func (t *FileReadTool) Description() string {
	return "读取本机文本文件内容，适合源码、Markdown、JSON、CSV 等文本；路径可用绝对路径、相对当前工作区路径或已上传附件文件名。结构化结果含 sha256/size/mtime，可供后续 text_edit 的 expected_sha256。不会读取 PDF、Office、图片、音视频等二进制/工作文件。PDF/Office/表格请用 read_artifact，PDF 文本抽取用 pdf_extract，图片元数据用 image_inspect"
}

func (t *FileReadTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "文本文件路径，可用绝对路径、相对当前工作区路径或已上传附件文件名；PDF/Office/图片不要传给 read_file", Type: "string", Required: true},
		{Name: "offset", Description: "起始行号，默认 1", Type: "integer", Required: false},
		{Name: "limit", Description: "最多读取多少行，默认读到 max_bytes 限制，最大 5000", Type: "integer", Required: false},
		{Name: "max_bytes", Description: "最多返回多少字节，默认 262144，最大 1048576", Type: "integer", Required: false},
	}
}

func (t *FileReadTool) IsReadOnly() bool {
	return true
}

func (t *FileReadTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "read_file",
		DisplayName: "Read File",
		Description: t.Description(),
		Purpose:     "读取有界的 UTF-8 文本范围，并返回可用于安全后续编辑的文件版本事实。",
		WhenToUse: []string{
			"需要查看源码、Markdown、JSON、CSV 或其他文本文件的实际内容时",
			"调用 text_edit 前需要取得当前 sha256 作为 expected_sha256 时",
		},
		WhenNotToUse: []string{
			"PDF、Office、图片或其他二进制工作文件应使用 read_artifact 或对应 artifact 工具",
			"只需要文件名集合时使用 list_files；需要跨文件内容匹配时使用 grep_search",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"path":            {Type: "string", Description: "实际读取路径"},
			"sha256":          {Type: "string", Description: "读取时文件内容摘要"},
			"expected_sha256": {Type: "string", Description: "可直接传给 text_edit 的版本摘要"},
			"size":            {Type: "integer", Description: "文件总字节数"},
			"mtime":           {Type: "string", Description: "读取时文件修改时间"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"workspace text file"}, Description: "读取文件内容和元数据，不修改文件。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "读取 Go 文件前 120 行", Input: map[string]any{"path": "src/tool/tool.go", "offset": 1, "limit": 120}, Outcome: "返回文本片段、sha256、size、mtime 和截断状态"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: tool.ErrorCodeValidationError, Cause: "路径或范围参数无效", Retryable: false, Recovery: []string{"修正参数后重试"}},
			{Code: tool.ErrorCodeToolExecutionFailed, Cause: "文件不存在、不可读或不是受支持文本", Retryable: true, Recovery: []string{"核对路径和可读范围", "二进制工作文件改用 read_artifact"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
}

func (t *FileReadTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if _, err := parseParams(input); err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	return tool.AllowPermission()
}

func (t *FileReadTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *FileReadTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	params, err := parseParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if strings.TrimSpace(params.Path) == "" {
		outcome := tool.FailedOutcome("文件路径不能为空", tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	if tool.HasPermissionContext(ctx) {
		checkedPath, err := permissionContext.ResolvePathChecked(params.Path)
		if err != nil {
			outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryTool, true)
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
		params.Path = checkedPath
	} else {
		params.Path = permissionContext.ResolvePath(params.Path)
	}

	content, meta, err := readTextRange(ctx, params)
	finished := time.Now().UTC()
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryTool, true)
		return outcome.WithTiming(started, finished), nil
	}

	outcome := tool.SucceededOutcome(content).WithTiming(started, finished)
	outcome.Diagnostic = map[string]any{
		"sha256":          meta.SHA256,
		"expected_sha256": meta.SHA256,
		"size":            meta.Size,
		"mtime":           meta.ModTime.UTC().Format(time.RFC3339Nano),
		"path":            params.Path,
	}
	outcome.Details = map[string]any{
		"path": params.Path, "sha256": meta.SHA256, "expected_sha256": meta.SHA256,
		"size": meta.Size, "mtime": meta.ModTime.UTC().Format(time.RFC3339Nano),
	}
	if meta.Truncated {
		outcome.Truncated = true
		outcome.Diagnostic["truncated"] = true
	}
	return outcome, nil
}

type fileReadMeta struct {
	SHA256    string
	Size      int64
	ModTime   time.Time
	Truncated bool
}

func parseParams(input string) (fileReadParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return fileReadParams{}, nil
	}
	if !strings.HasPrefix(input, "{") {
		return fileReadParams{Path: input}, nil
	}

	var params fileReadParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return fileReadParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象或文件路径: %w", err)
	}
	return params, nil
}

func readTextRange(ctx context.Context, params fileReadParams) (string, fileReadMeta, error) {
	path := strings.TrimSpace(params.Path)
	info, err := os.Stat(path)
	if err != nil {
		return "", fileReadMeta{}, fmt.Errorf("读取文件失败: %v", err)
	}
	if info.IsDir() {
		return "", fileReadMeta{}, fmt.Errorf("读取文件失败: %s 是目录", path)
	}

	binary, err := filesafe.LooksBinary(path)
	if err != nil {
		return "", fileReadMeta{}, fmt.Errorf("检测文件类型失败: %v", err)
	}
	if binary {
		return "", fileReadMeta{}, fmt.Errorf("拒绝读取二进制文件: %s。read_file 仅用于文本源码/Markdown/JSON/CSV 等文本；PDF/Office/表格请用 read_artifact，PDF 文本抽取用 pdf_extract，图片元数据用 image_inspect", path)
	}

	fullData, err := os.ReadFile(path)
	if err != nil {
		return "", fileReadMeta{}, fmt.Errorf("读取文件失败: %v", err)
	}
	sum := sha256.Sum256(fullData)
	meta := fileReadMeta{
		SHA256:  hex.EncodeToString(sum[:]),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}

	offset := normalizeOffset(params.Offset)
	limit := normalizeLimit(params.Limit)
	maxBytes := normalizeMaxBytes(params.MaxBytes)

	reader := bufio.NewReader(strings.NewReader(string(fullData)))
	var builder strings.Builder
	lineNumber := 0
	returnedLines := 0
	truncated := false
	for {
		if ctx.Err() != nil {
			return "", fileReadMeta{}, ctx.Err()
		}

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fileReadMeta{}, fmt.Errorf("读取文件失败: %v", err)
		}
		if line != "" {
			lineNumber++
			if lineNumber >= offset {
				if limit > 0 && returnedLines >= limit {
					truncated = true
					break
				}
				if !appendWithinLimit(&builder, line, maxBytes) {
					truncated = true
					break
				}
				returnedLines++
			}
		}
		if err == io.EOF {
			break
		}
	}

	content := strings.TrimSuffix(builder.String(), "\n")
	if truncated {
		meta.Truncated = true
		content += fmt.Sprintf("\n\n[read_file truncated: start_line=%d returned_lines=%d max_bytes=%d]", offset, returnedLines, maxBytes)
	}
	if content == "" {
		return fmt.Sprintf("[read_file: no content returned from line %d]", offset), meta, nil
	}
	return content, meta, nil
}

func normalizeOffset(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func normalizeLimit(value int) int {
	if value <= 0 {
		return 0
	}
	if value > maxLineLimit {
		return maxLineLimit
	}
	return value
}

func normalizeMaxBytes(value int) int {
	if value <= 0 {
		return defaultMaxBytes
	}
	if value > maxBytesLimit {
		return maxBytesLimit
	}
	return value
}

func appendWithinLimit(builder *strings.Builder, value string, maxBytes int) bool {
	if builder.Len()+len(value) <= maxBytes {
		builder.WriteString(value)
		return true
	}

	remaining := maxBytes - builder.Len()
	if remaining <= 0 {
		return false
	}
	builder.WriteString(takeBytesAtRuneBoundary(value, remaining))
	return false
}

func takeBytesAtRuneBoundary(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.ValidString(value[:maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

func init() {
	tool.MustRegisterToolWithMode(&FileReadTool{}, tool.DefinitionValidationStrictBuiltin)
}
