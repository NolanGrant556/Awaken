package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const DefaultMaxPlatformOutputChars = 4000

type DeliveryRouter struct {
	outputDir        string
	adapters         map[Platform]Adapter
	homeChannels     map[Platform]HomeChannel
	maxPlatformChars int
	now              func() time.Time
}

type RouterOption func(*DeliveryRouter)

func NewDeliveryRouter(opts ...RouterOption) *DeliveryRouter {
	router := &DeliveryRouter{
		outputDir:        "gateway-output",
		adapters:         map[Platform]Adapter{},
		homeChannels:     map[Platform]HomeChannel{},
		maxPlatformChars: DefaultMaxPlatformOutputChars,
		now:              time.Now,
	}
	for _, opt := range opts {
		opt(router)
	}
	return router
}

func WithOutputDir(dir string) RouterOption {
	return func(router *DeliveryRouter) {
		if strings.TrimSpace(dir) != "" {
			router.outputDir = dir
		}
	}
}

func WithAdapters(adapters map[Platform]Adapter) RouterOption {
	return func(router *DeliveryRouter) {
		for platform, adapter := range adapters {
			if platform.IsKnown() && adapter != nil {
				router.adapters[platform] = adapter
			}
		}
	}
}

func WithAdapter(platform Platform, adapter Adapter) RouterOption {
	return func(router *DeliveryRouter) {
		if platform.IsKnown() && adapter != nil {
			router.adapters[platform] = adapter
		}
	}
}

func WithHomeChannels(homeChannels map[Platform]HomeChannel) RouterOption {
	return func(router *DeliveryRouter) {
		for platform, home := range homeChannels {
			if platform.IsKnown() {
				home.Platform = platform
				router.homeChannels[platform] = home
			}
		}
	}
}

func WithMaxPlatformOutputChars(max int) RouterOption {
	return func(router *DeliveryRouter) {
		if max > 0 {
			router.maxPlatformChars = max
		}
	}
}

func WithClock(now func() time.Time) RouterOption {
	return func(router *DeliveryRouter) {
		if now != nil {
			router.now = now
		}
	}
}

func (r *DeliveryRouter) Deliver(ctx context.Context, content string, targets []DeliveryTarget, opts DeliveryOptions) []DeliveryResult {
	if len(targets) == 0 {
		targets = []DeliveryTarget{{Platform: PlatformLocal}}
	}

	results := make([]DeliveryResult, 0, len(targets))
	for _, target := range targets {
		resolved, err := r.resolveTarget(target)
		result := DeliveryResult{
			Target:        target.String(),
			Platform:      resolved.Platform,
			OriginalChars: runeCount(content),
		}
		if err != nil {
			result.Error = redactSensitiveText(err.Error())
			results = append(results, result)
			continue
		}

		if resolved.Platform == PlatformLocal {
			localResult := r.deliverLocal(content, opts)
			localResult.Target = target.String()
			localResult.Platform = PlatformLocal
			results = append(results, localResult)
			continue
		}

		platformResult := r.deliverPlatform(ctx, resolved, content, opts)
		platformResult.Target = target.String()
		results = append(results, platformResult)
	}
	return results
}

func (r *DeliveryRouter) resolveTarget(target DeliveryTarget) (DeliveryTarget, error) {
	if target.Platform == "" || !target.Platform.IsKnown() {
		return DeliveryTarget{Platform: PlatformLocal}, nil
	}
	if target.Platform == PlatformLocal {
		return target, nil
	}
	if strings.TrimSpace(target.ChatID) != "" {
		return target, nil
	}

	home, ok := r.homeChannels[target.Platform]
	if !ok || strings.TrimSpace(home.ChatID) == "" {
		return target, fmt.Errorf("no chat ID or home channel configured for %s", target.Platform)
	}
	target.ChatID = home.ChatID
	if strings.TrimSpace(target.ThreadID) == "" {
		target.ThreadID = home.ThreadID
	}
	return target, nil
}

func (r *DeliveryRouter) deliverLocal(content string, opts DeliveryOptions) DeliveryResult {
	path, err := r.writeLocalOutput(content, opts)
	if err != nil {
		return DeliveryResult{
			Platform:      PlatformLocal,
			OriginalChars: runeCount(content),
			Error:         redactSensitiveText(err.Error()),
		}
	}
	return DeliveryResult{
		Platform:       PlatformLocal,
		Success:        true,
		LocalPath:      path,
		OriginalChars:  runeCount(content),
		DeliveredChars: runeCount(content),
	}
}

func (r *DeliveryRouter) deliverPlatform(ctx context.Context, target DeliveryTarget, content string, opts DeliveryOptions) DeliveryResult {
	result := DeliveryResult{
		Platform:      target.Platform,
		OriginalChars: runeCount(content),
	}

	adapter := r.adapters[target.Platform]
	if adapter == nil {
		result.Error = fmt.Sprintf("no adapter configured for %s", target.Platform)
		return result
	}

	sendContent := content
	if runeCount(content) > r.maxPlatformChars {
		path, err := r.writeFullPlatformOutput(content, opts, target)
		if err != nil {
			result.Error = redactSensitiveText(err.Error())
			return result
		}
		result.Truncated = true
		result.FullOutputPath = path
		sendContent = truncateWithReference(content, r.maxPlatformChars, filepath.Base(path))
	}

	metadata := copyMetadata(opts.Metadata)
	if strings.TrimSpace(target.ThreadID) != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata["thread_id"] = target.ThreadID
	}

	sendResult, err := adapter.Send(ctx, target, sendContent, metadata)
	result.DeliveredChars = runeCount(sendContent)
	if err != nil {
		result.Error = redactSensitiveText(err.Error())
		return result
	}
	result.Success = true
	result.MessageID = redactSensitiveText(sendResult.MessageID)
	return result
}

func (r *DeliveryRouter) writeLocalOutput(content string, opts DeliveryOptions) (string, error) {
	jobID := safePathSegment(opts.JobID, "misc")
	dir := filepath.Join(r.outputDir, jobID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	lines := []string{"# " + firstNonEmpty(opts.JobName, "Delivery Output"), ""}
	lines = append(lines, "**Timestamp:** "+r.now().UTC().Format(time.RFC3339))
	if strings.TrimSpace(opts.JobID) != "" {
		lines = append(lines, "**Job ID:** "+safePathSegment(opts.JobID, "misc"))
	}
	for _, key := range sortedMetadataKeys(opts.Metadata) {
		value := sanitizedMetadata(opts.Metadata)[key]
		lines = append(lines, fmt.Sprintf("**%s:** %s", key, value))
	}
	lines = append(lines, "", "---", "", content)
	return writeUniqueFile(dir, r.timestampName(), ".md", []byte(strings.Join(lines, "\n")))
}

func (r *DeliveryRouter) writeFullPlatformOutput(content string, opts DeliveryOptions, target DeliveryTarget) (string, error) {
	jobID := safePathSegment(opts.JobID, "misc")
	targetName := safePathSegment(target.String(), string(target.Platform))
	dir := filepath.Join(r.outputDir, "platform-full", jobID, targetName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return writeUniqueFile(dir, r.timestampName(), ".txt", []byte(content))
}

func (r *DeliveryRouter) timestampName() string {
	return r.now().UTC().Format("20060102_150405.000000000")
}

func sortedMetadataKeys(metadata map[string]string) []string {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func safePathSegment(value, fallback string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	safe := strings.Trim(builder.String(), "._-")
	if len(safe) > 80 {
		safe = strings.Trim(safe[:80], "._-")
	}
	if safe == "" || safe == "." || safe == ".." {
		safe = fallback
	}
	return safe
}

func writeUniqueFile(dir, base, ext string, data []byte) (string, error) {
	path := filepath.Join(dir, base+ext)
	for i := 1; i < 1000; i++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			defer file.Close()
			if _, writeErr := file.Write(data); writeErr != nil {
				return "", writeErr
			}
			return path, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
	}
	return "", fmt.Errorf("could not allocate unique output path in %s", dir)
}

func truncateWithReference(content string, maxChars int, reference string) string {
	if maxChars <= 0 {
		return ""
	}
	suffix := "\n\n[truncated; full output saved locally as " + reference + "]"
	suffixChars := runeCount(suffix)
	if suffixChars >= maxChars {
		return truncateRunes(suffix, maxChars)
	}
	return truncateRunes(content, maxChars-suffixChars) + suffix
}

func truncateRunes(content string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	if runeCount(content) <= maxChars {
		return content
	}
	runes := []rune(content)
	return string(runes[:maxChars])
}

func runeCount(content string) int {
	return utf8.RuneCountInString(content)
}
