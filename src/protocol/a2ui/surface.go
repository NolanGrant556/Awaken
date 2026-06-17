package a2ui

import (
	"encoding/json"
	"strings"
)

const (
	Version           = "v0.9"
	ArtifactKind      = "a2ui_surface"
	ApprovalSurfaceID = "approval-surface"
)

type Surface struct {
	Version          string          `json:"version"`
	Metadata         SurfaceMetadata `json:"metadata"`
	UpdateComponents ComponentUpdate `json:"updateComponents"`
}

type SurfaceMetadata struct {
	Source   string `json:"source"`
	ReadOnly bool   `json:"readOnly"`
}

type ComponentUpdate struct {
	SurfaceID  string      `json:"surfaceId"`
	Components []Component `json:"components"`
}

type Component struct {
	ID        string   `json:"id"`
	Component string   `json:"component"`
	Text      string   `json:"text,omitempty"`
	Variant   string   `json:"variant,omitempty"`
	Tone      string   `json:"tone,omitempty"`
	Label     string   `json:"label,omitempty"`
	Value     string   `json:"value,omitempty"`
	Children  []string `json:"children,omitempty"`
	Child     string   `json:"child,omitempty"`
	Disabled  bool     `json:"disabled,omitempty"`
	Action    *Action  `json:"action,omitempty"`
}

type Action struct {
	Event ActionEvent `json:"event"`
}

type ActionEvent struct {
	Name string `json:"name"`
}

type ApprovalSurfaceRequest struct {
	SurfaceID    string
	ApprovalID   string
	Title        string
	Description  string
	RiskTone     string
	ToolName     string
	ToolCallID   string
	ApproveLabel string
	RejectLabel  string
}

func ApprovalSurfaceJSON(request ApprovalSurfaceRequest) (string, error) {
	surface := ApprovalSurface(request)
	data, err := json.Marshal(surface)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ApprovalSurface(request ApprovalSurfaceRequest) Surface {
	surfaceID := firstNonEmpty(request.SurfaceID, ApprovalSurfaceID)
	title := firstNonEmpty(request.Title, "Approval required")
	description := firstNonEmpty(request.Description, "Review this request in Awaken before continuing.")
	tone := normalizeTone(request.RiskTone)
	toolName := firstNonEmpty(request.ToolName, "unknown")
	toolCallID := firstNonEmpty(request.ToolCallID, "unknown")
	approvalID := firstNonEmpty(request.ApprovalID, "unknown")
	approveLabel := firstNonEmpty(request.ApproveLabel, "Approve")
	rejectLabel := firstNonEmpty(request.RejectLabel, "Reject")

	return Surface{
		Version: Version,
		Metadata: SurfaceMetadata{
			Source:   "awaken-runtime",
			ReadOnly: true,
		},
		UpdateComponents: ComponentUpdate{
			SurfaceID: surfaceID,
			Components: []Component{
				{
					ID:        "root",
					Component: "Card",
					Children:  []string{"title", "risk", "description", "tool", "tool-call", "approval", "actions-note", "actions"},
				},
				{
					ID:        "title",
					Component: "Text",
					Text:      title,
					Variant:   "title",
				},
				{
					ID:        "risk",
					Component: "Badge",
					Text:      strings.ToUpper(tone),
					Tone:      tone,
				},
				{
					ID:        "description",
					Component: "Text",
					Text:      description,
					Variant:   "body",
				},
				{
					ID:        "tool",
					Component: "KeyValue",
					Label:     "Tool",
					Value:     toolName,
				},
				{
					ID:        "tool-call",
					Component: "KeyValue",
					Label:     "Tool call",
					Value:     toolCallID,
				},
				{
					ID:        "approval",
					Component: "KeyValue",
					Label:     "Approval",
					Value:     approvalID,
				},
				{
					ID:        "actions-note",
					Component: "Text",
					Text:      "This generated surface is read-only in Phase 5; decisions still use Awaken ApprovalCard.",
					Variant:   "caption",
				},
				{
					ID:        "actions",
					Component: "Row",
					Children:  []string{"reject", "approve"},
				},
				{
					ID:        "reject",
					Component: "Button",
					Child:     "reject-label",
					Disabled:  true,
					Action:    &Action{Event: ActionEvent{Name: "approval.reject"}},
				},
				{
					ID:        "reject-label",
					Component: "Text",
					Text:      rejectLabel,
				},
				{
					ID:        "approve",
					Component: "Button",
					Child:     "approve-label",
					Disabled:  true,
					Action:    &Action{Event: ActionEvent{Name: "approval.approve"}},
				},
				{
					ID:        "approve-label",
					Component: "Text",
					Text:      approveLabel,
				},
			},
		},
	}
}

func normalizeTone(value string) string {
	switch strings.TrimSpace(value) {
	case "safe", "danger":
		return strings.TrimSpace(value)
	default:
		return "warning"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
