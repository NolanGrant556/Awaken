package magicdocs

import (
	"encoding/json"
	"fmt"
	"strings"
)

const EditToolName = "Edit"

type ToolPolicyBehavior string

const (
	ToolPolicyAllow ToolPolicyBehavior = "allow"
	ToolPolicyDeny  ToolPolicyBehavior = "deny"
)

type ToolPolicy struct {
	DocPath      string
	EditToolName string
}

type ToolPolicyDecision struct {
	Behavior ToolPolicyBehavior
	Message  string
}

func NewToolPolicy(docPath string) ToolPolicy {
	return ToolPolicy{DocPath: docPath, EditToolName: EditToolName}
}

func CheckToolPolicy(docPath string, toolName string, input any) ToolPolicyDecision {
	return NewToolPolicy(docPath).Check(toolName, input)
}

func (p ToolPolicy) Check(toolName string, input any) ToolPolicyDecision {
	editToolName := firstNonEmpty(p.EditToolName, EditToolName)
	if toolName == editToolName {
		if filePath, ok := inputFilePath(input); ok && filePath == p.DocPath {
			return ToolPolicyDecision{Behavior: ToolPolicyAllow}
		}
	}

	return ToolPolicyDecision{
		Behavior: ToolPolicyDeny,
		Message:  fmt.Sprintf("only %s is allowed for %s", editToolName, p.DocPath),
	}
}

func (d ToolPolicyDecision) Allowed() bool {
	return d.Behavior == ToolPolicyAllow
}

func inputFilePath(input any) (string, bool) {
	switch value := input.(type) {
	case map[string]string:
		filePath, ok := value["file_path"]
		return filePath, ok
	case map[string]any:
		filePath, ok := value["file_path"].(string)
		return filePath, ok
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
			return "", false
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return "", false
		}
		return inputFilePath(parsed)
	default:
		return "", false
	}
}
