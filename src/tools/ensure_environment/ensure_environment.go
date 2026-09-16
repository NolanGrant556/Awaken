package ensureenvironment

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

func capabilityNames() []any {
	catalog, err := managedenv.BuiltinCatalog()
	if err != nil {
		return nil
	}
	names := catalog.AdvertisedCapabilities()
	values := make([]any, len(names))
	for i, name := range names {
		values[i] = name
	}
	return values
}

func (*Tool) Name() string { return "ensure_environment" }
func (*Tool) Description() string {
	return "准备非办公扩展依赖，或恢复当前 Agent 的既有 environment_ref。普通 Office/PDF 和官方 creator helper 使用 load_workspace_dependencies 取得完整办公包；不要用本工具绕过固定包准备失败。"
}
func (*Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "capability", Type: "string", Description: "能力，例如 python-core、ripgrep、git、tesseract、dingtalk-workspace、lark-cli。与 requirements 二选一。"},
		{Name: "requirements", Type: "array", Description: "结构化依赖数组，每项含 ecosystem（component/python/node）、name、可选 version。不能指定 URL、目录或命令。"},
		{Name: "environment_ref", Type: "string", Description: "重新取得当前 Agent 以前返回的绑定；单独提供时只验证并恢复该绑定。"},
		{Name: "purpose", Type: "string", Description: "简短用途说明。"},
	}
}
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{Name: t.Name(), DisplayName: "准备运行环境", Description: t.Description(), Purpose: "复用用户级依赖，按需准备后让当前任务继续。", WhenToUse: []string{"非办公扩展、浏览器、OCR、CLI 等需要受管依赖", "恢复丢失的环境路径"}, WhenNotToUse: []string{"用户软件项目自身的构建依赖", "账号授权问题"}, SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectCreate, Resources: []string{"Awaken managed environments"}, Description: "下载并准备缺失的用户级运行组件。", Reversibility: tool.ReversibilityConditional}}, Examples: []tool.ToolExample{{Scenario: "准备 OCR 扩展", Input: map[string]any{"capability": "tesseract"}, Outcome: "返回 ready 和 environment_ref，后续 Bash 显式使用该引用。"}}, FailureModes: []tool.ToolFailureMode{{Code: "environment_platform_unsupported", Cause: "平台组件尚未提供", Recovery: []string{"选择支持的路线"}}, {Code: "environment_download_failed", Cause: "下载失败", Retryable: true, Recovery: []string{"恢复网络后重试"}}}, Annotations: tool.ToolAnnotations{ReadOnly: false, OpenWorld: true, CapabilityClass: tool.CapabilitySystemOperation, TrustedOrigin: tool.TrustedOriginBuiltin}, Permissions: tool.ToolPermissionSpec{WorkspaceScope: tool.WorkspaceScopeNone, FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeOpen, CapabilityClass: tool.CapabilitySystemOperation}, ProviderID: tool.ProviderID("builtin:ensure_environment"), Capabilities: []tool.Capability{tool.CapabilityDependencyInstall}, InputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{
		"capability": {Type: "string", Description: "Select an exact capability ID from this list, not a Skill name. Office tasks use load_workspace_dependencies for the complete bundle. Choose this or structured requirements.", Enum: capabilityNames()}, "environment_ref": {Type: "string", Description: "Existing reference owned by this Agent."}, "purpose": {Type: "string", Description: "Short task purpose."}, "requirements": {Type: "array", Description: "Packages or host components to prepare.", Items: &tool.ToolParam{Type: "object", Description: "One dependency requirement.", Properties: map[string]tool.ToolParam{"ecosystem": {Type: "string", Description: "Dependency ecosystem.", Enum: []any{"component", "python", "node"}}, "name": {Type: "string", Description: "Registered component or package name."}, "version": {Type: "string", Description: "Optional numeric version constraint."}}, Required: []string{"ecosystem", "name"}, AdditionalProperties: tool.BoolPtr(false)}},
	}, AdditionalProperties: tool.BoolPtr(false)}, OutputSchema: tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}})
}
func parse(raw string) (managedenv.Request, error) {
	var p managedenv.Request
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&p); e != nil {
		return p, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return p, errors.New("only one JSON object is allowed")
	}
	if len(p.Purpose) > 500 {
		return p, errors.New("purpose is too long")
	}
	return p, nil
}
func (*Tool) CheckPermission(ctx context.Context, raw string, _ tool.PermissionContext) tool.PermissionDecision {
	if e := ctx.Err(); e != nil {
		return tool.DenyPermission(e.Error())
	}
	if _, e := parse(raw); e != nil {
		return tool.DenyPermission(e.Error())
	}
	return tool.AllowPermission()
}
func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	o, e := t.ExecuteOutcome(ctx, raw, nil)
	if e != nil {
		return o.Content, e
	}
	if o.IsError() {
		return o.Content, tool.ToolError{Code: o.ErrorCode, Category: o.ErrorCategory, Message: o.ErrorMessage, Retryable: o.Retryable, RecoveryOptions: o.RecoveryOptions}
	}
	return o.Content, nil
}
func (*Tool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	start := time.Now()
	p, e := parse(raw)
	if e != nil {
		return tool.FailedOutcome(e.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false), nil
	}
	m := managedenv.FromContext(ctx)
	if m == nil {
		return tool.FailedOutcome("共享环境管理器尚未就绪", "environment_manager_unavailable", tool.ErrorCategoryRuntime, true), nil
	}
	b, e := m.Ensure(ctx, p, tool.ManagedEnvironmentOwner(ctx), tool.EnvironmentProgressFromContext(ctx))
	if e != nil {
		code := "environment_prepare_failed"
		var typed *managedenv.Error
		if errors.As(e, &typed) {
			code = typed.Code
		}
		retryable := code == "environment_download_failed" || code == "environment_disk_full" || code == "environment_permission_denied" || code == "environment_validation_failed" || code == "environment_binding_stale" || code == "environment_removing"
		outcome := tool.FailedOutcome(e.Error(), code, tool.ErrorCategoryRuntime, retryable).WithTiming(start, time.Now())
		if retryable {
			var hint map[string]any
			_ = json.Unmarshal([]byte(raw), &hint)
			outcome.NextActions = []tool.NextAction{{ID: "retry_environment", Description: "先处理本次报告的原因，再重试同一共享依赖请求；不要改为项目安装或连续重复安装。", Tool: "ensure_environment", InputHint: hint}}
		}
		return outcome, nil
	}
	data, _ := json.Marshal(b)
	o := tool.SucceededOutcome(string(data)).WithTiming(start, time.Now())
	o.Summary = "运行环境已就绪"
	o.Details = map[string]any{"environment_ref": b.EnvironmentRef, "instance_id": b.InstanceID, "reused": b.Reused}
	o.NextActions = []tool.NextAction{{ID: "execute_with_environment", Description: "每次 Bash 都传入此 environment_ref，包括使用绝对解释器路径时；Node 模块、字体和原生工具路径依赖此绑定。", Tool: "Bash", InputHint: map[string]any{"environment_ref": b.EnvironmentRef}}}
	return o, nil
}
func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
