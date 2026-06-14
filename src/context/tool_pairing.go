package context

import "fmt"

func ValidateToolResultPairing(messages []Message) error {
	pending := map[string]string{}

	for i, msg := range messages {
		switch msg.Role {
		case RoleAssistant:
			if len(pending) > 0 {
				return fmt.Errorf("missing tool result before assistant message at index %d: %s", i, pendingToolIDs(pending))
			}
			for _, tc := range msg.ToolCalls {
				if tc.ID == "" {
					return fmt.Errorf("assistant message at index %d has tool call with empty id", i)
				}
				if _, exists := pending[tc.ID]; exists {
					return fmt.Errorf("assistant message at index %d has duplicate tool call id %q", i, tc.ID)
				}
				pending[tc.ID] = tc.Function.Name
			}
		case RoleTool:
			if len(pending) == 0 {
				return fmt.Errorf("orphan tool result at index %d with id %q", i, msg.ToolCallID)
			}
			if msg.ToolCallID == "" {
				return fmt.Errorf("tool result at index %d has empty tool_call_id", i)
			}
			expectedName, ok := pending[msg.ToolCallID]
			if !ok {
				return fmt.Errorf("tool result at index %d references unknown tool_call_id %q", i, msg.ToolCallID)
			}
			if msg.Name != "" && expectedName != "" && msg.Name != expectedName {
				return fmt.Errorf("tool result at index %d has name %q, expected %q", i, msg.Name, expectedName)
			}
			delete(pending, msg.ToolCallID)
		default:
			if len(pending) > 0 {
				return fmt.Errorf("missing tool result before %s message at index %d: %s", msg.Role, i, pendingToolIDs(pending))
			}
		}
	}

	if len(pending) > 0 {
		return fmt.Errorf("missing tool result at end of messages: %s", pendingToolIDs(pending))
	}
	return nil
}

func pendingToolIDs(pending map[string]string) string {
	ids := ""
	for id := range pending {
		if ids != "" {
			ids += ", "
		}
		ids += id
	}
	return ids
}
