package agentcore

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"awaken/src/tool"
)

const ContextEpochSchemaVersion = 1

type ContextEpochSourceState string

const (
	ContextEpochSourceUnchanged    ContextEpochSourceState = "unchanged"
	ContextEpochSourceUpdated      ContextEpochSourceState = "updated"
	ContextEpochSourceUnavailable  ContextEpochSourceState = "unavailable"
	ContextEpochSourceRemoved      ContextEpochSourceState = "removed"
	ContextEpochSourceIncompatible ContextEpochSourceState = "incompatible"
)

type ContextEpochSourceCandidate struct {
	SourceKey string
	Kind      ContextContributionKind
	Content   string
	State     ContextEpochSourceState
}

type ContextEpochToolCandidate struct {
	Schema     tool.ToolSchema
	Provenance ToolSchemaProvenance
}

type ContextEpochCandidate struct {
	ThreadID            string
	WorkspaceRoot       string
	ProviderID          string
	ProfileID           string
	CredentialRevision  uint64
	ModelID             string
	ProtocolFamily      string
	EndpointContractID  string
	NormalizedBaseURL   string
	PromptSchemaVersion string
	CompactBoundaryID   string
	ExactSystemPrompt   string
	Sources             []ContextEpochSourceCandidate
	Tools               []ContextEpochToolCandidate
}

type ContextEpochReceipt struct {
	EpochID                string `json:"epoch_id"`
	BaselineDigest         string `json:"baseline_digest"`
	State                  string `json:"state"`
	BoundaryReason         string `json:"boundary_reason,omitempty"`
	CreationBoundaryReason string `json:"creation_boundary_reason,omitempty"`
	BaselineSequence       uint64 `json:"baseline_sequence"`
}

type ContextEpochProjection struct {
	SystemPrompt    string
	PromptSections  []PromptSection
	Tools           []tool.ToolSchema
	AttributedTools []AttributedToolSchema
	Receipt         ContextEpochReceipt
}

type ContextEpochStore interface {
	Project(stdcontext.Context, ContextEpochCandidate) (ContextEpochProjection, error)
}

func contextEpochCandidate(threadID string, config ModelConfig, req LLMRequest, endpointContractID, normalizedBaseURL string) ContextEpochCandidate {
	candidate := ContextEpochCandidate{
		ThreadID: threadID, WorkspaceRoot: configWorkspaceRoot(req), ProviderID: config.ProviderID,
		ProfileID: config.ProfileID, CredentialRevision: config.CredentialRevision, ModelID: req.Model,
		ProtocolFamily: config.Protocol, EndpointContractID: endpointContractID, NormalizedBaseURL: normalizedBaseURL,
		PromptSchemaVersion: "awaken_prompt_v1", ExactSystemPrompt: req.SystemPrompt,
		Sources: cloneContextEpochSourceCandidates(req.EpochSources),
	}
	if req.ContextWindowState.LastCompactBoundaryID != "" {
		candidate.CompactBoundaryID = req.ContextWindowState.LastCompactBoundaryID
	} else {
		candidate.CompactBoundaryID = lastCompactBoundaryID(req.Messages)
	}
	if len(candidate.Sources) == 0 {
		for _, section := range req.PromptSections {
			candidate.Sources = append(candidate.Sources, ContextEpochSourceCandidate{
				SourceKey: section.ID, Kind: section.Kind, Content: section.Content, State: ContextEpochSourceUpdated,
			})
		}
	}
	for index, schema := range req.Tools {
		provenance := ToolSchemaProvenance{}
		if index < len(req.AttributedTools) {
			provenance = req.AttributedTools[index].Provenance
		}
		candidate.Tools = append(candidate.Tools, ContextEpochToolCandidate{Schema: schema, Provenance: provenance})
	}
	return candidate
}

func configWorkspaceRoot(req LLMRequest) string {
	if req.EnvelopeDebug == nil {
		return ""
	}
	return req.EnvelopeDebug.RuntimeSnapshot.WorkspaceRoot
}

func cloneContextEpochSourceCandidates(values []ContextEpochSourceCandidate) []ContextEpochSourceCandidate {
	if len(values) == 0 {
		return nil
	}
	result := make([]ContextEpochSourceCandidate, len(values))
	copy(result, values)
	return result
}

func CanonicalContextEpochTools(values []ContextEpochToolCandidate) []ContextEpochToolCandidate {
	result := append([]ContextEpochToolCandidate(nil), values...)
	sort.SliceStable(result, func(i, j int) bool {
		left, right := contextEpochToolSortKey(result[i]), contextEpochToolSortKey(result[j])
		return left < right
	})
	return result
}

func contextEpochToolSortKey(value ContextEpochToolCandidate) string {
	rank := map[tool.ToolSource]string{
		tool.SourceBuiltin: "00", tool.SourceMCP: "10", tool.SourceConnector: "20",
		tool.SourceSkill: "30", tool.SourcePlugin: "40", tool.SourceRuntime: "50",
	}[value.Provenance.Origin]
	if rank == "" {
		rank = "90"
	}
	return strings.Join([]string{rank, value.Schema.Function.Name, string(value.Provenance.ProviderID), value.Provenance.RegistrationID}, "\x00")
}

func ContextEpochToolDigest(value ContextEpochToolCandidate) string {
	encoded, _ := json.Marshal(struct {
		Schema     tool.ToolSchema
		Provenance ToolSchemaProvenance
	}{value.Schema, value.Provenance})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func ContextEpochSourceDigest(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func ContextEpochToolIsDynamic(value ContextEpochToolCandidate) bool {
	return value.Provenance.Origin != "" && value.Provenance.Origin != tool.SourceBuiltin
}
