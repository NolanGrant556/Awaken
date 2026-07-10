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
	return tool.ToolDefinition{
		Name:        "read_file",
		DisplayName: "Read File",
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	}
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
	tool.MustRegisterTool(&FileReadTool{})
}
