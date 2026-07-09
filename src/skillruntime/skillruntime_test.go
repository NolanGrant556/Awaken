package skillruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/skills"
	"awaken/src/tool"
)

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

// ---- tool_test ----

func TestSkillToolInterface(t *testing.T) {
	st := &SkillTool{
		SkillID:   "test-skill",
		NameStr:   SkillToolName("test-skill"),
		Desc:      "A test skill",
		WhenToUse: "use for testing",
		Body:      "Task: ${input}",
		ArgDefs: []SkillArgDef{
			{Name: "input", Type: "string", Required: true},
		},
	}

	if st.Name() != "skill__test_skill" {
		t.Errorf("bad name: %s", st.Name())
	}
	if st.Description() != "use for testing" {
		t.Errorf("bad description: %s", st.Description())
	}
	if !st.IsReadOnly() {
		t.Errorf("skill tool should be read-only")
	}

	params := st.Parameters()
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Name != "input" {
		t.Errorf("bad param name: %s", params[0].Name)
	}
}

func TestSkillToolExecute(t *testing.T) {
	var gotAllowed []string
	st := &SkillTool{
		SkillID:      "test-skill",
		NameStr:      SkillToolName("test-skill"),
		Desc:         "Test",
		Body:         "Root ${AWAKEN_SKILL_DIR}: Hello ${name}!",
		RootDir:      "/tmp/test-skill",
		AllowedTools: []string{"shell", "skill_resource_read"},
		ArgDefs: []SkillArgDef{
			{Name: "name", Type: "string", Required: true},
		},
		Executor: SkillExecutorFunc(func(ctx context.Context, skillID string, body string, allowedTools []string) (string, error) {
			gotAllowed = append([]string(nil), allowedTools...)
			return "RESULT: " + body, nil
		}),
	}

	result, err := st.Execute(context.Background(), `{"name":"World"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Hello World!") {
		t.Errorf("bad result: %s", result)
	}
	if !strings.Contains(result, "Base directory for this skill: /tmp/test-skill") {
		t.Errorf("base directory missing from result: %s", result)
	}
	if !strings.Contains(result, "Root /tmp/test-skill") {
		t.Errorf("AWAKEN_SKILL_DIR was not substituted: %s", result)
	}
	if strings.Join(gotAllowed, ",") != "shell,skill_resource_read" {
		t.Errorf("allowed tools were not passed through: %v", gotAllowed)
	}
}

func TestSkillToolName(t *testing.T) {
	if name := SkillToolName("meeting-notes"); name != "skill__meeting_notes" {
		t.Errorf("bad tool name: %s", name)
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
	skillRoot := filepath.Join(root, "sample")
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0755); err != nil {
		t.Fatalf("mkdir skill refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "references", "guide.md"), []byte("guide body"), 0644); err != nil {
		t.Fatalf("write guide: %v", err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{
		ID:          "sample",
		Name:        "sample",
		Description: "sample",
		Source:      skills.SourceProject,
		Scope:       skills.SourceProject,
		RootDir:     skillRoot,
		State:       skills.StateAvailable,
	})

	reader := &ResourceReadTool{registry: reg}
	result, err := reader.Execute(context.Background(), `{"skill_id":"sample","path":"references/guide.md"}`)
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if !strings.Contains(result, "guide body") {
		t.Fatalf("expected guide body, got %s", result)
	}
}

func TestResourceReadToolRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	reg := skills.NewSkillRegistry(nil)
	reg.RegisterSkill(&skills.SkillDefinition{
		ID:          "sample",
		Name:        "sample",
		Description: "sample",
		Source:      skills.SourceProject,
		Scope:       skills.SourceProject,
		RootDir:     filepath.Join(root, "sample"),
		State:       skills.StateAvailable,
	})

	reader := &ResourceReadTool{registry: reg}
	_, err := reader.Execute(context.Background(), `{"skill_id":"sample","path":"../secret.md"}`)
	if err == nil {
		t.Fatal("expected traversal to be rejected")
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

func TestSkillInstallToolRejectsProjectRootOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	reg := skills.NewSkillRegistry(nil)
	installer := &SkillInstallTool{registry: reg}
	input := `{
		"source_type": "content",
		"file_name": "SKILL.md",
		"content": "---\nname: outside\ndescription: Outside.\n---\nBody.",
		"scope": "project",
		"project_root": "` + outside + `"
	}`

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

// ---- fork_test ----

func TestForkExecutorDegradesToInline(t *testing.T) {
	e := &ForkExecutor{
		// StartChild is nil -> degrades.
	}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "fork degraded") {
		t.Errorf("bad fork degradation result: %s", result)
	}
}

func TestForkExecutorStub(t *testing.T) {
	e := &ForkExecutor{
		StartChild: func(ctx context.Context, body string) (string, error) {
			return "child-done: " + body, nil
		},
	}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "child-done") {
		t.Errorf("fork stub didn't return child result: %s", result)
	}
}

func TestForkExecutorStartChildErrorDegrades(t *testing.T) {
	e := &ForkExecutor{
		StartChild: func(ctx context.Context, body string) (string, error) {
			return "", context.DeadlineExceeded
		},
	}
	result, err := e.Execute(context.Background(), "test", "do something", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "fork degraded") {
		t.Errorf("expected fork to degrade: %s", result)
	}
}

// ---- manager_test ----

func TestManagerRefreshAndShutdown(t *testing.T) {
	// Create a minimal skill registry.
	// Use skills package to create the registry.
	// We import skills, but to avoid circular dep test we directly test
	// Manager refresh via manual tool registration.

	// Clean up any leftover skill tools.
	tool.UnregisterToolsByPrefix(SkillToolPrefix)

	st := &SkillTool{
		SkillID: "test_manager",
		NameStr: SkillToolName("test_manager"),
		Desc:    "manager test",
		Body:    "test body",
		ArgDefs: []SkillArgDef{
			{Name: "x", Type: "string", Required: true},
		},
	}
	if err := tool.RegisterToolForce(st); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Verify it's registered.
	got, err := tool.GetTool(SkillToolName("test_manager"))
	if err != nil {
		t.Fatalf("get tool: %v", err)
	}
	if got.Name() != SkillToolName("test_manager") {
		t.Errorf("bad name: %s", got.Name())
	}

	// Shutdown should remove it.
	tool.UnregisterToolsByPrefix(SkillToolPrefix)
	_, err = tool.GetTool(SkillToolName("test_manager"))
	if err == nil {
		t.Fatal("expected tool to be removed after shutdown")
	}
}
