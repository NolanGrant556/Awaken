package computer

import (
	"context"
	"testing"

	"awaken/src/tool"
)

type outcomeTool interface {
	tool.Tool
	ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error)
}

func TestComputerToolsReturnUnsupported(t *testing.T) {
	ctx := context.Background()
	tools := []outcomeTool{&observeTool{}, &actTool{}, &verifyTool{}}
	for _, tt := range tools {
		outcome, err := tt.ExecuteOutcome(ctx, `{}`, nil)
		if err != nil {
			t.Fatalf("tool %s returned error: %v", tt.Name(), err)
		}
		if outcome.Status != tool.OutcomeUnsupported {
			t.Errorf("tool %s: expected unsupported, got %s", tt.Name(), outcome.Status)
		}
		if outcome.ErrorCode != "computer_unsupported" {
			t.Errorf("tool %s: expected error code computer_unsupported, got %s", tt.Name(), outcome.ErrorCode)
		}
	}
}

func TestComputerToolsRegistered(t *testing.T) {
	for _, name := range []string{ToolNameObserve, ToolNameAct, ToolNameVerify} {
		_, err := tool.GetRegisteredTool(name)
		if err != nil {
			t.Errorf("tool %s should be registered: %v", name, err)
		}
	}
}
