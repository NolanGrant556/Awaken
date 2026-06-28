package providerrouting

import (
	"sort"
	"strings"
)

const (
	ReasonDenied          = "denied"
	ReasonNotAllowed      = "not_allowed"
	ReasonUnavailable     = "unavailable"
	ReasonDisabled        = "disabled"
	ReasonUnknownProvider = "unknown_provider"
)

type ProviderStatus string

const (
	ProviderStatusAvailable   ProviderStatus = "available"
	ProviderStatusUnavailable ProviderStatus = "unavailable"
	ProviderStatusDisabled    ProviderStatus = "disabled"
)

type Provider struct {
	Name   string
	Status ProviderStatus
}

type Preferences struct {
	AllowList          []string
	DenyList           []string
	PriorityOrder      []string
	RequireOnlyAllowed bool
}

type Plan struct {
	Providers []Provider
	Included  []ProviderDecision
	Excluded  []ProviderDecision
}

type ProviderDecision struct {
	Provider Provider
	Included bool
	Reasons  []string
}

func NormalizePreferences(prefs Preferences) Preferences {
	return Preferences{
		AllowList:          normalizeNameList(prefs.AllowList),
		DenyList:           normalizeNameList(prefs.DenyList),
		PriorityOrder:      normalizeNameList(prefs.PriorityOrder),
		RequireOnlyAllowed: prefs.RequireOnlyAllowed,
	}
}

func (prefs Preferences) Plan(providers []Provider) Plan {
	return PlanProviders(providers, prefs)
}

func PlanProviders(providers []Provider, prefs Preferences) Plan {
	normalized := NormalizePreferences(prefs)
	allowSet := nameSet(normalized.AllowList)
	denySet := nameSet(normalized.DenyList)
	priority := priorityMap(normalized.PriorityOrder)

	included := make([]plannedProvider, 0, len(providers))
	plan := Plan{
		Excluded: make([]ProviderDecision, 0),
	}

	for index, provider := range providers {
		key := normalizeName(provider.Name)
		reasons := exclusionReasons(provider, key, allowSet, denySet, normalized.RequireOnlyAllowed)
		decision := ProviderDecision{
			Provider: provider,
			Included: len(reasons) == 0,
			Reasons:  append([]string(nil), reasons...),
		}
		if decision.Included {
			included = append(included, plannedProvider{
				decision:      decision,
				key:           key,
				originalIndex: index,
			})
			continue
		}
		plan.Excluded = append(plan.Excluded, copyDecision(decision))
	}

	sort.SliceStable(included, func(i, j int) bool {
		leftRank, leftPriority := priority[included[i].key]
		rightRank, rightPriority := priority[included[j].key]
		if leftPriority && rightPriority {
			return leftRank < rightRank
		}
		if leftPriority != rightPriority {
			return leftPriority
		}
		return included[i].originalIndex < included[j].originalIndex
	})

	plan.Providers = make([]Provider, 0, len(included))
	plan.Included = make([]ProviderDecision, 0, len(included))
	for _, item := range included {
		plan.Providers = append(plan.Providers, item.decision.Provider)
		plan.Included = append(plan.Included, copyDecision(item.decision))
	}

	return plan
}

type plannedProvider struct {
	decision      ProviderDecision
	key           string
	originalIndex int
}

func exclusionReasons(provider Provider, key string, allowSet, denySet map[string]struct{}, requireOnlyAllowed bool) []string {
	var reasons []string
	if key == "" {
		return []string{ReasonUnknownProvider}
	}
	reasons = append(reasons, providerStateReasons(provider.Status)...)
	if _, denied := denySet[key]; denied {
		reasons = append(reasons, ReasonDenied)
	}
	if requireOnlyAllowed {
		if _, allowed := allowSet[key]; !allowed {
			reasons = append(reasons, ReasonNotAllowed)
		}
	}
	return reasons
}

func providerStateReasons(status ProviderStatus) []string {
	switch status {
	case ProviderStatusUnavailable:
		return []string{ReasonUnavailable}
	case ProviderStatusDisabled:
		return []string{ReasonDisabled}
	default:
		return nil
	}
}

func priorityMap(names []string) map[string]int {
	priority := make(map[string]int, len(names))
	for index, name := range names {
		if _, exists := priority[name]; exists {
			continue
		}
		priority[name] = index
	}
	return priority
}

func nameSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

func normalizeNameList(names []string) []string {
	normalized := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = normalizeName(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func copyDecision(decision ProviderDecision) ProviderDecision {
	decision.Reasons = append([]string(nil), decision.Reasons...)
	return decision
}
