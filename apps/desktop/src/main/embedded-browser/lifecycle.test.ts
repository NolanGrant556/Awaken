import { EventEmitter } from "node:events";
import type { BrowserWindow, WebContents } from "electron";
import { describe, expect, it, vi } from "vitest";
vi.mock("electron", () => ({ shell: { openExternal: vi.fn() } }));
import { EmbeddedBrowserViewController } from "./controller.js";
import { EmbeddedBrowserViewManager } from "./manager.js";
import { registerEmbeddedBrowserReloadHandler } from "./ipc.js";
import { runBrowserAutomation } from "./automation.js";

class Guest extends EventEmitter {
  url = "";
  session = { setPermissionRequestHandler: vi.fn(), setPermissionCheckHandler: vi.fn(), on: vi.fn() };
  navigationHistory = { canGoBack: () => false, canGoForward: () => false };
  isDestroyed = () => false;
  getURL = () => this.url;
  getTitle = () => "Test page";
  isLoading = () => false;
  setWindowOpenHandler = vi.fn();
  loadURL = vi.fn(async (url: string) => {
    this.url = url;
    this.emit("did-navigate");
  });
  executeJavaScript = vi.fn(async (code: string) => {
    if (code.includes("document.body")) return { url: this.url, title: "Test page", text: "Page text" };
    return { url: this.url, title: "Test page", items: ["[1] button Test"] };
  });
}
const window = { isDestroyed: () => false } as BrowserWindow;
const rect = { x: 0, y: 0, width: 300, height: 200 };
const ref = { threadId: "thread-a", viewId: "slot-1" };

function managerWithState(onState = vi.fn(), onRequestShow = vi.fn()) {
  return new EmbeddedBrowserViewManager({ getWindow: () => window, onState, onRequestShow });
}
function attach(manager: EmbeddedBrowserViewManager, guest: Guest, allowFile = false) {
  manager.attachGuest(ref, guest as unknown as WebContents, allowFile);
  manager.setActive(ref);
  manager.setViewport(ref, rect);
}

describe("browser guest lifecycle and Agent admission", () => {
  it.each([false, true])("reports clicks only from the active guest (local=%s)", (allowFile) => {
    const onInteraction = vi.fn();
    const manager = new EmbeddedBrowserViewManager({ getWindow: () => window, onState: vi.fn(), onRequestShow: vi.fn(), onInteraction });
    const guest = new Guest();
    attach(manager, guest, allowFile);
    const mouse = (type: string, x = 10, button = "left") => guest.emit("before-mouse-event", {}, { type, x, y: 10, button });
    const click = () => { mouse("mouseDown"); mouse("mouseUp"); };
    mouse("mouseDown");
    expect(onInteraction).not.toHaveBeenCalled();
    mouse("mouseUp");
    expect(onInteraction).toHaveBeenLastCalledWith({ ...ref, type: "primary-click" });
    onInteraction.mockClear();
    mouse("mouseDown"); mouse("mouseMove", 40); mouse("mouseUp", 40);
    mouse("mouseDown", 10, "right"); mouse("mouseUp", 10, "right");
    expect(onInteraction).not.toHaveBeenCalled();
    mouse("mouseDown"); manager.setActive(null); manager.setActive(ref); manager.setViewport(ref, rect); mouse("mouseUp");
    expect(onInteraction).not.toHaveBeenCalled();
    mouse("mouseDown"); manager.setViewport(ref, null); manager.setViewport(ref, rect); mouse("mouseUp");
    expect(onInteraction).not.toHaveBeenCalled();
    manager.setActive(null); click();
    expect(onInteraction).not.toHaveBeenCalled();
    manager.setActive(ref); manager.setViewport(ref, rect); click();
    expect(onInteraction).toHaveBeenCalledTimes(1);
    manager.detachGuest(ref); click();
    expect(onInteraction).toHaveBeenCalledTimes(1);
    expect(guest.listenerCount("before-mouse-event")).toBe(0);
  });

  it("routes popup links into the current public guest without admitting a new origin", async () => {
    const manager = managerWithState();
    const guest = new Guest();
    attach(manager, guest);
    await manager.navigate(ref, "https://example.com/");
    const open = guest.setWindowOpenHandler.mock.calls[0][0] as (details: { url: string }) => { action: string };
    expect(open({ url: "https://other.example/page" })).toEqual({ action: "deny" });
    await Promise.resolve();
    expect(guest.url).toBe("https://other.example/page");
    expect(await runBrowserAutomation(manager, { action: "click", threadId: ref.threadId, ref: "1" })).toMatchObject({ ok: false, error: "origin_lease_expired" });
    const loads = guest.loadURL.mock.calls.length;
    expect(open({ url: "javascript:alert(1)" })).toEqual({ action: "deny" });
    expect(open({ url: "file:///tmp/private.html" })).toEqual({ action: "deny" });
    expect(guest.loadURL).toHaveBeenCalledTimes(loads);
  });

  it("hands local popup HTTP links to the manager to switch guest partitions", async () => {
    const manager = managerWithState();
    const guest = new Guest();
    guest.url = "file:///tmp/local.html";
    attach(manager, guest, true);
    const open = guest.setWindowOpenHandler.mock.calls[0][0] as (details: { url: string }) => { action: string };
    expect(open({ url: "https://example.com/page" })).toEqual({ action: "deny" });
    expect(guest.loadURL).not.toHaveBeenCalled();
    expect(manager.getState(ref).url).toBe("https://example.com/page");
  });
  it("clears browser views only for main-document reloads, not guest loading", () => {
    const contents = new EventEmitter();
    const hide = vi.fn();
    registerEmbeddedBrowserReloadHandler(contents as unknown as WebContents, hide);
    contents.emit("did-start-loading");
    contents.emit("did-start-navigation", { isMainFrame: false, isSameDocument: false });
    contents.emit("did-start-navigation", { isMainFrame: true, isSameDocument: true });
    expect(hide).not.toHaveBeenCalled();
    contents.emit("did-start-navigation", { isMainFrame: true, isSameDocument: false });
    expect(hide).toHaveBeenCalledTimes(1);
  });

  it("mounts a fresh public guest from the Agent URL before waiting for visibility", async () => {
    const guest = new Guest();
    let attached = false;
    const onState = vi.fn((_viewId, _threadId, state) => {
      if (attached) return;
      expect(state.url).toBe("https://example.com/");
      attached = true;
      attach(manager, guest);
    });
    const manager = managerWithState(onState, vi.fn(() => manager.setActive(ref)));
    const result = await runBrowserAutomation(manager, { action: "navigate", threadId: ref.threadId, url: "example.com" });
    expect(result).toMatchObject({ ok: true, url: "https://example.com/" });
    expect(guest.loadURL).toHaveBeenCalledTimes(1);
    expect(await runBrowserAutomation(manager, { action: "snapshot", threadId: ref.threadId })).toMatchObject({ ok: true, snapshot: "[1] button Test" });
  });

  it("waits for loading and does not replay a redirect or admit its destination", async () => {
    const guest = new Guest();
    let finishLoad!: () => void;
    const loaded = new Promise<void>((resolve) => { finishLoad = resolve; });
    guest.loadURL.mockImplementation(async () => {
      await loaded;
      guest.url = "https://redirect.example/";
      guest.emit("did-navigate");
    });
    let attached = false;
    const manager = managerWithState(vi.fn(() => {
      if (attached) return;
      attached = true;
      attach(manager, guest);
    }), vi.fn(() => manager.setActive(ref)));
    let finished = false;
    const action = runBrowserAutomation(manager, { action: "navigate", threadId: ref.threadId, url: "https://example.com/" });
    void action.then(() => { finished = true; });
    await Promise.resolve();
    await Promise.resolve();
    expect(finished).toBe(false);
    finishLoad();
    expect(await action).toMatchObject({ ok: true, url: "https://redirect.example/" });
    expect(guest.loadURL).toHaveBeenCalledTimes(1);
    expect(manager.viewForAction(ref.threadId, "mutate")?.originAdmitted(guest.url)).toBe(false);
  });

  it("denies every Agent action on local HTML and switches navigation to a public guest", async () => {
    const local = new Guest();
    local.url = "file:///tmp/index.html";
    const publicGuest = new Guest();
    let switching = false;
    const manager = managerWithState(vi.fn((_viewId, _threadId, state) => {
      if (!switching || !state.url.startsWith("https:")) return;
      switching = false;
      attach(manager, publicGuest);
    }), vi.fn(() => manager.setActive(ref)));
    attach(manager, local, true);
    for (const action of ["snapshot", "extract", "wait", "click", "type"] as const) {
      expect((await runBrowserAutomation(manager, { action, threadId: ref.threadId, ref: "1", text: "test" })).ok).toBe(false);
    }
    expect(local.executeJavaScript).not.toHaveBeenCalled();
    switching = true;
    expect(await runBrowserAutomation(manager, { action: "navigate", threadId: ref.threadId, url: "https://example.com/" })).toMatchObject({ ok: true, url: "https://example.com/" });
    expect(local.loadURL).not.toHaveBeenCalled();
    expect(manager.viewForAction(ref.threadId, "observe")?.loadsLocalFiles).toBe(false);
  });

  it("invalidates cross-origin admission until an explicit navigation, including after remount", async () => {
    const guest = new Guest();
    guest.url = "https://one.example/";
    const manager = managerWithState();
    attach(manager, guest);
    await manager.navigate(ref, guest.url);
    guest.url = "https://two.example/";
    guest.emit("did-navigate");
    expect(await runBrowserAutomation(manager, { action: "click", threadId: ref.threadId, ref: "1" })).toMatchObject({ ok: false, error: "origin_lease_expired" });
    manager.detachGuest(ref);
    expect(guest.listenerCount("did-navigate")).toBe(0);
    const restored = new Guest();
    restored.url = "https://two.example/";
    attach(manager, restored);
    expect(await runBrowserAutomation(manager, { action: "type", threadId: ref.threadId, ref: "1", text: "test" })).toMatchObject({ ok: false, error: "origin_lease_expired" });
    await manager.navigate(ref, restored.url);
    expect(manager.viewForAction(ref.threadId, "mutate")?.originAdmitted(restored.url)).toBe(true);
  });

  it("restores the latest URL and existing lease only for the same thread and tab", async () => {
    const guest = new Guest();
    guest.url = "https://one.example/";
    const manager = managerWithState();
    attach(manager, guest);
    await manager.navigate(ref, guest.url);
    guest.url = "https://one.example/next";
    guest.emit("did-navigate");
    guest.emit("destroyed");
    expect(manager.getState(ref).url).toBe(guest.url);
    manager.detachGuest(ref);
    expect(manager.getState(ref).url).toBe(guest.url);
    expect(manager.getState({ ...ref, threadId: "thread-b" }).hasPage).toBe(false);
    const restored = new Guest();
    restored.url = guest.url;
    attach(manager, restored);
    expect(restored.loadURL).not.toHaveBeenCalled();
    expect(manager.viewForAction(ref.threadId, "mutate")?.originAdmitted(restored.url)).toBe(true);
    manager.detachGuest(ref);
    manager.closeThread(ref.threadId);
    expect(manager.getState(ref).hasPage).toBe(false);
  });

  it("blocks direct Agent execution and HTTP loading on a local controller", async () => {
    const guest = new Guest();
    guest.url = "file:///tmp/index.html";
    const controller = new EmbeddedBrowserViewController(window, vi.fn(), { guest: guest as unknown as WebContents, allowFile: true });
    await expect(controller.executeJavaScript("document.body.innerText")).rejects.toThrow("unsupported_scheme");
    await expect(controller.navigate("https://example.com/")).rejects.toThrow("unsupported_scheme");
    expect(guest.loadURL).not.toHaveBeenCalled();
    controller.destroy();
  });
});
