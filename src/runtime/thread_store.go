package runtime

import (
	"fmt"
	"os"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/desktopdata"
)

type ThreadStore interface {
	CreateThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error
	AppendRunInput(snapshot ThreadSnapshot, run Run, sessionID string, input string, attachments []RunInputAttachment) error
	AppendConversation(snapshot ThreadSnapshot, run Run, messages []convcontext.Message) error
	AppendRunEvents(snapshot ThreadSnapshot, run Run, events []Event) error
	ListThreads() ([]ThreadSnapshot, error)
	RestoreThread(threadID string) (storedThreadRecord, restoredTranscript, error)
	GetTranscript(threadID string) (ThreadTranscript, error)
	RenameThread(threadID, label string, updatedAt time.Time) (ThreadSnapshot, error)
	UpdateThreadTitleMetadata(threadID, label string, source ThreadTitleSource, status ThreadTitleStatus, seedRunID string, updatedAt time.Time) (ThreadSnapshot, error)
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

func newThreadStore() ThreadStore {
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
