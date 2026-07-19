import type { AguiEvent } from "../../shared/agui";
import type { RunInputAttachment } from "../../shared/runtime";
import {
  toolDisplayStateFromStatus,
  type ToolDisplayState,
} from "../../shared/toolStatus";
import type {
  CompactBoundaryItem,
  PromptNarrativeToken,
  RunAction,
  RunNarrativeItem,
  RunNarrativeState,
} from "./types";

const MAX_SEEN_EVENTS = 1000;
const READ_ONLY_TOOL_NAMES = new Set(["Read", "Glob", "Grep", "WebSearch"]);

export function createInitialRunNarrativeState(): RunNarrativeState {
  return {
    committedItems: [],
    seenEventKeys: [],
  };
}

export function createRunRequestedNarrative(
  prompt: string,
  attachments?: RunInputAttachment[],
  scopeId = "current",
  tokens?: PromptNarrativeToken[],
): RunNarrativeState {
  return appendRunRequestedNarrative(
    createInitialRunNarrativeState(),
    prompt,
    attachments,
    scopeId,
    tokens,
  );
}

export function appendRunRequestedNarrative(
  state: RunNarrativeState,
  prompt: string,
  attachments?: RunInputAttachment[],
  scopeId = "current",
  tokens?: PromptNarrativeToken[],
): RunNarrativeState {
  const trimmed = prompt.trim();
  const settledState = state.activeItem ? commitActive(state, "done") : state;
  const promptAttachments = attachments && attachments.length > 0
    ? attachments.map((attachment) => ({ ...attachment }))
    : undefined;
  const promptTokens = tokens && tokens.length > 0
    ? tokens.map((token) => ({
        ...token,
        payload: token.payload ? { ...token.payload } : undefined,
      }))
    : undefined;
  const committedItems = trimmed || promptAttachments || promptTokens
    ? upsertCommitted(settledState.committedItems, {
        kind: "prompt",
        id: `prompt:${scopeId}`,
        text: trimmed,
        ...(promptAttachments ? { attachments: promptAttachments } : {}),
        ...(promptTokens ? { tokens: promptTokens } : {}),
      })
    : settledState.committedItems;

  return {
    ...settledState,
    committedItems,
    activeItem: {
      kind: "planning",
      id: `planning:${scopeId}`,
      text: "Planning next moves",
      status: "active",
    },
    inspectorSelection: undefined,
    seenEventKeys: [],
  };
}

export function addRunErrorToNarrative(
  state: RunNarrativeState,
  message: string,
  id = "runtime",
): RunNarrativeState {
  const settledState = state.activeItem ? commitActive(state, "done") : state;
  const item: RunNarrativeItem = {
    kind: "answer",
    id: `answer:error:${id}`,
    markdown: message || "Runtime returned an error",
    status: "error",
  };
  return {
    ...settledState,
    committedItems: upsertCommitted(settledState.committedItems, item),
    activeItem: undefined,
  };
}

export function projectAguiEventToNarrative(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const key = eventKey(event);
  if (key && state.seenEventKeys.includes(key)) {
    return state;
  }

  const base = key ? markSeen(state, key) : state;

  switch (event.type) {
    case "RUN_STARTED":
      return commitActive(base, "done");
    case "REASONING_START":
    case "REASONING_MESSAGE_START":
      return activateThinking(base, event);
    case "REASONING_MESSAGE_CONTENT":
      return appendThinking(base, event.delta ?? "", event);
    case "REASONING_MESSAGE_END":
    case "REASONING_END":
      return commitMatchingActive(base, thinkingId(event), "done");
    case "TEXT_MESSAGE_START":
      return activateAnswer(base, event);
    case "TEXT_MESSAGE_CONTENT":
      return appendAnswer(base, event.delta ?? "", event);
    case "TEXT_MESSAGE_END":
      return commitMatchingActiveAnswer(base, event, "done");
    case "TOOL_CALL_START":
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active", "Tool invocation started");
      }
      return activateTool(base, event, "Tool invocation started");
    case "TOOL_CALL_ARGS_DELTA":
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(
          base,
          event,
          "active",
          event.delta ? `Preparing arguments: ${event.delta}` : "Preparing tool arguments",
        );
      }
      return activateTool(
        base,
        event,
        event.delta ? `Preparing arguments: ${event.delta}` : "Preparing tool arguments",
      );
    case "TOOL_CALL_ARGS":
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active", event.delta || "Streaming tool arguments");
      }
      return activateTool(base, event, event.delta || "Streaming tool arguments");
    case "TOOL_PROGRESS": {
      const progress = event.progress || event.message || "Tool is running";
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active", progress);
      }
      return activateTool(base, event, progress);
    }
    case "AWAKEN_TOOL_PERMISSION_DECIDED": {
      // Task 16 unified observability: surface the profile summary, transform
      // rewrite, deny reason, trash notification, and guardian violation in
      // the narrative's tool detail. This event arrives after
      // TOOL_CALL_START (which activated the tool item) and before
      // TOOL_PROGRESS started, so we update the active tool's detail in
      // place. For read-only tools we update the exploring action instead.
      const detail = toolDecisionDetail(event);
      if (!detail) {
        return base;
      }
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active", detail);
      }
      return activateTool(base, event, detail);
    }
    case "TOOL_CALL_END":
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active", "Tool call ready");
      }
      return activateTool(base, event, "Tool call ready");
    case "TOOL_CALL_RESULT": {
      // TrashedPaths are parsed from the tool output after execution and
      // now ride on the ToolResult event (post-execution). Surface the
      // "Deleted file moved to recycle bin" notification in the detail so
      // the user sees that deleted files are recoverable.
      const trashNote =
        event.trashedPaths && event.trashedPaths.length > 0
          ? "Deleted file moved to recycle bin"
          : "";
      const displayStatus = toolDisplayStateFromStatus(event.toolStatus);
      const diagnosticReason =
        typeof event.diagnostic?.reason === "string"
          ? event.diagnostic.reason
          : "";
      if (isReadOnlyToolEvent(event)) {
        const summary = summarizeToolResult(event.content);
        const detail = [
          summary,
          diagnosticReason ? `diagnostic: ${diagnosticReason}` : "",
          trashNote,
        ]
          .filter(Boolean)
          .join("\n");
        return updateExploringAction(base, event, displayStatus, detail, event.toolStatus);
      }
      const baseDetail = event.content || "Tool returned empty content";
      const detail = [
        baseDetail,
        diagnosticReason ? `diagnostic: ${diagnosticReason}` : "",
        trashNote,
      ]
        .filter(Boolean)
        .join("\n");
      return commitTool(base, event, displayStatus, detail, event.toolStatus);
    }
    case "AWAKEN_AUTH_REQUIRED":
      return commitRuntimeNotice(
        base,
        event,
        "auth",
        "Authorization required",
        [event.authProvider, event.authScope, event.authUrl]
          .filter(Boolean)
          .join(" | "),
        "error",
      );
    case "AWAKEN_RUNTIME_PREPARE":
    case "AWAKEN_RUNTIME_PREPARE_PROGRESS":
    case "AWAKEN_RUNTIME_READY":
    case "AWAKEN_RUNTIME_REPAIR_FAILED":
    case "AWAKEN_RUNTIME_MISSING":
      return commitRuntimeNotice(
        base,
        event,
        "runtime",
        event.type === "AWAKEN_RUNTIME_READY"
          ? "Runtime ready"
          : event.type === "AWAKEN_RUNTIME_MISSING" || event.type === "AWAKEN_RUNTIME_REPAIR_FAILED"
            ? "Runtime missing"
            : "Runtime preparing",
        [
          event.runtimeName,
          event.runtimeComponent,
          event.runtimePrepareStatus,
          event.runtimeMissingReason,
          event.message,
        ]
          .filter(Boolean)
          .join(" | "),
        event.type === "AWAKEN_RUNTIME_MISSING" || event.type === "AWAKEN_RUNTIME_REPAIR_FAILED"
          ? "error"
          : event.type === "AWAKEN_RUNTIME_READY"
            ? "done"
            : "active",
      );
    case "AWAKEN_TOOL_RECOVERY_REQUIRED":
    case "AWAKEN_TOOL_RETRY_BLOCKED":
      return commitRuntimeNotice(
        base,
        event,
        event.type === "AWAKEN_TOOL_RETRY_BLOCKED" ? "retry-blocked" : "recovery",
        event.type === "AWAKEN_TOOL_RETRY_BLOCKED"
          ? "Tool retry blocked"
          : event.message?.includes("strategy_change_required")
            ? "Strategy change required"
            : "Tool recovery required",
        [
          event.toolCallName,
          event.errorCode ?? event.code,
          event.message,
          ...(event.recoveryOptions ?? []),
        ]
          .filter(Boolean)
          .join(" | "),
        "error",
      );
    case "AWAKEN_USER_QUESTION_REQUESTED":
    case "AWAKEN_USER_QUESTION_RESOLVED": {
      let detail = "";
      if (event.type === "AWAKEN_USER_QUESTION_REQUESTED") {
        try {
          const parsed = JSON.parse(event.content ?? "{}");
          if (Array.isArray(parsed.questions)) {
            detail = parsed.questions
              .map((q: { question?: string }) => q.question)
              .filter(Boolean)
              .join(" | ");
          }
        } catch {
          detail = event.content ?? "";
        }
      } else {
        detail = event.questionAnswer ?? "";
      }
      return commitRuntimeNotice(
        base,
        event,
        "user-question",
        event.type === "AWAKEN_USER_QUESTION_RESOLVED"
          ? "Answer received"
          : "Waiting for your answer",
        detail,
        event.type === "AWAKEN_USER_QUESTION_REQUESTED" ? "active" : "done",
      );
    }
    case "AWAKEN_ARTIFACT_CREATED":
    case "AWAKEN_ARTIFACT_UPDATED":
      return base;
    case "AWAKEN_COMPACT_STARTED":
    case "AWAKEN_COMPACT_FINISHED":
    case "AWAKEN_COMPACT_FAILED":
      return commitCompactBoundary(base, event);
    case "AWAKEN_MEMORY_RECALLED":
    case "AWAKEN_MEMORY_CREATED":
    case "AWAKEN_MEMORY_UPDATED":
    case "AWAKEN_MEMORY_SUPERSEDED":
    case "AWAKEN_MEMORY_DELETED":
    case "AWAKEN_MEMORY_REJECTED":
      return commitMemoryReceipt(base, event);
    case "AWAKEN_MEMORY_DREAM_COMPLETED":
    case "AWAKEN_MEMORY_DREAM_FAILED":
    case "AWAKEN_MEMORY_DREAM_CANCELLED":
    case "AWAKEN_MEMORY_DREAM_INTERRUPTED":
      return commitDreamReceipt(base, event);
    case "RUN_FINISHED":
      return markLatestAnswerFinal(commitActive(base, "done"), event);
    case "RUN_ERROR":
      if (event.code === "RUN_CANCELLED") {
        return commitRunCancelledHint(base, event, "你停止了生成本次回答");
      }
      return commitErrorAnswer(base, event);
    case "RUN_STATUS_CHANGED":
      if (event.status === "cancelled") {
        return commitRunCancelledHint(
          base,
          event,
          event.message || "你停止生成了本次回答",
        );
      }
      if (event.status === "finished") {
        return markLatestAnswerFinal(commitActive(base, "done"), event);
      }
      return base;
    default:
      return base;
  }
}

function commitMemoryReceipt(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const count = event.memories?.length ?? 0;
  const titles: Record<string, string> = {
    AWAKEN_MEMORY_RECALLED: count > 0 ? `已召回 ${count} 条记忆` : "未召回相关记忆",
    AWAKEN_MEMORY_CREATED: "已写入长期记忆",
    AWAKEN_MEMORY_UPDATED: "已更新长期记忆",
    AWAKEN_MEMORY_SUPERSEDED: "已归档旧记忆",
    AWAKEN_MEMORY_DELETED: "已删除长期记忆",
    AWAKEN_MEMORY_REJECTED: "记忆操作未写入",
  };
  const metadata = (event.memories ?? []).map((memory) =>
    [memory.scope === "project" ? "项目" : "用户", memory.memoryType, `r${memory.revision}`]
      .filter(Boolean)
      .join(" · "),
  );
  const detail = [...metadata, event.errorCode ?? ""].filter(Boolean).join("；");
  const item: RunNarrativeItem = {
    kind: "memoryReceipt",
    id: `memory:${event.recordId ?? eventScopedId(event, event.type)}`,
    title: titles[event.type] ?? "记忆已处理",
    detail: detail || undefined,
    status: event.type === "AWAKEN_MEMORY_REJECTED" ? "error" : "done",
  };
  return { ...state, committedItems: upsertCommitted(state.committedItems, item) };
}

function commitDreamReceipt(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const report = event.diagnostic?.reports as Record<string, { updated?: number; superseded?: number; conflicts?: number }> | undefined;
  const totals = Object.values(report ?? {}).reduce<{ updated: number; superseded: number; conflicts: number }>(
    (sum, item) => ({
      updated: sum.updated + (item.updated ?? 0),
      superseded: sum.superseded + (item.superseded ?? 0),
      conflicts: sum.conflicts + (item.conflicts ?? 0),
    }),
    { updated: 0, superseded: 0, conflicts: 0 },
  );
  const completed = event.type === "AWAKEN_MEMORY_DREAM_COMPLETED";
  const title = completed
    ? "记忆整理完成"
    : event.type === "AWAKEN_MEMORY_DREAM_INTERRUPTED"
      ? "记忆整理因重启中断"
      : event.type === "AWAKEN_MEMORY_DREAM_CANCELLED"
        ? "记忆整理已取消"
        : "记忆整理失败";
  const detail = completed
    ? `更新 ${totals.updated} 条 · 归档 ${totals.superseded} 条 · 冲突 ${totals.conflicts} 条`
    : event.message || event.errorCode;
  const item: RunNarrativeItem = {
    kind: "memoryReceipt",
    id: `dream:${event.recordId ?? eventScopedId(event, event.type)}`,
    title,
    detail,
    status: completed ? "done" : "error",
  };
  return { ...state, committedItems: upsertCommitted(state.committedItems, item) };
}

function eventKey(event: AguiEvent): string | undefined {
  const stableSeq = event.semanticSeq ?? event.seq;
  if (typeof stableSeq === "number") {
    // One durable SemanticRecord can project into multiple AG-UI frames
    // (for example START / CONTENT / END) that intentionally share the
    // record sequence. Deduplicate exact replayed frames, not every frame
    // produced from the same record.
    const frameId =
      event.messageId ||
      event.toolCallId ||
      event.artifactId ||
      event.contextCompactBoundaryId ||
      event.compactBoundaryId ||
      "frame";
    return `seq:${eventScope(event)}:${stableSeq}:${event.type}:${frameId}`;
  }

  if (
    event.type === "TEXT_MESSAGE_CONTENT" ||
    event.type === "REASONING_MESSAGE_CONTENT" ||
    event.type === "TOOL_CALL_ARGS" ||
    event.type === "TOOL_CALL_ARGS_DELTA" ||
    event.type === "TOOL_PROGRESS"
  ) {
    return undefined;
  }

  const id =
    event.messageId ||
    event.toolCallId ||
    event.artifactId ||
    event.contextCompactBoundaryId ||
    event.compactBoundaryId ||
    event.runId ||
    event.type;
  const content = event.delta || event.content || event.status || "";
  return `${event.type}:${eventScope(event)}:${id}:${content}`;
}

function markSeen(state: RunNarrativeState, key: string): RunNarrativeState {
  return {
    ...state,
    seenEventKeys: [...state.seenEventKeys, key].slice(-MAX_SEEN_EVENTS),
  };
}

function upsertCommitted(
  items: RunNarrativeItem[],
  item: RunNarrativeItem,
): RunNarrativeItem[] {
  const index = items.findIndex((candidate) => candidate.id === item.id);
  if (index < 0) {
    return [...items, item];
  }
  const updated = items.slice();
  updated[index] = item;
  return updated;
}

function commitActive(
  state: RunNarrativeState,
  status?: "done" | "error",
): RunNarrativeState {
  if (!state.activeItem) {
    return state;
  }
  const item = applyStatus(state.activeItem, status);
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
    activeItem: undefined,
  };
}

function commitRunCancelledHint(
  state: RunNarrativeState,
  event: AguiEvent,
  text: string,
): RunNarrativeState {
  // Settle whatever is currently streaming (answer, tool trace, etc.) so the
  // cancellation hint can be appended after all committed items.
  const settledState = state.activeItem ? commitActive(state, "done") : state;
  const hint: RunNarrativeItem = {
    kind: "hint",
    id: `hint:run-cancelled:${eventScope(event)}`,
    text,
  };
  return {
    ...settledState,
    committedItems: upsertCommitted(settledState.committedItems, hint),
    activeItem: undefined,
  };
}

function commitMatchingActive(
  state: RunNarrativeState,
  id: string,
  status: "done" | "error",
): RunNarrativeState {
  if (state.activeItem?.id !== id) {
    return state;
  }
  return commitActive(state, status);
}

function commitMatchingActiveAnswer(
  state: RunNarrativeState,
  event: AguiEvent,
  status: "done" | "error",
): RunNarrativeState {
  if (
    state.activeItem?.kind !== "answer" ||
    !isAnswerSegmentId(state.activeItem.id, answerBaseId(event))
  ) {
    return state;
  }
  return commitActive(state, status);
}

function commitErrorAnswer(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  return addRunErrorToNarrative(
    state,
    narrativeErrorMessage(event),
    `${event.seq ?? event.code ?? "runtime"}`,
  );
}

function markLatestAnswerFinal(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  for (let index = state.committedItems.length - 1; index >= 0; index -= 1) {
    const item = state.committedItems[index];
    if (item.kind === "answer" && item.status === "done" && isAnswerForEvent(item.id, event)) {
      const committedItems = state.committedItems.slice();
      committedItems[index] = { ...item, isFinal: true };
      return { ...state, committedItems };
    }
  }
  return state;
}

function isAnswerForEvent(itemId: string, event: AguiEvent): boolean {
  if (!event.runId) {
    return true;
  }
  return itemId === `answer:${event.runId}` || itemId.startsWith(`answer:${event.runId}:`);
}

function setActive(state: RunNarrativeState, item: RunNarrativeItem): RunNarrativeState {
  if (state.activeItem?.id === item.id) {
    return {
      ...state,
      activeItem: item,
    };
  }
  const committed = state.activeItem
    ? upsertCommitted(state.committedItems, applyStatus(state.activeItem, "done"))
    : state.committedItems;
  return {
    ...state,
    committedItems: committed,
    activeItem: item,
  };
}

function applyStatus(
  item: RunNarrativeItem,
  status?: "done" | "error",
): RunNarrativeItem {
  if (!status || item.kind === "prompt") {
    return item;
  }
  switch (item.kind) {
    case "planning":
    case "thinking":
    case "exploring":
    case "fileChange":
      return status === "done" ? { ...item, status } : item;
    case "compactBoundary":
    case "deliverables":
      return item;
    case "tool":
    case "memoryReceipt":
      return status === "done" || status === "error" ? { ...item, status } : item;
    case "answer":
      return status === "done" || status === "error" ? { ...item, status } : item;
    case "hint":
      return item;
  }
}

function thinkingId(event: AguiEvent): string {
  return `thinking:${eventScopedId(event, event.messageId)}`;
}

function answerBaseId(event: AguiEvent): string {
  return `answer:${eventScopedId(event, event.messageId)}`;
}

function isAnswerSegmentId(id: string, baseId: string): boolean {
  return id === baseId || id.startsWith(`${baseId}:segment:`);
}

function answerId(state: RunNarrativeState, event: AguiEvent): string {
  const baseId = answerBaseId(event);
  if (state.activeItem?.kind === "answer" && isAnswerSegmentId(state.activeItem.id, baseId)) {
    return state.activeItem.id;
  }

  const usedIds = state.committedItems
    .filter((item) => item.kind === "answer")
    .map((item) => item.id)
    .filter((id) => isAnswerSegmentId(id, baseId));

  if (usedIds.length === 0) {
    return baseId;
  }

  const usedIdSet = new Set(usedIds);
  let segmentIndex = usedIds.length + 1;
  let segmentId = `${baseId}:segment:${segmentIndex}`;
  while (usedIdSet.has(segmentId)) {
    segmentIndex += 1;
    segmentId = `${baseId}:segment:${segmentIndex}`;
  }
  return segmentId;
}

function narrativeErrorMessage(event: AguiEvent): string {
  const rawMessage = event.message || "Runtime returned an error";
  if (event.code === "PROVIDER_AUTH_MISSING") {
    return "API key 缺失，请在 Settings > Models 配置模型 API key";
  }
  if (event.code === "PROVIDER_REQUEST_FAILED") {
    return `Provider 请求失败：${rawMessage}`;
  }
  return rawMessage;
}

function toolId(event: AguiEvent): string {
  return `tool:${eventScopedId(event, event.toolCallId ?? event.toolCallName)}`;
}

function exploringActionId(event: AguiEvent): string {
  return `explore:${eventScopedId(event, event.toolCallId ?? event.toolCallName)}`;
}

function exploringId(event: AguiEvent): string {
  return `exploring:${eventScope(event)}:${event.toolCallId ?? event.toolCallName ?? "readonly"}`;
}

function compactBoundaryId(event: AguiEvent): string {
  const boundary =
    compactString(event.contextCompactBoundaryId) ||
    compactString(event.compactBoundaryId) ||
    compactBoundaryFromMessage(event.message);
  return `compact:${eventScopedId(event, boundary ?? `${event.type}:${event.seq ?? "latest"}`)}`;
}

function compactString(value?: string): string | undefined {
  const trimmed = value?.trim();
  return trimmed || undefined;
}

function compactStatus(event: AguiEvent): CompactBoundaryItem["status"] {
  if (event.type === "AWAKEN_COMPACT_FAILED") {
    return "failed";
  }
  if (event.type === "AWAKEN_COMPACT_FINISHED") {
    return "finished";
  }
  return "started";
}

function compactMode(event: AguiEvent): string | undefined {
  return compactString(event.contextCompactMode) ||
    compactString(event.compactMode) ||
    compactModeFromMessage(event.message);
}

function compactBoundaryValue(event: AguiEvent): string | undefined {
  return compactString(event.contextCompactBoundaryId) ||
    compactString(event.compactBoundaryId) ||
    compactBoundaryFromMessage(event.message);
}

function compactPreEstimate(event: AguiEvent): number | undefined {
  return finiteNumber(event.contextCompactPreEstimateTokens) ??
    finiteNumber(event.compactPreEstimate) ??
    (event.type === "AWAKEN_COMPACT_STARTED" ? finiteNumber(event.contextEstimatedTokens) : undefined);
}

function compactPostEstimate(event: AguiEvent): number | undefined {
  return finiteNumber(event.contextCompactPostEstimateTokens) ??
    finiteNumber(event.compactPostEstimate) ??
    (event.type === "AWAKEN_COMPACT_FINISHED"
      ? finiteNumber(event.contextEstimatedTokens) ?? compactEstimatedTokensFromMessage(event.message)
      : undefined);
}

function finiteNumber(value?: number): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function compactBoundaryFromMessage(message?: string): string | undefined {
  const match = message?.match(/\bboundary=([^\s]+)/);
  return match?.[1];
}

function compactModeFromMessage(message?: string): string | undefined {
  const match = message?.match(/\bcompact (?:started|finished|failed):\s*([a-z_]+)(?=\s|$)/i);
  return match?.[1]?.toLowerCase();
}

function compactEstimatedTokensFromMessage(message?: string): number | undefined {
  const match = message?.match(/\bestimated_tokens=(\d+)\b/i);
  if (!match) {
    return undefined;
  }
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function compactTitle(status: CompactBoundaryItem["status"], mode?: string): string {
  const modeLabel = mode === "manual" ? "手动" : mode === "auto" || !mode ? "自动" : "";
  if (status === "started") {
    return modeLabel ? `正在${modeLabel}压缩上下文` : "正在压缩上下文";
  }
  if (status === "failed") {
    return modeLabel ? `${modeLabel}压缩上下文失败` : "上下文压缩失败";
  }
  return modeLabel ? `已${modeLabel}压缩上下文` : "已压缩上下文";
}

function formatTokenCount(value: number): string {
  if (value >= 1000) {
    const rounded = Math.round(value / 100) / 10;
    return `${rounded.toLocaleString("en-US")}k`;
  }
  return value.toLocaleString("en-US");
}

function compactDetail(item: {
  boundaryId?: string;
  preEstimate?: number;
  postEstimate?: number;
  fallbackUsed?: boolean;
  status: CompactBoundaryItem["status"];
}): string | undefined {
  const parts: string[] = [];
  if (item.preEstimate !== undefined && item.postEstimate !== undefined) {
    parts.push(`${formatTokenCount(item.preEstimate)} -> ${formatTokenCount(item.postEstimate)}`);
  } else if (item.preEstimate !== undefined) {
    parts.push(`${formatTokenCount(item.preEstimate)} 压缩前`);
  } else if (item.postEstimate !== undefined) {
    parts.push(`${formatTokenCount(item.postEstimate)} 压缩后`);
  }
  if (item.fallbackUsed === true) {
    parts.push("fallback 摘要");
  }
  if (item.status === "failed") {
    parts.push("需要处理");
  }
  if (item.boundaryId) {
    parts.push(item.boundaryId);
  }
  return parts.length > 0 ? parts.join(" · ") : undefined;
}

function eventScope(event: AguiEvent): string {
  return event.runId ?? event.threadId ?? "current";
}

function eventScopedId(event: AguiEvent, value?: string): string {
  if (event.runId && value) {
    return `${event.runId}:${value}`;
  }
  if (event.threadId && value) {
    return `${event.threadId}:${value}`;
  }
  return value ?? eventScope(event);
}

function commitCompactBoundary(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const status = compactStatus(event);
  const mode = compactMode(event);
  const boundaryId = compactBoundaryValue(event);
  const preEstimate = compactPreEstimate(event);
  const postEstimate = compactPostEstimate(event);
  const fallbackUsed = event.compactFallbackUsed;
  const item: CompactBoundaryItem = {
    kind: "compactBoundary",
    id: compactBoundaryId(event),
    status,
    title: compactTitle(status, mode),
    detail: compactDetail({
      boundaryId,
      preEstimate,
      postEstimate,
      fallbackUsed,
      status,
    }),
    mode,
    boundaryId,
    preEstimate,
    postEstimate,
    fallbackUsed,
  };
  const settledState = state.activeItem ? commitActive(state, "done") : state;
  return {
    ...settledState,
    committedItems: upsertCommitted(settledState.committedItems, item),
  };
}

function isReadOnlyToolEvent(event: AguiEvent): boolean {
  return READ_ONLY_TOOL_NAMES.has(event.toolCallName ?? "");
}

// toolDecisionDetail builds a human-readable detail line for the
// AWAKEN_TOOL_PERMISSION_DECIDED event, surfacing the Task 16 unified
// observability data:
//   - transform: "Tool transformed to another registered tool"
//   - deny:      "Denied: sensitive path .git/config"
//   - trash:     "Deleted file moved to recycle bin"
//   - guardian:  "Guardian: <violation reason>"
//   - profile:   "Profile: workspace_write"
//
// Returns "" when the decision is a plain allow with no observability data to
// surface (the common case — the tool simply runs). For allow decisions with
// trashed paths (a delete that went to trash), the trash notification is
// returned so the user sees that files were recoverable.
function toolDecisionDetail(event: AguiEvent): string {
  const action = event.toolDecisionAction;
  const parts: string[] = [];

  if (event.guardianViolation) {
    parts.push(`Guardian: ${event.guardianViolation}`);
  }

  if (action === "transform" && event.transformedFrom && event.transformedTo) {
    parts.push(`${event.transformedFrom} command transformed to ${event.transformedTo}`);
  }

  if (action === "deny") {
    const reason = event.toolDecisionReason || event.message || "denied by policy";
    parts.push(`Denied: ${reason}`);
  }

  if (event.trashedPaths && event.trashedPaths.length > 0) {
    parts.push("Deleted file moved to recycle bin");
  }

  // For a plain allow with no trash/transform/guardian signal, surface the
  // profile name only when there is something notable to show (i.e. not the
  // default workspace_write profile, which would just be noise on every call).
  // We do surface it for deny/transform/guardian/trash since those are the
  // cases where the user benefits from knowing which profile governed the call.
  if (event.profileName && parts.length > 0) {
    parts.unshift(`Profile: ${event.profileName}`);
  }

  return parts.join(" · ");
}

function summarizeToolResult(content?: string): string {
  const normalized = (content ?? "").trim();
  if (!normalized) {
    return "Tool returned empty content";
  }
  const firstLine = normalized.split("\n").find((line) => line.trim()) ?? normalized;
  return firstLine.length > 96 ? `${firstLine.slice(0, 96)}...` : firstLine;
}

function parseToolArgs(delta?: string): Record<string, unknown> | undefined {
  const trimmed = (delta ?? "").trim();
  if (!trimmed || !trimmed.startsWith("{") || !trimmed.endsWith("}")) {
    return undefined;
  }
  try {
    const parsed = JSON.parse(trimmed);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

function stringArg(args: Record<string, unknown> | undefined, key: string): string | undefined {
  const value = args?.[key];
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function numberArg(args: Record<string, unknown> | undefined, key: string): number | undefined {
  const value = args?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function readOnlyActionTitle(event: AguiEvent): string {
  const args = parseToolArgs(event.delta);
  switch (event.toolCallName) {
    case "Read": {
	  const path = stringArg(args, "file_path") ?? "file";
      const offset = numberArg(args, "offset");
      const limit = numberArg(args, "limit");
      const range =
        offset && limit
          ? ` L${offset}-L${offset + limit - 1}`
          : offset
            ? ` from L${offset}`
            : "";
      return `Read ${path}${range}`;
    }
    case "Glob": {
      const pattern = stringArg(args, "pattern");
	  const root = stringArg(args, "path");
      if (pattern && root) {
        return `List ${pattern} in ${root}`;
      }
      if (pattern) {
        return `List ${pattern}`;
      }
      return "List files";
    }
	case "Grep": {
	  const query = stringArg(args, "pattern");
	  const include = stringArg(args, "path") ?? stringArg(args, "glob");
      if (query && include) {
        return `Search "${query}" in ${include}`;
      }
      if (query) {
        return `Search "${query}"`;
      }
      return "Search files";
    }
	case "WebSearch": {
	  const query = stringArg(args, "query");
	  return query ? `Search web for "${query}"` : "Search web";
	}
    default:
      return event.toolCallName || "Explore project";
  }
}

function upsertAction(actions: RunAction[], action: RunAction): RunAction[] {
  const index = actions.findIndex((candidate) => candidate.id === action.id);
  if (index < 0) {
    return [...actions, action];
  }
  const updated = actions.slice();
  updated[index] = {
    ...updated[index],
    ...action,
    title:
      action.title === "Explore project" || action.title.endsWith("file") || action.title.endsWith("files")
        ? updated[index].title
        : action.title,
  };
  return updated;
}

function updateExploringAction(
  state: RunNarrativeState,
  event: AguiEvent,
  status: ToolDisplayState,
  detail?: string,
  toolStatus?: string,
): RunNarrativeState {
  const existing =
    state.activeItem?.kind === "exploring"
      ? state.activeItem
      : undefined;
  const id = existing?.id ?? exploringId(event);
  const action: RunAction = {
    id: exploringActionId(event),
    title: readOnlyActionTitle(event),
    detail,
    status,
    toolStatus,
  };

  return setActive(
    {
      ...state,
      inspectorSelection: { kind: "tool", id: toolId(event) },
    },
    {
      kind: "exploring",
      id,
      actions: upsertAction(existing?.actions ?? [], action),
      status: status === "active" ? "active" : existing?.status ?? "active",
    },
  );
}

function activateThinking(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const item: RunNarrativeItem = {
    kind: "thinking",
    id: thinkingId(event),
    text:
      state.activeItem?.id === thinkingId(event) && state.activeItem.kind === "thinking"
        ? state.activeItem.text
        : "",
    status: "active",
  };
  return setActive(state, item);
}

function appendThinking(
  state: RunNarrativeState,
  delta: string,
  event: AguiEvent,
): RunNarrativeState {
  const id = thinkingId(event);
  const current =
    state.activeItem?.id === id && state.activeItem.kind === "thinking"
      ? state.activeItem.text
      : "";
  return setActive(state, {
    kind: "thinking",
    id,
    text: current + delta,
    status: "active",
  });
}

function activateAnswer(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const id = answerId(state, event);
  const item: RunNarrativeItem = {
    kind: "answer",
    id,
    markdown:
      state.activeItem?.id === id && state.activeItem.kind === "answer"
        ? state.activeItem.markdown
        : "",
    status: "active",
  };
  return setActive(state, item);
}

function appendAnswer(
  state: RunNarrativeState,
  delta: string,
  event: AguiEvent,
): RunNarrativeState {
  const id = answerId(state, event);
  const current =
    state.activeItem?.id === id && state.activeItem.kind === "answer"
      ? state.activeItem.markdown
      : "";
  return setActive(state, {
    kind: "answer",
    id,
    // A durable assistant_message frame is a full snapshot projected from the
    // journal. It replaces any ephemeral deltas already shown for the same
    // message instead of appending the text a second time.
    markdown: event.recordId ? delta : current + delta,
    status: "active",
  });
}

function activateTool(
  state: RunNarrativeState,
  event: AguiEvent,
  detail: string,
): RunNarrativeState {
  const id = toolId(event);
  const existing =
    state.activeItem?.id === id && state.activeItem.kind === "tool"
      ? state.activeItem
      : undefined;
  return setActive(
    {
      ...state,
      inspectorSelection: { kind: "tool", id },
    },
    {
      kind: "tool",
      id,
      title: event.toolCallName || existing?.title || "Tool call",
      detail,
      status: "active",
    },
  );
}

function commitTool(
  state: RunNarrativeState,
  event: AguiEvent,
  status: ToolDisplayState,
  detail: string,
  toolStatus?: string,
): RunNarrativeState {
  const id = toolId(event);
  const active =
    state.activeItem?.id === id && state.activeItem.kind === "tool"
      ? state.activeItem
      : undefined;
  const item: RunNarrativeItem = {
    kind: "tool",
    id,
    title: event.toolCallName || active?.title || "Tool result",
    detail,
    status,
    toolStatus,
  };

  if (active) {
    return {
      ...state,
      committedItems: upsertCommitted(state.committedItems, item),
      activeItem: undefined,
      inspectorSelection: { kind: "tool", id },
    };
  }

  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
    inspectorSelection: { kind: "tool", id },
  };
}

function commitRuntimeNotice(
  state: RunNarrativeState,
  event: AguiEvent,
  prefix: string,
  title: string,
  detail: string,
  status: "active" | "done" | "error",
): RunNarrativeState {
  const id = `${prefix}:${eventScopedId(
    event,
    event.toolCallId ??
      event.runtimeComponent ??
      event.runtimeName ??
      event.authProvider ??
      event.type,
  )}`;
  const item: RunNarrativeItem = {
    kind: "tool",
    id,
    title,
    detail,
    status,
  };

  if (status === "active") {
    return setActive(
      {
        ...state,
        inspectorSelection: { kind: "tool", id },
      },
      item,
    );
  }

  if (state.activeItem?.id === id) {
    return {
      ...state,
      committedItems: upsertCommitted(state.committedItems, item),
      activeItem: undefined,
      inspectorSelection: { kind: "tool", id },
    };
  }
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
    inspectorSelection: { kind: "tool", id },
  };
}
