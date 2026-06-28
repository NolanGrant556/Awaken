package releasenotes

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormatVersionBullets(t *testing.T) {
	releases := []Release{{
		Version: "1.2.3",
		Notes: []string{
			"Added release notes",
			"Fixed cached fallback",
		},
	}}

	got := Format(releases)
	want := "Version 1.2.3:\n· Added release notes\n· Fixed cached fallback"
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestSelectNotesFreshBeatsCached(t *testing.T) {
	fresh := []Release{{Version: "2.0.0", Notes: []string{"Fresh note"}}}
	cached := []Release{{Version: "1.0.0", Notes: []string{"Cached note"}}}

	got := SelectNotes(fresh, cached, Options{})
	if got.Source != SourceFresh {
		t.Fatalf("Source = %q, want %q", got.Source, SourceFresh)
	}
	if !reflect.DeepEqual(got.Releases, fresh) {
		t.Fatalf("Releases = %#v, want %#v", got.Releases, fresh)
	}
	if !strings.Contains(got.Text, "Fresh note") {
		t.Fatalf("Text = %q, want fresh notes", got.Text)
	}
}

func TestSelectNotesTimeoutUsesCached(t *testing.T) {
	cached := []Release{{Version: "1.0.0", Notes: []string{"Cached note"}}}

	got := SelectNotes(nil, cached, Options{FreshTimedOut: true})
	if got.Source != SourceTimeout {
		t.Fatalf("Source = %q, want %q", got.Source, SourceTimeout)
	}
	if !got.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
	if !reflect.DeepEqual(got.Releases, cached) {
		t.Fatalf("Releases = %#v, want %#v", got.Releases, cached)
	}
	if !strings.Contains(got.Text, "Cached note") {
		t.Fatalf("Text = %q, want cached notes", got.Text)
	}
}

func TestSelectNotesNoNotesReturnsLink(t *testing.T) {
	const link = "https://example.test/changelog"

	got := SelectNotes(nil, nil, Options{ChangelogURL: link})
	if got.Source != SourceLink {
		t.Fatalf("Source = %q, want %q", got.Source, SourceLink)
	}
	if got.Link != link {
		t.Fatalf("Link = %q, want %q", got.Link, link)
	}
	want := "See the full changelog at: " + link
	if got.Text != want {
		t.Fatalf("Text = %q, want %q", got.Text, want)
	}
}

func TestParseChangelogHeadingsAndBullets(t *testing.T) {
	markdown := `# Changelog

Intro text that should be ignored.

## 1.2.0 - 2026-06-28
- Added release notes
- Fixed cache fallback

### Internal
* Kept subsection bullets

## Version v1.1.0
+ Added plus bullet

## Not a version
- Ignore me
`

	got := ParseChangelog(markdown)
	want := []Release{
		{
			Version: "1.2.0",
			Notes: []string{
				"Added release notes",
				"Fixed cache fallback",
				"Kept subsection bullets",
			},
		},
		{
			Version: "v1.1.0",
			Notes:   []string{"Added plus bullet"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseChangelog() = %#v, want %#v", got, want)
	}
}

func TestParseChangelogSkipsEmptyVersions(t *testing.T) {
	markdown := `## 2.0.0
This section has no bullets.

## 1.0.0
- First real note
`

	got := ParseChangelog(markdown)
	want := []Release{{Version: "1.0.0", Notes: []string{"First real note"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseChangelog() = %#v, want %#v", got, want)
	}
}

func TestSelectNotesLimitsMaxVersions(t *testing.T) {
	fresh := []Release{
		{Version: "3.0.0", Notes: []string{"Three"}},
		{Version: "2.0.0", Notes: []string{"Two"}},
		{Version: "1.0.0", Notes: []string{"One"}},
	}

	got := SelectNotes(fresh, nil, Options{MaxVersions: 2})
	want := fresh[:2]
	if !reflect.DeepEqual(got.Releases, want) {
		t.Fatalf("Releases = %#v, want %#v", got.Releases, want)
	}
	if strings.Contains(got.Text, "Version 1.0.0:") {
		t.Fatalf("Text = %q, did not expect third version", got.Text)
	}
}

func TestSourceMarkers(t *testing.T) {
	cached := []Release{{Version: "1.0.0", Notes: []string{"Cached note"}}}

	cachedResult := SelectNotes(nil, cached, Options{})
	if cachedResult.Source != SourceCached {
		t.Fatalf("cached Source = %q, want %q", cachedResult.Source, SourceCached)
	}

	timeoutResult := SelectNotes(nil, nil, Options{FreshTimedOut: true})
	if timeoutResult.Source != SourceTimeout {
		t.Fatalf("timeout Source = %q, want %q", timeoutResult.Source, SourceTimeout)
	}

	if SourceFresh != "fresh" || SourceCached != "cached" || SourceLink != "link" || SourceTimeout != "timeout" {
		t.Fatalf("unexpected source marker values")
	}
}
