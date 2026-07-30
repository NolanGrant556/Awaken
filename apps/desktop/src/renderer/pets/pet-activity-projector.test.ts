import { describe, expect, it } from "vitest";
import { projectPetActivity, highestPetStatus } from "./pet-activity-projector";
import type { RuntimeViewState, ThreadItem } from "../runtime/reducer";

function emptyViewState(): RuntimeViewState {
  return {
    prompt: "",
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
    artifacts: [],
    inspectorSummary: { unknownEvents: [] },
    narrative: { committedItems: [], activeItem: undefined, seenEventKeys: [] },
    threads: [],
    threadViews: {},
    supervisorStatus: null,
  } as RuntimeViewState;
}

function threadItem(overrides: Partial<ThreadItem> = {}): ThreadItem {
  return {
    threadId: "thread-1",
    label: "Test thread",
    artifactCount: 0,
    ...overrides,
  };
}

describe("projectPetActivity", () => {
  it("returns idle when no threads", () => {
    const snapshot = projectPetActivity(emptyViewState());
    expect(snapshot.sessions).toHaveLength(0);
    expect(highestPetStatus(snapshot)).toBe("idle");
  });

  it("maps running thread to running", () => {
    const state = emptyViewState();
    state.threads = [threadItem({ runStatus: "running", activeRunId: "run-1" })];
    const snapshot = projectPetActivity(state);
    expect(snapshot.sessions[0]?.status).toBe("running");
  });

  it("uses the active assistant text as the pet bubble subtitle", () => {
    const state = emptyViewState();
    state.threads = [threadItem({ runStatus: "running", activeRunId: "run-1" })];
    state.activeThreadId = "thread-1";
    state.assistantText = "收到，这次我们终于有日志了。";
    const snapshot = projectPetActivity(state);
    expect(snapshot.sessions[0]?.subtitle).toBe("收到，这次我们终于有日志了。");
  });

  it("maps error thread to failed", () => {
    const state = emptyViewState();
    state.threads = [threadItem({ runStatus: "error" })];
    const snapshot = projectPetActivity(state);
    expect(snapshot.sessions[0]?.status).toBe("failed");
  });

  it("maps finished thread to review", () => {
    const state = emptyViewState();
    state.threads = [threadItem({ runStatus: "finished" })];
    const snapshot = projectPetActivity(state);
    expect(snapshot.sessions[0]?.status).toBe("review");
  });

  it("keeps the currently viewed finished thread visible for a short review window", () => {
    const state = emptyViewState();
    state.threads = [threadItem({ threadId: "viewed", runStatus: "finished" })];
    state.activeThreadId = "viewed";
    const snapshot = projectPetActivity(state);
    expect(snapshot.sessions[0]?.status).toBe("review");
  });

  it("prioritizes waiting over running", () => {
    const state = emptyViewState();
    state.threads = [
      threadItem({ threadId: "a", runStatus: "running", activeRunId: "run-1" }),
      threadItem({ threadId: "b", runStatus: "finished" }),
    ];
    state.activeThreadId = "a";
    state.userQuestions = [
      {
        id: "q-waiting",
        questions: [{ question: "Continue?", options: [{ label: "yes" }] }],
      },
    ];
    const snapshot = projectPetActivity(state);
    expect(highestPetStatus(snapshot)).toBe("waiting");
  });

  it("generates question waiting request for active thread", () => {
    const state = emptyViewState();
    state.threads = [threadItem({ threadId: "a", runStatus: "running", activeRunId: "run-1" })];
    state.activeThreadId = "a";
    state.userQuestions = [
      {
        id: "q-1",
        questions: [{ question: "研究深度?", options: [{ label: "standard" }] }],
      },
    ];
    const snapshot = projectPetActivity(state);
    expect(snapshot.sessions[0]?.waitingRequest).toMatchObject({
      type: "question",
      requestId: "q-1",
    });
  });
});
