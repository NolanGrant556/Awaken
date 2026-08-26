package skillruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/skills"
	"awaken/src/tool"
	tooltrash "awaken/src/tool/trash"
)

type skillInstallTrashAdapter struct{ dir string }

func (a skillInstallTrashAdapter) TrashPath(source string) (tooltrash.TrashResult, error) {
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return tooltrash.TrashResult{}, err
	}
	destination := filepath.Join(a.dir, filepath.Base(source))
	if err := os.Rename(source, destination); err != nil {
		return tooltrash.TrashResult{}, err
	}
	return tooltrash.TrashResult{OriginalPath: source, TrashedPath: destination, Method: "test_rename"}, nil
}

// ---- args_test ----

func TestSubstituteArgsBasic(t *testing.T) {
	body := "Hello ${name}, your task is ${task}."
	argDefs := []SkillArgDef{
		{Name: "name", Type: "string", Required: true},
		{Name: "task", Type: "string", Required: true},
	}
	args := map[string]string{"name": "World", "task": "testing"}
	result, err := SubstituteArgs(body, "test", argDefs, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello World, your task is testing." {
		t.Errorf("bad substitution: %q", result)
	}
}

func TestSubstituteArgsMissingRequired(t *testing.T) {
	body := "Hello ${name}."
	argDefs := []SkillArgDef{
		{Name: "name", Type: "string", Required: true},
	}
	args := map[string]string{}
	_, err := SubstituteArgs(body, "test", argDefs, args)
	if err == nil {
		t.Fatal("expected error for missing required arg")
	}
	if !strings.Contains(err.Error(), "missing required argument") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubstituteArgsDefaultValue(t *testing.T) {
	body := "Window: ${days} days."
	argDefs := []SkillArgDef{
		{Name: "days", Type: "string", Required: false, Default: "7"},
	}
	args := map[string]string{}
	result, err := SubstituteArgs(body, "test", argDefs, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Window: 7 days." {
		t.Errorf("bad default: %q", result)
	}
}

func TestSubstituteArgsBooleanTypeCheck(t *testing.T) {
	body := "Flag: ${flag}."
	argDefs := []SkillArgDef{
		{Name: "flag", Type: "boolean", Required: true},
	}
	// Valid.
	_, err := SubstituteArgs(body, "test", argDefs, map[string]string{"flag": "true"})
	if err != nil {
		t.Fatalf("unexpected error for valid bool: %v", err)
	}
	// Invalid.
	_, err = SubstituteArgs(body, "test", argDefs, map[string]string{"flag": "yes"})
	if err == nil {
		t.Fatal("expected error for invalid boolean")
	}
}

func TestSubstituteArgsEscapedPlaceholders(t *testing.T) {
	body := `Literal \${name} not replaced, but ${name} is.`
	argDefs := []SkillArgDef{
		{Name: "name", Type: "string", Required: true},
	}
	result, err := SubstituteArgs(body, "test", argDefs, map[string]string{"name": "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "${name}") {
		t.Errorf("escaped placeholder should be preserved, got %q", result)
	}
	if !strings.Contains(result, "Alice") {
		t.Errorf("actual placeholder should be replaced, got %q", result)
	}
}

// ---- executor_test ----

func TestInlineExecutor(t *testing.T) {
	e := &InlineExecutor{}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "executed inline") {
		t.Errorf("bad inline result: %s", result)
	}
	if !strings.Contains(result, "do something") {
		t.Errorf("body missing from inline result: %s", result)
	}
}

func TestResourceReadToolReadsWithinSkillPackage(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, ".awaken", "skills", "sample")
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0755); err != nil {
		t.Fatalf("mkdir skill refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "references", "guide.md"), []byte("guide body"), 0644); err != nil {
		t.Fatalf("write guide: %v", err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{
		ID:              "sample",
		Name:            "sample",
		Description:     "sample",
		Source:          skills.SourceProject,
		Scope:           skills.SourceProject,
		RootDir:         skillRoot,
		ResolvedRootDir: mustEvalSymlinks(t, skillRoot),
		ProjectPath:     root,
		State:           skills.StateLoaded,
	})

	reader := &ResourceReadTool{registry: reg}
	if err := tool.ValidateToolWithMode(reader, tool.DefinitionValidationStrictAll); err != nil {
		t.Fatalf("skill_resource_read must pass strict validation: %v", err)
	}
	result, err := reader.Execute(context.Background(), `{"skill_id":"sample","path":"references/guide.md"}`)
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if !strings.Contains(result, "guide body") {
		t.Fatalf("expected guide body, got %s", result)
	}
	outcome, err := reader.ExecuteOutcome(context.Background(), `{"skill_id":"sample","path":"references/guide.md"}`, nil)
	if err != nil {
		t.Fatalf("read structured resource: %v", err)
	}
	definition := tool.ResolveDefinition(reader)
	outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: ResourceReadToolName, OutputSchema: definition.OutputSchema,
		StrictOutputSchema: true, DeclaredSideEffects: definition.SideEffects,
	})
	if outcome.Status != tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("unexpected structured resource outcome: %+v", outcome)
	}
	if outcome.Details["path"] != "references/guide.md" || outcome.Details["size_bytes"] != 10 || outcome.Details["sha256"] == "" {
		t.Fatalf("resource details are incomplete: %+v", outcome.Details)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "references", "asset.bin"), []byte{0xff, 0xfe, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	binaryOutcome, err := reader.ExecuteOutcome(context.Background(), `{"skill_id":"sample","path":"references/asset.bin"}`, nil)
	if err != nil || binaryOutcome.ErrorCode != "skill_resource_binary" {
		t.Fatalf("binary resource should route to file/artifact tools: outcome=%+v err=%v", binaryOutcome, err)
	}
}

func TestResourceListToolDiscoversCompleteSkillPackage(t *testing.T) {
	workspace := t.TempDir()
	sourceRoot := filepath.Join(workspace, ".awaken", "skills")
	skillRoot := filepath.Join(sourceRoot, "sample")
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillRoot, "bulk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: sample\ndescription: List package resources when testing.\n---\nUse references/guide.md.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "references", "guide.md"), []byte("guide body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "assets", "template.txt"), []byte("template"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Keep references/ alphabetically after more than one inventory page. A
	// prefix request must apply its filter before the 512-entry budget.
	for index := 0; index < skills.MaxSkillResourceInventoryEntries; index++ {
		name := fmt.Sprintf("entry-%03d.txt", index)
		if err := os.WriteFile(filepath.Join(skillRoot, "bulk", name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceProject: sourceRoot})
	lister := &ResourceListTool{registry: registry}
	if err := tool.ValidateToolWithMode(lister, tool.DefinitionValidationStrictAll); err != nil {
		t.Fatalf("skill_resource_list must pass strict validation: %v", err)
	}
	outcome, err := lister.ExecuteOutcome(context.Background(), `{"skill_id":"sample","prefix":"references"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	definition := tool.ResolveDefinition(lister)
	outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: ResourceListToolName, OutputSchema: definition.OutputSchema,
		StrictOutputSchema: true, DeclaredSideEffects: definition.SideEffects,
	})
	if outcome.Status != tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("unexpected resource list outcome: %+v", outcome)
	}
	entries, ok := outcome.Details["entries"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("unexpected filtered resource entries: %#v", outcome.Details["entries"])
	}
	first, firstOK := entries[0].(map[string]any)
	second, secondOK := entries[1].(map[string]any)
	if !firstOK || !secondOK || first["path"] != "references" || first["kind"] != "directory" || second["path"] != "references/guide.md" || second["kind"] != "file" {
		t.Fatalf("resource list did not preserve the package tree: %#v", entries)
	}
	if outcome.Details["prefix"] != "references" || outcome.Details["inventory_truncated"] != false {
		t.Fatalf("resource list details incomplete: %+v", outcome.Details)
	}
	fullOutcome, err := lister.ExecuteOutcome(context.Background(), `{"skill_id":"sample"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	fullOutcome = fullOutcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: ResourceListToolName, OutputSchema: definition.OutputSchema,
		StrictOutputSchema: true, DeclaredSideEffects: definition.SideEffects,
	})
	fullEntries, ok := fullOutcome.Details["entries"].([]any)
	if !ok || len(fullEntries) != skills.MaxSkillResourceInventoryEntries || fullOutcome.Details["inventory_truncated"] != true {
		t.Fatalf("full inventory must be bounded and marked truncated: entries=%d details=%+v", len(fullEntries), fullOutcome.Details)
	}

	denied, err := lister.ExecuteOutcome(context.Background(), `{"skill_id":"sample","prefix":"../outside"}`, nil)
	if err != nil || denied.ErrorCode != "skill_resource_list_invalid_input" {
		t.Fatalf("resource list traversal should be rejected: outcome=%+v err=%v", denied, err)
	}
}

func TestResourceReadToolUsesRecordedDiscoveryRootForPluginPackage(t *testing.T) {
	pluginRoot := filepath.Join(t.TempDir(), "packages", "acme.tools")
	sourceRoot := filepath.Join(pluginRoot, "skills")
	packageRoot := filepath.Join(sourceRoot, "plugin-report")
	if err := os.MkdirAll(filepath.Join(packageRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte("---\nname: plugin-report\ndescription: Plugin report.\n---\nUse the asset.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "assets", "template.txt"), []byte("plugin template"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourcePlugin: sourceRoot})
	skillID := "plugin__acme-tools__plugin-report"
	definition := registry.Get(skillID)
	if definition == nil || definition.SourceRoot == "" || definition.State != skills.StateLoaded {
		t.Fatalf("plugin Skill did not retain its discovery root: %+v", definition)
	}
	reader := &ResourceReadTool{registry: registry}
	result, err := reader.Execute(context.Background(), `{"skill_id":"`+skillID+`","path":"assets/template.txt"}`)
	if err != nil || !strings.Contains(result, "plugin template") {
		t.Fatalf("plugin resource did not use recorded local boundary: result=%q err=%v", result, err)
	}
}

func TestResourceReadToolUsesRecordedDiscoveryRootForAgentPackage(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), ".agents", "skills")
	packageRoot := filepath.Join(sourceRoot, "agents-report")
	if err := os.MkdirAll(filepath.Join(packageRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte("---\nname: agents-report\ndescription: agents report skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "assets", "template.txt"), []byte("agent template"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceAgents: sourceRoot})
	definition := registry.Get("agents-report")
	if definition == nil || definition.SourceRoot == "" || definition.State != skills.StateLoaded {
		t.Fatalf("agents Skill did not retain its discovery root: %+v", definition)
	}
	reader := &ResourceReadTool{registry: registry}
	result, err := reader.Execute(context.Background(), `{"skill_id":"agents-report","path":"assets/template.txt"}`)
	if err != nil || !strings.Contains(result, "agent template") {
		t.Fatalf("agents resource did not use recorded local boundary: result=%q err=%v", result, err)
	}
}

func TestResourceReadToolRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, ".awaken", "skills", "sample")
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{
		ID:              "sample",
		Name:            "sample",
		Description:     "sample",
		Source:          skills.SourceProject,
		Scope:           skills.SourceProject,
		RootDir:         skillRoot,
		ResolvedRootDir: skillRoot,
		ProjectPath:     root,
		State:           skills.StateLoaded,
	})

	reader := &ResourceReadTool{registry: reg}
	_, err := reader.Execute(context.Background(), `{"skill_id":"sample","path":"../secret.md"}`)
	if err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestResourceReadToolRejectsSymlinkEscapeAtOpen(t *testing.T) {
	projectRoot := t.TempDir()
	skillRoot := filepath.Join(projectRoot, ".awaken", "skills", "symlink-skill")
	outside := t.TempDir()
	secretPath := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secretPath, []byte("outside secret"), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0o755); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}
	linkPath := filepath.Join(skillRoot, "references", "escape.md")
	if err := os.Symlink(secretPath, linkPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{
		ID: "symlink-skill", Name: "symlink-skill", Description: "symlink test",
		Source: skills.SourceProject, Scope: skills.SourceProject, RootDir: skillRoot,
		ResolvedRootDir: mustEvalSymlinks(t, skillRoot), ProjectPath: projectRoot, State: skills.StateLoaded,
	})
	reader := &ResourceReadTool{registry: reg}
	outcome, err := reader.ExecuteOutcome(context.Background(), `{"skill_id":"symlink-skill","path":"references/escape.md"}`, nil)
	if err != nil {
		t.Fatalf("typed symlink denial escaped: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != "skill_resource_path_denied" || strings.Contains(outcome.Content, "outside secret") {
		t.Fatalf("rooted read allowed a package escape: %+v", outcome)
	}
}

func TestResourceReadToolRejectsPackageRootReplacement(t *testing.T) {
	projectRoot := t.TempDir()
	skillRoot := filepath.Join(projectRoot, ".awaken", "skills", "root-swap")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatalf("mkdir Skill root: %v", err)
	}
	trustedRoot, err := filepath.EvalSymlinks(skillRoot)
	if err != nil {
		t.Fatalf("resolve trusted Skill root: %v", err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{
		ID: "root-swap", Name: "root-swap", Description: "root swap test",
		Source: skills.SourceProject, Scope: skills.SourceProject, RootDir: skillRoot,
		ResolvedRootDir: trustedRoot, ProjectPath: projectRoot, State: skills.StateLoaded,
	})
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("outside root secret"), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}
	if err := os.RemoveAll(skillRoot); err != nil {
		t.Fatalf("remove original Skill root: %v", err)
	}
	if err := os.Symlink(outside, skillRoot); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	reader := &ResourceReadTool{registry: reg}
	outcome, err := reader.ExecuteOutcome(context.Background(), `{"skill_id":"root-swap","path":"secret.md"}`, nil)
	if err != nil {
		t.Fatalf("typed root replacement denial escaped: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != "skill_resource_path_denied" || strings.Contains(outcome.Content, "outside root secret") {
		t.Fatalf("replaced package root escaped its registry identity: %+v", outcome)
	}
}

func TestSkillInstallToolInstallsContentProjectSkillAndReloads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	reg := skills.NewSkillRegistry(nil)
	installer := &SkillInstallTool{registry: reg}
	ctx := tool.ContextWithPermissionContext(
		context.Background(),
		tool.NewPermissionContext(workspace, []string{workspace}),
	)

	output, err := installer.Execute(ctx, `{
		"source_type": "content",
		"file_name": "SKILL.md",
		"content": "---\nname: daily-followup\ndescription: Daily followup.\n---\nDo the followup.",
		"scope": "project"
	}`)
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}

	var parsed skillInstallOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("parse output: %v\n%s", err, output)
	}
	if parsed.ID != "daily-followup" || parsed.Scope != "project" || !parsed.Reloaded {
		t.Fatalf("unexpected install output: %+v", parsed)
	}
	skillPath := filepath.Join(workspace, ".awaken", "skills", "daily-followup", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected installed SKILL.md: %v", err)
	}
	def := reg.Get("daily-followup")
	if def == nil {
		t.Fatal("expected installed skill to be reloaded into registry")
	}
	if def.SkillFilePath != skillPath {
		t.Fatalf("expected skill path %q, got %q", skillPath, def.SkillFilePath)
	}
}

func TestSkillInstallToolValidateOnlyDoesNotWriteOrReload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	reg := skills.NewSkillRegistry(nil)
	installer := &SkillInstallTool{registry: reg}
	ctx := tool.ContextWithPermissionContext(
		context.Background(),
		tool.NewPermissionContext(workspace, []string{workspace}),
	)

	outcome, err := installer.ExecuteOutcome(ctx, `{
		"source_type": "content",
		"file_name": "SKILL.md",
		"content": "---\nname: validate-only\ndescription: Validate only.\n---\nDo not install me.",
		"scope": "project",
		"validate_only": true
	}`, nil)
	if err != nil {
		t.Fatalf("validate skill: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("dry-run validation reported mutation: %+v", outcome)
	}
	if outcome.Details["install_status"] != "validated" || outcome.Details["file_count"] != 1 || outcome.Details["total_bytes"] == int64(0) {
		t.Fatalf("unexpected validation details: %+v", outcome.Details)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".awaken", "skills", "validate-only")); !os.IsNotExist(err) {
		t.Fatalf("validate_only created the Skill package: %v", err)
	}
	if reg.Get("validate-only") != nil {
		t.Fatal("validate_only reloaded the Skill registry")
	}
	if len(outcome.NextActions) != 1 || outcome.NextActions[0].InputHint["path"] != nil {
		t.Fatalf("dry-run guidance must not invent a candidate source path: %+v", outcome.NextActions)
	}
}

func TestSkillInstallToolValidateOnlyHonorsAbortAndReviewedReplace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	reg := skills.NewSkillRegistry(nil)
	installer := &SkillInstallTool{registry: reg}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	base := `{"source_type":"content","file_name":"SKILL.md","content":"---\nname: existing\ndescription: Existing.\n---\nBody.","scope":"project"}`
	if _, err := installer.Execute(ctx, base); err != nil {
		t.Fatal(err)
	}
	abortOutcome, err := installer.ExecuteOutcome(ctx, `{"source_type":"content","file_name":"SKILL.md","content":"---\nname: existing\ndescription: Changed.\n---\nChanged.","scope":"project","validate_only":true}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if abortOutcome.ErrorCode != "skill_install_already_exists" || abortOutcome.Operation == nil || abortOutcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
		t.Fatalf("dry-run abort ignored existing target: %+v", abortOutcome)
	}
	replaceOutcome, err := installer.ExecuteOutcome(ctx, `{"source_type":"content","file_name":"SKILL.md","content":"---\nname: existing\ndescription: Changed.\n---\nChanged.","scope":"project","overwrite_policy":"replace","validate_only":true}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if replaceOutcome.Status != tool.OutcomeSucceeded || replaceOutcome.Details["target_exists"] != true || replaceOutcome.Details["would_replace"] != true {
		t.Fatalf("reviewed replacement dry-run=%+v", replaceOutcome)
	}
	current, err := os.ReadFile(filepath.Join(workspace, ".awaken", "skills", "existing", "SKILL.md"))
	if err != nil || strings.Contains(string(current), "Changed.") {
		t.Fatalf("replacement dry-run changed live package: %v %q", err, current)
	}
}

func TestSkillPackageExportToolCopiesCompleteCandidateOutsideLiveRoot(t *testing.T) {
	workspace := t.TempDir()
	sourceRoot := t.TempDir()
	skillRoot := filepath.Join(sourceRoot, "export-me")
	if err := os.MkdirAll(filepath.Join(skillRoot, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: export-me\ndescription: Export me.\n---\nBody.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	asset := []byte{0x00, 0x01, 0xfe, 0xff}
	if err := os.WriteFile(filepath.Join(skillRoot, "assets", "binary.dat"), asset, 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(skillRoot)
	if err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{ID: "export-me", Name: "export-me", Description: "Export me.", Source: skills.SourceUser, Scope: skills.SourceUser, RootDir: skillRoot, ResolvedRootDir: resolvedRoot, SourceRoot: sourceRoot, SkillFilePath: filepath.Join(skillRoot, "SKILL.md"), State: skills.StateLoaded})
	exporter := &SkillPackageExportTool{registry: reg}
	if err := tool.ValidateToolWithMode(exporter, tool.DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("skill export definition: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	outcome, err := exporter.ExecuteOutcome(ctx, `{"skill_id":"export-me","destination":"authoring/export-me"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("outcome=%+v", outcome)
	}
	candidate := filepath.Join(workspace, "authoring", "export-me")
	copied, err := os.ReadFile(filepath.Join(candidate, "assets", "binary.dat"))
	if err != nil || string(copied) != string(asset) {
		t.Fatalf("binary asset was not preserved: %v %v", copied, err)
	}
	original, err := os.ReadFile(filepath.Join(skillRoot, "SKILL.md"))
	if err != nil || !strings.Contains(string(original), "Body.") {
		t.Fatalf("installed package changed: %v %q", err, original)
	}
	if _, err := exporter.Execute(ctx, `{"skill_id":"export-me","destination":"authoring/export-me"}`); err == nil {
		t.Fatal("export must not overwrite an existing candidate")
	}
}

func TestSkillInstallToolStrictDefinitionAndStructuredOutcome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tooltrash.SetAdapter(skillInstallTrashAdapter{dir: filepath.Join(t.TempDir(), "Trash")})
	t.Cleanup(func() { tooltrash.SetAdapter(nil) })
	workspace := t.TempDir()
	reg := skills.NewSkillRegistry(nil)
	installer := &SkillInstallTool{registry: reg}
	if err := tool.ValidateToolWithMode(installer, tool.DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("skill_install must pass strict builtin validation: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(
		context.Background(),
		tool.NewPermissionContext(workspace, []string{workspace}),
	)
	outcome, err := installer.ExecuteOutcome(ctx, `{
		"source_type": "content",
		"file_name": "SKILL.md",
		"content": "---\nname: structured-install\ndescription: Structured install.\n---\nRun it.",
		"scope": "project"
	}`, nil)
	if err != nil {
		t.Fatalf("execute structured skill install: %v", err)
	}
	def := tool.ResolveDefinition(installer)
	outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType:       SkillInstallToolName,
		DeclaredSideEffects: def.SideEffects,
		OutputSchema:        def.OutputSchema,
		StrictOutputSchema:  true,
	})
	if outcome.Status != tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("unexpected structured outcome state: %+v", outcome)
	}
	if outcome.Details["id"] != "structured-install" || outcome.Details["install_status"] != "installed" {
		t.Fatalf("unexpected structured details: %+v", outcome.Details)
	}
	if len(outcome.Changes) != 1 || outcome.Changes[0].Kind != "skill_package" || outcome.Changes[0].Action != "created" {
		t.Fatalf("expected one observed Skill package change: %+v", outcome.Changes)
	}
	if len(outcome.NextActions) != 1 || outcome.NextActions[0].Tool != CatalogToolName || outcome.NextActions[0].InputHint["skill"] != "structured-install" {
		t.Fatalf("expected actionable post-install guidance: %+v", outcome.NextActions)
	}
	replaceInput := `{
		"source_type": "content",
		"file_name": "SKILL.md",
		"content": "---\nname: structured-install\ndescription: Replaced install.\n---\nRun the replacement.",
		"scope": "project",
		"overwrite_policy": "replace"
	}`
	decision := installer.CheckPermission(ctx, replaceInput, tool.PermissionContextFromContext(ctx))
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("recoverable replacement should be an ordinary automatic tool call: %+v", decision)
	}
	replaced, err := installer.ExecuteOutcome(ctx, replaceInput, nil)
	if err != nil {
		t.Fatalf("typed replacement block escaped: %v", err)
	}
	replaced = replaced.Normalize(tool.OutcomeNormalizationContext{
		OperationType:       SkillInstallToolName,
		DeclaredSideEffects: def.SideEffects,
		OutputSchema:        def.OutputSchema,
		StrictOutputSchema:  true,
	})
	if replaced.Status != tool.OutcomeSucceeded || replaced.Operation == nil || replaced.Operation.SideEffectState != tool.OutcomeSideEffectApplied || len(replaced.Changes) != 2 {
		t.Fatalf("replacement must install and preserve the previous package in Trash: %+v", replaced)
	}
	currentSkill, err := os.ReadFile(filepath.Join(workspace, ".awaken", "skills", "structured-install", "SKILL.md"))
	if err != nil || !strings.Contains(string(currentSkill), "Run the replacement.") {
		t.Fatalf("replacement did not install the new Skill: err=%v content=%q", err, currentSkill)
	}
	trashedPath, _ := replaced.Details["previous_package_trashed_path"].(string)
	previousSkill, err := os.ReadFile(filepath.Join(trashedPath, "SKILL.md"))
	if err != nil || !strings.Contains(string(previousSkill), "Run it.") {
		t.Fatalf("previous Skill is not recoverable from Trash: err=%v content=%q", err, previousSkill)
	}
	notReloaded, err := installer.ExecuteOutcome(ctx, `{
		"source_type": "content",
		"file_name": "SKILL.md",
		"content": "---\nname: deferred-reload\ndescription: Deferred reload.\n---\nRun later.",
		"scope": "project",
		"reload_after_install": false
	}`, nil)
	if err != nil {
		t.Fatalf("install without reload: %v", err)
	}
	if len(notReloaded.NextActions) != 1 || notReloaded.NextActions[0].ID != "reload_skill_registry" || notReloaded.NextActions[0].Tool != "" {
		t.Fatalf("reloaded=false must require registry refresh without inventing an executable tool: %+v", notReloaded.NextActions)
	}
}

func TestSkillInstallToolReturnsRecoverableTypedInputFailure(t *testing.T) {
	installer := &SkillInstallTool{registry: skills.NewSkillRegistry(nil)}
	outcome, err := installer.ExecuteOutcome(context.Background(), `{}`, nil)
	if err != nil {
		t.Fatalf("typed failure must stay in the outcome: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != "skill_install_invalid_input" || outcome.Retryable {
		t.Fatalf("unexpected typed input failure: %+v", outcome)
	}
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType:       SkillInstallToolName,
		DeclaredSideEffects: tool.ResolveDefinition(installer).SideEffects,
	})
	if len(normalized.Errors) != 1 || normalized.Errors[0].RetryStrategy != tool.RetryChangeInput || len(normalized.NextActions) == 0 {
		t.Fatalf("failure must expose recovery guidance: %+v", normalized)
	}
	if normalized.TransportStatus != tool.TransportNotStarted || normalized.Operation == nil || normalized.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
		t.Fatalf("pre-execution validation must not imply installation side effects: %+v", normalized)
	}
}

func TestSkillInstallToolClassifiesDeterministicSourceErrorsAsInputRepair(t *testing.T) {
	installer := &SkillInstallTool{registry: skills.NewSkillRegistry(nil)}
	definition := tool.ResolveDefinition(installer)
	inputs := []string{
		`{"source_type":"content"}`,
		`{"source_type":"content_base64","content_base64":"not-base64"}`,
		`{"source_type":"local_path"}`,
		`{"source_type":"remote_url"}`,
		`{"source_type":"remote_url","url":"ftp://example.com/SKILL.md"}`,
		`{"source_type":"remote_git"}`,
	}
	for _, input := range inputs {
		outcome, err := installer.ExecuteOutcome(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("typed source failure escaped for %s: %v", input, err)
		}
		outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
			OperationType: SkillInstallToolName, DeclaredSideEffects: definition.SideEffects,
		})
		if outcome.ErrorCode != "skill_install_source_invalid" || outcome.ErrorCategory != tool.ErrorCategoryValidation || outcome.Retryable {
			t.Errorf("deterministic source error was misclassified for %s: %+v", input, outcome)
			continue
		}
		if outcome.TransportStatus != tool.TransportNotStarted || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
			t.Errorf("deterministic source error implied side effects for %s: %+v", input, outcome.Operation)
		}
		if len(outcome.Errors) != 1 || outcome.Errors[0].RetryStrategy != tool.RetryChangeInput || len(outcome.NextActions) == 0 || outcome.NextActions[0].ID != "correct_input" {
			t.Errorf("deterministic source error lacks change-input recovery for %s: errors=%+v next=%+v", input, outcome.Errors, outcome.NextActions)
		}
	}
}

func TestSkillInstallToolClassifiesRemoteServiceFailureAsBoundedRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporary", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	installer := &SkillInstallTool{registry: skills.NewSkillRegistry(nil)}
	definition := tool.ResolveDefinition(installer)
	outcome, err := installer.ExecuteOutcome(context.Background(), `{"source_type":"remote_url","url":"`+server.URL+`/SKILL.md"}`, nil)
	if err != nil {
		t.Fatalf("typed remote failure escaped: %v", err)
	}
	outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: SkillInstallToolName, DeclaredSideEffects: definition.SideEffects,
	})
	if outcome.ErrorCode != "skill_install_source_unavailable" || outcome.ErrorCategory != tool.ErrorCategoryRuntime || !outcome.Retryable {
		t.Fatalf("unexpected remote service classification: %+v", outcome)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone || outcome.Errors[0].RetryStrategy != tool.RetryAfterBackoff {
		t.Fatalf("remote failure must distinguish no install mutation from bounded transport retry: %+v", outcome)
	}
}

func TestSkillInstallToolRejectsGitRemoteHelpersAndOptionLikeRefsBeforeGit(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "remote-helper-ran")
	inputs := []map[string]any{
		{"source_type": "remote_git", "url": "ext::/usr/bin/touch% " + marker},
		{"source_type": "remote_git", "url": "file:///tmp/repository"},
		{"source_type": "remote_git", "url": "."},
		{"source_type": "remote_git", "url": "https://example.com/repository.git", "ref": "--upload-pack=/usr/bin/false"},
	}
	installer := &SkillInstallTool{registry: skills.NewSkillRegistry(nil)}
	definition := tool.ResolveDefinition(installer)
	for _, request := range inputs {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("encode malicious regression input: %v", err)
		}
		outcome, err := installer.ExecuteOutcome(context.Background(), string(encoded), nil)
		if err != nil {
			t.Fatalf("typed remote_git rejection escaped: %v", err)
		}
		outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
			OperationType: SkillInstallToolName, DeclaredSideEffects: definition.SideEffects,
		})
		if outcome.ErrorCode != "skill_install_source_invalid" || outcome.ErrorCategory != tool.ErrorCategoryValidation || outcome.TransportStatus != tool.TransportNotStarted || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
			t.Errorf("unsafe remote_git input reached execution: request=%v outcome=%+v", request, outcome)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Git remote helper executed before validation: %v", err)
	}
}

func TestSafeGitEnvironmentDropsAmbientExecutionConfiguration(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.ext.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
	t.Setenv("GIT_SSH_COMMAND", "/usr/bin/false")
	t.Setenv("GIT_TEMPLATE_DIR", t.TempDir())
	t.Setenv("GIT_SSL_NO_VERIFY", "1")
	env := safeGitEnvironment()
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, forbidden := range []string{"\nGIT_CONFIG_COUNT=", "\nGIT_CONFIG_KEY_0=", "\nGIT_CONFIG_VALUE_0=", "\nGIT_SSH_COMMAND=", "\nGIT_TEMPLATE_DIR=", "\nGIT_SSL_NO_VERIFY="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unsafe Git environment survived sanitization: %s", forbidden)
		}
	}
	for _, required := range []string{"\nGIT_CONFIG_NOSYSTEM=1\n", "\nGIT_TERMINAL_PROMPT=0\n", "\nGIT_PROTOCOL_FROM_USER=0\n", "\nGIT_ALLOW_PROTOCOL=https\n"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("safe Git environment lacks %q", required)
		}
	}
}

func TestManagerRegistersSkillInstallThroughStrictGate(t *testing.T) {
	previous, previousErr := tool.GetRegisteredTool(SkillInstallToolName)
	previousResource, previousResourceErr := tool.GetRegisteredTool(ResourceReadToolName)
	previousResourceList, previousResourceListErr := tool.GetRegisteredTool(ResourceListToolName)
	previousExport, previousExportErr := tool.GetRegisteredTool(SkillPackageExportToolName)
	defer func() {
		if previousErr == nil {
			_ = tool.RegisterToolForce(previous.Runtime)
		} else {
			tool.UnregisterTool(SkillInstallToolName)
		}
		if previousResourceErr == nil {
			_ = tool.RegisterToolForce(previousResource.Runtime)
		} else {
			tool.UnregisterTool(ResourceReadToolName)
		}
		if previousResourceListErr == nil {
			_ = tool.RegisterToolForce(previousResourceList.Runtime)
		} else {
			tool.UnregisterTool(ResourceListToolName)
		}
		if previousExportErr == nil {
			_ = tool.RegisterToolForce(previousExport.Runtime)
		} else {
			tool.UnregisterTool(SkillPackageExportToolName)
		}
	}()

	manager := NewManager(skills.NewSkillRegistry(nil), nil)
	defer manager.Shutdown()
	registered, err := tool.GetRegisteredTool(SkillInstallToolName)
	if err != nil {
		t.Fatalf("skill_install was not registered: %v", err)
	}
	if registered.Origin != tool.SourceBuiltin || registered.Quality != tool.DefinitionQualityStrict {
		t.Fatalf("skill_install must be a strict builtin: origin=%s quality=%s warnings=%v", registered.Origin, registered.Quality, registered.Warnings)
	}
	if _, ok := registered.Runtime.(tool.OutcomeTool); !ok {
		t.Fatal("skill_install must execute through the structured OutcomeTool path")
	}
	resource, err := tool.GetRegisteredTool(ResourceReadToolName)
	if err != nil {
		t.Fatalf("skill_resource_read was not registered: %v", err)
	}
	if resource.Origin != tool.SourceSkill || resource.Quality != tool.DefinitionQualityStrict {
		t.Fatalf("skill_resource_read must be a strict first-party Skill tool: origin=%s quality=%s warnings=%v", resource.Origin, resource.Quality, resource.Warnings)
	}
	if _, ok := resource.Runtime.(tool.OutcomeTool); !ok {
		t.Fatal("skill_resource_read must execute through the structured OutcomeTool path")
	}
	resourceList, err := tool.GetRegisteredTool(ResourceListToolName)
	if err != nil {
		t.Fatalf("skill_resource_list was not registered: %v", err)
	}
	if resourceList.Origin != tool.SourceSkill || resourceList.Quality != tool.DefinitionQualityStrict {
		t.Fatalf("skill_resource_list must be a strict first-party Skill tool: origin=%s quality=%s warnings=%v", resourceList.Origin, resourceList.Quality, resourceList.Warnings)
	}
	if _, ok := resourceList.Runtime.(tool.OutcomeTool); !ok {
		t.Fatal("skill_resource_list must execute through the structured OutcomeTool path")
	}
	exporter, err := tool.GetRegisteredTool(SkillPackageExportToolName)
	if err != nil {
		t.Fatalf("skill_package_export was not registered: %v", err)
	}
	if exporter.Origin != tool.SourceBuiltin || exporter.Quality != tool.DefinitionQualityStrict {
		t.Fatalf("skill_package_export must be a strict builtin: origin=%s quality=%s warnings=%v", exporter.Origin, exporter.Quality, exporter.Warnings)
	}
	if _, ok := exporter.Runtime.(tool.OutcomeTool); !ok {
		t.Fatal("skill_package_export must execute through the structured OutcomeTool path")
	}
}

func TestSkillInstallToolRejectsProjectRootOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	reg := skills.NewSkillRegistry(nil)
	installer := &SkillInstallTool{registry: reg}
	inputBytes, err := json.Marshal(map[string]any{
		"source_type":  "content",
		"file_name":    "SKILL.md",
		"content":      "---\nname: outside\ndescription: Outside.\n---\nBody.",
		"scope":        "project",
		"project_root": outside,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := string(inputBytes)

	decision := installer.CheckPermission(
		context.Background(),
		input,
		tool.NewPermissionContext(workspace, []string{workspace}),
	)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny, got %+v", decision)
	}
	if !strings.Contains(decision.Reason, "project_root") {
		t.Fatalf("expected project_root denial reason, got %q", decision.Reason)
	}
}

func TestSkillInstallToolRejectsProjectRootSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	linkPath := filepath.Join(workspace, "linked-project")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	reg := skills.NewSkillRegistry(nil)
	installer := &SkillInstallTool{registry: reg}
	inputBytes, err := json.Marshal(map[string]any{
		"source_type":  "content",
		"file_name":    "SKILL.md",
		"content":      "---\nname: escaped-install\ndescription: Escape test.\n---\nBody.",
		"scope":        "project",
		"project_root": linkPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := string(inputBytes)
	permissionContext := tool.NewPermissionContext(workspace, []string{workspace})
	decision := installer.CheckPermission(context.Background(), input, permissionContext)
	if decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "project_root") {
		t.Fatalf("permission check allowed symlink project escape: %+v", decision)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	outcome, err := installer.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		t.Fatalf("typed project-root denial escaped: %v", err)
	}
	outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: SkillInstallToolName, DeclaredSideEffects: tool.ResolveDefinition(installer).SideEffects,
	})
	if outcome.Status != tool.OutcomeDenied || outcome.ErrorCode != "skill_install_target_invalid" || outcome.TransportStatus != tool.TransportNotStarted || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
		t.Fatalf("symlink project escape was not a pre-execution denial: %+v", outcome)
	}
	if _, err := os.Stat(filepath.Join(outside, ".awaken")); !os.IsNotExist(err) {
		t.Fatalf("symlink project escape created state outside workspace: %v", err)
	}
}

func TestSkillInstallToolRejectsSymlinkedSkillDirectoryEscape(t *testing.T) {
	for _, linkRelative := range []string{".awaken", filepath.Join(".awaken", "skills")} {
		t.Run(filepath.ToSlash(linkRelative), func(t *testing.T) {
			workspace := t.TempDir()
			outside := t.TempDir()
			linkPath := filepath.Join(workspace, linkRelative)
			if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
				t.Fatalf("mkdir symlink parent: %v", err)
			}
			if err := os.Symlink(outside, linkPath); err != nil {
				t.Skipf("symlink unavailable on this platform: %v", err)
			}
			installer := &SkillInstallTool{registry: skills.NewSkillRegistry(nil)}
			input := `{
				"source_type": "content",
				"file_name": "SKILL.md",
				"content": "---\nname: escaped-skill-root\ndescription: Escape test.\n---\nBody.",
				"scope": "project"
			}`
			permissionContext := tool.NewPermissionContext(workspace, []string{workspace})
			decision := installer.CheckPermission(context.Background(), input, permissionContext)
			if decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "symlink") {
				t.Fatalf("permission check allowed symlinked Skill root: %+v", decision)
			}
			ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
			outcome, err := installer.ExecuteOutcome(ctx, input, nil)
			if err != nil {
				t.Fatalf("typed Skill-root denial escaped: %v", err)
			}
			outcome = outcome.Normalize(tool.OutcomeNormalizationContext{
				OperationType: SkillInstallToolName, DeclaredSideEffects: tool.ResolveDefinition(installer).SideEffects,
			})
			if outcome.Status != tool.OutcomeDenied || outcome.ErrorCode != "skill_install_target_invalid" || outcome.TransportStatus != tool.TransportNotStarted || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
				t.Fatalf("symlinked Skill root was not a pre-execution denial: %+v", outcome)
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("symlinked Skill root wrote outside workspace: entries=%v err=%v", entries, err)
			}
		})
	}
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return resolved
}
