package tool

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

type catalogTestTool struct {
	name       string
	paramName  string
	paramType  string
	executeErr error
}

func (t catalogTestTool) Name() string {
	return t.name
}

func (t catalogTestTool) Description() string {
	return "test tool"
}

func (t catalogTestTool) Parameters() []ToolParameter {
	if t.paramName == "" {
		return nil
	}
	return []ToolParameter{{Name: t.paramName, Type: t.paramType, Required: true}}
}

func (t catalogTestTool) Execute(ctx context.Context, input string) (string, error) {
	if t.executeErr != nil {
		return "", t.executeErr
	}
	return input, nil
}

func TestRegisterToolRejectsDuplicateName(t *testing.T) {
	candidate := catalogTestTool{name: "catalog_duplicate_tool"}
	if err := RegisterTool(candidate); err != nil {
		t.Fatalf("expected first registration to succeed, got %v", err)
	}

	if err := RegisterTool(candidate); !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool, got %v", err)
	}
}

func TestValidateToolRejectsInvalidNameAndParameterType(t *testing.T) {
	if err := ValidateTool(catalogTestTool{name: "bad name"}); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("expected invalid tool name error, got %v", err)
	}
	if err := ValidateTool(catalogTestTool{name: "valid_name", paramName: "flag", paramType: "bool"}); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("expected invalid parameter type error, got %v", err)
	}
	if err := ValidateTool(catalogTestTool{name: "valid_name", paramName: "bad param", paramType: "string"}); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("expected invalid parameter name error, got %v", err)
	}
}

func TestListToolsIsStableSorted(t *testing.T) {
	first := toolNames(ListTools())
	second := toolNames(ListTools())
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected stable tool order, got %v then %v", first, second)
	}
	if !sort.StringsAreSorted(first) {
		t.Fatalf("expected sorted tool names, got %v", first)
	}
}

func toolNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}
