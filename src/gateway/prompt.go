package gateway

import (
	"fmt"
	"sort"
	"strings"
)

func BuildSessionContextPrompt(context SessionContext, redactPII bool) string {
	redact := redactPII && shouldRedactPII(context.Source.Platform)
	lines := []string{
		"## Current Session Context",
		"",
	}

	source := context.Source
	if source.Platform == PlatformLocal {
		lines = append(lines, "**Source:** local (the machine running Awaken)")
	} else {
		lines = append(lines, fmt.Sprintf("**Source:** %s (%s)", source.Platform, sourceDescription(source, redact)))
	}

	if strings.TrimSpace(source.ChatTopic) != "" {
		lines = append(lines, "**Channel Topic:** "+source.ChatTopic)
	}

	if context.SharedMultiUserSession {
		label := "Multi-user session"
		if strings.TrimSpace(source.ThreadID) != "" {
			label = "Multi-user thread"
		}
		lines = append(lines, "**Session type:** "+label+"; messages may come from multiple senders.")
	} else if strings.TrimSpace(source.UserName) != "" {
		lines = append(lines, "**User:** "+source.UserName)
	} else if strings.TrimSpace(source.UserID) != "" {
		userID := source.UserID
		if redact {
			userID = hashUserID(userID)
		}
		lines = append(lines, "**User ID:** "+userID)
	}

	lines = append(lines, "**Connected Platforms:** "+connectedPlatformsLine(context.ConnectedPlatforms))

	if len(context.HomeChannels) > 0 {
		lines = append(lines, "", "**Home Channels (default destinations):**")
		for _, platform := range sortedHomePlatforms(context.HomeChannels) {
			home := context.HomeChannels[platform]
			chatID := home.ChatID
			if redact {
				chatID = hashChatID(chatID)
			}
			name := firstNonEmpty(home.Name, "home")
			lines = append(lines, fmt.Sprintf("- %s: %s (ID: %s)", platform, name, chatID))
		}
	}

	lines = append(lines, "", "**Delivery options:**")
	if source.Platform == PlatformLocal {
		lines = append(lines, "- `origin` -> local output")
	} else {
		originLabel := firstNonEmpty(source.ChatName, source.ChatID)
		if redact {
			originLabel = hashChatID(source.ChatID)
		}
		lines = append(lines, "- `origin` -> back to this chat ("+originLabel+")")
	}
	lines = append(lines, "- `local` -> save to local files on this machine")
	for _, platform := range sortedHomePlatforms(context.HomeChannels) {
		home := context.HomeChannels[platform]
		lines = append(lines, fmt.Sprintf("- `%s` -> home channel (%s)", platform, firstNonEmpty(home.Name, home.ChatID)))
	}
	lines = append(lines, "- Explicit targets use `platform:chat_id` or `platform:chat_id:thread_id`.")

	return strings.Join(lines, "\n")
}

func sourceDescription(source SessionSource, redact bool) string {
	if !redact {
		return source.Description()
	}

	chatType := strings.TrimSpace(source.ChatType)
	if chatType == "" {
		chatType = "dm"
	}
	user := firstNonEmpty(source.UserName, hashUserID(source.UserID), "user")
	chat := firstNonEmpty(source.ChatName, hashChatID(source.ChatID), "chat")

	switch chatType {
	case "dm":
		return "DM with " + user
	case "group":
		return "group: " + chat
	case "channel":
		return "channel: " + chat
	case "thread":
		return "thread: " + chat
	default:
		return chat
	}
}

func connectedPlatformsLine(platforms []Platform) string {
	seen := map[Platform]struct{}{PlatformLocal: {}}
	for _, platform := range platforms {
		if platform.IsKnown() {
			seen[platform] = struct{}{}
		}
	}

	labels := []string{"local (files on this machine)"}
	var rest []string
	for platform := range seen {
		if platform != PlatformLocal {
			rest = append(rest, string(platform))
		}
	}
	sort.Strings(rest)
	for _, label := range rest {
		labels = append(labels, label+": connected")
	}
	return strings.Join(labels, ", ")
}

func sortedHomePlatforms(homeChannels map[Platform]HomeChannel) []Platform {
	platforms := make([]Platform, 0, len(homeChannels))
	for platform := range homeChannels {
		if platform.IsKnown() && platform != PlatformLocal {
			platforms = append(platforms, platform)
		}
	}
	sort.Slice(platforms, func(i, j int) bool {
		return platforms[i] < platforms[j]
	})
	return platforms
}
