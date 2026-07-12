package skillruntime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"awaken/src/skills"
)

func writeCatalogSkill(t *testing.T, root, id, frontmatter, body string) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + id + "\ndescription: " + id + "\n" + frontmatter + "---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompileCatalogUsesOnePercentAndNamesOnlyFallback(t *testing.T) {
	defs := []*skills.SkillDefinition{
		{ID: "alpha", Description: strings.Repeat("a", 400), WhenToUse: "first", Source: skills.SourceProject, State: skills.StateLoaded},
		{ID: "beta", Description: strings.Repeat("b", 400), WhenToUse: "second", Source: skills.SourceUser, State: skills.StateLoaded},
		{ID: "manual", Description: "manual", Source: skills.SourceUser, State: skills.StateLoaded, DisableModelInvocation: true},
	}
	full := CompileCatalog(defs, 100_000)
	if full.BudgetChars != 4000 || full.Mode != CatalogModeDescriptions || strings.Contains(full.Listing, strings.Repeat("a", 251)) {
		t.Fatalf("full catalog=%+v", full)
	}
	tiny := CompileCatalog(defs, 100)
	if tiny.Mode != CatalogModeNamesOnly || tiny.Listing != "- alpha\n- beta" || strings.Contains(tiny.Listing, "manual") {
		t.Fatalf("tiny catalog=%+v", tiny)
	}
}

func TestCatalogToolLoadsBodyOnDemandAndSubstitutesArgs(t *testing.T) {
	root := t.TempDir()
	writeCatalogSkill(t, root, "lazy", "allowed-tools: [Read, VendorTool]\n", "Old $ARGUMENTS")
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	if body := reg.Get("lazy").Body; body != "" {
		t.Fatalf("catalog retained full body: %q", body)
	}
	writeCatalogSkill(t, root, "lazy", "allowed-tools: [Read, VendorTool]\n", "New $ARGUMENTS")
	tool := &CatalogTool{Registry: reg, Activator: NewActivator(reg, nil)}
	result, err := tool.Invoke(context.Background(), SkillInvocationRequest{Skill: "lazy", Args: "input.pdf", Source: InvocationUser})
	if err != nil || !strings.Contains(result.Content, "New input.pdf") || strings.Contains(result.Content, "Old") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(result.Plan.ToolHints) != 2 || result.Plan.ToolHints[0].Canonical != "read_file" {
		t.Fatalf("hints=%+v", result.Plan.ToolHints)
	}
}

func TestSequentialComplementarySkillsNeverIntersectParentToolPool(t *testing.T) {
	root := t.TempDir()
	writeCatalogSkill(t, root, "research", "allowed-tools: [Read, WebSearch]\n", "Research $ARGUMENTS")
	writeCatalogSkill(t, root, "publish", "allowed-tools: [Write, VendorPublisher]\n", "Publish $ARGUMENTS")

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	catalogTool := &CatalogTool{Registry: registry, Activator: NewActivator(registry, nil)}
	parentPool := []string{"read_file", "shell", "text_file_create", "web_search"}

	first, err := catalogTool.Invoke(context.Background(), SkillInvocationRequest{
		Skill: "research", Args: "sources", Source: InvocationModel, ParentToolPool: parentPool,
	})
	if err != nil {
		t.Fatalf("invoke research Skill: %v", err)
	}
	second, err := catalogTool.Invoke(context.Background(), SkillInvocationRequest{
		Skill: "publish", Args: "report", Source: InvocationModel, ParentToolPool: parentPool,
	})
	if err != nil {
		t.Fatalf("invoke publish Skill: %v", err)
	}

	if !strings.Contains(first.Content, "Research sources") || !strings.Contains(second.Content, "Publish report") {
		t.Fatalf("sequential Skill bodies were not independently loaded: first=%q second=%q", first.Content, second.Content)
	}
	if first.Plan == nil || second.Plan == nil ||
		!reflect.DeepEqual(first.Plan.EffectiveTools, parentPool) ||
		!reflect.DeepEqual(second.Plan.EffectiveTools, parentPool) {
		t.Fatalf("Skill hints narrowed the pinned parent pool: first=%+v second=%+v", first.Plan, second.Plan)
	}
	if reflect.DeepEqual(first.Plan.ToolHints, second.Plan.ToolHints) {
		t.Fatalf("test fixture must exercise complementary tool hints: first=%+v second=%+v", first.Plan.ToolHints, second.Plan.ToolHints)
	}
	for _, skillID := range []string{"research", "publish"} {
		if invokedAt, ok := registry.LastInvokedAt(skillID); !ok || invokedAt.IsZero() {
			t.Fatalf("successful sequential invocation was not recorded for %s", skillID)
		}
	}
}

func TestDisableModelInvocationStillAllowsExplicitUserInvoke(t *testing.T) {
	root := t.TempDir()
	writeCatalogSkill(t, root, "manual", "disable-model-invocation: true\n", "Manual body")
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	catalogTool := &CatalogTool{Registry: reg, Activator: NewActivator(reg, nil)}
	if _, err := catalogTool.Execute(context.Background(), `{"skill":"manual"}`); err == nil || !strings.Contains(err.Error(), "skill_model_invocation_disabled") {
		t.Fatalf("model invocation was not blocked: %v", err)
	}
	result, err := catalogTool.Invoke(context.Background(), SkillInvocationRequest{Skill: "manual", Source: InvocationUser})
	if err != nil || !strings.Contains(result.Content, "Manual body") {
		t.Fatalf("explicit invocation failed: result=%+v err=%v", result, err)
	}
}

func TestCatalogForkUsesRunScopedStarterAndNeverFallsBackInline(t *testing.T) {
	root := t.TempDir()
	writeCatalogSkill(t, root, "forked", "context: fork\nagent: researcher\n", "Fork body")
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	catalogTool := &CatalogTool{
		Registry: reg, Activator: NewActivator(reg, nil),
		ForkStarter: ChildAgentStarterFunc(func(context.Context, SkillForkRequest) (SkillForkResult, error) {
			return SkillForkResult{ChildID: "global", Content: "global"}, nil
		}),
	}
	ctx := WithChildAgentStarter(context.Background(), ChildAgentStarterFunc(func(_ context.Context, request SkillForkRequest) (SkillForkResult, error) {
		if request.Agent != "researcher" || !strings.Contains(request.Body, "Fork body") {
			t.Fatalf("fork request=%+v", request)
		}
		return SkillForkResult{ChildID: "run-child", Content: "child result"}, nil
	}))
	result, err := catalogTool.Invoke(ctx, SkillInvocationRequest{Skill: "forked", Source: InvocationUser})
	if err != nil || !result.Forked || result.ChildID != "run-child" || result.Content != "child result" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	catalogTool.ForkStarter = nil
	if _, err := catalogTool.Invoke(context.Background(), SkillInvocationRequest{Skill: "forked", Source: InvocationUser}); err == nil || !strings.Contains(err.Error(), "skill_fork_unavailable") {
		t.Fatalf("missing starter silently inlined: %v", err)
	}
}

func TestRecentInvocationSurvivesLocalReload(t *testing.T) {
	root := t.TempDir()
	writeCatalogSkill(t, root, "recent", "", "Body")
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	want := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	reg.MarkInvoked("recent", want)
	reg.ReloadLocalSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	got, ok := reg.LastInvokedAt("recent")
	def := reg.Get("recent")
	if !ok || !got.Equal(want) || def == nil || def.LastInvokedAt == nil || !def.LastInvokedAt.Equal(want) {
		t.Fatalf("recent invocation lost: got=%v ok=%v def=%+v", got, ok, def)
	}
}

func TestLegacyFlatCommandActivatesOnDemand(t *testing.T) {
	commands := t.TempDir()
	if err := os.WriteFile(filepath.Join(commands, "review.md"), []byte("# Review\n\nInspect $ARGUMENTS now."), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceLegacy: commands})
	catalogTool := &CatalogTool{Registry: reg, Activator: NewActivator(reg, nil)}
	result, err := catalogTool.Invoke(context.Background(), SkillInvocationRequest{Skill: "review", Args: "src/main.go", Source: InvocationUser})
	if err != nil || !strings.Contains(result.Content, "Inspect src/main.go now") {
		t.Fatalf("legacy activation result=%+v err=%v", result, err)
	}
}
