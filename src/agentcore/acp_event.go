package agentcore

import "awaken/src/tool"

// MapACPEvent maps a tool-package neutral ACP event to the canonical
// agentcore.AwakenEvent type so it can flow through the live event stream.
func MapACPEvent(e tool.ACPEvent) AwakenEvent {
	event := AwakenEvent{
		Type:          mapACPEventType(e.Type),
		Timestamp:     timeNowUTC(),
		ThreadID:      e.ThreadID,
		RunID:         e.RunID,
		ToolCallID:    e.ToolCallID,
		InteractionID: e.InteractionID,
		ToolName:      e.ToolName,
		Content:       e.Content,
		Diagnostic:    cloneDiagnostic(e.Diagnostic),
	}
	if event.Diagnostic == nil {
		event.Diagnostic = make(map[string]any)
	}
	if e.Status != "" {
		event.Diagnostic["status"] = e.Status
	}
	return event
}

func mapACPEventType(t tool.ACPEventType) AwakenEventType {
	switch t {
	case tool.ACPEventTypeSessionStarted:
		return ACPSessionStartedEventType
	case tool.ACPEventTypeSessionClosed:
		return ACPSessionClosedEventType
	case tool.ACPEventTypeSessionInterrupted:
		return ACPSessionInterruptedEventType
	case tool.ACPEventTypeTurnStarted:
		return ACPTurnStartedEventType
	case tool.ACPEventTypeTurnFinished:
		return ACPTurnFinishedEventType
	case tool.ACPEventTypeToolSnapshot:
		return ACPToolSnapshotEventType
	case tool.ACPEventTypePermissionRequested:
		return ACPPermissionRequestedEventType
	case tool.ACPEventTypePermissionResolved:
		return ACPPermissionResolvedEventType
	default:
		return AwakenEventType(string(t))
	}
}

func cloneDiagnostic(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
