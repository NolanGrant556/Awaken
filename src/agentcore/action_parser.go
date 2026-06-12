package agentcore

import (
	"fmt"
	"strings"
)

type ParsedAction struct {
	ActionType string
	ToolName   string
	Input      string
}

func ParseAction(response string) (ParsedAction, error) {
	result := ParsedAction{}

	thoughtStart := strings.Index(response, "<thought>")
	thoughtEnd := strings.Index(response, "</thought>")
	if thoughtStart != -1 && thoughtEnd != -1 {
		result.ActionType = "thought"
	}

	actionStart := strings.Index(response, "<action>")
	actionEnd := strings.Index(response, "</action>")
	if actionStart == -1 || actionEnd == -1 {
		return result, fmt.Errorf("invalid action format")
	}

	actionContent := response[actionStart+8 : actionEnd]

	nameStart := strings.Index(actionContent, "<name>")
	nameEnd := strings.Index(actionContent, "</name>")
	if nameStart == -1 || nameEnd == -1 {
		return result, fmt.Errorf("missing action name")
	}
	result.ToolName = strings.TrimSpace(actionContent[nameStart+6 : nameEnd])

	inputStart := strings.Index(actionContent, "<input>")
	inputEnd := strings.Index(actionContent, "</input>")
	if inputStart == -1 || inputEnd == -1 {
		return result, fmt.Errorf("missing action input")
	}
	result.Input = strings.TrimSpace(actionContent[inputStart+7 : inputEnd])

	result.ActionType = "action"
	return result, nil
}
