package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"awaken/src/tool"
)

type WebSearchTool struct{}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "用于在互联网上搜索信息"
}

func (t *WebSearchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{
			Name:        "query",
			Description: "搜索关键词",
			Type:        "string",
			Required:    true,
		},
	}
}

func (t *WebSearchTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        "web_search",
		DisplayName: "Web Search",
		Source:      tool.SourceBuiltin,
		Description: "用于在互联网上搜索信息",
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityBrowserRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
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
	tool.MustRegisterTool(&WebSearchTool{})
}
