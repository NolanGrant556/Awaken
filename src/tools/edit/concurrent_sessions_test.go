package edit

import (
	"awaken/src/tool"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConcurrentSessionsDoNotLoseSuccessfulEdits(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	initial := "FIRST\n" + strings.Repeat("padding\n", 100000) + "LAST\n"
	for attempt := 0; attempt < 15; attempt++ {
		if e := os.WriteFile(path, []byte(initial), 0600); e != nil {
			t.Fatal(e)
		}
		a, b := editContext(root), editContext(root)
		primeRead(t, a, "sample.txt")
		primeRead(t, b, "sample.txt")
		start := make(chan struct{})
		results := make(chan tool.ExecutionOutcome, 2)
		for i, ctx := range []context.Context{a, b} {
			go func(i int, ctx context.Context) {
				<-start
				input := `{"file_path":"sample.txt","old_string":"FIRST","new_string":"FIRST-A"}`
				if i == 1 {
					input = `{"file_path":"sample.txt","old_string":"LAST","new_string":"LAST-B"}`
				}
				r, _ := (&Tool{}).ExecuteOutcome(ctx, input, nil)
				results <- r
			}(i, ctx)
		}
		close(start)
		r1, r2 := <-results, <-results
		if !r1.IsError() && !r2.IsError() {
			data, _ := os.ReadFile(path)
			if !strings.Contains(string(data), "FIRST-A") || !strings.Contains(string(data), "LAST-B") {
				t.Fatalf("attempt %d: both Edits returned success but file lost one change (first=%t,last=%t)", attempt, strings.Contains(string(data), "FIRST-A"), strings.Contains(string(data), "LAST-B"))
			}
		}
	}
}
