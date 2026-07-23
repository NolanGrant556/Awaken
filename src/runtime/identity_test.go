package runtime

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"awaken/src/filestore"
)

func TestResolveRuntimeManifestPaths(t *testing.T) {
	root := t.TempDir()
	paths := ResolveRuntimeManifestPaths(root)
	if paths.Dir != filepath.Join(root, "runtime") {
		t.Fatalf("dir mismatch: %s", paths.Dir)
	}
	if paths.Manifest != filepath.Join(paths.Dir, "core.json") {
		t.Fatalf("manifest mismatch: %s", paths.Manifest)
	}
	if paths.Lock != filepath.Join(paths.Dir, "core.lock") {
		t.Fatalf("lock mismatch: %s", paths.Lock)
	}
}

func TestGenerateRuntimeIdentity(t *testing.T) {
	root := t.TempDir()
	id, err := GenerateRuntimeIdentity(root, "headless", "http://127.0.0.1:12345")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if id.SchemaVersion != 1 {
		t.Fatalf("schema version = %d", id.SchemaVersion)
	}
	if id.RuntimeIdentity == "" {
		t.Fatal("missing runtime_identity")
	}
	if id.InstanceID == "" {
		t.Fatal("missing instance_id")
	}
	if id.PID != os.Getpid() {
		t.Fatalf("pid = %d", id.PID)
	}
	if id.OwnerMode != "headless" {
		t.Fatalf("owner_mode = %s", id.OwnerMode)
	}
	if id.BaseURL != "http://127.0.0.1:12345" {
		t.Fatalf("base_url = %s", id.BaseURL)
	}
	if len(id.ControlToken) != 64 {
		t.Fatalf("control token length = %d", len(id.ControlToken))
	}
}

func TestGenerateRuntimeIdentityRejectsInvalidInputs(t *testing.T) {
	root := t.TempDir()
	if _, err := GenerateRuntimeIdentity("", "headless", "http://127.0.0.1:1"); err == nil {
		t.Fatal("expected error for empty userDataDir")
	}
	if _, err := GenerateRuntimeIdentity(root, "invalid", "http://127.0.0.1:1"); err == nil {
		t.Fatal("expected error for invalid owner mode")
	}
	if _, err := GenerateRuntimeIdentity(root, "headless", "http://localhost:1"); err == nil {
		t.Fatal("expected error for localhost")
	}
	if _, err := GenerateRuntimeIdentity(root, "headless", "http://127.0.0.1"); err == nil {
		t.Fatal("expected error for missing port")
	}
	if _, err := GenerateRuntimeIdentity(root, "headless", "http://192.168.1.1:1"); err == nil {
		t.Fatal("expected error for non-loopback")
	}
}

func TestManifestReadWriteRoundTrip(t *testing.T) {
	root := t.TempDir()
	id, err := GenerateRuntimeIdentity(root, "desktop", "http://127.0.0.1:9999")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := WriteManifest(root, id); err != nil {
		t.Fatalf("write: %v", err)
	}
	paths := ResolveRuntimeManifestPaths(root)
	info, err := os.Stat(paths.Manifest)
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	if goruntime.GOOS != "windows" && info.Mode().Perm() != filestore.FilePerm {
		t.Fatalf("manifest mode = %o", info.Mode().Perm())
	}

	loaded, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if loaded.InstanceID != id.InstanceID {
		t.Fatalf("instance_id mismatch")
	}
	if loaded.ControlToken != id.ControlToken {
		t.Fatal("control_token mismatch")
	}

	if err := ValidateManifest(loaded, id.RuntimeIdentity); err != nil {
		t.Fatalf("validate loaded: %v", err)
	}
}

func TestValidateManifestDetectsMismatch(t *testing.T) {
	root := t.TempDir()
	id, _ := GenerateRuntimeIdentity(root, "desktop", "http://127.0.0.1:1")
	if err := ValidateManifest(id, "other"); err == nil {
		t.Fatal("expected identity mismatch error")
	}
	id2 := *id
	id2.SchemaVersion = 99
	if err := ValidateManifest(&id2, id.RuntimeIdentity); err == nil {
		t.Fatal("expected schema version error")
	}
	id3 := *id
	id3.ControlToken = ""
	if err := ValidateManifest(&id3, id.RuntimeIdentity); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestValidateControlToken(t *testing.T) {
	root := t.TempDir()
	id, _ := GenerateRuntimeIdentity(root, "desktop", "http://127.0.0.1:1")
	if !ValidateControlToken(id, id.ControlToken) {
		t.Fatal("valid token rejected")
	}
	if ValidateControlToken(id, id.ControlToken+"x") {
		t.Fatal("invalid length token accepted")
	}
	if ValidateControlToken(id, "") {
		t.Fatal("empty token accepted")
	}
	if ValidateControlToken(nil, id.ControlToken) {
		t.Fatal("nil identity accepted")
	}
}

func TestAcquireRuntimeLock(t *testing.T) {
	root := t.TempDir()
	lock, err := AcquireRuntimeLock(root, "owner-a", 5*time.Second)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer lock.Release()

	// Same owner can re-acquire after releasing.
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	lock2, err := AcquireRuntimeLock(root, "owner-b", 5*time.Second)
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	lock2.Release()
}

func TestDeleteManifest(t *testing.T) {
	root := t.TempDir()
	id, _ := GenerateRuntimeIdentity(root, "desktop", "http://127.0.0.1:1")
	WriteManifest(root, id)
	if err := DeleteManifest(root); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := ReadManifest(root); !os.IsNotExist(err) {
		t.Fatalf("expected not exist, got %v", err)
	}
	// Double delete is safe.
	if err := DeleteManifest(root); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}
