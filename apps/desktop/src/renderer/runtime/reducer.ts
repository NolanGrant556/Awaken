import type { ApprovalItem, RunStepItem } from "@awaken/ui";
import type { AguiApprovalRiskTone, AguiArtifactStatus, AguiEvent } from "./agui";
import type {
  RuntimeRun,
  RuntimeRunStatus,
  RuntimeThread,
  SupervisorStatus,
  SupervisorStatusInfo,
} from "../../shared/runtime";

type RuntimePhase = "idle" | "starting" | "running" | "reconnecting" | "cancelling" | "done" | "error" | "cancelled";

type TimelineStatus = "ready" | "running" | "done" | "error";

type StepRecord = RunStepItem & {
  key: string;
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

export type ThreadItem = {
  threadId: string;
  label: string;
  runId?: string;
  activeRunId?: string;
};

type ThreadViewState = {
  prompt: string;
  runId?: string;
  threadId?: string;
  phase: RuntimePhase;
  status?: RuntimeRunStatus;
  timelineTitle: string;
  timelineStatus: TimelineStatus;
  steps: RunStepItem[];
  assistantText: string;
  reasoningText: string;
  toolResults: Array<{ id: string; title: string; content: string }>;
  approvals: ApprovalItem[];
  artifacts: ArtifactRecord[];
  errorMessage?: string;
};

export type RuntimeViewState = {
  prompt: string;
  runId?: string;
  threadId?: string;
  phase: RuntimePhase;
  status?: RuntimeRunStatus;
  timelineTitle: string;
  timelineStatus: TimelineStatus;
  steps: RunStepItem[];
  assistantText: string;
  reasoningText: string;
  toolResults: Array<{ id: string; title: string; content: string }>;
  approvals: ApprovalItem[];
  artifacts: ArtifactRecord[];
  errorMessage?: string;
  threads: ThreadItem[];
  activeThreadId?: string;
  threadViews: Record<string, ThreadViewState>;
  supervisorStatus: SupervisorStatus | null;
};

export type RuntimeAction =
  | { type: "run/reset" }
  | { type: "run/requested"; prompt: string }
  | { type: "run/created"; run: RuntimeRun }
  | { type: "run/createFailed"; message: string }
  | { type: "run/cancelRequested" }
  | { type: "run/cancelSync"; run: RuntimeRun }
  | { type: "run/streamError"; message: string }
  | { type: "run/reconnecting" }
  | { type: "run/recovered"; run: RuntimeRun }
  | { type: "run/threadsLoaded"; threads: RuntimeThread[] }
  | { type: "run/threadAdded"; threadId: string; label: string; runId: string }
  | { type: "run/threadSwitch"; threadId: string }
  | { type: "run/event"; event: AguiEvent }
  | { type: "supervisor/status-changed"; info: SupervisorStatusInfo };

const initialState: RuntimeViewState = {
  prompt: "",
  phase: "idle",
  timelineTitle: "等待运行",
  timelineStatus: "ready",
  steps: [],
  assistantText: "",
  reasoningText: "",
  toolResults: [],
  approvals: [],
  artifacts: [],
  threads: [],
  threadViews: {},
  supervisorStatus: null,
};

function viewFromState(state: RuntimeViewState): ThreadViewState {
  return {
    prompt: state.prompt,
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
    approvals: state.approvals,
    artifacts: state.artifacts,
    errorMessage: state.errorMessage,
  };
}

function emptyThreadView(threadId: string, runId?: string): ThreadViewState {
  return {
    ...viewFromState(initialState),
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

function restoreThreadView(state: RuntimeViewState, threadId: string): RuntimeViewState {
  const thread = state.threads.find((item) => item.threadId === threadId);
  const view = state.threadViews[threadId] ?? emptyThreadView(threadId, thread?.runId);
  return {
    ...state,
    ...view,
    activeThreadId: threadId,
  };
}

function upsertThread(threads: ThreadItem[], next: ThreadItem): ThreadItem[] {
  const index = threads.findIndex((thread) => thread.threadId === next.threadId);
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

function upsertApproval(approvals: ApprovalItem[], next: ApprovalItem): ApprovalItem[] {
  const index = approvals.findIndex((approval) => approval.id === next.id);
  if (index < 0) {
    return [...approvals, next];
  }

  const updated = approvals.slice();
  updated[index] = next;
  return updated;
}

function removeApproval(approvals: ApprovalItem[], approvalId?: string): ApprovalItem[] {
  if (!approvalId) {
    return approvals;
  }
  return approvals.filter((approval) => approval.id !== approvalId);
}

function upsertArtifact(artifacts: ArtifactRecord[], event: AguiEvent): ArtifactRecord[] {
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

function syncFromRun(state: RuntimeViewState, run: RuntimeRun): RuntimeViewState {
  if (run.status === "cancelling") {
    return {
      ...state,
      runId: run.runId,
      threadId: run.threadId,
      phase: "cancelling",
      status: run.status,
      timelineStatus: "running",
      approvals: [],
      steps: toSteps(
        upsertStep(
          state.steps.map((step, index) => ({ ...step, key: step.id, index: index + 1 })),
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
        supervisorStatus: state.supervisorStatus,
        threads: state.threads,
        threadViews: saveActiveThreadView(state).threadViews,
      };
    case "run/requested":
      return {
        prompt: action.prompt,
        phase: "starting",
        timelineTitle: excerpt(action.prompt, 72) || "New run",
        timelineStatus: "running",
        steps: [],
        assistantText: "",
        reasoningText: "",
        toolResults: [],
        approvals: [],
        artifacts: [],
        supervisorStatus: state.supervisorStatus,
        threads: state.threads,
        activeThreadId: state.activeThreadId,
        threadViews: state.threadViews,
      };
    case "run/created":
      return saveActiveThreadView({
        ...syncFromRun(state, action.run),
        activeThreadId: action.run.threadId,
      });
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
      return saveActiveThreadView({
        ...state,
        phase: "cancelling",
        status: "cancelling",
        timelineStatus: "running",
        approvals: [],
      });
    case "run/cancelSync":
      return saveActiveThreadView(syncFromRun(state, action.run));
    case "run/streamError":
      return saveActiveThreadView({
        ...state,
        phase: "error",
        timelineStatus: "error",
        errorMessage: action.message,
        approvals: [],
      });
    case "run/reconnecting":
      return saveActiveThreadView({
        ...state,
        phase: "reconnecting",
        timelineStatus: "running",
      });
    case "run/recovered":
      return saveActiveThreadView({
        ...syncFromRun(initialState, action.run),
        supervisorStatus: state.supervisorStatus,
        timelineTitle: "恢复运行",
        threads: upsertThread(state.threads, {
          threadId: action.run.threadId,
          label:
            state.threads.find((thread) => thread.threadId === action.run.threadId)?.label ||
            "Recovered thread",
          runId: action.run.runId,
          activeRunId:
            action.run.status === "running" || action.run.status === "awaiting_approval"
              ? action.run.runId
              : undefined,
        }),
        threadViews: state.threadViews,
        activeThreadId: action.run.threadId,
      });
    case "run/threadsLoaded": {
      const threadViews = { ...state.threadViews };
      const loaded = action.threads.map(threadItemFromRuntime);
      for (const thread of loaded) {
        if (!threadViews[thread.threadId]) {
          threadViews[thread.threadId] = emptyThreadView(thread.threadId, thread.runId);
        }
      }
      const activeThreadStillExists =
        !state.activeThreadId || loaded.some((thread) => thread.threadId === state.activeThreadId);
      if (!activeThreadStillExists) {
        return {
          ...initialState,
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
        }),
      };
      return saveActiveThreadView(next);
    }
    case "run/threadSwitch":
      return restoreThreadView(saveActiveThreadView(state), action.threadId);
    case "run/event": {
      const nextState = { ...state };
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
          nextState.activeThreadId = nextState.activeThreadId ?? action.event.threadId;
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
            description: action.event.runId ? `run ${action.event.runId}` : "Awaiting streamed events",
            status: "done",
            tag: "runtime",
          });
          break;
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
          }
          records = upsertStep(records, status === "cancelled" ? "run-cancelled" : `status:${status ?? "unknown"}`, {
            title: status === "awaiting_approval"
              ? "Awaiting approval"
              : status === "cancelling"
                ? "Cancelling run"
                : status === "cancelled"
                  ? "Run cancelled"
                  : "Run status changed",
            description: `Status: ${status ?? "unknown"}`,
            status: "done",
            tag: "runtime",
          });
          break;
        }
        case "RUN_FINISHED":
          nextState.phase = "done";
          nextState.timelineStatus = "done";
          nextState.approvals = [];
          records = upsertStep(records, "run-finished", {
            title: "Run finished",
            description: excerpt(action.event.result || nextState.assistantText || "Runtime completed"),
            status: "done",
            tag: action.event.outcome?.type || "success",
          });
          break;
        case "TEXT_MESSAGE_START":
          records = upsertStep(records, `assistant:${action.event.messageId ?? "message"}`, {
            title: "Assistant response",
            description: "Streaming text output",
            status: "active",
            tag: action.event.role || "assistant",
          });
          break;
        case "TEXT_MESSAGE_CONTENT": {
          const delta = action.event.delta ?? "";
          nextState.assistantText += delta;
          records = upsertStep(records, `assistant:${action.event.messageId ?? "message"}`, {
            title: "Assistant response",
            description: excerpt(nextState.assistantText) || "Streaming text output",
            status: "active",
            tag: action.event.role || "assistant",
          });
          break;
        }
        case "TEXT_MESSAGE_END":
          records = upsertStep(records, `assistant:${action.event.messageId ?? "message"}`, {
            title: "Assistant response",
            description: excerpt(nextState.assistantText) || "Completed text output",
            status: "done",
            tag: action.event.role || "assistant",
          });
          break;
        case "REASONING_START":
          records = upsertStep(records, `reasoning:${action.event.messageId ?? "reasoning"}`, {
            title: "Reasoning",
            description: "Streaming internal reasoning summary",
            status: "active",
            tag: "thinking",
          });
          break;
        case "REASONING_MESSAGE_CONTENT": {
          const delta = action.event.delta ?? "";
          nextState.reasoningText += delta;
          records = upsertStep(records, `reasoning:${action.event.messageId ?? "reasoning"}`, {
            title: "Reasoning",
            description: excerpt(nextState.reasoningText) || "Streaming internal reasoning summary",
            status: "active",
            tag: "thinking",
          });
          break;
        }
        case "REASONING_END":
          records = upsertStep(records, `reasoning:${action.event.messageId ?? "reasoning"}`, {
            title: "Reasoning",
            description: excerpt(nextState.reasoningText) || "Reasoning complete",
            status: "done",
            tag: "thinking",
          });
          break;
        case "TOOL_CALL_START":
          records = upsertStep(records, `tool:${action.event.toolCallId ?? "tool"}`, {
            title: action.event.toolCallName || "Tool call",
            description: "Tool invocation started",
            status: "active",
            tag: "tool",
          });
          break;
        case "TOOL_CALL_ARGS":
          records = upsertStep(records, `tool:${action.event.toolCallId ?? "tool"}`, {
            title: action.event.toolCallName || "Tool call",
            description: excerpt(action.event.delta || "Streaming tool arguments"),
            status: "active",
            tag: "tool",
          });
          break;
        case "TOOL_CALL_RESULT": {
          const content = action.event.content || "Tool returned empty content";
          const id = action.event.toolCallId || `tool-result-${state.toolResults.length + 1}`;
          const existingIndex = nextState.toolResults.findIndex((tool) => tool.id === id);
          const nextToolResults = nextState.toolResults.slice();
          const record = {
            id,
            title: action.event.toolCallName || "Tool result",
            content,
          };
          if (existingIndex >= 0) {
            nextToolResults[existingIndex] = record;
          } else {
            nextToolResults.push(record);
          }
          nextState.toolResults = nextToolResults;

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
          records = upsertStep(records, `tool:${action.event.toolCallId ?? "tool"}`, {
            title: action.event.toolCallName || "Tool call",
            description: "Tool call complete",
            status: "done",
            tag: "tool",
          });
          break;
        case "AWAKEN_APPROVAL_REQUESTED": {
          const approvalId = action.event.approvalId ?? `approval-${nextState.approvals.length + 1}`;
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
            description: approval.description || "Waiting for approval decision",
            status: "active",
            tag: "approval",
          });
          break;
        }
        case "AWAKEN_APPROVAL_RESOLVED": {
          const approvalId = action.event.approvalId ?? "approval";
          nextState.approvals = removeApproval(nextState.approvals, action.event.approvalId);
          const decision = action.event.approvalDecision || "resolved";
          records = upsertStep(records, `approval:${approvalId}`, {
            title: action.event.approvalTitle || "Approval resolved",
            description:
              action.event.approvalDescription ||
              `Decision: ${decision}`,
            status: decision === "approved" ? "done" : "error",
            tag: decision,
          });
          break;
        }
        case "AWAKEN_ARTIFACT_CREATED":
        case "AWAKEN_ARTIFACT_UPDATED": {
          nextState.artifacts = upsertArtifact(nextState.artifacts, action.event);
          const artifact =
            nextState.artifacts.find((candidate) => candidate.id === action.event.artifactId) ??
            nextState.artifacts[nextState.artifacts.length - 1];
          records = upsertStep(records, `artifact:${artifact.id}`, {
            title: artifact.title,
            description: excerpt(buildArtifactDescription(artifact)) || "Artifact updated",
            status: "done",
            tag: action.event.type === "AWAKEN_ARTIFACT_CREATED" ? "artifact:new" : "artifact:update",
          });
          break;
        }
        case "RUN_ERROR":
          if (action.event.code === "RUN_CANCELLED") {
            nextState.phase = "cancelled";
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
            nextState.timelineStatus = "error";
            const rawMessage = action.event.message || "Runtime returned an error";
            let errorMessage = rawMessage;
            let errorDescription = rawMessage;
            if (action.event.code === "PROVIDER_AUTH_MISSING") {
              errorMessage = "API key 缺失，请在 .env 中配置 LLM_API_KEY";
              errorDescription = "API key 缺失，请在 .env 中配置 LLM_API_KEY";
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
          break;
        default:
          break;
      }

      if (
        nextState.threadId &&
        (action.event.type === "RUN_FINISHED" ||
          action.event.type === "RUN_ERROR" ||
          (action.event.type === "RUN_STATUS_CHANGED" && action.event.status === "cancelled"))
      ) {
        nextState.threads = upsertThread(nextState.threads, {
          threadId: nextState.threadId,
          label:
            state.threads.find((thread) => thread.threadId === nextState.threadId)?.label ||
            state.prompt,
          runId: nextState.runId,
          activeRunId: undefined,
        });
      }

      nextState.steps = toSteps(records);
      return saveActiveThreadView(nextState);
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
