import { afterEach, describe, expect, it, vi } from "vitest";
import type { AwakenBridge } from "../../shared/runtime";
import { threadActivitySubscriber } from "./activity-client";

type ActivityHandlers = Parameters<NonNullable<AwakenBridge["subscribeThreadActivity"]>>[1];

afterEach(() => vi.useRealTimers());

describe("thread activity subscription recovery", () => {
  it("discards a subscription that closes before its ID arrives", async () => {
    vi.useFakeTimers();
    const listeners: ActivityHandlers[] = [];
    const unsubscribeRun = vi.fn();
    const bridge = {
      unsubscribeRun,
      subscribeThreadActivity: vi.fn(async (_threadId: string, handlers: ActivityHandlers) => {
        listeners.push(handlers);
        if (listeners.length === 1) handlers.onClose?.();
        return { ok: true, value: `subscription-${listeners.length}` };
      }),
    } as unknown as AwakenBridge;
    const onEvent = vi.fn();
    const stop = await threadActivitySubscriber(bridge)!("thread", { onEvent });
    expect(unsubscribeRun).toHaveBeenCalledWith("subscription-1");
    await vi.advanceTimersByTimeAsync(1000);
    listeners[0].onEvent({ type: "AWAKEN_SUBAGENT_CHANGED", streamCursor: 10 });
    listeners[1].onEvent({ type: "AWAKEN_SUBAGENT_CHANGED", streamCursor: 20 });
    expect(onEvent).toHaveBeenCalledTimes(1);
    expect(onEvent.mock.calls[0][0].streamCursor).toBe(20);
    stop();
    listeners[1].onClose?.();
    await vi.advanceTimersByTimeAsync(20000);
    expect(bridge.subscribeThreadActivity).toHaveBeenCalledTimes(2);
    expect(unsubscribeRun).toHaveBeenCalledWith("subscription-2");
  });

  it("requests canonical replay when its cursor expires", async () => {
    vi.useFakeTimers();
    const bridge = {
      unsubscribeRun: vi.fn(),
      subscribeThreadActivity: vi.fn(async () => ({ ok: false, issue: { code: "ACTIVITY_CURSOR_EXPIRED", message: "reload history" } })),
    } as unknown as AwakenBridge;
    const onError = vi.fn();
    const stop = await threadActivitySubscriber(bridge)!("thread", { onEvent: vi.fn(), onError });
    await vi.advanceTimersByTimeAsync(20000);
    expect(onError).toHaveBeenCalledWith("ACTIVITY_CURSOR_EXPIRED");
    expect(bridge.subscribeThreadActivity).toHaveBeenCalledTimes(1);
    stop();
  });
});
