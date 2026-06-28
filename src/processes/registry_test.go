package processes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"
)

func TestRegistryStartListAndReadBackgroundLog(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(RegistryConfig{})
	ctx := context.Background()

	session, err := registry.Start(ctx, StartOptions{
		Command:           "printf ready; sleep 5",
		Background:        true,
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() { _, _ = registry.Kill(context.Background(), session.ID) }()

	if session.Mode != ModePipe || session.Status != StatusRunning {
		t.Fatalf("unexpected session: %+v", session)
	}

	sessions := registry.List()
	if len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("expected listed session %q, got %+v", session.ID, sessions)
	}

	log := waitForLog(t, registry, session.ID, "ready")
	if log.Stdout != "ready" || log.Stderr != "" {
		t.Fatalf("unexpected log: %+v", log)
	}
}

func TestRegistryWriteStdinToPipeProcess(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(RegistryConfig{})

	session, err := registry.Start(context.Background(), StartOptions{
		Command:           "cat",
		Background:        true,
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() { _, _ = registry.Kill(context.Background(), session.ID) }()

	n, err := registry.WriteStdin(context.Background(), session.ID, "hello from stdin\n")
	if err != nil {
		t.Fatalf("WriteStdin returned error: %v", err)
	}
	if n != len("hello from stdin\n") {
		t.Fatalf("expected %d written bytes, got %d", len("hello from stdin\n"), n)
	}

	log := waitForLog(t, registry, session.ID, "hello from stdin")
	if !strings.Contains(log.Stdout, "hello from stdin") {
		t.Fatalf("expected stdin echo in stdout, got %+v", log)
	}
}

func TestRegistryKillLongRunningProcess(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(RegistryConfig{})

	session, err := registry.Start(context.Background(), StartOptions{
		Command:           "sleep 30",
		Background:        true,
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	killed, err := registry.Kill(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Kill returned error: %v", err)
	}
	if killed.Status != StatusKilled {
		t.Fatalf("expected killed status, got %+v", killed)
	}

	waited, err := registry.Wait(context.Background(), session.ID, time.Second)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if waited.TimedOut || waited.Session.Status != StatusKilled {
		t.Fatalf("unexpected wait result: %+v", waited)
	}
}

func TestRegistryCapsLogs(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(RegistryConfig{MaxLogChars: 80})

	session, err := registry.Start(context.Background(), StartOptions{
		Command:           "i=0; while [ $i -lt 200 ]; do printf abcdefghij; i=$((i+1)); done",
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if session.Status != StatusExited {
		t.Fatalf("expected exited session, got %+v", session)
	}

	log, err := registry.GetLog(session.ID, 0)
	if err != nil {
		t.Fatalf("GetLog returned error: %v", err)
	}
	if !log.Truncated || !strings.Contains(log.Stdout, "truncated") || len([]rune(log.Stdout)) > 140 {
		t.Fatalf("expected capped stdout log, got truncated=%v len=%d stdout=%q", log.Truncated, len([]rune(log.Stdout)), log.Stdout)
	}
}

func TestRegistryRejectsWorkdirOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	registry := NewRegistry(RegistryConfig{})

	_, err := registry.Start(context.Background(), StartOptions{
		Command:           "pwd",
		Workdir:           outside,
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err == nil || !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected outside workspace error, got %v", err)
	}
}

func TestRegistryDoesNotInheritSensitiveEnvByDefaultOrAllowlist(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(RegistryConfig{})
	t.Setenv("AWAKEN_PROCESS_TEST_SECRET", "please-do-not-leak")

	session, err := registry.Start(context.Background(), StartOptions{
		Command:           "printenv AWAKEN_PROCESS_TEST_SECRET",
		EnvAllowlist:      []string{"AWAKEN_PROCESS_TEST_SECRET"},
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if session.Status != StatusExited {
		t.Fatalf("expected exited session, got %+v", session)
	}

	log, err := registry.GetLog(session.ID, 0)
	if err != nil {
		t.Fatalf("GetLog returned error: %v", err)
	}
	if strings.Contains(log.Stdout, "please-do-not-leak") || strings.Contains(log.Combined, "please-do-not-leak") {
		t.Fatalf("sensitive env leaked into process output: %+v", log)
	}
	if len(session.EnvAllowlist) != 0 {
		t.Fatalf("expected sensitive env allowlist entry to be filtered from snapshot, got %+v", session.EnvAllowlist)
	}
}

func TestRegistryRejectsFileWorkdir(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	registry := NewRegistry(RegistryConfig{})

	_, err := registry.Start(context.Background(), StartOptions{
		Command:           "pwd",
		Workdir:           file,
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not a directory error, got %v", err)
	}
}

func TestRegistryWriteRejectsExitedProcess(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(RegistryConfig{})

	session, err := registry.Start(context.Background(), StartOptions{
		Command:           "printf done",
		PermissionContext: tool.NewPermissionContext(root, nil),
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	_, err = registry.WriteStdin(context.Background(), session.ID, "late\n")
	if !errors.Is(err, ErrSessionExited) {
		t.Fatalf("expected ErrSessionExited, got %v", err)
	}
}

func waitForLog(t *testing.T, registry *Registry, sessionID string, want string) ProcessLog {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		log, err := registry.GetLog(sessionID, 0)
		if err != nil {
			t.Fatalf("GetLog returned error: %v", err)
		}
		if strings.Contains(log.Stdout, want) || strings.Contains(log.Stderr, want) || strings.Contains(log.Combined, want) {
			return log
		}
		time.Sleep(10 * time.Millisecond)
	}
	log, _ := registry.GetLog(sessionID, 0)
	t.Fatalf("timed out waiting for %q in log: %+v", want, log)
	return ProcessLog{}
}
