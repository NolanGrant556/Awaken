package sidequestion

import (
	"errors"
	"strings"
)

func BuildProviderRoute(candidates []ProviderCandidate, preferred string, auto bool) ProviderRoute {
	return BuildProviderRouteForTask(candidates, preferred, auto, TaskTypeText)
}

func BuildProviderRouteForTask(candidates []ProviderCandidate, preferred string, auto bool, taskType TaskType) ProviderRoute {
	taskType = normalizeTaskType(taskType)

	route := ProviderRoute{
		TaskType:     taskType,
		Preferred:    strings.TrimSpace(preferred),
		Auto:         auto,
		CurrentIndex: 0,
	}

	seen := map[string]bool{}
	preferredID := normalizeProviderID(preferred)
	if preferredID != "" {
		for _, candidate := range candidates {
			if providerMatches(candidate, preferredID) && candidateUsableForTask(candidate, taskType) {
				route.Candidates = append(route.Candidates, candidate)
				seen[providerKey(candidate)] = true
				break
			}
		}
		if !auto {
			return route
		}
	}

	if auto {
		for _, candidate := range candidates {
			if !candidateUsableForTask(candidate, taskType) {
				continue
			}
			key := providerKey(candidate)
			if seen[key] {
				continue
			}
			route.Candidates = append(route.Candidates, candidate)
			seen[key] = true
		}
	}

	return route
}

func (r ProviderRoute) Current() (ProviderCandidate, bool) {
	if r.CurrentIndex < 0 || r.CurrentIndex >= len(r.Candidates) {
		return ProviderCandidate{}, false
	}
	return r.Candidates[r.CurrentIndex], true
}

func (r *ProviderRoute) AdvanceOnError(err error) (ProviderCandidate, bool) {
	current, hasCurrent := r.Current()
	class := ClassifyProviderError(err)
	if hasCurrent {
		r.FailureRecord = append(r.FailureRecord, ProviderFailure{
			Provider: providerID(current),
			Model:    current.Model,
			Class:    class,
			Message:  errorString(err),
		})
	}

	if !r.Auto || !providerErrorAllowsFallback(class) {
		return ProviderCandidate{}, false
	}

	r.CurrentIndex++
	return r.Current()
}

func (r *ProviderRoute) AdvanceOnFailure(err error) (ProviderCandidate, bool) {
	return r.AdvanceOnError(err)
}

func ClassifyProviderError(err error) ProviderErrorClass {
	if err == nil {
		return ProviderErrorNone
	}

	var providerErr ProviderError
	if errors.As(err, &providerErr) {
		if class := classifyStructuredProviderError(providerErr); class != ProviderErrorFatal {
			return class
		}
	}

	var providerErrPtr *ProviderError
	if errors.As(err, &providerErrPtr) && providerErrPtr != nil {
		if class := classifyStructuredProviderError(*providerErrPtr); class != ProviderErrorFatal {
			return class
		}
	}

	message := strings.ToLower(err.Error())
	if containsAny(message, "402", "credit", "credits", "insufficient funds", "can only afford", "billing", "payment required", "quota exceeded", "quota_exceeded") {
		return ProviderErrorCreditExhausted
	}
	if containsAny(message, "429", "rate limit", "rate_limit", "rate-limited", "too many requests") {
		return ProviderErrorRateLimited
	}
	if containsAny(message, "connection refused", "connection reset", "no route to host", "network is unreachable", "timed out", "timeout", "dns", "tls", "ssl") {
		return ProviderErrorConnection
	}
	if containsAny(message, "401", "403", "unauthorized", "authentication", "permission denied") {
		return ProviderErrorAuth
	}
	return ProviderErrorFatal
}

func classifyStructuredProviderError(providerErr ProviderError) ProviderErrorClass {
	if providerErr.Class != "" && providerErr.Class != ProviderErrorFatal {
		return providerErr.Class
	}
	switch providerErr.StatusCode {
	case 401, 403:
		return ProviderErrorAuth
	case 402:
		return ProviderErrorCreditExhausted
	case 408:
		return ProviderErrorConnection
	case 429:
		return ProviderErrorRateLimited
	default:
		return ProviderErrorFatal
	}
}

func providerErrorAllowsFallback(class ProviderErrorClass) bool {
	switch class {
	case ProviderErrorCreditExhausted, ProviderErrorRateLimited, ProviderErrorConnection:
		return true
	default:
		return false
	}
}

func normalizeTaskType(taskType TaskType) TaskType {
	switch taskType {
	case TaskTypeVision:
		return TaskTypeVision
	default:
		return TaskTypeText
	}
}

func candidateUsableForTask(candidate ProviderCandidate, taskType TaskType) bool {
	if !candidate.Available {
		return false
	}
	switch taskType {
	case TaskTypeVision:
		return candidate.SupportsVision
	default:
		return candidate.SupportsText || (!candidate.SupportsText && !candidate.SupportsVision)
	}
}

func providerMatches(candidate ProviderCandidate, normalizedPreferred string) bool {
	if normalizedPreferred == "" {
		return false
	}
	return normalizeProviderID(candidate.Provider) == normalizedPreferred ||
		normalizeProviderID(candidate.Name) == normalizedPreferred
}

func providerKey(candidate ProviderCandidate) string {
	key := providerID(candidate) + "\x00" + strings.TrimSpace(candidate.Model)
	if key == "\x00" {
		return candidate.Name
	}
	return key
}

func providerID(candidate ProviderCandidate) string {
	if strings.TrimSpace(candidate.Provider) != "" {
		return strings.TrimSpace(candidate.Provider)
	}
	return strings.TrimSpace(candidate.Name)
}

func normalizeProviderID(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
