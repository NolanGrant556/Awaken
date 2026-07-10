package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/skills"
	"awaken/src/skillruntime"
	"awaken/src/tool"
	_ "awaken/src/tools"
)

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

func TestPreActivateComposerSkillsBeforeLLM(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "deck")
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0755); err != nil {
		t.Fatal(err)
	}
	body := "preload recipes"
	sum := sha256.Sum256([]byte(body))
	digest := hex.EncodeToString(sum[:])
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
    sha256: `+digest+`
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
			if cfg.ResolvedToolPool == nil {
				t.Fatal("expected resolved tool pool before LLM")
			}
			hasReadFile := false
			for _, name := range cfg.ResolvedToolPool {
				if name == "shell" {
					t.Fatalf("shell must not remain in resolved pool: %v", cfg.ResolvedToolPool)
				}
				if name == "read_file" {
					hasReadFile = true
				}
			}
			if !hasReadFile {
				t.Fatalf("expected read_file in resolved pool: %v", cfg.ResolvedToolPool)
			}
			if len(cfg.SkillContextBlocks) == 0 || !strings.Contains(cfg.SkillContextBlocks[0].Content, "Host-loaded skill body") {
				t.Fatalf("missing skill context: %+v", cfg.SkillContextBlocks)
			}
			if !strings.Contains(cfg.SkillContextBlocks[0].Content, "preload recipes") {
				t.Fatal("preload resource missing from context")
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
			if rec.Payload["sha256"] != digest {
				t.Fatalf("receipt=%v", rec.Payload)
			}
		}
	}
	if !sawActivated || !sawReceipt {
		t.Fatalf("missing semantic records activated=%v receipt=%v records=%d", sawActivated, sawReceipt, len(recorder.records))
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
			if def.ID == "local-only" && def.State == skills.StateAvailable {
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
	if a.Body == b.Body {
		t.Fatal("project skill bodies leaked across workspaces")
	}
	_ = tool.CapabilityArtifactRead
}
