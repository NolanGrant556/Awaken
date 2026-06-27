package toolpreview

import "unicode/utf8"

const (
	DefaultShortLength = 40

	ellipsis = "..."
)

type Mode string

const (
	ModeOff     Mode = "off"
	ModeNew     Mode = "new"
	ModeAll     Mode = "all"
	ModeVerbose Mode = "verbose"
)

type LengthSource string

const (
	LengthSourceDefault          LengthSource = "default"
	LengthSourceGlobal           LengthSource = "global"
	LengthSourcePlatformOverride LengthSource = "platform_override"
)

type Resolution struct {
	Mode             Mode
	Length           int
	ConfiguredLength int
	Unlimited        bool
	Defaulted        bool
	Source           LengthSource
}

type Policy struct {
	Mode          Mode
	PreviewLength int
}

type Preview struct {
	ToolName      string
	Preview       string
	Text          string
	Truncated     bool
	OriginalRunes int
}

func ResolveLength(global, platformOverride *int, mode Mode) Resolution {
	configured, source := configuredLength(global, platformOverride)
	resolution := Resolution{
		Mode:             mode,
		Length:           configured,
		ConfiguredLength: configured,
		Source:           source,
	}

	switch mode {
	case ModeOff:
		resolution.Length = 0
	case ModeVerbose:
		if configured == 0 {
			resolution.Unlimited = true
		}
	case ModeAll, ModeNew:
		if configured == 0 {
			resolution.Length = DefaultShortLength
			resolution.Defaulted = true
			resolution.Source = LengthSourceDefault
		}
	default:
		if configured == 0 {
			resolution.Length = DefaultShortLength
			resolution.Defaulted = true
			resolution.Source = LengthSourceDefault
		}
	}

	return resolution
}

func Format(toolName, preview string, policy Policy) Preview {
	originalRunes := utf8.RuneCountInString(preview)
	if policy.Mode == ModeOff {
		return Preview{
			ToolName:      toolName,
			Text:          toolName,
			OriginalRunes: originalRunes,
		}
	}

	resolution := ResolveLength(&policy.PreviewLength, nil, policy.Mode)
	formatted, truncated := truncate(preview, resolution)
	return Preview{
		ToolName:      toolName,
		Preview:       formatted,
		Text:          render(toolName, formatted),
		Truncated:     truncated,
		OriginalRunes: originalRunes,
	}
}

func configuredLength(global, platformOverride *int) (int, LengthSource) {
	if platformOverride != nil {
		return normalizeLength(*platformOverride), LengthSourcePlatformOverride
	}
	if global != nil {
		return normalizeLength(*global), LengthSourceGlobal
	}
	return 0, LengthSourceDefault
}

func normalizeLength(length int) int {
	if length < 0 {
		return 0
	}
	return length
}

func truncate(preview string, resolution Resolution) (string, bool) {
	originalRunes := utf8.RuneCountInString(preview)
	if resolution.Unlimited || resolution.Length <= 0 || originalRunes <= resolution.Length {
		return preview, false
	}

	ellipsisRunes := []rune(ellipsis)
	if resolution.Length <= len(ellipsisRunes) {
		return string(ellipsisRunes[:resolution.Length]), true
	}

	runes := []rune(preview)
	keep := resolution.Length - len(ellipsisRunes)
	return string(runes[:keep]) + ellipsis, true
}

func render(toolName, preview string) string {
	if toolName == "" {
		return preview
	}
	if preview == "" {
		return toolName
	}
	return toolName + ": " + preview
}
