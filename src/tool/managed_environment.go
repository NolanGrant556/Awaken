package tool

import (
	"awaken/src/managedenv"
	"context"
)

type environmentProgressKey struct{}
type environmentBindingKey struct{}

func ManagedEnvironmentOwner(ctx context.Context) managedenv.Owner {
	id, _ := ToolExecutionIdentityFromContext(ctx)
	agent := id.AgentID
	if agent == "" {
		agent = "root"
	}
	return managedenv.Owner{ThreadID: ToolThreadIDFromContext(ctx), AgentID: agent, RunID: id.RunID, TaskID: id.SubagentTaskID, Kind: "run"}
}
func ContextWithEnvironmentProgress(ctx context.Context, fn managedenv.Progress) context.Context {
	return context.WithValue(ctx, environmentProgressKey{}, fn)
}
func EnvironmentProgressFromContext(ctx context.Context) managedenv.Progress {
	fn, _ := ctx.Value(environmentProgressKey{}).(managedenv.Progress)
	return fn
}
func ContextWithEnvironmentBinding(ctx context.Context, b managedenv.Binding) context.Context {
	return context.WithValue(ctx, environmentBindingKey{}, b)
}
func EnvironmentBindingFromContext(ctx context.Context) (managedenv.Binding, bool) {
	b, ok := ctx.Value(environmentBindingKey{}).(managedenv.Binding)
	return b, ok
}

// A read-only tool may discover a ready component, but never install one.
func ManagedExecutable(ctx context.Context, name string) (managedenv.Launch, bool) {
	if b, ok := EnvironmentBindingFromContext(ctx); ok {
		if l, ok := b.Executables[name]; ok {
			return l, true
		}
	}
	if m := managedenv.FromContext(ctx); m != nil {
		return m.AcquireExecutable(name, ManagedEnvironmentOwner(ctx))
	}
	return managedenv.Launch{}, false
}

// Prepared dependencies refine execution readiness without replacing the pinned
// tool implementation or changing authentication/policy decisions.
func ManagedReadiness(ctx context.Context, report ProviderReadinessReport) ProviderReadinessReport {
	if report.Readiness != ProviderNeedsRuntime || len(report.Dependencies) == 0 {
		return report
	}
	for _, observation := range report.Dependencies {
		if observation.Status == DependencyReady || observation.Requirement.Optional {
			continue
		}
		if observation.Requirement.Kind != DependencyOSExecutable {
			return report
		}
		if _, ok := ManagedExecutable(ctx, observation.Requirement.Name); !ok {
			return report
		}
	}
	report.Readiness = ProviderReady
	report.Reason = "managed execution dependencies are ready"
	return report
}
