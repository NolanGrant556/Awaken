package remoteauth

import (
	"reflect"
	"testing"
	"time"
)

func TestGenerateCodeSuccess(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH")))

	result := store.GenerateCode("dingtalk", "user-1", "Nolan", now)
	if result.Status != GenerateCodeCreated {
		t.Fatalf("expected created status, got %v", result.Status)
	}
	if result.Code != "ABCDEFGH" {
		t.Fatalf("expected generated code, got %q", result.Code)
	}
	if result.ExpiresAt != now.Add(CodeTTL) {
		t.Fatalf("expected expiry %v, got %v", now.Add(CodeTTL), result.ExpiresAt)
	}

	pending := store.ListPending("dingtalk", now)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending code, got %d", len(pending))
	}
	if pending[0].Code != "ABCDEFGH" || pending[0].UserID != "user-1" || pending[0].UserName != "Nolan" {
		t.Fatalf("unexpected pending entry: %+v", pending[0])
	}
	if store.IsApproved("dingtalk", "user-1") {
		t.Fatalf("user should not be approved before code approval")
	}
}

func TestGenerateCodeRateLimit(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH", "JKLMNPQR")))

	first := store.GenerateCode("slack", "user-1", "Nolan", now)
	if first.Status != GenerateCodeCreated {
		t.Fatalf("expected first code to be created, got %v", first.Status)
	}

	limited := store.GenerateCode("slack", "user-1", "Nolan", now.Add(RateLimitWindow-time.Nanosecond))
	if limited.Status != GenerateCodeRateLimited {
		t.Fatalf("expected rate limited status, got %v", limited.Status)
	}
	if limited.RetryAt != now.Add(RateLimitWindow) {
		t.Fatalf("expected retry at %v, got %v", now.Add(RateLimitWindow), limited.RetryAt)
	}

	allowed := store.GenerateCode("slack", "user-1", "Nolan", now.Add(RateLimitWindow))
	if allowed.Status != GenerateCodeCreated {
		t.Fatalf("expected generation after rate limit window, got %v", allowed.Status)
	}
	if allowed.Code != "JKLMNPQR" {
		t.Fatalf("expected second generated code, got %q", allowed.Code)
	}
}

func TestGenerateCodeMaxPendingPerPlatform(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH", "JKLMNPQR", "STUVWXYZ", "23456789")))

	for i, userID := range []string{"user-1", "user-2", "user-3"} {
		result := store.GenerateCode("wechat", userID, "", now.Add(time.Duration(i)*time.Minute))
		if result.Status != GenerateCodeCreated {
			t.Fatalf("expected pending code %d to be created, got %v", i+1, result.Status)
		}
	}

	blocked := store.GenerateCode("wechat", "user-4", "", now.Add(4*time.Minute))
	if blocked.Status != GenerateCodeMaxPending {
		t.Fatalf("expected max pending status, got %v", blocked.Status)
	}
	if blocked.PendingCount != MaxPendingPerPlatform {
		t.Fatalf("expected pending count %d, got %d", MaxPendingPerPlatform, blocked.PendingCount)
	}
}

func TestExpiredPendingCodesAreCleanedUp(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH", "JKLMNPQR", "STUVWXYZ", "23456789")))

	for i, userID := range []string{"user-1", "user-2", "user-3"} {
		result := store.GenerateCode("discord", userID, "", now.Add(time.Duration(i)*time.Minute))
		if result.Status != GenerateCodeCreated {
			t.Fatalf("expected code %d to be created, got %v", i+1, result.Status)
		}
	}

	later := now.Add(CodeTTL + 3*time.Minute)
	if pending := store.ListPending("discord", later); len(pending) != 0 {
		t.Fatalf("expected expired pending codes to be cleaned up, got %+v", pending)
	}

	result := store.GenerateCode("discord", "user-4", "", later)
	if result.Status != GenerateCodeCreated {
		t.Fatalf("expected new code after cleanup, got %v", result.Status)
	}
	if result.Code != "23456789" {
		t.Fatalf("expected next generated code, got %q", result.Code)
	}
}

func TestApproveCodeSuccessAndCodeNormalization(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH")))

	generated := store.GenerateCode("dingtalk", "user-1", "Nolan", now)
	if generated.Status != GenerateCodeCreated {
		t.Fatalf("expected generated code, got %v", generated.Status)
	}

	approved := store.ApproveCode("dingtalk", "  abcdefgh  ", now.Add(time.Minute))
	if approved.Status != ApproveCodeApproved {
		t.Fatalf("expected approved status, got %v", approved.Status)
	}
	if approved.User.Platform != "dingtalk" || approved.User.UserID != "user-1" || approved.User.UserName != "Nolan" {
		t.Fatalf("unexpected approved user: %+v", approved.User)
	}
	if !store.IsApproved("dingtalk", "user-1") {
		t.Fatalf("expected approved user lookup to succeed")
	}
	if pending := store.ListPending("dingtalk", now.Add(time.Minute)); len(pending) != 0 {
		t.Fatalf("expected pending code to be removed after approval, got %+v", pending)
	}
}

func TestInvalidCodeFailureCountAndLockout(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH")))

	for i := 1; i < MaxFailedAttempts; i++ {
		result := store.ApproveCode("telegram", "BADCODE1", now.Add(time.Duration(i)*time.Minute))
		if result.Status != ApproveCodeInvalid {
			t.Fatalf("expected invalid status on attempt %d, got %v", i, result.Status)
		}
		if result.FailedAttempts != i {
			t.Fatalf("expected failure count %d, got %d", i, result.FailedAttempts)
		}
	}

	lockedAt := now.Add(MaxFailedAttempts * time.Minute)
	locked := store.ApproveCode("telegram", "BADCODE1", lockedAt)
	if locked.Status != ApproveCodeLockedOut {
		t.Fatalf("expected lockout on fifth failed attempt, got %v", locked.Status)
	}
	if locked.FailedAttempts != MaxFailedAttempts {
		t.Fatalf("expected failure count %d, got %d", MaxFailedAttempts, locked.FailedAttempts)
	}
	if locked.LockoutUntil != lockedAt.Add(LockoutDuration) {
		t.Fatalf("expected lockout until %v, got %v", lockedAt.Add(LockoutDuration), locked.LockoutUntil)
	}

	blocked := store.GenerateCode("telegram", "user-1", "", lockedAt.Add(time.Minute))
	if blocked.Status != GenerateCodeLockedOut {
		t.Fatalf("expected generation to be blocked during lockout, got %v", blocked.Status)
	}

	allowed := store.GenerateCode("telegram", "user-1", "", locked.LockoutUntil)
	if allowed.Status != GenerateCodeCreated {
		t.Fatalf("expected generation after lockout, got %v", allowed.Status)
	}
}

func TestApproveExpiredCode(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH")))

	generated := store.GenerateCode("slack", "user-1", "Nolan", now)
	if generated.Status != GenerateCodeCreated {
		t.Fatalf("expected generated code, got %v", generated.Status)
	}

	result := store.ApproveCode("slack", "ABCDEFGH", now.Add(CodeTTL))
	if result.Status != ApproveCodeExpired {
		t.Fatalf("expected expired status, got %v", result.Status)
	}
	if result.FailedAttempts != 1 {
		t.Fatalf("expected expired approval to count as a failed attempt, got %d", result.FailedAttempts)
	}
	if pending := store.ListPending("slack", now.Add(CodeTTL)); len(pending) != 0 {
		t.Fatalf("expected expired code to be removed, got %+v", pending)
	}
}

func TestRevoke(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH")))

	store.GenerateCode("wechat", "user-1", "Nolan", now)
	store.ApproveCode("wechat", "ABCDEFGH", now.Add(time.Minute))

	revoked := store.Revoke("wechat", "user-1")
	if revoked.Status != RevokeRevoked {
		t.Fatalf("expected revoked status, got %v", revoked.Status)
	}
	if revoked.User.UserID != "user-1" {
		t.Fatalf("unexpected revoked user: %+v", revoked.User)
	}
	if store.IsApproved("wechat", "user-1") {
		t.Fatalf("expected user to no longer be approved")
	}

	missing := store.Revoke("wechat", "user-1")
	if missing.Status != RevokeNotFound {
		t.Fatalf("expected not found on second revoke, got %v", missing.Status)
	}
}

func TestListPendingAndApproved(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH", "JKLMNPQR", "STUVWXYZ")))

	store.GenerateCode("wechat", "user-2", "B", now.Add(2*time.Minute))
	store.GenerateCode("dingtalk", "user-1", "A", now)
	store.GenerateCode("wechat", "user-3", "C", now.Add(3*time.Minute))
	store.ApproveCode("wechat", "ABCDEFGH", now.Add(4*time.Minute))

	pending := store.ListPending("", now.Add(5*time.Minute))
	gotPending := fieldsOf(pending, func(item PendingCode) string {
		return item.Platform + ":" + item.UserID + ":" + item.Code
	})
	wantPending := []string{
		"dingtalk:user-1:JKLMNPQR",
		"wechat:user-3:STUVWXYZ",
	}
	if !reflect.DeepEqual(gotPending, wantPending) {
		t.Fatalf("unexpected pending list: got %v want %v", gotPending, wantPending)
	}

	approved := store.ListApproved("")
	gotApproved := fieldsOf(approved, func(item ApprovedUser) string {
		return item.Platform + ":" + item.UserID + ":" + item.UserName
	})
	wantApproved := []string{"wechat:user-2:B"}
	if !reflect.DeepEqual(gotApproved, wantApproved) {
		t.Fatalf("unexpected approved list: got %v want %v", gotApproved, wantApproved)
	}
}

func TestClearPending(t *testing.T) {
	now := testTime()
	store := NewStore(WithCodeGenerator(sequenceGenerator(t, "ABCDEFGH", "JKLMNPQR")))

	store.GenerateCode("wechat", "user-1", "", now)
	store.GenerateCode("dingtalk", "user-2", "", now)

	result := store.ClearPending("wechat")
	if result.Status != ClearPendingCleared {
		t.Fatalf("expected clear pending status, got %v", result.Status)
	}
	if result.Count != 1 {
		t.Fatalf("expected to clear 1 pending code, got %d", result.Count)
	}
	if pending := store.ListPending("", now); len(pending) != 1 || pending[0].Platform != "dingtalk" {
		t.Fatalf("expected only dingtalk pending code to remain, got %+v", pending)
	}

	result = store.ClearPending("")
	if result.Count != 1 {
		t.Fatalf("expected to clear remaining pending code, got %d", result.Count)
	}
	if pending := store.ListPending("", now); len(pending) != 0 {
		t.Fatalf("expected no pending codes after global clear, got %+v", pending)
	}
}

func testTime() time.Time {
	return time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
}

func sequenceGenerator(t *testing.T, codes ...string) CodeGenerator {
	t.Helper()
	index := 0
	return func() (string, error) {
		t.Helper()
		if index >= len(codes) {
			t.Fatalf("code generator called more than %d times", len(codes))
		}
		code := codes[index]
		index++
		return code, nil
	}
}

func fieldsOf[T any](items []T, field func(T) string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, field(item))
	}
	return result
}
