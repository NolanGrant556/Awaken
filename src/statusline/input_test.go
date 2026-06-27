package statusline

import (
	"reflect"
	"testing"
)

func TestBuildInputNormalizesPathsAndAddedDirs(t *testing.T) {
	input := BuildInput(Config{
		SessionName:      "Morning run",
		SessionID:        "session-1",
		ModelID:          "model-id",
		ModelDisplayName: "Model",
		CurrentDir:       " /tmp//awaken/ ",
		ProjectDir:       "/tmp/awaken/./project",
		AddedDirs: []string{
			"/tmp/awaken/b/",
			"/tmp/awaken/a//",
			"/tmp/awaken/b",
			"",
			"  /tmp/awaken/c/../c  ",
		},
		Worktree: Worktree{
			Name:        "status-line-runtime",
			Path:        "/tmp//awaken-wt/",
			OriginalCwd: "/tmp/awaken/../awaken",
		},
	})

	if input.CurrentDir != "/tmp/awaken" {
		t.Fatalf("expected clean current dir, got %q", input.CurrentDir)
	}
	if input.ProjectDir != "/tmp/awaken/project" {
		t.Fatalf("expected clean project dir, got %q", input.ProjectDir)
	}

	expectedAddedDirs := []string{"/tmp/awaken/a", "/tmp/awaken/b", "/tmp/awaken/c"}
	if !reflect.DeepEqual(input.AddedDirs, expectedAddedDirs) {
		t.Fatalf("expected added dirs %v, got %v", expectedAddedDirs, input.AddedDirs)
	}
	if input.Worktree.Path != "/tmp/awaken-wt" {
		t.Fatalf("expected clean worktree path, got %q", input.Worktree.Path)
	}
	if input.Worktree.OriginalCwd != "/tmp/awaken" {
		t.Fatalf("expected clean original cwd, got %q", input.Worktree.OriginalCwd)
	}
}

func TestBuildInputClampsPercentages(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected Input
	}{
		{
			name: "context window lower and upper bounds",
			config: Config{
				ContextWindow: ContextWindow{
					UsedPercentage:      -12,
					RemainingPercentage: 140,
				},
			},
			expected: Input{
				ContextWindow: ContextWindow{
					UsedPercentage:      0,
					RemainingPercentage: 100,
				},
			},
		},
		{
			name: "rate limit lower and upper bounds",
			config: Config{
				RateLimits: RateLimits{
					FiveHour: RateLimit{UsedPercentage: 101},
					SevenDay: RateLimit{UsedPercentage: -1},
				},
			},
			expected: Input{
				RateLimits: RateLimits{
					FiveHour: RateLimit{UsedPercentage: 100},
					SevenDay: RateLimit{UsedPercentage: 0},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := BuildInput(tt.config)
			if input.ContextWindow != tt.expected.ContextWindow {
				t.Fatalf("expected context window %+v, got %+v", tt.expected.ContextWindow, input.ContextWindow)
			}
			if input.RateLimits != tt.expected.RateLimits {
				t.Fatalf("expected rate limits %+v, got %+v", tt.expected.RateLimits, input.RateLimits)
			}
		})
	}
}
