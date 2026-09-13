import { describe, expect, it, vi } from "vitest";
import { EMPTY_EMBEDDED_BROWSER_STATE } from "../../shared/embedded-browser.js";
import { EmbeddedBrowserViewManager } from "./manager.js";

function fakeController() {
  const controller = {
    shown: false,
    hasViewport: false,
    hide: vi.fn(() => {
      controller.shown = false;
      controller.hasViewport = false;
    }),
    setBounds: vi.fn((rect: { width: number; height: number } | null) => {
      controller.shown = Boolean(rect && rect.width > 0 && rect.height > 0);
      controller.hasViewport = controller.shown;
    }),
    navigate: vi.fn(async (url: string) => ({
      ...EMPTY_EMBEDDED_BROWSER_STATE,
      url,
      hasPage: true,
      secure: url.startsWith("https:"),
    })),
    state: () => ({
      ...EMPTY_EMBEDDED_BROWSER_STATE,
      url: "https://example.com/",
      hasPage: true,
      secure: true,
    }),
    currentURL: () => "https://example.com/",
    originAdmitted: () => true,
    destroy: vi.fn(),
    waitForLiveViewport: vi.fn(async () => true),
    back: () => EMPTY_EMBEDDED_BROWSER_STATE,
    forward: () => EMPTY_EMBEDDED_BROWSER_STATE,
    reload: () => EMPTY_EMBEDDED_BROWSER_STATE,
    stop: () => EMPTY_EMBEDDED_BROWSER_STATE,
  };
  return controller;
}

describe("EmbeddedBrowserViewManager", () => {
  it("keeps thread pages isolated and hides inactive views", async () => {
    const created: ReturnType<typeof fakeController>[] = [];
    const manager = new EmbeddedBrowserViewManager({
      getWindow: () => ({ isDestroyed: () => false }) as never,
      onState: vi.fn(),
      onRequestShow: vi.fn(),
      createView: () => {
        const controller = fakeController();
        created.push(controller);
        return controller as never;
      },
    });

    const first = { viewId: "slot:1", threadId: "thread-a" };
    const second = { viewId: "slot:1", threadId: "thread-b" };
    await manager.navigate(first, "https://one.example/");
    await manager.navigate(second, "https://two.example/");
    expect(created).toHaveLength(2);
    expect(created[0].hide).toHaveBeenCalled();
    expect(created[1].navigate).toHaveBeenCalledWith("https://two.example/");

    manager.setActive(first);
    expect(created[1].hide).toHaveBeenCalled();
    manager.setViewport(first, { x: 10, y: 20, width: 400, height: 300 });
    expect(created[0].setBounds).toHaveBeenCalled();
  });

  it("fails agent visibility when the renderer never focuses a tab", async () => {
    const manager = new EmbeddedBrowserViewManager({
      getWindow: () => ({ isDestroyed: () => false }) as never,
      onState: vi.fn(),
      onRequestShow: vi.fn(),
      createView: () => fakeController() as never,
    });
    vi.useFakeTimers();
    try {
      const pending = expect(
        manager.ensureVisibleForAgent("thread-z"),
      ).rejects.toThrow("browser_view_not_visible");
      await vi.advanceTimersByTimeAsync(4100);
      await pending;
    } finally {
      vi.useRealTimers();
    }
  });
});
