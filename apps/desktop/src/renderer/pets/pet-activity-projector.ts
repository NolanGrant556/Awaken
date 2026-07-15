import type { RuntimeViewState, ThreadItem } from "../runtime/reducer";
import type {
  PetActivitySnapshot,
  PetSessionActivity,
  PetSessionStatus,
  PetWaitingRequest,
} from "../../shared/pets";

type WaitingStateLike = Pick<
  RuntimeViewState,
  "approvals" | "userQuestions"
>;

function isWaiting(state: WaitingStateLike): boolean {
  return (
    state.approvals.length > 0 || state.userQuestions.length > 0
  );
}

function statusForThread(
  thread: ThreadItem,
  view: WaitingStateLike | undefined,
  isActive: boolean,
): PetSessionStatus {
  if ((view && isWaiting(view)) || (thread.approvalCount ?? 0) > 0) return "waiting";
  if (!thread.runStatus) return "idle";
  switch (thread.runStatus) {
    case "running":
    case "cancelling":
      return "running";
    case "error":
    case "cancelled":
      return "failed";
    case "finished":
      return "review";
    default:
      return "idle";
  }
}

type ActiveViewLike = Pick<
  RuntimeViewState,
  | "approvals"
  | "userQuestions"
  | "steps"
  | "assistantText"
  | "inspectorSummary"
  | "runId"
  | "runStartedAtMs"
  | "runCompletedAtMs"
>;

function activeView(state: RuntimeViewState): ActiveViewLike {
  return (
    (state.activeThreadId ? state.threadViews[state.activeThreadId] : undefined) ?? state
  );
}

function waitingRequestForActiveThread(
  state: RuntimeViewState,
): PetWaitingRequest | null {
  const view = activeView(state);

  const userQuestion = view.userQuestions[0];
  if (userQuestion) {
    return {
      type: "question",
      requestId: userQuestion.id,
      prompt: userQuestion.questions.map((q) => q.question).join("\n"),
      options: userQuestion.questions[0]?.options.map((o) => ({
        id: o.label,
        label: o.label,
        description: o.description,
      })) ?? [],
    };
  }

  const approval = view.approvals[0];
  if (approval) {
    return {
      type: "command-approval",
      requestId: approval.id,
      summary: approval.title || "需要审批",
    };
  }

  return null;
}

function subtitleForActiveThread(state: RuntimeViewState): string | undefined {
  const view = activeView(state);

  const assistantPreview = compactAssistantPreview(view.assistantText);
  if (assistantPreview) return assistantPreview;

  const activeStep = view.steps.find((s) => s.status === "active" || s.status === "pending");
  if (activeStep?.title) return activeStep.title;

  const lastTool = view.inspectorSummary.lastToolDecision;
  if (lastTool?.toolName) return lastTool.toolName;

  if (view.approvals.length > 0) return "等待审批";
  if (view.userQuestions.length > 0) return "等待回答";

  return "Thinking";
}

function compactAssistantPreview(text: string | undefined): string | undefined {
  const preview = (text ?? "").replace(/\s+/g, " ").trim();
  if (!preview) return undefined;
  return preview.length > 96 ? `${preview.slice(0, 95)}...` : preview;
}

export function projectPetActivity(
  state: RuntimeViewState,
): PetActivitySnapshot {
  const sessions: PetSessionActivity[] = [];
  const now = Date.now();

  for (const thread of state.threads) {
    const isActive = state.activeThreadId === thread.threadId;
    const view = isActive ? activeView(state) : state.threadViews[thread.threadId];
    const status = statusForThread(thread, view, isActive);
    const updatedAtMs =
      status === "review" || status === "failed"
        ? view?.runCompletedAtMs ?? view?.runStartedAtMs ?? now
        : now;

    sessions.push({
      key: thread.threadId,
      status,
      runId: thread.activeRunId ?? thread.runId,
      threadId: thread.threadId,
      turnKey: view?.runId ?? thread.runId ?? null,
      title: thread.label || "Untitled thread",
      subtitle: isActive ? subtitleForActiveThread(state) : undefined,
      updatedAtMs,
      waitingRequest:
        isActive && view && isWaiting(view) ? waitingRequestForActiveThread(state) : null,
    });
  }

  return {
    sessions,
    emittedAtMs: now,
  };
}

export function highestPetStatus(snapshot: PetActivitySnapshot): PetSessionStatus {
  const priority: PetSessionStatus[] = ["waiting", "failed", "review", "running", "idle"];
  for (const status of priority) {
    if (snapshot.sessions.some((s) => s.status === status)) {
      return status;
    }
  }
  return "idle";
}
