import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AguiEvent } from "../../shared/agui";
import { createRuntimeClient } from "./client";

type MockEventSourceInstance = {
  url: string;
  onmessage: ((event: { data: string }) => void) | null;
  onerror: (() => void) | null;
  close: () => void;
};

let mockInstances: MockEventSourceInstance[] = [];

class MockEventSource {
  url: string;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    mockInstances.push(this);
  }

  close(): void {
    this.closed = true;
  }
}

beforeEach(() => {
  vi.useFakeTimers();
  mockInstances = [];
  vi.stubGlobal("EventSource", MockEventSource);
  // Stub a minimal window so createRuntimeClient() falls through to the
  // browser client (window.awaken is undefined).
  vi.stubGlobal("window", { location: { origin: "http://localhost" } });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function emitEvent(source: MockEventSourceInstance, event: AguiEvent): void {
  source.onmessage?.({ data: JSON.stringify(event) });
}

describe("browser client reconnect", () => {
  it("non-terminal close triggers reconnecting and resubscribes with after_seq", async () => {
    const client = createRuntimeClient();
    const onReconnect = vi.fn();
    const onEvent = vi.fn();

    const unsubscribe = await client.subscribeRun("run-1", {
      onEvent,
      onReconnect,
    });

    // First EventSource created.
    expect(mockInstances).toHaveLength(1);
    const firstSource = mockInstances[0];

    // Deliver an event with seq to set lastSeq.
    emitEvent(firstSource, { type: "TEXT_MESSAGE_CONTENT", seq: 16, delta: "hi" });
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({ seq: 16 }));

    // Simulate connection error.
    firstSource.onerror?.();

    // Should dispatch reconnecting.
    expect(onReconnect).toHaveBeenCalledTimes(1);

    // Advance timer to trigger reconnect.
    await vi.advanceTimersByTimeAsync(1000);

    // New EventSource should be created with after_seq=16.
    expect(mockInstances).toHaveLength(2);
    expect(mockInstances[1].url).toContain("after_seq=16");

    unsubscribe();
  });

  it("max retries delivers readable error message", async () => {
    const client = createRuntimeClient();
    const onError = vi.fn();
    const onReconnect = vi.fn();

    await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onError,
      onReconnect,
    });

    // Simulate consecutive errors with timer advances. Each iteration triggers
    // onerror on the current source, which schedules a reconnect timer; advancing
    // the timer creates a new source. After MAX_RECONNECT_RETRIES (5) reconnects,
    // the next onerror hits the retry limit and calls onError.
    for (let i = 0; i < 6; i++) {
      const source = mockInstances[mockInstances.length - 1];
      source.onerror?.();
      await vi.advanceTimersByTimeAsync(10000);
    }

    // After MAX_RECONNECT_RETRIES (5), onError should fire with a readable message.
    expect(onError).toHaveBeenCalledWith(
      expect.stringContaining("failed after 5 reconnect attempts"),
    );
  });

  it("terminal event does not trigger reconnect", async () => {
    const client = createRuntimeClient();
    const onReconnect = vi.fn();
    const onClose = vi.fn();

    await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onReconnect,
      onClose,
    });

    const source = mockInstances[0];
    // Deliver terminal event.
    emitEvent(source, { type: "RUN_FINISHED", seq: 16 });

    // onClose should fire (terminal close).
    expect(onClose).toHaveBeenCalledTimes(1);

    // Simulate error after terminal — should NOT reconnect.
    source.onerror?.();
    expect(onReconnect).not.toHaveBeenCalled();
  });

  it("dispose prevents further reconnect", async () => {
    const client = createRuntimeClient();
    const onReconnect = vi.fn();

    const unsubscribe = await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onReconnect,
    });

    const source = mockInstances[0];
    unsubscribe();

    // Error after dispose — should NOT reconnect.
    source.onerror?.();
    expect(onReconnect).not.toHaveBeenCalled();
  });
});
