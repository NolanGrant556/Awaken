package dialect

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"awaken/src/plugins/ir"
)

func TestSafeReadRejectsHardlinkedComponent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileInfo does not expose hardlink count")
	}
	root := t.TempDir()
	original := filepath.Join(t.TempDir(), "source.md")
	if err := os.WriteFile(original, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "linked.md")
	if err := os.Link(original, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := safeRead(root, "linked.md"); err == nil || !strings.Contains(err.Error(), "hardlink") {
		t.Fatalf("hardlink read err=%v", err)
	}
}

func TestScopedHookNeverFallsBackToPackageActivation(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindHook, LogicalID: "hook/scoped", Mapping: ir.MappingAdapted, Validation: "valid",
		SourceDialect: ir.DialectCursor,
		Hook:          &ir.HookSpec{SourceEvent: "sessionEnd", MappedEvent: "RunEnd", HandlerType: "command", Command: "true", Scope: "agent", OwnerComponentID: "agent/research"},
	}
	result := finalizeHookMappings([]ir.Component{component}, ir.DialectCursor)
	if result[0].Mapping != ir.MappingRecognizedUnsupported || len(result[0].Diagnostics) == 0 || result[0].Diagnostics[0].Code != "plugin_hook_scope_deferred" {
		t.Fatalf("scoped Hook was not kept out of package activation: %#v", result[0])
	}
}

func TestIgnoredFilesReportsTraversalFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "agents")); err != nil {
		t.Fatal(err)
	}

	components, diagnostics := ignoredFiles(
		root,
		"agents",
		map[string]bool{".md": true},
		ir.KindAgent,
		ir.DialectCodex,
		".codex-plugin/plugin.json",
		"not loaded",
	)
	if len(components) != 0 || len(diagnostics) != 1 || diagnostics[0].Code != "component_path_invalid" {
		t.Fatalf("components=%#v diagnostics=%#v", components, diagnostics)
	}
}
