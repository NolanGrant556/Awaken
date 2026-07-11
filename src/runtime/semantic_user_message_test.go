package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

func TestStructuredUserInputSurvivesDurableServiceRestart(t *testing.T) {
	setRuntimeUserDataDir(t)
	workspace := t.TempDir()
	filePath := filepath.Join(t.TempDir(), "acceptance.txt")
	if err := os.WriteFile(filePath, []byte("attachment content"), 0o600); err != nil {
		t.Fatalf("write file attachment: %v", err)
	}

	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected model path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()

	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				MaxThoughts: 1,
				ModelConfig: agentcore.ModelConfig{
					APIKey:  "test-key",
					BaseURL: model.URL,
					Model:   "test-model",
				},
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return agent.RunStreamWithEvents(ctx, input, sink)
			})
		},
	)

	token := ComposerToken{
		ID:    "prompt:acceptance:user",
		Type:  "prompt",
		Label: "acceptance",
		Payload: ComposerTokenPayload{
			SkillID:      "acceptance",
			SourceOrigin: "user",
		},
	}
	run, err := service.StartRun(StartRunRequest{
		Input: "inspect structured inputs",
		Attachments: []RunInputAttachment{
			{
				ID:          "image-1",
				Kind:        RunInputAttachmentKindImage,
				MimeType:    "image/png",
				DataBase64:  "aGVsbG8=",
				DisplayName: "clipboard.png",
				Width:       20,
				Height:      10,
			},
			{
				ID:          "file-1",
				Kind:        RunInputAttachmentKindFile,
				MimeType:    "text/plain",
				Path:        filePath,
				DisplayName: "acceptance.txt",
			},
		},
		ComposerTokens: []ComposerToken{token},
		Workspace:      WorkspaceConfig{WorkspaceRoot: workspace},
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	restarted := NewRunService(nil, nil)
	transcript, err := restarted.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript after restart: %v", err)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("expected one durable run, got %+v", transcript.Runs)
	}
	replayed := transcript.Runs[0].Run
	if replayed.Input != "inspect structured inputs" || len(replayed.InputAttachments) != 2 {
		t.Fatalf("structured input missing after restart: %+v", replayed)
	}
	if replayed.InputAttachments[0].ID != "image-1" || replayed.InputAttachments[0].DataBase64 != "aGVsbG8=" ||
		replayed.InputAttachments[1].ID != "file-1" || replayed.InputAttachments[1].Path != filePath {
		t.Fatalf("attachment identity/order changed after restart: %+v", replayed.InputAttachments)
	}
	if len(replayed.ComposerTokens) != 1 || replayed.ComposerTokens[0] != token {
		t.Fatalf("composer token changed after restart: %+v", replayed.ComposerTokens)
	}
}

func TestV1AdapterPreservesStructuredUserInputForDurableRun(t *testing.T) {
	now := time.Date(2026, 7, 11, 1, 2, 3, 0, time.UTC)
	token := ComposerToken{
		ID:    "skill:docs:user",
		Type:  "skill",
		Label: "docs",
		Payload: ComposerTokenPayload{
			SkillID:       "docs",
			SourceOrigin:  "user",
			SkillFilePath: "/tmp/skills/docs/SKILL.md",
		},
	}
	records := projectV1TranscriptRecords([]TranscriptRecord{{
		RecordType: transcriptRecordTypeMessage,
		ThreadID:   "thread-v1-input",
		RunID:      "run-v1-input",
		MessageID:  "message-v1-input",
		Role:       string(convcontext.RoleUser),
		Content:    "read attachment",
		Attachments: []transcriptMessageAttachment{{
			ID:          "file-v1",
			Kind:        RunInputAttachmentKindFile,
			MimeType:    "text/plain",
			Path:        "/tmp/v1-attachment.txt",
			DisplayName: "v1-attachment.txt",
		}},
		ComposerTokens: []ComposerToken{token},
		Timestamp:      now,
	}})

	run := durableRunsFromSemanticRecords("thread-v1-input", records)["run-v1-input"].Run
	if len(run.InputAttachments) != 1 || run.InputAttachments[0].ID != "file-v1" {
		t.Fatalf("v1 attachment lost through adapter: %+v", run.InputAttachments)
	}
	if len(run.ComposerTokens) != 1 || run.ComposerTokens[0] != token {
		t.Fatalf("v1 composer token lost through adapter: %+v", run.ComposerTokens)
	}
}

func TestLegacyV2AttachmentCountDoesNotInventStructuredInput(t *testing.T) {
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindUserMessage,
		"thread-old-v2",
		"run-old-v2",
		"",
		"",
		"",
		map[string]any{
			"content":          "legacy text",
			"role":             "user",
			"attachment_count": 2,
		},
	)

	run := durableRunsFromSemanticRecords("thread-old-v2", []agentcore.SemanticRecord{record})["run-old-v2"].Run
	if run.Input != "legacy text" {
		t.Fatalf("legacy v2 text changed: %+v", run)
	}
	if len(run.InputAttachments) != 0 || len(run.ComposerTokens) != 0 {
		t.Fatalf("legacy count-only record invented structured input: %+v", run)
	}
}

func TestDurableRunKeepsImageIdentityWhenOwnedBlobIsMissing(t *testing.T) {
	setRuntimeUserDataDir(t)
	record := agentcore.NewSemanticRecord(
		agentcore.SemanticKindUserMessage,
		"thread-missing-blob",
		"run-missing-blob",
		"",
		"",
		"",
		map[string]any{
			"content": "inspect image",
			"attachments": []agentcore.SemanticUserMessageAttachment{{
				ID:          "image-missing",
				Kind:        "image",
				MimeType:    "image/png",
				DisplayName: "missing.png",
				StorageKey:  "thread-missing-blob/image-missing.png",
			}},
		},
	)

	run := durableRunsFromSemanticRecords("thread-missing-blob", []agentcore.SemanticRecord{record})["run-missing-blob"].Run
	if len(run.InputAttachments) != 1 || run.InputAttachments[0].ID != "image-missing" || run.InputAttachments[0].DataBase64 != "" {
		t.Fatalf("durable image identity was erased or fabricated: %+v", run.InputAttachments)
	}
	if messages := messagesFromSemanticRecords([]agentcore.SemanticRecord{record}); len(messages) != 1 || len(messages[0].Attachments) != 0 {
		t.Fatalf("missing image blob must not be injected into model history: %+v", messages)
	}
}
