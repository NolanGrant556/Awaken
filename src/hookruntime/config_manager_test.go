package hookruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMutateConfigCreatesNormalizedUserHookAndSupportsDelete(t *testing.T) {
	home := t.TempDir()
	store, err := NewStore(home)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.MutateConfig(ConfigMutationRequest{
		Action: "upsert", Source: SourceUser, Event: EventPreToolUse,
		Matcher: Matcher{ToolNames: []string{"Write"}},
		Hook:    CommandHook{ID: "validate-write", Type: "command", Command: []string{"./validate.sh"}, OnFailure: "block"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.HookCount != 1 || result.RequiresTrust || result.Hooks[0].TimeoutMS != 10000 || result.Hooks[0].CWD != "workspace_root" {
		t.Fatalf("unexpected result: %+v", result)
	}
	info, err := os.Stat(store.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("user config mode = %o", info.Mode().Perm())
	}
	snapshot, err := store.BuildSnapshot("", nil, false)
	if err != nil || len(snapshot.HooksByEvent[EventPreToolUse]) != 1 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}

	deleted, err := store.MutateConfig(ConfigMutationRequest{Action: "delete", Source: SourceUser, HookID: "validate-write"})
	if err != nil || deleted.HookCount != 0 {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
}

func TestProjectLocalConfigIsDiscoveredSeparatelyAndRequiresTrust(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store, _ := NewStore(home)
	result, err := store.MutateConfig(ConfigMutationRequest{
		Action: "upsert", Source: SourceProjectLocal, ProjectRoot: project, Event: EventRunStart,
		Hook: CommandHook{ID: "local-start", Type: "command", Command: []string{"./local.sh"}, CWD: "project_root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.RequiresTrust || !strings.HasSuffix(result.Path, filepath.Join(".awaken", "hooks.local.json")) {
		t.Fatalf("result = %+v", result)
	}
	status, err := store.Inspect("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Sources) != 1 || status.Sources[0].Kind != SourceProjectLocal || status.Sources[0].Status != StatusUntrusted {
		t.Fatalf("sources = %+v", status.Sources)
	}
	if err := store.TrustStore().Trust("project-1", status.Sources[0].Path, status.Sources[0].Digest); err != nil {
		t.Fatal(err)
	}
	trusted, err := store.BuildSnapshot("project-1", []string{project}, false)
	if err != nil || trusted.Sources[0].Status != StatusEnabled || len(trusted.HooksByEvent[EventRunStart]) != 1 {
		t.Fatalf("trusted=%+v err=%v", trusted, err)
	}
}

func TestMutateConfigDoesNotOverwriteInvalidExistingFile(t *testing.T) {
	home := t.TempDir()
	store, _ := NewStore(home)
	writeConfig(t, store.UserConfigPath(), `{"version":1,"enabled":true,"unknown":true}`, 0o600)
	before, _ := os.ReadFile(store.UserConfigPath())
	_, err := store.MutateConfig(ConfigMutationRequest{Action: "set_enabled", Source: SourceUser, Enabled: boolPointer(false)})
	if err == nil {
		t.Fatal("expected invalid existing config to be preserved")
	}
	after, _ := os.ReadFile(store.UserConfigPath())
	if string(after) != string(before) {
		t.Fatal("invalid existing config was overwritten")
	}
}

func boolPointer(value bool) *bool { return &value }
