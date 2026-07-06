package executors

import (
	"context"
	"fmt"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// GuardianExecutor wraps execution of guardian / review-agent tools and of
// any tool call that lands on the GuardianReadOnly profile. Guardian agents
// must never mutate state, so the executor enforces read-only before
// delegating: if the tool does not implement ReadOnlyTool or reports
// IsReadOnly() == false, the executor denies.
//
// This executor is selected by the Router in two cases:
//   - intent.ToolKind == ToolKindReviewAgent (regardless of profile)
//   - profile is GuardianReadOnly (regardless of tool kind)
//
// The read-only check uses the tool's own IsReadOnly() declaration. Tools
// that do not implement ReadOnlyTool are treated as non-read-only and
// denied, matching the spec's "guardian/review 只做只读复核" rule.
type GuardianExecutor struct{}

// NewGuardianExecutor returns a GuardianExecutor.
func NewGuardianExecutor() *GuardianExecutor {
	return &GuardianExecutor{}
}

// Execute enforces the read-only invariant and the profile's NetworkPolicy
// before delegating to the tool.
//
// NetworkPolicy enforcement (guardian-specific):
//   - Guardian agents must run under NetworkRestricted so they can read
//     from external sources for review but cannot leak data or mutate the
//     workspace. When a tool declares OpenWorld and the profile's
//     NetworkPolicy is NOT NetworkRestricted, the call is denied — the
//     guardian profile has been misconfigured (or escalated) to allow
//     unrestricted or no network access, both of which violate the
//     guardian contract.
//   - When the tool is OpenWorld and the profile is NetworkRestricted,
//     the call is allowed: the restricted bound is enforced at the
//     network layer.
func (e *GuardianExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	if err := enforceReadOnly(intent, t); err != nil {
		return deniedOutput("guardian_non_read_only", err.Error())
	}
	if t.Definition().Annotations.OpenWorld && execProfile.NetworkPolicy != profile.NetworkRestricted {
		return deniedOutput("guardian_network_violation",
			fmt.Sprintf("guardian profile requires restricted network but tool is OpenWorld under policy %q", execProfile.NetworkPolicy))
	}
	content, err := executeWithObserver(ctx, t, input, observer)
	if err != nil {
		return ToolExecutionOutput{}, err
	}
	return ToolExecutionOutput{Content: content}, nil
}

// enforceReadOnly denies tools that are not declared read-only. A tool is
// considered read-only when it implements ReadOnlyTool and IsReadOnly()
// returns true.
func enforceReadOnly(intent profile.ExecutionIntent, t tool.Tool) error {
	readOnlyTool, ok := t.(tool.ReadOnlyTool)
	if !ok || !readOnlyTool.IsReadOnly() {
		return fmt.Errorf(
			"guardian profile requires a read-only tool; %q is not read-only",
			intent.ToolName,
		)
	}
	return nil
}
