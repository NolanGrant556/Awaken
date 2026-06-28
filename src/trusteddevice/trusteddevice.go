package trusteddevice

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	SessionIngressTokenPrefix = "sk-ant-si-"
	MinRefreshDelay           = 30 * time.Second
	MaxRefreshFailures        = 3
)

// DecodeJWTPayload decodes the payload segment of a JWT without verifying it.
// It strips the session-ingress token prefix used by bridge tokens.
func DecodeJWTPayload(token string) (map[string]any, bool) {
	jwt := strings.TrimPrefix(token, SessionIngressTokenPrefix)
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || parts[1] == "" {
		return nil, false
	}

	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, false
	}

	decoder := json.NewDecoder(bytes.NewReader(payloadBytes))
	decoder.UseNumber()

	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, false
	}
	return payload, true
}

// DecodeJWTExpiry returns the exp claim from a JWT payload as Unix seconds.
func DecodeJWTExpiry(token string) (int64, bool) {
	payload, ok := DecodeJWTPayload(token)
	if !ok {
		return 0, false
	}

	exp, ok := payload["exp"]
	if !ok {
		return 0, false
	}

	switch value := exp.(type) {
	case json.Number:
		return jsonNumberToInt64(value)
	case float64:
		if math.Trunc(value) != value || value > math.MaxInt64 || value < math.MinInt64 {
			return 0, false
		}
		return int64(value), true
	case int64:
		return value, true
	case int:
		return int64(value), true
	default:
		return 0, false
	}
}

func decodeBase64URL(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func jsonNumberToInt64(value json.Number) (int64, bool) {
	if parsed, err := value.Int64(); err == nil {
		return parsed, true
	}

	parsed, err := strconv.ParseFloat(value.String(), 64)
	if err != nil || math.Trunc(parsed) != parsed || parsed > math.MaxInt64 || parsed < math.MinInt64 {
		return 0, false
	}
	return int64(parsed), true
}

type TokenSource struct {
	GateEnabled bool
	EnvToken    string
	StoredToken string

	cacheValid        bool
	cachedStoredToken string
}

func (s *TokenSource) Token() (string, bool) {
	if s == nil || !s.GateEnabled {
		return "", false
	}
	if s.EnvToken != "" {
		return s.EnvToken, true
	}
	if !s.cacheValid {
		s.cachedStoredToken = s.StoredToken
		s.cacheValid = true
	}
	if s.cachedStoredToken == "" {
		return "", false
	}
	return s.cachedStoredToken, true
}

func (s *TokenSource) ClearStoredToken() bool {
	if s == nil || !s.GateEnabled {
		return false
	}

	deleted := s.StoredToken != ""
	s.StoredToken = ""
	s.ClearCache()
	return deleted
}

func (s *TokenSource) ClearCache() {
	if s == nil {
		return
	}
	s.cacheValid = false
	s.cachedStoredToken = ""
}

func (s TokenSource) ShouldEnroll() bool {
	return s.GateEnabled && s.EnvToken == ""
}

type EnrollmentInput struct {
	GateEnabled          bool
	EssentialTrafficOnly bool
	OAuthToken           string
	EnvToken             string
	FreshLogin           bool
}

type EnrollmentSkipReason string

const (
	EnrollmentSkipNone                 EnrollmentSkipReason = ""
	EnrollmentSkipGateOff              EnrollmentSkipReason = "gate_off"
	EnrollmentSkipEssentialTrafficOnly EnrollmentSkipReason = "essential_traffic_only"
	EnrollmentSkipEnvToken             EnrollmentSkipReason = "env_token"
	EnrollmentSkipNoOAuthToken         EnrollmentSkipReason = "no_oauth"
	EnrollmentSkipNotFreshLogin        EnrollmentSkipReason = "not_fresh_login"
)

type EnrollmentDecisionResult struct {
	Enroll bool
	Reason EnrollmentSkipReason
}

func EnrollmentDecision(input EnrollmentInput) EnrollmentDecisionResult {
	switch {
	case !input.GateEnabled:
		return EnrollmentDecisionResult{Reason: EnrollmentSkipGateOff}
	case input.EssentialTrafficOnly:
		return EnrollmentDecisionResult{Reason: EnrollmentSkipEssentialTrafficOnly}
	case input.EnvToken != "":
		return EnrollmentDecisionResult{Reason: EnrollmentSkipEnvToken}
	case input.OAuthToken == "":
		return EnrollmentDecisionResult{Reason: EnrollmentSkipNoOAuthToken}
	case !input.FreshLogin:
		return EnrollmentDecisionResult{Reason: EnrollmentSkipNotFreshLogin}
	default:
		return EnrollmentDecisionResult{Enroll: true}
	}
}

type ScheduleAction string

const (
	ScheduleDelay        ScheduleAction = "delay"
	ScheduleImmediate    ScheduleAction = "immediate"
	ScheduleKeepExisting ScheduleAction = "keepExisting"
)

type SchedulePlan struct {
	SessionID  string
	Action     ScheduleAction
	Delay      time.Duration
	Expiry     time.Time
	Generation int
	Reason     string
}

func ScheduleFromJWT(sessionID, token string, now time.Time, buffer time.Duration) SchedulePlan {
	expirySeconds, ok := DecodeJWTExpiry(token)
	if !ok {
		return keepExistingPlan(sessionID)
	}
	return scheduleFromExpiry(sessionID, expirySeconds, now, buffer, 0)
}

func ScheduleFromExpiresIn(sessionID string, expiresInSeconds int64, buffer time.Duration) SchedulePlan {
	return scheduleFromExpiresIn(sessionID, expiresInSeconds, buffer, 0)
}

type RefreshPlanner struct {
	generations   map[string]int
	failureCounts map[string]int
}

func NewRefreshPlanner() *RefreshPlanner {
	return &RefreshPlanner{
		generations:   make(map[string]int),
		failureCounts: make(map[string]int),
	}
}

func (p *RefreshPlanner) ScheduleFromJWT(sessionID, token string, now time.Time, buffer time.Duration) SchedulePlan {
	expirySeconds, ok := DecodeJWTExpiry(token)
	if !ok {
		return keepExistingPlan(sessionID)
	}
	return scheduleFromExpiry(sessionID, expirySeconds, now, buffer, p.nextGeneration(sessionID))
}

func (p *RefreshPlanner) ScheduleFromExpiresIn(sessionID string, expiresInSeconds int64, buffer time.Duration) SchedulePlan {
	return scheduleFromExpiresIn(sessionID, expiresInSeconds, buffer, p.nextGeneration(sessionID))
}

func (p *RefreshPlanner) Cancel(sessionID string) int {
	gen := p.nextGeneration(sessionID)
	delete(p.failureCounts, sessionID)
	return gen
}

func (p *RefreshPlanner) Generation(sessionID string) int {
	p.ensureMaps()
	return p.generations[sessionID]
}

func (p *RefreshPlanner) IsStaleGeneration(sessionID string, generation int) bool {
	return p.Generation(sessionID) != generation
}

type RefreshFailureDecision struct {
	SessionID string
	Failures  int
	Retry     bool
	GiveUp    bool
}

func (p *RefreshPlanner) RecordRefreshFailure(sessionID string) RefreshFailureDecision {
	p.ensureMaps()

	failures := p.failureCounts[sessionID] + 1
	if failures > MaxRefreshFailures {
		failures = MaxRefreshFailures
	}
	p.failureCounts[sessionID] = failures

	return RefreshFailureDecision{
		SessionID: sessionID,
		Failures:  failures,
		Retry:     failures < MaxRefreshFailures,
		GiveUp:    failures >= MaxRefreshFailures,
	}
}

func (p *RefreshPlanner) RecordRefreshSuccess(sessionID string) {
	p.ensureMaps()
	delete(p.failureCounts, sessionID)
}

func (p *RefreshPlanner) FailureCount(sessionID string) int {
	p.ensureMaps()
	return p.failureCounts[sessionID]
}

func (p *RefreshPlanner) nextGeneration(sessionID string) int {
	p.ensureMaps()
	next := p.generations[sessionID] + 1
	p.generations[sessionID] = next
	return next
}

func (p *RefreshPlanner) ensureMaps() {
	if p.generations == nil {
		p.generations = make(map[string]int)
	}
	if p.failureCounts == nil {
		p.failureCounts = make(map[string]int)
	}
}

func keepExistingPlan(sessionID string) SchedulePlan {
	return SchedulePlan{
		SessionID: sessionID,
		Action:    ScheduleKeepExisting,
		Reason:    "undecodable_jwt",
	}
}

func scheduleFromExpiry(sessionID string, expirySeconds int64, now time.Time, buffer time.Duration, generation int) SchedulePlan {
	expiry := time.Unix(expirySeconds, 0).UTC()
	delay := expiry.Sub(now) - buffer
	if delay <= 0 {
		return SchedulePlan{
			SessionID:  sessionID,
			Action:     ScheduleImmediate,
			Expiry:     expiry,
			Generation: generation,
		}
	}
	return SchedulePlan{
		SessionID:  sessionID,
		Action:     ScheduleDelay,
		Delay:      delay,
		Expiry:     expiry,
		Generation: generation,
	}
}

func scheduleFromExpiresIn(sessionID string, expiresInSeconds int64, buffer time.Duration, generation int) SchedulePlan {
	delay := time.Duration(expiresInSeconds)*time.Second - buffer
	if delay < MinRefreshDelay {
		delay = MinRefreshDelay
	}
	return SchedulePlan{
		SessionID:  sessionID,
		Action:     ScheduleDelay,
		Delay:      delay,
		Generation: generation,
	}
}

func ToCompatSessionID(id string) string {
	return ToCompatSessionIDWithShim(id, true)
}

func ToCompatSessionIDWithShim(id string, shimEnabled bool) string {
	if !shimEnabled || !strings.HasPrefix(id, "cse_") {
		return id
	}
	return "session_" + strings.TrimPrefix(id, "cse_")
}

func ToInfraSessionID(id string) string {
	if !strings.HasPrefix(id, "session_") {
		return id
	}
	return "cse_" + strings.TrimPrefix(id, "session_")
}
