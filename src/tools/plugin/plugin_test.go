package plugintool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/plugins"
	"awaken/src/tool"
	"awaken/src/tool/trash"
)

type testTrash struct{ root string }

func (a testTrash) TrashPath(source string) (trash.TrashResult, error) {
	if err := os.MkdirAll(a.root, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.root, filepath.Base(source))
	if err := os.Rename(source, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: source, TrashedPath: target, Method: "test_rename"}, nil
}

func writePlugin(t *testing.T, root, description string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "skills", "helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"$schema":"` + plugins.SchemaPluginV1 + `","name":"creator.test","version":"1.0.0","description":"` + description + `"}`
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "helper", "SKILL.md"), []byte("---\nname: helper\ndescription: Helper skill.\n---\nHelp.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPluginToolValidateInstallInspectAndReplace(t *testing.T) {
	workspace := t.TempDir()
	candidate := filepath.Join(workspace, "candidate")
	writePlugin(t, candidate, "First")
	store := plugins.NewStoreAtWithTrash(t.TempDir(), testTrash{root: filepath.Join(t.TempDir(), "Trash")})
	pluginTool := New(store)
	if err := tool.ValidateToolWithMode(pluginTool, tool.DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("plugin tool definition: %v", err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))

	validateRaw, err := pluginTool.Execute(ctx, `{"action":"validate","source":{"kind":"local_path","path":"candidate"}}`)
	if err != nil {
		t.Fatal(err)
	}
	var validated result
	if err := json.Unmarshal([]byte(validateRaw), &validated); err != nil {
		t.Fatal(err)
	}
	if !validated.Valid || validated.Installed || validated.SkillCount != 1 {
		t.Fatalf("validated=%+v", validated)
	}
	if listed, err := store.List(); err != nil || len(listed) != 0 {
		t.Fatalf("validation changed store: list=%v err=%v", listed, err)
	}

	installRaw, err := pluginTool.Execute(ctx, `{"action":"install","source":{"kind":"local_path","path":"candidate"}}`)
	if err != nil {
		t.Fatal(err)
	}
	var installed result
	if err := json.Unmarshal([]byte(installRaw), &installed); err != nil {
		t.Fatal(err)
	}
	if !installed.Valid || !installed.Installed || installed.Replaced || installed.RootDir == "" {
		t.Fatalf("installed=%+v", installed)
	}
	exportRaw, err := pluginTool.Execute(ctx, `{"action":"export","name":"creator.test","destination":"exported/creator.test"}`)
	if err != nil {
		t.Fatal(err)
	}
	var exported result
	if err := json.Unmarshal([]byte(exportRaw), &exported); err != nil {
		t.Fatal(err)
	}
	if !exported.Exported || !exported.Installed || exported.RootDir != filepath.Join(workspace, "exported", "creator.test") {
		t.Fatalf("exported=%+v", exported)
	}
	if _, err := os.Stat(filepath.Join(exported.RootDir, "skills", "helper", "SKILL.md")); err != nil {
		t.Fatalf("export omitted embedded Skill: %v", err)
	}

	inspectRaw, err := pluginTool.Execute(ctx, `{"action":"inspect","name":"creator.test"}`)
	if err != nil || !strings.Contains(inspectRaw, `"installed": true`) {
		t.Fatalf("inspect=%s err=%v", inspectRaw, err)
	}
	recordedDigest := installed.Digest
	if err := os.WriteFile(filepath.Join(installed.RootDir, "skills", "helper", "SKILL.md"), []byte("---\nname: helper\ndescription: Updated helper skill.\n---\nUpdated help.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commitRaw, err := pluginTool.Execute(ctx, `{"action":"commit","name":"creator.test","expected_digest":"`+recordedDigest+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	var committed result
	if err := json.Unmarshal([]byte(commitRaw), &committed); err != nil {
		t.Fatal(err)
	}
	if committed.Action != "commit" || !committed.Valid || !committed.Installed || !committed.Changed || committed.Digest == recordedDigest || committed.PreviousDigest != recordedDigest || committed.SkillCount != 1 {
		t.Fatalf("committed=%+v", committed)
	}
	stale, err := pluginTool.ExecuteOutcome(ctx, `{"action":"commit","name":"creator.test","expected_digest":"`+recordedDigest+`"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stale.ErrorCode != "plugin_recorded_digest_changed" || stale.Retryable {
		t.Fatalf("stale commit outcome=%+v", stale)
	}
	if err := os.WriteFile(filepath.Join(candidate, "changed.txt"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceRaw, err := pluginTool.Execute(ctx, `{"action":"install","source":{"kind":"local_path","path":"candidate"},"replace":true}`)
	if err != nil {
		t.Fatal(err)
	}
	var replaced result
	if err := json.Unmarshal([]byte(replaceRaw), &replaced); err != nil {
		t.Fatal(err)
	}
	if !replaced.Replaced || replaced.PreviousTrashedPath == "" {
		t.Fatalf("replaced=%+v", replaced)
	}
}

func TestPluginToolCommitRequiresNameAndExpectedDigest(t *testing.T) {
	pluginTool := New(plugins.NewStoreAt(t.TempDir()))
	for _, raw := range []string{
		`{"action":"commit","name":"creator.test"}`,
		`{"action":"commit","expected_digest":"sha256-v2:recorded"}`,
	} {
		outcome, err := pluginTool.ExecuteOutcome(context.Background(), raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.ErrorCode != "plugin_invalid_input" {
			t.Fatalf("raw=%s outcome=%+v", raw, outcome)
		}
	}
}

func TestPluginToolCommitMapsInvalidInstalledPackage(t *testing.T) {
	home := t.TempDir()
	store := plugins.NewStoreAt(home)
	src := t.TempDir()
	writePlugin(t, src, "Before invalid edit")
	installed, err := store.Install(t.Context(), plugins.InstallRequest{Source: plugins.Source{Kind: plugins.SourceLocalPath, Path: src}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "plugin.json"), []byte(`{"name":"creator.test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	pluginTool := New(store)
	raw := `{"action":"commit","name":"creator.test","expected_digest":"` + installed.Plugin.Digest + `"}`
	outcome, err := pluginTool.ExecuteOutcome(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ErrorCode != "plugin_source_invalid" || outcome.Retryable {
		t.Fatalf("invalid commit outcome=%+v", outcome)
	}
}
