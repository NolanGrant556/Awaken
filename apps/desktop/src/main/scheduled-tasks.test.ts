import { describe, expect, it, vi } from "vitest";
import type { ScheduledTaskEvent } from "../shared/runtime.js";
import {
  parseScheduledTaskEventFrame,
  scheduledTaskNotificationContent,
  startScheduledTaskEventStream,
} from "./scheduled-tasks.js";

function streamFrom(text: string): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(text));
      controller.close();
    },
  });
}

function pendingStream(): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({ start: () => {} });
}

function okResponse(body: ReadableStream<Uint8Array>): Response {
  return { ok: true, status: 200, body } as unknown as Response;
}

const instantSleep = () => Promise.resolve();

describe("parseScheduledTaskEventFrame", () => {
  it("parses the data payload of an SSE frame", () => {
    const wire = parseScheduledTaskEventFrame(
      'event: scheduled-task\ndata: {"type":"notice","task_id":"sched_1","content":"喝水"}\n',
    );
    expect(wire).toEqual({ type: "notice", task_id: "sched_1", content: "喝水" });
  });

  it("returns null for comment/heartbeat frames and broken JSON", () => {
    expect(parseScheduledTaskEventFrame(": keep-alive")).toBeNull();
    expect(parseScheduledTaskEventFrame("event: scheduled-task")).toBeNull();
    expect(parseScheduledTaskEventFrame("data: {broken")).toBeNull();
  });
});

describe("scheduledTaskNotificationContent", () => {
  it("turns a live notice with content into a notification", () => {
    expect(
      scheduledTaskNotificationContent({
        type: "notice",
        taskId: "sched_1",
        taskName: "喝水提醒",
        content: "该喝水了",
      }),
    ).toEqual({ title: "喝水提醒", body: "该喝水了" });
  });

  it("falls back to a generic title when the task name is missing", () => {
    expect(
      scheduledTaskNotificationContent({ type: "notice", taskId: "sched_1", content: "正文" }),
    ).toEqual({ title: "定时任务", body: "正文" });
  });

  it("never notifies for non-notice events or empty bodies", () => {
    expect(
      scheduledTaskNotificationContent({ type: "task_changed", taskId: "sched_1" }),
    ).toBeNull();
    expect(
      scheduledTaskNotificationContent({ type: "execution_finished", taskId: "sched_1" }),
    ).toBeNull();
    expect(
      scheduledTaskNotificationContent({ type: "notice", taskId: "sched_1", content: "  " }),
    ).toBeNull();
    expect(
      scheduledTaskNotificationContent({ type: "notice", taskId: "sched_1" }),
    ).toBeNull();
  });
});

describe("startScheduledTaskEventStream", () => {
  it("pumps frames to normalized events and reconnects after the stream closes", async () => {
    const events: ScheduledTaskEvent[] = [];
    const errors: string[] = [];
    const fetchFn = vi
      .fn()
      .mockResolvedValueOnce(
        okResponse(
          streamFrom(
            'event: scheduled-task\ndata: {"type":"execution_started","task_id":"sched_1","project_id":"project_1","execution_id":"exec_1","run_id":"run_1","thread_id":"thread_1","status":"running"}\n\n',
          ),
        ),
      )
      .mockResolvedValueOnce(okResponse(pendingStream()));

    const stream = startScheduledTaskEventStream({
      baseUrl: "http://127.0.0.1:9999",
      fetchFn: fetchFn as unknown as typeof fetch,
      sleep: instantSleep,
      onEvent: (event) => events.push(event),
      onError: (message) => errors.push(message),
    });

    await vi.waitFor(() => {
      expect(fetchFn).toHaveBeenCalledTimes(2);
    });
    expect(events).toEqual([
      {
        type: "execution_started",
        taskId: "sched_1",
        projectId: "project_1",
        executionId: "exec_1",
        runId: "run_1",
        threadId: "thread_1",
        taskName: undefined,
        status: "running",
        content: undefined,
      },
    ]);
    expect(errors).toEqual(["scheduled task event stream closed"]);
    expect(fetchFn.mock.calls[0][0]).toBe("http://127.0.0.1:9999/v1/scheduled-tasks/events");

    stream.stop();
  });

  it("backs off and retries when the connection fails", async () => {
    const sleeps: number[] = [];
    const fetchFn = vi
      .fn()
      .mockRejectedValueOnce(new Error("connection refused"))
      .mockResolvedValueOnce(okResponse(pendingStream()));
    const errors: string[] = [];

    const stream = startScheduledTaskEventStream({
      baseUrl: "http://127.0.0.1:9999/",
      fetchFn: fetchFn as unknown as typeof fetch,
      sleep: (ms) => {
        sleeps.push(ms);
        return Promise.resolve();
      },
      initialReconnectDelayMs: 1000,
      onEvent: () => {},
      onError: (message) => errors.push(message),
    });

    await vi.waitFor(() => {
      expect(fetchFn).toHaveBeenCalledTimes(2);
    });
    expect(errors).toEqual(["connection refused"]);
    expect(sleeps).toEqual([1000]);

    stream.stop();
  });

  it("treats a non-OK response as a retryable failure", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 503, body: null } as unknown as Response)
      .mockResolvedValueOnce(okResponse(pendingStream()));
    const errors: string[] = [];

    const stream = startScheduledTaskEventStream({
      baseUrl: "http://127.0.0.1:9999",
      fetchFn: fetchFn as unknown as typeof fetch,
      sleep: instantSleep,
      onEvent: () => {},
      onError: (message) => errors.push(message),
    });

    await vi.waitFor(() => {
      expect(fetchFn).toHaveBeenCalledTimes(2);
    });
    expect(errors).toEqual(["scheduled task event stream failed (503)"]);

    stream.stop();
  });
});
