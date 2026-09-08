import {
  normalizeScheduledTaskEvent,
  type ScheduledTaskEvent,
  type ScheduledTaskEventWire,
} from "../shared/runtime.js";

// Scheduled-task Main-side adapter (contract [HTTP、IPC 与事件](docs/12_定时任务/定时执行与提醒/实现方案.md#httpipc-与事件)): parses
// the Core /v1/scheduled-tasks/events SSE stream, keeps it alive across
// reconnects and decides which events become native notifications. The
// module is Electron-free so vitest can drive it with mocked fetch/sleep.

/** Parses one SSE frame into the scheduled-task event wire payload, or null
 * when the frame carries no data lines (comments, heartbeats). */
export function parseScheduledTaskEventFrame(frame: string): ScheduledTaskEventWire | null {
  const dataLines = frame
    .split("\n")
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trimStart());
  if (dataLines.length === 0) {
    return null;
  }
  try {
    return JSON.parse(dataLines.join("\n")) as ScheduledTaskEventWire;
  } catch {
    return null;
  }
}

/** Only a live notice event with a body becomes a native notification. Core
 * emits a notice exactly once when it fires (never on replay/reconnect), so
 * filtering by type is sufficient to avoid duplicate notifications. */
export function scheduledTaskNotificationContent(
  event: ScheduledTaskEvent,
): { title: string; body: string } | null {
  if (event.type !== "notice") {
    return null;
  }
  const body = (event.content ?? "").trim();
  if (!body) {
    return null;
  }
  const title = (event.taskName ?? "").trim() || "定时任务";
  return { title, body };
}

export type ScheduledTaskEventStreamOptions = {
  baseUrl: string;
  onEvent: (event: ScheduledTaskEvent) => void;
  onError?: (message: string) => void;
  /** Injectable for tests; defaults to global fetch. */
  fetchFn?: typeof fetch;
  /** Injectable for tests; defaults to setTimeout-based sleep. */
  sleep?: (ms: number) => Promise<void>;
  initialReconnectDelayMs?: number;
  maxReconnectDelayMs?: number;
};

export type ScheduledTaskEventStream = {
  stop: () => void;
};

const defaultSleep = (ms: number) =>
  new Promise<void>((resolve) => {
    setTimeout(resolve, ms);
  });

async function pumpScheduledTaskEvents(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: ScheduledTaskEvent) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split("\n\n");
      buffer = frames.pop() ?? "";
      for (const frame of frames) {
        if (!frame.trim()) {
          continue;
        }
        const wire = parseScheduledTaskEventFrame(frame);
        if (wire) {
          onEvent(normalizeScheduledTaskEvent(wire));
        }
      }
    }
    if (buffer.trim()) {
      const wire = parseScheduledTaskEventFrame(buffer);
      if (wire) {
        onEvent(normalizeScheduledTaskEvent(wire));
      }
    }
  } finally {
    reader.releaseLock();
  }
}

/** Keeps a resident subscription to the Core scheduled-task event stream.
 * The stream is event-sourced live facts (no replay cursor), so a reconnect
 * simply re-opens the stream; Core re-pushes nothing and nothing is
 * duplicated. Reconnects back off exponentially up to a cap. */
export function startScheduledTaskEventStream(
  options: ScheduledTaskEventStreamOptions,
): ScheduledTaskEventStream {
  const fetchFn = options.fetchFn ?? fetch;
  const sleep = options.sleep ?? defaultSleep;
  const initialDelay = options.initialReconnectDelayMs ?? 1000;
  const maxDelay = options.maxReconnectDelayMs ?? 30_000;
  const eventsUrl = new URL(
    "/v1/scheduled-tasks/events",
    options.baseUrl.endsWith("/") ? options.baseUrl : `${options.baseUrl}/`,
  ).toString();

  let stopped = false;
  let controller: AbortController | null = null;

  void (async () => {
    let attempt = 0;
    while (!stopped) {
      try {
        controller = new AbortController();
        const response = await fetchFn(eventsUrl, {
          headers: { Accept: "text/event-stream" },
          signal: controller.signal,
        });
        if (!response.ok) {
          throw new Error(`scheduled task event stream failed (${response.status})`);
        }
        if (!response.body) {
          throw new Error("scheduled task event stream body is missing");
        }
        attempt = 0;
        await pumpScheduledTaskEvents(response.body, options.onEvent);
        if (stopped) {
          return;
        }
        throw new Error("scheduled task event stream closed");
      } catch (error) {
        if (stopped) {
          return;
        }
        options.onError?.(error instanceof Error ? error.message : String(error));
        const delay = Math.min(initialDelay * Math.pow(2, attempt), maxDelay);
        attempt += 1;
        await sleep(delay);
      }
    }
  })();

  return {
    stop: () => {
      stopped = true;
      controller?.abort();
    },
  };
}
