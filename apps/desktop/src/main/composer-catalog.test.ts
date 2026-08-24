import { describe, expect, it, vi } from "vitest";
import {
  parseComposerCatalogEventFrame,
  startComposerCatalogEventStream,
} from "./composer-catalog.js";

describe("composer catalog event stream", () => {
  it("parses typed revision events and rejects malformed frames", () => {
    expect(
      parseComposerCatalogEventFrame(
        'event: composer-catalog\ndata: {"revision":3,"kind":"plugins"}',
      ),
    ).toEqual({ revision: 3, kind: "plugins" });
    expect(parseComposerCatalogEventFrame("data: {}")).toBeNull();
    expect(parseComposerCatalogEventFrame("event: ping")).toBeNull();
  });

  it("delivers stream frames and can be stopped", async () => {
    const encoder = new TextEncoder();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode(
            'event: composer-catalog\ndata: {"revision":2,"kind":"skills"}\n\n',
          ),
        );
        controller.close();
      },
    });
    const onEvent = vi.fn();
    const stream = startComposerCatalogEventStream({
      baseUrl: "http://127.0.0.1:9999",
      onEvent,
      fetchFn: vi.fn(
        async () => new Response(body, { status: 200 }),
      ) as typeof fetch,
      sleep: async () => new Promise<void>(() => {}),
    });
    await vi.waitFor(() =>
      expect(onEvent).toHaveBeenCalledWith({ revision: 2, kind: "skills" }),
    );
    stream.stop();
  });
});
