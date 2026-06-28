package sessionbranch

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	TypeUser               = "user"
	TypeAssistant          = "assistant"
	TypeAttachment         = "attachment"
	TypeSystem             = "system"
	TypeProgress           = "progress"
	TypeContentReplacement = "content-replacement"

	DefaultFirstPrompt = "Branched conversation"
)

var (
	ErrNoTranscript       = errors.New("no transcript to branch")
	ErrNoMessages         = errors.New("no messages to branch")
	ErrMissingForkSession = errors.New("fork session id is required")
)

type SerializedMessage struct {
	UUID        string         `json:"uuid,omitempty"`
	Type        string         `json:"type"`
	SessionID   string         `json:"sessionId,omitempty"`
	ParentUUID  *string        `json:"parentUuid,omitempty"`
	IsSidechain bool           `json:"isSidechain,omitempty"`
	Content     any            `json:"content,omitempty"`
	ForkedFrom  *ForkedFrom    `json:"forkedFrom,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Entry struct {
	SerializedMessage
	Replacements []ContentReplacementRecord `json:"replacements,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ContentReplacementRecord struct {
	Kind        string         `json:"kind"`
	ToolUseID   string         `json:"toolUseId"`
	Replacement string         `json:"replacement"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ForkedFrom struct {
	SessionID   string `json:"sessionId"`
	MessageUUID string `json:"messageUuid"`
}

type Options struct {
	ForkSessionID     string
	OriginalSessionID string
	CustomTitle       string
	ExistingTitles    []string
}

type Fork struct {
	SessionID           string
	OriginalSessionID   string
	Title               string
	Entries             []Entry
	Messages            []SerializedMessage
	ContentReplacements []ContentReplacementRecord
}

func DeriveFirstPrompt(messages []SerializedMessage) string {
	for _, message := range messages {
		if message.Type != TypeUser || message.IsSidechain {
			continue
		}
		if raw, ok := firstTextContent(message.Content); ok {
			return normalizePrompt(raw)
		}
	}
	return DefaultFirstPrompt
}

func BuildFork(entries []Entry, opts Options) (Fork, error) {
	if len(entries) == 0 {
		return Fork{}, ErrNoTranscript
	}

	mainEntries := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if isMainTranscriptMessage(entry) {
			mainEntries = append(mainEntries, entry)
		}
	}
	if len(mainEntries) == 0 {
		return Fork{}, ErrNoMessages
	}

	forkSessionID := strings.TrimSpace(opts.ForkSessionID)
	if forkSessionID == "" {
		return Fork{}, ErrMissingForkSession
	}

	originalSessionID := strings.TrimSpace(opts.OriginalSessionID)
	if originalSessionID == "" {
		originalSessionID = mainEntries[0].SessionID
	}

	forkEntries := make([]Entry, 0, len(mainEntries)+1)
	messages := make([]SerializedMessage, 0, len(mainEntries))
	var parentUUID *string

	for _, entry := range mainEntries {
		forkEntry := cloneEntry(entry)
		forkEntry.SessionID = forkSessionID
		forkEntry.ParentUUID = cloneStringPtr(parentUUID)
		forkEntry.IsSidechain = false
		forkEntry.ForkedFrom = &ForkedFrom{
			SessionID:   originalSessionID,
			MessageUUID: entry.UUID,
		}

		forkEntries = append(forkEntries, forkEntry)
		messages = append(messages, forkEntry.SerializedMessage)
		if entry.Type != TypeProgress {
			parentUUID = stringPtr(entry.UUID)
		}
	}

	replacements := collectContentReplacementRecords(entries, originalSessionID)
	if len(replacements) > 0 {
		forkEntries = append(forkEntries, Entry{
			SerializedMessage: SerializedMessage{
				Type:      TypeContentReplacement,
				SessionID: forkSessionID,
			},
			Replacements: cloneReplacementRecords(replacements),
		})
	}

	baseTitle := strings.TrimSpace(opts.CustomTitle)
	if baseTitle == "" {
		baseTitle = DeriveFirstPrompt(messages)
	}

	return Fork{
		SessionID:           forkSessionID,
		OriginalSessionID:   originalSessionID,
		Title:               UniqueForkName(baseTitle, opts.ExistingTitles),
		Entries:             forkEntries,
		Messages:            messages,
		ContentReplacements: cloneReplacementRecords(replacements),
	}, nil
}

func UniqueForkName(base string, existing []string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = DefaultFirstPrompt
	}

	candidate := base + " (Branch)"
	if !containsTitle(existing, candidate) {
		return candidate
	}

	usedNumbers := map[int]bool{1: true}
	for _, title := range existing {
		if number, ok := branchNumber(base, title); ok {
			usedNumbers[number] = true
		}
	}

	next := 2
	for usedNumbers[next] {
		next++
	}
	return fmt.Sprintf("%s (Branch %d)", base, next)
}

func firstTextContent(content any) (string, bool) {
	switch value := content.(type) {
	case string:
		return value, true
	case ContentBlock:
		return textFromBlock(value)
	case []ContentBlock:
		for _, block := range value {
			if text, ok := textFromBlock(block); ok {
				return text, true
			}
		}
	case []map[string]any:
		for _, block := range value {
			if text, ok := textFromMap(block); ok {
				return text, true
			}
		}
	case []map[string]string:
		for _, block := range value {
			if block["type"] == "text" {
				return block["text"], true
			}
		}
	case []any:
		for _, block := range value {
			if text, ok := textFromAnyBlock(block); ok {
				return text, true
			}
		}
	case map[string]any:
		return textFromMap(value)
	case map[string]string:
		if value["type"] == "text" {
			return value["text"], true
		}
	}
	return "", false
}

func textFromAnyBlock(block any) (string, bool) {
	switch value := block.(type) {
	case ContentBlock:
		return textFromBlock(value)
	case map[string]any:
		return textFromMap(value)
	case map[string]string:
		if value["type"] == "text" {
			return value["text"], true
		}
	}
	return "", false
}

func textFromBlock(block ContentBlock) (string, bool) {
	if block.Type != "text" {
		return "", false
	}
	return block.Text, true
}

func textFromMap(block map[string]any) (string, bool) {
	blockType, _ := block["type"].(string)
	if blockType != "text" {
		return "", false
	}
	text, ok := block["text"].(string)
	return text, ok
}

func normalizePrompt(raw string) string {
	prompt := strings.Join(strings.Fields(raw), " ")
	if prompt == "" {
		return DefaultFirstPrompt
	}

	runes := []rune(prompt)
	if len(runes) > 100 {
		return string(runes[:100])
	}
	return prompt
}

func isMainTranscriptMessage(entry Entry) bool {
	if entry.IsSidechain {
		return false
	}
	switch entry.Type {
	case TypeUser, TypeAssistant, TypeAttachment, TypeSystem:
		return true
	default:
		return false
	}
}

func collectContentReplacementRecords(entries []Entry, originalSessionID string) []ContentReplacementRecord {
	var records []ContentReplacementRecord
	for _, entry := range entries {
		if entry.Type != TypeContentReplacement || entry.SessionID != originalSessionID {
			continue
		}
		records = append(records, cloneReplacementRecords(entry.Replacements)...)
	}
	return records
}

func containsTitle(titles []string, target string) bool {
	for _, title := range titles {
		if title == target {
			return true
		}
	}
	return false
}

func branchNumber(base, title string) (int, bool) {
	if title == base+" (Branch)" {
		return 1, true
	}

	prefix := base + " (Branch "
	if !strings.HasPrefix(title, prefix) || !strings.HasSuffix(title, ")") {
		return 0, false
	}

	rawNumber := strings.TrimSuffix(strings.TrimPrefix(title, prefix), ")")
	number, err := strconv.Atoi(rawNumber)
	if err != nil || number < 1 {
		return 0, false
	}
	return number, true
}

func cloneEntry(entry Entry) Entry {
	clone := entry
	clone.ParentUUID = cloneStringPtr(entry.ParentUUID)
	clone.ForkedFrom = cloneForkedFrom(entry.ForkedFrom)
	clone.Metadata = cloneMetadata(entry.Metadata)
	clone.Replacements = cloneReplacementRecords(entry.Replacements)
	return clone
}

func cloneReplacementRecords(records []ContentReplacementRecord) []ContentReplacementRecord {
	if len(records) == 0 {
		return nil
	}

	clone := make([]ContentReplacementRecord, len(records))
	for i, record := range records {
		clone[i] = record
		clone[i].Metadata = cloneMetadata(record.Metadata)
	}
	return clone
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}

	clone := make(map[string]any, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func cloneForkedFrom(forkedFrom *ForkedFrom) *ForkedFrom {
	if forkedFrom == nil {
		return nil
	}
	clone := *forkedFrom
	return &clone
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	return stringPtr(*value)
}

func stringPtr(value string) *string {
	return &value
}
