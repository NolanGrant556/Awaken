import type { ApprovalItem, RunStepItem } from "@awaken/ui";
import type { AguiApprovalRiskTone, AguiArtifactStatus, AguiEvent } from "./agui";
import type { RuntimeRun, RuntimeRunStatus } from "../../shared/runtime";

type RuntimePhase = "idle" | "starting" | "running" | "cancelling" | "done" | "error" | "cancelled";

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
};

export type RuntimeAction =
  | { type: "run/reset" }
  | { type: "run/requested"; prompt: string }
  | { type: "run/created"; run: RuntimeRun }
  | { type: "run/createFailed"; message: string }
  | { type: "run/cancelRequested" }
  | { type: "run/cancelSync"; run: RuntimeRun }
  | { type: "run/streamError"; message: string }
  | { type: "run/event"; event: AguiEvent };

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
};

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
      return initialState;
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
      };
    case "run/created":
      return syncFromRun(state, action.run);
    case "run/createFailed":
      return {
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
      };
    case "run/cancelRequested":
      return {
        ...state,
        phase: "cancelling",
        status: "cancelling",
        timelineStatus: "running",
        approvals: [],
      };
    case "run/cancelSync":
      return syncFromRun(state, action.run);
    case "run/streamError":
      return {
        ...state,
        phase: "error",
        timelineStatus: "error",
        errorMessage: action.message,
        approvals: [],
      };
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
            nextState.errorMessage = action.event.message || "Run failed";
            nextState.approvals = [];
            records = upsertStep(records, "run-error", {
              title: "Run failed",
              description: action.event.message || "Runtime returned an error",
              status: "error",
              tag: action.event.code || "error",
            });
          }
          break;
        default:
          break;
      }

      nextState.steps = toSteps(records);
      return nextState;
    }
    default:
      return state;
  }
}
