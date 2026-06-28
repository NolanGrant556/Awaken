package gateway

import (
	"context"
	"strings"
)

type Platform string

const (
	PlatformLocal       Platform = "local"
	PlatformDingTalk    Platform = "dingtalk"
	PlatformFeishu      Platform = "feishu"
	PlatformSlack       Platform = "slack"
	PlatformDiscord     Platform = "discord"
	PlatformTelegram    Platform = "telegram"
	PlatformWeChat      Platform = "wechat"
	PlatformWhatsApp    Platform = "whatsapp"
	PlatformSignal      Platform = "signal"
	PlatformBlueBubbles Platform = "bluebubbles"
)

var knownPlatforms = map[Platform]struct{}{
	PlatformLocal:       {},
	PlatformDingTalk:    {},
	PlatformFeishu:      {},
	PlatformSlack:       {},
	PlatformDiscord:     {},
	PlatformTelegram:    {},
	PlatformWeChat:      {},
	PlatformWhatsApp:    {},
	PlatformSignal:      {},
	PlatformBlueBubbles: {},
}

func (p Platform) IsKnown() bool {
	_, ok := knownPlatforms[p]
	return ok
}

func normalizePlatform(value string) (Platform, bool) {
	platform := Platform(strings.ToLower(strings.TrimSpace(value)))
	return platform, platform.IsKnown()
}

type SessionSource struct {
	Platform     Platform
	ChatID       string
	ChatName     string
	ChatType     string
	UserID       string
	UserName     string
	ThreadID     string
	ChatTopic    string
	UserIDAlt    string
	ChatIDAlt    string
	GuildID      string
	ParentChatID string
	MessageID    string
	IsBot        bool
}

func (s SessionSource) Description() string {
	if s.Platform == PlatformLocal {
		return "local machine"
	}

	chatType := strings.TrimSpace(s.ChatType)
	if chatType == "" {
		chatType = "dm"
	}

	var description string
	switch chatType {
	case "dm":
		person := firstNonEmpty(s.UserName, s.UserID, "user")
		description = "DM with " + person
	case "group":
		description = "group: " + firstNonEmpty(s.ChatName, s.ChatID, "unknown")
	case "channel":
		description = "channel: " + firstNonEmpty(s.ChatName, s.ChatID, "unknown")
	case "thread":
		description = "thread: " + firstNonEmpty(s.ChatName, s.ChatID, "unknown")
	default:
		description = firstNonEmpty(s.ChatName, s.ChatID, chatType)
	}

	if strings.TrimSpace(s.ThreadID) != "" {
		description += ", thread: " + s.ThreadID
	}
	return description
}

type HomeChannel struct {
	Platform Platform
	ChatID   string
	ThreadID string
	Name     string
}

type SessionContext struct {
	Source                 SessionSource
	ConnectedPlatforms     []Platform
	HomeChannels           map[Platform]HomeChannel
	SharedMultiUserSession bool
	SessionKey             string
	SessionID              string
}

type Adapter interface {
	Send(ctx context.Context, target DeliveryTarget, content string, metadata map[string]string) (AdapterSendResult, error)
}

type AdapterSendResult struct {
	MessageID string
}

type DeliveryOptions struct {
	JobID    string
	JobName  string
	Metadata map[string]string
}

type DeliveryResult struct {
	Target         string
	Platform       Platform
	Success        bool
	LocalPath      string
	FullOutputPath string
	MessageID      string
	Truncated      bool
	OriginalChars  int
	DeliveredChars int
	Error          string
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
