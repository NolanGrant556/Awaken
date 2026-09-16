package agentcore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/managedenv"
	"awaken/src/tool/profile"
)

func TestManagedBindingSurvivesProtocolProjectionAndCompaction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer fixture; compiled on Windows")
	}
	body := []byte("#!/bin/sh\nprintf 'fixture 1.0.0\\n'\n")
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, body, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(body)
	catalog := managedenv.Catalog{SchemaVersion: 1, Revision: "test", Components: map[string]managedenv.Component{"fixture": {Version: "1.0.0", Probe: "fixture", Assets: map[string]managedenv.Asset{managedenv.Target(): {URL: (&url.URL{Scheme: "file", Path: path}).String(), SHA256: hex.EncodeToString(hash[:]), Format: "binary", Executables: map[string]string{"fixture": "bin/fixture"}}}}}, Capabilities: map[string]managedenv.Spec{"fixture": {Components: []string{"fixture"}}}}
	m, err := managedenv.New(managedenv.Options{Root: t.TempDir(), Catalog: catalog, AllowFileAssets: true})
	if err != nil {
		t.Fatal(err)
	}
	previous := managedenv.Default()
	managedenv.SetDefault(m)
	defer func() { managedenv.SetDefault(previous); _ = m.Close() }()
	owner := managedenv.Owner{ThreadID: "bound-thread", AgentID: "root", RunID: "bound-run"}
	binding, err := m.Ensure(context.Background(), managedenv.Request{Capability: "fixture"}, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := managedenv.Owner{ThreadID: owner.ThreadID, AgentID: "child", RunID: "child-run"}
	if err = m.Inherit(owner, child); err != nil {
		t.Fatal(err)
	}
	childBindings := m.Bindings(child)
	if len(childBindings) != 1 || childBindings[0].EnvironmentRef == binding.EnvironmentRef {
		t.Fatal("child reused parent identity")
	}
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIChat, llm.ProtocolOpenAIResponses, llm.ProtocolAnthropicMessages} {
		for _, isChild := range []bool{false, true} {
			config := AgentConfig{ModelConfig: ModelConfig{Model: "fixture-model", Protocol: string(protocol)}}
			ref := binding.EnvironmentRef
			if isChild {
				config.SubagentOrigin = &profile.SubagentOrigin{AgentID: "child", ParentAgentID: "root"}
				ref = childBindings[0].EnvironmentRef
			}
			request := NewRequestBuilder(config).BindThread(owner.ThreadID).Build(convcontext.New(), nil)
			prepared, err := llm.PrepareRequest(llm.Config{APIKey: "fixture", BaseURL: "https://example.invalid", Model: "fixture-model", Protocol: protocol}, llm.Request{Model: request.Model, SystemPrompt: request.SystemPrompt, Messages: request.Messages}, true)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(prepared.Body), ref) || (!strings.Contains(string(prepared.Body), "ensure_environment") || !strings.Contains(string(prepared.Body), "load_workspace_dependencies")) {
				t.Fatalf("%s lost binding/rules", protocol)
			}
			if isChild && strings.Contains(string(prepared.Body), binding.EnvironmentRef) {
				t.Fatal("parent reference leaked into child binding context")
			}
		}
	}
	runner := NewRunner(AgentConfig{}, convcontext.NewConversation(nil), NewRequestBuilder(AgentConfig{}))
	runner.threadID = owner.ThreadID
	bundle, err := runner.buildPostCompactContextBundleV2(CompactResult{Mode: CompactModeAuto, BoundaryID: "managed-compact", Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "continue current task", CompactBoundaryID: "managed-compact"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	restored, err := DecodePostCompactContextBundleV2(raw)
	if err != nil || len(restored.EnvironmentBindings) != 1 || restored.EnvironmentBindings[0].Reference != binding.EnvironmentRef {
		t.Fatalf("compact lost durable binding: %v", err)
	}
	restored.EnvironmentBindings[0].InstanceID = "tampered"
	if restored.Validate() == nil {
		t.Fatal("binding was not covered by bundle digest")
	}
}
