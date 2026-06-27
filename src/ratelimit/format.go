package ratelimit

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func FormatCompact(state State, now time.Time) string {
	if !state.HasData() {
		return "rate limit: no data"
	}

	parts := make([]string, 0, 5)
	if state.Provider != "" {
		parts = append(parts, state.Provider)
	}
	for _, bucket := range stateBuckets(state) {
		if !bucketHasValues(bucket.bucket) {
			continue
		}
		parts = append(parts, formatBucketCompact(bucket, now))
	}
	if len(parts) == 0 {
		return "rate limit: no bucket data"
	}
	return strings.Join(parts, "; ")
}

func FormatCount(n int) string {
	sign := ""
	value := n
	if value < 0 {
		sign = "-"
		value = -value
	}
	if value >= 1_000_000 {
		return fmt.Sprintf("%s%.1fM", sign, float64(value)/1_000_000)
	}
	if value >= 1_000 {
		return fmt.Sprintf("%s%.1fK", sign, float64(value)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func FormatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	totalSeconds := int64(math.Ceil(duration.Seconds()))
	if totalSeconds < 60 {
		return fmt.Sprintf("%ds", totalSeconds)
	}

	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes < 60 {
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}

	hours := minutes / 60
	minutes = minutes % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func formatBucketCompact(entry bucketEntry, now time.Time) string {
	label := shortBucketLabel(entry.resource, entry.window)
	resetIn := FormatDuration(entry.bucket.RemainingDuration(now))
	if entry.bucket.Limit <= 0 {
		return fmt.Sprintf("%s remaining %s, reset %s", label, FormatCount(entry.bucket.Remaining), resetIn)
	}
	return fmt.Sprintf(
		"%s used %s/%s (%.1f%%), remaining %s, reset %s",
		label,
		FormatCount(entry.bucket.Used()),
		FormatCount(entry.bucket.Limit),
		entry.bucket.UsagePct(),
		FormatCount(entry.bucket.Remaining),
		resetIn,
	)
}

func shortBucketLabel(resource Resource, window Window) string {
	resourceLabel := "req"
	if resource == ResourceTokens {
		resourceLabel = "tok"
	}
	windowLabel := "min"
	if window == WindowHour {
		windowLabel = "hour"
	}
	return resourceLabel + "/" + windowLabel
}
