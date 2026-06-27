package ratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Resource string

const (
	ResourceRequests Resource = "requests"
	ResourceTokens   Resource = "tokens"
)

type Window string

const (
	WindowMinute Window = "minute"
	WindowHour   Window = "hour"
)

const HotUsageThreshold = 80.0

type Bucket struct {
	Limit      int
	Remaining  int
	Reset      time.Duration
	CapturedAt time.Time
}

func (b Bucket) Used() int {
	if b.Limit <= 0 {
		return 0
	}
	used := b.Limit - b.Remaining
	if used < 0 {
		return 0
	}
	return used
}

func (b Bucket) UsagePct() float64 {
	if b.Limit <= 0 {
		return 0
	}
	return float64(b.Used()) / float64(b.Limit) * 100
}

func (b Bucket) RemainingDuration(now time.Time) time.Duration {
	if b.Reset <= 0 || b.CapturedAt.IsZero() {
		return 0
	}
	elapsed := now.Sub(b.CapturedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	remaining := b.Reset - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

type State struct {
	RequestsMinute Bucket
	RequestsHour   Bucket
	TokensMinute   Bucket
	TokensHour     Bucket
	Provider       string
	CapturedAt     time.Time
}

func (s State) HasData() bool {
	return !s.CapturedAt.IsZero()
}

func (s State) Age(now time.Time) time.Duration {
	if !s.HasData() {
		return 0
	}
	age := now.Sub(s.CapturedAt)
	if age < 0 {
		return 0
	}
	return age
}

type Warning struct {
	Resource Resource
	Window   Window
	Bucket   Bucket
	UsagePct float64
	ResetIn  time.Duration
}

type ThrottleDecision struct {
	ShouldThrottle bool
	Resource       Resource
	Window         Window
	ResetIn        time.Duration
}

func ParseHeaders(headers http.Header, provider string) (State, bool) {
	return ParseHeadersAt(headers, provider, time.Now().UTC())
}

func ParseHeadersAt(headers http.Header, provider string, capturedAt time.Time) (State, bool) {
	values := normalizeHeaders(headers)
	if !hasKnownRateLimitHeader(values) {
		return State{}, false
	}

	state := State{
		RequestsMinute: parseBucket(values, ResourceRequests, WindowMinute, capturedAt),
		RequestsHour:   parseBucket(values, ResourceRequests, WindowHour, capturedAt),
		TokensMinute:   parseBucket(values, ResourceTokens, WindowMinute, capturedAt),
		TokensHour:     parseBucket(values, ResourceTokens, WindowHour, capturedAt),
		Provider:       provider,
		CapturedAt:     capturedAt,
	}
	return state, true
}

func (s State) HotWarnings(now time.Time) []Warning {
	warnings := make([]Warning, 0, 4)
	for _, bucket := range stateBuckets(s) {
		if bucket.bucket.Limit <= 0 || bucket.bucket.UsagePct() < HotUsageThreshold {
			continue
		}
		warnings = append(warnings, Warning{
			Resource: bucket.resource,
			Window:   bucket.window,
			Bucket:   bucket.bucket,
			UsagePct: bucket.bucket.UsagePct(),
			ResetIn:  bucket.bucket.RemainingDuration(now),
		})
	}
	return warnings
}

func (s State) ShouldThrottle(now time.Time) bool {
	return s.ThrottleDecision(now).ShouldThrottle
}

func (s State) ThrottleDecision(now time.Time) ThrottleDecision {
	var decision ThrottleDecision
	for _, bucket := range stateBuckets(s) {
		resetIn := bucket.bucket.RemainingDuration(now)
		if !bucketHasValues(bucket.bucket) || bucket.bucket.Remaining > 0 || resetIn <= 0 {
			continue
		}
		if !decision.ShouldThrottle || resetIn > decision.ResetIn {
			decision = ThrottleDecision{
				ShouldThrottle: true,
				Resource:       bucket.resource,
				Window:         bucket.window,
				ResetIn:        resetIn,
			}
		}
	}
	return decision
}

type bucketEntry struct {
	resource Resource
	window   Window
	bucket   Bucket
}

func stateBuckets(s State) []bucketEntry {
	return []bucketEntry{
		{resource: ResourceRequests, window: WindowMinute, bucket: s.RequestsMinute},
		{resource: ResourceRequests, window: WindowHour, bucket: s.RequestsHour},
		{resource: ResourceTokens, window: WindowMinute, bucket: s.TokensMinute},
		{resource: ResourceTokens, window: WindowHour, bucket: s.TokensHour},
	}
}

func bucketHasValues(bucket Bucket) bool {
	return bucket.Limit != 0 || bucket.Remaining != 0 || bucket.Reset != 0
}

func normalizeHeaders(headers http.Header) map[string]string {
	values := make(map[string]string, len(headers))
	for key, headerValues := range headers {
		lowerKey := strings.ToLower(key)
		if len(headerValues) == 0 {
			values[lowerKey] = ""
			continue
		}
		values[lowerKey] = strings.TrimSpace(headerValues[0])
		for _, value := range headerValues {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				values[lowerKey] = trimmed
				break
			}
		}
	}
	return values
}

func hasKnownRateLimitHeader(values map[string]string) bool {
	for _, name := range knownHeaderNames {
		if _, ok := values[name]; ok {
			return true
		}
	}
	return false
}

func parseBucket(values map[string]string, resource Resource, window Window, capturedAt time.Time) Bucket {
	suffix := string(resource)
	if window == WindowHour {
		suffix += "-1h"
	}
	return Bucket{
		Limit:      parseInt(values["x-ratelimit-limit-"+suffix]),
		Remaining:  parseInt(values["x-ratelimit-remaining-"+suffix]),
		Reset:      parseReset(values["x-ratelimit-reset-"+suffix]),
		CapturedAt: capturedAt,
	}
}

func parseInt(value string) int {
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int(parsed)
}

func parseReset(value string) time.Duration {
	if value == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

var knownHeaderNames = []string{
	"x-ratelimit-limit-requests",
	"x-ratelimit-limit-requests-1h",
	"x-ratelimit-limit-tokens",
	"x-ratelimit-limit-tokens-1h",
	"x-ratelimit-remaining-requests",
	"x-ratelimit-remaining-requests-1h",
	"x-ratelimit-remaining-tokens",
	"x-ratelimit-remaining-tokens-1h",
	"x-ratelimit-reset-requests",
	"x-ratelimit-reset-requests-1h",
	"x-ratelimit-reset-tokens",
	"x-ratelimit-reset-tokens-1h",
}
