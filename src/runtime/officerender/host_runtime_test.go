package officerender

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeHostSoffice(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "soffice")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHostRuntimeResolverUsesExistingSoffice(t *testing.T) {
	soffice := writeHostSoffice(t)
	t.Setenv("AWAKEN_SOFFICE", soffice)
	resolver := NewHostRuntimeResolver()
	resolved, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantSoffice, _ := filepath.EvalSymlinks(soffice)
	if resolved.SOfficePath != wantSoffice || resolved.Root != filepath.Dir(wantSoffice) {
		t.Fatalf("unexpected host runtime: %#v", resolved)
	}
	if resolved.Revision == "" || resolved.Attestation == "" {
		t.Fatalf("host executable must carry an invocation fingerprint: %#v", resolved)
	}
}

func TestHostRuntimeResolverDetectsChangedExecutable(t *testing.T) {
	soffice := writeHostSoffice(t)
	t.Setenv("AWAKEN_SOFFICE", soffice)
	resolver := NewHostRuntimeResolver()
	resolved, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(soffice, []byte("#!/bin/sh\necho changed\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := resolver.Attest(context.Background(), resolved); err == nil {
		t.Fatal("expected changed executable to invalidate invocation fingerprint")
	}
}
