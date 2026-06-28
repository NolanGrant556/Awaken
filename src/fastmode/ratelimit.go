package fastmode

import (
	"math"
	"strconv"
	"strings"
	"time"
)

const MinResetForBreakerSeconds = 60.0

type RateLimitBucket struct {
	Limit        int
	Remaining    int
	ResetSeconds float64
	CapturedAt   time.Time
	HasLimit     bool
	HasRemaining bool
	HasReset     bool
}

func (b RateLimitBucket) Used() int {
	used := b.Limit - b.Remaining
	if used < 0 {
		return 0
	}
	return used
}

func (b RateLimitBucket) UsagePct() float64 {
	if b.Limit <= 0 {
		return 0
	}
	return float64(b.Used()) / float64(b.Limit) * 100
}

func (b RateLimitBucket) RemainingSeconds(now time.Time) float64 {
	if b.CapturedAt.IsZero() {
		return maxFloat(0, b.ResetSeconds)
	}
	return maxFloat(0, b.ResetSeconds-now.Sub(b.CapturedAt).Seconds())
}

type RateLimitState struct {
	RequestsMin  RateLimitBucket
	RequestsHour RateLimitBucket
	TokensMin    RateLimitBucket
	TokensHour   RateLimitBucket
	CapturedAt   time.Time
	Provider     string
}

func (s RateLimitState) HasData() bool {
	return !s.CapturedAt.IsZero()
}

func (s RateLimitState) AgeSeconds(now time.Time) float64 {
	if !s.HasData() {
		return math.Inf(1)
	}
	return now.Sub(s.CapturedAt).Seconds()
}

func ParseRateLimitHeaders(headers map[string]string, provider string, capturedAt ...time.Time) (RateLimitState, bool) {
	lowered := lowerHeaderMap(headers)
	if !hasRateLimitHeader(lowered) {
		return RateLimitState{}, false
	}

	now := time.Now()
	if len(capturedAt) > 0 {
		now = capturedAt[0]
	}

	return RateLimitState{
		RequestsMin:  rateLimitBucket(lowered, "requests", now),
		RequestsHour: rateLimitBucket(lowered, "requests-1h", now),
		TokensMin:    rateLimitBucket(lowered, "tokens", now),
		TokensHour:   rateLimitBucket(lowered, "tokens-1h", now),
		CapturedAt:   now,
		Provider:     provider,
	}, true
}

func ShouldTripBreaker(headers map[string]string, lastKnown *RateLimitState, nowArg ...time.Time) bool {
	now := time.Now()
	if len(nowArg) > 0 {
		now = nowArg[0]
	}

	if current, ok := ParseRateLimitHeaders(headers, "", now); ok {
		for _, bucket := range current.buckets() {
			if bucket.HasRemaining && bucket.Remaining <= 0 && bucket.HasReset && bucket.ResetSeconds >= MinResetForBreakerSeconds {
				return true
			}
		}
	}

	if lastKnown == nil {
		return false
	}
	for _, bucket := range lastKnown.buckets() {
		if bucket.Limit <= 0 {
			continue
		}
		if bucket.Remaining > 0 {
			continue
		}
		if bucket.RemainingSeconds(now) >= MinResetForBreakerSeconds {
			return true
		}
	}
	return false
}

func (s RateLimitState) buckets() []RateLimitBucket {
	return []RateLimitBucket{s.RequestsMin, s.RequestsHour, s.TokensMin, s.TokensHour}
}

func rateLimitBucket(headers map[string]string, tag string, capturedAt time.Time) RateLimitBucket {
	limit, hasLimit := parseHeaderInt(headers["x-ratelimit-limit-"+tag])
	remaining, hasRemaining := parseHeaderInt(headers["x-ratelimit-remaining-"+tag])
	reset, hasReset := parseHeaderFloat(headers["x-ratelimit-reset-"+tag])
	return RateLimitBucket{
		Limit:        limit,
		Remaining:    remaining,
		ResetSeconds: reset,
		CapturedAt:   capturedAt,
		HasLimit:     hasLimit,
		HasRemaining: hasRemaining,
		HasReset:     hasReset,
	}
}

func lowerHeaderMap(headers map[string]string) map[string]string {
	lowered := make(map[string]string, len(headers))
	for key, value := range headers {
		lowered[strings.ToLower(key)] = value
	}
	return lowered
}

func hasRateLimitHeader(headers map[string]string) bool {
	for key := range headers {
		if strings.HasPrefix(key, "x-ratelimit-") {
			return true
		}
	}
	return false
}

func parseHeaderInt(raw string) (int, bool) {
	value, ok := parseHeaderFloat(raw)
	return int(value), ok
}

func parseHeaderFloat(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
