package settingssync

import (
	"fmt"
	"sort"
)

const DefaultMaxEntryBytes = 500 * 1024

type Source string

const (
	SourceLocal         Source = "local"
	SourceUser          Source = "user"
	SourceProject       Source = "project"
	SourceProfile       Source = "profile"
	SourceRemote        Source = "remote"
	SourceRemoteManaged Source = "remote_managed"
)

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
	ScopeProfile Scope = "profile"
	ScopeMachine Scope = "machine"
)

type Entry struct {
	Key       string
	Value     string
	Source    Source
	Scope     Scope
	ProjectID string
	Profile   string
	Sensitive bool
}

func (e Entry) SizeBytes() int {
	return len([]byte(e.Value))
}

func (e Entry) IsSensitive() bool {
	return e.Sensitive || IsSensitiveKey(e.Key) || LooksSensitiveValue(e.Value)
}

func (e Entry) RedactedValue() string {
	return RedactValue(e.Key, e.Value, e.Sensitive)
}

func (e Entry) String() string {
	return fmt.Sprintf(
		"settingssync.Entry{key:%q source:%q scope:%q bytes:%d value:%q}",
		e.Key,
		e.Source,
		e.Scope,
		e.SizeBytes(),
		e.RedactedValue(),
	)
}

type ManifestEntry struct {
	Key       string `json:"key"`
	Source    Source `json:"source"`
	Scope     Scope  `json:"scope"`
	ProjectID string `json:"projectId,omitempty"`
	Profile   string `json:"profile,omitempty"`
	SizeBytes int    `json:"sizeBytes"`
	Checksum  string `json:"checksum"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type SyncManifest struct {
	Version      int                      `json:"version"`
	Checksum     string                   `json:"checksum,omitempty"`
	LastModified string                   `json:"lastModified,omitempty"`
	Entries      map[string]ManifestEntry `json:"entries"`
}

func (m SyncManifest) EntryCount() int {
	return len(m.Entries)
}

type RemoteFetchResult struct {
	Success      bool
	Empty        bool
	NotModified  bool
	Manifest     SyncManifest
	Entries      map[string]string
	Checksum     string
	LastModified string
	Error        string
	SkipRetry    bool
}

func (r RemoteFetchResult) String() string {
	return fmt.Sprintf(
		"settingssync.RemoteFetchResult{success:%t empty:%t notModified:%t entries:%d checksum:%q error:%q skipRetry:%t}",
		r.Success,
		r.Empty,
		r.NotModified,
		len(r.Entries),
		r.Checksum,
		r.Error,
		r.SkipRetry,
	)
}

type EntryChange struct {
	Key       string
	OldValue  string
	NewValue  string
	Source    Source
	Scope     Scope
	Sensitive bool
}

func (c EntryChange) String() string {
	sensitive := c.Sensitive || IsSensitiveKey(c.Key) || LooksSensitiveValue(c.OldValue) || LooksSensitiveValue(c.NewValue)
	return fmt.Sprintf(
		"settingssync.EntryChange{key:%q source:%q scope:%q old:%q new:%q}",
		c.Key,
		c.Source,
		c.Scope,
		RedactValue(c.Key, c.OldValue, sensitive),
		RedactValue(c.Key, c.NewValue, sensitive),
	)
}

func sortedEntryKeys(entries map[string]Entry) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(entries map[string]string) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
