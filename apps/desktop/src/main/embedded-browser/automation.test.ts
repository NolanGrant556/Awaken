import { describe, expect, it, vi } from "vitest";
import { EMPTY_EMBEDDED_BROWSER_STATE } from "../../shared/embedded-browser.js";
import { runBrowserAutomation, type BrowserAutomationHost } from "./automation.js";

function fakeController(overrides: Record<string, unknown> = {}) {
  return {
    shown: true,
    hasViewport: true,
    state: () => ({
      ...EMPTY_EMBEDDED_BROWSER_STATE,
      url: "https://example.com/",
      title: "Example",
      hasPage: true,
      secure: true,
    }),
    currentURL: () => "https://example.com/",
    originAdmitted: () => true,
    navigate: vi.fn(async (url: string) => ({
      ...EMPTY_EMBEDDED_BROWSER_STATE,
      url,
      hasPage: true,
      secure: url.startsWith("https:"),
    })),
    executeJavaScript: vi.fn(async (code: string) => {
      if (code.includes("querySelector('[data-awaken-ref")) return true;
      if (code.includes("document.body")) {
        return { url: "https://example.com/", title: "Example", text: "hello world" };
      }
      return {
        url: "https://example.com/",
        title: "Example",
        items: ["[1] a Home", "[2] button Search"],
      };
    }),
    ...overrides,
  };
}

function asHost(manager: object): BrowserAutomationHost {
  return manager as BrowserAutomationHost;
}

describe("runBrowserAutomation", () => {
  it("rejects missing thread and non-http navigate", async () => {
    const manager = {
      ensureVisibleForAgent: vi.fn(),
      viewForAction: vi.fn(),
    };
    expect(await runBrowserAutomation(asHost(manager), { action: "snapshot", threadId: "" })).toEqual({
      ok: false,
      error: "thread_id_required",
    });
    expect(
      await runBrowserAutomation(asHost(manager), {
        action: "navigate",
        threadId: "t",
        url: "file:///tmp/index.html",
      }),
    ).toEqual({ ok: false, error: "unsupported_scheme" });
  });

  it("navigates a visible view and snapshots numbered refs", async () => {
    const controller = fakeController();
    const manager = {
      ensureVisibleForAgent: vi.fn(async () => controller),
      viewForAction: vi.fn(() => controller),
    };
    const navigated = await runBrowserAutomation(asHost(manager), {
      action: "navigate",
      threadId: "thread",
      url: "example.com",
    });
    expect(navigated.ok).toBe(true);
    expect(controller.navigate).toHaveBeenCalledWith("https://example.com/");

    const snapshot = await runBrowserAutomation(asHost(manager), {
      action: "snapshot",
      threadId: "thread",
    });
    expect(snapshot.snapshot).toContain("[1] a Home");
  });

  it("fails mutate when the view is hidden", async () => {
    const manager = {
      ensureVisibleForAgent: vi.fn(),
      viewForAction: vi.fn(() => null),
    };
    expect(
      await runBrowserAutomation(asHost(manager), {
        action: "click",
        threadId: "thread",
        ref: "[1]",
      }),
    ).toEqual({ ok: false, error: "browser_view_not_visible" });
  });

  it("fails click after the page origin changes", async () => {
    const controller = fakeController({ originAdmitted: () => false });
    const manager = {
      ensureVisibleForAgent: vi.fn(),
      viewForAction: vi.fn(() => controller),
    };
    expect(
      await runBrowserAutomation(asHost(manager), {
        action: "click",
        threadId: "thread",
        ref: "1",
      }),
    ).toEqual({ ok: false, error: "origin_lease_expired" });
  });
});
