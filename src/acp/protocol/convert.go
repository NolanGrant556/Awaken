package protocol

import (
	"fmt"
	"strings"

	"awaken/src/acp"

	acpsdk "github.com/coder/acp-go-sdk"
)

func fromSDKCapabilities(capabilities acpsdk.AgentCapabilities) acp.AgentCapabilitiesSnapshot {
	s := acp.AgentCapabilitiesSnapshot{
		SupportsTextPrompt: true, // baseline per ACP spec
	}
	if capabilities.PromptCapabilities.Image {
		s.SupportsImagePrompt = true
	}
	if capabilities.PromptCapabilities.Audio {
		s.SupportsAudioPrompt = true
	}
	if capabilities.PromptCapabilities.EmbeddedContext {
		s.SupportsEmbeddedContext = true
	}
	return s
}

func fromSDKPermissionRequest(req acpsdk.RequestPermissionRequest) acp.PermissionRequest {
	r := acp.PermissionRequest{
		ACPSessionID: string(req.SessionId),
		ToolCallID:   string(req.ToolCall.ToolCallId),
	}
	if req.ToolCall.Title != nil {
		r.Title = *req.ToolCall.Title
	}
	if req.ToolCall.Kind != nil {
		r.Kind = string(*req.ToolCall.Kind)
	}
	for _, opt := range req.Options {
		r.Options = append(r.Options, acp.PermissionOption{
			OptionID: string(opt.OptionId),
			Name:     opt.Name,
			Kind:     string(opt.Kind),
		})
	}
	return r
}

func fromSDKSessionUpdate(n acpsdk.SessionNotification) (acp.SessionUpdate, bool) {
	u := n.Update
	switch {
	case u.AgentMessageChunk != nil:
		return acp.SessionUpdate{
			SessionID: string(n.SessionId),
			Kind:      acp.SessionUpdateKindAgentText,
			TextDelta: extractText(u.AgentMessageChunk.Content),
		}, true
	case u.AgentThoughtChunk != nil:
		return acp.SessionUpdate{
			SessionID: string(n.SessionId),
			Kind:      acp.SessionUpdateKindAgentThought,
			TextDelta: extractText(u.AgentThoughtChunk.Content),
		}, true
	case u.ToolCall != nil:
		return acp.SessionUpdate{
			SessionID: string(n.SessionId),
			Kind:      acp.SessionUpdateKindToolCall,
			ToolCall:  toolCallSnapshotFromSDK(u.ToolCall),
		}, true
	case u.ToolCallUpdate != nil:
		return acp.SessionUpdate{
			SessionID: string(n.SessionId),
			Kind:      acp.SessionUpdateKindToolCallUpdate,
			ToolCall:  toolCallUpdateSnapshotFromSDK(u.ToolCallUpdate),
		}, true
	case u.Plan != nil:
		return acp.SessionUpdate{
			SessionID: string(n.SessionId),
			Kind:      acp.SessionUpdateKindPlan,
			Plan:      planFromSDK(u.Plan.Entries),
		}, true
	}
	return acp.SessionUpdate{}, false
}

func extractText(block acpsdk.ContentBlock) string {
	if block.Text != nil {
		return block.Text.Text
	}
	return ""
}

func toolCallSnapshotFromSDK(t *acpsdk.SessionUpdateToolCall) *acp.ToolCallSnapshot {
	s := acp.ToolCallSnapshot{
		ToolCallID: string(t.ToolCallId),
		Title:      t.Title,
		Kind:       string(t.Kind),
		Status:     string(t.Status),
	}
	for _, loc := range t.Locations {
		s.Locations = append(s.Locations, loc.Path)
	}
	for _, c := range t.Content {
		if c.Content != nil && c.Content.Content.Text != nil {
			s.Summary += c.Content.Content.Text.Text
		}
	}
	s.Summary = strings.TrimSpace(s.Summary)
	return &s
}

func toolCallUpdateSnapshotFromSDK(t *acpsdk.SessionToolCallUpdate) *acp.ToolCallSnapshot {
	s := acp.ToolCallSnapshot{
		ToolCallID: string(t.ToolCallId),
	}
	if t.Title != nil {
		s.Title = *t.Title
	}
	if t.Kind != nil {
		s.Kind = string(*t.Kind)
	}
	if t.Status != nil {
		s.Status = string(*t.Status)
	}
	for _, loc := range t.Locations {
		s.Locations = append(s.Locations, loc.Path)
	}
	for _, c := range t.Content {
		if c.Content != nil && c.Content.Content.Text != nil {
			s.Summary += c.Content.Content.Text.Text
		}
	}
	s.Summary = strings.TrimSpace(s.Summary)
	return &s
}

func planFromSDK(entries []acpsdk.PlanEntry) []acp.PlanEntry {
	out := make([]acp.PlanEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, acp.PlanEntry{
			Title:  e.Content,
			Status: string(e.Status),
		})
	}
	return out
}

// safePreview returns a length-limited, control-character-stripped preview.
func safePreview(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func unsupportedError(method string) error {
	return fmt.Errorf("%s: %s", acp.ErrCodeCapabilityUnsupported, method)
}
