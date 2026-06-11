package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
	return "计算器"
}

func (t *CalculatorTool) Description() string {
	return "用于执行数学计算，支持加减乘除、幂运算、平方根等基本数学运算"
}

func (t *CalculatorTool) Parameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "expression",
			Description: "要计算的数学表达式",
			Type:        "string",
			Required:    true,
		},
	}
}

func (t *CalculatorTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("表达式不能为空")
	}
	
	result, err := evaluateExpression(input)
	if err != nil {
		return "", fmt.Errorf("计算失败: %v", err)
	}
	
	return fmt.Sprintf("%.10g", result), nil
}

func evaluateExpression(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	
	if strings.Contains(expr, "^") {
		parts := strings.SplitN(expr, "^", 2)
		base, err := evaluateExpression(parts[0])
		if err != nil {
			return 0, err
		}
		exp, err := evaluateExpression(parts[1])
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}
	
	if strings.Contains(expr, "sqrt(") && strings.HasSuffix(expr, ")") {
		inner := expr[5 : len(expr)-1]
		val, err := evaluateExpression(inner)
		if err != nil {
			return 0, err
		}
		return math.Sqrt(val), nil
	}
	
	if strings.Contains(expr, "+") {
		parts := strings.Split(expr, "+")
		var sum float64
		for _, part := range parts {
			val, err := evaluateExpression(part)
			if err != nil {
				return 0, err
			}
			sum += val
		}
		return sum, nil
	}
	
	if strings.Contains(expr, "-") && !strings.HasPrefix(expr, "-") {
		parts := strings.Split(expr, "-")
		if len(parts) == 0 {
			return 0, fmt.Errorf("无效表达式")
		}
		val, err := evaluateExpression(parts[0])
		if err != nil {
			return 0, err
		}
		for i := 1; i < len(parts); i++ {
			sub, err := evaluateExpression(parts[i])
			if err != nil {
				return 0, err
			}
			val -= sub
		}
		return val, nil
	}
	
	if strings.Contains(expr, "*") {
		parts := strings.Split(expr, "*")
		var product float64 = 1
		for _, part := range parts {
			val, err := evaluateExpression(part)
			if err != nil {
				return 0, err
			}
			product *= val
		}
		return product, nil
	}
	
	if strings.Contains(expr, "/") {
		parts := strings.Split(expr, "/")
		if len(parts) == 0 {
			return 0, fmt.Errorf("无效表达式")
		}
		val, err := evaluateExpression(parts[0])
		if err != nil {
			return 0, err
		}
		for i := 1; i < len(parts); i++ {
			div, err := evaluateExpression(parts[i])
			if err != nil {
				return 0, err
			}
			if div == 0 {
				return 0, fmt.Errorf("除零错误")
			}
			val /= div
		}
		return val, nil
	}
	
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return evaluateExpression(expr[1 : len(expr)-1])
	}
	
	return strconv.ParseFloat(expr, 64)
}

type FileReadTool struct{}

func (t *FileReadTool) Name() string {
	return "读取文件"
}

func (t *FileReadTool) Description() string {
	return "用于读取指定文件的内容"
}

func (t *FileReadTool) Parameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "path",
			Description: "文件路径",
			Type:        "string",
			Required:    true,
		},
	}
}

func (t *FileReadTool) Execute(ctx context.Context, input string) (string, error) {
	path := strings.TrimSpace(input)
	if path == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}
	
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	
	return string(content), nil
}

type FileWriteTool struct{}

func (t *FileWriteTool) Name() string {
	return "写入文件"
}

func (t *FileWriteTool) Description() string {
	return "用于将内容写入指定文件，支持覆盖和追加模式"
}

func (t *FileWriteTool) Parameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "path",
			Description: "文件路径",
			Type:        "string",
			Required:    true,
		},
		{
			Name:        "content",
			Description: "要写入的内容",
			Type:        "string",
			Required:    true,
		},
		{
			Name:        "append",
			Description: "是否追加模式，默认false（覆盖）",
			Type:        "bool",
			Required:    false,
		},
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	
	var params map[string]interface{}
	err := json.Unmarshal([]byte(input), &params)
	if err != nil {
		return "", fmt.Errorf("参数格式错误，需要JSON格式: %v", err)
	}
	
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("缺少必需参数: path")
	}
	
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: content")
	}
	
	appendMode := false
	if v, ok := params["append"].(bool); ok {
		appendMode = v
	}
	
	var file *os.File
	if appendMode {
		file, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	} else {
		file, err = os.Create(path)
	}
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()
	
	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}
	
	return fmt.Sprintf("文件 %s 写入成功", path), nil
}

type WebSearchTool struct{}

func (t *WebSearchTool) Name() string {
	return "网络搜索"
}

func (t *WebSearchTool) Description() string {
	return "用于在互联网上搜索信息"
}

func (t *WebSearchTool) Parameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "query",
			Description: "搜索关键词",
			Type:        "string",
			Required:    true,
		},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, input string) (string, error) {
	query := strings.TrimSpace(input)
	if query == "" {
		return "", fmt.Errorf("搜索关键词不能为空")
	}
	
	url := "https://api.search.brave.com/res/v1/web/search?q=" + query
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("搜索请求失败: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("搜索失败，状态码: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}
	
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	
	results, ok := result["web"].(map[string]interface{})["results"].([]interface{})
	if !ok || len(results) == 0 {
		return "未找到相关结果", nil
	}
	
	var summary strings.Builder
	summary.WriteString("搜索结果:\n")
	for i, r := range results {
		if i >= 5 {
			break
		}
		resultMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		title := resultMap["title"].(string)
		url := resultMap["url"].(string)
		description := ""
		if d, ok := resultMap["description"].(string); ok {
			description = d
		}
		summary.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n   摘要: %s\n\n", i+1, title, url, description))
	}
	
	return summary.String(), nil
}

func init() {
	RegisterTool(&CalculatorTool{})
	RegisterTool(&FileReadTool{})
	RegisterTool(&FileWriteTool{})
	RegisterTool(&WebSearchTool{})
}