import { describe, expect, it } from "vitest";
import { createInitialRuntimeState, reduceRuntimeState } from "./reducer";
import type { RuntimeViewState } from "./reducer";
import type { AguiEvent } from "../../shared/agui";

describe("child activity continuity", () => {
  it("restores the Agent plan immediately when a new assignment appears", () => {
    let state: RuntimeViewState = { ...createInitialRuntimeState(), runId: "run", threadId: "thread", activeThreadId: "thread" };
    for (const event of [
      { type: "AWAKEN_SUBAGENT_CHANGED", subagentTaskId: "old", subagentStatus: "completed" },
      { type: "AWAKEN_PLAN_UPDATED", subagentTaskId: "old", planVersion: 3, planItems: [{ step: "continue from saved progress", activeForm: "reading", status: "in_progress" }] },
      { type: "AWAKEN_SUBAGENT_CHANGED", subagentTaskId: "new", subagentStatus: "running" },
    ] as AguiEvent[]) {
      state = reduceRuntimeState(state, { type: "run/event", event: { ...event, runId: "run", threadId: "thread", subagentAgentId: "child" } });
    }
    expect(state.subagentsById.new.planItems?.[0].step).toBe("continue from saved progress");
    expect(state.subagentsById.new.planVersion).toBe(3);
    expect(state.runPlansById).toEqual({});
  });
  it("closes every assignment of the same Agent and rejects stale reopening", () => {
    let state: RuntimeViewState = { ...createInitialRuntimeState(), runId: "run", threadId: "thread", activeThreadId: "thread" };
    const apply = (taskId: string, agentId: string, closed?: boolean) => {
      state = reduceRuntimeState(state, { type: "run/event", event: { type: "AWAKEN_SUBAGENT_CHANGED", runId: "run", threadId: "thread", subagentTaskId: taskId, subagentAgentId: agentId, subagentStatus: "completed", subagentClosed: closed } });
    };
    apply("old", "child");
    apply("latest", "child");
    apply("sibling", "other");
    apply("latest", "child", true);
    apply("old", "child", false);
    apply("late-replay", "child", false);
    expect(state.subagentsById.old.closed).toBe(true);
    expect(state.subagentsById.latest.closed).toBe(true);
    expect(state.subagentsById["late-replay"].closed).toBe(true);
    expect(state.subagentsById.sibling.closed).toBe(false);
  });
  it("keeps its own plan through progress and stale replay", () => {
    let state: RuntimeViewState = { ...createInitialRuntimeState(), runId: "run", threadId: "thread", activeThreadId: "thread" };
    const apply = (event: AguiEvent) => { state = reduceRuntimeState(state, { type: "run/event", event: { ...event, runId:"run", threadId:"thread", subagentTaskId:"task", subagentAgentId:"child" } }); };
    apply({type:"AWAKEN_SUBAGENT_CHANGED", subagentStatus:"running"});
    apply({type:"AWAKEN_PLAN_UPDATED", planVersion:2, planItems:[{step:"saved step",activeForm:"working",status:"in_progress"}]});
    apply({type:"AWAKEN_SUBAGENT_ACTIVITY", subagentActivity:"waiting", subagentToolUseCount:2});
    apply({type:"AWAKEN_PLAN_UPDATED", planVersion:1, planItems:[{step:"old step",activeForm:"old",status:"pending"}]});
    expect(state.subagentsById.task.planItems?.[0].step).toBe("saved step");
    expect(state.subagentsById.task.planVersion).toBe(2);
    expect(state.runPlansById).toEqual({});
  });
  it("does not revive legacy child questions after the root finishes", () => {
    let state: RuntimeViewState = { ...createInitialRuntimeState(), runId:"run", threadId:"thread", activeThreadId:"thread" };
    state = reduceRuntimeState(state, {type:"run/event", event:{type:"AWAKEN_USER_QUESTION_REQUESTED", threadId:"thread", runId:"run", subagentTaskId:"task", subagentAgentId:"child", questionId:"q", content:JSON.stringify({questions:[{question:"continue?",options:[]}]})}});
    state = reduceRuntimeState(state, {type:"run/event", event:{type:"RUN_FINISHED",runId:"run",threadId:"thread"}});
    expect(state.userQuestions).toHaveLength(0);
  });
  it("keeps child Computer controls across root completion and a new round, then closes only their session", () => {
    let state: RuntimeViewState = { ...createInitialRuntimeState(), runId: "old", threadId: "thread", activeThreadId: "thread" };
    const apply = (event: AguiEvent) => { state = reduceRuntimeState(state, { type: "run/event", event: { threadId: "thread", ...event } }); };
    apply({ type: "AWAKEN_COMPUTER_HANDOFF_STARTED", runId: "old", subagentTaskId: "child", subagentAgentId: "agent", computerSessionId: "child-session" });
    apply({ type: "RUN_FINISHED", runId: "old" });
    expect(state.computerHandoffs[0]).toMatchObject({ runId: "old", taskId: "child", active: true });
    state = reduceRuntimeState(state, { type: "run/requested", runId: "new", prompt: "next round" });
    apply({ type: "RUN_STARTED", runId: "new" });
    expect(state.computerHandoffs[0]).toMatchObject({ runId: "old", taskId: "child", active: true });
    expect(state.computerSession).toMatchObject({ runId: "old", taskId: "child", sessionId: "child-session" });
    apply({ type: "AWAKEN_COMPUTER_SESSION_CHANGED", runId: "new", computerSessionId: "root-session", computerSessionState: "closed" });
    expect(state.computerSession?.sessionId).toBe("child-session");
    apply({ type: "AWAKEN_COMPUTER_SESSION_CHANGED", runId: "old", subagentTaskId: "child", subagentAgentId: "agent", computerSessionId: "child-session", computerSessionState: "closed" });
    expect(state.computerHandoffs).toHaveLength(0);
    expect(state.computerSession).toBeUndefined();
    expect(state.runId).toBe("new");
  });

});
