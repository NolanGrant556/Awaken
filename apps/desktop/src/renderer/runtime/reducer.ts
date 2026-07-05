import type { ApprovalItem, RunStepItem } from "@awaken/design-system";
import type {
  AguiApprovalRiskTone,
  AguiArtifactStatus,
  AguiEvent,
} from "./agui";
import type {
  RunInputAttachment,
  RuntimeRun,
  RuntimeRunStatus,
  RuntimeThread,
  SupervisorStatus,
  SupervisorStatusInfo,
} from "../../shared/runtime";
import {
  addRunErrorToNarrative,
  appendRunRequestedNarrative,
  createInitialRunNarrativeState,
  createRunRequestedNarrative,
  projectAguiEventToNarrative,
  type RunNarrativeItem,
  type RunNarrativeState,
} from "../run-narrative";

type RuntimePhase =
  | "idle"
  | "starting"
  | "running"
  | "reconnecting"
  | "cancelling"
  | "done"
  | "error"
  | "cancelled";

type TimelineStatus = "ready" | "running" | "done" | "error";

type StepRecord = RunStepItem & {
  key: string;
};

type ToolResultRecord = {
  id: string;
  title: string;
  content: string;
  args?: string;
  commandOutput?: string;
  commandOutputTruncated?: boolean;
  status?: "active" | "done" | "error";
};

export type PatchPreviewRecord = {
  id: string;
  title: string;
  patch: string;
  summary?: string;
  affectedPaths: string[];
  truncated?: boolean;
};

export type ArtifactRecord = {
  id: string;
  title: string;
  description?: string;
  kind: string;
  path?: string;
  content?: string;
  status: AguiArtifactStatus;
};

export type TurnDiffRecord = {
  diff: string;
  truncated?: boolean;
};

export type ContextWindowRecord = {
  eventType: AguiEvent["type"];
  message?: string;
  modelWindow?: number;
  outputReserve?: number;
  effectiveWindow?: number;
  estimatedTokens?: number;
  actualTokens?: number;
  warningThreshold?: number;
  autoCompactThreshold?: number;
  compactTarget?: number;
  estimatedDeltaTokens?: number;
};

type CompactFallbackState = "structured" | "legacy";

export type CompactRecord = ContextWindowRecord & {
  status: "started" | "finished" | "failed";
  mode?: string;
  boundaryId?: string;
  preCompactEstimatedTokens?: number;
  postCompactEstimatedTokens?: number;
  fallbackState?: CompactFallbackState;
  toolResultsPrunedCount?: number;
  prunedSafeGroupCount?: number;
  prunedMessageCount?: number;
  summaryMaxTokens?: number;
  summaryRetryCount?: number;
  summaryRawChars?: number;
  summaryParsedSections?: number;
  summaryMeaningfulSections?: number;
  summaryQualityIssue?: string;
  summarySourceHadUser?: boolean;
  summaryLatestUserPinned?: boolean;
  fallbackMarker?: string;
};

export type ToolDecisionRecord = {
  toolCallId: string;
  toolName: string;
  action?: string;
  source?: string;
  reason?: string;
  staticPermissionBehavior?: string;
  staticPermissionReason?: string;
  affectedPaths: string[];
  patchSummary?: string;
};

export type TurnRecord = {
  turnId?: string;
  turnCount?: number;
  status: "active" | "done";
  transitionReason?: string;
  terminalReason?: string;
};

export type UnknownEventRecord = {
  rawType?: string;
  message?: string;
};

export type InspectorSummary = {
  currentTurn?: TurnRecord;
  lastToolDecision?: ToolDecisionRecord;
  contextWindow?: ContextWindowRecord;
  compact?: CompactRecord;
  terminalReason?: string;
  terminalMessage?: string;
  unknownEvents: UnknownEventRecord[];
};

function createInspectorSummary(): InspectorSummary {
  return { unknownEvents: [] };
}

export type ThreadItem = {
  threadId: string;
  label: string;
  runId?: string;
  activeRunId?: string;
  runStatus?: RuntimeRunStatus;
  approvalCount?: number;
  artifactCount?: number;
};

type ThreadViewState = {
  prompt: string;
  promptAttachments?: RunInputAttachment[];
  runId?: string;
  threadId?: string;
  phase: RuntimePhase;
  status?: RuntimeRunStatus;
  timelineTitle: string;
  timelineStatus: TimelineStatus;
  steps: RunStepItem[];
  assistantText: string;
  reasoningText: string;
  toolResults: ToolResultRecord[];
  patchPreviews: PatchPreviewRecord[];
  approvals: ApprovalItem[];
  artifacts: ArtifactRecord[];
  turnDiff?: TurnDiffRecord;
  inspectorSummary: InspectorSummary;
  narrative: RunNarrativeState;
  errorMessage?: string;
  runStartedAtMs?: number;
  runCompletedAtMs?: number;
};

export type RuntimeViewState = {
  prompt: string;
  promptAttachments?: RunInputAttachment[];
  runId?: string;
  threadId?: string;
  phase: RuntimePhase;
  status?: RuntimeRunStatus;
  timelineTitle: string;
  timelineStatus: TimelineStatus;
  steps: RunStepItem[];
  assistantText: string;
  reasoningText: string;
  toolResults: ToolResultRecord[];
  patchPreviews: PatchPreviewRecord[];
  approvals: ApprovalItem[];
  artifacts: ArtifactRecord[];
  turnDiff?: TurnDiffRecord;
  inspectorSummary: InspectorSummary;
  narrative: RunNarrativeState;
  errorMessage?: string;
  runStartedAtMs?: number;
  runCompletedAtMs?: number;
  threads: ThreadItem[];
  activeThreadId?: string;
  threadViews: Record<string, ThreadViewState>;
  supervisorStatus: SupervisorStatus | null;
};

export type RuntimeAction =
  | { type: "run/reset" }
  | {
      type: "run/requested";
      prompt: string;
      promptAttachments?: RunInputAttachment[];
      startedAtMs?: number;
    }
  | { type: "run/created"; run: RuntimeRun }
  | { type: "run/createFailed"; message: string }
  | { type: "run/cancelRequested" }
  | { type: "run/cancelSync"; run: RuntimeRun }
  | { type: "run/streamError"; message: string }
  | { type: "run/streamFailed"; message: string }
  | { type: "run/reconnecting" }
  | { type: "run/recovered"; run: RuntimeRun; prompt?: string }
  | { type: "run/threadReplayStarted"; threadId: string; runId?: string }
  | { type: "run/threadsLoaded"; threads: RuntimeThread[] }
  | { type: "run/threadAdded"; threadId: string; label: string; runId: string }
  | { type: "run/threadSwitch"; threadId: string }
  | { type: "run/event"; event: AguiEvent; receivedAtMs?: number }
  | { type: "supervisor/status-changed"; info: SupervisorStatusInfo };

const initialState: RuntimeViewState = {
  prompt: "",
  promptAttachments: undefined,
  phase: "idle",
  timelineTitle: "Run",
  timelineStatus: "ready",
  steps: [],
  assistantText: "",
  reasoningText: "",
  toolResults: [],
  patchPreviews: [],
  approvals: [],
  artifacts: [],
  narrative: createInitialRunNarrativeState(),
  inspectorSummary: createInspectorSummary(),
  threads: [],
  threadViews: {},
  supervisorStatus: null,
};

function viewFromState(state: RuntimeViewState): ThreadViewState {
  return {
    prompt: state.prompt,
    promptAttachments: state.promptAttachments,
    runId: state.runId,
    threadId: state.threadId,
    phase: state.phase,
    status: state.status,
    timelineTitle: state.timelineTitle,
    timelineStatus: state.timelineStatus,
    steps: state.steps,
    assistantText: state.assistantText,
    reasoningText: state.reasoningText,
    toolResults: state.toolResults,
    patchPreviews: state.patchPreviews,
    approvals: state.approvals,
    artifacts: state.artifacts,
    turnDiff: state.turnDiff,
    inspectorSummary: state.inspectorSummary,
    narrative: state.narrative,
    errorMessage: state.errorMessage,
    runStartedAtMs: state.runStartedAtMs,
    runCompletedAtMs: state.runCompletedAtMs,
  };
}

function emptyThreadView(threadId: string, runId?: string): ThreadViewState {
  return {
    ...viewFromState(initialState),
    narrative: createInitialRunNarrativeState(),
    threadId,
    runId,
  };
}

function saveActiveThreadView(state: RuntimeViewState): RuntimeViewState {
  if (!state.activeThreadId) {
    return state;
  }
  return {
    ...state,
    threadViews: {
      ...state.threadViews,
      [state.activeThreadId]: viewFromState(state),
    },
  };
}

function restoreThreadView(
  state: RuntimeViewState,
  threadId: string,
): RuntimeViewState {
  const thread = state.threads.find((item) => item.threadId === threadId);
  const view =
    state.threadViews[threadId] ?? emptyThreadView(threadId, thread?.runId);
  return syncActiveThreadSummary({
    ...state,
    ...view,
    activeThreadId: threadId,
  });
}

function upsertThread(threads: ThreadItem[], next: ThreadItem): ThreadItem[] {
  const index = threads.findIndex(
    (thread) => thread.threadId === next.threadId,
  );
  if (index < 0) {
    return [...threads, next];
  }
  const updated = threads.slice();
  updated[index] = {
    ...updated[index],
    ...next,
    label: next.label || updated[index].label,
  };
  return updated;
}

function threadItemFromRuntime(thread: RuntimeThread): ThreadItem {
  return {
    threadId: thread.threadId,
    label: thread.label || "Untitled thread",
    runId: thread.runId,
    activeRunId: thread.activeRunId,
    runStatus: thread.activeRunId ? "running" : undefined,
    approvalCount: 0,
    artifactCount: 0,
  };
}

function isActiveRunStatus(status?: RuntimeRunStatus): boolean {
  return (
    status === "running" ||
    status === "awaiting_approval" ||
    status === "cancelling"
  );
}

function syncActiveThreadSummary(state: RuntimeViewState): RuntimeViewState {
  const threadId = state.threadId ?? state.activeThreadId;
  if (!threadId) {
    return state;
  }

  const existing = state.threads.find((thread) => thread.threadId === threadId);
  return {
    ...state,
    threads: upsertThread(state.threads, {
      threadId,
      label: existing?.label || state.prompt || "Untitled thread",
      runId: state.runId ?? existing?.runId,
      activeRunId: isActiveRunStatus(state.status) ? state.runId : undefined,
      runStatus: state.status ?? existing?.runStatus,
      approvalCount: state.approvals.length,
      artifactCount: state.artifacts.length,
    }),
  };
}

function recoveryPrompt(
  state: RuntimeViewState,
  run: RuntimeRun,
  prompt?: string,
): string {
  const explicit = prompt?.trim();
  if (explicit) {
    return explicit;
  }
  return (
    state.threads.find((thread) => thread.threadId === run.threadId)?.label ??
    ""
  );
}

function nextNarrativeScope(state: RuntimeViewState): string {
  const owner = state.activeThreadId ?? state.threadId ?? "draft";
  const committedPromptCount = state.narrative.committedItems.filter(
    (item) => item.kind === "prompt",
  ).length;
  const activePromptCount =
    state.narrative.activeItem?.kind === "prompt" ? 1 : 0;
  return `${owner}:${committedPromptCount + activePromptCount + 1}`;
}

function currentNarrativeScope(state: RuntimeViewState): string {
  return state.runId ?? state.threadId ?? state.activeThreadId ?? "current";
}

function recoveredNarrativeScope(run: RuntimeRun): string {
  return `${run.threadId ?? run.runId ?? "recovered"}:1`;
}

function shouldAppendNarrative(state: RuntimeViewState): boolean {
  return Boolean(state.activeThreadId || state.threadId);
}

function scopeNarrativeEvent(
  event: AguiEvent,
  state: RuntimeViewState,
): AguiEvent {
  return {
    ...event,
    threadId: event.threadId ?? state.threadId ?? state.activeThreadId,
    runId: event.runId ?? state.runId,
  };
}

function validTimestamp(value?: number): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isSuccessfulTerminalEvent(event: AguiEvent): boolean {
  return (
    event.type === "RUN_FINISHED" ||
    (event.type === "RUN_STATUS_CHANGED" && event.status === "finished")
  );
}

function elapsedMsForTerminalRun(
  state: RuntimeViewState,
  receivedAtMs?: number,
): number | undefined {
  if (!validTimestamp(state.runStartedAtMs)) {
    return undefined;
  }

  const completedAtMs = validTimestamp(state.runCompletedAtMs)
    ? state.runCompletedAtMs
    : receivedAtMs;
  if (!validTimestamp(completedAtMs)) {
    return undefined;
  }

  return Math.max(0, completedAtMs - state.runStartedAtMs);
}

function isAnswerForRun(itemId: string, runId?: string): boolean {
  if (!runId) {
    return true;
  }
  return itemId === `answer:${runId}` || itemId.startsWith(`answer:${runId}:`);
}

function attachElapsedMsToLatestAnswer(
  narrative: RunNarrativeState,
  elapsedMs: number | undefined,
  runId?: string,
): RunNarrativeState {
  if (!validTimestamp(elapsedMs)) {
    return narrative;
  }

  let answerIndex = -1;
  for (
    let index = narrative.committedItems.length - 1;
    index >= 0;
    index -= 1
  ) {
    const item = narrative.committedItems[index];
    if (
      item.kind === "answer" &&
      item.status === "done" &&
      item.elapsedMs === undefined &&
      isAnswerForRun(item.id, runId)
    ) {
      answerIndex = index;
      break;
    }
  }

  if (answerIndex < 0) {
    return narrative;
  }

  const committedItems = narrative.committedItems.slice();
  const answer = committedItems[answerIndex];
  if (answer.kind !== "answer") {
    return narrative;
  }

  committedItems[answerIndex] = {
    ...answer,
    elapsedMs,
  };

  return {
    ...narrative,
    committedItems,
  };
}

function appendFinalFileChangeSummary(
  narrative: RunNarrativeState,
  artifacts: ArtifactRecord[],
  runId?: string,
): RunNarrativeState {
  if (artifacts.length === 0) {
    return narrative;
  }

  const id = `fileChange:${runId ?? "current"}:summary`;
  const item: RunNarrativeItem = {
    kind: "fileChange",
    id,
    files: artifacts.map((artifact) => ({
      id: artifact.id,
      path: artifact.path,
      title: artifact.title || artifact.path || "File change",
      description: artifact.description,
      status: artifact.status === "error" ? "error" : "done",
    })),
    status: "done",
  };

  return {
    ...narrative,
    committedItems: [
      ...narrative.committedItems.filter(
        (candidate) =>
          !(candidate.kind === "fileChange" && candidate.id === id),
      ),
      item,
    ],
    activeItem:
      narrative.activeItem?.id === id ? undefined : narrative.activeItem,
  };
}

function toSteps(records: StepRecord[]): RunStepItem[] {
  return records.map(({ key: _key, ...step }) => step);
}

function upsertStep(
  records: StepRecord[],
  key: string,
  next: Omit<RunStepItem, "id" | "index"> & { id?: string },
): StepRecord[] {
  const existingIndex = records.findIndex((record) => record.key === key);
  const existing = existingIndex >= 0 ? records[existingIndex] : undefined;
  const record: StepRecord = {
    id: next.id ?? existing?.id ?? key,
    index: existing?.index ?? records.length + 1,
    key,
    title: next.title,
    description: next.description,
    tag: next.tag,
    status: next.status,
  };

  if (existingIndex >= 0) {
    const updated = records.slice();
    updated[existingIndex] = record;
    return updated;
  }

  return [...records, record];
}

function excerpt(value: string, max = 96): string {
  const trimmed = value.trim();
  if (!trimmed) {
    return "";
  }
  if (trimmed.length <= max) {
    return trimmed;
  }
  return `${trimmed.slice(0, max - 3)}...`;
}

function upsertToolResult(
  records: ToolResultRecord[],
  next: ToolResultRecord,
): ToolResultRecord[] {
  const index = records.findIndex((record) => record.id === next.id);
  if (index < 0) {
    return [...records, next];
  }
  const updated = records.slice();
  updated[index] = {
    ...updated[index],
    ...next,
    args: next.args ?? updated[index].args,
    content: next.content || updated[index].content,
    commandOutput: next.commandOutput ?? updated[index].commandOutput,
    commandOutputTruncated:
      next.commandOutputTruncated ?? updated[index].commandOutputTruncated,
  };
  return updated;
}

function upsertPatchPreview(
  records: PatchPreviewRecord[],
  next: PatchPreviewRecord,
): PatchPreviewRecord[] {
  const index = records.findIndex((record) => record.id === next.id);
  if (index < 0) {
    return [...records, next];
  }
  const updated = records.slice();
  updated[index] = {
    ...updated[index],
    ...next,
    patch: next.patch || updated[index].patch,
    affectedPaths:
      next.affectedPaths.length > 0
        ? next.affectedPaths
        : updated[index].affectedPaths,
  };
  return updated;
}

function toolRecordStatus(content?: string): "done" | "error" {
  const normalized = (content ?? "").trim();
  if (!normalized) {
    return "error";
  }
  if (
    normalized.includes("工具执行失败") ||
    normalized.includes("工具权限拒绝") ||
    normalized.includes("AUTO policy 拒绝") ||
    normalized.includes("工具执行已被取消") ||
    normalized.includes("工具参数错误") ||
    normalized.includes("审批拒绝")
  ) {
    return "error";
  }
  return "done";
}

function normalizeRiskTone(value?: string): AguiApprovalRiskTone {
  switch (value) {
    case "safe":
    case "danger":
      return value;
    default:
      return "warning";
  }
}

function normalizeArtifactStatus(value?: string): AguiArtifactStatus {
  switch (value) {
    case "pending":
    case "error":
      return value;
    default:
      return "ready";
  }
}

function contextWindowRecord(event: AguiEvent): ContextWindowRecord {
  return {
    eventType: event.type,
    message: event.message,
    modelWindow: event.contextModelWindow,
    outputReserve: event.contextOutputReserve,
    effectiveWindow: event.contextEffectiveWindow,
    estimatedTokens: event.contextEstimatedTokens,
    actualTokens: event.contextActualTokens,
    warningThreshold: event.contextWarningThreshold,
    autoCompactThreshold: event.contextAutoCompactThreshold,
    compactTarget: event.contextCompactTarget,
    estimatedDeltaTokens: event.contextEstimatedDeltaTokens,
  };
}

function hasContextWindowPayload(event: AguiEvent): boolean {
  return [
    event.contextModelWindow,
    event.contextOutputReserve,
    event.contextEffectiveWindow,
    event.contextEstimatedTokens,
    event.contextActualTokens,
    event.contextWarningThreshold,
    event.contextAutoCompactThreshold,
    event.contextCompactTarget,
    event.contextEstimatedDeltaTokens,
  ].some((value) => typeof value === "number" && Number.isFinite(value));
}

function mergeContextWindowRecord(
  previous: ContextWindowRecord | undefined,
  event: AguiEvent,
): ContextWindowRecord {
  const next = contextWindowRecord(event);
  if (!previous) {
    return next;
  }
  return {
    eventType: previous.eventType,
    message: next.message ?? previous.message,
    modelWindow: next.modelWindow ?? previous.modelWindow,
    outputReserve: next.outputReserve ?? previous.outputReserve,
    effectiveWindow: next.effectiveWindow ?? previous.effectiveWindow,
    estimatedTokens: next.estimatedTokens ?? previous.estimatedTokens,
    actualTokens: next.actualTokens ?? previous.actualTokens,
    warningThreshold: next.warningThreshold ?? previous.warningThreshold,
    autoCompactThreshold:
      next.autoCompactThreshold ?? previous.autoCompactThreshold,
    compactTarget: next.compactTarget ?? previous.compactTarget,
    estimatedDeltaTokens:
      next.estimatedDeltaTokens ?? previous.estimatedDeltaTokens,
  };
}

function isFiniteNumber(value: number | undefined): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function compactBoundaryFromMessage(message?: string): string | undefined {
  const match = message?.match(/\bboundary=([^\s]+)/);
  return match?.[1];
}

function compactModeFromMessage(message?: string): string | undefined {
  const match = message?.match(
    /\bcompact (?:started|finished|failed):\s*([a-z_]+)(?=\s|$)/i,
  );
  return match?.[1]?.toLowerCase();
}

function compactEstimatedTokensFromMessage(
  message?: string,
): number | undefined {
  const match = message?.match(/\bestimated_tokens=(\d+)\b/i);
  if (!match) {
    return undefined;
  }
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function compactStatus(event: AguiEvent): CompactRecord["status"] {
  if (event.type === "AWAKEN_COMPACT_FAILED") {
    return "failed";
  }
  if (event.type === "AWAKEN_COMPACT_FINISHED") {
    return "finished";
  }
  return "started";
}

function compactRecord(
  event: AguiEvent,
  previousContextWindow?: ContextWindowRecord,
): CompactRecord {
  const structuredMode = event.contextCompactMode?.trim();
  const fallbackMode = compactModeFromMessage(event.message);
  const mode = structuredMode || fallbackMode;

  const structuredBoundaryId = event.contextCompactBoundaryId?.trim();
  const fallbackBoundaryId = compactBoundaryFromMessage(event.message);
  const boundaryId = structuredBoundaryId || fallbackBoundaryId;

  const structuredPreEstimate = event.contextCompactPreEstimateTokens;
  const fallbackPreEstimate =
    event.type === "AWAKEN_COMPACT_STARTED"
      ? event.contextEstimatedTokens
      : previousContextWindow?.estimatedTokens;
  const preCompactEstimatedTokens = isFiniteNumber(structuredPreEstimate)
    ? structuredPreEstimate
    : fallbackPreEstimate;

  const structuredPostEstimate = event.contextCompactPostEstimateTokens;
  const fallbackPostEstimate =
    event.type === "AWAKEN_COMPACT_FINISHED"
      ? (event.contextEstimatedTokens ??
        compactEstimatedTokensFromMessage(event.message))
      : undefined;
  const postCompactEstimatedTokens = isFiniteNumber(structuredPostEstimate)
    ? structuredPostEstimate
    : fallbackPostEstimate;

  const usedFallback =
    (!structuredMode && Boolean(fallbackMode)) ||
    (!structuredBoundaryId && Boolean(fallbackBoundaryId)) ||
    (!isFiniteNumber(structuredPreEstimate) &&
      isFiniteNumber(preCompactEstimatedTokens)) ||
    (!isFiniteNumber(structuredPostEstimate) &&
      isFiniteNumber(postCompactEstimatedTokens));

  return {
    ...contextWindowRecord(event),
    status: compactStatus(event),
    mode,
    boundaryId,
    preCompactEstimatedTokens,
    postCompactEstimatedTokens,
    fallbackState: usedFallback ? "legacy" : "structured",
    toolResultsPrunedCount: event.compactToolResultsPrunedCount,
    prunedSafeGroupCount: event.compactPrunedSafeGroupCount,
    prunedMessageCount: event.compactPrunedMessageCount,
    summaryMaxTokens: event.compactSummaryMaxTokens,
    summaryRetryCount: event.compactSummaryRetryCount,
    summaryRawChars: event.compactSummaryRawChars,
    summaryParsedSections: event.compactSummaryParsedSections,
    summaryMeaningfulSections: event.compactSummaryMeaningfulSections,
    summaryQualityIssue: event.compactSummaryQualityIssue,
    summarySourceHadUser: event.compactSummarySourceHadUser,
    summaryLatestUserPinned: event.compactSummaryLatestUserPinned,
    fallbackMarker: event.compactFallbackMarker,
  };
}

function toolDecisionRecord(event: AguiEvent): ToolDecisionRecord {
  return {
    toolCallId: event.toolCallId || "tool-decision",
    toolName: event.toolCallName || "Tool",
    action: event.toolDecisionAction,
    source: event.toolDecisionSource,
    reason: event.toolDecisionReason || event.message,
    staticPermissionBehavior: event.staticPermissionBehavior,
    staticPermissionReason: event.staticPermissionReason,
    affectedPaths: event.approvalAffectedPaths ?? [],
    patchSummary: event.approvalPatchSummary,
  };
}

function terminalReasonForEvent(event: AguiEvent): string | undefined {
  if (event.terminalReason) {
    return event.terminalReason;
  }
  if (event.type === "RUN_FINISHED") {
    return "completed";
  }
  if (event.type === "RUN_STATUS_CHANGED" && event.status === "cancelled") {
    return "cancelled";
  }
  return undefined;
}

function toolDecisionStatus(
  decision: ToolDecisionRecord,
): "active" | "done" | "error" {
  if (decision.action === "deny" || decision.action === "abort") {
    return "error";
  }
  if (decision.action === "allow") {
    return "done";
  }
  return "active";
}

function appendUnknownEvent(
  records: UnknownEventRecord[],
  event: AguiEvent,
): UnknownEventRecord[] {
  return [
    ...records,
    {
      rawType: event.rawType,
      message: event.message,
    },
  ].slice(-10);
}

function buildArtifactDescription(artifact: ArtifactRecord): string {
  const details: string[] = [];
  if (artifact.description) {
    details.push(artifact.description);
  }

  const meta = [artifact.kind, artifact.status].filter(Boolean).join(" / ");
  if (meta) {
    details.push(meta);
  }
  if (artifact.path) {
    details.push(artifact.path);
  }

  return details.join(" | ");
}

function upsertApproval(
  approvals: ApprovalItem[],
  next: ApprovalItem,
): ApprovalItem[] {
  const index = approvals.findIndex((approval) => approval.id === next.id);
  if (index < 0) {
    return [...approvals, next];
  }

  const updated = approvals.slice();
  updated[index] = next;
  return updated;
}

function removeApproval(
  approvals: ApprovalItem[],
  approvalId?: string,
): ApprovalItem[] {
  if (!approvalId) {
    return approvals;
  }
  return approvals.filter((approval) => approval.id !== approvalId);
}

function upsertArtifact(
  artifacts: ArtifactRecord[],
  event: AguiEvent,
): ArtifactRecord[] {
  const id = event.artifactId ?? `artifact-${artifacts.length + 1}`;
  const existingIndex = artifacts.findIndex((artifact) => artifact.id === id);
  const existing = existingIndex >= 0 ? artifacts[existingIndex] : undefined;
  const next: ArtifactRecord = {
    id,
    title: event.artifactTitle || existing?.title || "Artifact",
    description: event.artifactDescription || existing?.description,
    kind: event.artifactKind || existing?.kind || "file",
    path: event.artifactPath || existing?.path,
    content: event.artifactContent ?? existing?.content,
    status: normalizeArtifactStatus(event.artifactStatus || existing?.status),
  };

  if (existingIndex >= 0) {
    const updated = artifacts.slice();
    updated[existingIndex] = next;
    return updated;
  }

  return [...artifacts, next];
}

function syncFromRun(
  state: RuntimeViewState,
  run: RuntimeRun,
): RuntimeViewState {
  if (run.status === "cancelling") {
    return {
      ...state,
      runId: run.runId,
      threadId: run.threadId,
      promptAttachments: run.inputAttachments,
      phase: "cancelling",
      status: run.status,
      timelineStatus: "running",
      approvals: [],
      steps: toSteps(
        upsertStep(
          state.steps.map((step, index) => ({
            ...step,
            key: step.id,
            index: index + 1,
          })),
          "run-cancel",
          {
            title: "Cancelling run",
            description: "Waiting for runtime shutdown",
            status: "active",
            tag: "cancel",
          },
        ),
      ),
    };
  }

  if (run.status === "finished") {
    return {
      ...state,
      runId: run.runId,
      threadId: run.threadId,
      promptAttachments: run.inputAttachments,
      phase: "done",
      status: run.status,
      timelineStatus: "done",
      approvals: [],
    };
  }

  if (run.status === "cancelled") {
    return {
      ...state,
      runId: run.runId,
      threadId: run.threadId,
      promptAttachments: run.inputAttachments,
      phase: "cancelled",
      status: run.status,
      timelineStatus: "done",
      approvals: [],
    };
  }

  if (run.status === "error") {
    return {
      ...state,
      runId: run.runId,
      threadId: run.threadId,
      promptAttachments: run.inputAttachments,
      phase: "error",
      status: run.status,
      timelineStatus: "error",
      errorMessage: run.error || "Run failed",
      approvals: [],
    };
  }

  return {
    ...state,
    runId: run.runId,
    threadId: run.threadId,
    promptAttachments: run.inputAttachments,
    phase: "running",
    status: run.status,
    timelineStatus: "running",
  };
}

export function createInitialRuntimeState(): RuntimeViewState {
  return initialState;
}

export function reduceRuntimeState(
  state: RuntimeViewState,
  action: RuntimeAction,
): RuntimeViewState {
  switch (action.type) {
    case "run/reset":
      return {
        ...initialState,
        narrative: createInitialRunNarrativeState(),
        supervisorStatus: state.supervisorStatus,
        threads: state.threads,
        threadViews: saveActiveThreadView(state).threadViews,
      };
    case "run/requested":
      return {
        prompt: action.prompt,
        promptAttachments: action.promptAttachments,
        phase: "starting",
        timelineTitle: excerpt(action.prompt, 72) || "New run",
        timelineStatus: "running",
        steps: [],
        assistantText: "",
        reasoningText: "",
        toolResults: [],
        patchPreviews: [],
        approvals: [],
        artifacts: [],
        turnDiff: undefined,
        inspectorSummary: createInspectorSummary(),
        narrative: shouldAppendNarrative(state)
          ? appendRunRequestedNarrative(
              state.narrative,
              action.prompt,
              action.promptAttachments,
              nextNarrativeScope(state),
            )
          : createRunRequestedNarrative(
              action.prompt,
              action.promptAttachments,
              nextNarrativeScope(state),
            ),
        supervisorStatus: state.supervisorStatus,
        threads: state.threads,
        activeThreadId: state.activeThreadId,
        threadViews: state.threadViews,
        runStartedAtMs: action.startedAtMs,
        runCompletedAtMs: undefined,
      };
    case "run/created":
      return saveActiveThreadView(
        syncActiveThreadSummary({
          ...syncFromRun(state, action.run),
          activeThreadId: action.run.threadId,
        }),
      );
    case "run/createFailed":
      return saveActiveThreadView({
        ...state,
        phase: "error",
        timelineStatus: "error",
        errorMessage: action.message,
        steps: toSteps(
          upsertStep([], "run-create-error", {
            title: "Create run failed",
            description: action.message,
            status: "error",
            tag: "http",
          }),
        ),
      });
    case "run/cancelRequested":
      return saveActiveThreadView(
        syncActiveThreadSummary({
          ...state,
          phase: "cancelling",
          status: "cancelling",
          timelineStatus: "running",
          approvals: [],
        }),
      );
    case "run/cancelSync":
      return saveActiveThreadView(
        syncActiveThreadSummary(syncFromRun(state, action.run)),
      );
    case "run/streamError":
      return saveActiveThreadView(
        syncActiveThreadSummary({
          ...state,
          phase: "reconnecting",
          timelineStatus: "running",
          errorMessage: undefined,
          steps: toSteps(
            upsertStep(
              state.steps.map((step, index) => ({
                ...step,
                key: step.id,
                index: index + 1,
              })),
              "stream-interrupted",
              {
                title: "Stream interrupted",
                description: action.message,
                status: "active",
                tag: "reconnect",
              },
            ),
          ),
        }),
      );
    case "run/streamFailed":
      return saveActiveThreadView(
        syncActiveThreadSummary({
          ...state,
          phase: "error",
          status: "error",
          timelineStatus: "error",
          errorMessage: action.message,
          approvals: [],
          inspectorSummary: {
            ...state.inspectorSummary,
            terminalReason: "stream_failed",
            terminalMessage: action.message,
          },
          steps: toSteps(
            upsertStep(
              state.steps.map((step, index) => ({
                ...step,
                key: step.id,
                index: index + 1,
              })),
              "stream-failed",
              {
                title: "Stream failed",
                description: action.message,
                status: "error",
                tag: "reconnect",
              },
            ),
          ),
          narrative: addRunErrorToNarrative(
            state.narrative,
            action.message,
            `${currentNarrativeScope(state)}:stream`,
          ),
        }),
      );
    case "run/reconnecting":
      return saveActiveThreadView(
        syncActiveThreadSummary({
          ...state,
          phase: "reconnecting",
          timelineStatus: "running",
        }),
      );
    case "run/recovered": {
      const prompt = recoveryPrompt(state, action.run, action.prompt);
      const hasPromptAttachments =
        (action.run.inputAttachments?.length ?? 0) > 0;
      return saveActiveThreadView(
        syncActiveThreadSummary({
          ...syncFromRun(initialState, action.run),
          prompt,
          narrative: prompt
            ? createRunRequestedNarrative(
                prompt,
                action.run.inputAttachments,
                recoveredNarrativeScope(action.run),
              )
            : hasPromptAttachments
              ? createRunRequestedNarrative(
                  prompt,
                  action.run.inputAttachments,
                  recoveredNarrativeScope(action.run),
                )
              : createInitialRunNarrativeState(),
          supervisorStatus: state.supervisorStatus,
          timelineTitle: "恢复运行",
          threads: state.threads,
          threadViews: state.threadViews,
          activeThreadId: action.run.threadId,
        }),
      );
    }
    case "run/threadReplayStarted": {
      const view = emptyThreadView(action.threadId, action.runId);
      return {
        ...initialState,
        ...view,
        activeThreadId: action.threadId,
        supervisorStatus: state.supervisorStatus,
        threads: state.threads,
        threadViews: {
          ...state.threadViews,
          [action.threadId]: view,
        },
      };
    }
    case "run/threadsLoaded": {
      const threadViews = { ...state.threadViews };
      const loaded = action.threads.map((thread) => {
        const existing = state.threads.find(
          (item) => item.threadId === thread.threadId,
        );
        return {
          ...threadItemFromRuntime(thread),
          runStatus:
            existing?.runStatus ?? (thread.activeRunId ? "running" : undefined),
          approvalCount: existing?.approvalCount ?? 0,
          artifactCount: existing?.artifactCount ?? 0,
        };
      });
      for (const thread of loaded) {
        if (!threadViews[thread.threadId]) {
          threadViews[thread.threadId] = emptyThreadView(
            thread.threadId,
            thread.runId,
          );
        }
      }
      const activeThreadStillExists =
        !state.activeThreadId ||
        loaded.some((thread) => thread.threadId === state.activeThreadId);
      if (!activeThreadStillExists) {
        return {
          ...initialState,
          narrative: createInitialRunNarrativeState(),
          supervisorStatus: state.supervisorStatus,
          threads: loaded,
          threadViews,
        };
      }
      return {
        ...state,
        threads: loaded,
        threadViews,
      };
    }
    case "run/threadAdded": {
      const next = {
        ...state,
        activeThreadId: action.threadId,
        threads: upsertThread(state.threads, {
          threadId: action.threadId,
          label: action.label,
          runId: action.runId,
          activeRunId: action.runId,
          runStatus: "running",
          approvalCount: 0,
          artifactCount: 0,
        }),
      };
      return saveActiveThreadView(next);
    }
    case "run/threadSwitch":
      return restoreThreadView(saveActiveThreadView(state), action.threadId);
    case "run/event": {
      const nextState = { ...state };
      const narrativeEvent = scopeNarrativeEvent(action.event, nextState);
      let records = state.steps.map((step, index) => ({
        ...step,
        key: step.id,
        index: index + 1,
      }));

      switch (action.event.type) {
        case "RUN_STARTED":
          nextState.phase = "running";
          nextState.timelineStatus = "running";
          nextState.runId = action.event.runId ?? nextState.runId;
          nextState.threadId = action.event.threadId ?? nextState.threadId;
          nextState.activeThreadId =
            nextState.activeThreadId ?? action.event.threadId;
          if (action.event.threadId) {
            nextState.threads = upsertThread(state.threads, {
              threadId: action.event.threadId,
              label: state.prompt || "Untitled thread",
              runId: action.event.runId,
              activeRunId: action.event.runId,
            });
          }
          records = upsertStep(records, "run-started", {
            title: "Runtime accepted run",
            description: action.event.runId
              ? `run ${action.event.runId}`
              : "Awaiting streamed events",
            status: "done",
            tag: "runtime",
          });
          break;
        case "AWAKEN_TURN_STARTED": {
          const turnKey =
            action.event.turnId ?? String(action.event.turnCount ?? "current");
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            currentTurn: {
              turnId: action.event.turnId,
              turnCount: action.event.turnCount,
              status: "active",
              transitionReason: action.event.transitionReason,
            },
          };
          records = upsertStep(records, `turn:${turnKey}`, {
            title: "Agent turn started",
            description:
              action.event.transitionReason ||
              `Turn ${action.event.turnCount ?? turnKey}`,
            status: "active",
            tag: "turn",
          });
          break;
        }
        case "AWAKEN_TURN_FINISHED": {
          const turnKey =
            action.event.turnId ?? String(action.event.turnCount ?? "current");
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            currentTurn: {
              turnId:
                action.event.turnId ??
                nextState.inspectorSummary.currentTurn?.turnId,
              turnCount:
                action.event.turnCount ??
                nextState.inspectorSummary.currentTurn?.turnCount,
              status: "done",
              transitionReason:
                action.event.transitionReason ??
                nextState.inspectorSummary.currentTurn?.transitionReason,
              terminalReason: action.event.terminalReason,
            },
          };
          records = upsertStep(records, `turn:${turnKey}`, {
            title: "Agent turn finished",
            description:
              action.event.terminalReason ||
              action.event.transitionReason ||
              "Turn complete",
            status: "done",
            tag: action.event.terminalReason || "turn",
          });
          break;
        }
        case "RUN_STATUS_CHANGED": {
          const status = action.event.status;
          nextState.status = status as RuntimeRunStatus | undefined;
          if (status === "running") {
            nextState.phase = "running";
            nextState.timelineStatus = "running";
          } else if (status === "awaiting_approval") {
            nextState.phase = "running";
          } else if (status === "cancelling") {
            nextState.phase = "cancelling";
            nextState.timelineStatus = "running";
            nextState.approvals = [];
          } else if (status === "cancelled") {
            nextState.phase = "cancelled";
            nextState.timelineStatus = "done";
            nextState.approvals = [];
          } else if (status === "finished") {
            nextState.phase = "done";
            nextState.timelineStatus = "done";
            nextState.approvals = [];
            nextState.runCompletedAtMs =
              nextState.runCompletedAtMs ?? action.receivedAtMs;
          } else if (status === "error") {
            nextState.phase = "error";
            nextState.timelineStatus = "error";
            nextState.approvals = [];
          }
          if (
            status === "cancelled" ||
            status === "finished" ||
            status === "error"
          ) {
            nextState.inspectorSummary = {
              ...nextState.inspectorSummary,
              terminalReason: terminalReasonForEvent(action.event) ?? status,
              terminalMessage: action.event.message,
            };
          }
          records = upsertStep(
            records,
            status === "cancelled"
              ? "run-cancelled"
              : `status:${status ?? "unknown"}`,
            {
              title:
                status === "awaiting_approval"
                  ? "Awaiting approval"
                  : status === "cancelling"
                    ? "Cancelling run"
                    : status === "cancelled"
                      ? "Run cancelled"
                      : "Run status changed",
              description: `Status: ${status ?? "unknown"}`,
              status: "done",
              tag: "runtime",
            },
          );
          break;
        }
        case "RUN_FINISHED":
          nextState.phase = "done";
          nextState.status = "finished";
          nextState.timelineStatus = "done";
          nextState.approvals = [];
          nextState.runCompletedAtMs =
            nextState.runCompletedAtMs ?? action.receivedAtMs;
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            contextWindow: hasContextWindowPayload(action.event)
              ? mergeContextWindowRecord(
                  nextState.inspectorSummary.contextWindow,
                  action.event,
                )
              : nextState.inspectorSummary.contextWindow,
            terminalReason: terminalReasonForEvent(action.event),
            terminalMessage: action.event.result || action.event.message,
          };
          records = upsertStep(records, "run-finished", {
            title: "Run finished",
            description: excerpt(
              action.event.result ||
                nextState.assistantText ||
                "Runtime completed",
            ),
            status: "done",
            tag: action.event.outcome?.type || "success",
          });
          break;
        case "TEXT_MESSAGE_START":
          records = upsertStep(
            records,
            `assistant:${action.event.messageId ?? "message"}`,
            {
              title: "Assistant response",
              description: "Streaming text output",
              status: "active",
              tag: action.event.role || "assistant",
            },
          );
          break;
        case "TEXT_MESSAGE_CONTENT": {
          const delta = action.event.delta ?? "";
          nextState.assistantText += delta;
          records = upsertStep(
            records,
            `assistant:${action.event.messageId ?? "message"}`,
            {
              title: "Assistant response",
              description:
                excerpt(nextState.assistantText) || "Streaming text output",
              status: "active",
              tag: action.event.role || "assistant",
            },
          );
          break;
        }
        case "TEXT_MESSAGE_END":
          records = upsertStep(
            records,
            `assistant:${action.event.messageId ?? "message"}`,
            {
              title: "Assistant response",
              description:
                excerpt(nextState.assistantText) || "Completed text output",
              status: "done",
              tag: action.event.role || "assistant",
            },
          );
          break;
        case "REASONING_START":
          records = upsertStep(
            records,
            `reasoning:${action.event.messageId ?? "reasoning"}`,
            {
              title: "Reasoning",
              description: "Streaming internal reasoning summary",
              status: "active",
              tag: "thinking",
            },
          );
          break;
        case "REASONING_MESSAGE_CONTENT": {
          const delta = action.event.delta ?? "";
          nextState.reasoningText += delta;
          records = upsertStep(
            records,
            `reasoning:${action.event.messageId ?? "reasoning"}`,
            {
              title: "Reasoning",
              description:
                excerpt(nextState.reasoningText) ||
                "Streaming internal reasoning summary",
              status: "active",
              tag: "thinking",
            },
          );
          break;
        }
        case "REASONING_END":
          records = upsertStep(
            records,
            `reasoning:${action.event.messageId ?? "reasoning"}`,
            {
              title: "Reasoning",
              description:
                excerpt(nextState.reasoningText) || "Reasoning complete",
              status: "done",
              tag: "thinking",
            },
          );
          break;
        case "TOOL_CALL_START":
          records = upsertStep(
            records,
            `tool:${action.event.toolCallId ?? "tool"}`,
            {
              title: action.event.toolCallName || "Tool call",
              description: "Tool invocation started",
              status: "active",
              tag: "tool",
            },
          );
          break;
        case "TOOL_CALL_ARGS":
        case "TOOL_CALL_ARGS_DELTA": {
          const id =
            action.event.toolCallId ||
            `tool-result-${nextState.toolResults.length + 1}`;
          const existing = nextState.toolResults.find((tool) => tool.id === id);
          const delta = action.event.delta ?? "";
          const args =
            action.event.type === "TOOL_CALL_ARGS"
              ? delta || existing?.args
              : `${existing?.args ?? ""}${delta}`;
          nextState.toolResults = upsertToolResult(nextState.toolResults, {
            id,
            title: action.event.toolCallName || existing?.title || "Tool call",
            args,
            content: existing?.content ?? "",
            status: "active",
          });
          records = upsertStep(
            records,
            `tool:${action.event.toolCallId ?? "tool"}`,
            {
              title: action.event.toolCallName || "Tool call",
              description: excerpt(
                delta ||
                  (action.event.type === "TOOL_CALL_ARGS_DELTA"
                    ? "Preparing tool arguments"
                    : "Streaming tool arguments"),
              ),
              status: "active",
              tag: "tool",
            },
          );
          break;
        }
        case "TOOL_PROGRESS": {
          const id =
            action.event.toolCallId ||
            `tool-result-${nextState.toolResults.length + 1}`;
          const existing = nextState.toolResults.find((tool) => tool.id === id);
          const progress =
            action.event.progress || action.event.message || "Tool is running";
          nextState.toolResults = upsertToolResult(nextState.toolResults, {
            id,
            title: action.event.toolCallName || existing?.title || "Tool call",
            args: existing?.args,
            content: existing?.content ?? "",
            status: "active",
          });
          records = upsertStep(
            records,
            `tool:${action.event.toolCallId ?? "tool"}`,
            {
              title:
                action.event.toolCallName || existing?.title || "Tool call",
              description: excerpt(progress),
              status: "active",
              tag: "tool",
            },
          );
          break;
        }
        case "AWAKEN_COMMAND_OUTPUT": {
          const id =
            action.event.toolCallId ||
            `tool-result-${nextState.toolResults.length + 1}`;
          const existing = nextState.toolResults.find((tool) => tool.id === id);
          const delta = action.event.delta ?? "";
          const stream = action.event.commandStream || "output";
          nextState.toolResults = upsertToolResult(nextState.toolResults, {
            id,
            title: action.event.toolCallName || existing?.title || "Command",
            args: existing?.args,
            content: existing?.content ?? "",
            commandOutput: `${existing?.commandOutput ?? ""}${delta}`,
            commandOutputTruncated:
              action.event.truncated || existing?.commandOutputTruncated,
            status: "active",
          });
          records = upsertStep(
            records,
            `tool:${action.event.toolCallId ?? "tool"}`,
            {
              title: action.event.toolCallName || existing?.title || "Command",
              description: excerpt(delta || `Streaming ${stream}`),
              status: "active",
              tag: stream,
            },
          );
          break;
        }
        case "AWAKEN_TOOL_PERMISSION_DECIDED": {
          const decision = toolDecisionRecord(action.event);
          const existing = nextState.toolResults.find(
            (tool) => tool.id === decision.toolCallId,
          );
          const status = toolDecisionStatus(decision);
          const decisionLine = [
            decision.action ? `decision: ${decision.action}` : "",
            decision.source ? `source: ${decision.source}` : "",
            decision.reason ? `reason: ${decision.reason}` : "",
            decision.patchSummary ? `patch: ${decision.patchSummary}` : "",
          ]
            .filter(Boolean)
            .join("\n");

          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            lastToolDecision: decision,
          };
          nextState.toolResults = upsertToolResult(nextState.toolResults, {
            id: decision.toolCallId,
            title: decision.toolName || existing?.title || "Tool decision",
            args: existing?.args,
            content: existing?.content || decisionLine,
            commandOutput: existing?.commandOutput,
            commandOutputTruncated: existing?.commandOutputTruncated,
            status,
          });
          records = upsertStep(
            records,
            `tool-decision:${decision.toolCallId}`,
            {
              title: `${decision.toolName} ${decision.action || "decision"}`,
              description: excerpt(
                decision.reason ||
                  decision.source ||
                  decision.patchSummary ||
                  "AUTO decision",
              ),
              status,
              tag: decision.source || "auto",
            },
          );
          break;
        }
        case "TOOL_CALL_RESULT": {
          const content = action.event.content || "Tool returned empty content";
          const id =
            action.event.toolCallId ||
            `tool-result-${nextState.toolResults.length + 1}`;
          const existing = nextState.toolResults.find((tool) => tool.id === id);
          nextState.toolResults = upsertToolResult(nextState.toolResults, {
            id,
            title:
              action.event.toolCallName || existing?.title || "Tool result",
            content,
            args: existing?.args,
            status: toolRecordStatus(action.event.content),
          });

          const stepKey = action.event.toolCallId
            ? `tool:${action.event.toolCallId}`
            : `tool-result:${id}`;
          records = upsertStep(records, stepKey, {
            title: action.event.toolCallName || "Tool result",
            description: excerpt(content),
            status: "done",
            tag: "tool",
          });
          break;
        }
        case "TOOL_CALL_END":
          records = upsertStep(
            records,
            `tool:${action.event.toolCallId ?? "tool"}`,
            {
              title: action.event.toolCallName || "Tool call",
              description: "Tool call complete",
              status: "done",
              tag: "tool",
            },
          );
          break;
        case "AWAKEN_CONTEXT_ESTIMATED":
        case "AWAKEN_CONTEXT_WARNING":
        case "AWAKEN_CONTEXT_BLOCKED": {
          const contextWindow = contextWindowRecord(action.event);
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            contextWindow,
          };
          records = upsertStep(records, "context-window", {
            title:
              action.event.type === "AWAKEN_CONTEXT_BLOCKED"
                ? "Context window blocked"
                : action.event.type === "AWAKEN_CONTEXT_WARNING"
                  ? "Context window warning"
                  : "Context window estimated",
            description: excerpt(
              action.event.message ||
                `${action.event.contextEstimatedTokens ?? 0} tokens`,
            ),
            status:
              action.event.type === "AWAKEN_CONTEXT_BLOCKED" ? "error" : "done",
            tag: "context",
          });
          break;
        }
        case "AWAKEN_COMPACT_STARTED":
        case "AWAKEN_COMPACT_FINISHED":
        case "AWAKEN_COMPACT_FAILED": {
          const compact = compactRecord(
            action.event,
            nextState.inspectorSummary.contextWindow,
          );
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            contextWindow: compact,
            compact,
          };
          records = upsertStep(records, "compact", {
            title:
              action.event.type === "AWAKEN_COMPACT_FAILED"
                ? "Compact failed"
                : action.event.type === "AWAKEN_COMPACT_FINISHED"
                  ? "Compact finished"
                  : "Compact started",
            description: excerpt(
              action.event.message || compact.boundaryId || "Context compact",
            ),
            status:
              compact.status === "failed"
                ? "error"
                : compact.status === "started"
                  ? "active"
                  : "done",
            tag: compact.boundaryId || "compact",
          });
          break;
        }
        case "AWAKEN_PATCH_UPDATED": {
          const id =
            action.event.toolCallId ||
            `patch-${nextState.patchPreviews.length + 1}`;
          nextState.patchPreviews = upsertPatchPreview(
            nextState.patchPreviews,
            {
              id,
              title: action.event.toolCallName || "Patch preview",
              patch: action.event.patch || "",
              summary: action.event.patchSummary,
              affectedPaths: action.event.approvalAffectedPaths ?? [],
              truncated: action.event.truncated,
            },
          );
          records = upsertStep(records, `patch:${id}`, {
            title: "Patch preview updated",
            description: excerpt(
              action.event.patchSummary ||
                action.event.patch ||
                "Patch updated",
            ),
            status: "active",
            tag: action.event.truncated ? "truncated" : "patch",
          });
          break;
        }
        case "AWAKEN_TURN_DIFF":
          nextState.turnDiff = {
            diff: action.event.diff || "",
            truncated: action.event.truncated,
          };
          records = upsertStep(records, "turn-diff", {
            title: "Turn diff captured",
            description: excerpt(action.event.diff || "No diff content"),
            status: "done",
            tag: action.event.truncated ? "truncated" : "diff",
          });
          break;
        case "AWAKEN_APPROVAL_REQUESTED": {
          const approvalId =
            action.event.approvalId ??
            `approval-${nextState.approvals.length + 1}`;
          const approval: ApprovalItem = {
            id: approvalId,
            title: action.event.approvalTitle || "Approval required",
            description: action.event.approvalDescription,
            riskTone: normalizeRiskTone(action.event.approvalRiskTone),
            approveLabel: "Approve",
            rejectLabel: "Reject",
            affectedPaths: action.event.approvalAffectedPaths,
            patchSummary: action.event.approvalPatchSummary,
          };
          nextState.approvals = upsertApproval(nextState.approvals, approval);
          records = upsertStep(records, `approval:${approvalId}`, {
            title: approval.title,
            description:
              approval.description || "Waiting for approval decision",
            status: "active",
            tag: "approval",
          });
          break;
        }
        case "AWAKEN_APPROVAL_RESOLVED": {
          const approvalId = action.event.approvalId ?? "approval";
          nextState.approvals = removeApproval(
            nextState.approvals,
            action.event.approvalId,
          );
          const decision = action.event.approvalDecision || "resolved";
          records = upsertStep(records, `approval:${approvalId}`, {
            title: action.event.approvalTitle || "Approval resolved",
            description:
              action.event.approvalDescription || `Decision: ${decision}`,
            status: decision === "approved" ? "done" : "error",
            tag: decision,
          });
          break;
        }
        case "AWAKEN_ARTIFACT_CREATED":
        case "AWAKEN_ARTIFACT_UPDATED": {
          nextState.artifacts = upsertArtifact(
            nextState.artifacts,
            action.event,
          );
          const artifact =
            nextState.artifacts.find(
              (candidate) => candidate.id === action.event.artifactId,
            ) ?? nextState.artifacts[nextState.artifacts.length - 1];
          records = upsertStep(records, `artifact:${artifact.id}`, {
            title: artifact.title,
            description:
              excerpt(buildArtifactDescription(artifact)) || "Artifact updated",
            status: "done",
            tag:
              action.event.type === "AWAKEN_ARTIFACT_CREATED"
                ? "artifact:new"
                : "artifact:update",
          });
          break;
        }
        case "RUN_ERROR":
          if (action.event.code === "RUN_CANCELLED") {
            nextState.phase = "cancelled";
            nextState.status = "cancelled";
            nextState.timelineStatus = "done";
            nextState.approvals = [];
            records = upsertStep(records, "run-cancelled", {
              title: "Run cancelled",
              description: action.event.message || "Run was cancelled by user",
              status: "done",
              tag: "cancelled",
            });
          } else {
            nextState.phase = "error";
            nextState.status = "error";
            nextState.timelineStatus = "error";
            const rawMessage =
              action.event.message || "Runtime returned an error";
            let errorMessage = rawMessage;
            let errorDescription = rawMessage;
            if (action.event.code === "PROVIDER_AUTH_MISSING") {
              errorMessage =
                "API key 缺失，请在 Settings > Models 配置模型 API key";
              errorDescription =
                "API key 缺失，请在 Settings > Models 配置模型 API key";
            } else if (action.event.code === "PROVIDER_REQUEST_FAILED") {
              errorMessage = `Provider 请求失败：${rawMessage}`;
              errorDescription = `Provider 请求失败：${rawMessage}`;
            }
            nextState.errorMessage = errorMessage;
            nextState.approvals = [];
            records = upsertStep(records, "run-error", {
              title: "Run failed",
              description: errorDescription,
              status: "error",
              tag: action.event.code || "error",
            });
          }
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            terminalReason:
              terminalReasonForEvent(action.event) ??
              action.event.code ??
              "error",
            terminalMessage: nextState.errorMessage || action.event.message,
          };
          break;
        case "AWAKEN_UNKNOWN_EVENT": {
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            unknownEvents: appendUnknownEvent(
              nextState.inspectorSummary.unknownEvents,
              action.event,
            ),
          };
          records = upsertStep(
            records,
            `unknown:${action.event.seq ?? action.event.rawType ?? "event"}`,
            {
              title: action.event.rawType || "Unknown runtime event",
              description: excerpt(
                action.event.message || "Runtime sent an unknown event",
              ),
              status: "done",
              tag: "unknown",
            },
          );
          break;
        }
        default:
          break;
      }

      if (
        nextState.threadId &&
        (action.event.type === "RUN_FINISHED" ||
          action.event.type === "RUN_ERROR" ||
          (action.event.type === "RUN_STATUS_CHANGED" &&
            action.event.status === "cancelled"))
      ) {
        nextState.threads = upsertThread(nextState.threads, {
          threadId: nextState.threadId,
          label:
            state.threads.find(
              (thread) => thread.threadId === nextState.threadId,
            )?.label || state.prompt,
          runId: nextState.runId,
          activeRunId: undefined,
        });
      }

      nextState.steps = toSteps(records);
      const projectedNarrative = projectAguiEventToNarrative(
        state.narrative,
        narrativeEvent,
      );
      nextState.narrative = isSuccessfulTerminalEvent(action.event)
        ? appendFinalFileChangeSummary(
            attachElapsedMsToLatestAnswer(
              projectedNarrative,
              elapsedMsForTerminalRun(nextState, action.receivedAtMs),
              nextState.runId,
            ),
            nextState.artifacts,
            nextState.runId,
          )
        : projectedNarrative;
      return saveActiveThreadView(syncActiveThreadSummary(nextState));
    }
    case "supervisor/status-changed":
      return {
        ...state,
        supervisorStatus: action.info.status,
      };
    default:
      return state;
  }
}
