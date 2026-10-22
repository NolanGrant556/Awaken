import { describe, expect, it, vi } from "vitest";
import { EMPTY_EMBEDDED_BROWSER_STATE } from "../../shared/embedded-browser.js";
import { runBrowserAutomation, type BrowserAutomationHost, type BrowserHostRequest } from "./automation.js";
import { BrowserControlGate } from "./control.js";

const identity = { threadId: "thread", runId: "run", agentId: "A", taskId: "task-A", turnId: "turn", requestId: "call-A" };
function request(action: BrowserHostRequest["action"], extra: Partial<BrowserHostRequest> = {}): BrowserHostRequest {
  return { ...identity, action, ...extra };
}
function fixture() {
  let url = "https://example.com/";
  const controller = {
    documentRevision: 0,
    stop: vi.fn(),
    state: () => ({ ...EMPTY_EMBEDDED_BROWSER_STATE, url, title: "Example", hasPage: true }),
    currentURL: () => url,
    originAdmitted: () => true,
    executeJavaScript: vi.fn(async (code: string): Promise<any> => {
      if (code.includes("document.body")) return { url, title: "Example", text: "hello" };
      if (code.includes("const node = document.querySelector")) return true;
      return { url, title: "Example", items: ["[1] a Home"] };
    }),
  };
  const manager = {
    agentControl: new BrowserControlGate(),
    navigateForAgent: vi.fn(async (_thread: string, value: string) => { url = value; controller.documentRevision++; return controller.state(); }),
    viewForAction: vi.fn(() => controller),
  };
  const run = (action: BrowserHostRequest["action"], extra: Partial<BrowserHostRequest> = {}, signal?: AbortSignal) => runBrowserAutomation(manager as unknown as BrowserAutomationHost, request(action, extra), signal);
  return { manager, controller, run };
}

describe("browser ownership and receipts", () => {
  it("rejects missing identity, unsafe URL and actions without control", async () => {
    const { run } = fixture();
    expect(await run("navigate", { agentId: "", url: "example.com" })).toMatchObject({ ok: false, error: "browser_identity_required" });
    expect(await run("navigate", { url: "file:///private.html" })).toMatchObject({ ok: false, error: "unsupported_scheme" });
    expect(await run("snapshot")).toMatchObject({ ok: false, error: "browser_control_required" });
  });

  it("retains ownership across turns and returns this invocation's receipt", async () => {
    const { run } = fixture();
    expect(await run("navigate", { url: "example.com" })).toMatchObject({ ok: true, url: "https://example.com/", requestId: "call-A" });
    expect(await run("snapshot", { turnId: "next-turn", requestId: "snapshot-A" })).toMatchObject({ ok: true, snapshot: "[1] a Home", requestId: "snapshot-A" });
    expect(await run("click", { ref: "1" })).toMatchObject({ ok: true });
  });

  it("queues B behind A's full sequence and returns each requested page", async () => {
    vi.useFakeTimers();
    try {
      const { run, manager } = fixture();
      await run("navigate", { url: "https://example.com/A1" });
      const b = { agentId: "B", taskId: "task-B", requestId: "call-B", url: "https://example.com/B1" };
      let finished = false;
      const pending = run("navigate", b).then((value) => { finished = true; return value; });
      await vi.advanceTimersByTimeAsync(100);
      expect(finished).toBe(false);
      expect(manager.navigateForAgent).toHaveBeenCalledTimes(1);
      expect(await run("extract")).toMatchObject({ ok: true, url: "https://example.com/A1" });
      await run("navigate", { url: "https://example.com/A2" });
      expect(await run("snapshot")).toMatchObject({ ok: true, url: "https://example.com/A2" });
      await run("release");
      await vi.advanceTimersByTimeAsync(25);
      expect(await pending).toMatchObject({ ok: true, url: b.url, requestId: b.requestId });
      expect(await run("extract")).toMatchObject({ ok: false, error: "browser_busy" });
    } finally { vi.useRealTimers(); }
  });

  it("releases on completion and rejects late calls from the old assignment", async () => {
    const { run } = fixture();
    await run("navigate", { url: "example.com" });
    await run("end_owner");
    expect(await run("navigate", { url: "example.com/late" })).toMatchObject({ ok: false, error: "browser_owner_ended" });
    expect(await run("navigate", { taskId: "next-assignment", url: "example.com/next" })).toMatchObject({ ok: true });
    // A delayed cleanup for the earlier attempt cannot revoke the new lease.
    await run("end_owner");
    expect(await run("extract", { taskId: "next-assignment" })).toMatchObject({ ok: true });
  });

  it("does not navigate a queued request after HTTP cancellation", async () => {
    vi.useFakeTimers();
    try {
      const { run, manager } = fixture();
      await run("navigate", { url: "example.com/A" });
      const abort = new AbortController();
      const pending = run("navigate", { agentId: "B", url: "example.com/B" }, abort.signal);
      abort.abort();
      await vi.advanceTimersByTimeAsync(25);
      expect(await pending).toMatchObject({ ok: false, error: "browser_action_cancelled" });
      await run("release");
      expect(manager.navigateForAgent).toHaveBeenCalledTimes(1);
    } finally { vi.useRealTimers(); }
  });

  it("fails an in-flight read if the user takes control", async () => {
    const { run, controller, manager } = fixture();
    await run("navigate", { url: "example.com/A" });
    controller.executeJavaScript.mockImplementationOnce(async () => {
      manager.agentControl.invalidate();
      return { url: "https://example.com/B", text: "wrong page" };
    });
    expect(await run("extract")).toMatchObject({ ok: false, error: "browser_control_lost" });
  });

  it("rejects stale documents and requires a new snapshot before clicking", async () => {
    const { run, controller } = fixture();
    await run("navigate", { url: "example.com/A" });
    expect(await run("click", { ref: "1" })).toMatchObject({ ok: false, error: "browser_snapshot_expired" });
    await run("navigate", { url: "example.com/A" });
    await run("snapshot");
    controller.documentRevision++;
    expect(await run("type", { ref: "1", text: "test" })).toMatchObject({ ok: false, error: "browser_navigation_interrupted" });
    expect(controller.executeJavaScript).toHaveBeenCalledTimes(1);
  });

  it("preserves origin restrictions", async () => {
    const { run, controller } = fixture();
    await run("navigate", { url: "example.com" });
    await run("snapshot");
    controller.originAdmitted = () => false;
    expect(await run("click", { ref: "1" })).toMatchObject({ ok: false, error: "origin_lease_expired" });
  });
  it("cancels an active wait on assignment end and admits the next owner", async () => {
    const { run, controller } = fixture();
    await run("navigate", { url: "example.com/A" });
    const waiting = run("wait", { seconds: 120 });
    await run("end_owner");
    expect(await waiting).toMatchObject({ ok: false, error: "browser_owner_ended" });
    expect(controller.stop).toHaveBeenCalledTimes(1);
    expect(await run("navigate", { agentId: "B", url: "example.com/B" })).toMatchObject({ ok: true, url: "https://example.com/B" });
  });

});
