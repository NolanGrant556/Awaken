package main

import (
	"os"
	"testing"
)

func TestResolveDefaultWorkspaceUsesEnv(t *testing.T) {
	t.Setenv("AWAKEN_DEFAULT_WORKSPACE", "/tmp/awaken-workspace")

	if got := resolveDefaultWorkspace(); got != "/tmp/awaken-workspace" {
		t.Fatalf("expected workspace from env, got %q", got)
	}
}

func TestResolveDefaultWorkspaceFallsBackToCwd(t *testing.T) {
	t.Setenv("AWAKEN_DEFAULT_WORKSPACE", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if got := resolveDefaultWorkspace(); got != cwd {
		t.Fatalf("expected workspace from cwd %q, got %q", cwd, got)
	}
}
