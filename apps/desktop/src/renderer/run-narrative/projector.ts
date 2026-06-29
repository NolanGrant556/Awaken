import type { AguiEvent } from "../../shared/agui";
import type {
  ChangedFile,
  RunAction,
  RunNarrativeItem,
  RunNarrativeState,
} from "./types";
import { shellStatusFromContent } from "../runtime/shellToolResult";

const MAX_SEEN_EVENTS = 1000;
const READ_ONLY_TOOL_NAMES = new Set(["read_file", "list_files", "grep_search"]);

export function createInitialRunNarrativeState(): RunNarrativeState {
  return {
    committedItems: [],
    seenEventKeys: [],
  };
}

export function createRunRequestedNarrative(
  prompt: string,
  scopeId = "current",
): RunNarrativeState {
  return appendRunRequestedNarrative(createInitialRunNarrativeState(), prompt, scopeId);
}

export function appendRunRequestedNarrative(
  state: RunNarrativeState,
  prompt: string,
  scopeId = "current",
): RunNarrativeState {
  const trimmed = prompt.trim();
  const settledState = state.activeItem ? commitActive(state, "done") : state;
  const committedItems = trimmed
    ? upsertCommitted(settledState.committedItems, {
        kind: "prompt",
        id: `prompt:${scopeId}`,
        text: trimmed,
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
      return commitMatchingActive(base, answerId(event), "done");
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
    case "TOOL_CALL_END":
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(base, event, "active", "Tool call ready");
      }
      return activateTool(base, event, "Tool call ready");
    case "TOOL_CALL_RESULT":
      if (isReadOnlyToolEvent(event)) {
        return updateExploringAction(
          base,
          event,
          toolResultStatus(event.content),
          summarizeToolResult(event.content),
        );
      }
      return commitTool(
        base,
        event,
        shellStatusFromContent(event.toolCallName, event.content) ??
          (event.content ? "done" : "error"),
        event.content || "Tool returned empty content",
      );
    case "AWAKEN_APPROVAL_REQUESTED":
      return activateApproval(base, event);
    case "AWAKEN_APPROVAL_RESOLVED":
      return resolveApproval(base, event);
    case "AWAKEN_ARTIFACT_CREATED":
    case "AWAKEN_ARTIFACT_UPDATED":
      return activateFileChange(base, event);
    case "RUN_FINISHED":
      return markFinalAnswerForEvent(commitActive(base, "done"), event);
    case "RUN_ERROR":
      if (event.code === "RUN_CANCELLED") {
        return commitActive(base, "done");
      }
      return markLatestAnswerFinal(commitErrorAnswer(base, event));
    case "RUN_STATUS_CHANGED":
      if (event.status === "cancelled") {
        return commitActive(base, "done");
      }
      if (event.status === "finished") {
        return markFinalAnswerForEvent(commitActive(base, "done"), event);
      }
      return base;
    default:
      return base;
  }
}

function eventKey(event: AguiEvent): string | undefined {
  if (typeof event.seq === "number") {
    return `seq:${eventScope(event)}:${event.seq}`;
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
    event.approvalId ||
    event.artifactId ||
    event.runId ||
    event.type;
  const content = event.delta || event.content || event.status || event.approvalDecision || "";
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
  status?: "done" | "error" | "approved" | "rejected" | "waiting",
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

function commitErrorAnswer(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  return addRunErrorToNarrative(
    state,
    narrativeErrorMessage(event),
    `${event.seq ?? event.code ?? "runtime"}`,
  );
}

function markFinalAnswerForEvent(
  state: RunNarrativeState,
  event: AguiEvent,
): RunNarrativeState {
  return markAnswerFinal(state, (item) => answerBelongsToEvent(item, event));
}

function markLatestAnswerFinal(state: RunNarrativeState): RunNarrativeState {
  return markAnswerFinal(state, () => true);
}

function markAnswerFinal(
  state: RunNarrativeState,
  shouldMark: (item: Extract<RunNarrativeItem, { kind: "answer" }>) => boolean,
): RunNarrativeState {
  const items = state.activeItem
    ? [...state.committedItems, state.activeItem]
    : state.committedItems;
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.kind === "answer" && shouldMark(item)) {
      if (item.isFinal) {
        return state;
      }
      const finalItem: RunNarrativeItem = { ...item, isFinal: true };
      if (i < state.committedItems.length) {
        return {
          ...state,
          committedItems: upsertCommitted(state.committedItems, finalItem),
        };
      }
      return { ...state, activeItem: finalItem };
    }
  }
  return state;
}

function answerBelongsToEvent(
  item: Extract<RunNarrativeItem, { kind: "answer" }>,
  event: AguiEvent,
): boolean {
  if (!event.runId && !event.threadId) {
    return true;
  }
  const scope = eventScope(event);
  const scopedAnswerId = `answer:${scope}`;
  return item.id === scopedAnswerId || item.id.startsWith(`${scopedAnswerId}:`);
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
  status?: "done" | "error" | "approved" | "rejected" | "waiting",
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
    case "tool":
      return status === "done" || status === "error" ? { ...item, status } : item;
    case "approval":
      if (status === "approved" || status === "rejected" || status === "waiting") {
        return { ...item, status };
      }
      return item;
    case "answer":
      return status === "done" || status === "error" ? { ...item, status } : item;
  }
}

function thinkingId(event: AguiEvent): string {
  return `thinking:${eventScopedId(event, event.messageId)}`;
}

function answerId(event: AguiEvent): string {
  return `answer:${eventScopedId(event, event.messageId)}`;
}

function narrativeErrorMessage(event: AguiEvent): string {
  const rawMessage = event.message || "Runtime returned an error";
  if (event.code === "PROVIDER_AUTH_MISSING") {
    return "API key 缺失，请在 .env 中配置 LLM_API_KEY";
  }
  if (event.code === "PROVIDER_REQUEST_FAILED") {
    return `Provider 请求失败：${rawMessage}`;
  }
  return rawMessage;
}

function toolId(event: AguiEvent): string {
  return `tool:${eventScopedId(event, event.toolCallId ?? event.toolCallName)}`;
}

function approvalId(event: AguiEvent): string {
  return `approval:${eventScopedId(event, event.approvalId)}`;
}

function artifactId(event: AguiEvent): string {
  return `fileChange:${eventScopedId(event, event.artifactId)}`;
}

function exploringActionId(event: AguiEvent): string {
  return `explore:${eventScopedId(event, event.toolCallId ?? event.toolCallName)}`;
}

function exploringId(event: AguiEvent): string {
  return `exploring:${eventScope(event)}:readonly`;
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

function isReadOnlyToolEvent(event: AguiEvent): boolean {
  return READ_ONLY_TOOL_NAMES.has(event.toolCallName ?? "");
}

function toolResultStatus(content?: string): "done" | "error" {
  const normalized = (content ?? "").trim();
  if (!normalized) {
    return "error";
  }
  if (
    normalized.includes("工具执行失败") ||
    normalized.includes("工具权限拒绝") ||
    normalized.includes("工具参数错误") ||
    normalized.includes("审批拒绝")
  ) {
    return "error";
  }
  return "done";
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
    case "read_file": {
      const path = stringArg(args, "path") ?? "file";
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
    case "list_files": {
      const pattern = stringArg(args, "pattern");
      const root = stringArg(args, "root");
      if (pattern && root) {
        return `List ${pattern} in ${root}`;
      }
      if (pattern) {
        return `List ${pattern}`;
      }
      return "List files";
    }
    case "grep_search": {
      const query = stringArg(args, "query");
      const include = stringArg(args, "include");
      if (query && include) {
        return `Search "${query}" in ${include}`;
      }
      if (query) {
        return `Search "${query}"`;
      }
      return "Search files";
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
  status: "active" | "done" | "error",
  detail?: string,
): RunNarrativeState {
  const id = exploringId(event);
  const existing =
    state.activeItem?.id === id && state.activeItem.kind === "exploring"
      ? state.activeItem
      : undefined;
  const action: RunAction = {
    id: exploringActionId(event),
    title: readOnlyActionTitle(event),
    detail,
    status,
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
  const item: RunNarrativeItem = {
    kind: "answer",
    id: answerId(event),
    markdown:
      state.activeItem?.id === answerId(event) && state.activeItem.kind === "answer"
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
  const id = answerId(event);
  const current =
    state.activeItem?.id === id && state.activeItem.kind === "answer"
      ? state.activeItem.markdown
      : "";
  return setActive(state, {
    kind: "answer",
    id,
    markdown: current + delta,
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
  status: "done" | "error",
  detail: string,
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

function activateApproval(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const id = approvalId(event);
  return setActive(
    {
      ...state,
      inspectorSelection: { kind: "approval", id },
    },
    {
      kind: "approval",
      id,
      title: event.approvalTitle || "Approval required",
      status: "waiting",
    },
  );
}

function resolveApproval(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const id = approvalId(event);
  const status = event.approvalDecision === "rejected" ? "rejected" : "approved";
  const item: RunNarrativeItem = {
    kind: "approval",
    id,
    title: event.approvalTitle || "Approval resolved",
    status,
  };
  if (state.activeItem?.id === id) {
    return {
      ...state,
      committedItems: upsertCommitted(state.committedItems, item),
      activeItem: undefined,
      inspectorSelection: { kind: "approval", id },
    };
  }
  return {
    ...state,
    committedItems: upsertCommitted(state.committedItems, item),
    inspectorSelection: { kind: "approval", id },
  };
}

function activateFileChange(state: RunNarrativeState, event: AguiEvent): RunNarrativeState {
  const id = artifactId(event);
  const file: ChangedFile = {
    id: event.artifactId ?? "artifact",
    path: event.artifactPath,
    title: event.artifactTitle || "Artifact",
    description: event.artifactDescription,
    status: event.artifactStatus === "error" ? "error" : "done",
  };
  return setActive(
    {
      ...state,
      inspectorSelection: { kind: "artifact", id: event.artifactId ?? id },
    },
    {
      kind: "fileChange",
      id,
      files: [file],
      status: "done",
    },
  );
}
