package loadworkspacedependencies

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"awaken/src/managedenv"
	"awaken/src/tool"
)

type Tool struct{}

func (*Tool) Name() string { return "load_workspace_dependencies" }
func (*Tool) Description() string {
	return "取得固定办公包的 Python、Node、文档库、渲染工具与准确路径；环境缺失或损坏时自动准备可用的已发布包，跨项目复用。无需指定包列表或安装位置。environment_ref 只属于当前 Agent；其他项目或 Agent 调用本工具取得各自引用，底层办公包仍可复用。"
}
func (*Tool) Parameters() []tool.ToolParameter { return nil }
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: t.Name(), DisplayName: "准备办公环境", Description: t.Description(), Purpose: "为普通 Word、Excel、PPT、PDF 任务取得一套完整的受管办公环境。",
		WhenToUse: []string{"执行常用办公 Skill 前取得解释器、库和渲染路径"}, WhenNotToUse: []string{"用户软件项目依赖或外部账号授权"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectCreate, Resources: []string{"Awaken managed office bundle"}, Description: "首次使用可下载并部署固定办公包；不修改业务项目或系统安装。", Reversibility: tool.ReversibilityConditional}},
		Examples:     []tool.ToolExample{{Scenario: "准备 Word 生成和排版检查", Input: map[string]any{}, Outcome: "返回整包版本、路径与 environment_ref，Bash 使用此绑定。"}},
		FailureModes: []tool.ToolFailureMode{{Code: "environment_source_unavailable", Cause: "办公包未发布或当前平台没有有效资源", Recovery: []string{"等待产品资源就绪，不改用系统软件冒充整包"}}},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, OpenWorld: true, CapabilityClass: tool.CapabilitySystemOperation, TrustedOrigin: tool.TrustedOriginBuiltin},
		Permissions:  tool.ToolPermissionSpec{WorkspaceScope: tool.WorkspaceScopeNone, FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeOpen, CapabilityClass: tool.CapabilitySystemOperation},
		ProviderID:   tool.ProviderID("builtin:load_workspace_dependencies"), Capabilities: []tool.Capability{tool.CapabilityDependencyInstall},
		InputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{}, AdditionalProperties: tool.BoolPtr(false)}, OutputSchema: tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)},
	})
}
func validate(raw string) error {
	var input map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	if input == nil || len(input) != 0 {
		return errors.New("provide an empty JSON object; installation options are host-owned")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("only one JSON object is allowed")
	}
	return nil
}
func (*Tool) CheckPermission(ctx context.Context, raw string, _ tool.PermissionContext) tool.PermissionDecision {
	if err := ctx.Err(); err != nil {
		return tool.DenyPermission(err.Error())
	}
	if err := validate(raw); err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermission()
}
func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	out, err := t.ExecuteOutcome(ctx, raw, nil)
	if err != nil {
		return out.Content, err
	}
	if out.IsError() {
		return out.Content, tool.ToolError{Code: out.ErrorCode, Category: out.ErrorCategory, Message: out.ErrorMessage, Retryable: out.Retryable}
	}
	return out.Content, nil
}
func (*Tool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now()
	if err := validate(raw); err != nil {
		return tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false), nil
	}
	m := managedenv.FromContext(ctx)
	if m == nil {
		return tool.FailedOutcome(officePreparationFailure("共享环境管理器尚未就绪"), "environment_manager_unavailable", tool.ErrorCategoryRuntime, true), nil
	}
	binding, err := m.EnsureOfficeBundle(ctx, tool.ManagedEnvironmentOwner(ctx), tool.EnvironmentProgressFromContext(ctx))
	if err != nil {
		code := "environment_prepare_failed"
		var typed *managedenv.Error
		if errors.As(err, &typed) {
			code = typed.Code
		}
		retryable := code == "environment_download_failed" || code == "environment_disk_full" || code == "environment_permission_denied" || code == "environment_binding_stale"
		out := tool.FailedOutcome(officePreparationFailure(err.Error()), code, tool.ErrorCategoryRuntime, retryable).WithTiming(started, time.Now())
		if retryable {
			out.NextActions = []tool.NextAction{{ID: "retry_office_bundle", Description: "处理报告的原因后重试固定办公包准备。", Tool: "load_workspace_dependencies", InputHint: map[string]any{}}}
		}
		return out, nil
	}
	data, _ := json.Marshal(binding)
	out := tool.SucceededOutcome(string(data)).WithTiming(started, time.Now())
	out.Summary = "固定办公环境已就绪"
	out.Details = map[string]any{"bundle_version": binding.BundleVersion, "environment_ref": binding.EnvironmentRef, "instance_id": binding.InstanceID, "reused": binding.Reused}
	out.NextActions = []tool.NextAction{{ID: "execute_with_office_bundle", Description: "使用返回的解释器和启动器，Bash 传入此 environment_ref。", Tool: "Bash", InputHint: map[string]any{"environment_ref": binding.EnvironmentRef}}}
	return out, nil
}

// A failure is a task blocker, not permission to choose a different runtime.
func officePreparationFailure(cause string) string {
	return cause + "\n办公环境尚未就绪，本次没有返回可执行的 environment_ref。请向用户说明以上原因并暂停依赖此环境的办公步骤；问题解决后重新调用本工具继续。不要探测或改用系统 Python、Node、LibreOffice，也不要另行安装办公依赖。"
}
func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
