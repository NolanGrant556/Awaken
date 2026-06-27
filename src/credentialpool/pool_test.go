package credentialpool

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLeastUsedSelection(t *testing.T) {
	pool := mustPool(t, StrategyLeastUsed, testCredentials()...)

	got := selectIDs(t, pool, 6)
	want := []string{"a", "b", "c", "a", "b", "c"}
	assertIDs(t, got, want)

	for _, id := range []string{"a", "b", "c"} {
		stats, ok := pool.Stats(id)
		if !ok {
			t.Fatalf("missing stats for %s", id)
		}
		if stats.Requests != 2 {
			t.Fatalf("expected %s to have 2 requests, got %d", id, stats.Requests)
		}
	}
}

func TestRoundRobinSelection(t *testing.T) {
	pool := mustPool(t, StrategyRoundRobin, testCredentials()...)

	got := selectIDs(t, pool, 5)
	want := []string{"a", "b", "c", "a", "b"}
	assertIDs(t, got, want)
}

func TestStickyUntilFailureSelection(t *testing.T) {
	pool := mustPool(t, StrategyStickyUntilFailure, testCredentials()...)

	first, ok := pool.Select()
	if !ok {
		t.Fatal("expected first credential")
	}
	second, ok := pool.Select()
	if !ok {
		t.Fatal("expected sticky credential")
	}
	if first.ID != "a" || second.ID != "a" {
		t.Fatalf("expected sticky credential a twice, got %s then %s", first.ID, second.ID)
	}

	next, ok, err := pool.MarkFailure(first.ID, FailureReasonUnknown)
	if err != nil {
		t.Fatalf("mark failure: %v", err)
	}
	if !ok || next.ID != "b" {
		t.Fatalf("expected sticky failover to b, got %q ok=%v", next.ID, ok)
	}

	again, ok := pool.Select()
	if !ok {
		t.Fatal("expected sticky credential after failover")
	}
	if again.ID != "b" {
		t.Fatalf("expected b to remain sticky after failover, got %s", again.ID)
	}
}

func TestUnauthorizedTemporarilyDisablesAndFailsOver(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	pool := mustPoolWithOptions(t, StrategyLeastUsed, PoolOptions{
		UnauthorizedCooldown: time.Minute,
		Now:                  func() time.Time { return now },
	}, testCredentials()...)

	selected, ok := pool.Select()
	if !ok {
		t.Fatal("expected selected credential")
	}
	if selected.ID != "a" {
		t.Fatalf("expected first credential a, got %s", selected.ID)
	}

	next, ok, err := pool.MarkHTTPFailure(selected.ID, 401)
	if err != nil {
		t.Fatalf("mark 401: %v", err)
	}
	if !ok || next.ID != "b" {
		t.Fatalf("expected 401 failover to b, got %q ok=%v", next.ID, ok)
	}

	snapshot := pool.Snapshot()
	a := snapshotCredential(t, snapshot, "a")
	if a.DisabledUntil.IsZero() {
		t.Fatal("expected unauthorized credential to be temporarily disabled")
	}
	if !a.DisabledUntil.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected disabled_until: %s", a.DisabledUntil)
	}
	if a.Stats.UnauthorizedFailures != 1 {
		t.Fatalf("expected one unauthorized failure, got %d", a.Stats.UnauthorizedFailures)
	}

	seen := selectIDs(t, pool, 4)
	for _, id := range seen {
		if id == "a" {
			t.Fatalf("temporarily disabled credential was selected: %v", seen)
		}
	}
}

func TestSuccessClearsPreviousDisableState(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	pool := mustPoolWithOptions(t, StrategyLeastUsed, PoolOptions{
		UnauthorizedCooldown: time.Minute,
		Now:                  func() time.Time { return now },
	}, testCredentials()...)

	if _, _, err := pool.MarkFailure("a", FailureReasonUnauthorized); err != nil {
		t.Fatalf("mark unauthorized: %v", err)
	}
	if err := pool.MarkSuccess("a"); err != nil {
		t.Fatalf("mark success: %v", err)
	}

	snapshot := pool.Snapshot()
	a := snapshotCredential(t, snapshot, "a")
	if !a.DisabledUntil.IsZero() {
		t.Fatalf("success should clear disabled state, got %s", a.DisabledUntil)
	}
	if a.LastFailureReason != "" {
		t.Fatalf("success should clear last failure reason, got %s", a.LastFailureReason)
	}
}

func TestRateLimitRecordsFailureWithoutDisabling(t *testing.T) {
	pool := mustPool(t, StrategyRoundRobin, testCredentials()...)

	selected, ok := pool.Select()
	if !ok {
		t.Fatal("expected selected credential")
	}
	next, ok, err := pool.MarkHTTPFailure(selected.ID, 429)
	if err != nil {
		t.Fatalf("mark 429: %v", err)
	}
	if !ok || next.ID != "b" {
		t.Fatalf("expected round-robin rate-limit rotation to b, got %q ok=%v", next.ID, ok)
	}

	snapshot := pool.Snapshot()
	a := snapshotCredential(t, snapshot, "a")
	if !a.DisabledUntil.IsZero() {
		t.Fatalf("rate limit should not disable credential, disabled until %s", a.DisabledUntil)
	}
	if a.LastFailureReason != FailureReasonRateLimit {
		t.Fatalf("expected rate limit failure reason, got %s", a.LastFailureReason)
	}
	if a.Stats.RateLimitFailures != 1 {
		t.Fatalf("expected one rate limit failure, got %d", a.Stats.RateLimitFailures)
	}
}

func TestSingleCredentialRateLimitRemainsAvailable(t *testing.T) {
	pool := mustPool(t, StrategyLeastUsed, testCredentials()[0])

	selected, ok := pool.Select()
	if !ok {
		t.Fatal("expected selected credential")
	}
	next, ok, err := pool.MarkFailure(selected.ID, FailureReasonRateLimit)
	if err != nil {
		t.Fatalf("mark rate limit: %v", err)
	}
	if !ok || next.ID != selected.ID {
		t.Fatalf("expected only credential to remain available, got %q ok=%v", next.ID, ok)
	}
}

func TestSnapshotRestore(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	pool := mustPoolWithOptions(t, StrategyLeastUsed, PoolOptions{
		UnauthorizedCooldown: time.Minute,
		Now:                  func() time.Time { return now },
	}, testCredentials()...)

	selected, ok := pool.Select()
	if !ok {
		t.Fatal("expected selected credential")
	}
	if _, _, err := pool.MarkFailure(selected.ID, FailureReasonUnauthorized); err != nil {
		t.Fatalf("mark unauthorized: %v", err)
	}
	snapshot := pool.Snapshot()

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	assertNoSecrets(t, string(encoded))

	restored, err := RestoreWithOptions(snapshot, testCredentials(), PoolOptions{
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	restoredSelected, ok := restored.Select()
	if !ok {
		t.Fatal("expected restored pool selection")
	}
	if restoredSelected.ID == "a" {
		t.Fatal("restored pool selected temporarily disabled credential")
	}
	stats, ok := restored.Stats("a")
	if !ok {
		t.Fatal("expected restored stats for a")
	}
	if stats.UnauthorizedFailures != 1 {
		t.Fatalf("expected restored unauthorized failure count, got %d", stats.UnauthorizedFailures)
	}
	if restoredSelected.Secret == "" {
		t.Fatal("restored pool should return fresh credential secrets supplied by provider config")
	}
}

func TestRedactionForStringAndJSON(t *testing.T) {
	credential := testCredentials()[0]
	if strings.Contains(fmt.Sprint(credential), credential.Secret) {
		t.Fatal("String output leaked full credential secret")
	}

	encodedCredential, err := json.Marshal(credential)
	if err != nil {
		t.Fatalf("marshal credential: %v", err)
	}
	assertNoSecrets(t, string(encodedCredential))

	pool := mustPool(t, StrategyLeastUsed, testCredentials()...)
	encodedPool, err := json.Marshal(pool)
	if err != nil {
		t.Fatalf("marshal pool: %v", err)
	}
	assertNoSecrets(t, string(encodedPool))
	if strings.Contains(fmt.Sprint(pool), credential.Secret) {
		t.Fatal("pool String output leaked full credential secret")
	}
}

func TestConcurrentSelectionAndStats(t *testing.T) {
	pool := mustPool(t, StrategyLeastUsed,
		ProviderCredential{ID: "a", Secret: "sk-a-secret"},
		ProviderCredential{ID: "b", Secret: "sk-b-secret"},
		ProviderCredential{ID: "c", Secret: "sk-c-secret"},
		ProviderCredential{ID: "d", Secret: "sk-d-secret"},
	)

	const goroutines = 24
	const iterations = 200
	var wg sync.WaitGroup
	errs := make(chan string, goroutines*iterations)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				credential, ok := pool.Select()
				if !ok {
					errs <- "select returned no credential"
					return
				}
				if credential.Secret == "" {
					errs <- "selected credential missing secret"
					return
				}
				if err := pool.MarkSuccess(credential.ID); err != nil {
					errs <- err.Error()
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}

	snapshot := pool.Snapshot()
	total := 0
	minRequests := goroutines * iterations
	maxRequests := 0
	for _, credential := range snapshot.Credentials {
		requests := credential.Stats.Requests
		total += requests
		if requests < minRequests {
			minRequests = requests
		}
		if requests > maxRequests {
			maxRequests = requests
		}
	}
	if total != goroutines*iterations {
		t.Fatalf("expected %d total requests, got %d", goroutines*iterations, total)
	}
	if maxRequests-minRequests > 1 {
		t.Fatalf("least_used should distribute evenly, min=%d max=%d", minRequests, maxRequests)
	}
}

func mustPool(t *testing.T, strategy Strategy, credentials ...ProviderCredential) *Pool {
	t.Helper()
	return mustPoolWithOptions(t, strategy, PoolOptions{}, credentials...)
}

func mustPoolWithOptions(t *testing.T, strategy Strategy, options PoolOptions, credentials ...ProviderCredential) *Pool {
	t.Helper()
	pool, err := NewPoolWithOptions("openai", strategy, credentials, options)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	return pool
}

func testCredentials() []ProviderCredential {
	return []ProviderCredential{
		{ID: "a", Provider: "openai", Label: "primary", Secret: "sk-test-primary-secret", Metadata: map[string]string{"source": "env"}},
		{ID: "b", Provider: "openai", Label: "backup", Secret: "sk-test-backup-secret"},
		{ID: "c", Provider: "openai", Label: "tertiary", Secret: "sk-test-tertiary-secret"},
	}
}

func selectIDs(t *testing.T, pool *Pool, count int) []string {
	t.Helper()
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		credential, ok := pool.Select()
		if !ok {
			t.Fatalf("selection %d returned no credential", i)
		}
		ids = append(ids, credential.ID)
	}
	return ids
}

func assertIDs(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func snapshotCredential(t *testing.T, snapshot PoolSnapshot, id string) CredentialSnapshot {
	t.Helper()
	for _, credential := range snapshot.Credentials {
		if credential.ID == id {
			return credential
		}
	}
	t.Fatalf("missing snapshot credential %s", id)
	return CredentialSnapshot{}
}

func assertNoSecrets(t *testing.T, text string) {
	t.Helper()
	for _, credential := range testCredentials() {
		if strings.Contains(text, credential.Secret) {
			t.Fatalf("text leaked secret %q: %s", credential.Secret, text)
		}
	}
}
