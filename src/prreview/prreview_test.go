package prreview

import (
	"strings"
	"testing"
	"time"
)

func TestFormatCommentsNoComments(t *testing.T) {
	got := FormatComments(CommentsInput{})
	if got != "No comments found." {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestFormatCommentsPRLevelComment(t *testing.T) {
	got := FormatComments(CommentsInput{
		PRComments: []Comment{{
			ID:      "1",
			Author:  "alice",
			Body:    "Please update the README.",
			Created: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		}},
	})

	want := "## Comments\n\n" +
		"- @alice PR:\n" +
		"  > Please update the README."
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormatCommentsCodeReviewWithHunk(t *testing.T) {
	got := FormatComments(CommentsInput{
		ReviewComments: []Comment{{
			ID:       "2",
			Author:   "bob",
			Body:     "This branch should keep the old behavior covered.",
			Created:  time.Date(2026, 6, 28, 10, 1, 0, 0, time.UTC),
			Path:     "src/runtime/run_service.go",
			Line:     42,
			DiffHunk: "@@ -40,7 +40,7 @@\n- oldBehavior()\n+ newBehavior()",
		}},
	})

	want := "## Comments\n\n" +
		"- @bob src/runtime/run_service.go#42:\n" +
		"  ```diff\n" +
		"  @@ -40,7 +40,7 @@\n" +
		"  - oldBehavior()\n" +
		"  + newBehavior()\n" +
		"  ```\n" +
		"  > This branch should keep the old behavior covered."
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormatCommentsNestedReply(t *testing.T) {
	got := FormatComments(CommentsInput{
		ReviewComments: []Comment{
			{
				ID:      "10",
				Author:  "alice",
				Body:    "Please fix this edge case.",
				Created: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
				Path:    "src/agentcore/runner.go",
				Line:    18,
			},
			{
				ID:          "11",
				InReplyToID: "10",
				Author:      "carol",
				Body:        "Fixed in the latest push.",
				Created:     time.Date(2026, 6, 28, 10, 2, 0, 0, time.UTC),
				Path:        "src/agentcore/runner.go",
				Line:        18,
			},
		},
	})

	want := "## Comments\n\n" +
		"- @alice src/agentcore/runner.go#18:\n" +
		"  > Please fix this edge case.\n\n" +
		"  - @carol src/agentcore/runner.go#18:\n" +
		"    > Fixed in the latest push."
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormatCommentsMissingLineAndPathFallbacks(t *testing.T) {
	got := FormatComments(CommentsInput{
		ReviewComments: []Comment{
			{
				ID:      "20",
				Author:  "drew",
				Body:    "No path came back from the API.",
				Created: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
			},
			{
				ID:      "21",
				Author:  "erin",
				Body:    "The API returned a file but no line.",
				Created: time.Date(2026, 6, 28, 10, 1, 0, 0, time.UTC),
				Path:    "src/prreview/prreview.go",
			},
		},
	})

	want := "## Comments\n\n" +
		"- @drew PR:\n" +
		"  > No path came back from the API.\n\n" +
		"- @erin src/prreview/prreview.go:\n" +
		"  > The API returned a file but no line."
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestBuildReviewPromptWithPRNumber(t *testing.T) {
	got := BuildReviewPrompt("123")
	for _, want := range []string{
		"`gh pr list`",
		"`gh pr view <number>`",
		"`gh pr diff <number>`",
		"Correctness",
		"Project conventions",
		"Performance implications",
		"Tests and coverage",
		"Security considerations",
		"PR number: 123",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected prompt to contain %q:\n%s", want, got)
		}
	}
}
