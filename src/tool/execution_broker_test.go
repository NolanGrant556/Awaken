package tool

import (
	"context"
	"testing"
)

func TestExecutionBrokerContextDistinguishesAbsentFromExplicitNil(t *testing.T) {
	if broker, configured := ExecutionBrokerFromContext(context.Background()); configured || broker != nil {
		t.Fatalf("unexpected implicit broker: %T configured=%v", broker, configured)
	}
	ctx := ContextWithExecutionBroker(context.Background(), nil)
	if broker, configured := ExecutionBrokerFromContext(ctx); !configured || broker != nil {
		t.Fatalf("explicit nil broker must be observable: %T configured=%v", broker, configured)
	}
}
