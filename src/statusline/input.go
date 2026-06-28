package statusline

import (
	"path"
	"sort"
	"strings"
)

type Config struct {
	SessionName      string
	SessionID        string
	ModelID          string
	ModelDisplayName string
	CurrentDir       string
	ProjectDir       string
	AddedDirs        []string
	Version          string
	OutputStyle      string
	Cost             Cost
	ContextWindow    ContextWindow
	RateLimits       RateLimits
	AgentName        string
	RemoteSessionID  string
	Worktree         Worktree
}

type Input struct {
	SessionName      string
	SessionID        string
	ModelID          string
	ModelDisplayName string
	CurrentDir       string
	ProjectDir       string
	AddedDirs        []string
	Version          string
	OutputStyle      string
	Cost             Cost
	ContextWindow    ContextWindow
	RateLimits       RateLimits
	AgentName        string
	RemoteSessionID  string
	Worktree         Worktree
}

type Cost struct {
	TotalCostUSD       float64
	TotalDurationMS    int64
	TotalAPIDurationMS int64
	TotalLinesAdded    int
	TotalLinesRemoved  int
}

type ContextWindow struct {
	TotalInputTokens    int
	TotalOutputTokens   int
	ContextWindowSize   int
	CurrentUsage        int
	UsedPercentage      float64
	RemainingPercentage float64
	Exceeds200KTokens   bool
}

type RateLimits struct {
	FiveHour RateLimit
	SevenDay RateLimit
}

type RateLimit struct {
	UsedPercentage float64
	ResetsAt       string
}

type Worktree struct {
	Name           string
	Path           string
	Branch         string
	OriginalCwd    string
	OriginalBranch string
}

func BuildInput(config Config) Input {
	addedDirs := normalizeAddedDirs(config.AddedDirs)
	return Input{
		SessionName:      config.SessionName,
		SessionID:        config.SessionID,
		ModelID:          config.ModelID,
		ModelDisplayName: config.ModelDisplayName,
		CurrentDir:       cleanSlashPath(config.CurrentDir),
		ProjectDir:       cleanSlashPath(config.ProjectDir),
		AddedDirs:        addedDirs,
		Version:          config.Version,
		OutputStyle:      config.OutputStyle,
		Cost:             config.Cost,
		ContextWindow:    normalizeContextWindow(config.ContextWindow),
		RateLimits:       normalizeRateLimits(config.RateLimits),
		AgentName:        config.AgentName,
		RemoteSessionID:  config.RemoteSessionID,
		Worktree:         normalizeWorktree(config.Worktree),
	}
}

func normalizeContextWindow(window ContextWindow) ContextWindow {
	window.UsedPercentage = clampPercentage(window.UsedPercentage)
	window.RemainingPercentage = clampPercentage(window.RemainingPercentage)
	return window
}

func normalizeRateLimits(limits RateLimits) RateLimits {
	limits.FiveHour.UsedPercentage = clampPercentage(limits.FiveHour.UsedPercentage)
	limits.SevenDay.UsedPercentage = clampPercentage(limits.SevenDay.UsedPercentage)
	return limits
}

func normalizeWorktree(worktree Worktree) Worktree {
	worktree.Path = cleanSlashPath(worktree.Path)
	worktree.OriginalCwd = cleanSlashPath(worktree.OriginalCwd)
	return worktree
}

func normalizeAddedDirs(dirs []string) []string {
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		cleaned := cleanSlashPath(dir)
		if cleaned == "" {
			continue
		}
		seen[cleaned] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(seen))
	for dir := range seen {
		normalized = append(normalized, dir)
	}
	sort.Strings(normalized)
	return normalized
}

func cleanSlashPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "/")
	return path.Clean(value)
}

func clampPercentage(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
