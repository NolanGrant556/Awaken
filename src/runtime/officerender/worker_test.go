package officerender

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func writeFakeSoffice(t *testing.T, root string) Runtime {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed fake soffice is exercised on Unix; Windows is cross-compiled separately")
	}
	soffice := filepath.Join(root, "libreoffice", "program", "soffice")
	if err := os.MkdirAll(filepath.Dir(soffice), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
format=""
outdir=""
input=""
profile=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --convert-to)
      shift
      format="$1"
      ;;
    --outdir)
      shift
      outdir="$1"
      ;;
    -env:UserInstallation=*)
      profile="${1#-env:UserInstallation=}"
      ;;
    --*)
      ;;
    *)
      input="$1"
      ;;
  esac
  shift
done
if [ -n "${AWAKEN_FAKE_COUNT_FILE:-}" ]; then
  printf 'run\n' >> "$AWAKEN_FAKE_COUNT_FILE"
fi
if [ -n "${AWAKEN_FAKE_PROFILE_FILE:-}" ]; then
  printf '%s\n' "$profile" >> "$AWAKEN_FAKE_PROFILE_FILE"
fi
owns_lock=0
if [ -n "${AWAKEN_FAKE_LOCK_DIR:-}" ]; then
  if mkdir "$AWAKEN_FAKE_LOCK_DIR" 2>/dev/null; then
    owns_lock=1
  else
    printf 'overlap\n' > "${AWAKEN_FAKE_LOCK_DIR}.overlap"
  fi
fi
cleanup() {
  if [ "$owns_lock" = "1" ]; then
    rmdir "$AWAKEN_FAKE_LOCK_DIR" 2>/dev/null || true
  fi
}
trap cleanup EXIT
if [ -n "${AWAKEN_FAKE_CHILD_PID_FILE:-}" ]; then
  (trap '' TERM; while :; do sleep 1; done) &
  child_pid=$!
  printf '%s\n' "$child_pid" > "$AWAKEN_FAKE_CHILD_PID_FILE"
  wait "$child_pid"
fi
if [ -n "${AWAKEN_FAKE_DELAY:-}" ]; then
  sleep "$AWAKEN_FAKE_DELAY"
fi
if [ "${AWAKEN_FAKE_FAIL:-}" = "1" ]; then
  printf 'forced failure\n' >&2
  exit 9
fi
name=$(basename "$input")
stem=${name%.*}
extension=${format%%:*}
mkdir -p "$outdir"
printf 'rendered:%s\n' "$input" > "$outdir/$stem.$extension"
`
	if err := os.WriteFile(soffice, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return Runtime{Revision: "revision-1", Root: root, SOfficePath: soffice}
}

func writeInput(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("office input"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(payload))
}

func TestRenderSingleflightAndRevisionAttestationCache(t *testing.T) {
	runtimeRoot := t.TempDir()
	runtimeSpec := writeFakeSoffice(t, runtimeRoot)
	workspace := t.TempDir()
	input := writeInput(t, workspace, "deck.pptx")
	output := filepath.Join(workspace, "deck.pdf")
	countFile := filepath.Join(t.TempDir(), "count")
	profileFile := filepath.Join(t.TempDir(), "profiles")
	var attestations atomic.Int32
	worker := NewOfficeRenderWorker(Config{
		Environment: []string{
			"AWAKEN_FAKE_COUNT_FILE=" + countFile,
			"AWAKEN_FAKE_PROFILE_FILE=" + profileFile,
			"AWAKEN_FAKE_DELAY=0.2",
		},
		Attest: func(context.Context, Runtime) error {
			attestations.Add(1)
			return nil
		},
	})
	job := Job{Runtime: runtimeSpec, InputPath: input, OutputPath: output, Format: "pdf"}

	results := make([]Result, 2)
	errorsByCall := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errorsByCall[index] = worker.Render(context.Background(), job)
		}(index)
	}
	close(start)
	wait.Wait()
	for _, err := range errorsByCall {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(readLines(t, countFile)) != 1 {
		t.Fatalf("identical concurrent jobs must execute soffice once")
	}
	if attestations.Load() != 1 {
		t.Fatalf("runtime revision must be attested once, got %d", attestations.Load())
	}
	shared := 0
	for _, result := range results {
		if result.Shared {
			shared++
		}
	}
	if shared != 1 {
		t.Fatalf("expected exactly one singleflight follower, got %d", shared)
	}
	if profiles := readLines(t, profileFile); len(profiles) != 1 || !strings.HasPrefix(profiles[0], "file:") {
		t.Fatalf("expected one isolated profile URL, got %v", profiles)
	}
	if payload, err := os.ReadFile(output); err != nil || !strings.Contains(string(payload), "rendered:") {
		t.Fatalf("expected committed output, payload=%q err=%v", payload, err)
	}

	secondInput := writeInput(t, workspace, "sheet.xlsx")
	secondOutput := filepath.Join(workspace, "sheet.pdf")
	if _, err := worker.Render(context.Background(), Job{
		Runtime: runtimeSpec, InputPath: secondInput, OutputPath: secondOutput, Format: "pdf",
	}); err != nil {
		t.Fatal(err)
	}
	if attestations.Load() != 1 {
		t.Fatalf("same immutable revision must reuse attestation, got %d", attestations.Load())
	}
	profiles := readLines(t, profileFile)
	if len(profiles) != 2 || profiles[0] == profiles[1] {
		t.Fatalf("each executed job must use a distinct profile, got %v", profiles)
	}
}

func TestDefaultConcurrencyIsOne(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	lockDir := filepath.Join(t.TempDir(), "active")
	worker := NewOfficeRenderWorker(Config{Environment: []string{
		"AWAKEN_FAKE_LOCK_DIR=" + lockDir,
		"AWAKEN_FAKE_DELAY=0.2",
	}})
	jobs := []Job{
		{Runtime: runtimeSpec, InputPath: writeInput(t, workspace, "one.pptx"), OutputPath: filepath.Join(workspace, "one.pdf"), Format: "pdf"},
		{Runtime: runtimeSpec, InputPath: writeInput(t, workspace, "two.pptx"), OutputPath: filepath.Join(workspace, "two.pdf"), Format: "pdf"},
	}
	start := make(chan struct{})
	errorsByJob := make([]error, len(jobs))
	var wait sync.WaitGroup
	for index := range jobs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errorsByJob[index] = worker.Render(context.Background(), jobs[index])
		}(index)
	}
	close(start)
	wait.Wait()
	for _, err := range errorsByJob {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(lockDir + ".overlap"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default worker allowed overlapping soffice processes")
	}
}

func TestFiftySequentialConversionsLeaveNoProfilesOrStaging(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	worker := NewOfficeRenderWorker(Config{TempRoot: profileRoot})
	for index := 0; index < 50; index++ {
		name := fmt.Sprintf("deck-%02d.pptx", index)
		input := writeInput(t, workspace, name)
		output := filepath.Join(workspace, strings.TrimSuffix(name, ".pptx")+".pdf")
		if _, err := worker.Render(context.Background(), Job{
			Runtime:    runtimeSpec,
			InputPath:  input,
			OutputPath: output,
			Format:     "pdf",
		}); err != nil {
			t.Fatalf("conversion %d failed: %v", index+1, err)
		}
	}
	profiles, err := os.ReadDir(profileRoot)
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profiles must be removed after 50 conversions, entries=%v err=%v", profiles, err)
	}
	staging, err := filepath.Glob(filepath.Join(workspace, ".awaken-office-render-*"))
	if err != nil || len(staging) != 0 {
		t.Fatalf("staging must be removed after 50 conversions, entries=%v err=%v", staging, err)
	}
}

func TestAtomicOutputPreservesExistingFileOnFailure(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	input := writeInput(t, workspace, "deck.pptx")
	output := filepath.Join(workspace, "deck.pdf")
	if err := os.WriteFile(output, []byte("old-output"), 0o644); err != nil {
		t.Fatal(err)
	}
	worker := NewOfficeRenderWorker(Config{Environment: []string{"AWAKEN_FAKE_FAIL=1"}})
	if _, err := worker.Render(context.Background(), Job{
		Runtime: runtimeSpec, InputPath: input, OutputPath: output, Format: "pdf",
	}); err == nil {
		t.Fatal("expected conversion failure")
	}
	payload, err := os.ReadFile(output)
	if err != nil || string(payload) != "old-output" {
		t.Fatalf("failed render must preserve prior output, payload=%q err=%v", payload, err)
	}
	staging, err := filepath.Glob(filepath.Join(workspace, ".awaken-office-render-*"))
	if err != nil || len(staging) != 0 {
		t.Fatalf("staging directories must be cleaned, got %v err=%v", staging, err)
	}
}

func TestRevisionCacheRejectsChangedRuntimeIdentity(t *testing.T) {
	root := t.TempDir()
	first := writeFakeSoffice(t, root)
	workspace := t.TempDir()
	worker := NewOfficeRenderWorker(Config{})
	if _, err := worker.Render(context.Background(), Job{
		Runtime:    first,
		InputPath:  writeInput(t, workspace, "first.pptx"),
		OutputPath: filepath.Join(workspace, "first.pdf"),
		Format:     "pdf",
	}); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "other", "soffice")
	if err := os.MkdirAll(filepath.Dir(secondPath), 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(first.SOfficePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, payload, 0o755); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SOfficePath = secondPath
	_, err = worker.Render(context.Background(), Job{
		Runtime:    second,
		InputPath:  writeInput(t, workspace, "second.pptx"),
		OutputPath: filepath.Join(workspace, "second.pdf"),
		Format:     "pdf",
	})
	if !errors.Is(err, ErrRuntimeAttestation) {
		t.Fatalf("expected changed identity attestation failure, got %v", err)
	}
}

func TestRejectsMismatchedOutputFormat(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	_, err := NewOfficeRenderWorker(Config{}).Render(context.Background(), Job{
		Runtime:    runtimeSpec,
		InputPath:  writeInput(t, workspace, "deck.pptx"),
		OutputPath: filepath.Join(workspace, "deck.xlsx"),
		Format:     "pdf",
	})
	if !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("expected invalid job, got %v", err)
	}
}

func TestOfficeEnvironmentExcludesAmbientSecrets(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "must-not-leak")
	t.Setenv("AWAKEN_INTERNAL_TOKEN", "must-not-leak")
	environment := officeProcessEnvironment([]string{"AWAKEN_FAKE_DELAY=0.1"})
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "OPENAI_API_KEY") || strings.Contains(joined, "AWAKEN_INTERNAL_TOKEN") {
		t.Fatalf("office process inherited ambient secrets: %s", joined)
	}
	if !strings.Contains(joined, "AWAKEN_FAKE_DELAY=0.1") {
		t.Fatalf("explicit worker environment override was lost: %s", joined)
	}
	if !strings.Contains(joined, "PYTHONDONTWRITEBYTECODE=1") {
		t.Fatalf("office process may mutate its sealed embedded Python runtime: %s", joined)
	}
}

func TestFollowerCancellationDoesNotCancelLeader(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	job := Job{
		Runtime:    runtimeSpec,
		InputPath:  writeInput(t, workspace, "deck.pptx"),
		OutputPath: filepath.Join(workspace, "deck.pdf"),
		Format:     "pdf",
	}
	worker := NewOfficeRenderWorker(Config{Environment: []string{"AWAKEN_FAKE_DELAY=0.3"}})
	leaderDone := make(chan error, 1)
	go func() {
		_, err := worker.Render(context.Background(), job)
		leaderDone <- err
	}()
	time.Sleep(50 * time.Millisecond)
	followerContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := worker.Render(followerContext, job); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected follower deadline, got %v", err)
	}
	if err := <-leaderDone; err != nil {
		t.Fatalf("follower cancellation must not cancel leader: %v", err)
	}
}

func TestCallerDeadlineIsNotReportedAsWorkerTimeout(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	worker := NewOfficeRenderWorker(Config{
		Timeout:     30 * time.Second,
		Environment: []string{"AWAKEN_FAKE_DELAY=5"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := worker.Render(ctx, Job{
		Runtime:    runtimeSpec,
		InputPath:  writeInput(t, workspace, "caller-deadline.pptx"),
		OutputPath: filepath.Join(workspace, "caller-deadline.pdf"),
		Format:     "pdf",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected caller deadline, got %v", err)
	}
	if errors.Is(err, ErrRenderTimeout) {
		t.Fatalf("caller deadline must not be reported as worker timeout: %v", err)
	}
}

func TestLeaderCancellationDoesNotCancelActiveFollower(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	countFile := filepath.Join(t.TempDir(), "count")
	job := Job{
		Runtime:    runtimeSpec,
		InputPath:  writeInput(t, workspace, "deck.pptx"),
		OutputPath: filepath.Join(workspace, "deck.pdf"),
		Format:     "pdf",
	}
	worker := NewOfficeRenderWorker(Config{Environment: []string{
		"AWAKEN_FAKE_COUNT_FILE=" + countFile,
		"AWAKEN_FAKE_DELAY=0.4",
	}})
	leaderContext, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := worker.Render(leaderContext, job)
		leaderDone <- err
	}()
	// Whole-repository test runs compile and execute many packages in parallel;
	// allow the fake soffice process to be scheduled without weakening any
	// production render deadline or the assertions below.
	waitForPath(t, countFile, 10*time.Second)
	followerDone := make(chan error, 1)
	go func() {
		_, err := worker.Render(context.Background(), job)
		followerDone <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancelLeader()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected leader cancellation, got %v", err)
	}
	if err := <-followerDone; err != nil {
		t.Fatalf("active follower must keep shared render alive: %v", err)
	}
	if len(readLines(t, countFile)) != 1 {
		t.Fatal("leader/follower singleflight must execute soffice once")
	}
}

func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("path was not created within %s: %s", timeout, path)
}
