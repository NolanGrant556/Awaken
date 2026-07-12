package agentcore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/skills"
)

func TestAgentRefreshRunAvailabilityCarriesTaskAndSkillMetadataWithoutLoadingSkill(t *testing.T) {
	workspace := t.TempDir()
	temporaryRoot := filepath.Join(t.TempDir(), "task-temporary-root")
	projectSkillRoot := filepath.Join(workspace, ".awaken", "skills")
	packageRoot := filepath.Join(projectSkillRoot, "must-not-run")
	if err := os.MkdirAll(filepath.Join(packageRoot, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workspace, "skill-script-executed")
	if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte("---\nname: must-not-run\ndescription: metadata only\n---\nNever preload or execute this package during environment probing.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "scripts", "run.sh"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	// Keep the test independent from the developer's real configured roots and
	// shell files. The project/user defaults remain metadata only.
	t.Setenv(skills.EnvManagedSkillDirs, filepath.Join(t.TempDir(), "managed"))
	t.Setenv(skills.EnvAdditionalSkillDirs, filepath.Join(t.TempDir(), "additional"))
	t.Setenv(skills.EnvPluginSkillDirs, filepath.Join(t.TempDir(), "plugin"))
	t.Setenv(skills.EnvBundledSkillDirs, "")
	t.Setenv(skills.EnvLegacyCommandDirs, "")
	t.Setenv("SHELL", "/unsupported/test-shell")

	agent := NewAgent(AgentConfig{Workspace: WorkspaceConfig{
		ScopeID: "environment-snapshot-test", Root: workspace, TemporaryWorkRoot: temporaryRoot,
	}})
	agent.RefreshRunAvailabilityWithReason(context.Background(), "test_refresh")
	config := agent.GetConfig()
	if config.ExecutionEnvironmentSnapshot == nil || config.ToolAvailabilitySnapshot == nil {
		t.Fatal("agent refresh did not install environment/tool snapshots")
	}
	environment := config.ExecutionEnvironmentSnapshot
	if environment.RefreshReason != "test_refresh" || environment.TemporaryWorkRoot != temporaryRoot {
		t.Fatalf("run metadata was not propagated: %+v", environment)
	}
	foundProjectRoot := false
	for _, root := range environment.SkillRoots {
		if root.Source == string(skills.SourceProject) && root.Path == projectSkillRoot {
			foundProjectRoot = true
			break
		}
	}
	if !foundProjectRoot {
		t.Fatalf("project Skill root metadata missing: %+v", environment.SkillRoots)
	}
	envelope := agent.reqBuilder.BuildContextEnvelope(agent.conversation.Context, nil)
	if len(envelope.SystemContextItems) != 1 ||
		!strings.Contains(envelope.SystemContextItems[0].Content, "environment_refresh_reason=test_refresh") ||
		!strings.Contains(envelope.SystemContextItems[0].Content, "temporary_work_root="+temporaryRoot) ||
		!strings.Contains(envelope.SystemContextItems[0].Content, "project="+projectSkillRoot) {
		t.Fatalf("environment metadata did not reach the runtime snapshot projection: %+v", envelope.SystemContextItems)
	}
	if len(config.SkillContextBlocks) != 0 {
		t.Fatalf("environment refresh preloaded Skill context: %+v", config.SkillContextBlocks)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("environment refresh executed a Skill script: %v", err)
	}

	previousEnvironment := environment
	agent.RefreshRunAvailability(context.Background())
	refreshedEnvironment := agent.GetConfig().ExecutionEnvironmentSnapshot
	if reason := refreshedEnvironment.RefreshReason; reason != "explicit_refresh" {
		t.Fatalf("legacy explicit refresh wrapper lost provenance: %q", reason)
	}
	if refreshedEnvironment == previousEnvironment || previousEnvironment.RefreshReason != "test_refresh" {
		t.Fatalf("refresh mutated or reused the prior immutable snapshot: previous=%p refreshed=%p previous_reason=%q", previousEnvironment, refreshedEnvironment, previousEnvironment.RefreshReason)
	}
}
