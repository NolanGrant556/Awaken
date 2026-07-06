package profile

import "context"

// envPolicyKey is the context key used to carry an EnvPolicy through the
// call stack. It is an unexported struct type so it never collides with
// keys defined elsewhere in the runtime.
type envPolicyKey struct{}

// WithEnvPolicy returns a new context that carries the given EnvPolicy.
// Executors inject the profile's EnvPolicy into the context before
// delegating to the tool, and tools that spawn child processes (e.g.
// tools/shell.ShellExecutor) retrieve it via EnvPolicyFromContext to
// decide which environment variables to expose to the child.
func WithEnvPolicy(ctx context.Context, policy EnvPolicy) context.Context {
	return context.WithValue(ctx, envPolicyKey{}, policy)
}

// EnvPolicyFromContext returns the EnvPolicy carried by ctx. When no
// EnvPolicy has been set (e.g. a tool invoked outside the executor layer),
// it returns EnvInherit — the runtime's default behavior of inheriting
// the full parent environment — so callers can treat the absence of a
// policy as "inherit" without an extra nil check.
func EnvPolicyFromContext(ctx context.Context) EnvPolicy {
	if v, ok := ctx.Value(envPolicyKey{}).(EnvPolicy); ok {
		return v
	}
	return EnvInherit
}
