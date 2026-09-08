package runtime

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/desktopdata"
)

// ThreadListView selects which non-deleted threads a list query returns.
// Deleted records are excluded from every view; deletion always wins over
// archiving. Callers pass the explicit enum instead of raw boolean
// combinations (archive contract [查询合同](docs/01_桌面工作台/会话管理/实现方案.md#查询合同)).
type ThreadListView string

const (
	// ThreadListViewActive returns threads that are neither deleted nor
	// archived; it is the default sidebar authority.
	ThreadListViewActive ThreadListView = "active"
	// ThreadListViewArchived returns threads that are archived but not
	// deleted.
	ThreadListViewArchived ThreadListView = "archived"
	// ThreadListViewAll returns every non-deleted thread; it exists for
	// internal reconciliation and tests, not for the regular sidebar.
	ThreadListViewAll ThreadListView = "all"
)

// Both durable lists and merged live lists use the same ordering. Archive
// time takes priority only in the archived view; the thread ID breaks ties.
func sortThreadSnapshots(threads []ThreadSnapshot, view ThreadListView) {
	sort.Slice(threads, func(i, j int) bool {
		a, b := threads[i], threads[j]
		if view == ThreadListViewArchived && !a.ArchivedAt.Equal(b.ArchivedAt) {
			return a.ArchivedAt.After(b.ArchivedAt)
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ThreadID < b.ThreadID
	})
}

type ThreadStore interface {
	CreateThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error
	AppendRunInput(snapshot ThreadSnapshot, run Run, sessionID string, input string, attachments []RunInputAttachment) error
	AppendConversation(snapshot ThreadSnapshot, run Run, messages []convcontext.Message) error
	AppendRunEvents(snapshot ThreadSnapshot, run Run, events []Event) error
	ListThreads(view ThreadListView) ([]ThreadSnapshot, error)
	RestoreThread(threadID string) (storedThreadRecord, restoredTranscript, error)
	GetTranscript(threadID string) (ThreadTranscript, error)
	RenameThread(threadID, label string, updatedAt time.Time) (ThreadSnapshot, error)
	UpdateThreadTitleMetadata(threadID, label string, source ThreadTitleSource, status ThreadTitleStatus, seedRunID string, updatedAt time.Time) (ThreadSnapshot, error)
	SetThreadArchived(threadID string, archived bool, now time.Time) (ThreadSnapshot, error)
	DeleteThread(threadID string) error
}

type storedThreadRecord struct {
	Snapshot        ThreadSnapshot
	ModelProfileKey string
	Preview         string
}

// awakenHomeEnv is the business data root injected by Desktop Main
// (~/.awaken in production). It is the only pointer to the durable store.
const awakenHomeEnv = "AWAKEN_HOME"

// resolveRuntimeStoreRoot returns the FileStore business root:
//  1. AWAKEN_HOME when set (Desktop injects ~/.awaken);
//  2. empty when it is absent, so standalone callers choose an isolated temp
//     root instead of treating Electron userData as business authority.
func resolveRuntimeStoreRoot() string {
	return desktopdata.AwakenHomeRoot()
}

func newThreadStore() DesktopRepository {
	root := resolveRuntimeStoreRoot()
	if root == "" {
		dir, err := os.MkdirTemp("", "awaken-store-")
		if err != nil {
			panic(fmt.Sprintf("no AWAKEN_HOME and temp store root unavailable: %v", err))
		}
		root = dir
	}
	store, err := newFileDesktopStore(root)
	if err != nil {
		panic(fmt.Sprintf("cannot open Awaken FileStore at %s (failing closed): %v", root, err))
	}
	return store
}

func parseStoredTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func formatStoredTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func maxTime(values ...time.Time) time.Time {
	var latest time.Time
	for _, value := range values {
		if value.After(latest) {
			latest = value
		}
	}
	return latest
}
