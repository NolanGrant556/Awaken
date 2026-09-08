import type { ComposerBarMetaItem, AwakenContextUsagePanelProps } from "@awaken/design-system";
import type { ReasoningEffortLevel } from "../shared/runtime";
import type { createInitialRuntimeState } from "./runtime/reducer";
function formatTokenCount(value?: number): string | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  if (value >= 1_000_000) {
    const amount = value / 1_000_000;
    return `${Number.isInteger(amount) ? amount.toFixed(0) : amount.toFixed(1)}M`;
  }
  if (value >= 1000) {
    const amount = value / 1000;
    return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)}k`;
  }
  return String(value);
}

type RuntimeContextWindow = ReturnType<
  typeof createInitialRuntimeState
>["inspectorSummary"]["contextWindow"];

type RuntimeModelUsage = NonNullable<
  NonNullable<RuntimeContextWindow>["latestModelUsage"]
>;

type RuntimeModelIdentity = {
  id?: string;
  providerId?: string;
  credentialRevision?: number;
  modelId?: string;
  capabilityModelId?: string;
  endpointSelector?: string;
  modelSelectorKind?: "model" | "endpoint";
  protocol?: string;
  reasoningEffort?: ReasoningEffortLevel;
};

function normalizedTokenCount(value: number | undefined): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    return undefined;
  }
  return Math.trunc(value);
}

function safeTokenSum(...values: number[]): number | undefined {
  const total = values.reduce((sum, value) => sum + value, 0);
  return Number.isSafeInteger(total) && total >= 0 ? total : undefined;
}

function providerCurrentInputTokens(
  usage: RuntimeModelUsage | undefined,
): number | undefined {
  if (
    !usage ||
    usage.purpose !== "agent_turn" ||
    usage.status !== "succeeded" ||
    !usage.usageComplete
  ) {
    return undefined;
  }

  const input = normalizedTokenCount(usage.inputTokens);
  if (input !== undefined) {
    return input;
  }

  const cached = normalizedTokenCount(usage.cachedInputTokens);
  const uncached = normalizedTokenCount(usage.uncachedInputTokens);
  if (cached === undefined || uncached === undefined) {
    return undefined;
  }
  const cacheWrite = normalizedTokenCount(usage.cacheWriteTokens) ?? 0;
  return safeTokenSum(cached, uncached, cacheWrite);
}

// Source/tool rebuild boundaries do not change the provider/model identity of
// the last successful request, so its inclusive input remains displayable.
// `agents_instructions_changed` is a prefix-source change like
// `system_prompt_changed`; hard boundaries (provider/model/credential/
// reasoning/compact) stay outside this set and force the local fallback.
const PROVIDER_INPUT_COMPATIBLE_EPOCH_BOUNDARIES = new Set([
  "system_prompt_changed",
  "agents_instructions_changed",
  "tool_removed",
  "tool_schema_changed",
  "canonical_tool_added",
]);

function usageMatchesSnapshot(
  usage: RuntimeModelUsage | undefined,
  snapshot: NonNullable<RuntimeContextWindow>["snapshot"],
  currentModel?: RuntimeModelIdentity,
): boolean {
  if (!usage) {
    return false;
  }
  if (
    currentModel &&
    ((usage.profileId &&
      currentModel.id &&
      usage.profileId !== currentModel.id) ||
      (usage.providerId &&
        currentModel.providerId &&
        usage.providerId !== currentModel.providerId) ||
      (usage.credentialRevision !== undefined &&
        currentModel.credentialRevision !== undefined &&
        usage.credentialRevision !== currentModel.credentialRevision) ||
      (usage.modelId &&
        currentModel.modelId &&
        usage.modelId !== currentModel.modelId) ||
      (usage.capabilityModelId &&
        currentModel.capabilityModelId &&
        usage.capabilityModelId !== currentModel.capabilityModelId) ||
      (usage.endpointSelector &&
        currentModel.endpointSelector &&
        usage.endpointSelector !== currentModel.endpointSelector) ||
      (usage.modelSelectorKind &&
        currentModel.modelSelectorKind &&
        usage.modelSelectorKind !== currentModel.modelSelectorKind) ||
      (usage.protocolFamily &&
        currentModel.protocol &&
        usage.protocolFamily !== currentModel.protocol) ||
      (usage.reasoningPolicy &&
        currentModel.reasoningEffort &&
        usage.reasoningPolicy !== currentModel.reasoningEffort))
  ) {
    return false;
  }
  if (!snapshot) {
    return true;
  }
  if (usage.modelId && snapshot.modelId && usage.modelId !== snapshot.modelId) {
    return false;
  }
  if (
    (usage.capabilityModelId &&
      snapshot.capabilityModelId &&
      usage.capabilityModelId !== snapshot.capabilityModelId) ||
    (usage.endpointSelector &&
      snapshot.endpointSelector &&
      usage.endpointSelector !== snapshot.endpointSelector) ||
    (usage.modelSelectorKind &&
      snapshot.modelSelectorKind &&
      usage.modelSelectorKind !== snapshot.modelSelectorKind)
  ) {
    return false;
  }
  if (
    usage.contextEpochId &&
    snapshot.contextEpochId &&
    usage.contextEpochId !== snapshot.contextEpochId &&
    snapshot.contextEpochBoundaryReason !== undefined &&
    !PROVIDER_INPUT_COMPATIBLE_EPOCH_BOUNDARIES.has(
      snapshot.contextEpochBoundaryReason,
    )
  ) {
    return false;
  }
  return !(
    usage.protocolFamily &&
    snapshot.protocolFamily &&
    usage.protocolFamily !== snapshot.protocolFamily
  );
}

function compatibleProviderInputTokens(
  usage: RuntimeModelUsage | undefined,
  snapshot: NonNullable<RuntimeContextWindow>["snapshot"],
  currentModel?: RuntimeModelIdentity,
): number | undefined {
  if (!usageMatchesSnapshot(usage, snapshot, currentModel)) {
    return undefined;
  }
  return providerCurrentInputTokens(usage);
}

function currentContextTokens(
  contextWindow: RuntimeContextWindow,
  currentModel?: RuntimeModelIdentity,
): number | undefined {
  const usage = contextWindow?.latestModelUsage;
  const providerInput = compatibleProviderInputTokens(
    usage,
    contextWindow?.snapshot,
    currentModel,
  );
  return (
    providerInput ??
    normalizedTokenCount(contextWindow?.snapshot?.estimatedInputTokens) ??
    normalizedTokenCount(contextWindow?.estimatedTokens)
  );
}

export function contextUsageMeta(
  contextWindow: RuntimeContextWindow,
  currentModel?: RuntimeModelIdentity,
): Pick<ComposerBarMetaItem, "label" | "value" | "progress"> | undefined {
  const usageValue = currentContextTokens(contextWindow, currentModel);
  const usage = formatTokenCount(usageValue);
  const effectiveWindowValue =
    contextWindow?.snapshot?.effectiveInputWindowTokens ??
    contextWindow?.effectiveWindow ??
    (typeof contextWindow?.modelWindow === "number"
      ? Math.max(
          0,
          contextWindow.modelWindow - (contextWindow.outputReserve ?? 0),
        )
      : undefined);
  const effectiveWindow = formatTokenCount(effectiveWindowValue);
  if (usage && effectiveWindow) {
    const progress =
      typeof usageValue === "number" &&
      typeof effectiveWindowValue === "number" &&
      effectiveWindowValue > 0
        ? usageValue / effectiveWindowValue
        : undefined;
    return {
      label: "上下文窗口",
      value: `${usage} / ${effectiveWindow}`,
      progress,
    };
  }
  if (usage) {
    return {
      label: "上下文窗口",
      value: usage,
    };
  }
  return undefined;
}

const CONTEXT_PANEL_CONTRIBUTIONS = [
  { id: "system_prompt", label: "系统提示词" },
  { id: "builtin_tools", label: "内置工具" },
  { id: "mcp", label: "MCP" },
  { id: "skills", label: "技能" },
  { id: "memory", label: "记忆" },
  { id: "task_checkpoint", label: "任务检查点" },
  { id: "conversation", label: "对话" },
] as const;

const LEGACY_V3_CONTEXT_PANEL_CONTRIBUTIONS =
  CONTEXT_PANEL_CONTRIBUTIONS.filter(
    (contribution) => contribution.id !== "task_checkpoint",
  );

const CONTEXT_PANEL_OTHER_CONTRIBUTION = {
  id: "other",
  label: "其他/协议开销",
} as const;

function proportionallyAllocateTokens(
  values: number[],
  target: number,
): number[] {
  const sourceTotal = values.reduce((sum, value) => sum + value, 0);
  if (target <= 0) {
    return values.map(() => 0);
  }
  if (sourceTotal <= 0) {
    return values.map((_, index) => (index === values.length - 1 ? target : 0));
  }

  const exact = values.map((value) => (value * target) / sourceTotal);
  const allocated = exact.map((value) => Math.floor(value));
  let remainder = target - allocated.reduce((sum, value) => sum + value, 0);
  const byFraction = exact
    .map((value, index) => ({ index, fraction: value - Math.floor(value) }))
    .sort(
      (left, right) =>
        right.fraction - left.fraction || left.index - right.index,
    );
  for (let index = 0; remainder > 0; index += 1, remainder -= 1) {
    allocated[byFraction[index % byFraction.length].index] += 1;
  }
  return allocated;
}

function reconcileContributionTokens(
  values: number[],
  currentTokens: number,
): { values: number[]; otherTokens: number } {
  const sourceTotal = values.reduce((sum, value) => sum + value, 0);
  const delta = currentTokens - sourceTotal;
  if (delta === 0) {
    return { values, otherTokens: 0 };
  }
  if (delta > 0) {
    return { values, otherTokens: delta };
  }
  return {
    values: proportionallyAllocateTokens(values, currentTokens),
    otherTokens: 0,
  };
}

export function contextUsagePanelData(
  contextWindow: RuntimeContextWindow,
  currentModel?: RuntimeModelIdentity,
): Omit<AwakenContextUsagePanelProps, "id" | "onClose"> | undefined {
  if (!contextWindow) {
    return undefined;
  }
  const snapshot = contextWindow.snapshot;
  const currentInputTokens = currentContextTokens(contextWindow, currentModel);
  const outputReserveTokens =
    snapshot?.outputReserveTokens ?? contextWindow.outputReserve ?? 0;
  const modelWindowTokens =
    snapshot?.modelWindowTokens ?? contextWindow.modelWindow;
  const effectiveInputWindowTokens =
    snapshot?.effectiveInputWindowTokens ??
    contextWindow.effectiveWindow ??
    (typeof modelWindowTokens === "number"
      ? Math.max(0, modelWindowTokens - outputReserveTokens)
      : undefined);
  if (
    typeof currentInputTokens !== "number" ||
    typeof modelWindowTokens !== "number" ||
    typeof effectiveInputWindowTokens !== "number"
  ) {
    return undefined;
  }
  const rawContributions = snapshot?.contributions ?? [];
  const panelContributions =
    snapshot?.schemaVersion === 3 && snapshot.estimatorId === "awaken_leaf_v3"
      ? LEGACY_V3_CONTEXT_PANEL_CONTRIBUTIONS
      : CONTEXT_PANEL_CONTRIBUTIONS;
  const supportedKinds = new Set<string>(
    panelContributions.map((contribution) => contribution.id),
  );
  const hasRetainedContributionContract =
    ((snapshot?.schemaVersion === 4 &&
      snapshot.estimatorId === "awaken_leaf_v4") ||
      (snapshot?.schemaVersion === 3 &&
        snapshot.estimatorId === "awaken_leaf_v3")) &&
    rawContributions.every((contribution) =>
      supportedKinds.has(contribution.kind),
    );
  const tokensByKind = new Map<string, number>();
  if (hasRetainedContributionContract) {
    for (const contribution of rawContributions) {
      tokensByKind.set(
        contribution.kind,
        (tokensByKind.get(contribution.kind) ?? 0) +
          contribution.estimatedTokens,
      );
    }
  }
  const classifiedTokens = Array.from(tokensByKind.values()).reduce(
    (sum, tokens) => sum + tokens,
    0,
  );
  const hasCompleteBreakdown =
    rawContributions.length > 0 &&
    hasRetainedContributionContract &&
    classifiedTokens === snapshot?.estimatedInputTokens;
  const reconciled = hasCompleteBreakdown
    ? reconcileContributionTokens(
        panelContributions.map(
          (contribution) => tokensByKind.get(contribution.id) ?? 0,
        ),
        currentInputTokens,
      )
    : { values: [], otherTokens: 0 };

  const contributions: Array<{
    id: string;
    label: string;
    estimatedTokens: number;
  }> = hasCompleteBreakdown
    ? panelContributions.map((contribution, index) => ({
        ...contribution,
        estimatedTokens: reconciled.values[index],
      }))
    : [];
  if (hasCompleteBreakdown && reconciled.otherTokens > 0) {
    contributions.push({
      ...CONTEXT_PANEL_OTHER_CONTRIBUTION,
      estimatedTokens: reconciled.otherTokens,
    });
  }
  return {
    currentInputTokens,
    effectiveInputWindowTokens,
    modelWindowTokens,
    outputReserveTokens,
    autoCompactThresholdTokens:
      snapshot?.autoCompactThresholdTokens ??
      contextWindow.autoCompactThreshold ??
      0,
    contributions,
    // The usage-details section must share the exact compatibility decision
    // used for the current-total above. Rendering an identity/epoch-
    // incompatible record next to a local-estimate total would show a panel
    // whose "输入 Token" exceeds its own current total.
    latestModelUsage:
      contextWindow.latestModelUsage &&
      usageMatchesSnapshot(
        contextWindow.latestModelUsage,
        snapshot,
        currentModel,
      )
      ? {
          status: contextWindow.latestModelUsage.status,
          cacheMode: contextWindow.latestModelUsage.cacheMode,
          cacheKeyApplied: contextWindow.latestModelUsage.cacheKeyApplied,
          cachedInputTokens: contextWindow.latestModelUsage.cachedInputTokens,
            uncachedInputTokens:
              contextWindow.latestModelUsage.uncachedInputTokens,
          cacheWriteTokens: contextWindow.latestModelUsage.cacheWriteTokens,
          inputTokens: contextWindow.latestModelUsage.inputTokens,
          outputTokens: contextWindow.latestModelUsage.outputTokens,
          reasoningTokens: contextWindow.latestModelUsage.reasoningTokens,
          totalTokens: contextWindow.latestModelUsage.totalTokens,
          calculatedCostUsd: contextWindow.latestModelUsage.calculatedCostUsd,
          currency: contextWindow.latestModelUsage.currency,
          purpose: contextWindow.latestModelUsage.purpose,
          errorCode: contextWindow.latestModelUsage.errorCode,
        }
      : null,
  };
}
