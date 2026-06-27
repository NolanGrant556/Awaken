package gateway

import "strings"

type DeliveryTarget struct {
	Platform   Platform
	ChatID     string
	ThreadID   string
	IsOrigin   bool
	IsExplicit bool
}

func ParseDeliveryTarget(target string, origin *SessionSource) DeliveryTarget {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return localTarget(false)
	}

	lower := strings.ToLower(raw)
	switch lower {
	case "origin":
		if origin == nil || !origin.Platform.IsKnown() || origin.Platform == PlatformLocal || strings.TrimSpace(origin.ChatID) == "" {
			return localTarget(true)
		}
		return DeliveryTarget{
			Platform: origin.Platform,
			ChatID:   origin.ChatID,
			ThreadID: origin.ThreadID,
			IsOrigin: true,
		}
	case "local":
		return localTarget(false)
	}

	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 3)
		platform, ok := normalizePlatform(parts[0])
		if !ok {
			return localTarget(false)
		}
		chatID := ""
		if len(parts) > 1 {
			chatID = strings.TrimSpace(parts[1])
		}
		if chatID == "" {
			return localTarget(false)
		}
		threadID := ""
		if len(parts) > 2 {
			threadID = strings.TrimSpace(parts[2])
		}
		return DeliveryTarget{
			Platform:   platform,
			ChatID:     chatID,
			ThreadID:   threadID,
			IsExplicit: true,
		}
	}

	platform, ok := normalizePlatform(raw)
	if !ok {
		return localTarget(false)
	}
	return DeliveryTarget{Platform: platform}
}

func (t DeliveryTarget) String() string {
	if t.IsOrigin {
		return "origin"
	}
	if t.Platform == "" || t.Platform == PlatformLocal {
		return "local"
	}
	if t.ChatID != "" && t.ThreadID != "" {
		return string(t.Platform) + ":" + t.ChatID + ":" + t.ThreadID
	}
	if t.ChatID != "" {
		return string(t.Platform) + ":" + t.ChatID
	}
	return string(t.Platform)
}

func localTarget(isOrigin bool) DeliveryTarget {
	return DeliveryTarget{Platform: PlatformLocal, IsOrigin: isOrigin}
}
