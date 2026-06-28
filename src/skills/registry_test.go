package skills

import (
	"errors"
	"strings"
	"testing"
)

func TestNewStaticRegistryBuildsSnapshot(t *testing.T) {
	registry, err := NewStaticRegistry([]Definition{
		{
			ID:            "code-review",
			Name:          "Code Review",
			Description:   "Review a code change for regressions",
			Instructions:  "Focus on correctness and missing tests.",
			PromptSnippet: "Review these diffs carefully.",
			DefaultTools:  []string{"read_file", "rg"},
			Enabled:       true,
			Source:        "builtin",
		},
		{
			ID:           "triage",
			Name:         "Bug Triage",
			Description:  "Triage a failing issue report",
			DefaultTools: []string{"read_file"},
			Enabled:      false,
			Source:       "workspace",
		},
	})
	if err != nil {
		t.Fatalf("failed to build registry: %v", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.Summary.SkillCount != 2 || snapshot.Summary.EnabledSkillCount != 1 {
		t.Fatalf("unexpected snapshot summary: %+v", snapshot.Summary)
	}
	if snapshot.Skills[0].ID != "code-review" || snapshot.Skills[1].ID != "triage" {
		t.Fatalf("expected sorted skill ids, got %+v", snapshot.Skills)
	}
	if len(snapshot.Skills[0].DefaultTools) != 2 || snapshot.Skills[0].DefaultTools[0] != "read_file" {
		t.Fatalf("expected cloned and sorted default tools, got %+v", snapshot.Skills[0].DefaultTools)
	}
}

func TestNewRegistryRejectsDuplicateSkillIDs(t *testing.T) {
	_, err := NewStaticRegistry([]Definition{
		{ID: "dup", Name: "One", Description: "first"},
		{ID: "dup", Name: "Two", Description: "second"},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate skill id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestNewRegistryPropagatesLoaderError(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewRegistry(StaticLoader{Err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("expected loader error, got %v", err)
	}
}
