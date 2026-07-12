package tool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
)

var (
	ErrMassMessageApprovalMissing  = errors.New("mass-message approval grant is missing")
	ErrMassMessageApprovalInvalid  = errors.New("mass-message approval grant is invalid")
	ErrMassMessageApprovalMismatch = errors.New("mass-message approval grant does not match this execution")
)

// ToolExecutionIdentity binds an approval to the exact host-owned invocation.
// It is intentionally not derived from model-provided tool arguments.
type ToolExecutionIdentity struct {
	RunID      string
	ToolCallID string
	ToolName   string
}

type toolExecutionIdentityContextKey struct{}

func ContextWithToolExecutionIdentity(ctx context.Context, identity ToolExecutionIdentity) context.Context {
	return context.WithValue(ctx, toolExecutionIdentityContextKey{}, identity)
}

func ToolExecutionIdentityFromContext(ctx context.Context) (ToolExecutionIdentity, bool) {
	if ctx == nil {
		return ToolExecutionIdentity{}, false
	}
	identity, ok := ctx.Value(toolExecutionIdentityContextKey{}).(ToolExecutionIdentity)
	return identity, ok && validToolExecutionIdentity(identity)
}

// MassMessageApprovalBinding is the complete approval subject. All fields are
// required; partial or compatibility grants are rejected fail-closed.
type MassMessageApprovalBinding struct {
	RunID                   string
	ToolCallID              string
	ToolName                string
	ProviderID              ProviderID
	EnvelopeDigest          string
	BodyDigest              string
	RecipientIdentityDigest string
}

type massMessageApprovalContextKey struct{}

type massMessageApprovalContext struct {
	token string
}

var massMessageApprovalGrants sync.Map

// IssueMassMessageApprovalGrant creates an opaque, process-local, one-shot
// capability. Trusted ToolRuntime code calls this only after durable approval.
func IssueMassMessageApprovalGrant(ctx context.Context, binding MassMessageApprovalBinding) (context.Context, func(), error) {
	binding = normalizeMassMessageApprovalBinding(binding)
	if !validMassMessageApprovalBinding(binding) {
		return ctx, func() {}, ErrMassMessageApprovalInvalid
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return ctx, func() {}, err
	}
	token := hex.EncodeToString(random)
	massMessageApprovalGrants.Store(token, binding)
	revoke := func() { massMessageApprovalGrants.Delete(token) }
	return context.WithValue(ctx, massMessageApprovalContextKey{}, massMessageApprovalContext{token: token}), revoke, nil
}

// ConsumeMassMessageApprovalGrant atomically consumes the capability before
// any external transport begins. A mismatch also burns the token, preventing
// mutation probes and replay.
func ConsumeMassMessageApprovalGrant(ctx context.Context, expected MassMessageApprovalBinding) error {
	if ctx == nil {
		return ErrMassMessageApprovalMissing
	}
	grantContext, ok := ctx.Value(massMessageApprovalContextKey{}).(massMessageApprovalContext)
	if !ok || strings.TrimSpace(grantContext.token) == "" {
		return ErrMassMessageApprovalMissing
	}
	stored, ok := massMessageApprovalGrants.LoadAndDelete(grantContext.token)
	if !ok {
		return ErrMassMessageApprovalInvalid
	}
	actual, ok := stored.(MassMessageApprovalBinding)
	if !ok {
		return ErrMassMessageApprovalInvalid
	}
	expected = normalizeMassMessageApprovalBinding(expected)
	if !validMassMessageApprovalBinding(expected) || actual != expected {
		return ErrMassMessageApprovalMismatch
	}
	return nil
}

func normalizeMassMessageApprovalBinding(binding MassMessageApprovalBinding) MassMessageApprovalBinding {
	binding.RunID = strings.TrimSpace(binding.RunID)
	binding.ToolCallID = strings.TrimSpace(binding.ToolCallID)
	binding.ToolName = strings.TrimSpace(binding.ToolName)
	binding.ProviderID = ProviderID(strings.TrimSpace(string(binding.ProviderID)))
	binding.EnvelopeDigest = strings.TrimSpace(binding.EnvelopeDigest)
	binding.BodyDigest = strings.TrimSpace(binding.BodyDigest)
	binding.RecipientIdentityDigest = strings.TrimSpace(binding.RecipientIdentityDigest)
	return binding
}

func validToolExecutionIdentity(identity ToolExecutionIdentity) bool {
	return strings.TrimSpace(identity.RunID) != "" && strings.TrimSpace(identity.ToolCallID) != "" && strings.TrimSpace(identity.ToolName) != ""
}

func validMassMessageApprovalBinding(binding MassMessageApprovalBinding) bool {
	return validToolExecutionIdentity(ToolExecutionIdentity{RunID: binding.RunID, ToolCallID: binding.ToolCallID, ToolName: binding.ToolName}) &&
		strings.TrimSpace(string(binding.ProviderID)) != "" &&
		strings.TrimSpace(binding.EnvelopeDigest) != "" &&
		strings.TrimSpace(binding.BodyDigest) != "" &&
		strings.TrimSpace(binding.RecipientIdentityDigest) != ""
}
