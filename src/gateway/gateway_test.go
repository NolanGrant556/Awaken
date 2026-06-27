package gateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDeliveryTarget(t *testing.T) {
	origin := &SessionSource{
		Platform: PlatformDingTalk,
		ChatID:   "origin-chat",
		ThreadID: "origin-thread",
	}

	tests := []struct {
		name       string
		raw        string
		origin     *SessionSource
		platform   Platform
		chatID     string
		threadID   string
		isOrigin   bool
		isExplicit bool
	}{
		{name: "local", raw: "local", platform: PlatformLocal},
		{name: "origin", raw: "origin", origin: origin, platform: PlatformDingTalk, chatID: "origin-chat", threadID: "origin-thread", isOrigin: true},
		{name: "platform home", raw: "dingtalk", platform: PlatformDingTalk},
		{name: "platform chat", raw: "dingtalk:ChatCase", platform: PlatformDingTalk, chatID: "ChatCase", isExplicit: true},
		{name: "platform chat thread", raw: "dingtalk:ChatCase:ThreadCase", platform: PlatformDingTalk, chatID: "ChatCase", threadID: "ThreadCase", isExplicit: true},
		{name: "unknown platform fallback", raw: "unknown:chat", platform: PlatformLocal},
		{name: "empty explicit fallback", raw: "dingtalk:", platform: PlatformLocal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := ParseDeliveryTarget(tt.raw, tt.origin)
			if target.Platform != tt.platform {
				t.Fatalf("expected platform %s, got %s", tt.platform, target.Platform)
			}
			if target.ChatID != tt.chatID {
				t.Fatalf("expected chat ID %q, got %q", tt.chatID, target.ChatID)
			}
			if target.ThreadID != tt.threadID {
				t.Fatalf("expected thread ID %q, got %q", tt.threadID, target.ThreadID)
			}
			if target.IsOrigin != tt.isOrigin {
				t.Fatalf("expected IsOrigin %v, got %v", tt.isOrigin, target.IsOrigin)
			}
			if target.IsExplicit != tt.isExplicit {
				t.Fatalf("expected IsExplicit %v, got %v", tt.isExplicit, target.IsExplicit)
			}
		})
	}
}

func TestParseDeliveryTargetOriginFallback(t *testing.T) {
	target := ParseDeliveryTarget("origin", nil)
	if target.Platform != PlatformLocal || !target.IsOrigin {
		t.Fatalf("expected origin without source to fall back to local, got %+v", target)
	}

	unknown := &SessionSource{Platform: Platform("mystery"), ChatID: "chat-1"}
	target = ParseDeliveryTarget("origin", unknown)
	if target.Platform != PlatformLocal || !target.IsOrigin {
		t.Fatalf("expected unknown origin platform to fall back to local, got %+v", target)
	}

	missingChat := &SessionSource{Platform: PlatformDingTalk}
	target = ParseDeliveryTarget("origin", missingChat)
	if target.Platform != PlatformLocal || !target.IsOrigin {
		t.Fatalf("expected origin without chat ID to fall back to local, got %+v", target)
	}
}

func TestDeliveryRouterLocalDelivery(t *testing.T) {
	dir := t.TempDir()
	router := NewDeliveryRouter(
		WithOutputDir(dir),
		WithClock(fixedClock),
	)

	results := router.Deliver(context.Background(), "full local content", []DeliveryTarget{
		ParseDeliveryTarget("local", nil),
	}, DeliveryOptions{
		JobID:   "../bad job",
		JobName: "Local Report",
		Metadata: map[string]string{
			"visible": "yes",
			"token":   "secret-token",
		},
	})

	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	result := results[0]
	if !result.Success {
		t.Fatalf("expected local delivery success, got %+v", result)
	}
	rel, err := filepath.Rel(dir, result.LocalPath)
	if err != nil {
		t.Fatalf("Rel returned error: %v", err)
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Fatalf("local path escaped output dir: %s", result.LocalPath)
	}

	data, err := os.ReadFile(result.LocalPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# Local Report") || !strings.Contains(text, "full local content") || !strings.Contains(text, "**visible:** yes") {
		t.Fatalf("local output missing expected content:\n%s", text)
	}
	if strings.Contains(text, "secret-token") || !strings.Contains(text, "**token:** [redacted]") {
		t.Fatalf("local output did not redact sensitive metadata:\n%s", text)
	}
}

func TestDeliveryRouterLocalDeliveryDoesNotOverwriteSameTimestampOutput(t *testing.T) {
	dir := t.TempDir()
	router := NewDeliveryRouter(
		WithOutputDir(dir),
		WithClock(fixedClock),
	)

	first := router.Deliver(context.Background(), "first content", []DeliveryTarget{
		ParseDeliveryTarget("local", nil),
	}, DeliveryOptions{JobID: "job-1"})
	second := router.Deliver(context.Background(), "second content", []DeliveryTarget{
		ParseDeliveryTarget("local", nil),
	}, DeliveryOptions{JobID: "job-1"})

	if len(first) != 1 || len(second) != 1 || !first[0].Success || !second[0].Success {
		t.Fatalf("expected both deliveries to succeed, first=%+v second=%+v", first, second)
	}
	if first[0].LocalPath == second[0].LocalPath {
		t.Fatalf("expected distinct local output paths, got %q", first[0].LocalPath)
	}

	firstData, err := os.ReadFile(first[0].LocalPath)
	if err != nil {
		t.Fatalf("ReadFile first returned error: %v", err)
	}
	secondData, err := os.ReadFile(second[0].LocalPath)
	if err != nil {
		t.Fatalf("ReadFile second returned error: %v", err)
	}
	if !strings.Contains(string(firstData), "first content") || !strings.Contains(string(secondData), "second content") {
		t.Fatalf("outputs were not preserved separately:\nfirst=%s\nsecond=%s", firstData, secondData)
	}
}

func TestDeliveryRouterPlatformTruncationSavesFullOutput(t *testing.T) {
	dir := t.TempDir()
	adapter := &fakeAdapter{}
	router := NewDeliveryRouter(
		WithOutputDir(dir),
		WithAdapter(PlatformDingTalk, adapter),
		WithMaxPlatformOutputChars(90),
		WithClock(fixedClock),
	)
	content := strings.Repeat("A", 200)

	results := router.Deliver(context.Background(), content, []DeliveryTarget{
		ParseDeliveryTarget("dingtalk:chat-123:thread-456", nil),
	}, DeliveryOptions{JobID: "job-1"})

	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	result := results[0]
	if !result.Success || !result.Truncated {
		t.Fatalf("expected successful truncated delivery, got %+v", result)
	}
	if result.FullOutputPath == "" {
		t.Fatalf("expected full output path")
	}
	full, err := os.ReadFile(result.FullOutputPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(full) != content {
		t.Fatalf("expected full output to be saved verbatim")
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("expected one adapter call, got %d", len(adapter.calls))
	}
	call := adapter.calls[0]
	if call.target.ChatID != "chat-123" || call.target.ThreadID != "thread-456" {
		t.Fatalf("adapter received wrong target: %+v", call.target)
	}
	if runeCount(call.content) > 90 {
		t.Fatalf("expected adapter content <= 90 chars, got %d: %q", runeCount(call.content), call.content)
	}
	if !strings.Contains(call.content, "truncated") || !strings.Contains(call.content, filepath.Base(result.FullOutputPath)) {
		t.Fatalf("adapter content missing truncation reference: %q", call.content)
	}
	if call.metadata["thread_id"] != "thread-456" {
		t.Fatalf("expected thread_id metadata, got %+v", call.metadata)
	}
}

func TestDeliveryRouterAdapterErrorIsolation(t *testing.T) {
	dir := t.TempDir()
	adapter := &fakeAdapter{err: errors.New("send failed token=super-secret")}
	router := NewDeliveryRouter(
		WithOutputDir(dir),
		WithAdapter(PlatformDingTalk, adapter),
		WithClock(fixedClock),
	)

	results := router.Deliver(context.Background(), "content", []DeliveryTarget{
		ParseDeliveryTarget("dingtalk:chat-123", nil),
		ParseDeliveryTarget("local", nil),
	}, DeliveryOptions{JobID: "job-1"})

	if len(results) != 2 {
		t.Fatalf("expected two results, got %d", len(results))
	}
	if results[0].Success {
		t.Fatalf("expected adapter delivery to fail")
	}
	if strings.Contains(results[0].Error, "super-secret") || !strings.Contains(results[0].Error, "[redacted]") {
		t.Fatalf("expected adapter error to redact token, got %q", results[0].Error)
	}
	if !results[1].Success || results[1].LocalPath == "" {
		t.Fatalf("expected local delivery to continue after adapter error, got %+v", results[1])
	}
}

func TestBuildSessionContextPromptRedactsPIIWithoutMutatingRoutingIDs(t *testing.T) {
	source := SessionSource{
		Platform: PlatformDingTalk,
		ChatID:   "chat-raw-123",
		ChatType: "group",
		UserID:   "user-raw-456",
	}
	context := SessionContext{
		Source:             source,
		ConnectedPlatforms: []Platform{PlatformDingTalk, PlatformFeishu},
		HomeChannels: map[Platform]HomeChannel{
			PlatformDingTalk: {ChatID: "home-raw-789", Name: "ops"},
		},
	}

	prompt := BuildSessionContextPrompt(context, true)
	for _, raw := range []string{"chat-raw-123", "user-raw-456", "home-raw-789"} {
		if strings.Contains(prompt, raw) {
			t.Fatalf("prompt leaked raw ID %q:\n%s", raw, prompt)
		}
	}
	if !strings.Contains(prompt, "user_") || !strings.Contains(prompt, hashChatID("chat-raw-123")) || !strings.Contains(prompt, hashChatID("home-raw-789")) {
		t.Fatalf("prompt missing hashed IDs:\n%s", prompt)
	}

	origin := ParseDeliveryTarget("origin", &source)
	if origin.ChatID != "chat-raw-123" {
		t.Fatalf("expected routing target to retain raw chat ID, got %+v", origin)
	}
	if source.ChatID != "chat-raw-123" || source.UserID != "user-raw-456" {
		t.Fatalf("source IDs were mutated: %+v", source)
	}
}

func TestDeliveryRouterUsesHomeChannelForPlatformTarget(t *testing.T) {
	adapter := &fakeAdapter{}
	router := NewDeliveryRouter(
		WithOutputDir(t.TempDir()),
		WithAdapter(PlatformFeishu, adapter),
		WithHomeChannels(map[Platform]HomeChannel{
			PlatformFeishu: {ChatID: "home-chat", ThreadID: "home-thread", Name: "home"},
		}),
		WithClock(fixedClock),
	)

	results := router.Deliver(context.Background(), "content", []DeliveryTarget{
		ParseDeliveryTarget("feishu", nil),
	}, DeliveryOptions{})

	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected home channel delivery success, got %+v", results)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("expected one adapter call, got %d", len(adapter.calls))
	}
	call := adapter.calls[0]
	if call.target.ChatID != "home-chat" || call.metadata["thread_id"] != "home-thread" {
		t.Fatalf("home channel was not resolved, got target=%+v metadata=%+v", call.target, call.metadata)
	}
}

func fixedClock() time.Time {
	return time.Date(2026, 6, 28, 12, 34, 56, 123456789, time.UTC)
}

type fakeAdapter struct {
	calls []adapterCall
	err   error
}

func (a *fakeAdapter) Send(ctx context.Context, target DeliveryTarget, content string, metadata map[string]string) (AdapterSendResult, error) {
	if ctx.Err() != nil {
		return AdapterSendResult{}, ctx.Err()
	}
	a.calls = append(a.calls, adapterCall{
		target:   target,
		content:  content,
		metadata: copyMetadata(metadata),
	})
	if a.err != nil {
		return AdapterSendResult{}, a.err
	}
	return AdapterSendResult{MessageID: "message-1"}, nil
}

type adapterCall struct {
	target   DeliveryTarget
	content  string
	metadata map[string]string
}
