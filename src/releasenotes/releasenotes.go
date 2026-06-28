package releasenotes

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultFreshTimeout = 500 * time.Millisecond
	DefaultChangelogURL = "https://github.com/NolanGrant556/Awaken/blob/main/CHANGELOG.md"
)

type Source string

const (
	SourceFresh   Source = "fresh"
	SourceCached  Source = "cached"
	SourceLink    Source = "link"
	SourceTimeout Source = "timeout"
)

type Release struct {
	Version string
	Notes   []string
}

type Options struct {
	MaxVersions   int
	ChangelogURL  string
	FreshTimedOut bool
}

type Result struct {
	Source   Source
	Releases []Release
	Text     string
	Link     string
	TimedOut bool
}

var (
	headingRE = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$`)
	versionRE = regexp.MustCompile(`(?i)^v?\d+(?:\.\d+)+(?:-[0-9a-z.-]+)?(?:\+[0-9a-z.-]+)?$`)
)

func ParseChangelog(markdown string) []Release {
	var releases []Release
	var version string
	var notes []string
	currentLevel := 0

	flush := func() {
		if version == "" {
			return
		}
		clean := cleanNotes(notes)
		if len(clean) > 0 {
			releases = append(releases, Release{Version: version, Notes: clean})
		}
		version = ""
		notes = nil
		currentLevel = 0
	}

	for _, rawLine := range strings.Split(markdown, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if matches := headingRE.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			if parsedVersion, ok := parseVersionHeading(matches[2]); ok {
				flush()
				version = parsedVersion
				notes = nil
				currentLevel = level
				continue
			}
			if version != "" && level <= currentLevel {
				flush()
			}
			continue
		}

		if version == "" {
			continue
		}
		if note, ok := parseBullet(line); ok {
			notes = append(notes, note)
		}
	}

	flush()
	return releases
}

func Format(releases []Release) string {
	releases = cleanReleases(releases)
	if len(releases) == 0 {
		return ""
	}

	blocks := make([]string, 0, len(releases))
	for _, release := range releases {
		lines := []string{fmt.Sprintf("Version %s:", release.Version)}
		for _, note := range release.Notes {
			lines = append(lines, "· "+note)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func SelectNotes(fresh, cached []Release, opts Options) Result {
	fresh = limitReleases(fresh, opts.MaxVersions)
	cached = limitReleases(cached, opts.MaxVersions)
	link := changelogURL(opts)

	if !opts.FreshTimedOut && len(fresh) > 0 {
		return Result{
			Source:   SourceFresh,
			Releases: fresh,
			Text:     Format(fresh),
			Link:     link,
		}
	}

	if len(cached) > 0 {
		source := SourceCached
		if opts.FreshTimedOut {
			source = SourceTimeout
		}
		return Result{
			Source:   source,
			Releases: cached,
			Text:     Format(cached),
			Link:     link,
			TimedOut: opts.FreshTimedOut,
		}
	}

	source := SourceLink
	if opts.FreshTimedOut {
		source = SourceTimeout
	}
	return Result{
		Source:   source,
		Text:     linkText(link),
		Link:     link,
		TimedOut: opts.FreshTimedOut,
	}
}

func parseVersionHeading(heading string) (string, bool) {
	text := strings.TrimSpace(heading)
	if text == "" {
		return "", false
	}

	if strings.HasPrefix(text, "[") {
		if end := strings.Index(text, "]"); end > 1 {
			text = text[1:end]
		}
	}

	fields := strings.Fields(text)
	if len(fields) > 1 && strings.EqualFold(fields[0], "version") {
		text = strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	}

	if idx := strings.Index(text, " - "); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
	}

	text = strings.TrimSpace(text)
	if !versionRE.MatchString(text) {
		return "", false
	}
	return text, true
}

func parseBullet(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return "", false
	}
	switch {
	case strings.HasPrefix(trimmed, "- "):
		trimmed = strings.TrimSpace(trimmed[2:])
	case strings.HasPrefix(trimmed, "* "):
		trimmed = strings.TrimSpace(trimmed[2:])
	case strings.HasPrefix(trimmed, "+ "):
		trimmed = strings.TrimSpace(trimmed[2:])
	default:
		return "", false
	}
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func limitReleases(releases []Release, maxVersions int) []Release {
	releases = cleanReleases(releases)
	if maxVersions > 0 && len(releases) > maxVersions {
		return releases[:maxVersions]
	}
	return releases
}

func cleanReleases(releases []Release) []Release {
	clean := make([]Release, 0, len(releases))
	for _, release := range releases {
		version := strings.TrimSpace(release.Version)
		notes := cleanNotes(release.Notes)
		if version == "" || len(notes) == 0 {
			continue
		}
		clean = append(clean, Release{Version: version, Notes: notes})
	}
	return clean
}

func cleanNotes(notes []string) []string {
	clean := make([]string, 0, len(notes))
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		clean = append(clean, note)
	}
	return clean
}

func changelogURL(opts Options) string {
	if strings.TrimSpace(opts.ChangelogURL) != "" {
		return strings.TrimSpace(opts.ChangelogURL)
	}
	return DefaultChangelogURL
}

func linkText(link string) string {
	return fmt.Sprintf("See the full changelog at: %s", link)
}
