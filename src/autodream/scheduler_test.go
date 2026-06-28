package autodream

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEvaluateDisabledGates(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		gates      EnabledGates
		wantReason string
	}{
		{
			name:       "kairos",
			gates:      EnabledGates{KairosActive: true, AutoMemoryEnabled: true, AutoDreamEnabled: true},
			wantReason: "kairos_active",
		},
		{
			name:       "remote",
			gates:      EnabledGates{RemoteMode: true, AutoMemoryEnabled: true, AutoDreamEnabled: true},
			wantReason: "remote_mode",
		},
		{
			name:       "auto memory disabled",
			gates:      EnabledGates{AutoDreamEnabled: true},
			wantReason: "auto_memory_disabled",
		},
		{
			name:       "auto dream disabled",
			gates:      EnabledGates{AutoMemoryEnabled: true},
			wantReason: "auto_dream_disabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := failIfCalledDeps(t)
			decision := Evaluate(Inputs{
				Now:   now,
				Gates: tc.gates,
			}, deps)

			if decision.Status != StatusDisabled {
				t.Fatalf("expected disabled, got %s", decision.Status)
			}
			if !strings.Contains(decision.Reason, tc.wantReason) {
				t.Fatalf("expected reason %q in %q", tc.wantReason, decision.Reason)
			}
		})
	}
}

func TestEvaluateTimeGateNotEnoughHours(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	deps := successDeps(now.Add(-2*time.Hour), []string{"a", "b"}, time.Time{})

	decision := Evaluate(openInputs(now), deps)

	if decision.Status != StatusTimeGateClosed {
		t.Fatalf("expected time gate closed, got %s", decision.Status)
	}
	if decision.HoursSince != 2 {
		t.Fatalf("expected 2 hours since last consolidation, got %v", decision.HoursSince)
	}
}

func TestEvaluateScanThrottled(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	inputs := openInputs(now)
	inputs.State.LastSessionScanAt = now.Add(-5 * time.Minute)
	deps := Dependencies{
		ReadLast: func() (time.Time, error) {
			return now.Add(-25 * time.Hour), nil
		},
		ListSessions: func(time.Time) ([]string, error) {
			t.Fatal("listSessions should not run while scan throttle is active")
			return nil, nil
		},
		AcquireLock: func() (LockAcquireResult, error) {
			t.Fatal("lockAcquire should not run while scan throttle is active")
			return LockAcquireResult{}, nil
		},
	}

	decision := Evaluate(inputs, deps)

	if decision.Status != StatusScanThrottled {
		t.Fatalf("expected scan throttled, got %s", decision.Status)
	}
}

func TestEvaluateNotEnoughSessions(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	inputs := openInputs(now)
	inputs.Config.MinSessions = 2
	deps := successDeps(now.Add(-25*time.Hour), []string{"session-a"}, time.Time{})

	decision := Evaluate(inputs, deps)

	if decision.Status != StatusNotEnoughSessions {
		t.Fatalf("expected not enough sessions, got %s", decision.Status)
	}
	if decision.SessionsCount != 1 {
		t.Fatalf("expected 1 session, got %d", decision.SessionsCount)
	}
	if !decision.NextState.LastSessionScanAt.Equal(now) {
		t.Fatalf("expected scan timestamp to advance to now")
	}
}

func TestEvaluateExcludesCurrentSession(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	inputs := openInputs(now)
	inputs.Config.MinSessions = 2
	inputs.CurrentSessionID = "current"
	deps := successDeps(now.Add(-25*time.Hour), []string{"current", "older"}, time.Time{})

	decision := Evaluate(inputs, deps)

	if decision.Status != StatusNotEnoughSessions {
		t.Fatalf("expected current session exclusion to close session gate, got %s", decision.Status)
	}
	if got := strings.Join(decision.SessionIDs, ","); got != "older" {
		t.Fatalf("expected only older session, got %q", got)
	}
}

func TestEvaluateLockBusy(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	inputs := openInputs(now)
	inputs.Config.MinSessions = 2
	deps := successDeps(now.Add(-25*time.Hour), []string{"a", "b"}, time.Time{})
	deps.AcquireLock = func() (LockAcquireResult, error) {
		return LockAcquireResult{Acquired: false}, nil
	}

	decision := Evaluate(inputs, deps)

	if decision.Status != StatusLockBusy {
		t.Fatalf("expected lock busy, got %s", decision.Status)
	}
	if decision.Plan != nil {
		t.Fatal("expected no action plan when lock is busy")
	}
}

func TestEvaluateForcedStillNeedsLock(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	inputs := openInputs(now)
	inputs.Forced = true
	inputs.Gates = EnabledGates{}
	inputs.Config.MinSessions = 5
	deps := successDeps(now.Add(-1*time.Hour), []string{"one"}, time.Time{})
	deps.AcquireLock = func() (LockAcquireResult, error) {
		return LockAcquireResult{Acquired: false}, nil
	}

	decision := Evaluate(inputs, deps)

	if decision.Status != StatusLockBusy {
		t.Fatalf("expected forced run to reach lock and stop on busy lock, got %s", decision.Status)
	}
	if decision.SessionsCount != 1 {
		t.Fatalf("expected forced run to keep scanned sessions for prompt context, got %d", decision.SessionsCount)
	}
}

func TestEvaluateFirePlanPromptContainsConstraintsAndSessions(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	priorMtime := now.Add(-25 * time.Hour)
	inputs := openInputs(now)
	inputs.Config.MinSessions = 2
	inputs.CurrentSessionID = "current"
	inputs.MemoryRoot = "/tmp/memory"
	inputs.TranscriptDir = "/tmp/transcripts"
	deps := successDeps(priorMtime, []string{"session-a", "current", "session-b"}, priorMtime)

	decision := Evaluate(inputs, deps)

	if !decision.ShouldFire() {
		t.Fatalf("expected fire, got %s: %s", decision.Status, decision.Reason)
	}
	if !decision.PriorMtime.Equal(priorMtime) {
		t.Fatalf("expected prior mtime %s, got %s", priorMtime, decision.PriorMtime)
	}
	if decision.Plan == nil {
		t.Fatal("expected action plan")
	}
	plan := decision.Plan
	if plan.RegisterTask.SessionsReviewing != 2 || !plan.RegisterTask.PriorMtime.Equal(priorMtime) {
		t.Fatalf("unexpected register task plan: %+v", plan.RegisterTask)
	}
	if plan.ForkedAgent.QuerySource != "auto_dream" {
		t.Fatalf("expected auto_dream query source, got %q", plan.ForkedAgent.QuerySource)
	}
	if !plan.ForkedAgent.SkipTranscript {
		t.Fatal("expected forked agent to skip transcript")
	}
	if plan.ForkedAgent.CanUseTool.Mode != "restricted_to_memory_root" || plan.ForkedAgent.CanUseTool.MemoryRoot != "/tmp/memory" {
		t.Fatalf("unexpected tool restriction: %+v", plan.ForkedAgent.CanUseTool)
	}
	if plan.Completion.OnComplete == "" || plan.Completion.OnFail == "" || plan.Completion.OnRollback == "" {
		t.Fatalf("expected complete/fail/rollback summaries, got %+v", plan.Completion)
	}

	prompt := decision.Prompt
	for _, want := range []string{
		"Memory directory: `/tmp/memory`",
		"Session transcripts: `/tmp/transcripts`",
		"## Phase 1",
		"## Phase 2",
		"## Phase 3",
		"## Phase 4",
		"Bash is restricted to read-only commands",
		"grep narrowly",
		"don't read whole transcripts",
		"under MAX lines (200)",
		"Sessions since last consolidation (2):",
		"- session-a",
		"- session-b",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "- current") {
		t.Fatalf("expected current session to be excluded from prompt:\n%s", prompt)
	}
	if plan.ForkedAgent.Prompt != prompt {
		t.Fatal("expected plan prompt to match decision prompt")
	}
}

func TestEvaluateFailureReasonMapping(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		deps       Dependencies
		want       Status
		wantReason string
	}{
		{
			name: "read last",
			deps: Dependencies{
				ReadLast: func() (time.Time, error) {
					return time.Time{}, errors.New("readLast failed")
				},
			},
			want:       StatusReadLastError,
			wantReason: "readLast failed",
		},
		{
			name: "list sessions",
			deps: Dependencies{
				ReadLast: func() (time.Time, error) {
					return now.Add(-25 * time.Hour), nil
				},
				ListSessions: func(time.Time) ([]string, error) {
					return nil, errors.New("listSessions failed")
				},
			},
			want:       StatusListSessionsError,
			wantReason: "listSessions failed",
		},
		{
			name: "lock acquire",
			deps: Dependencies{
				ReadLast: func() (time.Time, error) {
					return now.Add(-25 * time.Hour), nil
				},
				ListSessions: func(time.Time) ([]string, error) {
					return []string{"a", "b"}, nil
				},
				AcquireLock: func() (LockAcquireResult, error) {
					return LockAcquireResult{}, errors.New("lockAcquire failed")
				},
			},
			want:       StatusLockAcquireError,
			wantReason: "lockAcquire failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputs := openInputs(now)
			inputs.Config.MinSessions = 2
			decision := Evaluate(inputs, tc.deps)

			if decision.Status != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, decision.Status)
			}
			if decision.Reason != tc.wantReason {
				t.Fatalf("expected reason %q, got %q", tc.wantReason, decision.Reason)
			}
		})
	}
}

func openInputs(now time.Time) Inputs {
	return Inputs{
		Now: now,
		Config: Config{
			MinHours:     24,
			MinSessions:  2,
			ScanThrottle: DefaultScanThrottle,
		},
		Gates: EnabledGates{
			AutoMemoryEnabled: true,
			AutoDreamEnabled:  true,
		},
		CurrentSessionID: "current",
		MemoryRoot:       "/memory",
		TranscriptDir:    "/transcripts",
	}
}

func successDeps(last time.Time, sessions []string, priorMtime time.Time) Dependencies {
	return Dependencies{
		ReadLast: func() (time.Time, error) {
			return last, nil
		},
		ListSessions: func(time.Time) ([]string, error) {
			return sessions, nil
		},
		AcquireLock: func() (LockAcquireResult, error) {
			return LockAcquireResult{Acquired: true, PriorMtime: priorMtime}, nil
		},
	}
}

func failIfCalledDeps(t *testing.T) Dependencies {
	t.Helper()
	return Dependencies{
		ReadLast: func() (time.Time, error) {
			t.Fatal("readLast should not be called")
			return time.Time{}, nil
		},
		ListSessions: func(time.Time) ([]string, error) {
			t.Fatal("listSessions should not be called")
			return nil, nil
		},
		AcquireLock: func() (LockAcquireResult, error) {
			t.Fatal("lockAcquire should not be called")
			return LockAcquireResult{}, nil
		},
	}
}
