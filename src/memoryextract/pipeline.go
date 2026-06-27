package memoryextract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type PipelineOptions struct {
	Extractor          Extractor
	Sink               Sink
	SecretScanner      SecretScanner
	MemoryPathPrefixes []string
	WriteToolNames     []string
}

type Pipeline struct {
	extractor          Extractor
	sink               Sink
	secretScanner      SecretScanner
	memoryPathPrefixes []string
	writeToolNames     map[string]struct{}
}

func NewPipeline(options PipelineOptions) *Pipeline {
	extractor := options.Extractor
	if extractor == nil {
		extractor = HeuristicExtractor{}
	}

	scanner := options.SecretScanner
	if scanner == nil {
		scanner = NewDefaultSecretScanner()
	}

	writeToolNames := map[string]struct{}{}
	names := options.WriteToolNames
	if len(names) == 0 {
		names = []string{"write", "edit", "file_write", "file_edit", "apply_patch"}
	}
	for _, name := range names {
		writeToolNames[strings.ToLower(name)] = struct{}{}
	}

	return &Pipeline{
		extractor:          extractor,
		sink:               options.Sink,
		secretScanner:      scanner,
		memoryPathPrefixes: append([]string(nil), options.MemoryPathPrefixes...),
		writeToolNames:     writeToolNames,
	}
}

func (p *Pipeline) Run(ctx context.Context, messages []TranscriptMessage, state CursorState) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	result := Result{State: cloneState(state)}
	since, cursorFound := MessagesSinceCursor(messages, state.LastMessageID)
	visible := modelVisibleMessages(since)
	result.CursorFound = cursorFound
	result.VisibleMessageCount = len(visible)

	if p.hasMemoryWrites(since) {
		result.SkippedReason = SkipManualMemoryWrite
		result.State = advanceCursor(state, messages)
		return result, nil
	}

	if len(visible) == 0 {
		result.SkippedReason = SkipNoVisibleMessages
		result.State = advanceCursor(state, messages)
		return result, nil
	}

	candidates, err := p.extractor.Extract(ctx, ExtractionInput{
		Messages: visible,
		State:    cloneState(state),
	})
	if err != nil {
		return result, err
	}

	accepted, filtered := p.filterCandidates(candidates, state)
	result.Candidates = accepted
	result.Filtered = filtered

	if p.sink != nil && len(accepted) > 0 {
		receipts, err := p.sink.WriteMemories(ctx, accepted)
		if err != nil {
			return result, err
		}
		result.Receipts = receipts
	}

	next := advanceCursor(state, messages)
	for _, candidate := range accepted {
		next.Fingerprints = appendUnique(next.Fingerprints, FingerprintCandidate(candidate))
	}
	result.State = next
	return result, nil
}

func MessagesSinceCursor(messages []TranscriptMessage, cursorID string) ([]TranscriptMessage, bool) {
	if cursorID == "" {
		return append([]TranscriptMessage(nil), messages...), true
	}
	for i, message := range messages {
		if message.ID == cursorID {
			return append([]TranscriptMessage(nil), messages[i+1:]...), true
		}
	}
	return append([]TranscriptMessage(nil), messages...), false
}

func CountModelVisibleMessagesSince(messages []TranscriptMessage, cursorID string) (int, bool) {
	since, found := MessagesSinceCursor(messages, cursorID)
	return len(modelVisibleMessages(since)), found
}

func (p *Pipeline) hasMemoryWrites(messages []TranscriptMessage) bool {
	for _, message := range messages {
		if message.Role != RoleAssistant {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.WritesMemory {
				return true
			}
			if _, ok := p.writeToolNames[strings.ToLower(call.Name)]; !ok {
				continue
			}
			if p.pathIsMemory(call.FilePath) {
				return true
			}
		}
	}
	return false
}

func (p *Pipeline) pathIsMemory(path string) bool {
	if path == "" {
		return false
	}
	for _, prefix := range p.memoryPathPrefixes {
		if prefix != "" && strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (p *Pipeline) filterCandidates(candidates []CandidateMemory, state CursorState) ([]CandidateMemory, []FilteredCandidate) {
	seen := map[string]struct{}{}
	for _, fingerprint := range state.Fingerprints {
		seen[fingerprint] = struct{}{}
	}

	accepted := []CandidateMemory{}
	filtered := []FilteredCandidate{}
	for _, candidate := range candidates {
		candidate.Text = strings.TrimSpace(candidate.Text)
		if candidate.Text == "" {
			filtered = append(filtered, FilteredCandidate{
				Candidate: candidate,
				Reason:    "empty",
			})
			continue
		}

		if p.secretScanner != nil {
			matches := p.secretScanner.Scan(candidate.Text)
			if len(matches) > 0 {
				filtered = append(filtered, FilteredCandidate{
					Candidate:     candidate,
					Reason:        "secret",
					SecretMatches: matches,
				})
				continue
			}
		}

		fingerprint := FingerprintCandidate(candidate)
		if _, ok := seen[fingerprint]; ok {
			filtered = append(filtered, FilteredCandidate{
				Candidate: candidate,
				Reason:    "duplicate",
			})
			continue
		}
		seen[fingerprint] = struct{}{}
		if candidate.ID == "" {
			candidate.ID = "mem_" + fingerprint[:16]
		}
		accepted = append(accepted, candidate)
	}

	return accepted, filtered
}

func FingerprintCandidate(candidate CandidateMemory) string {
	sum := sha256.Sum256([]byte(string(candidate.Category) + "\x00" + normalizeForFingerprint(candidate.Text)))
	return hex.EncodeToString(sum[:])
}

func normalizeForFingerprint(text string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	return strings.Trim(strings.Join(fields, " "), " \t\n\r.!,;:")
}

func modelVisibleMessages(messages []TranscriptMessage) []TranscriptMessage {
	visible := make([]TranscriptMessage, 0, len(messages))
	for _, message := range messages {
		if message.ModelVisible() {
			visible = append(visible, message)
		}
	}
	return visible
}

func advanceCursor(state CursorState, messages []TranscriptMessage) CursorState {
	next := cloneState(state)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ID != "" {
			next.LastMessageID = messages[i].ID
			return next
		}
	}
	return next
}

func cloneState(state CursorState) CursorState {
	return CursorState{
		LastMessageID: state.LastMessageID,
		Fingerprints:  append([]string(nil), state.Fingerprints...),
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
