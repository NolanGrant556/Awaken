import type { ComposerCatalogEvent } from "../shared/runtime.js";

export function parseComposerCatalogEventFrame(
  frame: string,
): ComposerCatalogEvent | null {
  const dataLines = frame
    .split("\n")
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trimStart());
  if (dataLines.length === 0) {
    return null;
  }
  try {
    const parsed = JSON.parse(
      dataLines.join("\n"),
    ) as Partial<ComposerCatalogEvent>;
    if (!Number.isFinite(parsed.revision) || Number(parsed.revision) <= 0) {
      return null;
    }
    return {
      revision: Number(parsed.revision),
      kind:
        typeof parsed.kind === "string" && parsed.kind
          ? parsed.kind
          : "snapshot",
    };
  } catch {
    return null;
  }
}

export type ComposerCatalogEventStream = { stop: () => void };

export function startComposerCatalogEventStream(options: {
  baseUrl: string;
  onEvent: (event: ComposerCatalogEvent) => void;
  onError?: (message: string) => void;
  fetchFn?: typeof fetch;
  sleep?: (ms: number) => Promise<void>;
  initialReconnectDelayMs?: number;
  maxReconnectDelayMs?: number;
}): ComposerCatalogEventStream {
  const fetchFn = options.fetchFn ?? fetch;
  const sleep =
    options.sleep ??
    ((ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms)));
  const initialDelay = options.initialReconnectDelayMs ?? 1000;
  const maxDelay = options.maxReconnectDelayMs ?? 30_000;
  const eventsUrl = new URL(
    "/v1/composer-catalog/events",
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
          throw new Error(
            `composer catalog event stream failed (${response.status})`,
          );
        }
        if (!response.body) {
          throw new Error("composer catalog event stream body is missing");
        }
        attempt = 0;
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        try {
          while (!stopped) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            const frames = buffer.split("\n\n");
            buffer = frames.pop() ?? "";
            for (const frame of frames) {
              const event = parseComposerCatalogEventFrame(frame);
              if (event) options.onEvent(event);
            }
          }
        } finally {
          reader.releaseLock();
        }
        if (stopped) return;
        throw new Error("composer catalog event stream closed");
      } catch (error) {
        if (stopped) return;
        options.onError?.(
          error instanceof Error ? error.message : String(error),
        );
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
