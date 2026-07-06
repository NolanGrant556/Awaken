package profile

import (
	"context"
	"testing"
)

// TestWithEnvPolicyAndEnvPolicyFromContext verifies that WithEnvPolicy
// stores the EnvPolicy in the context and EnvPolicyFromContext retrieves
// it correctly.
func TestWithEnvPolicyAndEnvPolicyFromContext(t *testing.T) {
	ctx := context.Background()

	// Default when no policy is set: EnvInherit.
	if got := EnvPolicyFromContext(ctx); got != EnvInherit {
		t.Fatalf("expected default EnvInherit, got %q", got)
	}

	// EnvMinimal round-trips through the context.
	ctxMinimal := WithEnvPolicy(ctx, EnvMinimal)
	if got := EnvPolicyFromContext(ctxMinimal); got != EnvMinimal {
		t.Fatalf("expected EnvMinimal, got %q", got)
	}

	// EnvInherit round-trips through the context.
	ctxInherit := WithEnvPolicy(ctx, EnvInherit)
	if got := EnvPolicyFromContext(ctxInherit); got != EnvInherit {
		t.Fatalf("expected EnvInherit, got %q", got)
	}

	// Original context is unaffected by the derived context.
	if got := EnvPolicyFromContext(ctx); got != EnvInherit {
		t.Fatalf("original context should still default to EnvInherit, got %q", got)
	}
}
