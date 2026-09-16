package searchsettings

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSearchFile(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "search.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaultsToExaWhenUserDataFileIsMissing(t *testing.T) {
	t.Setenv("AWAKEN_USER_DATA_DIR", t.TempDir())
	t.Setenv("AWAKEN_WEB_SEARCH_PROVIDERS", "tavily")
	got := Load()
	if got != (Snapshot{Exa: true}) {
		t.Fatalf("missing file must ignore leftover env and default to Exa: %+v", got)
	}
}

func TestLoadReadsUserDataFileAndPinsBothProviders(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWAKEN_USER_DATA_DIR", root)
	writeSearchFile(t, root, `{"version":1,"providers":{"exa":{"enabled":true},"tavily":{"enabled":true}}}`)
	got := Load()
	if !got.Exa || !got.Tavily || got.Error != "" || got.Providers() != "exa,tavily" {
		t.Fatalf("file snapshot: %+v providers=%q", got, got.Providers())
	}
}

func TestLoadReportsUnreadableUserDataFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWAKEN_USER_DATA_DIR", root)
	writeSearchFile(t, root, "{broken")
	got := Load()
	if got.Error == "" || got.Providers() != "" {
		t.Fatalf("corrupt file must fail closed: %+v", got)
	}
}

func TestLoadFallsBackToEnvironmentWithoutUserDataDir(t *testing.T) {
	t.Setenv("AWAKEN_USER_DATA_DIR", "")
	t.Setenv("AWAKEN_WEB_SEARCH_PROVIDERS", "")
	if got := Load(); got.Providers() != "" || got.Error != "" {
		t.Fatalf("empty env must disable search: %+v", got)
	}
	t.Setenv("AWAKEN_WEB_SEARCH_PROVIDERS", "tavily")
	if got := Load(); got != (Snapshot{Tavily: true}) {
		t.Fatalf("env tavily: %+v", got)
	}
	os.Unsetenv("AWAKEN_WEB_SEARCH_PROVIDERS")
	if got := Load(); got != (Snapshot{Exa: true}) {
		t.Fatalf("unset env must default to Exa: %+v", got)
	}
}

func TestParseKeepsExaDefaultWhenKeyOmitted(t *testing.T) {
	got, err := Parse([]byte(`{"version":1,"providers":{"tavily":{"enabled":true}}}`))
	if err != nil || !got.Exa || !got.Tavily {
		t.Fatalf("parse=%+v err=%v", got, err)
	}
}

func TestParseRejectsUnsupportedVersion(t *testing.T) {
	if _, err := Parse([]byte(`{"version":2,"providers":{}}`)); err == nil {
		t.Fatal("expected version error")
	}
}
