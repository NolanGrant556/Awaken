import "./AwakenPluginConsole.css";
import { Fragment, useMemo, useState, type ReactNode } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenDropdown } from "../AwakenDropdown";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenMarkdownRenderer } from "../AwakenMarkdownRenderer";
import {
  AwakenPluginBrandIcon,
  isAwakenPluginBrand,
} from "../AwakenPluginBrandIcon";
import { AwakenSwitch } from "../AwakenSwitch";

const SOURCE_ACTION_BUTTON_STYLE = { transition: "none" } as const;

export type AwakenPluginCompatibility =
  | "native"
  | "adapted"
  | "recognized_unsupported"
  | "ignored_by_dialect"
  | "invalid"
  | string;

export type AwakenPluginConsoleComponent = {
  logicalId: string;
  logicalIdEncoded: string;
  kind: string;
  name: string;
  description?: string;
  sourcePath?: string;
  sourceDialect: string;
  mappingState: AwakenPluginCompatibility;
  validation: string;
  desiredState: string;
  ruleActivation?: string;
  configurationState: string;
  authenticationState?: string;
  runtimeState: string;
  integrityState: string;
  switchable: boolean;
  diagnosticCount?: number;
  mcp?: { shortName?: string; transport?: string; connectionState?: string; toolsLoaded?: boolean; toolCount?: number; hostManaged?: boolean; connectorId?: string };
  rule?: { activation?: string; globs?: string[]; scope?: string };
  agent?: { model?: string; readOnly?: boolean; maxTurns?: number; timeoutSeconds?: number };
  command?: { invocationName?: string; argumentHint?: string; acceptsArguments?: boolean; argumentsRequired?: boolean };
  hook?: { sourceEvent?: string; mappedEvent?: string; matcher?: string; condition?: string; handlerType?: string; scope?: string; timeoutSeconds?: number; async?: boolean; asyncRewake?: boolean };
  variable?: { key?: string; type?: string; required?: boolean; secret?: boolean };
};

export type AwakenPluginSetupItem = {
  id: string;
  kind: string;
  title: string;
  description?: string;
  status: string;
  message?: string;
  required?: boolean;
  sourcePath?: string;
  connectorId?: string;
  actions: string[];
};

export type AwakenPluginConsoleDetail = {
  component: AwakenPluginConsoleComponent;
  preview?: string;
  source?: string;
  sourceEditable?: boolean;
  properties?: Record<string, unknown>;
  resources?: string[];
  diagnostics?: Array<{ severity?: string; code?: string; message?: string }>;
	command?: { argumentHint?: string; acceptsArguments?: boolean; argumentsRequired?: boolean };
  mcp?: {
    shortName: string;
    environment: "local_process" | "remote_endpoint";
    connectionState: string;
    errorReason?: string;
    toolsLoaded: boolean;
    tools: Array<{ name: string; description?: string; enabled: boolean }>;
	hostManaged?: boolean;
	connectorId?: string;
  };
  hook?: {
    sourceEvent: string;
    mappedEvent?: string;
    matcher?: string;
	condition?: string;
    handlerType: string;
    scope: string;
	executionMode?: "sync" | "async" | "dynamic";
	asyncRewake?: boolean;
    failurePolicy: string;
    retention?: string;
    receipts: Array<{
      id: string;
      status: string;
      action: string;
      reasonCode?: string;
      durationMs: number;
      createdAt: string;
      retryOf?: string;
    }>;
  };
};

export type AwakenPluginVariableField = {
  key: string;
  title?: string;
  description?: string;
  type: string;
  required?: boolean;
  secret?: boolean;
  configured: boolean;
  value?: unknown;
  enum?: unknown[];
  defaultValue?: unknown;
  multiple?: boolean;
  minimum?: number;
  maximum?: number;
  dependencies?: Array<{ logicalId: string; kind: string; name: string }>;
};

export type AwakenPluginConsoleFeedback = {
  message: string;
  tone: "info" | "success" | "error";
};

export type AwakenPluginConsoleProps = {
  name: string;
  displayName?: string;
  builtin?: boolean;
  brandIcon?: string;
  version?: string;
  enabled: boolean;
  components: AwakenPluginConsoleComponent[];
  setupItems?: AwakenPluginSetupItem[];
  selectedDetail?: AwakenPluginConsoleDetail | null;
  detailLoading?: boolean;
  activeDetailTab: "overview" | "preview" | "source" | "diagnostics";
  variables?: AwakenPluginVariableField[];
  variableValues?: Record<string, string>;
  loading?: boolean;
  busy?: boolean;
  errorMessage?: string | null;
  detailFeedback?: AwakenPluginConsoleFeedback | null;
  onBack: () => void;
  onEnabledChange: (enabled: boolean) => void;
  onUninstall?: () => void;
  onComponentOpen: (component: AwakenPluginConsoleComponent) => void;
  onComponentEnabledChange: (component: AwakenPluginConsoleComponent, enabled: boolean) => void;
  onSetupAction?: (item: AwakenPluginSetupItem, action: string) => void;
  onDetailTabChange: (tab: AwakenPluginConsoleProps["activeDetailTab"]) => void;
  onDetailClose: () => void;
  onMcpToolEnabledChange?: (
    detail: AwakenPluginConsoleDetail,
    toolName: string,
    enabled: boolean,
  ) => void;
  commandArguments?: string;
  onCommandArgumentsChange?: (value: string) => void;
  onRuleActivationChange?: (
    detail: AwakenPluginConsoleDetail,
    activation: "always" | "file_globs" | "manual" | "model_decides",
  ) => void;
  onHookReceiptRetry?: (detail: AwakenPluginConsoleDetail, receiptId: string) => void;
  onUseInChat?: (detail: AwakenPluginConsoleDetail, commandArguments?: string) => void;
  sourceEditing?: boolean;
  sourceDraft?: string;
  onSourceEditStart?: (detail: AwakenPluginConsoleDetail) => void;
  onSourceDraftChange?: (value: string) => void;
  onSourceEditCancel?: () => void;
  onSourceSave?: (detail: AwakenPluginConsoleDetail) => void;
  onVariableValueChange?: (key: string, value: string) => void;
  onSaveVariables?: () => void;
  onResetVariable?: (key: string) => void;
};

const GROUPS = [
  ["skill", "技能"],
  ["mcp_server", "MCP 服务"],
  ["rule", "规则"],
  ["agent", "子任务智能体"],
  ["command", "命令"],
  ["hook", "钩子"],
  ["variable", "配置变量"],
  ["app", "应用界面"],
] as const;
const GROUP_LABELS: Record<string, string> = Object.fromEntries(GROUPS);

const RUNTIME_LABELS: Record<string, string> = {
  not_applicable: "无需运行态",
  pending: "正在连接",
  connected: "已连接",
  needs_auth: "需要授权",
  failed: "运行失败",
  disabled: "已停用",
  stopped: "未启动",
  not_loaded: "尚未加载",
};

function componentSpecificSummary(component: AwakenPluginConsoleComponent): string | undefined {
  if (component.kind === "skill") return "查看技能内容";
  if (component.kind === "rule") {
    const activation = {
      always: "始终生效",
      file_globs: "按文件生效",
      manual: "手动使用",
      model_decides: "按需使用",
    }[component.rule?.activation ?? ""];
    return [activation, component.rule?.globs?.join("、")].filter(Boolean).join(" · ") || undefined;
  }
  if (component.kind === "agent") return "可在对话中作为子任务使用";
  if (component.kind === "hook") return [component.hook?.sourceEvent, component.hook?.condition || component.hook?.matcher].filter(Boolean).join(" · ") || undefined;
  if (component.kind === "mcp_server") {
    return component.mcp?.toolsLoaded ? `${component.mcp.toolCount ?? 0} 个工具` : "查看服务详情";
  }
  if (component.kind === "command") return [component.command?.invocationName, component.command?.argumentHint].filter(Boolean).join(" · ") || "可插入对话";
  if (component.kind === "variable") return component.variable?.required ? "必填配置" : "可选配置";
  return "查看组件详情";
}

function readOnlyReason(component: AwakenPluginConsoleComponent): string {
  if (component.validation === "invalid") return "修复无效的清单或源文件后才能操作";
  if (component.integrityState !== "intact") return "安装包完整性异常，运行已阻止";
  if (component.configurationState === "needs_configuration") return "完成必填配置后才能启用";
  if ((component.authenticationState ?? "not_required") !== "ready" && (component.authenticationState ?? "not_required") !== "not_required") return "完成账号连接后才能启用";
  if (component.mappingState === "recognized_unsupported") return "Awaken 当前没有安全的运行适配器";
  if (component.mappingState === "ignored_by_dialect") return "此来源方言不会加载该文件";
  if (component.kind === "variable") return "请打开详情管理配置";
  if (component.mcp?.hostManaged) return "运行和账号由 Awaken 连接器统一管理";
  return "此组件仅供查看";
}

function componentCanUseInChat(component: AwakenPluginConsoleComponent): boolean {
  return (
    component.desiredState === "enabled" &&
    component.validation !== "invalid" &&
    component.integrityState === "intact" &&
    component.configurationState !== "needs_configuration" &&
    ((component.authenticationState ?? "not_required") === "ready" || (component.authenticationState ?? "not_required") === "not_required") &&
    (component.mappingState === "native" || component.mappingState === "adapted")
  );
}

const SETUP_STATUS_LABELS: Record<string, string> = {
  ready: "已就绪",
  needs_configuration: "待配置",
  needs_auth: "待连接",
  authorizing: "等待授权",
  verifying: "正在验证",
  pending_admin_approval: "等待管理员审批",
  expired: "已过期",
  failed: "失败",
  unsupported: "当前不支持",
};

const SETUP_ACTION_LABELS: Record<string, string> = {
  configure: "配置",
  authorize: "连接账号",
  cancel: "取消",
  mark_admin_approval: "页面要求管理员审批",
  retry: "重试",
  probe: "重新检测",
  disconnect: "退出登录",
  reauthorize: "重新连接",
};

function setupActionLabel(item: AwakenPluginSetupItem, action: string): string {
  if (item.status === "pending_admin_approval" && action === "retry") {
    return "批准后重新连接";
  }
  return SETUP_ACTION_LABELS[action] ?? action;
}

function PluginSetupSection({
  items,
  displayName,
  brandIcon,
  busy,
  onAction,
}: {
  items: AwakenPluginSetupItem[];
  displayName: string;
  brandIcon?: string;
  busy?: boolean;
  onAction?: (item: AwakenPluginSetupItem, action: string) => void;
}) {
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  if (items.length === 0) return null;
  const pending = items.filter((item) => item.status !== "ready");
  if (pending.length === 0) {
    const accountItem = items.find((item) =>
      item.actions.includes("disconnect") &&
      ["host_connector", "oauth_pkce", "oauth_device", "mcp_oauth", "cli_login"].includes(item.kind),
    );
    if (!accountItem) return null;
    const accountTitle = brandIcon === "outlook" ? "Microsoft Outlook Email" : displayName;
    const accountDescription = brandIcon === "outlook"
      ? "搜索、读取和管理你的 Microsoft Outlook 邮件。"
      : accountItem.description || accountItem.message || "账号已连接，可供插件安全使用。";
    return (
      <section className="aw-plugin-account" aria-label="已连接账号">
        <header><h2>应用</h2><span>1</span></header>
        <div className="aw-plugin-account__row">
          <span className="aw-plugin-account__identity">
            <span className="aw-plugin-account__logo">
              {isAwakenPluginBrand(brandIcon) ? (
                <AwakenPluginBrandIcon brand={brandIcon} size="card" />
              ) : (
                <AwakenIcon concept="plugin-default" size={20} />
              )}
            </span>
            <span className="aw-plugin-account__copy">
              <strong>{accountTitle}</strong>
              <small>{accountDescription}</small>
            </span>
          </span>
          <AwakenDropdown
            open={accountMenuOpen}
            onOpenChange={setAccountMenuOpen}
            trigger={(
              <button
                type="button"
                className="aw-plugin-account__trigger"
                aria-label={`${accountTitle} 已连接，打开账号操作`}
                aria-haspopup="menu"
                disabled={busy || !onAction}
              >
                <span className="aw-plugin-account__status-dot" />
                已连接
                <AwakenIcon concept="dropdown" size={14} />
              </button>
            )}
          >
            {onAction ? (
              <div className="aw-plugin-account__menu" role="menu" aria-label={`${accountTitle} 账号操作`}>
                <button
                  type="button"
                  role="menuitem"
                  disabled={busy}
                  onClick={() => {
                    setAccountMenuOpen(false);
                    onAction(accountItem, "reauthorize");
                  }}
                >
                  <AwakenIcon concept="refresh" size={16} />
                  重新连接
                </button>
                <button
                  type="button"
                  role="menuitem"
                  className="is-danger"
                  disabled={busy}
                  onClick={() => {
                    setAccountMenuOpen(false);
                    onAction(accountItem, "disconnect");
                  }}
                >
                  <AwakenIcon concept="close" size={16} />
                  断开连接
                </button>
              </div>
            ) : null}
          </AwakenDropdown>
        </div>
      </section>
    );
  }
  return (
    <section className="aw-plugin-setup" aria-label="插件设置">
      <header>
        <span>
          <h2>待完成设置</h2>
          <p>{`${pending.length} 项需要处理，依赖这些设置的组件暂不可用`}</p>
        </span>
      </header>
      <div className="aw-plugin-setup__items">
        {pending.map((item) => {
          const actions = item.actions.filter((action) => action !== "inspect_only");
          return (
            <article key={item.id} className="aw-plugin-setup__item" data-status={item.status}>
              <span className="aw-plugin-setup__copy">
                <span className="aw-plugin-setup__title">
                  <strong>{item.title}</strong>
                  <em>{SETUP_STATUS_LABELS[item.status] ?? item.status}</em>
                </span>
                <small>{item.message || item.description || (item.connectorId ? `Connector ID：${item.connectorId}` : item.sourcePath)}</small>
              </span>
              {actions.length > 0 ? (
                <span className="aw-plugin-setup__actions">
                  {actions.map((action) => (
                    <AwakenButton key={action} size="sm" variant={action === "authorize" || action === "configure" ? "primary" : "default"} disabled={busy || !onAction} onClick={() => onAction?.(item, action)}>
                      {setupActionLabel(item, action)}
                    </AwakenButton>
                  ))}
                </span>
              ) : null}
            </article>
          );
        })}
      </div>
    </section>
  );
}

export function PluginComponentRow({
  component,
  parentEnabled = true,
  busy,
  onOpen,
  onEnabledChange,
}: {
  component: AwakenPluginConsoleComponent;
  parentEnabled?: boolean;
  busy?: boolean;
  onOpen: () => void;
  onEnabledChange: (enabled: boolean) => void;
}) {
  const enabled = parentEnabled && component.desiredState === "enabled";
  const typeSummary = componentSpecificSummary(component);
  return (
    <article className="aw-plugin-component-row">
      <button type="button" className="aw-plugin-component-row__main" onClick={onOpen}>
        <span className="aw-plugin-component-row__icon" aria-hidden="true">
          <AwakenIcon
            concept={component.kind === "mcp_server" ? "mcp-brand" : component.kind === "skill" ? "skill" : component.kind === "hook" ? "hook" : component.kind === "agent" ? "subagent" : "file-code"}
            size={16}
          />
        </span>
        <span className="aw-plugin-component-row__copy">
          <span className="aw-plugin-component-row__title">{component.name}</span>
          <span className="aw-plugin-component-row__description">
            {typeSummary || "查看详情"}
          </span>
        </span>
      </button>
      <div className="aw-plugin-component-row__action">
        {component.switchable ? (
          <AwakenSwitch
            checked={enabled}
            label={component.name}
            size="compact"
            busy={busy}
            disabled={!parentEnabled}
            onCheckedChange={onEnabledChange}
          />
        ) : (
          <span className="aw-plugin-component-row__readonly" title={readOnlyReason(component)} aria-hidden="true">
            <AwakenIcon concept="disclose" size={16} />
          </span>
        )}
      </div>
    </article>
  );
}

export function ComponentGroup({
  title,
  components,
  parentEnabled = true,
  busy,
  onOpen,
  onEnabledChange,
}: {
  title: string;
  components: AwakenPluginConsoleComponent[];
  parentEnabled?: boolean;
  busy?: boolean;
  onOpen: (component: AwakenPluginConsoleComponent) => void;
  onEnabledChange: (component: AwakenPluginConsoleComponent, enabled: boolean) => void;
}) {
  if (components.length === 0) return null;
  return (
    <section className="aw-plugin-component-group">
      <header>
        <h2>{title}</h2>
        <span>{components.length} 项</span>
      </header>
      <div className="aw-plugin-component-group__rows">
        {components.map((component) => (
          <PluginComponentRow
            key={component.logicalId}
            component={component}
            parentEnabled={parentEnabled}
            busy={busy}
            onOpen={() => onOpen(component)}
            onEnabledChange={(enabled) => onEnabledChange(component, enabled)}
          />
        ))}
      </div>
    </section>
  );
}

const PROPERTY_VALUE_LABELS: Record<string, Record<string, string>> = {
  componentType: GROUP_LABELS,
  validationState: { valid: "有效", invalid: "无效", warning: "需要关注" },
  activation: { always: "始终启用", file_globs: "匹配文件时启用", manual: "手动加入", model_decides: "由模型判断" },
  transport: { stdio: "本地进程", "in-process": "本机内置", http: "HTTP", sse: "SSE", streamable_http: "流式 HTTP" },
  environment: { local_process: "本地进程", remote_endpoint: "远程服务" },
  handlerType: { command: "命令", http: "HTTP", prompt: "提示词", agent: "子任务智能体", mcp_tool: "MCP 工具" },
  failurePolicy: { continue: "记录错误并继续", stop: "停止运行" },
  type: { string: "文本", directory: "目录", file: "文件", number: "数字", integer: "整数", boolean: "布尔值" },
};

function displayValue(value: unknown, propertyKey?: string): ReactNode {
  if (value === null || value === undefined || value === "") return <span className="aw-plugin-inspector__empty-value">未设置</span>;
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "string") return PROPERTY_VALUE_LABELS[propertyKey ?? ""]?.[value] ?? value;
  if (typeof value === "number") return String(value);
  if (Array.isArray(value)) return value.length > 0 ? value.map(String).join("、") : "无";
  return <span className="aw-plugin-inspector__empty-value">详见对应配置区</span>;
}

const PROPERTY_LABELS: Record<string, string> = {
  displayName: "名称", componentType: "组件类型", sourceDialect: "来源方言", sourcePath: "源文件",
  mappingState: "兼容映射", validationState: "校验状态", packagePath: "包路径", shortName: "服务名称",
  transport: "连接方式", environment: "运行环境", oauthResourceConfigured: "OAuth 资源", activation: "激活方式",
  globs: "文件匹配", description: "说明", model: "模型提示", effort: "推理强度", readOnly: "只读",
  maxTurns: "最大轮次", timeoutSeconds: "超时秒数", toolCount: "工具数量", skillReferenceCount: "Skill 引用",
  declaredMaxTurns: "插件声明最大轮次", declaredTimeoutSeconds: "插件声明超时秒数",
  mcpReferenceCount: "MCP 引用", invocationName: "调用名称", argumentHint: "参数提示", manualOnly: "仅手动调用",
  sourceEvent: "来源事件", mappedEvent: "Awaken 事件", matcher: "匹配条件", handlerType: "处理器类型",
  failurePolicy: "失败策略", key: "配置键", type: "值类型", title: "标题", required: "必填",
  hasDefault: "包含默认值", secret: "敏感值", connectorId: "连接器标识",
};

function productPropertyRows(properties?: Record<string, unknown>): Array<[string, string, unknown]> {
  const rows: Array<[string, string, unknown]> = [];
  const hiddenTechnicalState = new Set([
    "componentType",
    "displayName",
    "mappingState",
	"hostManaged",
    "packagePath",
    "sourceDialect",
    "sourcePath",
    "validationState",
  ]);
  for (const [key, value] of Object.entries(properties ?? {})) {
    if (hiddenTechnicalState.has(key)) continue;
    if (value && typeof value === "object" && !Array.isArray(value)) {
      for (const [nestedKey, nestedValue] of Object.entries(value as Record<string, unknown>)) {
		if (hiddenTechnicalState.has(nestedKey)) continue;
        rows.push([nestedKey, PROPERTY_LABELS[nestedKey] ?? nestedKey, nestedValue]);
      }
    } else {
      rows.push([key, PROPERTY_LABELS[key] ?? key, value]);
    }
  }
  return rows;
}

export function PluginComponentDetailDialog({
  detail,
  loading,
  activeTab,
  onTabChange,
  onClose,
  busy,
  componentControlsDisabled = false,
  onMcpToolEnabledChange,
  onUseInChat,
  sourceEditing = false,
  sourceDraft = "",
  onSourceEditStart,
  onSourceDraftChange,
  onSourceEditCancel,
  onSourceSave,
  variables = [],
  variableValues = {},
  onVariableValueChange,
  onSaveVariables,
  onResetVariable,
  feedback,
  commandArguments,
  onCommandArgumentsChange,
  onRuleActivationChange,
  onHookReceiptRetry,
}: {
  detail?: AwakenPluginConsoleDetail | null;
  loading?: boolean;
  activeTab: AwakenPluginConsoleProps["activeDetailTab"];
  onTabChange: AwakenPluginConsoleProps["onDetailTabChange"];
  onClose: () => void;
  busy?: boolean;
  componentControlsDisabled?: boolean;
  onMcpToolEnabledChange?: (detail: AwakenPluginConsoleDetail, toolName: string, enabled: boolean) => void;
  commandArguments?: string;
  onCommandArgumentsChange?: (value: string) => void;
  onRuleActivationChange?: (
    detail: AwakenPluginConsoleDetail,
    activation: "always" | "file_globs" | "manual" | "model_decides",
  ) => void;
  onHookReceiptRetry?: (detail: AwakenPluginConsoleDetail, receiptId: string) => void;
  onUseInChat?: (detail: AwakenPluginConsoleDetail, commandArguments?: string) => void;
  sourceEditing?: boolean;
  sourceDraft?: string;
  onSourceEditStart?: (detail: AwakenPluginConsoleDetail) => void;
  onSourceDraftChange?: (value: string) => void;
  onSourceEditCancel?: () => void;
  onSourceSave?: (detail: AwakenPluginConsoleDetail) => void;
  variables?: AwakenPluginVariableField[];
  variableValues?: Record<string, string>;
  onVariableValueChange?: (key: string, value: string) => void;
  onSaveVariables?: () => void;
  onResetVariable?: (key: string) => void;
  feedback?: AwakenPluginConsoleFeedback | null;
}) {
  const active = Boolean(detail || loading);
  if (!active) return null;
  const isSkillPreview = detail?.component.kind === "skill";
  const isEditableMcpSource = detail?.component.kind === "mcp_server" && detail.sourceEditable === true;
  const commandDetails = detail?.command;
  const canUseInChat = Boolean(
    detail &&
    onUseInChat &&
    !componentControlsDisabled &&
    componentCanUseInChat(detail.component),
  );
  const tabs = [
    ["overview", "详情"],
    ["preview", "预览"],
    ["source", "源文件"],
    ["diagnostics", "诊断"],
  ] as const;
  const availableTabs = tabs.filter(([id]) => {
    if (id === "preview") return Boolean(detail?.preview);
    if (id === "source") return Boolean(detail?.source);
    if (id === "diagnostics") return Boolean(detail?.diagnostics?.length);
    return true;
  });
  const feedbackNode = feedback ? (
    <span
      className="aw-plugin-component-dialog__feedback"
      data-tone={feedback.tone}
      role={feedback.tone === "error" ? "alert" : "status"}
    >
      {feedback.message}
    </span>
  ) : null;
  const footer = isSkillPreview ? (
    <div className="aw-plugin-component-dialog__skill-footer">
      {feedbackNode}
      {canUseInChat ? (
        <span className="aw-plugin-component-dialog__footer-actions">
          <AwakenButton size="sm" variant="primary" onClick={() => detail && onUseInChat?.(detail)}>
            在对话中使用
          </AwakenButton>
        </span>
      ) : null}
    </div>
  ) : isEditableMcpSource && activeTab === "source" ? (
    <div className="aw-plugin-component-dialog__source-footer">
      {feedbackNode}
      <span className="aw-plugin-component-dialog__footer-actions">
        {sourceEditing ? (
          <Fragment key="source-editing-actions">
            <AwakenButton size="sm" variant="default" style={SOURCE_ACTION_BUTTON_STYLE} disabled={busy} onClick={onSourceEditCancel}>
              取消
            </AwakenButton>
            <AwakenButton
              size="sm"
              variant="primary"
              style={SOURCE_ACTION_BUTTON_STYLE}
              disabled={busy || !onSourceSave}
              loading={busy}
              onClick={() => detail && onSourceSave?.(detail)}
            >
              保存
            </AwakenButton>
          </Fragment>
        ) : (
          <AwakenButton
            key="source-view-action"
            size="sm"
            variant="default"
            style={SOURCE_ACTION_BUTTON_STYLE}
            disabled={busy || !onSourceEditStart}
            onClick={() => detail && onSourceEditStart?.(detail)}
          >
            编辑源文件
          </AwakenButton>
        )}
      </span>
    </div>
  ) : feedbackNode ? (
    <div className="aw-plugin-component-dialog__feedback-footer">{feedbackNode}</div>
  ) : undefined;
  return (
    <AwakenDialog
      open={active}
      size="large"
      className="aw-plugin-component-dialog"
      bodyClassName="aw-plugin-component-dialog__body"
      title={
        <span className="aw-plugin-component-dialog__title">
          <span className="aw-plugin-component-dialog__title-icon" aria-hidden="true">
            <AwakenIcon
              concept={detail?.component.kind === "mcp_server" ? "mcp-brand" : detail?.component.kind === "skill" ? "skill" : detail?.component.kind === "hook" ? "hook" : detail?.component.kind === "agent" ? "subagent" : "file-code"}
              size={20}
            />
          </span>
          <span>
            <strong>{detail?.component.name ?? "正在读取组件…"}</strong>
          </span>
        </span>
      }
      footer={footer}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {detail?.component.kind === "agent" && canUseInChat ? (
        <div className="aw-plugin-inspector__primary-actions">
          <AwakenButton size="sm" variant="primary" onClick={() => detail && onUseInChat?.(detail)}>
            作为子任务 Agent 使用
          </AwakenButton>
        </div>
      ) : null}
      {!loading && !isSkillPreview ? (
        <div className="aw-plugin-inspector__tabs" role="tablist" aria-label="组件详情视图">
          {availableTabs.map(([id, label]) => (
            <button
              type="button"
              role="tab"
              aria-selected={activeTab === id}
              data-active={activeTab === id ? "true" : "false"}
              key={id}
              onClick={() => onTabChange(id)}
            >
              {label}
            </button>
          ))}
        </div>
      ) : null}
      <div className="aw-plugin-inspector__body" data-loading={loading ? "true" : "false"}>
        {loading ? <p>正在读取组件详情…</p> : null}
        {!loading && detail && isSkillPreview ? (
          <article className="aw-plugin-inspector__document">
            <AwakenMarkdownRenderer markdown={detail.preview ?? ""} renderImages={false} renderMermaid={false} />
          </article>
        ) : null}
        {!loading && detail && !isSkillPreview && activeTab === "overview" ? (
          <>
            {detail.component.mappingState === "recognized_unsupported" ? (
              <section className="aw-plugin-inspector__unsupported" role="status">
                <strong>当前仅识别该组件</strong>
                <p>Awaken 尚未提供它的运行适配器，因此不会把它作为可调用能力交给模型。</p>
              </section>
            ) : null}
            {detail.component.kind === "rule" && canUseInChat ? (
              <section className="aw-plugin-inspector__component-config">
                <label>
                  <span>激活方式</span>
                  <select
                    value={detail.component.ruleActivation || "manual"}
                    disabled={busy || !onRuleActivationChange}
                    onChange={(event) => onRuleActivationChange?.(
                      detail,
                      event.target.value as "always" | "file_globs" | "manual" | "model_decides",
                    )}
                  >
                    <option value="always">始终启用</option>
                    <option value="file_globs">匹配文件时启用</option>
                    <option value="manual">手动加入</option>
                    <option value="model_decides">仅手动加入（模型决定暂不自动注入）</option>
                  </select>
                </label>
                <AwakenButton size="sm" variant="primary" onClick={() => onUseInChat?.(detail)}>加入下一次运行</AwakenButton>
              </section>
            ) : null}
            {detail.component.kind === "command" && canUseInChat ? (
              <section className="aw-plugin-inspector__component-config">
                {commandDetails?.acceptsArguments ? (
                  <label>
                    <span>命令参数</span>
                    <input
                      value={commandArguments ?? ""}
                      placeholder={String(commandDetails.argumentHint || "可选参数；支持引号")}
                      disabled={busy}
                      onChange={(event) => onCommandArgumentsChange?.(event.target.value)}
                    />
                  </label>
                ) : null}
                <AwakenButton
                  size="sm"
                  variant="primary"
                  disabled={Boolean(commandDetails?.argumentsRequired && !commandArguments?.trim())}
                  onClick={() => onUseInChat?.(detail, commandArguments)}
                >
                  插入 Composer
                </AwakenButton>
              </section>
            ) : null}
            {detail.mcp ? (
              <section className="aw-plugin-mcp-configurator">
                <header>
                  <div><h3>MCP 运行状态</h3><p>{detail.mcp.environment === "local_process" ? "本机进程" : "远程服务"} · {RUNTIME_LABELS[detail.mcp.connectionState] ?? detail.mcp.connectionState}</p></div>
				  {detail.mcp.hostManaged ? <span className="aw-plugin-mcp-configurator__owner">由 Awaken 连接器管理</span> : null}
                </header>
                {detail.mcp.errorReason ? <p className="aw-plugin-mcp-configurator__error">{detail.mcp.errorReason}</p> : null}
                <div className="aw-plugin-mcp-configurator__tools">
                  <header><h4>工具</h4><span>{detail.mcp.toolsLoaded ? `${detail.mcp.tools.length} 项` : "工具尚未加载"}</span></header>
                  {detail.mcp.tools.map((tool) => (
                    <label key={tool.name}>
                      <span><strong>{tool.name}</strong><small>{tool.description || "没有描述"}</small></span>
                      <AwakenSwitch checked={!componentControlsDisabled && tool.enabled} label={tool.name} size="compact" busy={busy} disabled={componentControlsDisabled || detail.mcp?.hostManaged || !onMcpToolEnabledChange} onCheckedChange={(enabled) => onMcpToolEnabledChange?.(detail, tool.name, enabled)} />
                    </label>
                  ))}
                </div>
              </section>
            ) : null}
            {detail.hook ? (
              <section className="aw-plugin-hook-receipts">
                <header><div><h3>钩子运行状态</h3><p>{detail.hook.sourceEvent} → {detail.hook.mappedEvent || "未映射"} · {detail.hook.handlerType} · {detail.hook.executionMode === "async" ? "后台" : detail.hook.executionMode === "dynamic" ? "运行时决定" : "同步"}</p></div><span>{detail.hook.scope}</span></header>
                <dl>
				  <div><dt>匹配条件</dt><dd>{[detail.hook.matcher, detail.hook.condition].filter(Boolean).join(" · ") || "全部"}</dd></div>
                  <div><dt>失败策略</dt><dd>{detail.hook.failurePolicy}</dd></div>
                  <div><dt>回执保留</dt><dd>{detail.hook.retention === "memory_only" ? "当前 Core 进程内" : detail.hook.retention || "未知"}</dd></div>
                </dl>
                <h4>最近回执</h4>
                {detail.hook.receipts.length === 0 ? <p>尚无运行回执。</p> : (
                  <ul>{detail.hook.receipts.map((receipt) => (
                    <li key={receipt.id} data-status={receipt.status}>
                      <span><strong>{receipt.status}</strong><small>{new Date(receipt.createdAt).toLocaleString()} · {receipt.durationMs} ms · {receipt.action}{receipt.reasonCode ? ` · ${receipt.reasonCode}` : ""}</small></span>
                      {receipt.status !== "success" && receipt.status !== "blocked" ? <AwakenButton size="sm" variant="default" loading={busy} disabled={!onHookReceiptRetry} onClick={() => onHookReceiptRetry?.(detail, receipt.id)}>诊断性重试</AwakenButton> : null}
                    </li>
                  ))}</ul>
                )}
              </section>
            ) : null}
            {detail.component.kind === "variable" ? (
              <VariableEditor
                fields={variables.filter((field) => !detail.component.variable?.key || field.key === detail.component.variable.key)}
                values={variableValues}
                busy={busy}
                onValueChange={onVariableValueChange}
                onSave={onSaveVariables}
                onReset={onResetVariable}
              />
            ) : null}
            {productPropertyRows(detail.properties).length > 0 ? <dl className="aw-plugin-inspector__properties">
              {productPropertyRows(detail.properties).map(([propertyKey, label, value], index) => (
                <div key={`${propertyKey}-${index}`}><dt>{label}</dt><dd>{displayValue(value, propertyKey)}</dd></div>
              ))}
            </dl> : null}
            {detail.resources?.length ? (
              <section className="aw-plugin-inspector__resources">
                <h3>关联资源</h3>
                <ul>{detail.resources.map((resource) => <li key={resource}>{resource}</li>)}</ul>
              </section>
            ) : null}
          </>
        ) : null}
        {!loading && detail && !isSkillPreview && activeTab === "preview" ? <article className="aw-plugin-inspector__document"><AwakenMarkdownRenderer markdown={detail.preview ?? ""} renderImages={false} renderMermaid={false} /></article> : null}
        {!loading && detail && !isSkillPreview && activeTab === "source" ? (
          isEditableMcpSource ? (
            <textarea
              className="aw-plugin-inspector__source-surface"
              aria-label="MCP 源文件"
              value={sourceEditing ? sourceDraft : detail.source ?? ""}
              readOnly={!sourceEditing}
              disabled={sourceEditing && busy}
              data-editing={sourceEditing ? "true" : "false"}
              spellCheck={false}
              onChange={sourceEditing ? (event) => onSourceDraftChange?.(event.target.value) : undefined}
            />
          ) : (
            <pre className="aw-plugin-inspector__source"><code>{detail.source}</code></pre>
          )
        ) : null}
        {!loading && detail && !isSkillPreview && activeTab === "diagnostics" ? (
          <ul className="aw-plugin-inspector__diagnostics">
            {(detail.diagnostics ?? []).map((diagnostic, index) => (
              <li key={`${diagnostic.code ?? "diagnostic"}-${index}`} data-severity={diagnostic.severity ?? "info"}>
                <strong>{diagnostic.code || diagnostic.severity || "诊断"}</strong>
                <span>{diagnostic.message || "没有详细说明"}</span>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </AwakenDialog>
  );
}

export function VariableEditor({
  fields,
  values,
  busy,
  onValueChange,
  onSave,
  onReset,
}: {
  fields: AwakenPluginVariableField[];
  values: Record<string, string>;
  busy?: boolean;
  onValueChange?: (key: string, value: string) => void;
  onSave?: () => void;
  onReset?: (key: string) => void;
}) {
  if (fields.length === 0) return null;
  return (
    <section className="aw-plugin-variable-editor">
      <header><div><h2>配置变量</h2><p>Secret 只显示配置状态，保存后不会回显。</p></div><AwakenButton size="sm" variant="primary" disabled={busy || !onSave} onClick={onSave}>保存配置</AwakenButton></header>
      <div className="aw-plugin-variable-editor__fields">
        {fields.map((field) => {
          const defaultValue = field.secret ? undefined : field.defaultValue;
          const displayedValue = values[field.key] ?? (field.configured ? String(field.value ?? "") : String(defaultValue ?? ""));
          return <label key={field.key} className="aw-plugin-variable-editor__field">
            <span><strong>{field.title || field.key}</strong>{field.required ? <em>必填</em> : null}<small>{field.description || `${field.type}${field.secret ? " · Secret" : ""}`}</small></span>
            <span className="aw-plugin-variable-editor__control">
              {field.enum?.length ? (
                <select value={displayedValue} disabled={busy} onChange={(event) => onValueChange?.(field.key, event.target.value)}>
                  {!field.required && defaultValue === undefined ? <option value="">请选择</option> : null}
                  {field.enum.map((option) => <option key={String(option)} value={String(option)}>{String(option)}</option>)}
                </select>
              ) : field.type === "boolean" ? (
                <select value={displayedValue} disabled={busy} onChange={(event) => onValueChange?.(field.key, event.target.value)}><option value="">请选择</option><option value="true">是</option><option value="false">否</option></select>
              ) : (
                <input
                  type={field.secret ? "password" : field.type === "number" || field.type === "integer" ? "number" : "text"}
                  value={field.secret ? values[field.key] ?? "" : displayedValue}
                  min={field.minimum}
                  max={field.maximum}
                  step={field.type === "integer" ? 1 : undefined}
                  placeholder={field.secret && field.configured ? "已配置；输入新值以替换" : field.multiple ? "多个值用英文逗号分隔" : "未配置"}
                  disabled={busy}
                  autoComplete="off"
                  onChange={(event) => onValueChange?.(field.key, event.target.value)}
                />
              )}
              <span className="aw-plugin-variable-editor__configured" data-configured={field.configured ? "true" : "false"}>{field.configured ? "已配置" : defaultValue !== undefined ? `使用默认值 ${String(defaultValue)}` : "未配置"}</span>
              {field.configured && onReset ? <button type="button" disabled={busy} onClick={() => onReset(field.key)}>清除</button> : null}
            </span>
            {field.dependencies?.length ? <span className="aw-plugin-variable-editor__dependencies">影响组件：{field.dependencies.map((item) => `${item.name}（${GROUP_LABELS[item.kind] ?? item.kind}）`).join("、")}</span> : null}
          </label>;
        })}
      </div>
    </section>
  );
}

export function AwakenPluginConsole({
  name,
  displayName,
  builtin = false,
  brandIcon,
  version,
  enabled,
  components,
  setupItems = [],
  selectedDetail,
  detailLoading,
  activeDetailTab,
  variables = [],
  variableValues = {},
  loading = false,
  busy = false,
  errorMessage,
  detailFeedback,
  onBack,
  onEnabledChange,
  onUninstall,
  onComponentOpen,
  onComponentEnabledChange,
  onSetupAction,
  onDetailTabChange,
  onDetailClose,
  onMcpToolEnabledChange,
  onUseInChat,
  sourceEditing,
  sourceDraft,
  onSourceEditStart,
  onSourceDraftChange,
  onSourceEditCancel,
  onSourceSave,
  commandArguments,
  onCommandArgumentsChange,
  onRuleActivationChange,
  onHookReceiptRetry,
  onVariableValueChange,
  onSaveVariables,
  onResetVariable,
}: AwakenPluginConsoleProps) {
  const grouped = useMemo(
    () => GROUPS.map(([kind, title]) => ({ kind, title, items: components.filter((component) => component.kind === kind) })),
    [components],
  );
  const unknown = useMemo(
    () => components.filter((component) => !GROUPS.some(([kind]) => kind === component.kind)),
    [components],
  );
  return (
    <section
      className="aw-plugin-console"
      aria-label={`插件 ${displayName || name}`}
    >
      <main className="aw-plugin-console__main">
        <header className="aw-plugin-console__identity">
          <button
            type="button"
            className="aw-plugin-console__back"
            aria-label="返回插件列表"
            onClick={(event) => {
              event.preventDefault();
              event.stopPropagation();
              onBack();
            }}
          ><AwakenIcon concept="back" size={16} />返回插件</button>
          <div className="aw-plugin-console__identity-row">
            <span className="aw-plugin-console__logo">
              {isAwakenPluginBrand(brandIcon) ? (
                <AwakenPluginBrandIcon brand={brandIcon} size="detail" />
              ) : (
                <AwakenIcon concept="plugin-default" size={24} />
              )}
            </span>
            <div className="aw-plugin-console__title">
              <h1>{displayName || name}</h1>
              {builtin ? <span>Awaken 内置</span> : null}
              {version ? <span>v{version}</span> : null}
            </div>
            <div className="aw-plugin-console__actions">
              {onUninstall ? <AwakenButton size="sm" variant="ghost" disabled={busy} onClick={onUninstall}>卸载</AwakenButton> : null}
              <AwakenSwitch checked={enabled} label={displayName || name} busy={busy} onCheckedChange={onEnabledChange} />
            </div>
          </div>
        </header>
        {errorMessage ? <div className="aw-plugin-console__error" role="alert">{errorMessage}</div> : null}
        <PluginSetupSection
          items={setupItems}
          displayName={displayName || name}
          brandIcon={brandIcon}
          busy={busy}
          onAction={onSetupAction}
        />
        <div className="aw-plugin-console__groups" aria-busy={loading}>
          {loading ? <div className="aw-plugin-console__loading" role="status" aria-label="正在加载插件组件">
            <span /><span /><span />
          </div> : null}
          {!loading ? grouped.map((group) => (
            <Fragment key={group.kind}>
              <ComponentGroup title={group.title} components={group.items} parentEnabled={enabled} busy={busy} onOpen={onComponentOpen} onEnabledChange={onComponentEnabledChange} />
            </Fragment>
          )) : null}
          {!loading ? <ComponentGroup title="其他" components={unknown} parentEnabled={enabled} busy={busy} onOpen={onComponentOpen} onEnabledChange={onComponentEnabledChange} /> : null}
          {!loading && components.length === 0 ? <div className="aw-plugin-console__empty"><h2>未发现组件</h2><p>检查插件方言、清单与诊断后再试。</p></div> : null}
        </div>
      </main>
      <PluginComponentDetailDialog
        detail={selectedDetail}
        loading={detailLoading}
        activeTab={activeDetailTab}
        onTabChange={onDetailTabChange}
        onClose={onDetailClose}
        busy={busy}
        feedback={detailFeedback}
        componentControlsDisabled={!enabled}
        onMcpToolEnabledChange={onMcpToolEnabledChange}
        commandArguments={commandArguments}
        onCommandArgumentsChange={onCommandArgumentsChange}
        onRuleActivationChange={onRuleActivationChange}
        onHookReceiptRetry={onHookReceiptRetry}
        onUseInChat={onUseInChat}
        sourceEditing={sourceEditing}
        sourceDraft={sourceDraft}
        onSourceEditStart={onSourceEditStart}
        onSourceDraftChange={onSourceDraftChange}
        onSourceEditCancel={onSourceEditCancel}
        onSourceSave={onSourceSave}
        variables={variables}
        variableValues={variableValues}
        onVariableValueChange={onVariableValueChange}
        onSaveVariables={onSaveVariables}
        onResetVariable={onResetVariable}
      />
    </section>
  );
}
