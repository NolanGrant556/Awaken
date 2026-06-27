package lspdiagnostics

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultMaxDiagnosticsPerFile          = 10
	DefaultMaxTotalDiagnostics            = 30
	DefaultMaxDeliveredFiles              = 500
	DefaultMaxDeliveredDiagnosticsPerFile = 200
)

type Clock interface {
	Now() time.Time
}

type ClockFunc func() time.Time

func (fn ClockFunc) Now() time.Time {
	return fn()
}

type IDGenerator func() string

type RegistryOption func(*Registry)

type PendingDiagnostic struct {
	ID         string
	ServerName string
	Files      []DiagnosticFile
	Timestamp  time.Time
}

type DiagnosticBatch struct {
	ServerName  string
	ServerNames []string
	Files       []DiagnosticFile
	Timestamp   time.Time
}

type Registry struct {
	mu sync.Mutex

	clock Clock
	newID IDGenerator

	maxDiagnosticsPerFile int
	maxTotalDiagnostics   int

	pending map[string]PendingDiagnostic
	order   []string

	delivered *deliveredLRU
}

func NewRegistry(options ...RegistryOption) *Registry {
	registry := &Registry{
		clock:                 realClock{},
		newID:                 defaultIDGenerator,
		maxDiagnosticsPerFile: DefaultMaxDiagnosticsPerFile,
		maxTotalDiagnostics:   DefaultMaxTotalDiagnostics,
		pending:               make(map[string]PendingDiagnostic),
		delivered:             newDeliveredLRU(DefaultMaxDeliveredFiles, DefaultMaxDeliveredDiagnosticsPerFile),
	}

	for _, option := range options {
		option(registry)
	}
	registry.normalizeLimits()

	return registry
}

func WithClock(clock Clock) RegistryOption {
	return func(registry *Registry) {
		if clock != nil {
			registry.clock = clock
		}
	}
}

func WithClockFunc(fn func() time.Time) RegistryOption {
	return func(registry *Registry) {
		if fn != nil {
			registry.clock = ClockFunc(fn)
		}
	}
}

func WithIDGenerator(generator IDGenerator) RegistryOption {
	return func(registry *Registry) {
		if generator != nil {
			registry.newID = generator
		}
	}
}

func WithMaxDiagnosticsPerFile(max int) RegistryOption {
	return func(registry *Registry) {
		registry.maxDiagnosticsPerFile = max
	}
}

func WithMaxTotalDiagnostics(max int) RegistryOption {
	return func(registry *Registry) {
		registry.maxTotalDiagnostics = max
	}
}

func WithMaxDeliveredFiles(max int) RegistryOption {
	return func(registry *Registry) {
		registry.delivered.maxFiles = max
	}
}

func WithMaxDeliveredDiagnosticsPerFile(max int) RegistryOption {
	return func(registry *Registry) {
		registry.delivered.maxKeysPerFile = max
	}
}

func (r *Registry) Register(serverName string, params PublishDiagnostics) (string, bool) {
	files := FormatDiagnostics(params)
	if !hasDiagnostics(files) {
		return "", false
	}

	id := r.newID()
	if strings.TrimSpace(id) == "" {
		id = defaultIDGenerator()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.pending[id] = PendingDiagnostic{
		ID:         id,
		ServerName: serverName,
		Files:      cloneDiagnosticFiles(files),
		Timestamp:  r.clock.Now(),
	}
	r.order = append(r.order, id)

	return id, true
}

func (r *Registry) RegisterPayload(serverName string, payload any) (string, bool) {
	params, ok := PublishDiagnosticsFromPayload(payload)
	if !ok {
		return "", false
	}
	return r.Register(serverName, params)
}

func (r *Registry) CheckPending() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.pending) > 0
}

func (r *Registry) DrainPending() []DiagnosticBatch {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.pending) == 0 {
		return nil
	}

	allFiles := make([]DiagnosticFile, 0)
	serverNames := make([]string, 0)
	seenServers := make(map[string]struct{})
	var timestamp time.Time

	for _, id := range r.order {
		pending, ok := r.pending[id]
		if !ok {
			continue
		}
		allFiles = append(allFiles, cloneDiagnosticFiles(pending.Files)...)
		if _, exists := seenServers[pending.ServerName]; !exists {
			seenServers[pending.ServerName] = struct{}{}
			serverNames = append(serverNames, pending.ServerName)
		}
		if timestamp.IsZero() || pending.Timestamp.Before(timestamp) {
			timestamp = pending.Timestamp
		}
		delete(r.pending, id)
	}
	r.order = nil

	files := r.deduplicateAndLimit(allFiles)
	if !hasDiagnostics(files) {
		return nil
	}

	return []DiagnosticBatch{{
		ServerName:  strings.Join(serverNames, ", "),
		ServerNames: serverNames,
		Files:       files,
		Timestamp:   timestamp,
	}}
}

func (r *Registry) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.pending)
}

func (r *Registry) ClearPending() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pending = make(map[string]PendingDiagnostic)
	r.order = nil
}

func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pending = make(map[string]PendingDiagnostic)
	r.order = nil
	r.delivered.Clear()
}

func (r *Registry) ClearDeliveredForFile(uri string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.delivered.Delete(URIToPath(uri))
}

func (r *Registry) normalizeLimits() {
	if r.maxDiagnosticsPerFile <= 0 {
		r.maxDiagnosticsPerFile = DefaultMaxDiagnosticsPerFile
	}
	if r.maxTotalDiagnostics <= 0 {
		r.maxTotalDiagnostics = DefaultMaxTotalDiagnostics
	}
	if r.delivered == nil {
		r.delivered = newDeliveredLRU(DefaultMaxDeliveredFiles, DefaultMaxDeliveredDiagnosticsPerFile)
	}
	if r.delivered.maxFiles <= 0 {
		r.delivered.maxFiles = DefaultMaxDeliveredFiles
	}
	if r.delivered.maxKeysPerFile <= 0 {
		r.delivered.maxKeysPerFile = DefaultMaxDeliveredDiagnosticsPerFile
	}
}

func (r *Registry) deduplicateAndLimit(files []DiagnosticFile) []DiagnosticFile {
	seenByFile := make(map[string]map[string]struct{})
	fileIndex := make(map[string]int)
	deduped := make([]DiagnosticFile, 0, len(files))

	for _, file := range files {
		uri := file.URI
		index, exists := fileIndex[uri]
		if !exists {
			index = len(deduped)
			fileIndex[uri] = index
			seenByFile[uri] = make(map[string]struct{})
			deduped = append(deduped, DiagnosticFile{URI: uri})
		}

		for _, diagnostic := range file.Diagnostics {
			key := diagnosticKey(diagnostic)
			if _, seen := seenByFile[uri][key]; seen {
				continue
			}
			if r.delivered.Has(uri, key) {
				continue
			}
			seenByFile[uri][key] = struct{}{}
			deduped[index].Diagnostics = append(deduped[index].Diagnostics, diagnostic)
		}
	}

	total := 0
	for i := range deduped {
		sort.SliceStable(deduped[i].Diagnostics, func(a, b int) bool {
			return severityRank(deduped[i].Diagnostics[a].Severity) < severityRank(deduped[i].Diagnostics[b].Severity)
		})

		if len(deduped[i].Diagnostics) > r.maxDiagnosticsPerFile {
			deduped[i].Diagnostics = deduped[i].Diagnostics[:r.maxDiagnosticsPerFile]
		}

		remaining := r.maxTotalDiagnostics - total
		if remaining <= 0 {
			deduped[i].Diagnostics = nil
			continue
		}
		if len(deduped[i].Diagnostics) > remaining {
			deduped[i].Diagnostics = deduped[i].Diagnostics[:remaining]
		}
		total += len(deduped[i].Diagnostics)
	}

	limited := make([]DiagnosticFile, 0, len(deduped))
	for _, file := range deduped {
		if len(file.Diagnostics) == 0 {
			continue
		}
		limited = append(limited, file)
		for _, diagnostic := range file.Diagnostics {
			r.delivered.Add(file.URI, diagnosticKey(diagnostic))
		}
	}

	return limited
}

func hasDiagnostics(files []DiagnosticFile) bool {
	for _, file := range files {
		if len(file.Diagnostics) > 0 {
			return true
		}
	}
	return false
}

func cloneDiagnosticFiles(files []DiagnosticFile) []DiagnosticFile {
	cloned := make([]DiagnosticFile, len(files))
	for i, file := range files {
		cloned[i] = DiagnosticFile{
			URI:         file.URI,
			Diagnostics: append([]Diagnostic(nil), file.Diagnostics...),
		}
	}
	return cloned
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 1
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 3
	case SeverityHint:
		return 4
	default:
		return 4
	}
}

func diagnosticKey(diagnostic Diagnostic) string {
	data, err := json.Marshal(struct {
		Message  string   `json:"message"`
		Severity Severity `json:"severity"`
		Range    Range    `json:"range"`
		Source   string   `json:"source"`
		Code     string   `json:"code"`
	}{
		Message:  diagnostic.Message,
		Severity: diagnostic.Severity,
		Range:    diagnostic.Range,
		Source:   diagnostic.Source,
		Code:     diagnostic.Code,
	})
	if err != nil {
		return fmt.Sprintf("%s\x00%s\x00%d:%d-%d:%d\x00%s\x00%s",
			diagnostic.Message,
			diagnostic.Severity,
			diagnostic.Range.Start.Line,
			diagnostic.Range.Start.Character,
			diagnostic.Range.End.Line,
			diagnostic.Range.End.Character,
			diagnostic.Source,
			diagnostic.Code,
		)
	}
	return string(data)
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

var fallbackIDSequence atomic.Uint64

func defaultIDGenerator() string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err == nil {
		return hex.EncodeToString(buffer[:])
	}
	return fmt.Sprintf("lspdiag_%d_%d", time.Now().UnixNano(), fallbackIDSequence.Add(1))
}

type deliveredLRU struct {
	maxFiles       int
	maxKeysPerFile int
	files          map[string]*list.Element
	order          *list.List
}

type deliveredFile struct {
	uri  string
	keys *orderedKeySet
}

func newDeliveredLRU(maxFiles, maxKeysPerFile int) *deliveredLRU {
	return &deliveredLRU{
		maxFiles:       maxFiles,
		maxKeysPerFile: maxKeysPerFile,
		files:          make(map[string]*list.Element),
		order:          list.New(),
	}
}

func (lru *deliveredLRU) Has(uri, key string) bool {
	element, ok := lru.files[uri]
	if !ok {
		return false
	}
	lru.order.MoveToFront(element)
	return element.Value.(*deliveredFile).keys.Has(key)
}

func (lru *deliveredLRU) Add(uri, key string) {
	element, ok := lru.files[uri]
	if !ok {
		entry := &deliveredFile{
			uri:  uri,
			keys: newOrderedKeySet(lru.maxKeysPerFile),
		}
		element = lru.order.PushFront(entry)
		lru.files[uri] = element
		lru.evictFiles()
	} else {
		lru.order.MoveToFront(element)
	}
	element.Value.(*deliveredFile).keys.Add(key)
}

func (lru *deliveredLRU) Delete(uri string) {
	element, ok := lru.files[uri]
	if !ok {
		return
	}
	lru.order.Remove(element)
	delete(lru.files, uri)
}

func (lru *deliveredLRU) Clear() {
	lru.files = make(map[string]*list.Element)
	lru.order.Init()
}

func (lru *deliveredLRU) evictFiles() {
	for len(lru.files) > lru.maxFiles {
		element := lru.order.Back()
		if element == nil {
			return
		}
		entry := element.Value.(*deliveredFile)
		delete(lru.files, entry.uri)
		lru.order.Remove(element)
	}
}

type orderedKeySet struct {
	max   int
	keys  map[string]*list.Element
	order *list.List
}

func newOrderedKeySet(max int) *orderedKeySet {
	return &orderedKeySet{
		max:   max,
		keys:  make(map[string]*list.Element),
		order: list.New(),
	}
}

func (set *orderedKeySet) Has(key string) bool {
	element, ok := set.keys[key]
	if !ok {
		return false
	}
	set.order.MoveToFront(element)
	return true
}

func (set *orderedKeySet) Add(key string) {
	if element, ok := set.keys[key]; ok {
		set.order.MoveToFront(element)
		return
	}
	element := set.order.PushFront(key)
	set.keys[key] = element

	for len(set.keys) > set.max {
		oldest := set.order.Back()
		if oldest == nil {
			return
		}
		delete(set.keys, oldest.Value.(string))
		set.order.Remove(oldest)
	}
}
