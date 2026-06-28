package modeldisplay

import "strings"

const oneMContextSuffix = "[1m]"

type publicModelFamily struct {
	token       string
	publicName  string
	displayName string
	aliases     []string
}

var publicModelFamilies = []publicModelFamily{
	{token: "opus-4-6", publicName: "claude-opus-4-6", displayName: "Opus 4.6"},
	{token: "opus-4-5", publicName: "claude-opus-4-5", displayName: "Opus 4.5"},
	{token: "opus-4-1", publicName: "claude-opus-4-1", displayName: "Opus 4.1"},
	{token: "opus-4", publicName: "claude-opus-4", displayName: "Opus 4"},
	{token: "sonnet-4-6", publicName: "claude-sonnet-4-6", displayName: "Sonnet 4.6"},
	{token: "sonnet-4-5", publicName: "claude-sonnet-4-5", displayName: "Sonnet 4.5"},
	{token: "sonnet-4", publicName: "claude-sonnet-4", displayName: "Sonnet 4"},
	{token: "sonnet-3-7", publicName: "claude-sonnet-3-7", displayName: "Sonnet 3.7", aliases: []string{"claude-3-7-sonnet"}},
	{token: "haiku-4-5", publicName: "claude-haiku-4-5", displayName: "Haiku 4.5"},
	{token: "haiku-3-5", publicName: "claude-haiku-3-5", displayName: "Haiku 3.5", aliases: []string{"claude-3-5-haiku"}},
}

// SanitizeModelName maps known public Claude families to public IDs and hides unknown names.
func SanitizeModelName(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	for _, family := range publicModelFamilies {
		if strings.Contains(normalized, family.token) {
			return family.publicName
		}
	}
	return "claude"
}

// SanitizeSurfaceKey sanitizes only the model segment after the last slash.
func SanitizeSurfaceKey(surfaceKey string) string {
	slashIndex := strings.LastIndex(surfaceKey, "/")
	if slashIndex == -1 {
		return surfaceKey
	}

	surface := surfaceKey[:slashIndex]
	model := surfaceKey[slashIndex+1:]
	return surface + "/" + SanitizeModelName(model)
}

// PublicModelDisplayName returns a display name for known public model IDs.
func PublicModelDisplayName(model string) (string, bool) {
	baseName, hasOneMContext := splitOneMContextSuffix(model)
	family, ok := publicFamilyForID(baseName)
	if !ok {
		return "", false
	}
	if hasOneMContext {
		return family.displayName + " (1M context)", true
	}
	return family.displayName, true
}

// RenderModelName returns a public display name when known, otherwise a masked internal name.
func RenderModelName(model string) string {
	if publicName, ok := PublicModelDisplayName(model); ok {
		return publicName
	}
	return MaskModelCodename(model)
}

// MaskModelCodename masks only the first dash-separated segment and preserves a [1m] suffix.
func MaskModelCodename(model string) string {
	baseName, hasOneMContext := splitOneMContextSuffix(model)
	if baseName == "" {
		if hasOneMContext {
			return oneMContextSuffix
		}
		return ""
	}

	parts := strings.Split(baseName, "-")
	codename := parts[0]
	if len(codename) > 3 {
		parts[0] = codename[:3] + strings.Repeat("*", len(codename)-3)
	}

	masked := strings.Join(parts, "-")
	if hasOneMContext {
		return masked + oneMContextSuffix
	}
	return masked
}

// PublicAuthorName returns a public author label without assigning official names to unknown models.
func PublicAuthorName(model string) string {
	if publicName, ok := PublicModelDisplayName(model); ok {
		return "Claude " + publicName
	}

	model = strings.TrimSpace(model)
	if model == "" {
		return "Claude"
	}
	return "Claude (" + model + ")"
}

func publicFamilyForID(model string) (publicModelFamily, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	for _, family := range publicModelFamilies {
		if normalized == family.publicName {
			return family, true
		}
		for _, alias := range family.aliases {
			if normalized == alias {
				return family, true
			}
		}
	}
	return publicModelFamily{}, false
}

func splitOneMContextSuffix(model string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	if strings.HasSuffix(strings.ToLower(trimmed), oneMContextSuffix) {
		baseName := trimmed[:len(trimmed)-len(oneMContextSuffix)]
		return strings.TrimSpace(baseName), true
	}
	return trimmed, false
}
