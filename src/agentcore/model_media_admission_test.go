package agentcore

import (
	stdcontext "context"
	"strconv"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

type memoryModelContentStore struct {
	payloads map[string]string
}

func (s *memoryModelContentStore) PersistToolResult(_ stdcontext.Context, _ string, _ tool.ToolExecutionIdentity, parts []tool.ModelContentPart) ([]tool.ModelContentPart, error) {
	return tool.CloneModelContent(parts), nil
}

func (s *memoryModelContentStore) HydrateToolResult(_ stdcontext.Context, _ string, parts []tool.ModelContentPart) ([]tool.ModelContentPart, []tool.OutcomeNotice) {
	out := tool.CloneModelContent(parts)
	for i := range out {
		if strings.TrimSpace(out[i].DataBase64) != "" || s == nil {
			continue
		}
		if payload, ok := s.payloads[out[i].StorageKey]; ok {
			out[i].DataBase64 = payload
		}
	}
	return out, nil
}

func TestRequestProjectionCountsOldMediaButHydratesOnlyNearTail(t *testing.T) {
	store := &memoryModelContentStore{payloads: map[string]string{}}
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "read older charts"},
	}
	for i := 1; i <= 11; i++ {
		key := "thread/tool-results/old/part-" + strconv.Itoa(i) + ".png"
		store.payloads[key] = "old-image"
		callID := "old-" + strconv.Itoa(i)
		messages = append(messages,
			convcontext.Message{
				Role: convcontext.RoleAssistant,
				ToolCalls: []convcontext.ToolCall{{
					ID: callID, Type: "function",
					Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"old.png"}`},
				}},
			},
			convcontext.Message{
				Role: convcontext.RoleTool, ToolCallID: callID, Name: "Read",
				Content: "older image",
				ModelContent: []tool.ModelContentPart{{
					Type: "image", MimeType: "image/png", DisplayName: "old-" + strconv.Itoa(i) + ".png",
					StorageKey: key, ByteSize: 12,
				}},
			},
		)
	}
	nearKey := "thread/tool-results/near/part-001.png"
	store.payloads[nearKey] = "near-image"
	messages = append(messages,
		convcontext.Message{Role: convcontext.RoleUser, Content: "read the latest chart"},
		convcontext.Message{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{{
				ID: "near-1", Type: "function",
				Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"near.png"}`},
			}},
		},
		convcontext.Message{
			Role: convcontext.RoleTool, ToolCallID: "near-1", Name: "Read",
			Content: "latest image",
			ModelContent: []tool.ModelContentPart{{
				Type: "image", MimeType: "image/png", DisplayName: "near.png",
				StorageKey: nearKey, ByteSize: 12,
			}},
		},
	)

	projected := projectMessagesForRequest(messages, requestProjectionOptions{
		capabilities: tool.ModelCapabilities{SupportsVision: true},
		windowConfig: ContextWindowConfig{ModelWindow: 40000, OutputReserve: 8000},
		threadID:     "thread",
		store:        store,
	})

	mediaTokens := 0
	hydrated := 0
	for _, message := range projected {
		for _, part := range message.ModelContent {
			mediaTokens += convcontext.EstimateModelContentTokens(part)
			if strings.TrimSpace(part.DataBase64) != "" {
				hydrated++
			}
		}
	}
	if mediaTokens < 12*convcontext.MediaPartTokenEstimate {
		t.Fatalf("expected durable media to stay in the local ledger, got %d tokens", mediaTokens)
	}
	if hydrated != 1 {
		t.Fatalf("expected only the near-tail image to hydrate, got %d", hydrated)
	}
	if last := projected[len(projected)-1]; last.ModelContent[0].DataBase64 != "near-image" {
		t.Fatalf("near-tail image was not hydrated: %+v", last.ModelContent)
	}
	if !strings.Contains(projected[2].Content, mediaStubPrefix) {
		t.Fatalf("older image should become a stub, got %q", projected[2].Content)
	}
}

func TestRequestProjectionRebuildsLiveAndReplayIdentically(t *testing.T) {
	store := &memoryModelContentStore{payloads: map[string]string{
		"thread/tool-results/old/part-001.png":  "old-bytes",
		"thread/tool-results/near/part-001.png": "near-bytes",
	}}
	live := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "older"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{
			ID: "old", Function: convcontext.FunctionCall{Name: "Read"},
		}}},
		{Role: convcontext.RoleTool, ToolCallID: "old", Name: "Read", Content: "old",
			ModelContent: []tool.ModelContentPart{{
				Type: "image", MimeType: "image/png", DisplayName: "old.png",
				StorageKey: "thread/tool-results/old/part-001.png", ByteSize: 9, DataBase64: "old-bytes",
			}}},
		{Role: convcontext.RoleUser, Content: "latest"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{
			ID: "near", Function: convcontext.FunctionCall{Name: "Read"},
		}}},
		{Role: convcontext.RoleTool, ToolCallID: "near", Name: "Read", Content: "near",
			ModelContent: []tool.ModelContentPart{{
				Type: "image", MimeType: "image/png", DisplayName: "near.png",
				StorageKey: "thread/tool-results/near/part-001.png", ByteSize: 10, DataBase64: "near-bytes",
			}}},
	}
	replay := cloneContextMessages(live)
	replay[2].ModelContent[0].DataBase64 = ""
	replay[5].ModelContent[0].DataBase64 = ""

	opts := requestProjectionOptions{
		capabilities: tool.ModelCapabilities{SupportsVision: true},
		windowConfig: ContextWindowConfig{ModelWindow: 40000, OutputReserve: 8000},
		threadID:     "thread",
		store:        store,
	}
	liveProjected := projectMessagesForRequest(live, opts)
	replayProjected := projectMessagesForRequest(replay, opts)
	if liveProjected[2].ModelContent[0].DataBase64 != "" || replayProjected[2].ModelContent[0].DataBase64 != "" {
		t.Fatalf("older media leaked into the provider clone: live=%q replay=%q",
			liveProjected[2].ModelContent[0].DataBase64, replayProjected[2].ModelContent[0].DataBase64)
	}
	if liveProjected[5].ModelContent[0].DataBase64 != "near-bytes" || replayProjected[5].ModelContent[0].DataBase64 != "near-bytes" {
		t.Fatalf("near-tail hydrate diverged: live=%q replay=%q",
			liveProjected[5].ModelContent[0].DataBase64, replayProjected[5].ModelContent[0].DataBase64)
	}
}

func TestRequestBuilderVisionWindowProjectionStubsOlderImages(t *testing.T) {
	ctx := convcontext.New()
	ctx.SetMessages([]convcontext.Message{
		{Role: convcontext.RoleUser, Content: "older", Attachments: []convcontext.MessageAttachment{{
			ID: "old", Kind: "image", MimeType: "image/png", DisplayName: "old.png",
			StorageKey: "old-key", DataBase64: "b2xk", ByteSize: 3,
		}}},
		{Role: convcontext.RoleUser, Content: "latest", Attachments: []convcontext.MessageAttachment{{
			ID: "near", Kind: "image", MimeType: "image/png", DisplayName: "near.png",
			StorageKey: "near-key", DataBase64: "bmVhcg==", ByteSize: 4,
		}}},
	})
	request := NewRequestBuilder(AgentConfig{ModelConfig: ModelConfig{
		Model: "vision-model", ContextWindow: 40000,
		Capabilities: tool.ModelCapabilities{SupportsVision: true},
	}}).Build(ctx, nil)
	if got := request.Messages[0].Attachments[0].DataBase64; got != "" {
		t.Fatalf("older user image stayed provider-visible: %q", got)
	}
	if got := request.Messages[1].Attachments[0].DataBase64; got != "bmVhcg==" {
		t.Fatalf("near-tail user image should stay hydrated: %q", got)
	}
	if original := ctx.GetMessages()[0].Attachments[0].DataBase64; original != "b2xk" {
		t.Fatalf("projection mutated durable conversation media: %q", original)
	}
}
