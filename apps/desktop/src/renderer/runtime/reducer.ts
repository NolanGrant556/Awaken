import type { RunStepItem } from "@awaken/design-system";
import type {
  AguiACPPermissionOption,
  AguiArtifactStatus,
  AguiContextUsageSnapshot,
  AguiEvent,
  AguiModelUsageRecord,
} from "./agui";
import {
  toolDisplayStateFromStatus,
  type ToolDisplayState,
} from "../../shared/toolStatus";
import type {
  RunInputAttachment,
  RuntimeRun,
  RuntimeRunStatus,
  RuntimeThread,
  SupervisorStatus,
  SupervisorStatusInfo,
  ThreadTitleSource,
  ThreadTitleStatus,
} from "../../shared/runtime";
import {
  addRunErrorToNarrative,
  appendRunRequestedNarrative,
  createInitialRunNarrativeState,
  createRunRequestedNarrative,
  projectAguiEventToNarrative,
  type PromptNarrativeToken,
  type DeliverableFileType,
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
  status?: ToolDisplayState;
  /** Canonical ToolOutcome status from the wire; not a display projection. */
  toolStatus?: string;
  diagnostic?: Record<string, unknown>;
  environmentSnapshotId?: string;
  toolAvailabilitySnapshotId?: string;
  providerId?: string;
  capability?: string;
  capabilities?: string[];
  readiness?: string;
  dependency?: Record<string, unknown>;
  dependencies?: Array<Record<string, unknown>>;
  sideEffectState?: string;
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
  status?: AguiArtifactStatus;
  role: "deliverable" | "preview" | "support";
  contractVersion: number;
  producerTool?: string;
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
  snapshot?: AguiContextUsageSnapshot;
  latestModelUsage?: AguiModelUsageRecord;
  modelUsages?: AguiModelUsageRecord[];
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
  /** ExecutionProfile name applied to this call (Task 16 unified observability). */
  profileName?: string;
  /** Original tool name when a generic provider transform is used. */
  transformedFrom?: string;
  /** Rewritten tool name when a generic provider transform is used. */
  transformedTo?: string;
  /** Absolute paths moved to the OS recycle bin during this call. */
  trashedPaths?: string[];
  /** Populated when a guardian constraint was violated. */
  guardianViolation?: string;
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
  projectId?: string;
  label: string;
  titleSource?: ThreadTitleSource;
  titleStatus?: ThreadTitleStatus;
  titleUpdatedAt?: string;
  titleSeedRunId?: string;
  runId?: string;
  activeRunId?: string;
  selectedModelProfileId?: string;
  reasoningEffortByProfile?: Record<string, import("../../shared/runtime").ReasoningEffortLevel>;
  workspaceRoot?: string;
  runStatus?: RuntimeRunStatus;
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
  userQuestions: UserQuestionItem[];
  externalInteractions: ExternalInteractionItem[];
  computerReviews: ComputerReviewItem[];
  computerHandoffs: ComputerHandoffItem[];
  computerSession?: ComputerSessionItem;
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
  userQuestions: UserQuestionItem[];
  externalInteractions: ExternalInteractionItem[];
  computerReviews: ComputerReviewItem[];
  computerHandoffs: ComputerHandoffItem[];
  computerSession?: ComputerSessionItem;
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
      promptTokens?: PromptNarrativeToken[];
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
  | {
      type: "run/threadAdded";
      threadId: string;
      label: string;
      runId: string;
      projectId?: string;
      workspaceRoot?: string;
    }
  | { type: "run/threadSwitch"; threadId: string }
  | { type: "run/threadRenamed"; thread: RuntimeThread }
  | { type: "run/event"; event: AguiEvent; receivedAtMs?: number }
  | { type: "supervisor/status-changed"; info: SupervisorStatusInfo };

export type UserQuestionOption = {
  label: string;
  description?: string;
};

export type UserQuestion = {
  question: string;
  header?: string;
  options: UserQuestionOption[];
  multiSelect?: boolean;
};

export type UserQuestionItem = {
  id: string;
  questions: UserQuestion[];
  sourceTool?: string;
};

export type ExternalInteractionItem = {
  id: string;
  kind: "external" | "acp-permission";
  title?: string;
  safeMessage?: string;
  verificationUrl?: string;
  qrImageUrl?: string;
  provider?: string;
  recoveries: string[];
  sourceTool?: string;
  /** ACP permission options surfaced when kind is "acp-permission". */
  options?: AguiACPPermissionOption[];
};

export type ComputerReviewItem = {
  id: string;
  runId?: string;
  reviewId: string;
  actionId?: string;
  actionDigest?: string;
  targetSummary?: string;
  effectSummary?: string;
  risk?: string;
  sessionId?: string;
  observationId?: string;
  evidenceRefs?: string[];
};

export type ComputerHandoffItem = {
  id: string;
  runId?: string;
  sessionId?: string;
  active: boolean;
};

export type ComputerSessionItem = {
  runId?: string;
  sessionId?: string;
  state: string;
};

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
  userQuestions: [],
  externalInteractions: [],
  computerReviews: [],
  computerHandoffs: [],
  computerSession: undefined,
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
    userQuestions: state.userQuestions,
    externalInteractions: state.externalInteractions,
    computerReviews: state.computerReviews,
    computerHandoffs: state.computerHandoffs,
    computerSession: state.computerSession,
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
  const existing = threads[index];
  const nextIsManual = next.titleSource === "manual";
  const preserveManual = existing.titleSource === "manual" && !nextIsManual;
  const updated = threads.slice();
  updated[index] = {
    ...existing,
    ...next,
    label: existing.label || next.label,
    titleSource: preserveManual
      ? existing.titleSource
      : (next.titleSource ?? existing.titleSource),
    titleStatus: preserveManual
      ? existing.titleStatus
      : (next.titleStatus ?? existing.titleStatus),
    titleUpdatedAt: preserveManual
      ? existing.titleUpdatedAt
      : (next.titleUpdatedAt ?? existing.titleUpdatedAt),
    titleSeedRunId: preserveManual
      ? existing.titleSeedRunId
      : (next.titleSeedRunId ?? existing.titleSeedRunId),
  };
  return updated;
}

export function threadItemFromRuntime(thread: RuntimeThread): ThreadItem {
  return {
    threadId: thread.threadId,
    projectId: thread.projectId,
    label: thread.label || "Untitled thread",
    runId: thread.runId,
    activeRunId: thread.activeRunId,
    selectedModelProfileId: thread.selectedModelProfileId,
    reasoningEffortByProfile: thread.reasoningEffortByProfile,
    workspaceRoot: thread.workspace.workspaceRoot,
    runStatus: thread.activeRunId ? "running" : undefined,
    artifactCount: 0,
    ...(thread.titleSource ? { titleSource: thread.titleSource } : {}),
    ...(thread.titleStatus ? { titleStatus: thread.titleStatus } : {}),
    ...(thread.titleUpdatedAt
      ? { titleUpdatedAt: thread.titleUpdatedAt }
      : {}),
    ...(thread.titleSeedRunId
      ? { titleSeedRunId: thread.titleSeedRunId }
      : {}),
  };
}

function isActiveRunStatus(status?: RuntimeRunStatus): boolean {
  return status === "running" || status === "cancelling";
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
      label: existing?.label || "Untitled thread",
      projectId: existing?.projectId,
      runId: state.runId ?? existing?.runId,
      activeRunId: isActiveRunStatus(state.status) ? state.runId : undefined,
      runStatus: state.status ?? existing?.runStatus,
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

function terminalStatusForEvent(
  event: AguiEvent,
): "finished" | "error" | "cancelled" | undefined {
  if (
    event.type === "RUN_FINISHED" ||
    (event.type === "RUN_STATUS_CHANGED" && event.status === "finished")
  ) {
    return "finished";
  }
  if (
    (event.type === "RUN_ERROR" && event.code === "RUN_CANCELLED") ||
    (event.type === "RUN_STATUS_CHANGED" && event.status === "cancelled")
  ) {
    return "cancelled";
  }
  if (
    event.type === "RUN_ERROR" ||
    (event.type === "RUN_STATUS_CHANGED" && event.status === "error")
  ) {
    return "error";
  }
  return undefined;
}

function terminalStatusForRun(
  status: RuntimeRunStatus,
): "finished" | "error" | "cancelled" | undefined {
  switch (status) {
    case "finished":
    case "error":
    case "cancelled":
      return status;
    default:
      return undefined;
  }
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

const deliverableExtensionTypes = new Map<string, DeliverableFileType>([
  ["doc", "word"],
  ["docx", "word"],
  ["md", "markdown"],
  ["xls", "excel"],
  ["xlsx", "excel"],
  ["csv", "excel"],
  ["tsv", "excel"],
  ["pdf", "pdf"],
  ["ppt", "powerpoint"],
  ["pptx", "powerpoint"],
  ["png", "image"],
  ["jpg", "image"],
  ["jpeg", "image"],
  ["webp", "image"],
  ["mp4", "video"],
]);

function deliverableFileType(path: string): {
  extension: string;
  fileType: DeliverableFileType;
} | undefined {
  const filename = path.replace(/\\/g, "/").split("/").pop() ?? "";
  const dot = filename.lastIndexOf(".");
  if (dot <= 0 || dot === filename.length - 1) {
    return undefined;
  }
  const extension = filename.slice(dot + 1).toLowerCase();
  const fileType = deliverableExtensionTypes.get(extension);
  return fileType ? { extension, fileType } : undefined;
}

function appendFinalDeliverableSummary(
  narrative: RunNarrativeState,
  artifacts: ArtifactRecord[],
  terminalStatus: "finished" | "error" | "cancelled",
  runId?: string,
): RunNarrativeState {
  const normalizedRunId = runId ?? "current";
  const byPath = new Map<
    string,
    { artifact: ArtifactRecord; extension?: string; fileType?: DeliverableFileType }
  >();
  const pathOrder: string[] = [];

  for (const artifact of artifacts) {
    if (!artifact.path) {
      continue;
    }
    const type = deliverableFileType(artifact.path);
    const normalizedPath = artifact.path.replace(/\\/g, "/");
    const key = normalizedPath.toLowerCase();
    if (!byPath.has(key)) {
      pathOrder.push(key);
    }
    byPath.set(key, { artifact, ...type });
  }

  const eligiblePaths = pathOrder.filter((key) => {
    const candidate = byPath.get(key);
    return Boolean(
      candidate?.artifact.role === "deliverable" &&
        candidate.artifact.status === "ready" &&
        candidate.extension &&
        candidate.fileType,
    );
  });

  const id = `deliverables:${normalizedRunId}:summary`;
  const withoutLegacyFileChanges = narrative.committedItems.filter(
    (candidate) => candidate.kind !== "fileChange",
  );
  if (eligiblePaths.length === 0) {
    return {
      ...narrative,
      committedItems: withoutLegacyFileChanges.filter(
        (candidate) => !(candidate.kind === "deliverables" && candidate.id === id),
      ),
      activeItem:
        narrative.activeItem?.kind === "fileChange" || narrative.activeItem?.id === id
          ? undefined
          : narrative.activeItem,
    };
  }

  const item: RunNarrativeItem = {
    kind: "deliverables",
    id,
    runId: normalizedRunId,
    terminalStatus,
    files: eligiblePaths.map((key) => {
      const { artifact, extension, fileType } = byPath.get(key)!;
      const displayPath = artifact.path!.replace(/\\/g, "/");
      return {
        artifactId: artifact.id,
        runId: normalizedRunId,
        title: artifact.title || displayPath.split("/").pop() || "交付文件",
        displayPath,
        fileType: fileType!,
        extension: extension!,
        legacy: artifact.contractVersion < 2,
      };
    }),
  };

  return {
    ...narrative,
    committedItems: [
      ...withoutLegacyFileChanges.filter(
        (candidate) =>
          !(candidate.kind === "deliverables" && candidate.id === id),
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

function normalizeArtifactStatus(value?: string): AguiArtifactStatus | undefined {
  switch (value) {
    case "ready":
    case "pending":
    case "error":
      return value;
    default:
      return undefined;
  }
}

function contextWindowRecord(event: AguiEvent): ContextWindowRecord {
  const snapshot = event.contextUsageSnapshot;
  const agentTurnUsage =
    event.modelUsage?.purpose === "agent_turn" ? event.modelUsage : undefined;
  return {
    eventType: event.type,
    message: event.message,
    modelWindow: snapshot?.modelWindowTokens ?? event.contextModelWindow,
    outputReserve: snapshot?.outputReserveTokens ?? event.contextOutputReserve,
    effectiveWindow:
      snapshot?.effectiveInputWindowTokens ?? event.contextEffectiveWindow,
    estimatedTokens:
      snapshot?.estimatedInputTokens ?? event.contextEstimatedTokens,
    actualTokens: event.contextActualTokens,
    warningThreshold:
      snapshot?.warningThresholdTokens ?? event.contextWarningThreshold,
    autoCompactThreshold:
      snapshot?.autoCompactThresholdTokens ??
      event.contextAutoCompactThreshold,
    compactTarget: snapshot?.compactTargetTokens ?? event.contextCompactTarget,
    estimatedDeltaTokens: event.contextEstimatedDeltaTokens,
    snapshot,
    latestModelUsage: agentTurnUsage,
    modelUsages: event.modelUsage ? [event.modelUsage] : [],
  };
}

function mergeModelUsages(
  previous: AguiModelUsageRecord[] | undefined,
  next: AguiModelUsageRecord[] | undefined,
): AguiModelUsageRecord[] {
  const merged = [...(previous ?? [])];
  for (const usage of next ?? []) {
    const index = merged.findIndex((entry) => entry.requestId === usage.requestId);
    if (index >= 0) merged[index] = usage;
    else merged.push(usage);
  }
  return merged;
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
  ].some((value) => typeof value === "number" && Number.isFinite(value)) ||
    Boolean(event.contextUsageSnapshot);
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
    snapshot: next.snapshot ?? previous.snapshot,
    latestModelUsage: next.latestModelUsage ?? previous.latestModelUsage,
    modelUsages: mergeModelUsages(previous.modelUsages, next.modelUsages),
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
    affectedPaths: event.affectedPaths ?? [],
    patchSummary: event.patchSummary,
    // Task 16 unified observability: carry the profile name, transform
    // origin, trashed paths, and guardian violation reason so the UI can
    // render them.
    profileName: event.profileName,
    transformedFrom: event.transformedFrom,
    transformedTo: event.transformedTo,
    trashedPaths: event.trashedPaths,
    guardianViolation: event.guardianViolation,
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

function upsertUserQuestion(
  questions: UserQuestionItem[],
  next: UserQuestionItem,
): UserQuestionItem[] {
  const index = questions.findIndex((question) => question.id === next.id);
  if (index < 0) {
    return [...questions, next];
  }

  const updated = questions.slice();
  updated[index] = next;
  return updated;
}

function removeUserQuestion(
  questions: UserQuestionItem[],
  questionId?: string,
): UserQuestionItem[] {
  if (!questionId) {
    return questions;
  }
  return questions.filter((question) => question.id !== questionId);
}

function upsertExternalInteraction(
  items: ExternalInteractionItem[],
  next: ExternalInteractionItem,
): ExternalInteractionItem[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index < 0) {
    return [...items, next];
  }
  const updated = items.slice();
  updated[index] = next;
  return updated;
}

function removeExternalInteraction(
  items: ExternalInteractionItem[],
  interactionId?: string,
): ExternalInteractionItem[] {
  if (!interactionId) {
    return items;
  }
  return items.filter((item) => item.id !== interactionId);
}

function upsertComputerReview(
  items: ComputerReviewItem[],
  next: ComputerReviewItem,
): ComputerReviewItem[] {
  const index = items.findIndex((item) => item.reviewId === next.reviewId);
  if (index < 0) {
    return [...items, next];
  }
  const updated = items.slice();
  updated[index] = next;
  return updated;
}

function removeComputerReview(
  items: ComputerReviewItem[],
  reviewId?: string,
): ComputerReviewItem[] {
  if (!reviewId) {
    return items;
  }
  return items.filter((item) => item.reviewId !== reviewId);
}

function upsertComputerHandoff(
  items: ComputerHandoffItem[],
  next: ComputerHandoffItem,
): ComputerHandoffItem[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index < 0) {
    return [...items, next];
  }
  const updated = items.slice();
  updated[index] = next;
  return updated;
}

function inferLegacyArtifactRole(input: {
  kind?: string;
  path?: string;
  producerTool?: string;
}): "deliverable" | "preview" | "support" {
  const producer = input.producerTool?.trim().toLowerCase() ?? "";
  const kind = input.kind?.trim().toLowerCase() ?? "";
  const extension = input.path
    ? deliverableFileType(input.path)?.extension
    : undefined;

  if (
    producer === "presentation_render" ||
    producer === "artifact_render" ||
    producer.endsWith("_render")
  ) {
    return "preview";
  }
  if (producer === "presentation_create" || producer === "presentation_edit") {
    return extension === "ppt" || extension === "pptx"
      ? "deliverable"
      : "preview";
  }
  if (producer === "document_create" || producer === "document_edit") {
    return extension === "doc" || extension === "docx" || extension === "md"
      ? "deliverable"
      : "support";
  }
  if (producer === "spreadsheet_create" || producer === "spreadsheet_edit") {
    return extension === "xls" ||
      extension === "xlsx" ||
      extension === "csv" ||
      extension === "tsv"
      ? "deliverable"
      : "support";
  }
  if (
    producer === "artifact_export" ||
    producer === "artifact_publish" ||
    producer === "write_file" ||
    (producer === "text_file_create" && extension === "md")
  ) {
    return extension ? "deliverable" : "support";
  }
  if (kind === "image") {
    return "preview";
  }
  return "support";
}

function upsertArtifact(
  artifacts: ArtifactRecord[],
  event: AguiEvent,
): ArtifactRecord[] {
  const id = event.artifactId ?? `artifact-${artifacts.length + 1}`;
  const existingIndex = artifacts.findIndex((artifact) => artifact.id === id);
  const existing = existingIndex >= 0 ? artifacts[existingIndex] : undefined;
  const producerTool =
    event.artifactProducer || event.toolCallName || existing?.producerTool;
  const contractVersion = Number.isFinite(event.artifactContractVersion)
    ? Math.max(1, Number(event.artifactContractVersion))
    : existing?.contractVersion ?? 1;
  const role =
    event.artifactRole ||
    existing?.role ||
    inferLegacyArtifactRole({
      kind: event.artifactKind,
      path: event.artifactPath,
      producerTool,
    });
  const next: ArtifactRecord = {
    id,
    title: event.artifactTitle || existing?.title || "Artifact",
    description: event.artifactDescription || existing?.description,
    kind: event.artifactKind || existing?.kind || "file",
    path: event.artifactPath || existing?.path,
    content: event.artifactContent ?? existing?.content,
    status: normalizeArtifactStatus(event.artifactStatus || existing?.status),
    role,
    contractVersion,
    producerTool,
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
		userQuestions: [],
		externalInteractions: [],
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
		userQuestions: [],
		externalInteractions: [],
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
		userQuestions: [],
		externalInteractions: [],
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
		userQuestions: [],
		externalInteractions: [],
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
		userQuestions: [],
		externalInteractions: [],
		computerReviews: [],
		computerHandoffs: [],
		computerSession: undefined,
		artifacts: [],
        turnDiff: undefined,
        // Context usage is thread-scoped next-request state, not run-local
        // presentation state. Keep the last durable snapshot while a follow-up
        // starts and while transcript replay advances across later runs that
        // may have terminated before producing their own snapshot.
        inspectorSummary: {
          ...createInspectorSummary(),
          contextWindow:
            state.activeThreadId || state.threadId
              ? state.inspectorSummary.contextWindow
              : undefined,
        },
        narrative: shouldAppendNarrative(state)
          ? appendRunRequestedNarrative(
              state.narrative,
              action.prompt,
              action.promptAttachments,
              nextNarrativeScope(state),
              action.promptTokens,
            )
          : createRunRequestedNarrative(
              action.prompt,
              action.promptAttachments,
              nextNarrativeScope(state),
              action.promptTokens,
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
		userQuestions: [],
		externalInteractions: [],
	}),
      );
    case "run/cancelSync": {
      const synced = syncActiveThreadSummary(syncFromRun(state, action.run));
      const terminalStatus = terminalStatusForRun(action.run.status);
      if (!terminalStatus) {
        return saveActiveThreadView(synced);
      }

      let terminalNarrative = synced.narrative;
      if (terminalStatus === "error") {
        terminalNarrative = addRunErrorToNarrative(
          terminalNarrative,
          action.run.error || "Run failed",
          `${action.run.runId}:sync`,
        );
      } else {
        terminalNarrative = projectAguiEventToNarrative(terminalNarrative, {
          type: "RUN_STATUS_CHANGED",
          threadId: action.run.threadId,
          runId: action.run.runId,
          status: terminalStatus,
          message:
            terminalStatus === "cancelled"
              ? "你停止生成了本次回答"
              : undefined,
        });
      }
      return saveActiveThreadView({
        ...synced,
        narrative: appendFinalDeliverableSummary(
          terminalNarrative,
          synced.artifacts,
          terminalStatus,
          action.run.runId,
        ),
      });
    }
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
		userQuestions: [],
		externalInteractions: [],
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
          narrative: appendFinalDeliverableSummary(
            addRunErrorToNarrative(
              state.narrative,
              action.message,
              `${currentNarrativeScope(state)}:stream`,
            ),
            state.artifacts,
            "error",
            state.runId,
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
      const hasPromptTokens = (action.run.composerTokens?.length ?? 0) > 0;
      return saveActiveThreadView(
        syncActiveThreadSummary({
          ...syncFromRun(initialState, action.run),
          prompt,
          narrative: prompt || hasPromptTokens
            ? createRunRequestedNarrative(
                prompt,
                action.run.inputAttachments,
                recoveredNarrativeScope(action.run),
                action.run.composerTokens,
              )
            : hasPromptAttachments
              ? createRunRequestedNarrative(
                  prompt,
                  action.run.inputAttachments,
                  recoveredNarrativeScope(action.run),
                  action.run.composerTokens,
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
        const fromRuntime = threadItemFromRuntime(thread);
        const preserveManual =
          existing?.titleSource === "manual" &&
          fromRuntime.titleSource !== "manual";
        return {
          ...fromRuntime,
          label: preserveManual
            ? existing?.label || fromRuntime.label
            : fromRuntime.label,
          titleSource: preserveManual
            ? existing?.titleSource
            : fromRuntime.titleSource,
          titleStatus: preserveManual
            ? existing?.titleStatus
            : fromRuntime.titleStatus,
          titleUpdatedAt: preserveManual
            ? existing?.titleUpdatedAt
            : fromRuntime.titleUpdatedAt,
          titleSeedRunId: preserveManual
            ? existing?.titleSeedRunId
            : fromRuntime.titleSeedRunId,
          runStatus:
            existing?.runStatus ?? (thread.activeRunId ? "running" : undefined),
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
          projectId: action.projectId,
          activeRunId: action.runId,
          workspaceRoot: action.workspaceRoot,
          runStatus: "running",
          artifactCount: 0,
        }),
      };
      return saveActiveThreadView(next);
    }
    case "run/threadSwitch":
      return restoreThreadView(saveActiveThreadView(state), action.threadId);
    case "run/threadRenamed": {
      const next = threadItemFromRuntime(action.thread);
      const existing = state.threads.find(
        (thread) => thread.threadId === next.threadId,
      );
      return {
        ...state,
        threads: state.threads.map((thread) =>
          thread.threadId === next.threadId
            ? {
                ...next,
                runStatus: existing?.runStatus ?? next.runStatus,
                artifactCount: existing?.artifactCount ?? 0,
              }
            : thread,
        ),
      };
    }
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
          // The client records run/requested before startRun and Skill preflight
          // complete. Rebase elapsed time onto the durable server event so the
          // live view and transcript replay use the same clock boundary.
          nextState.runStartedAtMs = validTimestamp(action.event.timestamp)
            ? action.event.timestamp
            : nextState.runStartedAtMs;
          nextState.activeThreadId =
            nextState.activeThreadId ?? action.event.threadId;
          if (action.event.threadId) {
            nextState.threads = upsertThread(state.threads, {
              threadId: action.event.threadId,
              label: "Untitled thread",
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
          } else if (status === "cancelling") {
            nextState.phase = "cancelling";
            nextState.timelineStatus = "running";
            nextState.userQuestions = [];
            nextState.externalInteractions = [];
            nextState.computerReviews = [];
            nextState.computerHandoffs = [];
          } else if (status === "cancelled") {
            nextState.phase = "cancelled";
            nextState.timelineStatus = "done";
            nextState.userQuestions = [];
            nextState.externalInteractions = [];
            nextState.computerReviews = [];
            nextState.computerHandoffs = [];
          } else if (status === "finished") {
            nextState.phase = "done";
            nextState.timelineStatus = "done";
            nextState.userQuestions = [];
            nextState.externalInteractions = [];
            nextState.computerReviews = [];
            nextState.computerHandoffs = [];
            nextState.runCompletedAtMs =
              nextState.runCompletedAtMs ??
              (validTimestamp(action.event.timestamp)
                ? action.event.timestamp
                : action.receivedAtMs);
          } else if (status === "error") {
            nextState.phase = "error";
            nextState.timelineStatus = "error";
            nextState.userQuestions = [];
            nextState.externalInteractions = [];
            nextState.computerReviews = [];
            nextState.computerHandoffs = [];
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
                status === "cancelling"
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
          nextState.userQuestions = [];
          nextState.externalInteractions = [];
          nextState.computerReviews = [];
          nextState.computerHandoffs = [];
          nextState.runCompletedAtMs =
            nextState.runCompletedAtMs ??
            (validTimestamp(action.event.timestamp)
              ? action.event.timestamp
              : action.receivedAtMs);
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
          if (!(action.event.recordId && nextState.assistantText.endsWith(delta))) {
            nextState.assistantText += delta;
          }
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
          // Task 16 unified observability: include the profile name, transform
          // origin, trashed paths, and guardian violation in the decision
          // line so the inspector surfaces the full picture. Each piece is
          // only added when present so the line stays compact for the common
          // allow case.
          const transformLine =
            decision.transformedFrom && decision.transformedTo
              ? `transform: ${decision.transformedFrom} → ${decision.transformedTo}`
              : "";
          const profileLine = decision.profileName
            ? `profile: ${decision.profileName}`
            : "";
          const trashLine =
            decision.trashedPaths && decision.trashedPaths.length > 0
              ? `trashed: ${decision.trashedPaths.length} file(s)`
              : "";
          const guardianLine = decision.guardianViolation
            ? `guardian: ${decision.guardianViolation}`
            : "";
          const decisionLine = [
            decision.action ? `decision: ${decision.action}` : "",
            decision.source ? `source: ${decision.source}` : "",
            profileLine,
            transformLine,
            trashLine,
            guardianLine,
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
                transformLine ||
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
          const diagnosticReason =
            typeof action.event.diagnostic?.reason === "string"
              ? action.event.diagnostic.reason
              : "";
          // TrashedPaths are parsed from the tool output after execution and
          // now ride on the ToolResult event (post-execution). Surface the
          // "Deleted file moved to recycle bin" notification in the step
          // description and refresh inspectorSummary.lastToolDecision so the
          // inspector reflects the actual trashed paths rather than the nil
          // value carried by the pre-execution permission-decided event.
          const trashedPaths = action.event.trashedPaths ?? [];
          const trashNote =
            trashedPaths.length > 0 ? "Deleted file moved to recycle bin" : "";
          const id =
            action.event.toolCallId ||
            `tool-result-${nextState.toolResults.length + 1}`;
          const existing = nextState.toolResults.find((tool) => tool.id === id);
          const displayStatus = toolDisplayStateFromStatus(
            action.event.toolStatus,
          );
          nextState.toolResults = upsertToolResult(nextState.toolResults, {
            id,
            title:
              action.event.toolCallName || existing?.title || "Tool result",
            content,
            args: existing?.args,
            status: displayStatus,
            toolStatus: action.event.toolStatus,
            ...(action.event.diagnostic
              ? { diagnostic: action.event.diagnostic }
              : {}),
            ...(action.event.environmentSnapshotId !== undefined
              ? { environmentSnapshotId: action.event.environmentSnapshotId }
              : {}),
            ...(action.event.toolAvailabilitySnapshotId !== undefined
              ? {
                  toolAvailabilitySnapshotId:
                    action.event.toolAvailabilitySnapshotId,
                }
              : {}),
            ...(action.event.providerId !== undefined
              ? { providerId: action.event.providerId }
              : {}),
            ...(action.event.capability !== undefined
              ? { capability: action.event.capability }
              : {}),
            ...(action.event.capabilities !== undefined
              ? { capabilities: action.event.capabilities }
              : {}),
            ...(action.event.readiness !== undefined
              ? { readiness: action.event.readiness }
              : {}),
            ...(action.event.dependency !== undefined
              ? { dependency: action.event.dependency }
              : {}),
            ...(action.event.dependencies !== undefined
              ? { dependencies: action.event.dependencies }
              : {}),
            ...(action.event.sideEffectState !== undefined
              ? { sideEffectState: action.event.sideEffectState }
              : {}),
          });

          const stepKey = action.event.toolCallId
            ? `tool:${action.event.toolCallId}`
            : `tool-result:${id}`;
          const description = [
            excerpt(content),
            diagnosticReason ? `diagnostic: ${diagnosticReason}` : "",
            trashNote,
          ]
            .filter(Boolean)
            .join("\n");
          const stepStatus: "active" | "done" | "error" =
            displayStatus === "error" ||
            displayStatus === "denied" ||
            displayStatus === "cancelled" ||
            displayStatus === "interrupted" ||
            displayStatus === "blocked" ||
            displayStatus === "unsupported"
              ? "error"
              : "done";
          records = upsertStep(records, stepKey, {
            title: action.event.toolCallName || "Tool result",
            description,
            status: stepStatus,
            tag: "tool",
          });
          if (trashedPaths.length > 0) {
            const prevDecision = nextState.inspectorSummary.lastToolDecision;
            nextState.inspectorSummary = {
              ...nextState.inspectorSummary,
              lastToolDecision: prevDecision
                ? { ...prevDecision, trashedPaths }
                : {
                    toolCallId: action.event.toolCallId ?? "",
                    toolName: action.event.toolCallName ?? "Tool",
                    affectedPaths: [],
                    trashedPaths,
                  },
            };
          }
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
        case "AWAKEN_CONTEXT_SNAPSHOT": {
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            contextWindow: mergeContextWindowRecord(
              nextState.inspectorSummary.contextWindow,
              action.event,
            ),
          };
          break;
        }
        case "AWAKEN_MODEL_USAGE": {
          nextState.inspectorSummary = {
            ...nextState.inspectorSummary,
            contextWindow: mergeContextWindowRecord(
              nextState.inspectorSummary.contextWindow,
              action.event,
            ),
          };
          break;
        }
        case "AWAKEN_CONTEXT_ESTIMATED":
        case "AWAKEN_CONTEXT_WARNING":
        case "AWAKEN_CONTEXT_BLOCKED": {
          const contextWindow = mergeContextWindowRecord(
            nextState.inspectorSummary.contextWindow,
            action.event,
          );
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
              affectedPaths: action.event.affectedPaths ?? [],
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
        case "AWAKEN_AUTH_REQUIRED": {
          const authKey = [
            action.event.toolCallId,
            action.event.authProvider,
            action.event.authScope,
          ]
            .filter(Boolean)
            .join(":");
          records = upsertStep(records, `auth:${authKey || "required"}`, {
            title: "Authorization required",
            description: [
              action.event.authProvider,
              action.event.authScope,
              action.event.authUrl,
            ]
              .filter(Boolean)
              .join(" | "),
            status: "active",
            tag: "auth",
          });
          break;
        }
        case "AWAKEN_RUNTIME_PREPARE":
        case "AWAKEN_RUNTIME_PREPARE_PROGRESS":
        case "AWAKEN_RUNTIME_READY":
        case "AWAKEN_RUNTIME_REPAIR_FAILED":
        case "AWAKEN_RUNTIME_MISSING": {
          const runtimeKey = [
            action.event.runtimeName,
            action.event.runtimeComponent,
          ]
            .filter(Boolean)
            .join(":");
          const missing = action.event.type === "AWAKEN_RUNTIME_MISSING" || action.event.type === "AWAKEN_RUNTIME_REPAIR_FAILED";
          const ready = action.event.type === "AWAKEN_RUNTIME_READY";
          records = upsertStep(records, `runtime:${runtimeKey || "tool"}`, {
            title: ready
              ? "Runtime ready"
              : missing
                ? "Runtime missing"
                : "Runtime preparing",
            description: [
              action.event.runtimeComponent,
              action.event.runtimePrepareStatus,
              action.event.runtimeMissingReason,
              action.event.runtimeRequiredVersion,
              action.event.message,
            ]
              .filter(Boolean)
              .join(" | "),
            status: missing ? "error" : ready ? "done" : "active",
            tag: "runtime",
          });
          break;
        }
        case "AWAKEN_SKILL_DISCOVERED":
        case "AWAKEN_SKILL_INVOKED":
        case "AWAKEN_SKILL_CONTENT_LOADED":
        case "AWAKEN_SKILL_EXECUTION_CONTEXT":
        case "AWAKEN_SKILL_FAILED_TO_LOAD": {
          const skillId = action.event.skillId ?? "unknown-skill";
          const contentLoaded = action.event.type === "AWAKEN_SKILL_CONTENT_LOADED";
          const failed = action.event.type === "AWAKEN_SKILL_FAILED_TO_LOAD";
          const active = action.event.type === "AWAKEN_SKILL_INVOKED";
          const stepId = contentLoaded
            ? `skill:${skillId}:resource:${action.event.skillResource ?? "content"}`
            : `skill:${skillId}`;
          const title = failed
            ? "Skill failed to load"
            : contentLoaded
              ? "Skill content loaded"
              : active
                ? "Skill invoked"
                : action.event.type === "AWAKEN_SKILL_DISCOVERED"
                  ? "Skill discovered"
                  : "Skill execution context ready";
          records = upsertStep(records, stepId, {
            title,
            description: [
              action.event.skillId,
              action.event.skillVersion,
              action.event.skillSource,
              action.event.skillPath,
              action.event.skillResource,
              action.event.skillStatus,
              ...(action.event.skillDiagnostics ?? []),
            ]
              .filter(Boolean)
              .join(" | "),
            status: failed ? "error" : active ? "active" : "done",
            tag: "skill",
          });
          break;
        }
        case "AWAKEN_TOOL_RECOVERY_REQUIRED":
        case "AWAKEN_TOOL_RETRY_BLOCKED": {
          const recoveryKey = [
            action.event.toolCallId,
            action.event.toolCallName,
            action.event.errorCode ?? action.event.code,
          ]
            .filter(Boolean)
            .join(":");
          const blocked = action.event.type === "AWAKEN_TOOL_RETRY_BLOCKED";
          records = upsertStep(records, `recovery:${recoveryKey || "tool"}`, {
            title: blocked ? "Tool retry blocked" : "Tool recovery required",
            description: [
              action.event.toolCallName,
              action.event.errorCode ?? action.event.code,
              action.event.message,
              ...(action.event.recoveryOptions ?? []),
            ]
              .filter(Boolean)
              .join(" | "),
            status: "error",
            tag: blocked ? "retry-blocked" : "recovery",
          });
          break;
        }
        case "AWAKEN_USER_QUESTION_REQUESTED": {
          const questionId = action.event.questionId ?? action.event.toolCallId ?? "user-question";
          let questions: UserQuestion[] = [];
          try {
            const parsed = JSON.parse(action.event.content ?? "{}");
            if (Array.isArray(parsed.questions)) {
              questions = parsed.questions as UserQuestion[];
            }
          } catch {
            // ignore parse errors
          }
          nextState.userQuestions = upsertUserQuestion(
            nextState.userQuestions,
            {
              id: questionId,
              questions,
              sourceTool: action.event.toolCallName,
            },
          );
          records = upsertStep(records, `user-question:${questionId}`, {
            title: "Waiting for your answer",
            description: questions.map((q) => q.question).join(" | "),
            status: "active",
            tag: "user-question",
          });
          break;
        }
        case "AWAKEN_USER_QUESTION_RESOLVED": {
          const questionId = action.event.questionId ?? action.event.toolCallId ?? "user-question";
          nextState.userQuestions = removeUserQuestion(
            nextState.userQuestions,
            questionId,
          );
          records = upsertStep(records, `user-question:${questionId}`, {
            title: "Answer received",
            description: action.event.questionAnswer,
            status: "done",
            tag: "user-question",
          });
          break;
        }
        case "AWAKEN_EXTERNAL_INTERACTION_REQUESTED": {
          // Ephemeral composer gate only — never create narrative/step records.
          if (
            nextState.status === "finished" ||
            nextState.status === "error" ||
            nextState.status === "cancelled"
          ) {
            break;
          }
          const interactionId =
            action.event.interactionId ?? action.event.toolCallId ?? "external-interaction";
          nextState.externalInteractions = upsertExternalInteraction(
            nextState.externalInteractions,
            {
              id: interactionId,
              kind: "external",
              title: action.event.interactionTitle,
              safeMessage: action.event.interactionSafeMessage,
              verificationUrl: action.event.interactionVerificationUrl ?? action.event.authUrl,
              qrImageUrl: action.event.interactionQrImageUrl,
              provider: action.event.interactionProvider ?? action.event.authProvider,
              recoveries: Array.isArray(action.event.interactionRecoveries)
                ? action.event.interactionRecoveries
                : ["confirm_done", "cancel"],
              sourceTool: action.event.toolCallName,
            },
          );
          break;
        }
        case "AWAKEN_EXTERNAL_INTERACTION_RESOLVED": {
          const interactionId =
            action.event.interactionId ?? action.event.toolCallId ?? "external-interaction";
          nextState.externalInteractions = removeExternalInteraction(
            nextState.externalInteractions,
            interactionId,
          );
          break;
        }
        case "AWAKEN_ACP_PERMISSION_REQUESTED": {
          // Ephemeral composer gate only — never create narrative/step records.
          if (
            nextState.status === "finished" ||
            nextState.status === "error" ||
            nextState.status === "cancelled"
          ) {
            break;
          }
          const interactionId =
            action.event.interactionId ?? action.event.toolCallId ?? "acp-permission";
          nextState.externalInteractions = upsertExternalInteraction(
            nextState.externalInteractions,
            {
              id: interactionId,
              kind: "acp-permission",
              title: action.event.message ?? "外部 Agent 请求权限",
              safeMessage: action.event.acpRunnerId
                ? `${action.event.acpRunnerId} 需要你的确认`
                : "外部 Agent 需要你的确认",
              provider: action.event.acpRunnerId,
              recoveries: ["acp_option"],
              sourceTool: action.event.toolCallName,
              options: action.event.acpPermissionOptions ?? [],
            },
          );
          break;
        }
        case "AWAKEN_ACP_PERMISSION_RESOLVED": {
          const interactionId =
            action.event.interactionId ?? action.event.toolCallId ?? "acp-permission";
          nextState.externalInteractions = removeExternalInteraction(
            nextState.externalInteractions,
            interactionId,
          );
          break;
        }
        case "AWAKEN_COMPUTER_REVIEW_REQUESTED": {
          if (
            nextState.status === "finished" ||
            nextState.status === "error" ||
            nextState.status === "cancelled"
          ) {
            break;
          }
          const reviewId = action.event.computerReviewId ?? action.event.toolCallId ?? "computer-review";
          nextState.computerReviews = upsertComputerReview(nextState.computerReviews, {
            id: reviewId,
            runId: action.event.runId ?? nextState.runId,
            reviewId,
            actionId: action.event.computerActionId,
            actionDigest: action.event.computerActionDigest,
            targetSummary: action.event.computerTargetSummary,
            effectSummary: action.event.computerEffectSummary,
            risk: action.event.computerRisk,
            sessionId: action.event.computerSessionId,
            observationId: action.event.computerObservationId,
            evidenceRefs: action.event.computerEvidenceRefs,
          });
          records = upsertStep(records, `computer-review:${reviewId}`, {
            title: "等待计算机操作确认",
            description: action.event.computerTargetSummary || "Agent 请求执行计算机操作",
            status: "active",
            tag: "computer-review",
          });
          break;
        }
        case "AWAKEN_COMPUTER_REVIEW_RESOLVED": {
          const reviewId = action.event.computerReviewId ?? action.event.toolCallId ?? "computer-review";
          nextState.computerReviews = removeComputerReview(nextState.computerReviews, reviewId);
          records = upsertStep(records, `computer-review:${reviewId}`, {
            title: "计算机操作已确认",
            description: `${action.event.computerDecision ?? "resolved"}${action.event.computerActor ? ` by ${action.event.computerActor}` : ""}`,
            status: "done",
            tag: "computer-review",
          });
          break;
        }
        case "AWAKEN_COMPUTER_SESSION_CHANGED": {
          const sessionState = action.event.computerSessionState ?? "ready";
          nextState.computerSession =
            sessionState === "closed"
              ? undefined
              : {
                  runId: action.event.runId ?? nextState.runId,
                  sessionId: action.event.computerSessionId,
                  state: sessionState,
                };
          break;
        }
        case "AWAKEN_COMPUTER_VERIFICATION_COMPLETED": {
          const actionId = action.event.computerActionId ?? "computer-action";
          records = upsertStep(records, `computer-verification:${actionId}`, {
            title: "计算机操作结果验证",
            description: action.event.computerVerificationStatus ?? "inconclusive",
            status: "done",
            tag: "computer-verification",
          });
          break;
        }
        case "AWAKEN_COMPUTER_HANDOFF_STARTED": {
          const handoffId = action.event.computerSessionId ?? action.event.runId ?? "computer-handoff";
          nextState.computerHandoffs = upsertComputerHandoff(nextState.computerHandoffs, {
            id: handoffId,
            runId: action.event.runId ?? nextState.runId,
            sessionId: action.event.computerSessionId,
            active: true,
          });
          nextState.computerSession = {
            runId: action.event.runId ?? nextState.runId,
            sessionId: action.event.computerSessionId,
            state: "taken_over",
          };
          records = upsertStep(records, `computer-handoff:${handoffId}`, {
            title: "用户已接管计算机控制",
            description: "你可以直接操作系统；交还后 Agent 继续",
            status: "active",
            tag: "computer-handoff",
          });
          break;
        }
        case "AWAKEN_COMPUTER_HANDOFF_ENDED": {
          const handoffId = action.event.computerSessionId ?? action.event.runId ?? "computer-handoff";
          nextState.computerHandoffs = upsertComputerHandoff(nextState.computerHandoffs, {
            id: handoffId,
            runId: action.event.runId ?? nextState.runId,
            sessionId: action.event.computerSessionId,
            active: false,
          });
          nextState.computerSession = {
            runId: action.event.runId ?? nextState.runId,
            sessionId: action.event.computerSessionId,
            state: "ready",
          };
          records = upsertStep(records, `computer-handoff:${handoffId}`, {
            title: "计算机控制已交还 Agent",
            description: "Agent 恢复执行",
            status: "done",
            tag: "computer-handoff",
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
            nextState.userQuestions = [];
            nextState.externalInteractions = [];
            nextState.computerReviews = [];
            nextState.computerHandoffs = [];
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
            nextState.userQuestions = [];
            nextState.externalInteractions = [];
            nextState.computerReviews = [];
            nextState.computerHandoffs = [];
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
            )?.label || "Untitled thread",
          runId: nextState.runId,
          activeRunId: undefined,
        });
      }

      nextState.steps = toSteps(records);
      const projectedNarrative = projectAguiEventToNarrative(
        state.narrative,
        narrativeEvent,
      );
      const terminalStatus = terminalStatusForEvent(action.event);
      const narrativeWithElapsed = isSuccessfulTerminalEvent(action.event)
        ? attachElapsedMsToLatestAnswer(
            projectedNarrative,
            elapsedMsForTerminalRun(nextState, action.receivedAtMs),
            nextState.runId,
          )
        : projectedNarrative;
      nextState.narrative = terminalStatus
        ? appendFinalDeliverableSummary(
            narrativeWithElapsed,
            nextState.artifacts,
            terminalStatus,
            nextState.runId,
          )
        : narrativeWithElapsed;
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
