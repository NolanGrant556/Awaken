package tool

import (
	"strings"
	"time"
)

func mergeResourceChanges(groups ...[]ResourceChange) []ResourceChange {
	var result []ResourceChange
	seen := map[string]int{}
	for _, group := range groups {
		for _, change := range group {
			key := strings.Join([]string{strings.TrimSpace(change.Kind), strings.TrimSpace(change.Action), strings.TrimSpace(change.Resource)}, "\x00")
			if key == "\x00\x00" {
				continue
			}
			if index, ok := seen[key]; ok {
				if result[index].Summary == "" {
					result[index].Summary = change.Summary
				}
				if result[index].Certainty == "" || result[index].Certainty == ChangeUnknown {
					result[index].Certainty = change.Certainty
				}
				if result[index].Reversibility == "" || result[index].Reversibility == ChangeReversibilityUnknown {
					result[index].Reversibility = change.Reversibility
				}
				if len(change.Details) > 0 {
					if result[index].Details == nil {
						result[index].Details = map[string]any{}
					}
					for detailKey, detailValue := range change.Details {
						if _, exists := result[index].Details[detailKey]; !exists {
							result[index].Details[detailKey] = detailValue
						}
					}
				}
				continue
			}
			seen[key] = len(result)
			result = append(result, change)
		}
	}
	return result
}

func normalizeChanges(changes []ResourceChange) []ResourceChange {
	changes = mergeResourceChanges(changes)
	for i := range changes {
		if changes[i].Certainty == "" {
			changes[i].Certainty = ChangeUnknown
		}
		if changes[i].Reversibility == "" {
			changes[i].Reversibility = ChangeReversibilityUnknown
		}
	}
	return changes
}

func hasChangeResource(changes []ResourceChange, resource string) bool {
	resource = strings.TrimSpace(resource)
	for _, change := range changes {
		if strings.TrimSpace(change.Resource) == resource {
			return true
		}
		for _, key := range []string{"source_path", "destination_path", "original_path", "trashed_path"} {
			if value, ok := change.Details[key].(string); ok && strings.TrimSpace(value) == resource {
				return true
			}
		}
	}
	return false
}

func mergeArtifactDescriptors(groups ...[]ArtifactDescriptor) []ArtifactDescriptor {
	var result []ArtifactDescriptor
	seen := map[string]bool{}
	for _, group := range groups {
		for _, artifact := range group {
			key := firstNonBlank(strings.TrimSpace(artifact.ID), strings.TrimSpace(artifact.Path)+"\x00"+strings.TrimSpace(string(artifact.Action)))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, artifact)
		}
	}
	return result
}

func mergeNotices(groups ...[]OutcomeNotice) []OutcomeNotice {
	var result []OutcomeNotice
	seen := map[string]bool{}
	for _, group := range groups {
		for _, notice := range group {
			key := strings.TrimSpace(notice.Code) + "\x00" + strings.TrimSpace(notice.Message)
			if key == "\x00" || seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, notice)
		}
	}
	return result
}

func normalizeNextActions(actions []NextAction) []NextAction {
	var result []NextAction
	seen := map[string]bool{}
	for _, action := range actions {
		key := firstNonBlank(strings.TrimSpace(action.ID), strings.TrimSpace(action.Description))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		action.Preconditions = uniqueStrings(action.Preconditions)
		result = append(result, action)
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Now().UTC()
}
