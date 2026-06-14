package fileread

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
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
	return "用于读取指定文件的内容"
}

func (t *FileReadTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "文件路径", Type: "string", Required: true},
		{Name: "offset", Description: "起始行号，默认 1", Type: "integer", Required: false},
		{Name: "limit", Description: "最多读取多少行，默认读到 max_bytes 限制，最大 5000", Type: "integer", Required: false},
		{Name: "max_bytes", Description: "最多返回多少字节，默认 262144，最大 1048576", Type: "integer", Required: false},
	}
}

func (t *FileReadTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(params.Path) == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}

	return readTextRange(ctx, params)
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

func readTextRange(ctx context.Context, params fileReadParams) (string, error) {
	path := strings.TrimSpace(params.Path)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("读取文件失败: %s 是目录", path)
	}

	binary, err := filesafe.LooksBinary(path)
	if err != nil {
		return "", fmt.Errorf("检测文件类型失败: %v", err)
	}
	if binary {
		return "", fmt.Errorf("拒绝读取二进制文件: %s", path)
	}

	offset := normalizeOffset(params.Offset)
	limit := normalizeLimit(params.Limit)
	maxBytes := normalizeMaxBytes(params.MaxBytes)

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var builder strings.Builder
	lineNumber := 0
	returnedLines := 0
	truncated := false
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("读取文件失败: %v", err)
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
		content += fmt.Sprintf("\n\n[read_file truncated: start_line=%d returned_lines=%d max_bytes=%d]", offset, returnedLines, maxBytes)
	}
	if content == "" {
		return fmt.Sprintf("[read_file: no content returned from line %d]", offset), nil
	}
	return content, nil
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
