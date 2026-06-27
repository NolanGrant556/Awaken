package sessiontags

import "strings"

type Mode string

const (
	ModeCoordinator Mode = "coordinator"
	ModeNormal      Mode = "normal"
)

type WorktreeSession struct {
	OriginalCwd        string
	WorktreePath       string
	WorktreeName       string
	WorktreeBranch     string
	OriginalBranch     string
	OriginalHeadCommit string
	SessionID          string
	TmuxSessionName    string
	HookBased          bool
}

type Metadata struct {
	SessionID            string
	CustomTitle          string
	AITitle              string
	Tag                  string
	LastPrompt           string
	TaskSummary          string
	TaskSummaryTimestamp string
	AgentName            string
	AgentColor           string
	AgentSetting         string
	Mode                 Mode
	WorktreeSession      *WorktreeSession
}

func (m Metadata) DisplayTitle() string {
	if m.CustomTitle != "" {
		return m.CustomTitle
	}
	return m.AITitle
}

type Entry interface {
	isSessionTagEntry()
	sessionID() string
	applyTo(*Metadata)
}

type CustomTitleEntry struct {
	SessionID   string
	CustomTitle string
}

func (e CustomTitleEntry) isSessionTagEntry() {}
func (e CustomTitleEntry) sessionID() string  { return e.SessionID }
func (e CustomTitleEntry) applyTo(m *Metadata) {
	m.CustomTitle = e.CustomTitle
}

type AITitleEntry struct {
	SessionID string
	AITitle   string
}

func (e AITitleEntry) isSessionTagEntry() {}
func (e AITitleEntry) sessionID() string  { return e.SessionID }
func (e AITitleEntry) applyTo(m *Metadata) {
	m.AITitle = e.AITitle
}

type LastPromptEntry struct {
	SessionID  string
	LastPrompt string
}

func (e LastPromptEntry) isSessionTagEntry() {}
func (e LastPromptEntry) sessionID() string  { return e.SessionID }
func (e LastPromptEntry) applyTo(m *Metadata) {
	m.LastPrompt = e.LastPrompt
}

type TaskSummaryEntry struct {
	SessionID string
	Summary   string
	Timestamp string
}

func (e TaskSummaryEntry) isSessionTagEntry() {}
func (e TaskSummaryEntry) sessionID() string  { return e.SessionID }
func (e TaskSummaryEntry) applyTo(m *Metadata) {
	m.TaskSummary = e.Summary
	m.TaskSummaryTimestamp = e.Timestamp
}

type TagEntry struct {
	SessionID string
	Tag       string
}

func (e TagEntry) isSessionTagEntry() {}
func (e TagEntry) sessionID() string  { return e.SessionID }
func (e TagEntry) applyTo(m *Metadata) {
	m.Tag = e.Tag
}

type AgentNameEntry struct {
	SessionID string
	AgentName string
}

func (e AgentNameEntry) isSessionTagEntry() {}
func (e AgentNameEntry) sessionID() string  { return e.SessionID }
func (e AgentNameEntry) applyTo(m *Metadata) {
	m.AgentName = e.AgentName
}

type AgentColorEntry struct {
	SessionID  string
	AgentColor string
}

func (e AgentColorEntry) isSessionTagEntry() {}
func (e AgentColorEntry) sessionID() string  { return e.SessionID }
func (e AgentColorEntry) applyTo(m *Metadata) {
	m.AgentColor = e.AgentColor
}

type AgentSettingEntry struct {
	SessionID    string
	AgentSetting string
}

func (e AgentSettingEntry) isSessionTagEntry() {}
func (e AgentSettingEntry) sessionID() string  { return e.SessionID }
func (e AgentSettingEntry) applyTo(m *Metadata) {
	m.AgentSetting = e.AgentSetting
}

type ModeEntry struct {
	SessionID string
	Mode      Mode
}

func (e ModeEntry) isSessionTagEntry() {}
func (e ModeEntry) sessionID() string  { return e.SessionID }
func (e ModeEntry) applyTo(m *Metadata) {
	m.Mode = e.Mode
}

type WorktreeStateEntry struct {
	SessionID       string
	WorktreeSession *WorktreeSession
}

func (e WorktreeStateEntry) isSessionTagEntry() {}
func (e WorktreeStateEntry) sessionID() string  { return e.SessionID }
func (e WorktreeStateEntry) applyTo(m *Metadata) {
	m.WorktreeSession = cloneWorktreeSession(e.WorktreeSession)
}

func Apply(metadata Metadata, entry Entry) Metadata {
	if entry == nil {
		return metadata
	}

	sessionID := entry.sessionID()
	if metadata.SessionID == "" {
		metadata.SessionID = sessionID
	}
	if metadata.SessionID != sessionID {
		return metadata
	}

	entry.applyTo(&metadata)
	return metadata
}

func Fold(entries ...Entry) map[string]Metadata {
	index := NewIndex(entries...)
	return index.MetadataBySession()
}

type Index struct {
	sessions map[string]Metadata
	byTag    map[string]map[string]struct{}
	seen     map[string]struct{}
	order    []string
}

func NewIndex(entries ...Entry) *Index {
	index := &Index{}
	for _, entry := range entries {
		index.Apply(entry)
	}
	return index
}

func (i *Index) Apply(entry Entry) {
	if entry == nil {
		return
	}
	i.ensure()

	sessionID := entry.sessionID()
	if sessionID == "" {
		return
	}

	metadata, ok := i.sessions[sessionID]
	if !ok {
		metadata = Metadata{SessionID: sessionID}
		i.remember(sessionID)
	}

	oldTag := metadata.Tag
	metadata = Apply(metadata, entry)
	i.sessions[sessionID] = metadata

	if oldTag != metadata.Tag {
		i.updateTag(sessionID, oldTag, metadata.Tag)
	}
}

func (i *Index) Metadata(sessionID string) (Metadata, bool) {
	if i == nil {
		return Metadata{}, false
	}
	metadata, ok := i.sessions[sessionID]
	if !ok {
		return Metadata{}, false
	}
	return cloneMetadata(metadata), true
}

func (i *Index) MetadataBySession() map[string]Metadata {
	result := make(map[string]Metadata)
	if i == nil {
		return result
	}
	for sessionID, metadata := range i.sessions {
		result[sessionID] = cloneMetadata(metadata)
	}
	return result
}

func (i *Index) SessionIDsByTag(tag string) []string {
	if i == nil || tag == "" {
		return nil
	}

	tagged := i.byTag[tag]
	if len(tagged) == 0 {
		return nil
	}

	var sessionIDs []string
	for _, sessionID := range i.order {
		if _, ok := tagged[sessionID]; ok {
			sessionIDs = append(sessionIDs, sessionID)
		}
	}
	return sessionIDs
}

func (i *Index) Search(query string) []Metadata {
	if i == nil {
		return nil
	}

	needle := normalizeSearch(query)
	if needle == "" {
		return nil
	}

	var results []Metadata
	for _, sessionID := range i.order {
		metadata := i.sessions[sessionID]
		if searchMatches(metadata.DisplayTitle(), needle) || searchMatches(metadata.LastPrompt, needle) {
			results = append(results, cloneMetadata(metadata))
		}
	}
	return results
}

func (i *Index) SearchSessionIDs(query string) []string {
	matches := i.Search(query)
	if len(matches) == 0 {
		return nil
	}

	sessionIDs := make([]string, 0, len(matches))
	for _, metadata := range matches {
		sessionIDs = append(sessionIDs, metadata.SessionID)
	}
	return sessionIDs
}

func (i *Index) ensure() {
	if i.sessions == nil {
		i.sessions = make(map[string]Metadata)
	}
	if i.byTag == nil {
		i.byTag = make(map[string]map[string]struct{})
	}
	if i.seen == nil {
		i.seen = make(map[string]struct{})
	}
}

func (i *Index) remember(sessionID string) {
	if _, ok := i.seen[sessionID]; ok {
		return
	}
	i.seen[sessionID] = struct{}{}
	i.order = append(i.order, sessionID)
}

func (i *Index) updateTag(sessionID, oldTag, newTag string) {
	if oldTag != "" {
		delete(i.byTag[oldTag], sessionID)
		if len(i.byTag[oldTag]) == 0 {
			delete(i.byTag, oldTag)
		}
	}

	if newTag == "" {
		return
	}

	if i.byTag[newTag] == nil {
		i.byTag[newTag] = make(map[string]struct{})
	}
	i.byTag[newTag][sessionID] = struct{}{}
}

func normalizeSearch(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func searchMatches(value, normalizedQuery string) bool {
	return strings.Contains(strings.ToLower(value), normalizedQuery)
}

func cloneMetadata(metadata Metadata) Metadata {
	metadata.WorktreeSession = cloneWorktreeSession(metadata.WorktreeSession)
	return metadata
}

func cloneWorktreeSession(session *WorktreeSession) *WorktreeSession {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}
