package awaysummary

import (
	stdctx "context"
	"errors"
	"fmt"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestBuildRequestEmptyTranscriptDoesNotRequest(t *testing.T) {
	if _, ok := BuildRequest(nil, Options{}); ok {
		t.Fatal("expected empty transcript to skip request construction")
	}

	calls := 0
	result := Generate(stdctx.Background(), nil, Options{}, func(stdctx.Context, Request) (Response, error) {
		calls++
		return Response{Content: "unused"}, nil
	})
	if result != nil {
		t.Fatalf("expected nil result for empty transcript, got %+v", result)
	}
	if calls != 0 {
		t.Fatalf("expected runner not to be called, got %d calls", calls)
	}
}

func TestBuildRequestUsesRecentThirtyMessages(t *testing.T) {
	messages := make([]convcontext.Message, 35)
	for i := range messages {
		messages[i] = convcontext.Message{
			Role:    convcontext.RoleUser,
			Content: fmt.Sprintf("message-%02d", i+1),
		}
	}

	req, ok := BuildRequest(messages, Options{})
	if !ok {
		t.Fatal("expected request")
	}
	if len(req.Messages) != DefaultRecentMessageWindow+1 {
		t.Fatalf("expected 30 recent messages plus prompt, got %d", len(req.Messages))
	}
	if req.Messages[0].Content != "message-06" {
		t.Fatalf("expected first retained message-06, got %q", req.Messages[0].Content)
	}
	if req.Messages[29].Content != "message-35" {
		t.Fatalf("expected last transcript message-35, got %q", req.Messages[29].Content)
	}
	if req.Messages[30].Role != convcontext.RoleUser || !strings.Contains(req.Messages[30].Content, "The user stepped away") {
		t.Fatalf("expected appended away-summary user prompt, got %+v", req.Messages[30])
	}
	if len(messages) != 35 {
		t.Fatalf("expected original transcript untouched, got %d messages", len(messages))
	}
}

func TestBuildRequestInjectsSessionMemoryBlock(t *testing.T) {
	req, ok := BuildRequest(oneMessage(), Options{SessionMemory: "User prefers short Chinese closeouts."})
	if !ok {
		t.Fatal("expected request")
	}

	prompt := req.Messages[len(req.Messages)-1].Content
	if !strings.HasPrefix(prompt, "Session memory (broader context):\nUser prefers short Chinese closeouts.\n\n") {
		t.Fatalf("expected session memory block at prompt start, got %q", prompt)
	}
	if !strings.Contains(prompt, "The user stepped away and is coming back.") {
		t.Fatalf("expected away summary instruction after memory block, got %q", prompt)
	}
}

func TestBuildPromptWording(t *testing.T) {
	prompt := BuildPrompt("")
	for _, want := range []string{
		"Write exactly 1-3 short sentences",
		"high-level task",
		"what they are building or debugging",
		"concrete next step",
		"Skip status reports and commit recaps",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestGenerateReturnsNilOnAbortErrorOrAPIError(t *testing.T) {
	t.Run("context aborted", func(t *testing.T) {
		ctx, cancel := stdctx.WithCancel(stdctx.Background())
		cancel()

		result := Generate(ctx, oneMessage(), Options{}, func(stdctx.Context, Request) (Response, error) {
			t.Fatal("runner should not be called after context cancellation")
			return Response{}, nil
		})
		if result != nil {
			t.Fatalf("expected nil result, got %+v", result)
		}
	})

	t.Run("runner abort", func(t *testing.T) {
		result := Generate(stdctx.Background(), oneMessage(), Options{}, func(stdctx.Context, Request) (Response, error) {
			return Response{}, ErrAborted
		})
		if result != nil {
			t.Fatalf("expected nil result, got %+v", result)
		}
	})

	t.Run("runner error", func(t *testing.T) {
		result := Generate(stdctx.Background(), oneMessage(), Options{}, func(stdctx.Context, Request) (Response, error) {
			return Response{}, errors.New("network failed")
		})
		if result != nil {
			t.Fatalf("expected nil result, got %+v", result)
		}
	})

	t.Run("api error message", func(t *testing.T) {
		result := Generate(stdctx.Background(), oneMessage(), Options{}, func(stdctx.Context, Request) (Response, error) {
			return Response{APIError: true, Content: "provider error"}, nil
		})
		if result != nil {
			t.Fatalf("expected nil result, got %+v", result)
		}
	})
}

func TestCleanSummaryLimitsToThreeSentencesAndTrimsStatusPrefix(t *testing.T) {
	raw := "Current status update after inspecting the repository and test output: Awaken is adding away summary. The next step is to wire the request. Keep frontend unchanged. This extra sentence should drop."
	got := CleanSummary(raw)
	want := "Awaken is adding away summary. The next step is to wire the request. Keep frontend unchanged."
	if got != want {
		t.Fatalf("unexpected cleaned summary:\nwant: %q\n got: %q", want, got)
	}

	chinese := CleanSummary("进度汇报：正在实现离开后回来摘要。下一步是接入调用点。第四句会被丢掉。不要保留。")
	if chinese != "正在实现离开后回来摘要。 下一步是接入调用点。 第四句会被丢掉。" {
		t.Fatalf("unexpected Chinese cleanup: %q", chinese)
	}
}

func TestBuildRequestSetsModelCacheAndToolMarkers(t *testing.T) {
	req, ok := BuildRequest(oneMessage(), Options{})
	if !ok {
		t.Fatal("expected request")
	}

	if req.Model != SmallFastModelMarker {
		t.Fatalf("expected small-fast model marker %q, got %q", SmallFastModelMarker, req.Model)
	}
	if !req.SkipCacheWrite {
		t.Fatal("expected skip cache write marker")
	}
	if !req.ThinkingDisabled {
		t.Fatal("expected thinking disabled marker")
	}
	if len(req.Tools) != 0 || len(req.MCPTools) != 0 || len(req.Agents) != 0 || req.ToolChoice != "" {
		t.Fatalf("expected tool/agent options disabled, got %+v", req)
	}
	if req.QuerySource != QuerySourceAwaySummary {
		t.Fatalf("expected query source marker %q, got %q", QuerySourceAwaySummary, req.QuerySource)
	}
	if req.SystemPrompt != "" || req.NonInteractiveSession || req.HasAppendSystemPrompt {
		t.Fatalf("unexpected request shell options: %+v", req)
	}
}

func TestGenerateCleansSuccessfulSummary(t *testing.T) {
	result := Generate(stdctx.Background(), oneMessage(), Options{}, func(stdctx.Context, Request) (Response, error) {
		return Response{Content: "Status report: Building the away summary core. Next step is to call it from runtime. Extra stays. Fourth drops."}, nil
	})
	if result == nil {
		t.Fatal("expected result")
	}
	want := "Building the away summary core. Next step is to call it from runtime. Extra stays."
	if result.Summary != want {
		t.Fatalf("unexpected summary:\nwant: %q\n got: %q", want, result.Summary)
	}
}

func oneMessage() []convcontext.Message {
	return []convcontext.Message{{
		Role:    convcontext.RoleUser,
		Content: "continue the run",
	}}
}
