import { useEffect, useRef, useState } from "react";
import {
  AwakenPluginConsole,
  type AwakenPluginConsoleComponent,
  type AwakenPluginConsoleDetail,
  type AwakenPluginSetupItem,
} from "@awaken/design-system";
import type {
  RuntimePlugin,
  RuntimePluginComponent,
  RuntimePluginComponentDetail,
  RuntimePluginComponentState,
  RuntimePluginVariableField,
  RuntimePluginSetupItem,
  CliProviderId,
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

type PluginConsoleSnapshot = {
  digest: string;
  components: RuntimePluginComponentState[];
  variables: RuntimePluginVariableField[];
  setupItems: RuntimePluginSetupItem[];
};

const pluginConsoleSnapshots = new WeakMap<RuntimeClient, Map<string, PluginConsoleSnapshot>>();

function readPluginConsoleSnapshot(
  runtimeClient: RuntimeClient,
  plugin: RuntimePlugin,
): PluginConsoleSnapshot | undefined {
  const snapshot = pluginConsoleSnapshots.get(runtimeClient)?.get(plugin.name);
  return snapshot?.digest === (plugin.digest ?? "") ? snapshot : undefined;
}

function writePluginConsoleSnapshot(
  runtimeClient: RuntimeClient,
  plugin: RuntimePlugin,
  snapshot: Omit<PluginConsoleSnapshot, "digest">,
): void {
  let snapshots = pluginConsoleSnapshots.get(runtimeClient);
  if (!snapshots) {
    snapshots = new Map();
    pluginConsoleSnapshots.set(runtimeClient, snapshots);
  }
  snapshots.set(plugin.name, { ...snapshot, digest: plugin.digest ?? "" });
}

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
    authenticationState: state.authenticationState ?? "not_required",
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
	const mcpSpec = properties.mcp as
	  | { hostManaged?: boolean; connectorId?: string }
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
            connection?.transport === "stdio" ||
            connection?.transport === "in-process" ||
            detail.state.component.mcp.transport === "stdio" ||
            detail.state.component.mcp.transport === "in-process"
              ? "local_process"
              : "remote_endpoint",
          connectionState: connection?.state ?? "not_loaded",
          errorReason: connection?.errorReason,
          toolsLoaded: connection?.toolsLoaded === true,
          tools,
		  hostManaged: detail.state.component.mcp.hostManaged === true || mcpSpec?.hostManaged === true,
		  connectorId: detail.state.component.mcp.connectorId ?? mcpSpec?.connectorId,
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
    ((state.authenticationState ?? "not_required") === "ready" || (state.authenticationState ?? "not_required") === "not_required") &&
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
  const setupPollGenerationRef = useRef(0);
  const pollingSetupSessionsRef = useRef<Record<string, string>>({});
  const initialSnapshot = readPluginConsoleSnapshot(runtimeClient, plugin);
  const [components, setComponents] = useState<RuntimePluginComponentState[]>(
    () => initialSnapshot?.components ?? [],
  );
  const [variables, setVariables] = useState<RuntimePluginVariableField[]>(
    () => initialSnapshot?.variables ?? [],
  );
  const [setupItems, setSetupItems] = useState<RuntimePluginSetupItem[]>(
    () => initialSnapshot?.setupItems ?? plugin.setupItems ?? [],
  );
  const [activeSetupSessions, setActiveSetupSessions] = useState<Record<string, string>>({});
  const [variableValues, setVariableValues] = useState<Record<string, string>>({});
  const [commandArguments, setCommandArguments] = useState("");
  const [sourceEditing, setSourceEditing] = useState(false);
  const [sourceDraft, setSourceDraft] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<RuntimePluginComponentDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [componentsLoading, setComponentsLoading] = useState(() => !initialSnapshot);
  const [optimisticEnabled, setOptimisticEnabled] = useState(plugin.enabled);
  const [activeDetailTab, setActiveDetailTab] = useState<
    "overview" | "preview" | "source" | "diagnostics"
  >("overview");
  const [localBusy, setLocalBusy] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [detailFeedback, setDetailFeedback] = useState<{
    message: string;
    tone: "info" | "success" | "error";
  } | null>(null);

  useEffect(() => {
    setOptimisticEnabled(plugin.enabled);
  }, [plugin.enabled, plugin.name]);

  function reportDetailFeedback(message: string, tone: "info" | "success" = "success"): void {
    if (selectedDetail) {
      setDetailFeedback({ message, tone });
      return;
    }
    onNotice(message);
  }

  function updateSnapshot(
    partial: Partial<Omit<PluginConsoleSnapshot, "digest">>,
    snapshotPlugin: RuntimePlugin = plugin,
  ): void {
    const current = readPluginConsoleSnapshot(runtimeClient, snapshotPlugin) ?? {
      digest: snapshotPlugin.digest ?? "",
      components,
      variables,
      setupItems,
    };
    const next = {
      components: partial.components ?? current.components,
      variables: partial.variables ?? current.variables,
      setupItems: partial.setupItems ?? current.setupItems,
    };
    writePluginConsoleSnapshot(runtimeClient, snapshotPlugin, next);
    if (partial.components) setComponents(partial.components);
    if (partial.variables) setVariables(partial.variables);
    if (partial.setupItems) setSetupItems(partial.setupItems);
  }

  function loadPluginSetup() {
    return runtimeClient.getPluginSetup
      ? runtimeClient.getPluginSetup(plugin.name)
      : Promise.resolve({
          items: plugin.setupItems ?? [],
          summary: plugin.setupSummary ?? {
            total: 0,
            ready: 0,
            needsConfiguration: 0,
            needsAuth: 0,
            needsAdminApproval: 0,
            inProgress: 0,
            failed: 0,
            unsupported: 0,
          },
        });
  }

  useEffect(() => {
    const generation = ++loadGenerationRef.current;
    const setupPollGeneration = ++setupPollGenerationRef.current;
    const cached = readPluginConsoleSnapshot(runtimeClient, plugin);
    if (cached) {
      setComponents(cached.components);
      setVariables(cached.variables);
      setSetupItems(cached.setupItems);
      setComponentsLoading(false);
      void restoreActiveSetupSessions(cached.setupItems, setupPollGeneration);
    } else {
      setComponentsLoading(true);
    }
    setErrorMessage(null);
    setDetailFeedback(null);
    setSelectedDetail(null);
    setSourceEditing(false);
    setSourceDraft("");
    void Promise.all([
      runtimeClient.listPluginComponents(plugin.name),
      runtimeClient.getPluginConfiguration(plugin.name),
      loadPluginSetup(),
    ])
      .then(([nextComponents, configuration, setup]) => {
        if (generation !== loadGenerationRef.current) return;
        const snapshot = {
          components: nextComponents ?? [],
          variables: configuration.fields ?? [],
          setupItems: setup.items ?? [],
        };
        writePluginConsoleSnapshot(runtimeClient, plugin, snapshot);
        setComponents(snapshot.components);
        setVariables(snapshot.variables);
        setVariableValues({});
        setSetupItems(snapshot.setupItems);
        void restoreActiveSetupSessions(snapshot.setupItems, setupPollGeneration);
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
      setupPollGenerationRef.current += 1;
      pollingSetupSessionsRef.current = {};
    };
  }, [plugin.name, plugin.digest, runtimeClient]);

  async function refreshSetupAndRuntime(expectedPollGeneration?: number) {
    const [setup, nextPlugin, nextComponents] = await Promise.all([
      loadPluginSetup(),
      runtimeClient.getPlugin(plugin.name),
      runtimeClient.listPluginComponents(plugin.name),
    ]);
    if (
      expectedPollGeneration !== undefined
      && expectedPollGeneration !== setupPollGenerationRef.current
    ) {
      return setup;
    }
    updateSnapshot({ setupItems: setup.items ?? [], components: nextComponents ?? [] }, nextPlugin);
    onPluginUpdated(nextPlugin);
    return setup;
  }

  async function restoreActiveSetupSessions(
    items: RuntimePluginSetupItem[],
    pollGeneration: number,
  ): Promise<void> {
    if (!runtimeClient.getActivePluginAuthSession || !plugin.digest?.trim()) return;
    const pendingItems = items.filter(
      (item) => item.status === "authorizing" || item.status === "verifying",
    );
    if (pendingItems.length === 0) return;
    const recovered = await Promise.all(pendingItems.map(async (item) => {
      try {
        const session = await runtimeClient.getActivePluginAuthSession!(
          plugin.name,
          item.requirement.id,
          setupMutation(item),
        );
        return session ? { setupId: item.requirement.id, session } : null;
      } catch (error) {
        if (pollGeneration === setupPollGenerationRef.current) {
          setErrorMessage(pluginErrorMessage(error));
        }
        return null;
      }
    }));
    if (pollGeneration !== setupPollGenerationRef.current) return;
    const active = recovered.flatMap((entry) => (entry ? [entry] : []));
    if (active.length === 0) return;
    setActiveSetupSessions((current) => {
      const next = { ...current };
      for (const entry of active) next[entry.setupId] = entry.session.sessionId;
      return next;
    });
    for (const entry of active) {
      void pollSetupSession(entry.setupId, entry.session.sessionId, pollGeneration).catch((error) => {
        if (pollGeneration === setupPollGenerationRef.current) {
          setErrorMessage(pluginErrorMessage(error));
        }
      });
    }
  }

  async function pollSetupSession(
    setupId: string,
    sessionId: string,
    pollGeneration = setupPollGenerationRef.current,
  ): Promise<void> {
    if (pollingSetupSessionsRef.current[setupId] === sessionId) return;
    pollingSetupSessionsRef.current[setupId] = sessionId;
    try {
      for (let attempt = 0; attempt < 600; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 1000));
        if (pollGeneration !== setupPollGenerationRef.current) return;
        if (!runtimeClient.getPluginAuthSession) throw new Error("当前 Desktop 不支持插件授权会话");
        const session = await runtimeClient.getPluginAuthSession(plugin.name, sessionId);
        if (pollGeneration !== setupPollGenerationRef.current) return;
        if (session.status === "authorizing" || session.status === "verifying") continue;
        setActiveSetupSessions((current) => {
          if (current[setupId] !== sessionId) return current;
          const next = { ...current };
          delete next[setupId];
          return next;
        });
        const refreshedSetup = await refreshSetupAndRuntime(pollGeneration);
        if (pollGeneration !== setupPollGenerationRef.current) return;
        const refreshedItem = refreshedSetup.items?.find(
          (item) => item.requirement.id === setupId,
        );
        if (session.status === "ready" || refreshedItem?.status === "ready") {
          setErrorMessage(null);
          onNotice("插件账号已连接，关联组件状态已刷新");
        } else if (session.status === "pending_admin_approval") {
          onNotice("已记录等待组织管理员审批；批准后请重新连接");
        } else if (session.errorMessage) {
          setErrorMessage(session.errorMessage);
        }
        return;
      }
    } finally {
      if (pollingSetupSessionsRef.current[setupId] === sessionId) {
        delete pollingSetupSessionsRef.current[setupId];
      }
    }
  }

  function setupMutation(item: RuntimePluginSetupItem, force = false) {
    const pluginDigest = plugin.digest?.trim();
    if (!pluginDigest) throw new Error("插件完整性摘要不可用，请重新打开插件后再试");
    return {
      pluginDigest,
      requirementFingerprint: item.requirement.fingerprint,
      force,
    };
  }

  async function handleSetupAction(itemView: AwakenPluginSetupItem, action: string): Promise<void> {
    const item = setupItems.find((candidate) => candidate.requirement.id === itemView.id);
    if (!item) {
      setErrorMessage("设置项已经变化，请重新打开插件");
      return;
    }
    if (action === "configure") {
      const key = item.requirement.field?.key;
      const component = components.find((candidate) => candidate.component.variable?.key === key);
      if (component) await handleComponentOpen(toConsoleComponent(component));
      return;
    }
    setLocalBusy(true);
    setErrorMessage(null);
    try {
      const request = setupMutation(item, action === "reauthorize");
      if (action === "mark_admin_approval") {
        const sessionId = activeSetupSessions[item.requirement.id];
        if (!sessionId) throw new Error("当前没有可标记的插件授权会话");
        if (!runtimeClient.markPluginAdminApprovalPending) throw new Error("当前 Desktop 不支持管理员审批状态");
        await runtimeClient.markPluginAdminApprovalPending(plugin.name, sessionId);
        setActiveSetupSessions((current) => {
          const next = { ...current };
          delete next[item.requirement.id];
          return next;
        });
        await refreshSetupAndRuntime();
        onNotice("已记录等待组织管理员审批；批准后请重新连接");
        return;
      }
      if (action === "cancel") {
        const sessionId = activeSetupSessions[item.requirement.id];
        if (sessionId) {
          if (!runtimeClient.cancelPluginAuthSession) throw new Error("当前 Desktop 不支持取消插件授权");
          await runtimeClient.cancelPluginAuthSession(plugin.name, sessionId);
        }
        await refreshSetupAndRuntime();
        return;
      }
      if (action === "disconnect") {
        if (!runtimeClient.disconnectPluginSetup) throw new Error("当前 Desktop 不支持退出插件账号");
        const response = await runtimeClient.disconnectPluginSetup(plugin.name, item.requirement.id, request);
        updateSnapshot({ setupItems: response.items ?? [] });
        await refreshSetupAndRuntime();
        return;
      }
      if (action === "probe") {
        if (!runtimeClient.probePluginSetup) throw new Error("当前 Desktop 不支持重新检测插件设置");
        const response = await runtimeClient.probePluginSetup(plugin.name, item.requirement.id, request);
        updateSnapshot({ setupItems: response.items ?? [] });
        await refreshSetupAndRuntime();
        return;
      }
      if (item.requirement.kind === "cli_login" && item.requirement.providerId) {
        if (!runtimeClient.startPluginSetup) throw new Error("当前 Desktop 不支持插件账号连接");
        await runtimeClient.startPluginSetup(plugin.name, item.requirement.id, request);
        await runtimeClient.startCliAuth(item.requirement.providerId as CliProviderId);
        await refreshSetupAndRuntime();
        return;
      }
      if (!runtimeClient.startPluginSetup) throw new Error("当前 Desktop 不支持插件账号连接");
      const session = await runtimeClient.startPluginSetup(plugin.name, item.requirement.id, request);
      if (session.sessionId) {
        setActiveSetupSessions((current) => ({ ...current, [item.requirement.id]: session.sessionId }));
        const pollGeneration = setupPollGenerationRef.current;
        void pollSetupSession(item.requirement.id, session.sessionId, pollGeneration).catch((error) => {
          if (pollGeneration === setupPollGenerationRef.current) {
            setErrorMessage(pluginErrorMessage(error));
          }
        });
      }
      await refreshSetupAndRuntime();
    } catch (error) {
      setErrorMessage(pluginErrorMessage(error));
    } finally {
      setLocalBusy(false);
    }
  }

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
      updateSnapshot({ components: nextComponents ?? [] });
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
      updateSnapshot({ components: nextComponents ?? [] });
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
      updateSnapshot({ components: nextComponents ?? [] }, response.plugin);
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
    const previousEnabled = optimisticEnabled;
    setOptimisticEnabled(enabled);
    setLocalBusy(true);
    setErrorMessage(null);
    setDetailFeedback(null);
    try {
      const next = enabled
        ? await runtimeClient.enablePlugin(plugin.name)
        : await runtimeClient.disablePlugin(plugin.name);
      setOptimisticEnabled(next.enabled);
      onPluginUpdated(next);
      const nextComponents = await runtimeClient.listPluginComponents(plugin.name);
      updateSnapshot({ components: nextComponents ?? [] }, next);
      await reloadSelectedDetail();
    } catch (error) {
      setOptimisticEnabled(previousEnabled);
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
      updateSnapshot({ variables: response.fields ?? [] });
      setVariableValues({});
      const [nextPlugin, nextComponents] = await Promise.all([
        runtimeClient.getPlugin(plugin.name),
        runtimeClient.listPluginComponents(plugin.name),
      ]);
      onPluginUpdated(nextPlugin);
      updateSnapshot({ components: nextComponents ?? [] }, nextPlugin);
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
      builtin={plugin.builtin}
      brandIcon={plugin.brandIcon}
      version={plugin.version}
      enabled={optimisticEnabled}
      components={components.map(toConsoleComponent)}
      setupItems={setupItems.map((item) => ({
        id: item.requirement.id,
        kind: item.requirement.kind,
        title: item.requirement.title || item.requirement.id,
        description: item.requirement.description,
        status: item.status,
        message: item.message,
        required: item.requirement.required,
        sourcePath: item.requirement.sourcePath,
        connectorId: item.requirement.connectorId,
        actions: item.actions,
      }))}
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
      onUninstall={plugin.builtin ? undefined : onUninstall}
      onComponentOpen={(component) => void handleComponentOpen(component)}
      onComponentEnabledChange={(component, enabled) =>
        void handleComponentEnabledChange(component, enabled)
      }
      onSetupAction={(item, action) => void handleSetupAction(item, action)}
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
