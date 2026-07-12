package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/skillruntime"
	"awaken/src/skills"
)

func TestComposerForkSkillRunsChildBeforeParentWithoutContextLeak(t *testing.T) {
	var (
		requestMu sync.Mutex
		requests  []map[string]any
	)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode model request: %v", err)
		}
		requestMu.Lock()
		requests = append(requests, request)
		index := len(requests)
		requestMu.Unlock()
		content := "child-complete"
		if index == 2 {
			content = "parent-complete"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", content)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()

	skillRoot := t.TempDir()
	packageRoot := filepath.Join(skillRoot, "fork-composer")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte(`---
name: fork-composer
description: Execute separately.
context: fork
agent: Researcher
---
CHILD_ONLY_INSTRUCTION
`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceProject: skillRoot})
	service := NewRunService(nil, nil)
	service.allowNoopRecorder = true
	service.SetSkillActivator(skillruntime.NewActivator(registry, nil))

	workspace := t.TempDir()
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
		MaxThoughts: 1,
		ModelConfig: agentcore.ModelConfig{APIKey: "test-key", BaseURL: model.URL, Model: "test-model"},
		Workspace:   agentcore.WorkspaceConfig{Root: workspace, ReadableRoots: []string{workspace}, WritableRoots: []string{workspace}},
	}, "thread_composer_fork")
	recorder := &capturingRecorder{}
	agent.SetSemanticRecorder(recorder)
	agent.SetRunIdentity("run_composer_fork", "session_composer_fork")
	session := NewSession("run_composer_fork", "thread_composer_fork", "research this", nil, []ComposerToken{{
		ID: "skill:fork-composer:project", Type: "skill",
		Payload: ComposerTokenPayload{
			SkillID:       "fork-composer",
			SourceOrigin:  "project",
			RootDir:       packageRoot,
			SkillFilePath: filepath.Join(packageRoot, "SKILL.md"),
		},
	}}, func() {})
	executor := ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return agent.RunStreamWithEvents(ctx, input, sink)
	})
	service.execute(session, executor, agent, "research this", "research this", nil, context.Background())
	if snapshot := session.Snapshot(); snapshot.Status != RunStatusFinished || snapshot.Result != "parent-complete" {
		t.Fatalf("run=%+v", snapshot)
	}

	requestMu.Lock()
	defer requestMu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("model request count=%d", len(requests))
	}
	encodedChild, _ := json.Marshal(requests[0])
	encodedParent, _ := json.Marshal(requests[1])
	childSystemPrompt, _ := providerSystemAndLastUserContent(t, requests[0])
	parentSystemPrompt, parentUserContent := providerSystemAndLastUserContent(t, requests[1])
	if !strings.Contains(string(encodedChild), "CHILD_ONLY_INSTRUCTION") {
		t.Fatalf("child request missing fork body: %s", encodedChild)
	}
	if strings.Contains(childSystemPrompt, "<awaken_host_composer_skill_selection>") {
		t.Fatalf("composer selection leaked into fork child identity: %s", encodedChild)
	}
	if strings.Contains(string(encodedParent), "CHILD_ONLY_INSTRUCTION") || !strings.Contains(string(encodedParent), "child-complete") {
		t.Fatalf("parent must receive only child result: %s", encodedParent)
	}
	for _, expected := range []string{
		`"run_id":"run_composer_fork"`,
		`"invocation":"user_composer"`,
		`"selected_skill_id":"fork-composer"`,
		`"skill_id":"fork-composer"`,
		`"source":"project"`,
		`"activation_mode":"fork"`,
		`"root_dir":"` + packageRoot + `"`,
		`"skill_md_path":"` + filepath.Join(packageRoot, "SKILL.md") + `"`,
	} {
		if !strings.Contains(parentSystemPrompt, expected) {
			t.Fatalf("parent request missing explicit fork selection %q: %s", expected, encodedParent)
		}
	}
	if parentUserContent != "research this" {
		t.Fatalf("fork parent user content changed: %q", parentUserContent)
	}
	if selection := agent.GetConfig().ComposerSkillSelection; selection != nil {
		t.Fatalf("fork composer selection leaked after run: %+v", selection)
	}

	runTerminals := 0
	childAssistant := false
	for _, record := range recorder.records {
		if record.Kind == agentcore.SemanticKindRunTerminal {
			runTerminals++
		}
		if record.Kind == agentcore.SemanticKindAssistantMessage && strings.TrimSpace(fmt.Sprint(record.Payload["agent_id"])) != "" {
			childAssistant = true
		}
	}
	if runTerminals != 1 || !childAssistant {
		t.Fatalf("expected one parent terminal and tagged child trace: terminals=%d child=%v records=%+v", runTerminals, childAssistant, recorder.records)
	}
	if messages := messagesFromSemanticRecords(recorder.records); strings.Contains(fmt.Sprint(messages), "child-complete") {
		t.Fatalf("child conversation leaked into restored parent messages: %+v", messages)
	}
}
