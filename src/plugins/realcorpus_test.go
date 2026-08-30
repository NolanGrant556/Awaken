package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/hookruntime"
	"awaken/src/mcpclient"
	"awaken/src/plugins/ir"
)

const realCorpusEnv = "AWAKEN_PLUGIN_COMPAT_CORPUS"

type realCorpusFamily struct {
	Dialect      string
	VendorDir    string
	Name         string
	ManifestPath string
}

type realHookRecorder struct {
	started   []hookruntime.InputEnvelope
	completed []hookruntime.Outcome
}

func (r *realHookRecorder) InvocationStarted(_ context.Context, input hookruntime.InputEnvelope, _ hookruntime.CompiledHook) error {
	r.started = append(r.started, input)
	return nil
}

func (r *realHookRecorder) InvocationCompleted(_ context.Context, outcome hookruntime.Outcome) error {
	r.completed = append(r.completed, outcome)
	return nil
}

var realCorpusFamilies = []realCorpusFamily{
	{Dialect: "codex", VendorDir: "codex格式", Name: "magicpath", ManifestPath: ".codex-plugin/plugin.json"},
	{Dialect: "codex", VendorDir: "codex格式", Name: "openai-developers", ManifestPath: ".codex-plugin/plugin.json"},
	{Dialect: "codex", VendorDir: "codex格式", Name: "figma", ManifestPath: ".codex-plugin/plugin.json"},
	{Dialect: "claude_code", VendorDir: "claude-code格式", Name: "plugin-dev", ManifestPath: ".claude-plugin/plugin.json"},
	{Dialect: "claude_code", VendorDir: "claude-code格式", Name: "hookify", ManifestPath: ".claude-plugin/plugin.json"},
	{Dialect: "cursor", VendorDir: "cursor格式", Name: "cli-for-agent", ManifestPath: ".cursor-plugin/plugin.json"},
	{Dialect: "cursor", VendorDir: "cursor格式", Name: "continual-learning", ManifestPath: ".cursor-plugin/plugin.json"},
	{Dialect: "cursor", VendorDir: "cursor格式", Name: "cursor-team-kit", ManifestPath: ".cursor-plugin/plugin.json"},
}

func realCorpusRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv(realCorpusEnv)
	if root == "" {
		t.Skip(realCorpusEnv + " is not set")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		t.Skipf("%s is unavailable: %v", root, err)
	}
	return root
}

func TestRealCorpusDigestReport(t *testing.T) {
	root := realCorpusRoot(t)
	for _, family := range realCorpusFamilies {
		family := family
		t.Run(family.Dialect+"/"+family.Name, func(t *testing.T) {
			pluginRoot := filepath.Join(root, family.VendorDir, family.Name)
			if info, err := os.Stat(pluginRoot); err != nil || !info.IsDir() {
				t.Fatalf("expanded package is unavailable: %v", err)
			}
			if !isRegularFile(filepath.Join(pluginRoot, family.ManifestPath)) {
				t.Fatalf("manifest is missing: %s", family.ManifestPath)
			}
			if !isRegularFile(filepath.Join(root, family.VendorDir, family.Name+".zip")) {
				t.Fatalf("ZIP carrier is missing")
			}
			digest, err := digestTree(pluginRoot)
			if err != nil {
				t.Fatalf("digest expanded package: %v", err)
			}
			t.Logf("family=%s dialect=%s digest=%s", family.Name, family.Dialect, digest)
		})
	}
}

func TestRealCorpusValidationPreview(t *testing.T) {
	root := realCorpusRoot(t)
	expectedCounts := map[string]map[ir.ComponentKind]int{
		"figma":           {ir.KindSkill: 12, ir.KindMCPServer: 1},
		"hookify":         {ir.KindHook: 4},
		"cursor-team-kit": {ir.KindSkill: 18, ir.KindAgent: 2, ir.KindRule: 2},
	}
	for _, family := range realCorpusFamilies {
		family := family
		t.Run(family.Dialect+"/"+family.Name, func(t *testing.T) {
			store := NewStoreAt(t.TempDir())
			result, err := store.Validate(context.Background(), Source{
				Kind: SourceLocalPath, Path: filepath.Join(root, family.VendorDir, family.Name), DialectHint: ir.Dialect(family.Dialect),
			})
			if err != nil {
				t.Fatalf("validate real package: %v", err)
			}
			if !result.Valid {
				t.Fatalf("validation unexpectedly invalid: %#v", result.Diagnostics)
			}
			if result.Dialect != ir.Dialect(family.Dialect) {
				t.Fatalf("dialect=%q, want %q", result.Dialect, family.Dialect)
			}
			if result.Name != family.Name {
				t.Fatalf("name=%q, want %q", result.Name, family.Name)
			}
			counts := map[ir.ComponentKind]int{}
			for _, component := range result.Components {
				counts[component.Kind]++
				if component.LogicalID == "" || component.LogicalIDEncoded == "" || component.SourceDialect == "" {
					t.Fatalf("incomplete component identity: %#v", component)
				}
			}
			for kind, want := range expectedCounts[family.Name] {
				if got := counts[kind]; got != want {
					t.Fatalf("%s count=%d, want %d; all=%v", kind, got, want, counts)
				}
			}
		})
	}
}

func TestRealCorpusInstallInventory(t *testing.T) {
	root := realCorpusRoot(t)
	for _, family := range realCorpusFamilies {
		family := family
		for _, carrier := range []string{"directory", "zip"} {
			carrier := carrier
			t.Run(family.Dialect+"/"+family.Name+"/"+carrier, func(t *testing.T) {
				path := filepath.Join(root, family.VendorDir, family.Name)
				if carrier == "zip" {
					path += ".zip"
				}
				store := NewStoreAt(t.TempDir())
				source := Source{Kind: SourceLocalPath, Path: path, DialectHint: ir.Dialect(family.Dialect)}
				validated, err := store.Validate(context.Background(), source)
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest})
				if err != nil {
					t.Fatalf("install: %v", err)
				}
				if installed.Plugin == nil || installed.Plugin.Dialect != ir.Dialect(family.Dialect) || installed.Plugin.ManifestPath != family.ManifestPath {
					t.Fatalf("unexpected installed plugin: %#v", installed.Plugin)
				}
				if len(installed.Plugin.Components) != len(validated.Components) {
					t.Fatalf("inventory changed across install: validate=%d install=%d", len(validated.Components), len(installed.Plugin.Components))
				}
			})
		}
	}
}

func TestRealCorpusCodexOpenAIDevelopersMCPHandshakeAndHarmlessCall(t *testing.T) {
	root := realCorpusRoot(t)
	pluginRoot := filepath.Join(root, "codex格式", "openai-developers")
	store := NewStoreAt(t.TempDir())
	source := Source{Kind: SourceLocalPath, Path: pluginRoot, DialectHint: ir.DialectCodex}
	validated, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest})
	if err != nil {
		t.Fatal(err)
	}
	servers := store.PluginMCPServers()
	if len(servers) != 1 || servers[0].Config.Stdio == nil {
		t.Fatalf("translated servers=%#v", servers)
	}
	manager := mcpclient.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	defer manager.Shutdown(context.Background())
	manager.Start(ctx, servers)
	identity := servers[0].Name
	deadline := time.Now().Add(8 * time.Second)
	for {
		snapshot, snapshotErr := manager.SnapshotServer(identity)
		if snapshotErr == nil && snapshot.State == mcpclient.StateConnected {
			break
		}
		if snapshotErr == nil && snapshot.State == mcpclient.StateFailed {
			t.Fatalf("MCP failed to connect: %s", snapshot.ErrorReason)
		}
		if time.Now().After(deadline) {
			t.Fatalf("MCP did not connect: snapshot=%#v err=%v", snapshot, snapshotErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
	tools, err := manager.ServerTools(identity)
	if err != nil || len(tools) != 1 || tools[0].Name != "confirm_openai_api_key_local_destination" || !tools[0].Enabled {
		t.Fatalf("tools/list=%#v err=%v", tools, err)
	}
	_, callErr := manager.CallTool(ctx, mcpclient.McpToolName(identity, tools[0].Name), map[string]any{})
	if callErr == nil || !strings.Contains(callErr.Error(), "workspacePath") {
		t.Fatalf("expected real harmless validation call failure, got %v", callErr)
	}
	if installed.Plugin == nil || len(store.EnabledSkillSelections()) != 5 {
		t.Fatalf("installed runtime projection incomplete: plugin=%#v skills=%d", installed.Plugin, len(store.EnabledSkillSelections()))
	}
}

func TestRealCorpusClaudeHookifyCompilesAndRunsFourCommandEvents(t *testing.T) {
	root := realCorpusRoot(t)
	pluginRoot := filepath.Join(root, "claude-code格式", "hookify")
	store := NewStoreAt(t.TempDir())
	source := Source{Kind: SourceLocalPath, Path: pluginRoot, DialectHint: ir.DialectClaudeCode}
	validated, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest})
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := t.TempDir()
	hooks, sources, diagnostics, err := store.CompilePluginHooks(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 4 || len(sources) != 1 || len(diagnostics) != 0 {
		t.Fatalf("hooks=%d sources=%#v diagnostics=%#v", len(hooks), sources, diagnostics)
	}
	snapshot, err := hookruntime.MergePluginHooks(hookruntime.EmptySnapshot(), hooks, sources)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))
	recorder := &realHookRecorder{}
	tests := []struct {
		event   hookruntime.Event
		subject map[string]any
	}{
		{hookruntime.EventPreToolUse, map[string]any{"toolName": "Bash", "input": map[string]any{"command": "pwd"}}},
		{hookruntime.EventPostToolUse, map[string]any{"toolName": "Bash", "input": map[string]any{"command": "pwd"}, "output": "ok"}},
		{hookruntime.EventBeforeFinish, map[string]any{"candidateAnswer": "done", "priorBlockCount": 0}},
		{hookruntime.EventUserPromptSubmit, map[string]any{"text": "hello"}},
	}
	for _, test := range tests {
		result, dispatchErr := dispatcher.Dispatch(context.Background(), test.event, test.subject, hookruntime.InvocationIdentity{ThreadID: "thread", RunID: "run", WorkspaceRoot: projectRoot}, recorder)
		if dispatchErr != nil || len(result.Outcomes) != 1 || result.Outcomes[0].Status != "success" {
			t.Fatalf("event=%s result=%#v err=%v", test.event, result, dispatchErr)
		}
		if result.Outcomes[0].PluginName != installed.Plugin.Name || result.Outcomes[0].PluginDigest != installed.Plugin.Digest {
			t.Fatalf("missing Plugin receipt identity: %#v", result.Outcomes[0])
		}
	}
	if len(recorder.started) != 4 || len(recorder.completed) != 4 {
		t.Fatalf("receipts started=%d completed=%d", len(recorder.started), len(recorder.completed))
	}
}

func TestRealCorpusCursorContinualLearningRunsStopWithoutInventedPostCompact(t *testing.T) {
	root := realCorpusRoot(t)
	pluginRoot := filepath.Join(root, "cursor格式", "continual-learning")
	store := NewStoreAt(t.TempDir())
	source := Source{Kind: SourceLocalPath, Path: pluginRoot, DialectHint: ir.DialectCursor}
	validated, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest}); err != nil {
		t.Fatal(err)
	}
	projectRoot := t.TempDir()
	hooks, sources, diagnostics, err := store.CompilePluginHooks(projectRoot)
	if err != nil || len(hooks) != 1 || len(diagnostics) != 0 {
		t.Fatalf("hooks=%#v diagnostics=%#v err=%v", hooks, diagnostics, err)
	}
	if hooks[0].Event != hookruntime.EventBeforeFinish || hooks[0].SourceEvent != "stop" {
		t.Fatalf("event mapping=%#v", hooks[0])
	}
	for _, hook := range hooks {
		if hook.SourceEvent == "postCompact" || hook.Event == hookruntime.EventPostCompact {
			t.Fatal("Cursor postCompact was invented")
		}
	}
	snapshot, err := hookruntime.MergePluginHooks(hookruntime.EmptySnapshot(), hooks, sources)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := hookruntime.NewDispatcher(snapshot, os.Getenv("PATH"))
	recorder := &realHookRecorder{}
	result, err := dispatcher.Dispatch(context.Background(), hookruntime.EventBeforeFinish, map[string]any{"candidateAnswer": "done", "priorBlockCount": 0}, hookruntime.InvocationIdentity{ThreadID: "thread", RunID: "run", WorkspaceRoot: projectRoot}, recorder)
	if err != nil || len(result.Outcomes) != 1 || result.Outcomes[0].Status != "success" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestRealCorpusPluginAgentsCompileIntoNativeSubagentRoles(t *testing.T) {
	root := realCorpusRoot(t)
	tests := []struct {
		vendorDir string
		name      string
		dialect   ir.Dialect
		role      string
		readOnly  bool
	}{
		{"claude-code格式", "hookify", ir.DialectClaudeCode, "plugin__hookify__conversation-analyzer", true},
		{"cursor格式", "cursor-team-kit", ir.DialectCursor, "plugin__cursor-team-kit__ci-watcher", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStoreAt(t.TempDir())
			source := Source{Kind: SourceLocalPath, Path: filepath.Join(root, test.vendorDir, test.name), DialectHint: test.dialect}
			validated, err := store.Validate(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest})
			if err != nil {
				t.Fatal(err)
			}
			role, ok := store.ResolvePluginAgentRole(test.role)
			if !ok || role.ReadOnly != test.readOnly || role.PluginDigest != installed.Plugin.Digest || role.SystemPrompt == "" {
				t.Fatalf("role=%#v ok=%v", role, ok)
			}
			if _, err := store.ResolveAgentSelection(installed.Plugin.Name, "stale", role.LogicalID); err == nil {
				t.Fatal("stale Plugin Agent digest was accepted")
			}
		})
	}
}
