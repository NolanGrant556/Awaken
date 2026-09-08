import { describe, expect, it, vi } from "vitest";
import type { RuntimeThreadTranscript } from "../../shared/runtime";
import type { SubscribeHandlers } from "./client";
import { ThreadSession } from "./thread-session";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function transcript(
  id: string,
  status: "running" | "finished" = "running",
): RuntimeThreadTranscript {
  return {
    thread: {
      threadId: id,
      label: id,
      runId: `run-${id}`,
      archived: false,
      createdAt: "",
      updatedAt: "",
      workspace: { workspaceRoot: "/tmp/test", writableRoots: [] },
    },
    runs: [
      {
        run: {
          threadId: id,
          runId: `run-${id}`,
          status,
          cancelRequested: false,
          createdAt: "2026-09-08T00:00:00Z",
          updatedAt: "2026-09-08T00:00:01Z",
        },
        streamCursor: 7,
        events: [],
      },
    ],
  };
}

function fixture() {
  const values = new Map<string, string>();
  const storage = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
  const client = {
    getThreadTranscript: vi.fn(async (id: string) => transcript(id)),
    subscribeRun: vi.fn(
      async (
        _id: string,
        _handlers: SubscribeHandlers,
        _cursor?: number,
      ): Promise<() => void> => vi.fn<() => void>(),
    ),
  };
  const dispatch = vi.fn(),
    onTranscript = vi.fn();
  return {
    session: new ThreadSession({ client, dispatch, storage, onTranscript }),
    client,
    dispatch,
    storage,
    onTranscript,
  };
}

describe("selected thread session", () => {
  it("ignores an old transcript resolving after another selection", async () => {
    const f = fixture(),
      pending = deferred<RuntimeThreadTranscript>();
    f.client.getThreadTranscript.mockImplementationOnce(() => pending.promise);
    const a = f.session.replay("A");
    await f.session.replay("B");
    const calls = f.dispatch.mock.calls.length;
    pending.resolve(transcript("A"));
    await a;
    expect(f.dispatch).toHaveBeenCalledTimes(calls);
    expect(f.storage.getItem("awaken:runId")).toBe("run-B");
    expect(f.client.subscribeRun).toHaveBeenCalledTimes(1);
  });

  it("fences every callback and cleans up a pending subscription after switching", async () => {
    const f = fixture(),
      pending = deferred<() => void>();
    f.client.subscribeRun.mockImplementationOnce(() => pending.promise);
    const a = f.session.replay("A");
    await Promise.resolve();
    const handlers = f.client.subscribeRun.mock.calls[0][1];
    await f.session.replay("B");
    const calls = f.dispatch.mock.calls.length;
    handlers.onEvent({ type: "RUN_FINISHED", threadId: "A", runId: "run-A" });
    handlers.onReconnect?.();
    handlers.onError?.("late error");
    handlers.onClose?.();
    expect(f.dispatch).toHaveBeenCalledTimes(calls);
    expect(f.storage.getItem("awaken:runId")).toBe("run-B");
    const stop = vi.fn();
    pending.resolve(stop);
    await a;
    expect(stop).toHaveBeenCalledOnce();
    const stopB = await f.client.subscribeRun.mock.results[1].value;
    f.session.begin();
    expect(stopB).toHaveBeenCalledOnce();
  });

  it("does not reopen a stream for a terminal transcript", async () => {
    const f = fixture();
    f.client.getThreadTranscript.mockResolvedValue(transcript("A", "finished"));
    await f.session.replay("A");
    expect(f.client.subscribeRun).not.toHaveBeenCalled();
    expect(f.dispatch.mock.calls[0][0].actions.at(-1).type).toBe(
      "run/snapshotSynced",
    );
  });

  it("reconciles a missing stream once without inventing user cancellation", async () => {
    const f = fixture();
    await f.session.replay("A");
    f.client.subscribeRun.mock.calls[0][1].onError?.(
      "Run session was not found on the server",
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(f.client.getThreadTranscript).toHaveBeenCalledTimes(2);
    expect(f.client.subscribeRun).toHaveBeenCalledTimes(1);
    expect(f.dispatch.mock.calls.at(-1)?.[0].type).toBe("run/streamError");
  });
});
