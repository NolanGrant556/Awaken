package llm

import "strings"

// ReasoningOffKind is the host decision for a DisableReasoning intent.
// Adapters own the wire spelling; this type never names a provider field.
type ReasoningOffKind string

const (
	// ReasoningOffNone means the request is not asking to turn reasoning off.
	ReasoningOffNone ReasoningOffKind = ""
	// ReasoningOffWire means an exact provider/base/protocol/model contract
	// publishes a disable field. The adapter must send that field.
	ReasoningOffWire ReasoningOffKind = "wire"
	// ReasoningOffProtocolOmit means the protocol's documented off signal is
	// the absence of a thinking block, not a vendor private field.
	ReasoningOffProtocolOmit ReasoningOffKind = "protocol_omit"
	// ReasoningOffLowestEffort means off is unpublished on this route, but
	// the exact model already has a confirmed lowest effort mapping. Omit
	// is not treated as off.
	ReasoningOffLowestEffort ReasoningOffKind = "lowest_effort"
	// ReasoningOffUnsupported means off is unpublished and no confirmed
	// fallback exists. Adapters must not invent a disable field.
	ReasoningOffUnsupported ReasoningOffKind = "unsupported"
)

// ReasoningOffDecision is the resolved action for one prepared request.
type ReasoningOffDecision struct {
	Kind       ReasoningOffKind
	Capability ReasoningControlCapability
}

// ResolveReasoningOffDecision maps a logical off/auxiliary intent onto one
// published action. It does not serialize wire fields.
func ResolveReasoningOffDecision(cfg Config, req Request) ReasoningOffDecision {
	protocol := resolveProtocol(cfg)
	model := strings.ToLower(resolveModel(cfg, req))
	if isKimiK27Code(model) {
		if req.DisableReasoning || isAuxiliaryReasoningBoundedPurpose(req.Purpose) {
			return ReasoningOffDecision{Kind: ReasoningOffUnsupported}
		}
		return ReasoningOffDecision{}
	}
	if req.DisableReasoning {
		if capability, ok := ResolveReasoningControlCapability(cfg, req); ok {
			return ReasoningOffDecision{Kind: ReasoningOffWire, Capability: capability}
		}
	}
	if isKimiEffortModel(cfg.ProviderID, model) && isKimiAuxiliaryPurpose(req.Purpose) {
		return ReasoningOffDecision{Kind: ReasoningOffLowestEffort}
	}
	if protocol == ProtocolAnthropicMessages {
		if req.DisableReasoning {
			return ReasoningOffDecision{Kind: ReasoningOffProtocolOmit}
		}
		return ReasoningOffDecision{}
	}
	if isAuxiliaryReasoningBoundedPurpose(req.Purpose) &&
		(protocol == ProtocolOpenAIChat || protocol == ProtocolOpenAIResponses) &&
		supportsLowestEffortReasoningFallback(cfg, req) {
		return ReasoningOffDecision{Kind: ReasoningOffLowestEffort}
	}
	if req.DisableReasoning {
		return ReasoningOffDecision{Kind: ReasoningOffUnsupported}
	}
	return ReasoningOffDecision{}
}

func isAuxiliaryReasoningBoundedPurpose(purpose RequestPurpose) bool {
	return purpose == RequestPurposeThreadTitle || purpose == RequestPurposeCompactSummary
}

func supportsLowestEffortReasoningFallback(cfg Config, req Request) bool {
	provider := strings.ToLower(strings.TrimSpace(cfg.ProviderID))
	model := capabilityModelID(cfg, req)
	switch {
	case provider == "kimi-cn" && model == "kimi-k3":
		return true
	case provider == "kimi-coding-plan" && (model == "k3" || model == "k3-256k" || model == "kimi-for-coding"):
		return true
	case provider == "doubao" && (isDoubaoSeedReasoningEffortModel(model) || isDoubaoHostedDeepSeekGAModel(model)):
		return true
	default:
		return false
	}
}
