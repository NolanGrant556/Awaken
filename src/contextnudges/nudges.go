package contextnudges

import (
	"math"
	"os"
	"path/filepath"
	"strings"
)

const ResidueDirName = ".openclaw"

type Tip struct {
	ID               string
	Text             string
	CooldownSessions int
	Signals          Signals
}

type Signals struct {
	CLI       []string
	Tools     []string
	ReadFiles []string
	Context   []string
}

type Context struct {
	TipsDisabled  bool
	SessionNumber int
	History       History

	CLISignals     []string
	ToolSignals    []string
	ReadFilePaths  []string
	ContextSignals []string
}

type History struct {
	LastShown map[string]int
}

type Selection struct {
	Tip                    Tip
	SessionsSinceLastShown int
	Record                 RecordPlan
}

type FirstTouchFlag string

const (
	FirstTouchBusyInput      FirstTouchFlag = "busy_input_prompt"
	FirstTouchToolProgress   FirstTouchFlag = "tool_progress_prompt"
	FirstTouchResidueCleanup FirstTouchFlag = "openclaw_residue_cleanup"
)

type BusyInputMode string

const (
	BusyInputQueue     BusyInputMode = "queue"
	BusyInputSteer     BusyInputMode = "steer"
	BusyInputInterrupt BusyInputMode = "interrupt"
)

type Store interface {
	MarkFirstTouchSeen(flag FirstTouchFlag) error
}

type RecordPlan struct {
	TipID          string
	SessionNumber  int
	FirstTouchFlag FirstTouchFlag
}

func RelevantTips(tips []Tip, ctx Context) []Tip {
	if ctx.TipsDisabled {
		return nil
	}

	relevant := make([]Tip, 0, len(tips))
	for _, tip := range tips {
		if !matchesContext(tip.Signals, ctx) {
			continue
		}
		if SessionsSinceLastShown(ctx.History, tip.ID, ctx.SessionNumber) < normalizedCooldown(tip.CooldownSessions) {
			continue
		}
		relevant = append(relevant, tip)
	}
	return relevant
}

func SelectTipWithLongestTimeSinceShown(tips []Tip, history History, sessionNumber int) (Selection, bool) {
	if len(tips) == 0 {
		return Selection{}, false
	}

	bestTip := tips[0]
	bestSessions := SessionsSinceLastShown(history, bestTip.ID, sessionNumber)
	for _, tip := range tips[1:] {
		sessions := SessionsSinceLastShown(history, tip.ID, sessionNumber)
		if sessions > bestSessions {
			bestTip = tip
			bestSessions = sessions
		}
	}

	return Selection{
		Tip:                    bestTip,
		SessionsSinceLastShown: bestSessions,
		Record: RecordPlan{
			TipID:         bestTip.ID,
			SessionNumber: sessionNumber,
		},
	}, true
}

func SelectTip(tips []Tip, ctx Context) (Selection, bool) {
	return SelectTipWithLongestTimeSinceShown(RelevantTips(tips, ctx), ctx.History, ctx.SessionNumber)
}

func SessionsSinceLastShown(history History, tipID string, sessionNumber int) int {
	if history.LastShown == nil {
		return math.MaxInt
	}
	lastShown, ok := history.LastShown[tipID]
	if !ok {
		return math.MaxInt
	}
	return sessionNumber - lastShown
}

func RecordShown(history History, tipID string, sessionNumber int) History {
	if tipID == "" {
		return cloneHistory(history)
	}
	if history.LastShown != nil {
		if lastShown, ok := history.LastShown[tipID]; ok && lastShown == sessionNumber {
			return history
		}
	}

	next := cloneHistory(history)
	if next.LastShown == nil {
		next.LastShown = make(map[string]int, 1)
	}
	next.LastShown[tipID] = sessionNumber
	return next
}

func (p RecordPlan) ApplyToHistory(history History) History {
	if p.TipID == "" {
		return history
	}
	return RecordShown(history, p.TipID, p.SessionNumber)
}

func (p RecordPlan) MarkSeenBestEffort(store Store) bool {
	if p.FirstTouchFlag == "" || store == nil {
		return false
	}
	return store.MarkFirstTouchSeen(p.FirstTouchFlag) == nil
}

func FirstTouchHint(flag FirstTouchFlag, mode string, seen bool) (string, RecordPlan, bool) {
	if seen {
		return "", RecordPlan{}, false
	}

	var hint string
	switch flag {
	case FirstTouchBusyInput:
		hint = busyInputHint(mode)
	case FirstTouchToolProgress:
		hint = "Tip: that tool is taking a while, so Awaken can keep streaming progress. Use /verbose to change progress visibility. This tip only shows once."
	case FirstTouchResidueCleanup:
		hint = "Heads up: an old OpenClaw workspace was detected at ~/.openclaw. Archive it after migration so Awaken does not read stale config or memory. This tip only shows once."
	default:
		return "", RecordPlan{}, false
	}

	return hint, RecordPlan{FirstTouchFlag: flag}, true
}

func DetectResiduePath(home string) (string, bool) {
	if strings.TrimSpace(home) == "" {
		return "", false
	}
	path := filepath.Join(home, ResidueDirName)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return path, true
}

func busyInputHint(mode string) string {
	switch BusyInputMode(strings.ToLower(strings.TrimSpace(mode))) {
	case BusyInputQueue:
		return "Tip: your message was queued for the next turn. Use /busy interrupt to stop the current run, or /busy steer to inject mid-run. This tip only shows once."
	case BusyInputSteer:
		return "Tip: your message was steered into the current run and will land after the next tool call. Use /busy interrupt or /busy queue to change this. This tip only shows once."
	default:
		return "Tip: your message interrupted the current run. Use /busy queue to wait for the next turn, or /busy steer to inject mid-run. This tip only shows once."
	}
}

func matchesContext(signals Signals, ctx Context) bool {
	if len(signals.CLI) == 0 && len(signals.Tools) == 0 && len(signals.ReadFiles) == 0 && len(signals.Context) == 0 {
		return true
	}
	return anySignalMatches(signals.CLI, ctx.CLISignals) ||
		anySignalMatches(signals.Tools, ctx.ToolSignals) ||
		anyFileSignalMatches(signals.ReadFiles, ctx.ReadFilePaths) ||
		anySignalMatches(signals.Context, ctx.ContextSignals)
}

func anySignalMatches(expected []string, actual []string) bool {
	for _, want := range expected {
		for _, got := range actual {
			if strings.EqualFold(strings.TrimSpace(want), strings.TrimSpace(got)) {
				return true
			}
		}
	}
	return false
}

func anyFileSignalMatches(patterns []string, paths []string) bool {
	for _, pattern := range patterns {
		for _, path := range paths {
			if fileSignalMatches(pattern, path) {
				return true
			}
		}
	}
	return false
}

func fileSignalMatches(pattern string, path string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	path = filepath.ToSlash(strings.TrimSpace(path))
	if pattern == "" || path == "" {
		return false
	}
	if strings.EqualFold(pattern, path) {
		return true
	}
	if strings.HasPrefix(pattern, ".") && strings.EqualFold(filepath.Ext(path), pattern) {
		return true
	}
	if strings.ContainsAny(pattern, "*?[") {
		if ok, _ := filepath.Match(pattern, path); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, filepath.Base(path)); ok {
			return true
		}
	}
	return strings.Contains(strings.ToLower(path), strings.ToLower(pattern))
}

func normalizedCooldown(cooldown int) int {
	if cooldown < 0 {
		return 0
	}
	return cooldown
}

func cloneHistory(history History) History {
	if history.LastShown == nil {
		return History{}
	}
	next := History{LastShown: make(map[string]int, len(history.LastShown))}
	for tipID, sessionNumber := range history.LastShown {
		next.LastShown[tipID] = sessionNumber
	}
	return next
}
