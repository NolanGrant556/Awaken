package skills

import (
	"awaken/src/mcpclient"
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
	"time"
)

type bodyPromptSession struct {
	mcpclient.Session
	called      string
	resourceURI string
	args        map[string]string
}

func (s *bodyPromptSession) GetPrompt(_ context.Context, p *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
	s.called = p.Name
	s.args = p.Arguments
	return &mcp.GetPromptResult{Description: "Short summary", Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "ACTUAL INSTRUCTIONS"}}}}, nil
}

type bodyBridgeManager struct {
	McpBridgeManager
	s *bodyPromptSession
}

func (m bodyBridgeManager) Session(string) (mcpclient.Session, error) { return m.s, nil }
func TestPromptBodyUsesMessages(t *testing.T) {
	s := &bodyPromptSession{}
	b := NewBridge(nil, bodyBridgeManager{s: s})
	body, e := b.FetchBody(context.Background(), McpBinding{ServerName: "srv", PromptName: "prompt", Kind: "prompt"}, map[string]string{"name": "Ada"})
	if e != nil {
		t.Fatal(e)
	}
	if s.args["name"] != "Ada" {
		t.Fatal("prompt arguments were not forwarded")
	}
	if body != "ACTUAL INSTRUCTIONS" {
		t.Fatalf("prompt body=%q; messages omitted", body)
	}
}
func TestResourceUsesResourceRead(t *testing.T) {
	s := &bodyPromptSession{}
	b := NewBridge(nil, bodyBridgeManager{s: s})
	def := b.resourceToSkill("srv", &mcp.Resource{URI: "audit://templates/checklist", Name: "checklist", MIMEType: "text/x-prompt"}, "")
	body, err := def.BodyFetcher.FetchBody(context.Background(), *def.McpBinding, nil)
	if err != nil || body != "RESOURCE BODY" || s.resourceURI != "audit://templates/checklist" {
		t.Fatalf("resource fetch failed: %q %q %v", body, s.resourceURI, err)
	}
	if s.called != "" {
		t.Fatalf("resource called prompts/get name=%q instead of resources/read for original URI", s.called)
	}
}

func (s *bodyPromptSession) ReadResource(_ context.Context, p *mcp.ReadResourceParams) (*mcp.ReadResourceResult, error) {
	s.resourceURI = p.URI
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: p.URI, Text: "RESOURCE BODY"}}}, nil
}

type startupBridgeManager struct {
	McpBridgeManager
	session    *bodyPromptSession
	subscribed chan struct{}
}

func (m startupBridgeManager) Session(string) (mcpclient.Session, error) { return m.session, nil }
func (m startupBridgeManager) Snapshot() []mcpclient.ServerSnapshot {
	return []mcpclient.ServerSnapshot{{Name: "ready", State: mcpclient.StateConnected}}
}
func (m startupBridgeManager) Subscribe(ctx context.Context) <-chan mcpclient.ServerEvent {
	close(m.subscribed)
	return make(chan mcpclient.ServerEvent)
}
func (s *bodyPromptSession) ListPrompts(context.Context, *mcp.ListPromptsParams) (*mcp.ListPromptsResult, error) {
	return &mcp.ListPromptsResult{Prompts: []*mcp.Prompt{{Name: "startup"}}}, nil
}
func (s *bodyPromptSession) ListResources(context.Context, *mcp.ListResourcesParams) (*mcp.ListResourcesResult, error) {
	return &mcp.ListResourcesResult{}, nil
}
func TestBridgeStartsWithAlreadyConnectedServers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := startupBridgeManager{session: &bodyPromptSession{}, subscribed: make(chan struct{})}
	registry := NewSkillRegistry(nil)
	bridge := NewBridge(nil, manager)
	done := make(chan struct{})
	go func() { bridge.Start(ctx, manager, registry); close(done) }()
	<-manager.subscribed
	deadline := time.Now().Add(2 * time.Second)
	for registry.Get("mcp__ready__startup") == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if registry.Get("mcp__ready__startup") == nil {
		t.Fatal("already connected server never synchronized")
	}
	cancel()
	<-done
}
