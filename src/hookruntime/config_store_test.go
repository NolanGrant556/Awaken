package hookruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfig = `{
  "version": 1,
  "enabled": true,
  "hooks": {
    "PreToolUse": [{
      "matcher": {"toolNames": ["Write", "Edit"]},
      "hooks": [{"id":"validate","type":"command","command":["./check.sh"],"cwd":"project_root","onFailure":"block"}]
    }]
  }
}`

func writeConfig(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestBuildSnapshotRequiresProjectTrustAndPinsNormalizedDigest(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	store, err := NewStore(home)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, ".awaken", "hooks.json")
	writeConfig(t, path, validConfig, 0o644)

	first, err := store.BuildSnapshot("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Sources) != 1 || first.Sources[0].Status != StatusUntrusted {
		t.Fatalf("sources = %+v", first.Sources)
	}
	if len(first.HooksByEvent[EventPreToolUse]) != 0 {
		t.Fatal("untrusted project hook was compiled")
	}
	if err := store.TrustStore().Trust("project-1", first.Sources[0].Path, first.Sources[0].Digest); err != nil {
		t.Fatal(err)
	}

	trusted, err := store.BuildSnapshot("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if trusted.Sources[0].Status != StatusEnabled || len(trusted.HooksByEvent[EventPreToolUse]) != 1 {
		t.Fatalf("trusted snapshot = %+v", trusted)
	}

	writeConfig(t, path, validConfig+"\n", 0o644)
	whitespaceOnly, err := store.BuildSnapshot("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if whitespaceOnly.Sources[0].Status != StatusEnabled {
		t.Fatalf("normalized digest changed on whitespace: %+v", whitespaceOnly.Sources[0])
	}

	writeConfig(t, path, stringReplace(validConfig, "./check.sh", "./changed.sh"), 0o644)
	changed, err := store.BuildSnapshot("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Sources[0].Status != StatusUntrusted {
		t.Fatalf("changed config stayed trusted: %+v", changed.Sources[0])
	}
}

func TestInspectReturnsReviewDetailsWithoutChangingExecutionSnapshot(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store, _ := NewStore(home)
	path := filepath.Join(project, ".awaken", "hooks.json")
	writeConfig(t, path, validConfig, 0o644)

	inspection, err := store.Inspect("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Sources) != 1 || len(inspection.Sources[0].ReviewHooks) != 1 {
		t.Fatalf("inspection = %+v", inspection.Sources)
	}
	review := inspection.Sources[0].ReviewHooks[0]
	if review.ID != "validate" || review.Event != EventPreToolUse || len(review.Command) != 1 || review.Command[0] != "./check.sh" {
		t.Fatalf("review = %+v", review)
	}
	if len(inspection.HooksByEvent[EventPreToolUse]) != 0 {
		t.Fatal("inspection compiled an untrusted project Hook")
	}

	snapshot, err := store.BuildSnapshot("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sources[0].ReviewHooks) != 0 || snapshot.SnapshotID != inspection.SnapshotID {
		t.Fatalf("execution snapshot changed: snapshot=%+v inspection=%+v", snapshot.Sources[0], inspection.Sources[0])
	}
}

func TestInspectExposesOnlyHTTPSecretHeaderNames(t *testing.T) {
	home := t.TempDir()
	store, _ := NewStore(home)
	config := `{"version":1,"enabled":true,"hooks":{"RunStart":[{"matcher":{"toolNames":[]},"hooks":[{"id":"notify","type":"http","url":"https://example.com/hook","secretHeaders":{"Authorization":"prod-token","X-Signature":"signing-key"}}]}]}}`
	writeConfig(t, store.UserConfigPath(), config, 0o600)
	inspection, err := store.Inspect("", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	review := inspection.Sources[0].ReviewHooks[0]
	if got := review.SecretHeaderNames; len(got) != 2 || got[0] != "Authorization" || got[1] != "X-Signature" {
		t.Fatalf("header names = %#v", got)
	}
	encoded, _ := json.Marshal(review)
	if strings.Contains(string(encoded), "prod-token") || strings.Contains(string(encoded), "signing-key") {
		t.Fatalf("secret reference leaked: %s", encoded)
	}
}

func TestBuildSnapshotDoesNotScanProjectForSessionWorkspace(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store, _ := NewStore(home)
	writeConfig(t, filepath.Join(project, ".awaken", "hooks.json"), validConfig, 0o644)
	snapshot, err := store.BuildSnapshot("project-1", []string{project}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sources) != 0 {
		t.Fatalf("session workspace discovered project hook: %+v", snapshot.Sources)
	}
}

func TestUserConfigFailsClosedForPermissionsAndUnknownFields(t *testing.T) {
	home := t.TempDir()
	store, _ := NewStore(home)
	writeConfig(t, store.UserConfigPath(), validConfig, 0o644)
	if _, err := store.BuildSnapshot("", nil, false); err == nil {
		t.Fatal("expected unsafe permission error")
	}
	if err := os.Chmod(store.UserConfigPath(), 0o600); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, store.UserConfigPath(), stringReplace(validConfig, `"enabled": true`, `"enabled": true, "unknown": 1`), 0o600)
	if _, err := store.BuildSnapshot("", nil, false); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestConfigRejectsRegexMatcherAndBlockingPostFailure(t *testing.T) {
	badMatcher := stringReplace(validConfig, `"Write", "Edit"`, `"^Write$"`)
	if _, err := decodeConfig([]byte(badMatcher)); err == nil {
		t.Fatal("expected regex matcher rejection")
	}
	badPost := stringReplace(validConfig, `"PreToolUse"`, `"PostToolUse"`)
	if _, err := decodeConfig([]byte(badPost)); err == nil {
		t.Fatal("expected blocking post hook rejection")
	}
}

func TestProjectConfigRejectsAncestorSymlinkEscape(t *testing.T) {
	home, project, outside := t.TempDir(), t.TempDir(), t.TempDir()
	store, _ := NewStore(home)
	writeConfig(t, filepath.Join(outside, "hooks.json"), validConfig, 0o644)
	if err := os.Symlink(outside, filepath.Join(project, ".awaken")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.BuildSnapshot("project-1", []string{project}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sources) != 1 || snapshot.Sources[0].Status != StatusInvalid {
		t.Fatalf("escaped source = %+v", snapshot.Sources)
	}
}

func TestMatcherNamesAreTrimmedBeforeCompilation(t *testing.T) {
	config, err := decodeConfig([]byte(stringReplace(validConfig, `"Write", "Edit"`, `" Write ", "Edit"`)))
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Hooks[EventPreToolUse][0].Matcher.ToolNames[0]; got != "Write" {
		t.Fatalf("matcher = %q", got)
	}
}

func stringReplace(value, old, replacement string) string {
	for i := 0; i+len(old) <= len(value); i++ {
		if value[i:i+len(old)] == old {
			return value[:i] + replacement + value[i+len(old):]
		}
	}
	return value
}
