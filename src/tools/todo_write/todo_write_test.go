package todowrite

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestTodoWriteParameterSchemaIncludesItemsAndStatusEnum(t *testing.T) {
	schema := (&TodoWriteTool{}).ParameterSchema()
	if schema.Type != "object" {
		t.Fatalf("expected object schema, got %q", schema.Type)
	}
	if !reflect.DeepEqual(schema.Required, []string{"items"}) {
		t.Fatalf("expected items to be required, got %+v", schema.Required)
	}

	items, ok := schema.Properties["items"]
	if !ok {
		t.Fatal("expected items property")
	}
	if items.Type != "array" || items.Items == nil {
		t.Fatalf("expected items array schema, got %+v", items)
	}
	if !reflect.DeepEqual(items.Items.Required, []string{"id", "content", "status"}) {
		t.Fatalf("expected item required fields, got %+v", items.Items.Required)
	}

	status, ok := items.Items.Properties["status"]
	if !ok {
		t.Fatal("expected item status property")
	}
	wantStatuses := []string{statusPending, statusInProgress, statusCompleted, statusCancelled}
	if !reflect.DeepEqual(status.Enum, wantStatuses) {
		t.Fatalf("expected status enum %+v, got %+v", wantStatuses, status.Enum)
	}
}

func TestTodoWriteExecuteReturnsStructuredSummary(t *testing.T) {
	output, err := (&TodoWriteTool{}).Execute(context.Background(), `{
		"items": [
			{"id": "1", "content": "Inspect tool runtime", "status": "completed"},
			{"id": "2", "content": "Add todo_write", "status": "in_progress", "priority": "high"},
			{"id": "3", "content": "Run tests", "status": "pending"}
		]
	}`)
	if err != nil {
		t.Fatalf("expected todo_write to succeed: %v", err)
	}

	var summary todoWriteSummary
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		t.Fatalf("expected JSON summary, got %q: %v", output, err)
	}
	if summary.ArtifactID != todoArtifactID || summary.Total != 3 {
		t.Fatalf("unexpected summary identity/counts: %+v", summary)
	}
	if summary.Counts.Completed != 1 || summary.Counts.InProgress != 1 || summary.Counts.Pending != 1 || summary.Counts.Cancelled != 0 {
		t.Fatalf("unexpected status counts: %+v", summary.Counts)
	}
	if len(summary.InProgress) != 1 || summary.InProgress[0].ID != "2" || summary.InProgress[0].Priority != "high" {
		t.Fatalf("unexpected in-progress summary: %+v", summary.InProgress)
	}
	if len(summary.NextPending) != 1 || summary.NextPending[0].ID != "3" {
		t.Fatalf("unexpected pending summary: %+v", summary.NextPending)
	}
}

func TestTodoWriteRejectsInvalidParameters(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "non object", input: `[{"id":"1","content":"x","status":"pending"}]`, want: "JSON 对象"},
		{name: "missing items", input: `{}`, want: "items"},
		{name: "empty id", input: `{"items":[{"id":" ","content":"x","status":"pending"}]}`, want: "id"},
		{name: "empty content", input: `{"items":[{"id":"1","content":" ","status":"pending"}]}`, want: "content"},
		{name: "duplicate id", input: `{"items":[{"id":"1","content":"x","status":"pending"},{"id":"1","content":"y","status":"completed"}]}`, want: "重复"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&TodoWriteTool{}).Execute(context.Background(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestTodoWriteRejectsInvalidStatus(t *testing.T) {
	_, err := (&TodoWriteTool{}).Execute(context.Background(), `{"items":[{"id":"1","content":"x","status":"blocked"}]}`)
	if err == nil || !strings.Contains(err.Error(), "pending") || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected status validation error, got %v", err)
	}
}

func TestTodoWriteArtifactsContainFullPlanPayload(t *testing.T) {
	input := `{"items":[{"id":"1","content":"Write tests","status":"pending","priority":"medium"}]}`

	artifacts := (&TodoWriteTool{}).Artifacts(input, "", tool.NewPermissionContext(t.TempDir(), nil))
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact, got %+v", artifacts)
	}

	artifact := artifacts[0]
	if artifact.Action != tool.ArtifactActionUpdated {
		t.Fatalf("expected updated action, got %q", artifact.Action)
	}
	if artifact.ID != todoArtifactID || artifact.Kind != "todo" || artifact.Title == "" {
		t.Fatalf("unexpected artifact identity: %+v", artifact)
	}
	if artifact.Path != "" {
		t.Fatalf("todo artifact should not point at a file path, got %q", artifact.Path)
	}
	if artifact.Status != "ready" {
		t.Fatalf("expected ready artifact, got %q", artifact.Status)
	}

	var payload todoArtifactPayload
	if err := json.Unmarshal([]byte(artifact.Content), &payload); err != nil {
		t.Fatalf("expected artifact JSON payload, got %q: %v", artifact.Content, err)
	}
	if payload.SchemaVersion != 1 || payload.Kind != "todo" {
		t.Fatalf("unexpected artifact payload metadata: %+v", payload)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "1" || payload.Items[0].Priority != "medium" {
		t.Fatalf("unexpected artifact items: %+v", payload.Items)
	}
	if payload.Counts.Pending != 1 || payload.Counts.Completed != 0 {
		t.Fatalf("unexpected artifact counts: %+v", payload.Counts)
	}
}
