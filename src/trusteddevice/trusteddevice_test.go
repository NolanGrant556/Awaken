package trusteddevice

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeJWTPayloadStripsPrefixAndExpiry(t *testing.T) {
	token := makeJWT(t, map[string]any{
		"sub": "session",
		"exp": int64(2_000),
	})

	payload, ok := DecodeJWTPayload(SessionIngressTokenPrefix + token)
	if !ok {
		t.Fatalf("expected prefixed token payload to decode")
	}
	if payload["sub"] != "session" {
		t.Fatalf("expected sub=session, got %v", payload["sub"])
	}

	expiry, ok := DecodeJWTExpiry(SessionIngressTokenPrefix + token)
	if !ok {
		t.Fatalf("expected prefixed token expiry to decode")
	}
	if expiry != 2_000 {
		t.Fatalf("expected expiry 2000, got %d", expiry)
	}
}

func TestDecodeJWTPayloadInvalid(t *testing.T) {
	invalidTokens := []string{
		"",
		"one.two",
		"one..three",
		"one.not-base64!.three",
		"one." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".three",
		makeJWT(t, map[string]any{"sub": "missing-exp"}),
	}

	for _, token := range invalidTokens[:5] {
		if payload, ok := DecodeJWTPayload(token); ok {
			t.Fatalf("expected payload decode to fail for %q, got %+v", token, payload)
		}
	}
	if expiry, ok := DecodeJWTExpiry(invalidTokens[5]); ok {
		t.Fatalf("expected missing exp to fail, got %d", expiry)
	}
}

func TestTokenSourceGateEnvPrecedenceAndCache(t *testing.T) {
	source := TokenSource{
		GateEnabled: true,
		EnvToken:    "env-token",
		StoredToken: "stored-token",
	}

	token, ok := source.Token()
	if !ok || token != "env-token" {
		t.Fatalf("expected env token precedence, got token=%q ok=%v", token, ok)
	}
	if source.ShouldEnroll() {
		t.Fatalf("expected env token to disable enrollment")
	}

	source.EnvToken = ""
	token, ok = source.Token()
	if !ok || token != "stored-token" {
		t.Fatalf("expected stored token, got token=%q ok=%v", token, ok)
	}

	source.StoredToken = "new-stored-token"
	token, ok = source.Token()
	if !ok || token != "stored-token" {
		t.Fatalf("expected cached stored token, got token=%q ok=%v", token, ok)
	}

	if !source.ClearStoredToken() {
		t.Fatalf("expected stored token to be deleted")
	}
	if source.StoredToken != "" {
		t.Fatalf("expected stored token to be empty, got %q", source.StoredToken)
	}
	if source.cacheValid || source.cachedStoredToken != "" {
		t.Fatalf("expected stored token cache to be cleared, got cacheValid=%v cached=%q", source.cacheValid, source.cachedStoredToken)
	}

	source.StoredToken = "fresh-stored-token"
	token, ok = source.Token()
	if !ok || token != "fresh-stored-token" {
		t.Fatalf("expected cache to be cleared and reread fresh stored token, got token=%q ok=%v", token, ok)
	}

	source.GateEnabled = false
	if token, ok = source.Token(); ok {
		t.Fatalf("expected gate off to suppress token, got %q", token)
	}
	if source.ClearStoredToken() {
		t.Fatalf("expected gate off clear to be a no-op")
	}
	if source.StoredToken != "fresh-stored-token" {
		t.Fatalf("expected gate off clear to keep stored token, got %q", source.StoredToken)
	}
}

func TestEnrollmentDecision(t *testing.T) {
	tests := []struct {
		name   string
		input  EnrollmentInput
		enroll bool
		reason EnrollmentSkipReason
	}{
		{
			name:   "gate off",
			input:  EnrollmentInput{},
			reason: EnrollmentSkipGateOff,
		},
		{
			name: "essential traffic only",
			input: EnrollmentInput{
				GateEnabled:          true,
				EssentialTrafficOnly: true,
				OAuthToken:           "oauth",
				FreshLogin:           true,
			},
			reason: EnrollmentSkipEssentialTrafficOnly,
		},
		{
			name: "env token",
			input: EnrollmentInput{
				GateEnabled: true,
				EnvToken:    "env",
				OAuthToken:  "oauth",
				FreshLogin:  true,
			},
			reason: EnrollmentSkipEnvToken,
		},
		{
			name: "no oauth",
			input: EnrollmentInput{
				GateEnabled: true,
				FreshLogin:  true,
			},
			reason: EnrollmentSkipNoOAuthToken,
		},
		{
			name: "not fresh login",
			input: EnrollmentInput{
				GateEnabled: true,
				OAuthToken:  "oauth",
			},
			reason: EnrollmentSkipNotFreshLogin,
		},
		{
			name: "enroll",
			input: EnrollmentInput{
				GateEnabled: true,
				OAuthToken:  "oauth",
				FreshLogin:  true,
			},
			enroll: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := EnrollmentDecision(tt.input)
			if decision.Enroll != tt.enroll || decision.Reason != tt.reason {
				t.Fatalf("unexpected decision: %+v", decision)
			}
		})
	}
}

func TestScheduleFromJWT(t *testing.T) {
	now := time.Unix(1_000, 0).UTC()
	buffer := 5 * time.Minute

	delayToken := makeJWT(t, map[string]any{"exp": int64(1_600)})
	delayPlan := ScheduleFromJWT("session-1", delayToken, now, buffer)
	if delayPlan.Action != ScheduleDelay || delayPlan.Delay != 5*time.Minute {
		t.Fatalf("expected 5m delay, got %+v", delayPlan)
	}

	immediateToken := makeJWT(t, map[string]any{"exp": int64(1_100)})
	immediatePlan := ScheduleFromJWT("session-1", immediateToken, now, buffer)
	if immediatePlan.Action != ScheduleImmediate || immediatePlan.Delay != 0 {
		t.Fatalf("expected immediate refresh, got %+v", immediatePlan)
	}

	keepPlan := ScheduleFromJWT("session-1", "not-a-jwt", now, buffer)
	if keepPlan.Action != ScheduleKeepExisting {
		t.Fatalf("expected keep existing for undecodable JWT, got %+v", keepPlan)
	}
}

func TestScheduleFromExpiresInFloor(t *testing.T) {
	floored := ScheduleFromExpiresIn("session-1", 10, 5*time.Minute)
	if floored.Action != ScheduleDelay || floored.Delay != MinRefreshDelay {
		t.Fatalf("expected 30s floor, got %+v", floored)
	}

	normal := ScheduleFromExpiresIn("session-1", 1_000, 5*time.Minute)
	if normal.Delay != 700*time.Second {
		t.Fatalf("expected 700s delay, got %+v", normal)
	}
}

func TestRefreshPlannerFailuresAndGeneration(t *testing.T) {
	planner := NewRefreshPlanner()
	now := time.Unix(1_000, 0).UTC()
	token := makeJWT(t, map[string]any{"exp": int64(2_000)})

	first := planner.ScheduleFromJWT("session-1", token, now, 5*time.Minute)
	if first.Generation != 1 || planner.IsStaleGeneration("session-1", first.Generation) {
		t.Fatalf("expected first generation to be current, got plan=%+v", first)
	}

	keepExisting := planner.ScheduleFromJWT("session-1", "invalid", now, 5*time.Minute)
	if keepExisting.Action != ScheduleKeepExisting || planner.Generation("session-1") != 1 {
		t.Fatalf("expected invalid JWT to keep existing generation, got plan=%+v generation=%d", keepExisting, planner.Generation("session-1"))
	}

	second := planner.ScheduleFromExpiresIn("session-1", 600, 5*time.Minute)
	if second.Generation != 2 {
		t.Fatalf("expected second generation 2, got %+v", second)
	}
	if !planner.IsStaleGeneration("session-1", first.Generation) {
		t.Fatalf("expected first generation to be stale")
	}
	if planner.IsStaleGeneration("session-1", second.Generation) {
		t.Fatalf("expected second generation to be current")
	}

	firstFailure := planner.RecordRefreshFailure("session-1")
	if firstFailure.Failures != 1 || !firstFailure.Retry || firstFailure.GiveUp {
		t.Fatalf("unexpected first failure decision: %+v", firstFailure)
	}
	planner.RecordRefreshFailure("session-1")
	thirdFailure := planner.RecordRefreshFailure("session-1")
	if thirdFailure.Failures != MaxRefreshFailures || thirdFailure.Retry || !thirdFailure.GiveUp {
		t.Fatalf("unexpected third failure decision: %+v", thirdFailure)
	}
	fourthFailure := planner.RecordRefreshFailure("session-1")
	if fourthFailure.Failures != MaxRefreshFailures || fourthFailure.Retry || !fourthFailure.GiveUp {
		t.Fatalf("expected failures to remain capped, got %+v", fourthFailure)
	}

	planner.RecordRefreshSuccess("session-1")
	if count := planner.FailureCount("session-1"); count != 0 {
		t.Fatalf("expected success to clear failures, got %d", count)
	}
}

func TestSessionIDRetag(t *testing.T) {
	if got := ToCompatSessionID("cse_abc"); got != "session_abc" {
		t.Fatalf("expected default compat retag, got %q", got)
	}
	if got := ToCompatSessionIDWithShim("cse_abc", false); got != "cse_abc" {
		t.Fatalf("expected disabled shim to leave cse id unchanged, got %q", got)
	}
	if got := ToCompatSessionID("run_abc"); got != "run_abc" {
		t.Fatalf("expected non-cse id unchanged, got %q", got)
	}
	if got := ToInfraSessionID("session_abc"); got != "cse_abc" {
		t.Fatalf("expected infra retag, got %q", got)
	}
	if got := ToInfraSessionID("cse_abc"); got != "cse_abc" {
		t.Fatalf("expected non-session id unchanged, got %q", got)
	}
}

func makeJWT(t *testing.T, payload map[string]any) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{"alg": "none"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	header := base64.RawURLEncoding.EncodeToString(headerBytes)
	body := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + body + ".sig"
}
