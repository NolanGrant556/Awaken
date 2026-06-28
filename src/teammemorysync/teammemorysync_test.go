package teammemorysync

import (
	"strings"
	"testing"
)

func TestHashContentFormatStable(t *testing.T) {
	got := HashContent("hello")
	want := "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if len(got) != len("sha256:")+64 || !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("unexpected hash format: %q", got)
	}
}

func TestPlanPushSkipsSecretEntries(t *testing.T) {
	token := "ghp_" + strings.Repeat("A", 36)
	plan := PlanPush(map[string]string{
		"MEMORY.md": "safe team memory",
		"secret.md": "github token: " + token,
	}, nil, PushOptions{})

	if len(plan.SkippedSecrets) != 1 {
		t.Fatalf("expected one skipped secret, got %+v", plan.SkippedSecrets)
	}
	if plan.SkippedSecrets[0].Path != "secret.md" || plan.SkippedSecrets[0].RuleID != "github-pat" {
		t.Fatalf("unexpected skipped secret: %+v", plan.SkippedSecrets[0])
	}
	if _, ok := plan.Delta["secret.md"]; ok {
		t.Fatalf("secret entry should not be in delta: %+v", plan.Delta)
	}
	if plan.Delta["MEMORY.md"] != "safe team memory" {
		t.Fatalf("safe entry missing from delta: %+v", plan.Delta)
	}
}

func TestPlanPushDeltaOnlyIncludesHashChanges(t *testing.T) {
	serverChecksums := map[string]string{
		"same.md":    HashContent("same"),
		"changed.md": HashContent("old"),
		"deleted.md": HashContent("remote only"),
	}
	plan := PlanPush(map[string]string{
		"same.md":    "same",
		"changed.md": "new",
	}, serverChecksums, PushOptions{})

	if len(plan.Delta) != 1 || plan.Delta["changed.md"] != "new" {
		t.Fatalf("expected only changed.md in delta, got %+v", plan.Delta)
	}
	if plan.DeltaChecksums["changed.md"] != HashContent("new") {
		t.Fatalf("unexpected delta checksum: %+v", plan.DeltaChecksums)
	}
	if len(plan.DeletionsNotPropagated) != 1 || plan.DeletionsNotPropagated[0] != "deleted.md" {
		t.Fatalf("expected remote deletion preservation, got %+v", plan.DeletionsNotPropagated)
	}
}

func TestPlanPushSkipsOversizeEntries(t *testing.T) {
	plan := PlanPush(map[string]string{
		"big.md":   "abcdef",
		"small.md": "abc",
	}, nil, PushOptions{MaxEntryBytes: 5})

	if len(plan.SkippedOversize) != 1 || plan.SkippedOversize[0].Path != "big.md" {
		t.Fatalf("expected big.md to be skipped as oversize, got %+v", plan.SkippedOversize)
	}
	if _, ok := plan.Delta["big.md"]; ok {
		t.Fatalf("oversize entry should not be in delta: %+v", plan.Delta)
	}
	if plan.Delta["small.md"] != "abc" {
		t.Fatalf("small entry missing from delta: %+v", plan.Delta)
	}
}

func TestPlanPushSplitsBatchesByRequestBytes(t *testing.T) {
	maxBytes := 58
	plan := PlanPush(map[string]string{
		"a.md": strings.Repeat("a", 20),
		"b.md": strings.Repeat("b", 20),
		"c.md": strings.Repeat("c", 20),
	}, nil, PushOptions{MaxRequestBytes: maxBytes})

	if len(plan.Batches) < 2 {
		t.Fatalf("expected multiple batches, got %+v", plan.Batches)
	}
	for _, batch := range plan.Batches {
		if batch.Bytes > maxBytes {
			t.Fatalf("batch exceeded max bytes %d: %+v", maxBytes, batch)
		}
		if len(batch.Entries) == 0 {
			t.Fatalf("empty batch: %+v", plan.Batches)
		}
	}
}

func TestPlanPull404IsEmpty(t *testing.T) {
	plan := PlanPull(PullInput{HTTPStatus: 404})

	if plan.Status != StatusEmpty || !plan.Empty {
		t.Fatalf("expected empty 404 pull, got %+v", plan)
	}
	if len(plan.Entries) != 0 || len(plan.EntryChecksums) != 0 {
		t.Fatalf("expected empty maps, got %+v", plan)
	}
}

func TestPlanPushServerMaxEntriesTruncatesAndReportsTooMany(t *testing.T) {
	plan := PlanPush(map[string]string{
		"a.md": "a",
		"b.md": "b",
		"c.md": "c",
	}, map[string]string{
		"remote-only.md": HashContent("remote"),
	}, PushOptions{ServerMaxEntries: 2})

	if plan.Status != StatusTooManyEntries {
		t.Fatalf("expected too_many_entries status, got %+v", plan)
	}
	if plan.MaxEntries != 2 || plan.ReceivedEntries != 3 {
		t.Fatalf("unexpected entry cap details: %+v", plan)
	}
	if len(plan.TruncatedEntries) != 1 || plan.TruncatedEntries[0] != "c.md" {
		t.Fatalf("expected deterministic truncation of c.md, got %+v", plan.TruncatedEntries)
	}
	if len(plan.DeletionsNotPropagated) != 1 || plan.DeletionsNotPropagated[0] != "remote-only.md" {
		t.Fatalf("expected deletion-not-propagated marker, got %+v", plan.DeletionsNotPropagated)
	}
	if len(plan.Batches) != 0 {
		t.Fatalf("known over-cap plan should not produce upload batches: %+v", plan.Batches)
	}
}

func TestDecidePushResponseMapsStructuredTooManyEntries(t *testing.T) {
	decision := DecidePushResponse(PushResponseInput{
		HTTPStatus: 413,
		TooManyEntries: &TooManyEntries{
			MaxEntries:      10,
			ReceivedEntries: 12,
		},
	})

	if decision.Status != StatusTooManyEntries {
		t.Fatalf("expected too_many_entries decision, got %+v", decision)
	}
	if decision.MaxEntries != 10 || decision.ReceivedEntries != 12 {
		t.Fatalf("unexpected too-many details: %+v", decision)
	}
}

func TestDecisionTypesExpressAuthParseAndConflict(t *testing.T) {
	if got := PlanPull(PullInput{HTTPStatus: 401}).Status; got != StatusAuth {
		t.Fatalf("expected auth pull status, got %s", got)
	}
	if got := PlanPull(PullInput{HTTPStatus: 200, ParseError: true}).Status; got != StatusParse {
		t.Fatalf("expected parse pull status, got %s", got)
	}
	if got := DecidePushResponse(PushResponseInput{HTTPStatus: 412}).Status; got != StatusConflict {
		t.Fatalf("expected conflict push status, got %s", got)
	}
	if got := DecidePushResponse(PushResponseInput{HTTPStatus: 200, ParseError: true}).Status; got != StatusParse {
		t.Fatalf("expected parse push status, got %s", got)
	}
}
