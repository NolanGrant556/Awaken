package teammemorysync

import (
	"encoding/json"
	"sort"
)

const (
	DefaultMaxEntryBytes   = 250_000
	DefaultMaxRequestBytes = 200_000
)

type SyncStatus string

const (
	StatusOK             SyncStatus = "ok"
	StatusNotModified    SyncStatus = "not_modified"
	StatusEmpty          SyncStatus = "empty"
	StatusAuth           SyncStatus = "auth"
	StatusParse          SyncStatus = "parse"
	StatusConflict       SyncStatus = "conflict"
	StatusTooManyEntries SyncStatus = "too_many_entries"
	StatusUnknown        SyncStatus = "unknown"
)

type PushOptions struct {
	MaxEntryBytes    int
	MaxRequestBytes  int
	ServerMaxEntries int
}

type SkippedEntry struct {
	Path   string
	Bytes  int
	RuleID string
	Label  string
}

type Batch struct {
	Entries map[string]string
	Bytes   int
}

type PushPlan struct {
	Status                 SyncStatus
	Delta                  map[string]string
	DeltaChecksums         map[string]string
	Batches                []Batch
	SkippedSecrets         []SkippedEntry
	SkippedOversize        []SkippedEntry
	TruncatedEntries       []string
	DeletionsNotPropagated []string
	MaxEntries             int
	ReceivedEntries        int
}

type PullInput struct {
	HTTPStatus     int
	Entries        map[string]string
	EntryChecksums map[string]string
	Checksum       string
	Error          string
	ParseError     bool
}

type PullPlan struct {
	Status         SyncStatus
	HTTPStatus     int
	Entries        map[string]string
	EntryChecksums map[string]string
	Checksum       string
	Error          string
	Empty          bool
	NotModified    bool
}

type TooManyEntries struct {
	MaxEntries      int
	ReceivedEntries int
}

type PushResponseInput struct {
	HTTPStatus     int
	Error          string
	ParseError     bool
	TooManyEntries *TooManyEntries
}

type PushDecision struct {
	Status          SyncStatus
	HTTPStatus      int
	Error           string
	MaxEntries      int
	ReceivedEntries int
}

// PlanPush builds the upload delta and request batches from caller-provided state.
// It never reads files, writes files, or contacts a server.
func PlanPush(localEntries map[string]string, serverChecksums map[string]string, options PushOptions) PushPlan {
	options = normalizeOptions(options)
	plan := PushPlan{
		Status:         StatusOK,
		Delta:          map[string]string{},
		DeltaChecksums: map[string]string{},
		MaxEntries:     options.ServerMaxEntries,
	}

	eligible := map[string]string{}
	for _, path := range sortedKeys(localEntries) {
		content := localEntries[path]
		size := len([]byte(content))
		if size > options.MaxEntryBytes {
			plan.SkippedOversize = append(plan.SkippedOversize, SkippedEntry{
				Path:  path,
				Bytes: size,
			})
			continue
		}

		if matches := ScanSecrets(content); len(matches) > 0 {
			plan.SkippedSecrets = append(plan.SkippedSecrets, SkippedEntry{
				Path:   path,
				Bytes:  size,
				RuleID: matches[0].RuleID,
				Label:  matches[0].Label,
			})
			continue
		}

		eligible[path] = content
	}

	if options.ServerMaxEntries > 0 && len(eligible) > options.ServerMaxEntries {
		keys := sortedKeys(eligible)
		for _, path := range keys[options.ServerMaxEntries:] {
			plan.TruncatedEntries = append(plan.TruncatedEntries, path)
			delete(eligible, path)
		}
	}

	for _, path := range sortedKeys(serverChecksums) {
		if _, ok := localEntries[path]; !ok {
			plan.DeletionsNotPropagated = append(plan.DeletionsNotPropagated, path)
		}
	}

	for _, path := range sortedKeys(eligible) {
		content := eligible[path]
		hash := HashContent(content)
		if serverChecksums[path] == hash {
			continue
		}
		plan.Delta[path] = content
		plan.DeltaChecksums[path] = hash
	}

	if options.ServerMaxEntries > 0 {
		merged := map[string]bool{}
		for path := range serverChecksums {
			merged[path] = true
		}
		for path := range plan.Delta {
			merged[path] = true
		}
		plan.ReceivedEntries = len(merged)
		if plan.ReceivedEntries > options.ServerMaxEntries {
			plan.Status = StatusTooManyEntries
			return plan
		}
	}

	plan.Batches = BatchDeltaByBytes(plan.Delta, options.MaxRequestBytes)
	return plan
}

// BatchDeltaByBytes splits upload entries into deterministic JSON request bodies.
func BatchDeltaByBytes(delta map[string]string, maxRequestBytes int) []Batch {
	if len(delta) == 0 {
		return nil
	}
	if maxRequestBytes <= 0 {
		maxRequestBytes = DefaultMaxRequestBytes
	}

	var batches []Batch
	current := map[string]string{}
	for _, path := range sortedKeys(delta) {
		candidate := cloneEntries(current)
		candidate[path] = delta[path]
		if len(current) > 0 && encodedBatchBytes(candidate) > maxRequestBytes {
			batches = append(batches, Batch{
				Entries: cloneEntries(current),
				Bytes:   encodedBatchBytes(current),
			})
			current = map[string]string{}
		}
		current[path] = delta[path]
	}

	if len(current) > 0 {
		batches = append(batches, Batch{
			Entries: cloneEntries(current),
			Bytes:   encodedBatchBytes(current),
		})
	}
	return batches
}

func PlanPull(input PullInput) PullPlan {
	plan := PullPlan{
		HTTPStatus: input.HTTPStatus,
		Entries:    map[string]string{},
		Checksum:   input.Checksum,
		Error:      input.Error,
	}

	if input.ParseError {
		plan.Status = StatusParse
		return plan
	}

	switch input.HTTPStatus {
	case 200, 0:
		plan.Status = StatusOK
		plan.Entries = cloneEntries(input.Entries)
		plan.EntryChecksums = checksumsFor(input.Entries, input.EntryChecksums)
	case 304:
		plan.Status = StatusNotModified
		plan.NotModified = true
	case 404:
		plan.Status = StatusEmpty
		plan.Empty = true
		plan.EntryChecksums = map[string]string{}
	case 401, 403:
		plan.Status = StatusAuth
	default:
		plan.Status = StatusUnknown
	}

	return plan
}

func DecidePushResponse(input PushResponseInput) PushDecision {
	decision := PushDecision{
		HTTPStatus: input.HTTPStatus,
		Error:      input.Error,
	}

	if input.ParseError {
		decision.Status = StatusParse
		return decision
	}

	switch input.HTTPStatus {
	case 200, 201, 204, 0:
		decision.Status = StatusOK
	case 401, 403:
		decision.Status = StatusAuth
	case 412:
		decision.Status = StatusConflict
	case 413:
		if input.TooManyEntries != nil {
			decision.Status = StatusTooManyEntries
			decision.MaxEntries = input.TooManyEntries.MaxEntries
			decision.ReceivedEntries = input.TooManyEntries.ReceivedEntries
		} else {
			decision.Status = StatusUnknown
		}
	default:
		decision.Status = StatusUnknown
	}

	return decision
}

func normalizeOptions(options PushOptions) PushOptions {
	if options.MaxEntryBytes <= 0 {
		options.MaxEntryBytes = DefaultMaxEntryBytes
	}
	if options.MaxRequestBytes <= 0 {
		options.MaxRequestBytes = DefaultMaxRequestBytes
	}
	return options
}

func checksumsFor(entries map[string]string, provided map[string]string) map[string]string {
	if len(provided) > 0 {
		return cloneEntries(provided)
	}
	checksums := map[string]string{}
	for _, path := range sortedKeys(entries) {
		checksums[path] = HashContent(entries[path])
	}
	return checksums
}

func sortedKeys(entries map[string]string) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneEntries(entries map[string]string) map[string]string {
	if len(entries) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(entries))
	for key, value := range entries {
		clone[key] = value
	}
	return clone
}

type batchRequest struct {
	Entries map[string]string `json:"entries"`
}

func encodedBatchBytes(entries map[string]string) int {
	body, err := json.Marshal(batchRequest{Entries: entries})
	if err != nil {
		return 0
	}
	return len(body)
}
