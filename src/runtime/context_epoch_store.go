package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	"awaken/src/filestore"
	"awaken/src/tool"
)

type ContextEpochFileStore struct {
	mu sync.Mutex
}

func NewContextEpochStore() *ContextEpochFileStore { return &ContextEpochFileStore{} }

type contextEpochPointer struct {
	SchemaVersion int    `json:"schema_version"`
	EpochID       string `json:"epoch_id"`
}

type contextEpochSourceSnapshot struct {
	SourceKey       string                            `json:"source_key"`
	Kind            agentcore.ContextContributionKind `json:"kind"`
	CodecVersion    int                               `json:"codec_version"`
	BaselineDigest  string                            `json:"baseline_digest,omitempty"`
	BaselineValue   string                            `json:"baseline_value,omitempty"`
	ValueDigest     string                            `json:"value_digest,omitempty"`
	Value           string                            `json:"value,omitempty"`
	AppliedSequence uint64                            `json:"applied_sequence"`
	State           agentcore.ContextEpochSourceState `json:"state"`
}

type contextEpochToolSnapshot struct {
	Name       string                         `json:"name"`
	Digest     string                         `json:"digest"`
	Dynamic    bool                           `json:"dynamic"`
	FirstSeen  uint64                         `json:"first_seen_sequence"`
	Schema     tool.ToolSchema                `json:"schema"`
	Provenance agentcore.ToolSchemaProvenance `json:"provenance"`
}

type contextEpochDelta struct {
	Sequence  uint64                            `json:"sequence"`
	SourceKey string                            `json:"source_key"`
	Kind      agentcore.ContextContributionKind `json:"kind"`
	State     agentcore.ContextEpochSourceState `json:"state"`
	Digest    string                            `json:"digest"`
	Content   string                            `json:"content"`
}

type contextEpochSidecar struct {
	SchemaVersion       int                          `json:"schema_version"`
	EpochID             string                       `json:"epoch_id"`
	ThreadID            string                       `json:"thread_id"`
	BaselineSequence    uint64                       `json:"baseline_sequence"`
	ExactBaselineText   string                       `json:"exact_baseline_text"`
	BaselineDigest      string                       `json:"baseline_digest"`
	PromptSchemaVersion string                       `json:"prompt_schema_version"`
	WorkspaceRoot       string                       `json:"workspace_root"`
	ProviderID          string                       `json:"provider_id"`
	ProfileID           string                       `json:"profile_id"`
	CredentialRevision  uint64                       `json:"credential_revision"`
	ModelID             string                       `json:"model_id"`
	ProtocolFamily      string                       `json:"protocol_family"`
	EndpointContractID  string                       `json:"endpoint_contract_id"`
	NormalizedBaseURL   string                       `json:"normalized_base_url"`
	CompactBoundaryID   string                       `json:"compact_boundary_id,omitempty"`
	SourceSnapshots     []contextEpochSourceSnapshot `json:"source_snapshots"`
	Tools               []contextEpochToolSnapshot   `json:"tools"`
	Deltas              []contextEpochDelta          `json:"deltas,omitempty"`
	UpdatedAt           time.Time                    `json:"updated_at"`
}

func (store *ContextEpochFileStore) Project(_ context.Context, candidate agentcore.ContextEpochCandidate) (agentcore.ContextEpochProjection, error) {
	if strings.TrimSpace(candidate.ThreadID) == "" {
		return agentcore.ContextEpochProjection{}, errors.New("context epoch requires thread identity")
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	dir := filepath.Join(activeSessionsDir(), safeTranscriptFileName(candidate.ThreadID), "context", "epochs")
	if err := filestore.EnsureDir(dir); err != nil {
		return agentcore.ContextEpochProjection{}, err
	}
	if err := os.Chmod(dir, filestore.DirPerm); err != nil {
		return agentcore.ContextEpochProjection{}, err
	}

	current, err := readCurrentContextEpoch(dir)
	state := "unchanged"
	reason := ""
	switch {
	case errors.Is(err, os.ErrNotExist):
		current = newContextEpoch(candidate, 1)
		state = "created"
		reason = "initial_baseline"
	case err != nil:
		return agentcore.ContextEpochProjection{}, err
	default:
		if boundary := contextEpochBoundaryReason(current, candidate); boundary != "" {
			current = newContextEpoch(candidate, current.BaselineSequence+1)
			state = "rotated"
			reason = boundary
		} else {
			updated, boundary := updateContextEpoch(&current, candidate)
			if boundary != "" {
				current = newContextEpoch(candidate, current.BaselineSequence+1)
				state = "rotated"
				reason = boundary
			} else if updated {
				state = "updated"
				reason = "append_only_host_delta"
			}
		}
	}

	if state != "unchanged" {
		current.UpdatedAt = time.Now().UTC()
		if err := persistContextEpoch(dir, current); err != nil {
			return agentcore.ContextEpochProjection{}, err
		}
	}
	return projectContextEpoch(current, state, reason), nil
}

func newContextEpoch(candidate agentcore.ContextEpochCandidate, sequence uint64) contextEpochSidecar {
	credentialRevision := candidate.CredentialRevision
	if credentialRevision == 0 {
		credentialRevision = 1
	}
	baselineDigest := agentcore.ContextEpochSourceDigest(candidate.ExactSystemPrompt)
	epochSeed := strings.Join([]string{
		candidate.ThreadID, fmt.Sprint(sequence), baselineDigest, candidate.ProviderID,
		candidate.ProfileID, fmt.Sprint(credentialRevision), candidate.ModelID,
		candidate.ProtocolFamily, candidate.EndpointContractID, candidate.NormalizedBaseURL,
		candidate.CompactBoundaryID,
	}, "\x00")
	epochHash := sha256.Sum256([]byte(epochSeed))
	epoch := contextEpochSidecar{
		SchemaVersion: agentcore.ContextEpochSchemaVersion, EpochID: "epoch_" + hex.EncodeToString(epochHash[:12]),
		ThreadID: candidate.ThreadID, BaselineSequence: sequence, ExactBaselineText: candidate.ExactSystemPrompt,
		BaselineDigest: baselineDigest, PromptSchemaVersion: candidate.PromptSchemaVersion,
		WorkspaceRoot: candidate.WorkspaceRoot, ProviderID: candidate.ProviderID, ProfileID: candidate.ProfileID,
		CredentialRevision: credentialRevision, ModelID: candidate.ModelID, ProtocolFamily: candidate.ProtocolFamily,
		EndpointContractID: candidate.EndpointContractID, NormalizedBaseURL: candidate.NormalizedBaseURL,
		CompactBoundaryID: candidate.CompactBoundaryID,
	}
	for _, source := range canonicalEpochSources(candidate.Sources) {
		state := source.State
		if state == "" || state == agentcore.ContextEpochSourceUpdated {
			state = agentcore.ContextEpochSourceUnchanged
		}
		digest := agentcore.ContextEpochSourceDigest(source.Content)
		epoch.SourceSnapshots = append(epoch.SourceSnapshots, contextEpochSourceSnapshot{
			SourceKey: source.SourceKey, Kind: source.Kind, CodecVersion: 1,
			BaselineDigest: digest, BaselineValue: source.Content, ValueDigest: digest, Value: source.Content,
			AppliedSequence: 0, State: state,
		})
	}
	for index, candidateTool := range agentcore.CanonicalContextEpochTools(candidate.Tools) {
		epoch.Tools = append(epoch.Tools, contextEpochToolSnapshot{
			Name: candidateTool.Schema.Function.Name, Digest: agentcore.ContextEpochToolDigest(candidateTool),
			Dynamic: agentcore.ContextEpochToolIsDynamic(candidateTool), FirstSeen: uint64(index + 1),
			Schema: candidateTool.Schema, Provenance: candidateTool.Provenance,
		})
	}
	return epoch
}

func contextEpochBoundaryReason(current contextEpochSidecar, candidate agentcore.ContextEpochCandidate) string {
	revision := candidate.CredentialRevision
	if revision == 0 {
		revision = 1
	}
	switch {
	case current.SchemaVersion != agentcore.ContextEpochSchemaVersion || current.PromptSchemaVersion != candidate.PromptSchemaVersion:
		return "prompt_schema_incompatible"
	case current.WorkspaceRoot != candidate.WorkspaceRoot:
		return "workspace_moved"
	case current.ProviderID != candidate.ProviderID || current.ProfileID != candidate.ProfileID || current.CredentialRevision != revision:
		return "provider_credential_changed"
	case current.ModelID != candidate.ModelID:
		return "model_changed"
	case current.ProtocolFamily != candidate.ProtocolFamily || current.EndpointContractID != candidate.EndpointContractID || current.NormalizedBaseURL != candidate.NormalizedBaseURL:
		return "provider_authority_incompatible"
	case current.CompactBoundaryID != candidate.CompactBoundaryID:
		return "compact_succeeded"
	default:
		return ""
	}
}

func updateContextEpoch(current *contextEpochSidecar, candidate agentcore.ContextEpochCandidate) (bool, string) {
	if current == nil {
		return false, "missing_epoch"
	}
	candidateSources := make(map[string]agentcore.ContextEpochSourceCandidate, len(candidate.Sources))
	for _, source := range candidate.Sources {
		if key := strings.TrimSpace(source.SourceKey); key != "" {
			candidateSources[key] = source
		}
	}
	updated := false
	for index := range current.SourceSnapshots {
		snapshot := &current.SourceSnapshots[index]
		next, exists := candidateSources[snapshot.SourceKey]
		if !exists {
			next = agentcore.ContextEpochSourceCandidate{SourceKey: snapshot.SourceKey, Kind: snapshot.Kind, State: agentcore.ContextEpochSourceRemoved}
		}
		delete(candidateSources, snapshot.SourceKey)
		if next.State == agentcore.ContextEpochSourceUnavailable {
			if snapshot.State != agentcore.ContextEpochSourceUnavailable {
				snapshot.State = agentcore.ContextEpochSourceUnavailable
				updated = true
			}
			continue
		}
		if snapshot.SourceKey == "system_prompt" && next.State != agentcore.ContextEpochSourceRemoved &&
			agentcore.ContextEpochSourceDigest(next.Content) != snapshot.BaselineDigest {
			return false, "system_prompt_changed"
		}
		if next.State == agentcore.ContextEpochSourceRemoved {
			if snapshot.State == agentcore.ContextEpochSourceRemoved {
				continue
			}
			appendContextEpochDelta(current, snapshot, agentcore.ContextEpochSourceRemoved, "")
			updated = true
			continue
		}
		digest := agentcore.ContextEpochSourceDigest(next.Content)
		if digest == snapshot.ValueDigest && snapshot.State != agentcore.ContextEpochSourceRemoved {
			if snapshot.State != agentcore.ContextEpochSourceUnchanged {
				snapshot.State = agentcore.ContextEpochSourceUnchanged
				updated = true
			}
			continue
		}
		snapshot.Kind = next.Kind
		appendContextEpochDelta(current, snapshot, agentcore.ContextEpochSourceUpdated, next.Content)
		updated = true
	}
	for _, source := range canonicalEpochSourceMap(candidateSources) {
		if source.State == agentcore.ContextEpochSourceUnavailable || source.State == agentcore.ContextEpochSourceRemoved {
			continue
		}
		snapshot := contextEpochSourceSnapshot{
			SourceKey: source.SourceKey, Kind: source.Kind, CodecVersion: 1,
			ValueDigest: agentcore.ContextEpochSourceDigest(source.Content), Value: source.Content,
		}
		current.SourceSnapshots = append(current.SourceSnapshots, snapshot)
		appendContextEpochDelta(current, &current.SourceSnapshots[len(current.SourceSnapshots)-1], agentcore.ContextEpochSourceUpdated, source.Content)
		updated = true
	}

	candidateTools := make(map[string]agentcore.ContextEpochToolCandidate, len(candidate.Tools))
	for _, entry := range candidate.Tools {
		candidateTools[entry.Schema.Function.Name] = entry
	}
	for _, existing := range current.Tools {
		next, exists := candidateTools[existing.Name]
		if !exists {
			return false, "tool_removed"
		}
		delete(candidateTools, existing.Name)
		if agentcore.ContextEpochToolDigest(next) != existing.Digest {
			return false, "tool_schema_changed"
		}
	}
	newTools := make([]agentcore.ContextEpochToolCandidate, 0, len(candidateTools))
	for _, entry := range candidateTools {
		if !agentcore.ContextEpochToolIsDynamic(entry) {
			return false, "canonical_tool_added"
		}
		newTools = append(newTools, entry)
	}
	for _, entry := range agentcore.CanonicalContextEpochTools(newTools) {
		current.Tools = append(current.Tools, contextEpochToolSnapshot{
			Name: entry.Schema.Function.Name, Digest: agentcore.ContextEpochToolDigest(entry), Dynamic: true,
			FirstSeen: uint64(len(current.Tools) + 1), Schema: entry.Schema, Provenance: entry.Provenance,
		})
		updated = true
	}
	return updated, ""
}

func appendContextEpochDelta(epoch *contextEpochSidecar, snapshot *contextEpochSourceSnapshot, state agentcore.ContextEpochSourceState, value string) {
	sequence := uint64(len(epoch.Deltas) + 1)
	digest := agentcore.ContextEpochSourceDigest(value)
	deltaText := fmt.Sprintf("\n\n### Awaken host context update\nsource=%s\nstate=%s\nsequence=%d", snapshot.SourceKey, state, sequence)
	if state == agentcore.ContextEpochSourceRemoved {
		deltaText += "\nThis previously admitted host context source is removed."
		value = ""
	} else {
		deltaText += "\n\n" + value
	}
	epoch.Deltas = append(epoch.Deltas, contextEpochDelta{
		Sequence: sequence, SourceKey: snapshot.SourceKey, Kind: snapshot.Kind, State: state,
		Digest: digest, Content: deltaText,
	})
	snapshot.Value = value
	snapshot.ValueDigest = digest
	snapshot.AppliedSequence = sequence
	snapshot.State = state
}

func projectContextEpoch(epoch contextEpochSidecar, state, reason string) agentcore.ContextEpochProjection {
	projection := agentcore.ContextEpochProjection{
		SystemPrompt: epoch.ExactBaselineText,
		Receipt: agentcore.ContextEpochReceipt{
			EpochID: epoch.EpochID, BaselineDigest: epoch.BaselineDigest, State: state,
			BoundaryReason: reason, BaselineSequence: epoch.BaselineSequence,
		},
	}
	for _, snapshot := range epoch.SourceSnapshots {
		if snapshot.BaselineDigest == "" || snapshot.BaselineValue == "" {
			continue
		}
		projection.PromptSections = append(projection.PromptSections, agentcore.PromptSection{
			ID: snapshot.SourceKey, Kind: snapshot.Kind, Content: snapshot.BaselineValue,
		})
	}
	for _, delta := range epoch.Deltas {
		projection.SystemPrompt += delta.Content
		projection.PromptSections = append(projection.PromptSections, agentcore.PromptSection{
			ID: fmt.Sprintf("epoch_delta_%06d_%s", delta.Sequence, delta.SourceKey), Kind: delta.Kind, Content: delta.Content,
		})
	}
	for _, entry := range epoch.Tools {
		projection.Tools = append(projection.Tools, entry.Schema)
		projection.AttributedTools = append(projection.AttributedTools, agentcore.AttributedToolSchema{Schema: entry.Schema, Provenance: entry.Provenance})
	}
	return projection
}

func canonicalEpochSources(values []agentcore.ContextEpochSourceCandidate) []agentcore.ContextEpochSourceCandidate {
	result := append([]agentcore.ContextEpochSourceCandidate(nil), values...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].SourceKey < result[j].SourceKey })
	return result
}

func canonicalEpochSourceMap(values map[string]agentcore.ContextEpochSourceCandidate) []agentcore.ContextEpochSourceCandidate {
	result := make([]agentcore.ContextEpochSourceCandidate, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return canonicalEpochSources(result)
}

func persistContextEpoch(dir string, epoch contextEpochSidecar) error {
	encoded, err := json.MarshalIndent(epoch, "", "  ")
	if err != nil {
		return err
	}
	epochPath := filepath.Join(dir, epoch.EpochID+".json")
	if err := filestore.WriteFileAtomic(epochPath, append(encoded, '\n')); err != nil {
		return err
	}
	pointer, _ := json.MarshalIndent(contextEpochPointer{SchemaVersion: agentcore.ContextEpochSchemaVersion, EpochID: epoch.EpochID}, "", "  ")
	return filestore.WriteFileAtomic(filepath.Join(dir, "current.json"), append(pointer, '\n'))
}

func readCurrentContextEpoch(dir string) (contextEpochSidecar, error) {
	var pointer contextEpochPointer
	if err := filestore.ReadJSON(filepath.Join(dir, "current.json"), &pointer); err != nil {
		return contextEpochSidecar{}, err
	}
	if pointer.SchemaVersion != agentcore.ContextEpochSchemaVersion || strings.TrimSpace(pointer.EpochID) == "" || strings.Contains(pointer.EpochID, string(filepath.Separator)) || strings.Contains(pointer.EpochID, "..") {
		return contextEpochSidecar{}, errors.New("invalid context epoch pointer")
	}
	var epoch contextEpochSidecar
	if err := filestore.ReadJSON(filepath.Join(dir, pointer.EpochID+".json"), &epoch); err != nil {
		return contextEpochSidecar{}, err
	}
	if epoch.EpochID != pointer.EpochID || epoch.SchemaVersion != agentcore.ContextEpochSchemaVersion {
		return contextEpochSidecar{}, errors.New("context epoch sidecar identity mismatch")
	}
	return epoch, nil
}
