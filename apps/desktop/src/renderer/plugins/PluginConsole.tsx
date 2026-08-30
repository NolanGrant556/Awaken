import { useEffect, useRef, useState } from "react";
import {
  AwakenPluginConsole,
  type AwakenPluginConsoleComponent,
  type AwakenPluginConsoleDetail,
} from "@awaken/design-system";
import type {
  RuntimePlugin,
  RuntimePluginComponent,
  RuntimePluginComponentDetail,
  RuntimePluginComponentState,
  RuntimePluginVariableField,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { pluginErrorMessage } from "./pluginErrors";

type PluginConsoleProps = {
  plugin: RuntimePlugin;
  runtimeClient: RuntimeClient;
  busy?: boolean;
  onBack: () => void;
  onPluginUpdated: (plugin: RuntimePlugin) => void;
  onUninstall: () => void;
  onUsePlugin: (plugin: RuntimePlugin) => void;
  onUsePluginComponent: (
    plugin: RuntimePlugin,
    component: RuntimePluginComponent,
    commandArguments: string[],
  ) => void;
  onNotice: (message: string) => void;
};

function componentName(component: RuntimePluginComponent): string {
  return (
    component.displayName?.trim() ||
    component.declaredName?.trim() ||
    component.derivedName?.trim() ||
    component.logicalId
  );
}

function toConsoleComponent(state: RuntimePluginComponentState): AwakenPluginConsoleComponent {
  const component = state.component;
  return {
    logicalId: component.logicalId,
    logicalIdEncoded: component.logicalIdEncoded,
    kind: component.kind,
    name: componentName(component),
    description: component.description,
    sourcePath: component.sourcePath,
    sourceDialect: component.sourceDialect,
    mappingState: state.mappingState,
    validation: component.validation,
    desiredState: state.desiredState,
    ruleActivation: state.ruleActivation,
    configurationState: state.configurationState,
    runtimeState: state.runtimeState,
    integrityState: state.integrityState,
    switchable: state.switchable,
    diagnosticCount: component.diagnostics?.length,
    mcp: component.mcp,
    rule: component.rule,
    agent: component.agent,
    command: component.command,
    hook: component.hook,
    variable: component.variable,
  };
}

function toConsoleDetail(detail: RuntimePluginComponentDetail): AwakenPluginConsoleDetail {
  const properties = { ...(detail.properties ?? {}) };
  const connection = properties.connection as
    | { state?: string; errorReason?: string; transport?: string; toolsLoaded?: boolean; toolCount?: number }
    | undefined;
  const tools = Array.isArray(properties.tools)
    ? (properties.tools as Array<{ name: string; description?: string; enabled: boolean }>)
    : [];
  const hookSpec = properties.hook as
    | { sourceEvent?: string; mappedEvent?: string; matcher?: string; condition?: string; handlerType?: string; scope?: string; async?: boolean; asyncRewake?: boolean }
    | undefined;
  const commandSpec = properties.command as
	| { argumentHint?: string; acceptsArguments?: boolean; argumentsRequired?: boolean }
	| undefined;
  delete properties.connection;
  delete properties.tools;
  delete properties.toolsState;
  delete properties.hook;
  delete properties.command;
  return {
    component: toConsoleComponent(detail.state),
    preview: detail.preview,
    source: detail.source,
    sourceEditable: detail.sourceEditable,
    properties,
    resources: detail.resources,
    diagnostics: detail.state.component.diagnostics,
	command: detail.state.component.command
	  ? {
		  argumentHint: commandSpec?.argumentHint ?? detail.state.component.command.argumentHint,
		  acceptsArguments: commandSpec?.acceptsArguments ?? detail.state.component.command.acceptsArguments === true,
		  argumentsRequired: commandSpec?.argumentsRequired ?? detail.state.component.command.argumentsRequired === true,
		}
	  : undefined,
    mcp: detail.state.component.mcp
      ? {
          shortName: detail.state.component.mcp.shortName ?? componentName(detail.state.component),
          environment:
            connection?.transport === "stdio" || detail.state.component.mcp.transport === "stdio"
              ? "local_process"
              : "remote_endpoint",
          connectionState: connection?.state ?? "not_loaded",
          errorReason: connection?.errorReason,
          toolsLoaded: connection?.toolsLoaded === true,
          tools,
        }
      : undefined,
    hook: detail.state.component.hook && hookSpec
      ? {
          sourceEvent: hookSpec.sourceEvent ?? "unknown",
          mappedEvent: hookSpec.mappedEvent,
          matcher: hookSpec.matcher,
		  condition: hookSpec.condition,
          handlerType: hookSpec.handlerType ?? "unknown",
		  executionMode: hookSpec.async
			? "async"
			: detail.state.component.sourceDialect === "claude_code" && hookSpec.handlerType === "command"
			  ? "dynamic"
			  : "sync",
		  asyncRewake: hookSpec.asyncRewake === true,
          scope: hookSpec.scope === "plugin" || !hookSpec.scope ? "插件级" : hookSpec.scope,
          failurePolicy: "continue",
          retention: detail.hookReceiptRetention,
          receipts: (detail.hookReceipts ?? []).map((receipt) => ({
            id: receipt.id,
            status: receipt.status,
            action: receipt.action,
            reasonCode: receipt.reasonCode,
            durationMs: receipt.durationMs,
            createdAt: receipt.createdAt,
            retryOf: receipt.retryOf,
          })),
        }
      : undefined,
  };
}

function configurationValue(field: RuntimePluginVariableField, raw: string): unknown {
  if (field.schema.type === "boolean") return raw === "true";
  if (field.schema.type === "number" || field.schema.type === "integer") return Number(raw);
  if (field.schema.multiple) {
    return raw
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return raw;
}

export function parsePluginCommandArguments(value: string): string[] {
  const argumentsList: string[] = [];
  let current = "";
  let quote: "'" | '"' | null = null;
  let escaping = false;
  for (const character of value.trim()) {
    if (escaping) {
      current += character;
      escaping = false;
      continue;
    }
    if (character === "\\" && quote !== "'") {
      escaping = true;
      continue;
    }
    if (quote) {
      if (character === quote) quote = null;
      else current += character;
      continue;
    }
    if (character === "'" || character === '"') {
      quote = character;
      continue;
    }
    if (/\s/.test(character)) {
      if (current) {
        argumentsList.push(current);
        current = "";
      }
      continue;
    }
    current += character;
  }
  if (escaping) current += "\\";
  if (quote) throw new Error("命令参数中的引号没有闭合");
  if (current) argumentsList.push(current);
  return argumentsList;
}

export function isPluginComponentUsableInChat(
  pluginEnabled: boolean,
  state: RuntimePluginComponentState,
): boolean {
  return (
    pluginEnabled &&
    state.desiredState === "enabled" &&
    state.component.validation !== "invalid" &&
    state.integrityState === "intact" &&
    state.configurationState !== "needs_configuration" &&
    (state.mappingState === "native" || state.mappingState === "adapted")
  );
}

export function PluginConsole({
  plugin,
  runtimeClient,
  busy = false,
  onBack,
  onPluginUpdated,
  onUninstall,
  onUsePluginComponent,
  onNotice,
}: PluginConsoleProps) {
  const loadGenerationRef = useRef(0);
  const detailGenerationRef = useRef(0);
  const [components, setComponents] = useState<RuntimePluginComponentState[]>([]);
  const [variables, setVariables] = useState<RuntimePluginVariableField[]>([]);
  const [variableValues, setVariableValues] = useState<Record<string, string>>({});
  const [commandArguments, setCommandArguments] = useState("");
  const [sourceEditing, setSourceEditing] = useState(false);
  const [sourceDraft, setSourceDraft] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<RuntimePluginComponentDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [componentsLoading, setComponentsLoading] = useState(true);
  const [activeDetailTab, setActiveDetailTab] = useState<
    "overview" | "preview" | "source" | "diagnostics"
  >("overview");
  const [localBusy, setLocalBusy] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [detailFeedback, setDetailFeedback] = useState<{
    message: string;
    tone: "info" | "success" | "error";
  } | null>(null);

  function reportDetailFeedback(message: string, tone: "info" | "success" = "success"): void {
    if (selectedDetail) {
      setDetailFeedback({ message, tone });
      return;
    }
    onNotice(message);
  }

  useEffect(() => {
    const generation = ++loadGenerationRef.current;
    setComponentsLoading(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    setSelectedDetail(null);
    setSourceEditing(false);
    setSourceDraft("");
    void Promise.all([
      runtimeClient.listPluginComponents(plugin.name),
      runtimeClient.getPluginConfiguration(plugin.name),
    ])
      .then(([nextComponents, configuration]) => {
        if (generation !== loadGenerationRef.current) return;
        setComponents(nextComponents ?? []);
        setVariables(configuration.fields ?? []);
        setVariableValues({});
      })
      .catch((error: unknown) => {
        if (generation === loadGenerationRef.current) {
          setErrorMessage(pluginErrorMessage(error));
        }
      })
      .finally(() => {
        if (generation === loadGenerationRef.current) setComponentsLoading(false);
      });
    return () => {
      loadGenerationRef.current += 1;
      detailGenerationRef.current += 1;
    };
  }, [plugin.name, runtimeClient]);

  async function handleComponentOpen(component: AwakenPluginConsoleComponent): Promise<void> {
    const generation = ++detailGenerationRef.current;
    setDetailLoading(true);
    setActiveDetailTab("overview");
    setCommandArguments("");
    setSourceEditing(false);
    setSourceDraft("");
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const detail = await runtimeClient.getPluginComponent(
        plugin.name,
        component.kind,
        component.logicalIdEncoded,
      );
      if (component.kind === "hook") {
        const receipts = await runtimeClient.listPluginHookReceipts(
          plugin.name,
          component.logicalIdEncoded,
          detail.state.component.revisionId,
        );
        detail.hookReceipts = receipts.items ?? [];
        detail.hookReceiptRetention = receipts.retention;
      }
      if (generation === detailGenerationRef.current) {
        setSelectedDetail(detail);
        setSourceDraft(detail.source ?? "");
      }
    } catch (error) {
      if (generation === detailGenerationRef.current) {
        setErrorMessage(pluginErrorMessage(error));
      }
    } finally {
      if (generation === detailGenerationRef.current) setDetailLoading(false);
    }
  }

  async function handleRuleActivationChange(
    detail: AwakenPluginConsoleDetail,
    activation: "always" | "file_globs" | "manual" | "model_decides",
  ): Promise<void> {
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const response = await runtimeClient.setPluginRuleActivation(
        plugin.name,
        detail.component.logicalIdEncoded,
        activation,
      );
      onPluginUpdated(response.plugin);
      const nextComponents = await runtimeClient.listPluginComponents(plugin.name);
      setComponents(nextComponents ?? []);
      await reloadSelectedDetail();
      reportDetailFeedback("Rule 激活方式已保存，将在新的 run 快照中生效");
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

  function handleUseComponent(
    detail: AwakenPluginConsoleDetail,
    rawCommandArguments?: string,
  ): void {
    const componentState = components.find(
      (state) => state.component.logicalId === detail.component.logicalId,
    );
    if (!componentState) {
      setErrorMessage("组件已经变化，请重新打开详情");
      return;
    }
    const component = componentState.component;
    if (!isPluginComponentUsableInChat(plugin.enabled, componentState)) {
      setErrorMessage("该组件当前不可运行，请查看诊断后重试");
      return;
    }
    try {
      const argumentsList =
        detail.component.kind === "command"
          ? parsePluginCommandArguments(rawCommandArguments ?? "")
          : [];
      onUsePluginComponent(plugin, component, argumentsList);
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    }
  }

  async function handleComponentEnabledChange(
    component: AwakenPluginConsoleComponent,
    enabled: boolean,
  ): Promise<void> {
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const result = await runtimeClient.setPluginComponentState(
        plugin.name,
        component.kind,
        component.logicalIdEncoded,
        enabled,
      );
      onPluginUpdated(result.plugin);
      const nextComponents = await runtimeClient.listPluginComponents(plugin.name);
      setComponents(nextComponents ?? []);
      await reloadSelectedDetail();
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

  async function reloadSelectedDetail(): Promise<void> {
    const current = selectedDetail;
    if (!current) return;
    const generation = ++detailGenerationRef.current;
    const detail = await runtimeClient.getPluginComponent(
      plugin.name,
      current.state.component.kind,
      current.state.component.logicalIdEncoded,
    );
    if (current.state.component.kind === "hook") {
      const receipts = await runtimeClient.listPluginHookReceipts(
        plugin.name,
        current.state.component.logicalIdEncoded,
        detail.state.component.revisionId,
      );
      detail.hookReceipts = receipts.items ?? [];
      detail.hookReceiptRetention = receipts.retention;
    }
    if (generation === detailGenerationRef.current) {
      setSelectedDetail(detail);
    }
  }

  async function handleHookReceiptRetry(
    _detail: AwakenPluginConsoleDetail,
    receiptId: string,
  ): Promise<void> {
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      await runtimeClient.retryPluginHookReceipt(
        plugin.name,
        selectedDetail?.state.component.logicalIdEncoded ?? "",
        receiptId,
      );
      await reloadSelectedDetail();
      reportDetailFeedback("已执行诊断性重试；没有重放原始 run 上下文", "info");
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

  async function handleMcpToolEnabledChange(
    detail: AwakenPluginConsoleDetail,
    toolName: string,
    enabled: boolean,
  ): Promise<void> {
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const response = await runtimeClient.setPluginMcpToolState(
        plugin.name,
        detail.component.logicalIdEncoded,
        toolName,
        enabled,
      );
      onPluginUpdated(response.plugin);
      await reloadSelectedDetail();
      reportDetailFeedback(enabled ? "MCP tool 已启用，将在新 run 快照生效" : "MCP tool 已停用，将在新 run 快照生效");
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

  async function handleSourceSave(): Promise<void> {
    const current = selectedDetail;
    if (!current || current.state.component.kind !== "mcp_server" || !current.sourceEditable) {
      setErrorMessage("此组件的源文件不可编辑");
      return;
    }
    const expectedDigest = plugin.digest?.trim();
    if (!expectedDigest) {
      setErrorMessage("插件完整性摘要不可用，请重新打开插件后再试");
      return;
    }
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const response = await runtimeClient.updatePluginComponentSource(
        plugin.name,
        current.state.component.kind,
        current.state.component.logicalIdEncoded,
        { expectedDigest, source: sourceDraft },
      );
      onPluginUpdated(response.plugin);
      const nextComponents = await runtimeClient.listPluginComponents(plugin.name);
      setComponents(nextComponents ?? []);
      const matchingComponent = (nextComponents ?? []).find(
        (state) => state.component.logicalId === current.state.component.logicalId,
      );
      if (!matchingComponent) {
        detailGenerationRef.current += 1;
        setSelectedDetail(null);
        setSourceDraft("");
      } else {
        const nextDetail = await runtimeClient.getPluginComponent(
          plugin.name,
          matchingComponent.component.kind,
          matchingComponent.component.logicalIdEncoded,
        );
        setSelectedDetail(nextDetail);
        setSourceDraft(nextDetail.source ?? "");
      }
      setSourceEditing(false);
      reportDetailFeedback(
        response.changed
          ? "MCP 源文件已保存，插件组件与运行状态已刷新"
          : "MCP 源文件内容未变化",
        response.changed ? "success" : "info",
      );
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

  async function handlePluginEnabledChange(enabled: boolean): Promise<void> {
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const next = enabled
        ? await runtimeClient.enablePlugin(plugin.name)
        : await runtimeClient.disablePlugin(plugin.name);
      onPluginUpdated(next);
      const nextComponents = await runtimeClient.listPluginComponents(plugin.name);
      setComponents(nextComponents ?? []);
      await reloadSelectedDetail();
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

  async function saveVariables(reset: string[] = []): Promise<void> {
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const fieldsByKey = new Map(variables.map((field) => [field.key, field]));
      const values: Record<string, unknown> = {};
      for (const [key, raw] of Object.entries(variableValues)) {
        const field = fieldsByKey.get(key);
        if (!field || (field.schema.secret && raw === "")) continue;
        values[key] = configurationValue(field, raw);
      }
      const response = await runtimeClient.updatePluginConfiguration(plugin.name, { values, reset });
      setVariables(response.fields ?? []);
      setVariableValues({});
      const [nextPlugin, nextComponents] = await Promise.all([
        runtimeClient.getPlugin(plugin.name),
        runtimeClient.listPluginComponents(plugin.name),
      ]);
      onPluginUpdated(nextPlugin);
      setComponents(nextComponents ?? []);
      reportDetailFeedback(reset.length > 0 ? "插件变量已清除" : "插件配置已保存");
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

  return (
    <AwakenPluginConsole
      name={plugin.name}
      displayName={plugin.displayName}
      version={plugin.version}
      enabled={plugin.enabled}
      components={components.map(toConsoleComponent)}
      selectedDetail={selectedDetail ? toConsoleDetail(selectedDetail) : null}
      detailLoading={detailLoading}
      activeDetailTab={activeDetailTab}
      variables={variables.map((field) => ({
        key: field.key,
        title: field.schema.title,
        description: field.schema.description,
        type: field.schema.type,
        required: field.schema.required,
        secret: field.schema.secret,
        configured: field.configured,
        value: field.value,
        enum: field.schema.enum,
        defaultValue: field.schema.default,
        multiple: field.schema.multiple,
        minimum: field.schema.minimum,
        maximum: field.schema.maximum,
        dependencies: field.dependencies,
      }))}
      variableValues={variableValues}
      loading={componentsLoading}
      busy={busy || localBusy}
      errorMessage={selectedDetail ? null : errorMessage}
      detailFeedback={selectedDetail && errorMessage
        ? { message: errorMessage, tone: "error" }
        : detailFeedback}
      onBack={onBack}
      onEnabledChange={(enabled) => void handlePluginEnabledChange(enabled)}
      onUninstall={onUninstall}
      onComponentOpen={(component) => void handleComponentOpen(component)}
      onComponentEnabledChange={(component, enabled) =>
        void handleComponentEnabledChange(component, enabled)
      }
      onDetailTabChange={(tab) => {
        setActiveDetailTab(tab);
        setErrorMessage(null);
        setDetailFeedback(null);
      }}
      onDetailClose={() => {
        detailGenerationRef.current += 1;
        setSelectedDetail(null);
        setDetailLoading(false);
        setSourceEditing(false);
        setSourceDraft("");
        setErrorMessage(null);
        setDetailFeedback(null);
      }}
      onMcpToolEnabledChange={(detail, toolName, enabled) =>
        void handleMcpToolEnabledChange(detail, toolName, enabled)
      }
      commandArguments={commandArguments}
      onCommandArgumentsChange={(value) => {
        setCommandArguments(value);
        setErrorMessage(null);
        setDetailFeedback(null);
      }}
      onRuleActivationChange={(detail, activation) =>
        void handleRuleActivationChange(detail, activation)
      }
      onHookReceiptRetry={(detail, receiptId) =>
        void handleHookReceiptRetry(detail, receiptId)
      }
      onUseInChat={handleUseComponent}
      sourceEditing={sourceEditing}
      sourceDraft={sourceDraft}
      onSourceEditStart={(detail) => {
        setSourceDraft(detail.source ?? "");
        setSourceEditing(true);
        setErrorMessage(null);
        setDetailFeedback(null);
      }}
      onSourceDraftChange={(value) => {
        setSourceDraft(value);
        setErrorMessage(null);
        setDetailFeedback(null);
      }}
      onSourceEditCancel={() => {
        setSourceDraft(selectedDetail?.source ?? "");
        setSourceEditing(false);
        setErrorMessage(null);
        setDetailFeedback(null);
      }}
      onSourceSave={() => void handleSourceSave()}
      onVariableValueChange={(key, value) => {
        setVariableValues((current) => ({ ...current, [key]: value }));
        setErrorMessage(null);
        setDetailFeedback(null);
      }}
      onSaveVariables={() => void saveVariables()}
      onResetVariable={(key) => void saveVariables([key])}
    />
  );
}
