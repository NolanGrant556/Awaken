package contextnudges

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelevantTipsDisabled(t *testing.T) {
	tips := []Tip{{ID: "global-tip", Text: "hello"}}

	got := RelevantTips(tips, Context{TipsDisabled: true})

	if len(got) != 0 {
		t.Fatalf("expected no tips when disabled, got %d", len(got))
	}
}

func TestRelevantTipsNoRelevantSignals(t *testing.T) {
	tips := []Tip{
		{ID: "cli-tip", Signals: Signals{CLI: []string{"git"}}},
		{ID: "tool-tip", Signals: Signals{Tools: []string{"read_file"}}},
	}

	got := RelevantTips(tips, Context{
		SessionNumber: 10,
		CLISignals:    []string{"npm"},
		ToolSignals:   []string{"shell"},
	})

	if len(got) != 0 {
		t.Fatalf("expected no relevant tips, got %+v", got)
	}
}

func TestRelevantTipsMatchesSignalKinds(t *testing.T) {
	tips := []Tip{
		{ID: "cli", Signals: Signals{CLI: []string{"npm"}}},
		{ID: "tool", Signals: Signals{Tools: []string{"read_file"}}},
		{ID: "read-file", Signals: Signals{ReadFiles: []string{".tsx"}}},
		{ID: "context", Signals: Signals{Context: []string{"plan_mode"}}},
	}

	got := RelevantTips(tips, Context{
		SessionNumber:  10,
		CLISignals:     []string{"npm"},
		ToolSignals:    []string{"read_file"},
		ReadFilePaths:  []string{"apps/desktop/src/renderer/App.tsx"},
		ContextSignals: []string{"plan_mode"},
	})

	if gotIDs(got) != "cli,tool,read-file,context" {
		t.Fatalf("unexpected relevant tips: %s", gotIDs(got))
	}
}

func TestRelevantTipsCooldown(t *testing.T) {
	tips := []Tip{
		{ID: "too-soon", CooldownSessions: 5},
		{ID: "ready", CooldownSessions: 5},
		{ID: "never-shown", CooldownSessions: 5},
	}
	ctx := Context{
		SessionNumber: 10,
		History: History{LastShown: map[string]int{
			"too-soon": 8,
			"ready":    5,
		}},
	}

	got := RelevantTips(tips, ctx)

	if gotIDs(got) != "ready,never-shown" {
		t.Fatalf("unexpected tips after cooldown filter: %s", gotIDs(got))
	}
}

func TestSelectTipWithLongestTimeSinceShown(t *testing.T) {
	tips := []Tip{
		{ID: "recent"},
		{ID: "older"},
		{ID: "never-shown"},
	}
	history := History{LastShown: map[string]int{
		"recent": 8,
		"older":  2,
	}}

	selection, ok := SelectTipWithLongestTimeSinceShown(tips, history, 10)

	if !ok {
		t.Fatal("expected a selected tip")
	}
	if selection.Tip.ID != "never-shown" {
		t.Fatalf("expected never-shown to win, got %q", selection.Tip.ID)
	}
	if selection.Record.TipID != "never-shown" || selection.Record.SessionNumber != 10 {
		t.Fatalf("unexpected record plan: %+v", selection.Record)
	}
}

func TestRecordShownIdempotent(t *testing.T) {
	original := History{LastShown: map[string]int{"other": 1}}

	recorded := RecordShown(original, "tip-a", 7)
	if got := recorded.LastShown["tip-a"]; got != 7 {
		t.Fatalf("expected tip-a shown at session 7, got %d", got)
	}
	if _, exists := original.LastShown["tip-a"]; exists {
		t.Fatal("RecordShown mutated the input history")
	}

	again := RecordShown(recorded, "tip-a", 7)
	if len(again.LastShown) != 2 || again.LastShown["tip-a"] != 7 {
		t.Fatalf("recording the same session should be idempotent, got %+v", again.LastShown)
	}
}

func TestBusyInputFirstTouchModes(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{mode: string(BusyInputQueue), want: "queued"},
		{mode: string(BusyInputSteer), want: "steered"},
		{mode: string(BusyInputInterrupt), want: "interrupted"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			hint, plan, ok := FirstTouchHint(FirstTouchBusyInput, tt.mode, false)
			if !ok {
				t.Fatal("expected first-touch hint")
			}
			if !strings.Contains(hint, tt.want) {
				t.Fatalf("expected hint to mention %q, got %q", tt.want, hint)
			}
			if plan.FirstTouchFlag != FirstTouchBusyInput {
				t.Fatalf("unexpected mark-seen plan: %+v", plan)
			}
		})
	}
}

func TestToolProgressFirstTouchOnce(t *testing.T) {
	store := &fakeStore{}

	hint, plan, ok := FirstTouchHint(FirstTouchToolProgress, "", false)
	if !ok {
		t.Fatal("expected tool progress hint")
	}
	if !strings.Contains(hint, "/verbose") {
		t.Fatalf("expected tool progress hint to mention /verbose, got %q", hint)
	}
	if !plan.MarkSeenBestEffort(store) {
		t.Fatal("expected mark-seen plan to succeed")
	}
	if store.marked != FirstTouchToolProgress {
		t.Fatalf("expected store to mark tool progress, got %q", store.marked)
	}

	_, _, ok = FirstTouchHint(FirstTouchToolProgress, "", true)
	if ok {
		t.Fatal("expected seen tool progress hint to be suppressed")
	}
}

func TestFirstTouchMarkSeenFailureIsBestEffort(t *testing.T) {
	_, plan, ok := FirstTouchHint(FirstTouchBusyInput, string(BusyInputQueue), false)
	if !ok {
		t.Fatal("expected busy input hint")
	}

	if plan.MarkSeenBestEffort(&fakeStore{err: errors.New("disk full")}) {
		t.Fatal("expected failed best-effort mark to return false")
	}
}

func TestResidueDetectionPath(t *testing.T) {
	home := t.TempDir()

	if path, ok := DetectResiduePath(home); ok || path != "" {
		t.Fatalf("expected no residue before directory exists, got path=%q ok=%v", path, ok)
	}

	want := filepath.Join(home, ResidueDirName)
	if err := os.Mkdir(want, 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := DetectResiduePath(home)
	if !ok {
		t.Fatal("expected residue path to be detected")
	}
	if got != want {
		t.Fatalf("expected residue path %q, got %q", want, got)
	}

	hint, plan, ok := FirstTouchHint(FirstTouchResidueCleanup, "", false)
	if !ok {
		t.Fatal("expected residue cleanup hint")
	}
	if !strings.Contains(hint, ".openclaw") {
		t.Fatalf("expected residue hint to mention .openclaw, got %q", hint)
	}
	if plan.FirstTouchFlag != FirstTouchResidueCleanup {
		t.Fatalf("unexpected residue mark plan: %+v", plan)
	}
}

type fakeStore struct {
	marked FirstTouchFlag
	err    error
}

func (s *fakeStore) MarkFirstTouchSeen(flag FirstTouchFlag) error {
	if s.err != nil {
		return s.err
	}
	s.marked = flag
	return nil
}

func gotIDs(tips []Tip) string {
	ids := make([]string, 0, len(tips))
	for _, tip := range tips {
		ids = append(ids, tip.ID)
	}
	return strings.Join(ids, ",")
}
