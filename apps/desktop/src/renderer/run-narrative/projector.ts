import { compactEventDetails } from "../runtime/compact-event";
import type { AguiEvent } from "../../shared/agui";
import type { FileLineComment, RunInputAttachment } from "../../shared/runtime";
import {
  toolDisplayStateFromStatus,
  type ToolDisplayState,
} from "../../shared/toolStatus";
import type {
  ACPDisplayStatus,
  CompactBoundaryItem,
  MediaGenerationItem,
  MediaGenerationStatus,
  PromptNarrativeToken,
  RunAction,
  RunNarrativeItem,
  RunNarrativeState,
  SubagentNarrativeTask,
} from "./types";
import { describeGlobPattern } from "./tool-display";
import { runtimeErrorMessageForDisplay } from "../runtime/error-display";

const MAX_SEEN_EVENTS = 1000;
const MAX_RUN_PLAN_RECEIPT_TOOL_IDS = 200;
const READ_ONLY_TOOL_NAMES = new Set(["Read", "Glob", "Grep", "WebSearch"]);
const MEDIA_TOOL_NAMES = new Set(["image_generate", "video_generate"]);

export function createInitialRunNarrativeState(): RunNarrativeState {
  return {
    committedItems: [],
    seenEventKeys: [],
    replayedItemIds: [],
    runPlanReceiptToolIds: [],
  };
}

export function createRunRequestedNarrative(
  prompt: string,
  attachments?: RunInputAttachment[],
  scopeId = "current",
  tokens?: PromptNarrativeToken[],
  fileComments?: FileLineComment[],
  runId?: string,
  createdAtMs?: number,
): RunNarrativeState {
  return appendRunRequestedNarrative(
    createInitialRunNarrativeState(),
    prompt,
    attachments,
    scopeId,
    tokens,
    fileComments,
    runId,
    createdAtMs,
  );
}

export function appendRunRequestedNarrative(
  state: RunNarrativeState,
  prompt: string,
  attachments?: RunInputAttachment[],
  scopeId = "current",
  tokens?: PromptNarrativeToken[],
  fileComments?: FileLineComment[],
  runId?: string,
  createdAtMs?: number,
): RunNarrativeState {
  const trimmed = prompt.trim();
  const settledState = state.activeItem ? commitActive(state, "done") : state;
  const promptAttachments =
    attachments && attachments.length > 0
    ? attachments.map((attachment) => ({ ...attachment }))
    : undefined;
  const promptTokens =
    tokens && tokens.length > 0
    ? tokens.map((token) => ({
        ...token,
        payload: token.payload ? { ...token.payload } : undefined,
      }))
    : undefined;
  const promptFileComments =
    fileComments && fileComments.length > 0
      ? fileComments.map((comment) => ({ ...comment }))
      : undefined;
  const committedItems =
    trimmed || promptAttachments || promptTokens || promptFileComments
    ? upsertCommitted(settledState.committedItems, {
        kind: "prompt",
        id: `prompt:${scopeId}`,
        ...(runId ? { runId } : {}),
        ...(typeof createdAtMs === "number" && Number.isFinite(createdAtMs)
          ? { createdAtMs }
          : {}),
        text: trimmed,
        ...(promptAttachments ? { attachments: promptAttachments } : {}),
        ...(promptTokens ? { tokens: promptTokens } : {}),
        ...(promptFileComments ? { fileComments: promptFileComments } : {}),
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
    isFinal: true,
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

  const seen = key ? markSeen(state, key) : state;
  const base = eventResumesAfterProviderReconnect(event)
    ? clearProviderReconnectItems(seen)
    : seen;

  switch (event.type) {
    case "RUN_STARTED":
      return commitActive(base, "done");
    case "AWAKEN_TURN_STARTED":
      return activateContinuationPlanning(base, event);
    case "REASONING_START":
    case "REASONING_MESSAGE_START":
      return activateThinking(base, event);
    case "REASONING_MESSAGE_CONTENT":
      return appendThinking(base, event.delta ?? "", event);
    case "REASONING_MESSAGE_END":
    case "REASONING_END":
      if (event.recordId) {
        return base;
      }
      return commitMatchingActive(base, thinkingId(event), "done");
    case "TEXT_MESSAGE_START":
      return activateAnswer(base, event);
    case "TEXT_MESSAGE_CONTENT":
      return appendAnswer(base, event.delta ?? "", event);
    case "TEXT_MESSAGE_END":
      if (event.recordId) {
        return base;
      }
      return commitMatchingActiveAnswer(base, event, "done");
    case "TOOL_CALL_START":
      if (isMediaToolEvent(event)) {
        return activateMediaGeneration(base, event, "preparing");
      }
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active");
      }
      return activateTool(base, event);
    case "TOOL_CALL_ARGS_DELTA":
      {
        const preparation = fileToolPreparation(event);
        if (preparation) {
          return activateTool(
            base,
            event,
            preparation.detail,
            preparation.title,
          );
        }
      }
      // Raw partial JSON remains transport-only. Unknown tools and deltas
      // without Core-derived safe progress do not create narrative state.
      return base;
    case "TOOL_CALL_ARGS": {
      if (isMediaToolEvent(event)) {
        const parameters = mediaParametersFromArgs(event.delta);
        return activateMediaGeneration(
          base,
          event,
          "preparing",
          parameters.detail,
          parameters.requestedCount,
          parameters.requestedAspectRatio,
        );
      }
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active");
      }
      return activateTool(base, event);
    }
    case "TOOL_PROGRESS": {
      // Progress is a side-channel update, never a tool identity authority.
      // If Core did not first deliver a scoped START/ARGS frame, fail closed
      // instead of creating a root tool row from a malformed child frame.
      if (!hasToolNarrativeIdentity(base, event)) {
        return base;
      }
      const progress = event.progress || event.message || "Tool is running";
      if (isMediaToolEvent(event)) {
        return activateMediaGeneration(
          base,
          event,
          mediaStatusFromProgress(event),
          progress === "Tool is running" ? undefined : progress,
        );
      }
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active", progress);
      }
      return activateTool(base, event, progress);
    }
    case "AWAKEN_COMMAND_OUTPUT":
      if (isMediaToolEvent(event) && event.commandStream === "media_status") {
        const update = mediaStatusFromCommandOutput(event.delta);
        return activateMediaGeneration(
          base,
          event,
          update.status,
          update.detail,
          undefined,
          undefined,
          update.taskId,
          update.mediaItems,
        );
      }
      return base;
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
      // This closes the model's argument block; execution ends only at RESULT.
      return base;
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
      if (isMediaToolEvent(event)) {
        return commitMediaGeneration(base, event, diagnosticReason);
      }
      if (isReadOnlyToolEvent(event)) {
        const summary = summarizeToolResult(event.content);
        const detail = [
          summary,
          diagnosticReason ? `diagnostic: ${diagnosticReason}` : "",
          trashNote,
        ]
          .filter(Boolean)
          .join("\n");
        return updateExploringAction(
          base,
          event,
          displayStatus,
          detail,
          event.toolStatus,
        );
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
          : event.type === "AWAKEN_RUNTIME_MISSING" ||
              event.type === "AWAKEN_RUNTIME_REPAIR_FAILED"
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
        event.type === "AWAKEN_RUNTIME_MISSING" ||
          event.type === "AWAKEN_RUNTIME_REPAIR_FAILED"
          ? "error"
          : event.type === "AWAKEN_RUNTIME_READY"
            ? "done"
            : "active",
      );
    case "AWAKEN_TOOL_RECOVERY_REQUIRED":
    case "AWAKEN_TOOL_RETRY_BLOCKED":
      // Recovery supervision describes what the agent must do after the
      // canonical tool result; it is not a second tool execution. The reducer
      // keeps these events as inspector/recovery facts, while the narrative
      // already renders the matching TOOL_CALL_RESULT. Creating another
      // tool-shaped item here double-counts both the tool and the failure.
      return base;
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
        detail =
          event.questionResolution === "cancelled"
            ? "The user chose not to answer; the run continues."
            : (event.questionAnswer ?? "");
      }
      return commitRuntimeNotice(
        base,
        event,
        "user-question",
        event.type === "AWAKEN_USER_QUESTION_RESOLVED"
          ? event.questionResolution === "cancelled"
            ? "Question cancelled"
            : "Answer received"
          : "Waiting for your answer",
        detail,
        event.type === "AWAKEN_USER_QUESTION_REQUESTED" ? "active" : "done",
      );
    }
    case "AWAKEN_ARTIFACT_CREATED":
    case "AWAKEN_ARTIFACT_UPDATED":
    case "AWAKEN_COMPUTER_POINTER_HINT":
      return base;
    case "AWAKEN_SKILL_DISCOVERED":
      return base;
    case "AWAKEN_SKILL_INVOKED":
    case "AWAKEN_SKILL_CONTENT_LOADED":
    case "AWAKEN_SKILL_EXECUTION_CONTEXT":
    case "AWAKEN_SKILL_FAILED_TO_LOAD":
      return commitSkillTrace(base, event);
    case "AWAKEN_HOOK_STARTED":
    case "AWAKEN_HOOK_COMPLETED":
      return commitHookTrace(base, event);
    case "AWAKEN_COMPACT_STARTED":
      return activateCompactPlanning(base, event);
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
    case "AWAKEN_ACP_SESSION_CHANGED":
      return commitACPSession(base, event);
    case "AWAKEN_ACP_TURN_CHANGED":
      return commitACPTurn(base, event);
    case "AWAKEN_ACP_TOOL_CHANGED":
      return commitACPTool(base, event);
    case "AWAKEN_ACP_PERMISSION_REQUESTED":
    case "AWAKEN_ACP_PERMISSION_RESOLVED":
      // Permission gates are handled by the runtime reducer; the narrative only
      // needs to avoid treating these as unknown events.
      return base;
    case "AWAKEN_SUBAGENT_CHANGED":
    case "AWAKEN_SUBAGENT_ACTIVITY":
    case "AWAKEN_SUBAGENT_MESSAGE_QUEUED":
      return upsertSubagentGroup(base, event);
    case "AWAKEN_PLAN_UPDATED":
      return recordRunPlanReceipt(base, event);
    case "AWAKEN_RESEARCH_PHASE":
      return base;
    case "RUN_FINISHED":
      return markLatestAnswerFinal(
        commitActive(clearProviderReconnectItems(base), "done"),
        event,
      );
    case "RUN_ERROR":
      if (event.code === "RUN_CANCELLED") {
        return commitRunCancelledHint(
          clearProviderReconnectItems(base),
          event,
          "你停止了生成本次回答",
        );
      }
      return commitErrorAnswer(clearProviderReconnectItems(base), event);
    case "RUN_STATUS_CHANGED":
      if (event.status === "cancelled") {
        return commitRunCancelledHint(
          clearProviderReconnectItems(base),
          event,
          event.message || "你停止生成了本次回答",
        );
      }
      if (event.status === "finished") {
        return markLatestAnswerFinal(
          commitActive(clearProviderReconnectItems(base), "done"),
          event,
        );
      }
      return base;
    case "AWAKEN_SCHEDULED_NOTICE":
      return commitScheduledNotice(base, event);
    case "AWAKEN_PROVIDER_RECONNECT":
      return projectProviderReconnect(base, event);
    default:
      return base;
  }
}

const continuationPlanningLabels: Record<string, string> = {
  tool_result_continuation: "正在分析工具结果",
  reactive_compact_retry: "上下文压缩中",
  max_output_continuation: "正在继续生成",
  xml_fallback_continuation: "正在继续处理工具结果",
  before_finish_hook_retry: "正在根据 Hook 反馈完善回答",
};

function compactPlanningId(event: AguiEvent): string {
  return `planning:${eventScope(event)}:context-compaction`;
}

function activateCompactPlanning(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  return setActive(state, {
    kind: "planning",
    id: compactPlanningId(event),
    text: "上下文压缩中",
    status: "active",
    phase: "compaction",
  });
}

export function discardInProgressModelOutput(
  state: RunNarrativeState,
  event?: AguiEvent,
): RunNarrativeState {
  const active = state.activeItem;
  const committedItems = state.committedItems.slice();
  while (
    committedItems.length > 0 &&
    (committedItems.at(-1)?.kind === "thinking" ||
      committedItems.at(-1)?.kind === "answer")
  ) {
    committedItems.pop();
  }
  const activeIsModelOutput =
    active?.kind === "thinking" || active?.kind === "answer";
  if (
    !activeIsModelOutput &&
    committedItems.length === state.committedItems.length
  ) {
    return state;
  }
  return {
    ...state,
    committedItems,
    activeItem: {
      kind: "planning",
      id: `planning:${event ? eventScope(event) : "current"}`,
      text: "Planning next moves",
      status: "active",
    },
  };
}

function clearProviderReconnectItems(
  state: RunNarrativeState,
): RunNarrativeState {
  const committedItems = state.committedItems.filter(
    (item) => item.kind !== "providerReconnect",
  );
  const activeItem =
    state.activeItem?.kind === "providerReconnect"
      ? undefined
      : state.activeItem;
  if (
    committedItems.length === state.committedItems.length &&
    activeItem === state.activeItem
  ) {
    return state;
  }
  return { ...state, committedItems, activeItem };
}

function providerReconnectItemId(
  event: AguiEvent,
  phase: "waiting_for_network" | "reconnecting",
): string {
  return `provider-reconnect:${eventScope(event)}:${phase}`;
}

export function eventResumesAfterProviderReconnect(event: AguiEvent): boolean {
  switch (event.type) {
    case "AWAKEN_TURN_STARTED":
    case "REASONING_START":
    case "REASONING_MESSAGE_START":
    case "REASONING_MESSAGE_CONTENT":
    case "REASONING_MESSAGE_END":
    case "REASONING_END":
    case "TEXT_MESSAGE_START":
    case "TEXT_MESSAGE_CONTENT":
    case "TEXT_MESSAGE_END":
    case "TOOL_CALL_START":
    case "TOOL_CALL_END":
    case "TOOL_CALL_RESULT":
      return true;
    default:
      return false;
  }
}

export function restoreLiveProviderReconnectItems(
  state: RunNarrativeState,
  items: readonly Extract<RunNarrativeItem, { kind: "providerReconnect" }>[],
): RunNarrativeState {
  if (items.length === 0) {
    return state;
  }
  const next = reactivateTrailingPlanningForProviderReconnect(state);
  const committedItems = [
    ...next.committedItems.filter((item) => item.kind !== "providerReconnect"),
    ...items,
  ];
  return { ...next, committedItems };
}

function reactivateTrailingPlanningForProviderReconnect(
  state: RunNarrativeState,
): RunNarrativeState {
  if (state.activeItem || state.committedItems.length === 0) {
    return state;
  }
  const trailingItem = state.committedItems.at(-1);
  if (trailingItem?.kind !== "planning") {
    return state;
  }
  return {
    ...state,
    committedItems: state.committedItems.slice(0, -1),
    activeItem: { ...trailingItem, status: "active" },
  };
}

function projectProviderReconnect(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const payload = event.providerReconnect;
  if (!payload || payload.phase === "recovered") {
    return clearProviderReconnectItems(state);
  }
  if (
    payload.phase !== "waiting_for_network" &&
    payload.phase !== "reconnecting"
  ) {
    return state;
  }

  const reset = payload.resetInProgress
    ? discardInProgressModelOutput(state, event)
    : state;
  const next = reactivateTrailingPlanningForProviderReconnect(reset);
  const detail = payload.detail?.trim();
  const item: RunNarrativeItem = {
    kind: "providerReconnect",
    id: providerReconnectItemId(event, payload.phase),
    phase: payload.phase,
    attempt: payload.attempt ?? 0,
    maxAttempts: payload.maxAttempts ?? 0,
    ...(detail ? { detail } : {}),
    ...(typeof event.timestamp === "number" && Number.isFinite(event.timestamp)
      ? { observedAtMs: event.timestamp }
      : {}),
    ...(typeof event.streamCursor === "number" &&
    Number.isFinite(event.streamCursor)
      ? { streamCursor: event.streamCursor }
      : {}),
  };
  return {
    ...next,
    committedItems: upsertCommitted(next.committedItems, item),
  };
}

function activateContinuationPlanning(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const transitionReason = event.transitionReason ?? "";
  const label = continuationPlanningLabels[transitionReason];
  if (!label) {
    return state;
  }
  const turnIdentity =
    event.turnId ?? String(event.turnCount ?? transitionReason);
  return setActive(state, {
    kind: "planning",
    id:
      transitionReason === "reactive_compact_retry"
        ? compactPlanningId(event)
        : `planning:${eventScopedId(event, turnIdentity)}`,
    text: label,
    status: "active",
    phase:
      transitionReason === "reactive_compact_retry"
        ? "compaction"
        : "continuation",
  });
}

function commitSkillTrace(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const skillId = event.skillId?.trim() || "unknown-skill";
  const failed = event.type === "AWAKEN_SKILL_FAILED_TO_LOAD";
  const done =
    event.type === "AWAKEN_SKILL_EXECUTION_CONTEXT" ||
    event.type === "AWAKEN_SKILL_CONTENT_LOADED";
  const source = event.skillSource?.trim();
  const version = event.skillVersion?.trim();
  const detail = [source, version ? `v${version}` : ""]
    .filter(Boolean)
    .join(" · ");
  const output = [
    event.type === "AWAKEN_SKILL_CONTENT_LOADED" && event.skillResource
      ? `已加载资源：${event.skillResource}`
      : "",
    event.skillStatus || "",
    ...(event.skillDiagnostics ?? []),
  ]
    .filter(Boolean)
    .join("\n");
  const item: RunNarrativeItem = {
    kind: "skill",
    id: `skill:${eventScope(event)}:${skillId}`,
    title: skillId,
    detail: detail || undefined,
    output: output || undefined,
    status: failed ? "error" : done ? "done" : "active",
  };
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
  };
}

function commitHookTrace(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const hookId = event.hookId?.trim() || "unknown-hook";
  const invocationId =
    event.hookInvocationId?.trim() ||
    event.recordId ||
    String(event.seq ?? hookId);
  const completed = event.type === "AWAKEN_HOOK_COMPLETED";
  const failed =
    completed && event.status !== "success" && event.status !== "blocked";
  const blocked = [
    "block_run",
    "block_tool",
    "block_compact",
    "block_subagent",
    "block_finish",
  ].includes(event.hookAction ?? "");
  const interrupted = event.status === "interrupted";
  const detail = [
    event.hookEvent,
    event.hookSourceKind,
    event.durationMs != null ? `${event.durationMs} ms` : "",
  ]
    .filter(Boolean)
    .join(" · ");
  const output = [
    event.hookReasonCode,
    event.hookReason,
    event.hookOutputPreview,
  ]
    .filter(Boolean)
    .join("\n");
  const item: RunNarrativeItem = {
    kind: "hook",
    id: `hook:${eventScope(event)}:${invocationId}`,
    title: `Hook · ${hookId}`,
    detail: detail || undefined,
    output: output || undefined,
    status: interrupted
      ? "interrupted"
      : blocked
        ? "blocked"
        : failed
          ? "error"
          : completed
            ? "done"
            : "active",
  };
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
  };
}

function commitScheduledNotice(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const item: RunNarrativeItem = {
    kind: "scheduledNotice",
    id: `scheduled-notice:${eventScope(event)}:${event.executionId ?? event.recordId ?? event.seq ?? event.type}`,
    taskId: event.taskId ?? "",
    executionId: event.executionId,
    taskName: event.taskName,
    noticeContent: event.noticeContent ?? "",
    timestamp: event.timestamp,
  };
  const settledState = state.activeItem ? commitActive(state, "done") : state;
  return {
    ...settledState,
    committedItems: upsertCommitted(settledState.committedItems, item),
  };
}

function commitMemoryReceipt(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const count = event.memories?.length ?? 0;
  const titles: Record<string, string> = {
    AWAKEN_MEMORY_RECALLED:
      count > 0 ? `已召回 ${count} 条记忆` : "未召回相关记忆",
    AWAKEN_MEMORY_CREATED: "已写入长期记忆",
    AWAKEN_MEMORY_UPDATED: "已更新长期记忆",
    AWAKEN_MEMORY_SUPERSEDED: "已归档旧记忆",
    AWAKEN_MEMORY_DELETED: "已删除长期记忆",
    AWAKEN_MEMORY_REJECTED: "记忆操作未写入",
  };
  const metadata = (event.memories ?? []).map((memory) =>
    [
      memory.scope === "project" ? "项目" : "用户",
      memory.memoryType,
      `r${memory.revision}`,
    ]
      .filter(Boolean)
      .join(" · "),
  );
  const detail = [...metadata, event.errorCode ?? ""]
    .filter(Boolean)
    .join("；");
  const item: RunNarrativeItem = {
    kind: "memoryReceipt",
    id: `memory:${event.recordId ?? eventScopedId(event, event.type)}`,
    title: titles[event.type] ?? "记忆已处理",
    detail: detail || undefined,
    status: event.type === "AWAKEN_MEMORY_REJECTED" ? "error" : "done",
  };

	// A mutation receipt is the single user-facing owner for this Memory write.
	// TOOL_CALL_RESULT remains on the wire for model/runtime correctness, but
	// its generic tool row is removed once the typed mutation receipt arrives.
	const mutationToolId = event.toolCallId ? toolId(event) : undefined;
	const committedItems = mutationToolId
    ? state.committedItems.filter(
        (candidate) => candidate.id !== mutationToolId,
      )
		: state.committedItems;
  const activeItem =
    mutationToolId && state.activeItem?.id === mutationToolId
		? undefined
		: state.activeItem;
	return {
		...state,
		committedItems: upsertCommitted(committedItems, item),
		activeItem,
		inspectorSelection:
			mutationToolId && state.inspectorSelection?.id === mutationToolId
				? undefined
				: state.inspectorSelection,
	};
}

function commitDreamReceipt(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const report = event.diagnostic?.reports as
    | Record<
        string,
        { updated?: number; superseded?: number; conflicts?: number }
      >
    | undefined;
  const totals = Object.values(report ?? {}).reduce<{
    updated: number;
    superseded: number;
    conflicts: number;
  }>(
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
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
  };
}

/**
 * Removes the generic Tool row of a successful update_plan call and remembers
 * its receipt identity so a later RESULT cannot recreate it. The task plan is
 * owned exclusively by the ComposerDock projection; neither live nor terminal
 * plan frames create narrative items.
 */
function recordRunPlanReceipt(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const planId = event.planId?.trim();
  const runId = event.runId?.trim();
  if (!planId || !runId || !Array.isArray(event.planItems)) {
    return state;
  }

  // The ComposerDock task surface is the single user-facing owner for this
  // update_plan call. TOOL_CALL_START/RESULT stay on the wire for
  // model/runtime correctness, but the generic tool row is removed once the
  // typed receipt arrives, and the receipt toolCallId is remembered so a
  // later RESULT cannot recreate the row.
  const receiptToolId = event.toolCallId ? toolId(event) : undefined;
  const committedItems = receiptToolId
    ? state.committedItems.filter((candidate) => candidate.id !== receiptToolId)
    : state.committedItems;
  const activeItem =
    receiptToolId && state.activeItem?.id === receiptToolId
      ? undefined
      : state.activeItem;
  return {
    ...state,
    committedItems,
    activeItem,
    inspectorSelection:
      receiptToolId && state.inspectorSelection?.id === receiptToolId
        ? undefined
        : state.inspectorSelection,
    runPlanReceiptToolIds: receiptToolId
      ? [...state.runPlanReceiptToolIds, receiptToolId].slice(
          -MAX_RUN_PLAN_RECEIPT_TOOL_IDS,
        )
      : state.runPlanReceiptToolIds,
  };
}

function acpSessionItemId(event: AguiEvent): string {
  const scope = eventScope(event);
  const runnerId = event.acpRunnerId ?? "unknown";
  const sessionId = event.acpSessionId ?? event.toolCallId ?? "current";
  return `acp-session:${scope}:${runnerId}:${sessionId}`;
}

function acpToolItemId(event: AguiEvent): string {
  const scope = eventScope(event);
  const runnerId = event.acpRunnerId ?? "unknown";
  const sessionId = event.acpSessionId ?? event.toolCallId ?? "current";
  const toolCallId =
    event.acpToolCallId ?? event.toolCallId ?? event.toolCallName ?? "snapshot";
  return `acp-tool:${scope}:${runnerId}:${sessionId}:${toolCallId}`;
}

function acpDisplayStatus(event: AguiEvent): ACPDisplayStatus {
  const status = event.status?.toLowerCase();
  switch (status) {
    case "started":
      return "started";
    case "active":
      return "active";
    case "finished":
    case "done":
      return "done";
    case "closed":
      return "closed";
    case "interrupted":
      return "interrupted";
    case "error":
    case "failed":
      return "error";
    default:
      return "active";
  }
}

function commitACPSession(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const status = acpDisplayStatus(event);
  const runnerId = event.acpRunnerId ?? "unknown";
  const message = event.message ?? event.content ?? "";
  const item: RunNarrativeItem = {
    kind: "acpSession",
    id: acpSessionItemId(event),
    runnerId,
    sessionId: event.acpSessionId,
    status,
    message: message || undefined,
  };
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
  };
}

function commitACPTurn(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const status = acpDisplayStatus(event);
  const runnerId = event.acpRunnerId ?? "unknown";
  const message = event.message ?? event.content ?? "";
  const item: RunNarrativeItem = {
    kind: "acpSession",
    id: acpSessionItemId(event),
    runnerId,
    sessionId: event.acpSessionId,
    status,
    message: message || undefined,
  };
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
  };
}

function commitACPTool(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const runnerId = event.acpRunnerId ?? "unknown";
  const title = event.toolCallName || event.content || "External agent tool";
  const detail =
    event.message && event.message !== event.content
      ? event.message
      : undefined;
  const item: RunNarrativeItem = {
    kind: "acpTool",
    id: acpToolItemId(event),
    runnerId,
    sessionId: event.acpSessionId,
    acpToolCallId: event.acpToolCallId,
    title,
    detail,
    status: "active",
  };
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
  };
}

function eventKey(event: AguiEvent): string | undefined {
  // Argument deltas are live-only and can arrive for every token. Only their
  // Core-derived safe preparation snapshot may affect the active row, so they
  // do not need replay/dedup bookkeeping in Renderer state.
  if (event.type === "TOOL_CALL_ARGS_DELTA") {
    return undefined;
  }
  // Live reconnect frames get unique SSE seq values. Keep that in the seen
  // key so the same phase can update in place; do not collapse by phase.
  if (event.type === "AWAKEN_PROVIDER_RECONNECT") {
    const payload = event.providerReconnect;
    const stableSeq = event.semanticSeq ?? event.seq;
    if (typeof stableSeq === "number") {
      return `seq:${eventScope(event)}:${stableSeq}:${event.type}:reconnect`;
    }
    return [
      event.type,
      eventScope(event),
      payload?.phase ?? "unknown",
      String(payload?.attempt ?? 0),
      payload?.detail ?? "",
      payload?.resetInProgress ? "reset" : "keep",
    ].join(":");
  }
  if (event.type.startsWith("AWAKEN_MEMORY_DREAM_") && event.recordId) {
    return `${event.type}:dream:${event.recordId}`;
  }
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
    event.type === "TOOL_PROGRESS"
  ) {
    return undefined;
  }

  if (
    event.type === "AWAKEN_SUBAGENT_CHANGED" ||
    event.type === "AWAKEN_SUBAGENT_ACTIVITY" ||
    event.type === "AWAKEN_SUBAGENT_MESSAGE_QUEUED"
  ) {
    const taskId = event.subagentTaskId || "task";
    const content = [
      event.subagentStatus,
      event.subagentActivity,
      event.subagentResultPreview,
      event.subagentErrorCode,
      event.toolCallId,
      event.messageId,
    ]
      .filter(Boolean)
      .join(":");
    return `${event.type}:${eventScope(event)}:${taskId}:${content}`;
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

function upsertCommittedBeforeRunAnswer(
  items: RunNarrativeItem[],
  item: RunNarrativeItem,
  event: AguiEvent,
): RunNarrativeItem[] {
  const existingIndex = items.findIndex(
    (candidate) => candidate.id === item.id,
  );
  if (existingIndex >= 0) {
    const updated = items.slice();
    updated[existingIndex] = item;
    return updated;
  }

  const scopeThinkingToTurn = item.kind === "thinking" && Boolean(event.turnId);
  const turnAnswerIndex = scopeThinkingToTurn
    ? items.findIndex(
        (candidate) =>
          candidate.kind === "answer" && candidate.turnId === event.turnId,
      )
    : -1;
  const answerIndex = scopeThinkingToTurn
    ? turnAnswerIndex
    : items.findIndex(
          (candidate) =>
          candidate.kind === "answer" && isAnswerForEvent(candidate.id, event),
        );
  if (answerIndex < 0) {
    return [...items, item];
  }

  const updated = items.slice();
  updated.splice(answerIndex, 0, item);
  return updated;
}

function upsertSubagentGroup(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const taskId = event.subagentTaskId?.trim();
  if (!taskId) return state;
  const id = `subagents:${eventScope(event)}`;
  const existing = state.committedItems.find(
    (item): item is Extract<RunNarrativeItem, { kind: "subagentGroup" }> =>
      item.kind === "subagentGroup" && item.id === id,
  );
  const previous = existing?.tasks.find((task) => task.taskId === taskId);
  const incomingStatus = event.subagentStatus ?? previous?.status ?? "running";
  const previousTerminal = previous
    ? ["completed", "failed", "cancelled", "interrupted"].includes(
        previous.status,
      )
    : false;
  const transitionAllowed =
    !previous ||
    previous.status === incomingStatus ||
    (previous.status === "queued" &&
      [
        "running",
        "cancelling",
        "cancelled",
        "interrupted",
        "failed",
        "completed",
      ].includes(incomingStatus)) ||
    (previous.status === "running" &&
      [
        "cancelling",
        "completed",
        "failed",
        "cancelled",
        "interrupted",
      ].includes(incomingStatus)) ||
    (previous.status === "cancelling" &&
      ["completed", "failed", "cancelled", "interrupted"].includes(
        incomingStatus,
      ));
  const status =
    previousTerminal || !transitionAllowed ? previous!.status : incomingStatus;
  const terminal = ["completed", "failed", "cancelled", "interrupted"].includes(
    status,
  );
  const task: SubagentNarrativeTask = {
    taskId,
    description:
      event.subagentDescription?.trim() ||
      previous?.description ||
      "未命名子任务",
    agentType: event.subagentAgentType ?? previous?.agentType ?? "explore",
    workspaceMode: event.subagentWorkspaceMode ?? previous?.workspaceMode,
    status,
    activity: terminal
      ? undefined
      : event.subagentActivity?.trim() ||
        event.toolCallName?.trim() ||
        previous?.activity,
    toolUseCount: event.subagentToolUseCount ?? previous?.toolUseCount ?? 0,
    estimatedTokens: event.subagentEstimatedTokens ?? previous?.estimatedTokens,
    inputTokens: event.subagentInputTokens ?? previous?.inputTokens,
    outputTokens: event.subagentOutputTokens ?? previous?.outputTokens,
    totalTokens: event.subagentTotalTokens ?? previous?.totalTokens,
    createdAt: event.subagentCreatedAt ?? previous?.createdAt,
    startedAt: event.subagentStartedAt ?? previous?.startedAt,
    finishedAt: event.subagentFinishedAt ?? previous?.finishedAt,
    resultPreview:
      event.subagentResultPreview?.trim() || previous?.resultPreview,
    errorCode: event.subagentErrorCode ?? previous?.errorCode,
    errorMessage: event.subagentErrorMessage ?? previous?.errorMessage,
  };
  const tasks = existing ? existing.tasks.slice() : [];
  const index = tasks.findIndex((candidate) => candidate.taskId === taskId);
  if (index >= 0) tasks[index] = task;
  else tasks.push(task);
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, {
      kind: "subagentGroup",
      id,
      tasks,
    }),
  };
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
  const settledState =
    state.activeItem?.kind === "mediaGeneration"
      ? {
          ...state,
          committedItems: upsertCommitted(state.committedItems, {
            ...state.activeItem,
            status: "cancelled" as const,
            detail: "生成已停止",
          }),
          activeItem: undefined,
        }
      : state.activeItem?.kind === "tool"
        ? {
            ...state,
            committedItems: upsertCommitted(state.committedItems, {
              ...state.activeItem,
              status: "cancelled" as const,
            }),
            activeItem: undefined,
          }
        : state.activeItem?.kind === "exploring"
          ? {
              ...state,
              committedItems: upsertCommitted(state.committedItems, {
                ...state.activeItem,
                actions: state.activeItem.actions.map((action) => ({
                  ...action,
                  status:
                    action.status === "active"
                      ? ("cancelled" as const)
                      : action.status,
                })),
                status: "done" as const,
              }),
              activeItem: undefined,
            }
          : state.activeItem
            ? commitActive(state, "done")
            : state;
  const hint: RunNarrativeItem = {
    kind: "hint",
    id: `hint:run-cancelled:${eventScope(event)}`,
    text,
    isTerminal: true,
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

function commitErrorAnswer(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  return addRunErrorToNarrative(
    state,
    narrativeErrorMessage(event),
    `${event.seq ?? event.code ?? "runtime"}`,
  );
}

function markLatestAnswerFinal(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  for (let index = state.committedItems.length - 1; index >= 0; index -= 1) {
    const item = state.committedItems[index];
    if (
      item.kind === "answer" &&
      item.status === "done" &&
      isAnswerForEvent(item.id, event)
    ) {
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
  return (
    itemId === `answer:${event.runId}` ||
    itemId.startsWith(`answer:${event.runId}:`)
  );
}

function setActive(
  state: RunNarrativeState,
  item: RunNarrativeItem,
): RunNarrativeState {
  if (state.activeItem?.id === item.id) {
    return {
      ...state,
      activeItem: item,
    };
  }
  const committed = state.activeItem
    ? upsertCommitted(
        state.committedItems,
        applyStatus(state.activeItem, "done"),
      )
    : state.committedItems;
  return {
    ...state,
    // A durable snapshot can reactivate an item that was already committed by
    // the live stream. Keep one owner for the item while it is active so React
    // never receives the same narrative id twice.
    committedItems: committed.filter((candidate) => candidate.id !== item.id),
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
    case "subagentGroup":
      return item;
    case "mediaGeneration":
      return status === "error" ? { ...item, status: "error" } : item;
    case "tool":
    case "skill":
    case "hook":
    case "memoryReceipt":
    case "acpSession":
    case "acpTool":
      return status === "done" || status === "error"
        ? { ...item, status }
        : item;
    case "answer":
      return status === "done" || status === "error"
        ? { ...item, status }
        : item;
    case "scheduledNotice":
    case "hint":
    case "providerReconnect":
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
  if (
    state.activeItem?.kind === "answer" &&
    isAnswerSegmentId(state.activeItem.id, baseId)
  ) {
    return state.activeItem.id;
  }

  // A durable assistant_message is the canonical snapshot for this message.
  // If reasoning replay temporarily moved the streamed answer into committed
  // items, reconcile the snapshot back into the same base identity instead of
  // inventing a second answer segment.
  if (
    event.recordId &&
    state.committedItems.some(
      (item) => item.kind === "answer" && item.id === baseId,
    )
  ) {
    return baseId;
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
  return runtimeErrorMessageForDisplay(event.code, event.message);
}

function toolId(event: AguiEvent): string {
  return `tool:${eventScopedId(event, event.toolCallId ?? event.toolCallName)}`;
}

function mediaGenerationId(event: AguiEvent): string {
  return `media:${eventScopedId(event, event.toolCallId ?? event.toolCallName)}`;
}

function exploringActionId(event: AguiEvent): string {
  return `explore:${eventScopedId(event, event.toolCallId ?? event.toolCallName)}`;
}

function exploringId(event: AguiEvent): string {
  return `exploring:${eventScope(event)}:${event.toolCallId ?? event.toolCallName ?? "readonly"}`;
}

function hasToolNarrativeIdentity(
  state: RunNarrativeState,
  event: AguiEvent,
): boolean {
  if (!event.toolCallId?.trim()) {
    return false;
  }
  const expectedToolId = toolId(event);
  const expectedMediaId = mediaGenerationId(event);
  const expectedActionId = exploringActionId(event);
  const items = [
    ...state.committedItems,
    ...(state.activeItem ? [state.activeItem] : []),
  ];
  return items.some((item) => {
    if (item.kind === "tool") {
      return item.id === expectedToolId;
    }
    if (item.kind === "mediaGeneration") {
      return item.id === expectedMediaId;
    }
    if (item.kind === "exploring") {
      return item.actions.some((action) => action.id === expectedActionId);
    }
    return false;
  });
}

function finiteNumber(value?: number): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function compactTitle(
  status: CompactBoundaryItem["status"],
  mode?: string,
): string {
  const modeLabel =
    mode === "manual" ? "手动" : mode === "auto" || !mode ? "自动" : "";
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
    parts.push(
      `${formatTokenCount(item.preEstimate)} -> ${formatTokenCount(item.postEstimate)}`,
    );
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

function commitCompactBoundary(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  const { status, mode, boundaryId, preCompactEstimatedTokens: preEstimate,
    postCompactEstimatedTokens: postEstimate } = compactEventDetails(event);
  const fallbackUsed = event.compactFallbackUsed;
  const targetTokens = finiteNumber(event.contextCompactTarget);
  const mandatoryFloorTokens = finiteNumber(event.compactMandatoryFloor);
  const localProjectedTokens = finiteNumber(event.compactLocalProjected);
  const conservativeProjectedTokens = finiteNumber(
    event.compactConservativeProjected,
  );
  const item: CompactBoundaryItem = {
    kind: "compactBoundary",
    id: `compact:${eventScopedId(event, boundaryId ?? `${event.type}:${event.seq ?? "latest"}`)}`,
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
    targetTokens,
    mandatoryFloorTokens,
    localProjectedTokens,
    conservativeProjectedTokens,
    contributionTokens: event.compactContributionTokens,
    nativeAttempted: event.compactNativeAttempted,
    nativeUsed: event.compactNativeUsed,
    nativeReason: event.compactNativeReason,
    failureReason: event.compactFailureReason,
  };
  const compactPlanningActive =
    state.activeItem?.kind === "planning" &&
    state.activeItem.id === compactPlanningId(event);
  const settledState =
    state.activeItem && !compactPlanningActive
      ? commitActive(state, "done")
      : state;
  return {
    ...settledState,
    committedItems: upsertCommitted(settledState.committedItems, item),
  };
}

function isReadOnlyToolEvent(event: AguiEvent): boolean {
  return READ_ONLY_TOOL_NAMES.has(event.toolCallName ?? "");
}

function isMediaToolEvent(event: AguiEvent): boolean {
  return MEDIA_TOOL_NAMES.has(event.toolCallName ?? "");
}

function mediaKind(event: AguiEvent): "image" | "video" {
  return event.toolCallName === "video_generate" ? "video" : "image";
}

function mediaParametersFromArgs(value?: string): {
  detail?: string;
  requestedCount?: number;
  requestedAspectRatio?: number;
} {
  const trimmed = value?.trim();
  if (!trimmed || !trimmed.startsWith("{")) return {};
  try {
    const parsed = JSON.parse(trimmed) as {
      prompt?: unknown;
      mode?: unknown;
      max_images?: unknown;
      size?: unknown;
      ratio?: unknown;
    };
    const prompt =
      typeof parsed.prompt === "string" ? parsed.prompt.trim() : "";
    const requestedCount =
      parsed.mode === "group" &&
      typeof parsed.max_images === "number" &&
      Number.isInteger(parsed.max_images) &&
      parsed.max_images >= 1 &&
      parsed.max_images <= 15
        ? parsed.max_images
        : undefined;
    const ratioValue =
      typeof parsed.ratio === "string"
      ? parsed.ratio
      : typeof parsed.size === "string"
        ? parsed.size
        : "";
    const ratioMatch = ratioValue
      .trim()
      .match(/^(\d+(?:\.\d+)?)\s*[:x×]\s*(\d+(?:\.\d+)?)$/i);
    const requestedAspectRatio = ratioMatch
      ? Number(ratioMatch[1]) / Number(ratioMatch[2])
      : undefined;
    return {
      ...(prompt
        ? { detail: prompt.length > 96 ? `${prompt.slice(0, 93)}...` : prompt }
        : {}),
      ...(requestedCount ? { requestedCount } : {}),
      ...(requestedAspectRatio && Number.isFinite(requestedAspectRatio)
        ? { requestedAspectRatio }
        : {}),
    };
  } catch {
    return {};
  }
}

function mediaStatusFromProgress(event: AguiEvent): MediaGenerationStatus {
  if (event.transportStatus === "unknown") return "unknown";
  switch (event.toolProgressStage) {
    case "failed":
    case "needs_auth":
    case "needs_runtime":
    case "unsupported":
      return "error";
    default:
      return "generating";
  }
}

function mediaStatusFromCommandOutput(value?: string): {
  status: MediaGenerationStatus;
  detail?: string;
  taskId?: string;
  mediaItems?: MediaGenerationItem[];
} {
  const line = value?.trim().split(/\r?\n/).filter(Boolean).at(-1) ?? "";
  try {
    const parsed = JSON.parse(line) as {
      stage?: unknown;
      message?: unknown;
      task_id?: unknown;
      items?: unknown;
    };
    const stage =
      typeof parsed.stage === "string" ? parsed.stage : "generating";
    const status: MediaGenerationStatus =
      stage === "queued" || stage === "downloading" || stage === "ready"
        ? stage
        : stage === "partial"
          ? "partial"
          : stage === "unknown"
            ? "unknown"
            : stage === "cancelled"
              ? "cancelled"
              : stage === "error"
                ? "error"
                : "generating";
    const mediaItems = parseMediaItems(parsed.items);
    return {
      status,
      detail: typeof parsed.message === "string" ? parsed.message : undefined,
      ...(typeof parsed.task_id === "string" ? { taskId: parsed.task_id } : {}),
      ...(mediaItems.length > 0 ? { mediaItems } : {}),
    };
  } catch {
    return { status: "generating", detail: line || undefined };
  }
}

function validMediaCount(value: unknown): number | undefined {
  return typeof value === "number" &&
    Number.isInteger(value) &&
    value >= 1 &&
    value <= 15
    ? value
    : undefined;
}

function parseMediaItems(value: unknown): MediaGenerationItem[] {
  if (!Array.isArray(value)) return [];
  const items: MediaGenerationItem[] = [];
  for (const candidate of value) {
    if (!candidate || typeof candidate !== "object") continue;
    const item = candidate as {
      index?: unknown;
      status?: unknown;
      path?: unknown;
    };
    const index = validMediaCount(item.index);
    if (
      !index ||
      (item.status !== "ready" &&
        item.status !== "error" &&
        item.status !== "unknown")
    ) {
      continue;
    }
    items.push({
      index,
      status: item.status,
      ...(typeof item.path === "string" && item.path
        ? { path: item.path }
        : {}),
    });
  }
  return items.sort((left, right) => left.index - right.index);
}

function activateMediaGeneration(
  state: RunNarrativeState,
  event: AguiEvent,
  status: MediaGenerationStatus,
  detail?: string,
  requestedCount?: number,
  requestedAspectRatio?: number,
  taskId?: string,
  mediaItems?: MediaGenerationItem[],
): RunNarrativeState {
  const id = mediaGenerationId(event);
  const existing =
    state.activeItem?.id === id && state.activeItem.kind === "mediaGeneration"
      ? state.activeItem
      : state.committedItems.find(
          (
            item,
          ): item is Extract<RunNarrativeItem, { kind: "mediaGeneration" }> =>
            item.id === id && item.kind === "mediaGeneration",
        );
  const kind = mediaKind(event);
  return setActive(
    {
      ...state,
      committedItems: state.committedItems.filter((item) => item.id !== id),
      inspectorSelection: { kind: "tool", id: toolId(event) },
    },
    {
      kind: "mediaGeneration",
      id,
      toolCallId: event.toolCallId ?? existing?.toolCallId ?? "media",
      runId: event.runId ?? existing?.runId,
      mediaKind: kind,
      title: kind === "image" ? "图片生成" : "视频生成",
      detail: detail || existing?.detail,
      status,
      ...((requestedCount ?? existing?.requestedCount)
        ? { requestedCount: requestedCount ?? existing?.requestedCount }
        : {}),
      ...((requestedAspectRatio ?? existing?.requestedAspectRatio)
        ? {
            requestedAspectRatio:
              requestedAspectRatio ?? existing?.requestedAspectRatio,
          }
        : {}),
      ...((taskId ?? existing?.taskId)
        ? { taskId: taskId ?? existing?.taskId }
        : {}),
      ...((mediaItems ?? existing?.mediaItems)
        ? { mediaItems: mediaItems ?? existing?.mediaItems }
        : {}),
      artifactPaths: existing?.artifactPaths ?? [],
    },
  );
}

function commitMediaGeneration(
  state: RunNarrativeState,
  event: AguiEvent,
  diagnosticReason?: string,
): RunNarrativeState {
  const id = mediaGenerationId(event);
  const existing =
    state.activeItem?.id === id && state.activeItem.kind === "mediaGeneration"
      ? state.activeItem
      : state.committedItems.find(
          (
            item,
          ): item is Extract<RunNarrativeItem, { kind: "mediaGeneration" }> =>
            item.id === id && item.kind === "mediaGeneration",
        );
  let paths: string[] = [];
  let payloadSucceeded = false;
  let payloadStatus: string | undefined;
  let payloadItems: MediaGenerationItem[] = [];
  let payloadRequestedCount: number | undefined;
  let payloadTaskId: string | undefined;
  try {
    const payload = JSON.parse(event.content ?? "{}") as {
      status?: unknown;
      generation_status?: unknown;
      paths?: unknown;
      items?: unknown;
      requested_count?: unknown;
      task_id?: unknown;
    };
    paths = Array.isArray(payload.paths)
      ? payload.paths.filter(
          (value): value is string =>
            typeof value === "string" && value.length > 0,
        )
      : [];
    const generationStatus = payload.generation_status ?? payload.status;
    payloadSucceeded = generationStatus === "succeeded" && paths.length > 0;
    payloadStatus =
      typeof generationStatus === "string" ? generationStatus : undefined;
    payloadItems = parseMediaItems(payload.items);
    payloadRequestedCount = validMediaCount(payload.requested_count);
    payloadTaskId =
      typeof payload.task_id === "string" ? payload.task_id : undefined;
  } catch {
    // Structured result parsing is best-effort; canonical toolStatus remains authoritative otherwise.
  }
  const displayStatus = toolDisplayStateFromStatus(event.toolStatus);
  const status: MediaGenerationStatus = payloadSucceeded
    ? "ready"
    : payloadStatus === "partial" && paths.length > 0
      ? "partial"
      : payloadStatus === "unknown" || event.transportStatus === "unknown"
        ? "unknown"
        : displayStatus === "cancelled" || displayStatus === "interrupted"
          ? "cancelled"
          : displayStatus === "done"
            ? "ready"
            : "error";
  const kind = mediaKind(event);
  const item: RunNarrativeItem = {
    kind: "mediaGeneration",
    id,
    toolCallId: event.toolCallId ?? existing?.toolCallId ?? "media",
    runId: event.runId ?? existing?.runId,
    mediaKind: kind,
    title: kind === "image" ? "图片生成" : "视频生成",
    detail:
      status === "ready"
        ? kind === "image"
          ? `已生成 ${paths.length || existing?.artifactPaths.length || 1} 张图片`
          : "视频已生成"
        : status === "partial"
          ? `已生成 ${paths.length} 张图片，部分结果未完成`
          : status === "unknown"
            ? "任务已提交，当前状态待确认"
            : diagnosticReason || event.content || existing?.detail,
    status,
    ...((payloadRequestedCount ?? existing?.requestedCount)
      ? { requestedCount: payloadRequestedCount ?? existing?.requestedCount }
      : {}),
    ...(existing?.requestedAspectRatio
      ? { requestedAspectRatio: existing.requestedAspectRatio }
      : {}),
    ...((payloadTaskId ?? existing?.taskId)
      ? { taskId: payloadTaskId ?? existing?.taskId }
      : {}),
    ...(payloadItems.length > 0 || existing?.mediaItems
      ? {
          mediaItems:
            payloadItems.length > 0 ? payloadItems : existing?.mediaItems,
        }
      : {}),
    artifactPaths: paths.length > 0 ? paths : (existing?.artifactPaths ?? []),
  };
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
    activeItem: state.activeItem?.id === id ? undefined : state.activeItem,
    inspectorSelection: { kind: "tool", id: toolId(event) },
  };
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
    parts.push(
      `${event.transformedFrom} command transformed to ${event.transformedTo}`,
    );
  }

  if (action === "deny") {
    const reason =
      event.toolDecisionReason || event.message || "denied by policy";
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
  const firstLine =
    normalized.split("\n").find((line) => line.trim()) ?? normalized;
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

function stringArg(
  args: Record<string, unknown> | undefined,
  key: string,
): string | undefined {
  const value = args?.[key];
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function numberArg(
  args: Record<string, unknown> | undefined,
  key: string,
): number | undefined {
  const value = args?.[key];
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function compactPathLabel(value: string): string {
  const normalized = value
    .replace(/[\u0000-\u001f\u007f]/g, "")
    .trim()
    .replace(/[\\/]+$/, "");
  const segments = normalized.split(/[\\/]/).filter(Boolean);
  const label = segments.at(-1) || normalized;
  return label.length > 72 ? `${label.slice(0, 71).trimEnd()}…` : label;
}

function safePreparationLineCount(value: number | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0)
    return 0;
  return Math.min(Math.floor(value), 10_000_000);
}

function fileToolPreparation(
  event: AguiEvent,
): { title: string; detail: string } | undefined {
  const kind = event.toolPreparationKind;
  if (kind !== "write" && kind !== "edit") return undefined;
  if (event.toolCallName?.trim().toLowerCase() !== kind) return undefined;

  const added = safePreparationLineCount(event.toolPreparationAddedLines);
  const deleted = safePreparationLineCount(event.toolPreparationDeletedLines);
  const path = compactPathLabel(event.toolPreparationPath ?? "");
  const action = kind === "write" ? "写入" : "编辑";
  return {
    title: path ? `${action} ${path}` : `${action}文件`,
    detail: kind === "write" ? `+${added} 行` : `+${added} −${deleted} 行`,
  };
}

function compactSearchScope(value: string): string {
  return /[*?[\]{}]/.test(value)
    ? describeGlobPattern(value)
    : compactPathLabel(value);
}

function readOnlyActionTitle(event: AguiEvent): string {
  const args = parseToolArgs(event.delta);
  switch (event.toolCallName) {
    case "Read": {
      const path = compactPathLabel(stringArg(args, "file_path") ?? "文件");
      const offset = numberArg(args, "offset");
      const limit = numberArg(args, "limit");
      const range =
        offset && limit
          ? ` L${offset}-L${offset + limit - 1}`
          : offset
            ? ` from L${offset}`
            : "";
      return `读取 ${path}${range}`;
    }
    case "Glob": {
      const pattern = stringArg(args, "pattern");
      const root = stringArg(args, "path");
      if (pattern && root) {
        return `搜索 ${describeGlobPattern(pattern)} · ${compactPathLabel(root)}`;
      }
      if (pattern) {
        return `搜索 ${describeGlobPattern(pattern)}`;
      }
      return "搜索文件";
    }
    case "Grep": {
      const query = stringArg(args, "pattern");
      const include = stringArg(args, "path") ?? stringArg(args, "glob");
      if (query && include) {
        return `搜索 "${query}" · ${compactSearchScope(include)}`;
      }
      if (query) {
        return `搜索 "${query}"`;
      }
      return "搜索文件";
    }
    case "WebSearch": {
      const query = stringArg(args, "query");
      return query ? `搜索网页 "${query}"` : "搜索网页";
    }
    default:
      return event.toolCallName || "检查项目";
  }
}

function isGenericReadOnlyActionTitle(title: string): boolean {
  return [
    "Explore project",
    "Read file",
    "List files",
    "Search files",
    "Search web",
    "检查项目",
    "读取 文件",
    "搜索文件",
    "搜索网页",
  ].includes(title);
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
    title: isGenericReadOnlyActionTitle(action.title)
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
    state.activeItem?.kind === "exploring" ? state.activeItem : undefined;
  const id = existing?.id ?? exploringId(event);
  const action: RunAction = {
    id: exploringActionId(event),
    toolCallId: event.toolCallId,
    visualKind: ["Glob", "Grep", "WebSearch"].includes(event.toolCallName ?? "")
      ? "search"
      : undefined,
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
      status: status === "active" ? "active" : (existing?.status ?? "active"),
    },
  );
}

function activateThinking(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  // Durable reasoning_summary records are canonical snapshots of reasoning
  // that the live stream has normally already shown. Their synthetic
  // START/CONTENT/END frames must not reactivate a completed Thought: doing so
  // remounts its active UI just before the terminal process fold and creates a
  // visible one-frame layout flash.
  if (event.recordId) {
    return state;
  }
  const item: RunNarrativeItem = {
    kind: "thinking",
    id: thinkingId(event),
    text:
      state.activeItem?.id === thinkingId(event) &&
      state.activeItem.kind === "thinking"
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
  if (event.recordId) {
    const item: RunNarrativeItem = {
      kind: "thinking",
      id,
      text: delta,
      status: "done",
    };
    return {
      ...state,
      committedItems: upsertCommittedBeforeRunAnswer(
        state.committedItems,
        item,
        event,
      ),
      activeItem: state.activeItem?.id === id ? undefined : state.activeItem,
    };
  }
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

function activateAnswer(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  // Like reasoning summaries, a durable assistant_message is a reconciliation
  // snapshot. Its synthetic START frame must not move an already rendered
  // answer back into active state.
  if (event.recordId) {
    return state;
  }
  const id = answerId(state, event);
  const item: RunNarrativeItem = {
    kind: "answer",
    id,
    ...(event.turnId ? { turnId: event.turnId } : {}),
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

  // Older compatible-provider journals may contain a leading <think> block
  // inside assistant_message because the adapter had not normalized it yet.
  // Repair only durable full snapshots; live text remains owned by the Core
  // stream state machine and arbitrary answer text containing tags is left
  // untouched unless the tag is the first non-whitespace block.
  if (event.recordId) {
    const legacy = splitLeadingThinkSnapshot(delta);
    if (legacy) {
      const settledState =
        state.activeItem && state.activeItem.id !== id
          ? commitActive(state, "done")
          : state;
      const withThinking: RunNarrativeState = {
        ...settledState,
        committedItems: upsertCommittedBeforeRunAnswer(
          settledState.committedItems,
          {
            kind: "thinking",
            id: thinkingId(event),
            text: legacy.thinking,
            status: "done",
          },
          event,
        ),
        activeItem:
          settledState.activeItem?.id === id
            ? undefined
            : settledState.activeItem,
      };
      return {
        ...withThinking,
        committedItems: upsertCommitted(withThinking.committedItems, {
          kind: "answer",
          id,
          ...(event.turnId ? { turnId: event.turnId } : {}),
          markdown: legacy.answer,
          status: "done",
        }),
      };
    }

    const settledState =
      state.activeItem && state.activeItem.id !== id
        ? commitActive(state, "done")
        : state;
    return {
      ...settledState,
      committedItems: upsertCommitted(settledState.committedItems, {
        kind: "answer",
        id,
        ...(event.turnId ? { turnId: event.turnId } : {}),
        markdown: delta,
        status: "done",
      }),
      activeItem:
        settledState.activeItem?.id === id
          ? undefined
          : settledState.activeItem,
    };
  }
  const current =
    state.activeItem?.id === id && state.activeItem.kind === "answer"
      ? state.activeItem.markdown
      : "";
  return setActive(state, {
    kind: "answer",
    id,
    ...(event.turnId ? { turnId: event.turnId } : {}),
    markdown: current + delta,
    status: "active",
  });
}

function splitLeadingThinkSnapshot(
  value: string,
): { thinking: string; answer: string } | undefined {
  const start = value.search(/\S/);
  if (start < 0 || !value.startsWith("<think>", start)) return undefined;
  const reasoningStart = start + "<think>".length;
  const end = value.indexOf("</think>", reasoningStart);
  if (end < 0) return undefined;
  return {
    thinking: value.slice(reasoningStart, end),
    answer: value.slice(end + "</think>".length).replace(/^[\r\n]+/, ""),
  };
}

function activateTool(
  state: RunNarrativeState,
  event: AguiEvent,
  detail?: string,
  title?: string,
): RunNarrativeState {
  const id = toolId(event);
  // A successful update_plan call is owned by its plan receipt; its frames
  // must not (re)activate a generic tool row.
  if (state.runPlanReceiptToolIds.includes(id)) {
    return state;
  }
  const existing =
    state.activeItem?.id === id && state.activeItem.kind === "tool"
      ? state.activeItem
      : undefined;
  const nextToolCallId = event.toolCallId ?? existing?.toolCallId;
  const nextTitle =
    title || existing?.title || event.toolCallName || "Tool call";
  const nextDetail = detail ?? existing?.detail;
  if (
    existing &&
    existing.toolCallId === nextToolCallId &&
    existing.title === nextTitle &&
    existing.detail === nextDetail &&
    existing.status === "active" &&
    state.inspectorSelection?.kind === "tool" &&
    state.inspectorSelection.id === id
  ) {
    return state;
  }
  return setActive(
    {
      ...state,
      inspectorSelection: { kind: "tool", id },
    },
    {
      kind: "tool",
      id,
      toolCallId: nextToolCallId,
      title: nextTitle,
      detail: nextDetail,
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
  // The plan receipt already replaced this successful update_plan row; a
  // later RESULT frame (journal order puts plan_updated before tool_result)
  // must not recreate it. Failed calls have no receipt and keep their row.
  if (state.runPlanReceiptToolIds.includes(id)) {
    return state.activeItem?.id === id
      ? { ...state, activeItem: undefined }
      : state;
  }
  const active =
    state.activeItem?.id === id && state.activeItem.kind === "tool"
      ? state.activeItem
      : undefined;
  const item: RunNarrativeItem = {
    kind: "tool",
    id,
    toolCallId: event.toolCallId ?? active?.toolCallId,
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
