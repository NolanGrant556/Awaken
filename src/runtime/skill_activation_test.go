package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	_ "awaken/src/tools"
)

func TestComposerMultiSkillActivationPersistenceIsAtomic(t *testing.T) {
	root := t.TempDir()
	for _, skillID := range []string{"first-skill", "second-skill"} {
		skillRoot := filepath.Join(root, skillID)
		if err := os.MkdirAll(skillRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		definition := "---\nname: " + skillID + "\ndescription: atomic activation\ntoolPolicy:\n  allow: [read_file]\n---\nBody\n"
		if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(definition), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	service := NewRunService(nil, nil)
	service.allowNoopRecorder = true
	service.SetSkillActivator(skillruntime.NewActivator(registry, nil))
	recorder := &multiActivationFailRecorder{}
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, "thread_multi_skill")
	agent.SetSemanticRecorder(recorder)
	agent.SetRunIdentity("run_multi_skill", "session_multi_skill")
	session := NewSession("run_multi_skill", "thread_multi_skill", "use both", nil, []ComposerToken{
		{ID: "skill:first-skill:user", Type: "skill", Payload: ComposerTokenPayload{SkillID: "first-skill"}},
		{ID: "skill:second-skill:user", Type: "skill", Payload: ComposerTokenPayload{SkillID: "second-skill"}},
	}, func() {})
	executed := false
	service.execute(session, ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
		executed = true
		return "must not execute", nil
	}), agent, "use both", "use both", nil, context.Background())

	if executed {
		t.Fatal("executor ran after activation journal batch failed")
	}
	if recorder.activationBatches != 1 {
		t.Fatalf("multi-skill activations were not submitted as one batch: %d", recorder.activationBatches)
	}
	if session.Snapshot().Status != RunStatusError || session.Snapshot().Error == "" {
		t.Fatalf("expected durable activation failure, got %+v", session.Snapshot())
	}
	want := []agentcore.SemanticKind{agentcore.SemanticKindUserMessage, agentcore.SemanticKindRunStarted, agentcore.SemanticKindRunTerminal}
	if len(recorder.records) != len(want) {
		t.Fatalf("partial activation records escaped failed batch: %+v", recorder.records)
	}
	for i, kind := range want {
		if recorder.records[i].Kind != kind {
			t.Fatalf("record[%d]=%s want %s", i, recorder.records[i].Kind, kind)
		}
	}
}

func TestComposerSkillHintsDoNotChangeDefaultToolExposure(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "hidden-patch")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: hidden-patch
description: must not inherit hidden tools
toolPolicy:
  allow: [apply_patch]
requiredTools: [apply_patch]
---
Body
`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	service := NewRunService(nil, nil)
	service.SetSkillActivator(skillruntime.NewActivator(registry, nil))
	recorder := &capturingRecorder{}
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, "thread_hidden_patch")
	agent.SetSemanticRecorder(recorder)
	agent.SetRunIdentity("run_hidden_patch", "session_hidden_patch")
	snapshot := NewSession("run_hidden_patch", "thread_hidden_patch", "use hidden patch", nil, []ComposerToken{{
		ID:   "skill:hidden-patch:user",
		Type: "skill",
		Payload: ComposerTokenPayload{
			SkillID: "hidden-patch",
		},
	}}, func() {}).Snapshot()

	err := service.preActivateComposerSkills(context.Background(), agent, snapshot)
	if err != nil {
		t.Fatalf("allowed-tools hints must not reject activation: %v", err)
	}
	if blocks := agent.GetConfig().SkillContextBlocks; len(blocks) != 1 || !strings.Contains(blocks[0].Content, "Body") {
		t.Fatalf("Skill body not injected: %+v", blocks)
	}
	for _, name := range agent.ModelVisibleToolPool() {
		if name == "apply_patch" {
			t.Fatalf("apply_patch unexpectedly visible in default production exposure: %v", agent.ModelVisibleToolPool())
		}
	}
}

type capturingRecorder struct {
	records []agentcore.SemanticRecord
}

func (c *capturingRecorder) Append(_ context.Context, record agentcore.SemanticRecord) error {
	c.records = append(c.records, record)
	return nil
}
func (c *capturingRecorder) AppendBatch(_ context.Context, records []agentcore.SemanticRecord) error {
	c.records = append(c.records, records...)
	return nil
}
func (c *capturingRecorder) Flush(context.Context) error { return nil }

type multiActivationFailRecorder struct {
	records           []agentcore.SemanticRecord
	activationBatches int
}

func (r *multiActivationFailRecorder) Append(ctx context.Context, record agentcore.SemanticRecord) error {
	return r.AppendBatch(ctx, []agentcore.SemanticRecord{record})
}

func (r *multiActivationFailRecorder) AppendBatch(_ context.Context, records []agentcore.SemanticRecord) error {
	activationCount := 0
	for _, record := range records {
		if record.Kind == agentcore.SemanticKindSkillActivated {
			activationCount++
		}
	}
	if activationCount > 0 {
		r.activationBatches++
		if activationCount >= 2 {
			return errors.New("injected atomic activation batch failure")
		}
	}
	r.records = append(r.records, records...)
	return nil
}

func (r *multiActivationFailRecorder) Flush(context.Context) error { return nil }

func TestPreActivateComposerSkillsBeforeLLM(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "deck")
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0755); err != nil {
		t.Fatal(err)
	}
	body := "on-demand recipes"
	if err := os.WriteFile(filepath.Join(skillRoot, "references", "recipes.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: deck
description: slides
version: "9"
toolPolicy:
  allow: [read_file, text_edit]
requiredTools: [read_file]
requiredCapabilities: [artifact_read]
requiredResources:
  - path: references/recipes.md
    required: true
    mode: preload
---
Host-loaded skill body.
`), 0644); err != nil {
		t.Fatal(err)
	}

	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	activator := skillruntime.NewActivator(reg, nil)

	service := NewRunService(func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
		return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
	}, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			cfg := agent.GetConfig()
			if len(cfg.SkillContextBlocks) == 0 || !strings.Contains(cfg.SkillContextBlocks[0].Content, "Host-loaded skill body") {
				t.Fatalf("missing skill context: %+v", cfg.SkillContextBlocks)
			}
			if strings.Contains(cfg.SkillContextBlocks[0].Content, body) {
				t.Fatal("package reference was eagerly preloaded into context")
			}
			return "ok", nil
		})
	})
	service.allowNoopRecorder = true
	service.SetSkillActivator(activator)
	_ = service.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: t.TempDir()})

	recorder := &capturingRecorder{}
	agent := service.createAgent(WorkspaceConfig{WorkspaceRoot: t.TempDir()}, "thread_test", nil)
	agent.SetSemanticRecorder(recorder)
	agent.SetRunIdentity("run_test", "session_test")

	session := NewSession("run_test", "thread_test", "make slides", nil, []ComposerToken{{
		ID:    "skill:deck:user",
		Type:  "skill",
		Label: "deck",
		Payload: ComposerTokenPayload{
			SkillID: "deck",
		},
	}}, func() {})

	service.execute(session, service.newExecutor(agent), agent, "make slides", "make slides", nil, context.Background())
	snap := session.Snapshot()
	if snap.Status != RunStatusFinished {
		t.Fatalf("status=%s error=%s", snap.Status, snap.Error)
	}

	var sawActivated, sawReceipt bool
	for _, rec := range recorder.records {
		switch rec.Kind {
		case agentcore.SemanticKindSkillActivated:
			sawActivated = true
			if rec.Payload["skill_id"] != "deck" {
				t.Fatalf("activated payload=%v", rec.Payload)
			}
		case agentcore.SemanticKindResourceReceipt:
			sawReceipt = true
		}
	}
	if !sawActivated || sawReceipt {
		t.Fatalf("on-demand activation facts activated=%v unexpected_receipt=%v records=%d", sawActivated, sawReceipt, len(recorder.records))
	}
}

func TestComposerMissingDeclaredResourceDoesNotDisableSkillOrUseNetworkFallback(t *testing.T) {
	var networkRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		networkRequests.Add(1)
		http.Error(w, "resource fallback must not be used", http.StatusInternalServerError)
	}))
	defer server.Close()

	root := t.TempDir()
	skillRoot := filepath.Join(root, "missing-resource")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: missing-resource
description: must fail before the first model request
toolPolicy:
  allow: [read_file]
requiredResources:
  - path: references/required.md
    required: true
    mode: preload
  - path: "`+server.URL+`/fallback.md"
    required: true
    mode: preload
---
The package is incomplete. Do not download a replacement from `+server.URL+`.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	service := NewRunService(nil, nil)
	service.allowNoopRecorder = true
	service.SetSkillActivator(skillruntime.NewActivator(registry, nil))
	recorder := &capturingRecorder{}
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, "thread_missing_resource")
	agent.SetSemanticRecorder(recorder)
	agent.SetRunIdentity("run_missing_resource", "session_missing_resource")

	executorCalls := 0
	session := NewSession("run_missing_resource", "thread_missing_resource", "use missing resource", nil, []ComposerToken{{
		ID:   "skill:missing-resource:user",
		Type: "skill",
		Payload: ComposerTokenPayload{
			SkillID: "missing-resource",
		},
	}}, func() {})
	service.execute(session, ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
		executorCalls++
		return "must not reach the model executor", nil
	}), agent, "use missing resource", "use missing resource", nil, context.Background())

	if executorCalls != 1 {
		t.Fatalf("structurally valid Skill should reach executor once, got %d", executorCalls)
	}
	if got := networkRequests.Load(); got != 0 {
		t.Fatalf("missing package resource triggered %d network fallback request(s)", got)
	}
	if snapshot := session.Snapshot(); snapshot.Status != RunStatusFinished {
		t.Fatalf("expected successful activation with execution-time dependency handling, got %+v", snapshot)
	}
}

func TestMarketStylePPTSkillLoadsWithoutPlatformTextConflict(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "legacy-ppt")
	if err := os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "scripts", "generate.py"), []byte("print('legacy')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: legacy-ppt
description: legacy dynamic Python deck generator
---
Run pip install python-pptx, execute scripts/generate.py, then call soffice.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	service := NewRunService(nil, nil)
	service.allowNoopRecorder = true
	service.SetSkillActivator(skillruntime.NewActivator(registry, nil))
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, "thread_legacy_ppt_scope")
	recorder := &capturingRecorder{}
	agent.SetSemanticRecorder(recorder)
	agent.SetRunIdentity("run_legacy_ppt", "session_legacy_ppt")

	executorCalls := 0
	legacyRun := NewSession("run_legacy_ppt", "thread_legacy_ppt_scope", "use legacy ppt", nil, []ComposerToken{{
		ID:   "skill:legacy-ppt:user",
		Type: "skill",
		Payload: ComposerTokenPayload{
			SkillID: "legacy-ppt",
		},
	}}, func() {})
	service.execute(legacyRun, ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
		executorCalls++
		if blocks := agent.GetConfig().SkillContextBlocks; len(blocks) != 1 || !strings.Contains(blocks[0].Content, "soffice") {
			t.Fatalf("market Skill body missing: %+v", blocks)
		}
		return "market skill guidance loaded", nil
	}), agent, "use legacy ppt", "use legacy ppt", nil, context.Background())

	if executorCalls != 1 {
		t.Fatalf("market Skill did not reach executor exactly once: %d", executorCalls)
	}
	if snapshot := legacyRun.Snapshot(); snapshot.Status != RunStatusFinished || snapshot.Result != "market skill guidance loaded" {
		t.Fatalf("market Skill run failed: %+v", snapshot)
	}
}

func TestProjectSkillIsolationAcrossWorkspaces(t *testing.T) {
	base := t.TempDir()
	projectA := filepath.Join(base, "a")
	projectB := filepath.Join(base, "b")
	for _, project := range []string{projectA, projectB} {
		if err := os.MkdirAll(filepath.Join(project, ".awaken", "skills", "local-only"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(project, name, body string) {
		path := filepath.Join(project, ".awaken", "skills", "local-only", "SKILL.md")
		content := "---\nname: local-only\ndescription: " + name + "\n---\n" + body + "\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(projectA, "skill-a", "A body")
	write(projectB, "skill-b", "B body")

	reg := skills.NewSkillRegistry(nil)
	previewA := reg.PreviewLocalSources(skills.LocalSourceDirs(projectA))
	previewB := reg.PreviewLocalSources(skills.LocalSourceDirs(projectB))

	find := func(defs []*skills.SkillDefinition) *skills.SkillDefinition {
		for _, def := range defs {
			if def.ID == "local-only" && def.State == skills.StateLoaded {
				return def
			}
		}
		return nil
	}
	a := find(previewA)
	b := find(previewB)
	if a == nil || b == nil {
		t.Fatalf("missing project skills a=%v b=%v", a != nil, b != nil)
	}
	if a.ProjectPath == b.ProjectPath {
		t.Fatalf("project paths should differ: %q", a.ProjectPath)
	}
	if a.SkillFilePath == b.SkillFilePath || a.ResolvedRootDir == b.ResolvedRootDir {
		t.Fatalf("project package identities should differ: a=%+v b=%+v", a, b)
	}
	if a.Body != "" || b.Body != "" {
		t.Fatal("catalog discovery must not cache either project body")
	}
	regA := skills.NewSkillRegistry(nil)
	regA.LoadFromSources(skills.LocalSourceDirs(projectA))
	regB := skills.NewSkillRegistry(nil)
	regB.LoadFromSources(skills.LocalSourceDirs(projectB))
	planA, err := skillruntime.NewActivator(regA, nil).Activate(skillruntime.ActivationRequest{SkillID: "local-only"})
	if err != nil {
		t.Fatal(err)
	}
	planB, err := skillruntime.NewActivator(regB, nil).Activate(skillruntime.ActivationRequest{SkillID: "local-only"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(planA.MainBody, "A body") || !strings.Contains(planB.MainBody, "B body") {
		t.Fatalf("on-demand bodies crossed projects: A=%q B=%q", planA.MainBody, planB.MainBody)
	}
}

func TestComposerSkillContextDoesNotLeakIntoNextRun(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "read-only")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: read-only
description: read only
toolPolicy:
  allow: [read_file]
---
Read-only body.
`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})

	var seen []agentcore.AgentConfig
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
		SkillContextBlocks: []agentcore.ContextBlock{{
			ID:      "baseline",
			Content: "baseline context",
		}},
	}, "thread_no_leak")
	service := NewRunService(nil, nil)
	service.allowNoopRecorder = true
	service.SetSkillActivator(skillruntime.NewActivator(reg, nil))
	executor := ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
		cfg := agent.GetConfig()
		cfg.SkillContextBlocks = append([]agentcore.ContextBlock(nil), cfg.SkillContextBlocks...)
		cfg.ComposerSkillSelection = cloneAgentComposerSkillSelection(cfg.ComposerSkillSelection)
		seen = append(seen, cfg)
		return "ok", nil
	})

	withSkill := NewSession("run_skill", "thread_no_leak", "use skill", nil, []ComposerToken{{
		ID:   "skill:read-only:user",
		Type: "skill",
		Payload: ComposerTokenPayload{
			SkillID: "read-only",
		},
	}}, func() {})
	service.execute(withSkill, executor, agent, "use skill", "use skill", nil, context.Background())
	if snap := withSkill.Snapshot(); snap.Status != RunStatusFinished {
		t.Fatalf("skill run status=%s error=%s", snap.Status, snap.Error)
	}
	if got := len(agent.GetConfig().SkillContextBlocks); got != 1 {
		t.Fatalf("skill context leaked after run: %d", got)
	}
	if selection := agent.GetConfig().ComposerSkillSelection; selection != nil {
		t.Fatalf("run-scoped composer selection leaked after run: %+v", selection)
	}

	ordinary := NewSession("run_plain", "thread_no_leak", "plain", nil, nil, func() {})
	service.execute(ordinary, executor, agent, "plain", "plain", nil, context.Background())
	if snap := ordinary.Snapshot(); snap.Status != RunStatusFinished {
		t.Fatalf("ordinary run status=%s error=%s", snap.Status, snap.Error)
	}
	if len(seen) != 2 {
		t.Fatalf("executor calls=%d", len(seen))
	}
	if len(seen[0].SkillContextBlocks) != 2 || !strings.Contains(seen[0].SkillContextBlocks[1].Content, "Read-only body") {
		t.Fatalf("activated context=%+v", seen[0].SkillContextBlocks)
	}
	if selection := seen[0].ComposerSkillSelection; selection == nil || selection.RunID != "run_skill" ||
		len(selection.Selections) != 1 || selection.Selections[0].SkillID != "read-only" {
		t.Fatalf("explicit composer selection missing from selected run: %+v", selection)
	}
	if len(seen[1].SkillContextBlocks) != 1 || seen[1].SkillContextBlocks[0].ID != "baseline" {
		t.Fatalf("ordinary context inherited skill block: %+v", seen[1].SkillContextBlocks)
	}
	if selection := seen[1].ComposerSkillSelection; selection != nil {
		t.Fatalf("ordinary run inherited explicit composer selection: %+v", selection)
	}
}

func TestComposerSkillHintsDoNotExpandOrNarrowParentAndActivationIsDurable(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "too-wide")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: too-wide
description: asks for a tool outside the run
toolPolicy:
  allow: [read_file, text_edit]
---
Body.
`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	service := NewRunService(nil, nil)
	service.allowNoopRecorder = true
	service.SetSkillActivator(skillruntime.NewActivator(reg, nil))
	recorder := &capturingRecorder{}
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, "thread_parent_gate")
	agent.SetSemanticRecorder(recorder)
	agent.SetRunIdentity("run_parent_gate", "session_parent_gate")
	executed := false
	session := NewSession("run_parent_gate", "thread_parent_gate", "use skill", nil, []ComposerToken{{
		ID:   "skill:too-wide:user",
		Type: "skill",
		Payload: ComposerTokenPayload{
			SkillID: "too-wide",
		},
	}}, func() {})
	service.execute(session, ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
		executed = true
		return "executed", nil
	}), agent, "use skill", "use skill", nil, context.Background())

	snap := session.Snapshot()
	if snap.Status != RunStatusFinished || snap.Result != "executed" {
		t.Fatalf("status=%s error=%s", snap.Status, snap.Error)
	}
	if !executed {
		t.Fatal("valid Skill did not reach executor")
	}
	if len(recorder.records) != 2 || recorder.records[0].Kind != agentcore.SemanticKindUserMessage || recorder.records[1].Kind != agentcore.SemanticKindSkillActivated {
		t.Fatalf("records=%+v", recorder.records)
	}
	activated := recorder.records[1]
	if activated.Payload["compatibility_status"] != skillruntime.CompatibilityOK {
		t.Fatalf("activation=%v", activated.Payload)
	}
}

func TestComposerSkillActivationIsIsolatedAcrossConcurrentThreads(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "inherit")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: inherit\ndescription: inherit parent\n---\nBody.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	service := NewRunService(nil, nil)
	service.allowNoopRecorder = true
	service.SetSkillActivator(skillruntime.NewActivator(reg, nil))

	type runCase struct {
		thread string
		marker string
	}
	cases := []runCase{{thread: "thread_alpha", marker: "alpha"}, {thread: "thread_beta", marker: "beta"}}
	errCh := make(chan string, len(cases))
	var wg sync.WaitGroup
	for _, tc := range cases {
		tc := tc
		wg.Add(1)
		go func() {
			defer wg.Done()
			agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{SkillContextBlocks: []agentcore.ContextBlock{{ID: tc.marker, Content: tc.marker}}}, tc.thread)
			session := NewSession("run_"+tc.marker, tc.thread, "use skill", nil, []ComposerToken{{
				ID:   "skill:inherit:user",
				Type: "skill",
				Payload: ComposerTokenPayload{
					SkillID: "inherit",
				},
			}}, func() {})
			service.execute(session, ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) {
				blocks := agent.GetConfig().SkillContextBlocks
				if len(blocks) != 2 || blocks[0].ID != tc.marker {
					errCh <- tc.thread + " saw foreign context"
				}
				return "ok", nil
			}), agent, "use skill", "use skill", nil, context.Background())
			if snap := session.Snapshot(); snap.Status != RunStatusFinished {
				errCh <- tc.thread + " status " + string(snap.Status) + ": " + snap.Error
			}
			blocks := agent.GetConfig().SkillContextBlocks
			if len(blocks) != 1 || blocks[0].ID != tc.marker {
				errCh <- tc.thread + " leaked activation context"
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
}
