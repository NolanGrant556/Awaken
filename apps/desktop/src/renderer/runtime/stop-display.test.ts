import { describe, expect, it } from "vitest";
import { stopMessage, terminalMessage } from "./stop-display";
import { projectAguiEventToNarrative, createInitialRunNarrativeState } from "../run-narrative/projector";

describe("host stop evidence", () => {
 it.each([
  ["user", "你停止了本次运行"], ["parent", "主任务已停止此子任务"],
  ["shutdown", "应用关闭，运行已中断"], ["timeout", "本次运行已超时"],
  ["unknown", "本次运行已中断"], [undefined, "本次运行已中断"],
 ] as const)("projects %s consistently", (source, text) => {
  expect(stopMessage(source)).toBe(text);
  for (const event of [{type:"RUN_ERROR",code:"RUN_CANCELLED"},{type:"RUN_STATUS_CHANGED",status:"cancelled"}] as const) {
    const state = projectAguiEventToNarrative(createInitialRunNarrativeState(), {...event, stopSource: source});
    expect(state.committedItems).toContainEqual(expect.objectContaining({kind:"hint",text}));
  }
 });
 it("distinguishes exhausted provider stream from Desktop SSE failures", () => {
  expect(terminalMessage({type:"RUN_ERROR",terminalReason:"model_stream_disconnected"})).toBe("模型连接中断，本次运行未能继续");
  expect(terminalMessage({type:"RUN_ERROR",terminalReason:"stream_failed"})).toBeUndefined();
 });
});
