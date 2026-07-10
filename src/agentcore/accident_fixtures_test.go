package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestShellExit1SuccessBaselineFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "shell_exit_1_success_baseline.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		ID                           string `json:"id"`
		ToolName                     string `json:"tool_name"`
		ToolOutputJSON               map[string]any `json:"tool_output_json"`
		CurrentToolRuntimeProjection struct {
			IsError   bool   `json:"is_error"`
			Status    string `json:"status"`
			ErrorCode string `json:"error_code"`
		} `json:"current_tool_runtime_projection"`
		Phase1TargetProjection struct {
			Status          string `json:"status"`
			ExitCode        int    `json:"exit_code"`
			ErrorCode       string `json:"error_code"`
			ProcessStatus   string `json:"process_status"`
			SemanticStatus  string `json:"semantic_status"`
			TransportStatus string `json:"transport_status"`
		} `json:"phase1_target_projection"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fixture.ID != "shell_exit_1_success_baseline" {
		t.Fatalf("unexpected id %q", fixture.ID)
	}
	if fixture.ToolName != "shell" {
		t.Fatalf("unexpected tool %q", fixture.ToolName)
	}
	exitCode, ok := fixture.ToolOutputJSON["exit_code"].(float64)
	if !ok || int(exitCode) != 1 {
		t.Fatalf("expected exit_code=1, got %#v", fixture.ToolOutputJSON["exit_code"])
	}
	if fixture.CurrentToolRuntimeProjection.IsError {
		t.Fatal("baseline must record current IsError=false")
	}
	if fixture.CurrentToolRuntimeProjection.Status != "succeeded" {
		t.Fatalf("baseline status=%q", fixture.CurrentToolRuntimeProjection.Status)
	}
	if fixture.Phase1TargetProjection.Status != "failed" ||
		fixture.Phase1TargetProjection.ErrorCode != "process_exit_nonzero" ||
		fixture.Phase1TargetProjection.ExitCode != 1 {
		t.Fatalf("phase1 target projection mismatch: %+v", fixture.Phase1TargetProjection)
	}
}
