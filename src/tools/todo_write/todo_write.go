package todowrite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/tool"
)

const (
	statusPending    = "pending"
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
	statusCancelled  = "cancelled"

	todoArtifactID = "todo:current"
)

type TodoWriteTool struct{}

type todoWriteParams struct {
	Items []todoItem `json:"items"`
}

type todoItem struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
}

type todoStatusCounts struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

type todoSummaryItem struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Priority string `json:"priority,omitempty"`
}

type todoWriteSummary struct {
	ArtifactID  string            `json:"artifact_id"`
	Total       int               `json:"total"`
	Counts      todoStatusCounts  `json:"counts"`
	InProgress  []todoSummaryItem `json:"in_progress,omitempty"`
	NextPending []todoSummaryItem `json:"next_pending,omitempty"`
}

type todoArtifactPayload struct {
	SchemaVersion int              `json:"schema_version"`
	Kind          string           `json:"kind"`
	Items         []todoItem       `json:"items"`
	Counts        todoStatusCounts `json:"counts"`
}

func (t *TodoWriteTool) Name() string {
	return "todo_write"
}

func (t *TodoWriteTool) Description() string {
	return "维护当前任务清单。输入必须是 JSON 对象，包含 items 数组；每个 item 需要 id、content、status，可选 priority"
}

func (t *TodoWriteTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "items", Description: "完整任务清单数组，每个 item 包含 id、content、status、priority?", Type: "array", Required: true},
	}
}

func (t *TodoWriteTool) ParameterSchema() tool.ToolParametersSchema {
	return tool.ToolParametersSchema{
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"items": {
				Type:        "array",
				Description: "完整任务清单数组。调用时传入当前完整列表，而不是增量 patch。",
				Items: &tool.ToolParam{
					Type: "object",
					Properties: map[string]tool.ToolParam{
						"id":       {Type: "string", Description: "稳定任务 ID，用于后续更新同一项"},
						"content":  {Type: "string", Description: "任务内容"},
						"status":   {Type: "string", Description: "任务状态", Enum: []string{statusPending, statusInProgress, statusCompleted, statusCancelled}},
						"priority": {Type: "string", Description: "可选优先级标签，例如 high、medium、low"},
					},
					Required: []string{"id", "content", "status"},
				},
			},
		},
		Required: []string{"items"},
	}
}

func (t *TodoWriteTool) IsReadOnly() bool {
	return true
}

func (t *TodoWriteTool) Execute(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	params, err := parseParams(input)
	if err != nil {
		return "", err
	}

	summary := buildSummary(params.Items)
	data, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t *TodoWriteTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	params, err := parseParams(input)
	if err != nil {
		return nil
	}

	content, err := buildArtifactContent(params.Items)
	if err != nil {
		return nil
	}
	counts := countStatuses(params.Items)

	return []tool.ArtifactDescriptor{{
		Action:      tool.ArtifactActionUpdated,
		ID:          todoArtifactID,
		Title:       "Current todo plan",
		Description: fmt.Sprintf("%d todos: %d pending, %d in progress, %d completed, %d cancelled", len(params.Items), counts.Pending, counts.InProgress, counts.Completed, counts.Cancelled),
		Kind:        "todo",
		Content:     content,
		Status:      "ready",
	}}
}

func parseParams(input string) (todoWriteParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return todoWriteParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil || raw == nil {
		if err == nil {
			err = fmt.Errorf("decoded null")
		}
		return todoWriteParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}

	rawItems, ok := raw["items"]
	if !ok {
		return todoWriteParams{}, fmt.Errorf("缺少必需参数: items")
	}
	if strings.TrimSpace(string(rawItems)) == "null" {
		return todoWriteParams{}, fmt.Errorf("items 必须是数组")
	}

	var items []todoItem
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return todoWriteParams{}, fmt.Errorf("items 必须是数组: %w", err)
	}

	normalized, err := normalizeItems(items)
	if err != nil {
		return todoWriteParams{}, err
	}
	return todoWriteParams{Items: normalized}, nil
}

func normalizeItems(items []todoItem) ([]todoItem, error) {
	seenIDs := make(map[string]struct{}, len(items))
	normalized := make([]todoItem, 0, len(items))
	for i, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Content = strings.TrimSpace(item.Content)
		item.Status = strings.TrimSpace(item.Status)
		item.Priority = strings.TrimSpace(item.Priority)

		if item.ID == "" {
			return nil, fmt.Errorf("items[%d].id 不能为空", i)
		}
		if item.Content == "" {
			return nil, fmt.Errorf("items[%d].content 不能为空", i)
		}
		if !validStatus(item.Status) {
			return nil, fmt.Errorf("items[%d].status 必须是 pending、in_progress、completed 或 cancelled", i)
		}
		if _, ok := seenIDs[item.ID]; ok {
			return nil, fmt.Errorf("items[%d].id %q 重复", i, item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func validStatus(status string) bool {
	switch status {
	case statusPending, statusInProgress, statusCompleted, statusCancelled:
		return true
	default:
		return false
	}
}

func buildSummary(items []todoItem) todoWriteSummary {
	summary := todoWriteSummary{
		ArtifactID: todoArtifactID,
		Total:      len(items),
		Counts:     countStatuses(items),
	}
	for _, item := range items {
		switch item.Status {
		case statusInProgress:
			if len(summary.InProgress) < 3 {
				summary.InProgress = append(summary.InProgress, summarizeItem(item))
			}
		case statusPending:
			if len(summary.NextPending) < 3 {
				summary.NextPending = append(summary.NextPending, summarizeItem(item))
			}
		}
	}
	return summary
}

func summarizeItem(item todoItem) todoSummaryItem {
	return todoSummaryItem{
		ID:       item.ID,
		Content:  item.Content,
		Priority: item.Priority,
	}
}

func countStatuses(items []todoItem) todoStatusCounts {
	var counts todoStatusCounts
	for _, item := range items {
		switch item.Status {
		case statusPending:
			counts.Pending++
		case statusInProgress:
			counts.InProgress++
		case statusCompleted:
			counts.Completed++
		case statusCancelled:
			counts.Cancelled++
		}
	}
	return counts
}

func buildArtifactContent(items []todoItem) (string, error) {
	data, err := json.MarshalIndent(todoArtifactPayload{
		SchemaVersion: 1,
		Kind:          "todo",
		Items:         items,
		Counts:        countStatuses(items),
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func init() {
	tool.MustRegisterTool(&TodoWriteTool{})
}
