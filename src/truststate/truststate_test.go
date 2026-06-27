package truststate

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateNoRiskDefaultsTrustedAndPolicyCanRequireReview(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

	trusted := Evaluate(WorkspaceInput{
		WorkspaceRoot: "/workspace",
		Now:           now,
	})
	if trusted.Decision != DecisionTrusted {
		t.Fatalf("expected no-risk workspace to be trusted, got %q", trusted.Decision)
	}
	if trusted.Acceptance.Status != AcceptanceNotPresent {
		t.Fatalf("expected no acceptance status, got %q", trusted.Acceptance.Status)
	}

	review := Evaluate(WorkspaceInput{
		WorkspaceRoot: "/workspace",
		Now:           now,
		Policy: Policy{
			NoSignalDecision: DecisionNeedsReview,
		},
	})
	if review.Decision != DecisionNeedsReview {
		t.Fatalf("expected policy to require review, got %q", review.Decision)
	}
	if got := strings.Join(review.Reasons, "\n"); !strings.Contains(got, "policy requires workspace trust review") {
		t.Fatalf("expected policy reason, got %q", got)
	}
}

func TestDangerousSignalTriggersReview(t *testing.T) {
	state := Evaluate(WorkspaceInput{
		WorkspaceRoot: "/workspace",
		Now:           time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		Signals: []RiskSignal{{
			Kind:   SignalDangerousEnvVar,
			Source: "env",
			Detail: "DANGEROUSLY_SKIP_PERMISSIONS=true",
		}},
	})

	if state.Decision != DecisionNeedsReview {
		t.Fatalf("expected dangerous env var to require review, got %q", state.Decision)
	}
	if len(state.Signals) != 1 || state.Signals[0].Severity != SeverityHigh {
		t.Fatalf("expected dangerous env var to normalize as high severity, got %#v", state.Signals)
	}
	summary := state.Summary()
	if summary.Signals[0].Detail != "DANGEROUSLY_SKIP_PERMISSIONS=<redacted>" {
		t.Fatalf("expected redacted summary detail, got %q", summary.Signals[0].Detail)
	}
}

func TestHighRiskCombinationSortsDeterministically(t *testing.T) {
	state := Evaluate(WorkspaceInput{
		WorkspaceRoot: "/workspace",
		Now:           time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		Signals: []RiskSignal{
			{Kind: SignalProjectSettingsPresent, Source: ".awaken/settings.json"},
			{Kind: SignalRemoteGatewayCaller, Source: "gateway"},
			{Kind: SignalLocalExecutionBackend, Source: "local"},
			{Kind: SignalHooksPresent, Source: ".claude/hooks.json"},
			{Kind: SignalLocalSettingsPresent, Severity: SeverityCritical, Source: "~/.awaken/settings.json"},
		},
	})

	want := []RiskSignalKind{
		SignalLocalSettingsPresent,
		SignalHooksPresent,
		SignalLocalExecutionBackend,
		SignalRemoteGatewayCaller,
		SignalProjectSettingsPresent,
	}
	if len(state.Signals) != len(want) {
		t.Fatalf("expected %d signals, got %d", len(want), len(state.Signals))
	}
	for i, wantKind := range want {
		if state.Signals[i].Kind != wantKind {
			t.Fatalf("signal %d kind mismatch: got %q want %q; full order %#v", i, state.Signals[i].Kind, wantKind, state.Signals)
		}
	}
	if state.Decision != DecisionUntrusted {
		t.Fatalf("expected critical signal to be untrusted without acceptance, got %q", state.Decision)
	}
}

func TestAcceptanceStillValidExpiredAndWorkspaceMismatch(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	base := WorkspaceInput{
		WorkspaceRoot: "/workspace",
		Revision:      "rev-a",
		Now:           now,
		Signals: []RiskSignal{{
			Kind:   SignalHooksPresent,
			Source: ".claude/hooks.json",
		}},
	}
	fingerprint := ComputeFingerprint(base)

	t.Run("valid", func(t *testing.T) {
		input := base
		input.Acceptance = &Acceptance{
			AcceptedAt:    now.Add(-time.Hour),
			AcceptedBy:    "operator@example.com",
			WorkspaceRoot: "/workspace",
			Fingerprint:   fingerprint,
			Revision:      "rev-a",
			ExpiresAt:     now.Add(time.Hour),
		}
		state := Evaluate(input)
		if state.Decision != DecisionTrusted {
			t.Fatalf("expected valid acceptance to trust workspace, got %q", state.Decision)
		}
		if !state.Acceptance.StillValid() {
			t.Fatalf("expected valid acceptance, got %#v", state.Acceptance)
		}
	})

	t.Run("expired", func(t *testing.T) {
		input := base
		input.Acceptance = &Acceptance{
			AcceptedAt:    now.Add(-2 * time.Hour),
			AcceptedBy:    "operator@example.com",
			WorkspaceRoot: "/workspace",
			Fingerprint:   fingerprint,
			ExpiresAt:     now.Add(-time.Second),
		}
		state := Evaluate(input)
		if state.Decision != DecisionNeedsReview {
			t.Fatalf("expected expired acceptance to require review, got %q", state.Decision)
		}
		if state.Acceptance.Status != AcceptanceExpired {
			t.Fatalf("expected expired status, got %#v", state.Acceptance)
		}
	})

	t.Run("workspace mismatch", func(t *testing.T) {
		input := base
		input.Acceptance = &Acceptance{
			AcceptedAt:    now.Add(-time.Hour),
			AcceptedBy:    "operator@example.com",
			WorkspaceRoot: "/other",
			Fingerprint:   fingerprint,
			ExpiresAt:     now.Add(time.Hour),
		}
		state := Evaluate(input)
		if state.Decision != DecisionNeedsReview {
			t.Fatalf("expected workspace mismatch to require review, got %q", state.Decision)
		}
		if state.Acceptance.Status != AcceptanceWorkspaceMismatch {
			t.Fatalf("expected workspace mismatch status, got %#v", state.Acceptance)
		}
	})
}

func TestSummaryRedactsDangerousEnvValues(t *testing.T) {
	state := Evaluate(WorkspaceInput{
		WorkspaceRoot: "/workspace",
		Now:           time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		Signals: []RiskSignal{{
			Kind:   SignalDangerousEnvVar,
			Source: "env",
			Detail: "AWS_SECRET_ACCESS_KEY=super-secret-token",
		}},
	})

	if state.Signals[0].Detail != "AWS_SECRET_ACCESS_KEY=super-secret-token" {
		t.Fatalf("expected raw signal detail to remain available for internal state, got %q", state.Signals[0].Detail)
	}
	summary := state.Summary()
	if summary.Signals[0].Detail != "AWS_SECRET_ACCESS_KEY=<redacted>" {
		t.Fatalf("expected summary to redact secret value, got %q", summary.Signals[0].Detail)
	}
	joined := strings.Join(append(summary.Reasons, summary.Signals[0].Reason), "\n")
	if strings.Contains(joined, "super-secret-token") {
		t.Fatalf("expected summary reasons to redact secret value, got %q", joined)
	}
}
