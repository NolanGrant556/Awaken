package remoteauth

import (
	"crypto/rand"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	Alphabet              = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	CodeLength            = 8
	CodeTTL               = time.Hour
	RateLimitWindow       = 10 * time.Minute
	LockoutDuration       = time.Hour
	MaxPendingPerPlatform = 3
	MaxFailedAttempts     = 5
)

type CodeGenerator func() (string, error)

type Option func(*Store)

func WithCodeGenerator(generator CodeGenerator) Option {
	return func(s *Store) {
		s.codeGenerator = generator
	}
}

type Store struct {
	mu sync.Mutex

	codeGenerator CodeGenerator
	pending       map[string]map[string]pendingEntry
	approved      map[string]map[string]approvedEntry
	rateLimits    map[userKey]time.Time
	failureCounts map[string]int
	lockouts      map[string]time.Time
}

func NewStore(options ...Option) *Store {
	store := &Store{
		codeGenerator: DefaultCodeGenerator,
	}
	for _, option := range options {
		option(store)
	}
	return store
}

type GenerateCodeStatus int

const (
	GenerateCodeCreated GenerateCodeStatus = iota
	GenerateCodeInvalidInput
	GenerateCodeAlreadyApproved
	GenerateCodeRateLimited
	GenerateCodeMaxPending
	GenerateCodeLockedOut
	GenerateCodeGeneratorError
	GenerateCodeCollision
)

type GenerateCodeResult struct {
	Status       GenerateCodeStatus
	Code         string
	ExpiresAt    time.Time
	RetryAt      time.Time
	LockoutUntil time.Time
	PendingCount int
}

type ApproveCodeStatus int

const (
	ApproveCodeApproved ApproveCodeStatus = iota
	ApproveCodeInvalidInput
	ApproveCodeInvalid
	ApproveCodeExpired
	ApproveCodeLockedOut
)

type ApproveCodeResult struct {
	Status         ApproveCodeStatus
	User           ApprovedUser
	FailedAttempts int
	LockoutUntil   time.Time
}

type RevokeStatus int

const (
	RevokeRevoked RevokeStatus = iota
	RevokeInvalidInput
	RevokeNotFound
)

type RevokeResult struct {
	Status RevokeStatus
	User   ApprovedUser
}

type ClearPendingStatus int

const (
	ClearPendingCleared ClearPendingStatus = iota
)

type ClearPendingResult struct {
	Status ClearPendingStatus
	Count  int
}

type PendingCode struct {
	Platform  string
	Code      string
	UserID    string
	UserName  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type ApprovedUser struct {
	Platform   string
	UserID     string
	UserName   string
	ApprovedAt time.Time
}

type pendingEntry struct {
	userID    string
	userName  string
	createdAt time.Time
}

type approvedEntry struct {
	userName   string
	approvedAt time.Time
}

type userKey struct {
	platform string
	userID   string
}

func DefaultCodeGenerator() (string, error) {
	var builder strings.Builder
	builder.Grow(CodeLength)
	max := big.NewInt(int64(len(Alphabet)))

	for i := 0; i < CodeLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(Alphabet[n.Int64()])
	}

	return builder.String(), nil
}

func (s *Store) GenerateCode(platform, userID, userName string, now time.Time) GenerateCodeResult {
	now = normalizeNow(now)
	platform = strings.TrimSpace(platform)
	userID = strings.TrimSpace(userID)
	userName = strings.TrimSpace(userName)

	if platform == "" || userID == "" {
		return GenerateCodeResult{Status: GenerateCodeInvalidInput}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLocked()
	s.cleanupExpiredLocked(platform, now)

	if until, locked := s.lockoutLocked(platform, now); locked {
		return GenerateCodeResult{Status: GenerateCodeLockedOut, LockoutUntil: until}
	}

	if _, ok := s.approved[platform][userID]; ok {
		return GenerateCodeResult{Status: GenerateCodeAlreadyApproved}
	}

	key := userKey{platform: platform, userID: userID}
	if last, ok := s.rateLimits[key]; ok && last.Add(RateLimitWindow).After(now) {
		return GenerateCodeResult{
			Status:  GenerateCodeRateLimited,
			RetryAt: last.Add(RateLimitWindow),
		}
	}

	pending := s.pending[platform]
	if len(pending) >= MaxPendingPerPlatform {
		return GenerateCodeResult{
			Status:       GenerateCodeMaxPending,
			PendingCount: len(pending),
		}
	}

	if pending == nil {
		pending = make(map[string]pendingEntry)
		s.pending[platform] = pending
	}

	for attempts := 0; attempts < 16; attempts++ {
		code, ok := s.generateCodeLocked()
		if !ok {
			return GenerateCodeResult{Status: GenerateCodeGeneratorError}
		}
		if _, exists := pending[code]; exists {
			continue
		}

		pending[code] = pendingEntry{
			userID:    userID,
			userName:  userName,
			createdAt: now,
		}
		s.rateLimits[key] = now

		return GenerateCodeResult{
			Status:       GenerateCodeCreated,
			Code:         code,
			ExpiresAt:    now.Add(CodeTTL),
			PendingCount: len(pending),
		}
	}

	return GenerateCodeResult{Status: GenerateCodeCollision}
}

func (s *Store) ApproveCode(platform, code string, now time.Time) ApproveCodeResult {
	now = normalizeNow(now)
	platform = strings.TrimSpace(platform)
	code = normalizeCode(code)

	if platform == "" || code == "" {
		return ApproveCodeResult{Status: ApproveCodeInvalidInput}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLocked()

	if until, locked := s.lockoutLocked(platform, now); locked {
		return ApproveCodeResult{Status: ApproveCodeLockedOut, LockoutUntil: until}
	}

	entry, ok := s.pending[platform][code]
	if !ok {
		failedAttempts, lockoutUntil, locked := s.recordFailedAttemptLocked(platform, now)
		if locked {
			return ApproveCodeResult{
				Status:         ApproveCodeLockedOut,
				FailedAttempts: failedAttempts,
				LockoutUntil:   lockoutUntil,
			}
		}
		return ApproveCodeResult{Status: ApproveCodeInvalid, FailedAttempts: failedAttempts}
	}

	if isExpired(entry.createdAt, now) {
		delete(s.pending[platform], code)
		s.deleteEmptyPendingPlatformLocked(platform)
		failedAttempts, lockoutUntil, locked := s.recordFailedAttemptLocked(platform, now)
		if locked {
			return ApproveCodeResult{
				Status:         ApproveCodeLockedOut,
				FailedAttempts: failedAttempts,
				LockoutUntil:   lockoutUntil,
			}
		}
		return ApproveCodeResult{Status: ApproveCodeExpired, FailedAttempts: failedAttempts}
	}

	s.cleanupExpiredLocked(platform, now)
	delete(s.pending[platform], code)
	s.deleteEmptyPendingPlatformLocked(platform)

	if s.approved[platform] == nil {
		s.approved[platform] = make(map[string]approvedEntry)
	}
	s.approved[platform][entry.userID] = approvedEntry{
		userName:   entry.userName,
		approvedAt: now,
	}

	return ApproveCodeResult{
		Status: ApproveCodeApproved,
		User: ApprovedUser{
			Platform:   platform,
			UserID:     entry.userID,
			UserName:   entry.userName,
			ApprovedAt: now,
		},
	}
}

func (s *Store) IsApproved(platform, userID string) bool {
	platform = strings.TrimSpace(platform)
	userID = strings.TrimSpace(userID)
	if platform == "" || userID == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLocked()

	_, ok := s.approved[platform][userID]
	return ok
}

func (s *Store) Revoke(platform, userID string) RevokeResult {
	platform = strings.TrimSpace(platform)
	userID = strings.TrimSpace(userID)
	if platform == "" || userID == "" {
		return RevokeResult{Status: RevokeInvalidInput}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLocked()

	entry, ok := s.approved[platform][userID]
	if !ok {
		return RevokeResult{Status: RevokeNotFound}
	}

	delete(s.approved[platform], userID)
	if len(s.approved[platform]) == 0 {
		delete(s.approved, platform)
	}

	return RevokeResult{
		Status: RevokeRevoked,
		User: ApprovedUser{
			Platform:   platform,
			UserID:     userID,
			UserName:   entry.userName,
			ApprovedAt: entry.approvedAt,
		},
	}
}

func (s *Store) ListPending(platform string, now time.Time) []PendingCode {
	now = normalizeNow(now)
	platform = strings.TrimSpace(platform)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLocked()

	platforms := []string{platform}
	if platform == "" {
		platforms = mapKeys(s.pending)
	}

	var result []PendingCode
	for _, p := range platforms {
		s.cleanupExpiredLocked(p, now)
		for code, entry := range s.pending[p] {
			result = append(result, PendingCode{
				Platform:  p,
				Code:      code,
				UserID:    entry.userID,
				UserName:  entry.userName,
				CreatedAt: entry.createdAt,
				ExpiresAt: entry.createdAt.Add(CodeTTL),
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Platform != result[j].Platform {
			return result[i].Platform < result[j].Platform
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].Code < result[j].Code
	})

	return result
}

func (s *Store) ListApproved(platform string) []ApprovedUser {
	platform = strings.TrimSpace(platform)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLocked()

	platforms := []string{platform}
	if platform == "" {
		platforms = mapKeys(s.approved)
	}

	var result []ApprovedUser
	for _, p := range platforms {
		for userID, entry := range s.approved[p] {
			result = append(result, ApprovedUser{
				Platform:   p,
				UserID:     userID,
				UserName:   entry.userName,
				ApprovedAt: entry.approvedAt,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Platform != result[j].Platform {
			return result[i].Platform < result[j].Platform
		}
		return result[i].UserID < result[j].UserID
	})

	return result
}

func (s *Store) ClearPending(platform string) ClearPendingResult {
	platform = strings.TrimSpace(platform)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLocked()

	if platform == "" {
		count := 0
		for _, pending := range s.pending {
			count += len(pending)
		}
		s.pending = make(map[string]map[string]pendingEntry)
		return ClearPendingResult{Status: ClearPendingCleared, Count: count}
	}

	count := len(s.pending[platform])
	delete(s.pending, platform)
	return ClearPendingResult{Status: ClearPendingCleared, Count: count}
}

func (s *Store) ensureLocked() {
	if s.codeGenerator == nil {
		s.codeGenerator = DefaultCodeGenerator
	}
	if s.pending == nil {
		s.pending = make(map[string]map[string]pendingEntry)
	}
	if s.approved == nil {
		s.approved = make(map[string]map[string]approvedEntry)
	}
	if s.rateLimits == nil {
		s.rateLimits = make(map[userKey]time.Time)
	}
	if s.failureCounts == nil {
		s.failureCounts = make(map[string]int)
	}
	if s.lockouts == nil {
		s.lockouts = make(map[string]time.Time)
	}
}

func (s *Store) generateCodeLocked() (string, bool) {
	code, err := s.codeGenerator()
	if err != nil {
		return "", false
	}

	code = normalizeCode(code)
	if !isValidCode(code) {
		return "", false
	}

	return code, true
}

func (s *Store) cleanupExpiredLocked(platform string, now time.Time) {
	pending := s.pending[platform]
	for code, entry := range pending {
		if isExpired(entry.createdAt, now) {
			delete(pending, code)
		}
	}
	s.deleteEmptyPendingPlatformLocked(platform)
}

func (s *Store) deleteEmptyPendingPlatformLocked(platform string) {
	if len(s.pending[platform]) == 0 {
		delete(s.pending, platform)
	}
}

func (s *Store) lockoutLocked(platform string, now time.Time) (time.Time, bool) {
	until, ok := s.lockouts[platform]
	if !ok {
		return time.Time{}, false
	}
	if until.After(now) {
		return until, true
	}
	delete(s.lockouts, platform)
	return time.Time{}, false
}

func (s *Store) recordFailedAttemptLocked(platform string, now time.Time) (int, time.Time, bool) {
	count := s.failureCounts[platform] + 1
	if count >= MaxFailedAttempts {
		delete(s.failureCounts, platform)
		until := now.Add(LockoutDuration)
		s.lockouts[platform] = until
		return MaxFailedAttempts, until, true
	}

	s.failureCounts[platform] = count
	return count, time.Time{}, false
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func isExpired(createdAt, now time.Time) bool {
	return !createdAt.Add(CodeTTL).After(now)
}

func isValidCode(code string) bool {
	if len(code) != CodeLength {
		return false
	}
	for i := 0; i < len(code); i++ {
		if !strings.ContainsRune(Alphabet, rune(code[i])) {
			return false
		}
	}
	return true
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
