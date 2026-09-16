/** @vitest-environment jsdom */
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeInfo, RuntimeModelProfileSummary } from "../shared/runtime";
import { useDesktopRuntime } from "./useDesktopRuntime";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const info = (root: string): RuntimeInfo => ({ defaultWorkspace: root, runtimeVersion: "test", apiVersion: "v1", configuredModel: "test", protocol: "test", hasApiKey: true, baseUrlHost: "local" });
let root: Root;
let result: ReturnType<typeof useDesktopRuntime>;
let client: Parameters<typeof useDesktopRuntime>[0];
const dispatch = vi.fn();
function Harness({ status }: { status: Parameters<typeof useDesktopRuntime>[1]; }) {
  result = useDesktopRuntime(client, status, dispatch);
  return null;
}
async function render(status: Parameters<typeof useDesktopRuntime>[1] = "running") {
  await act(async () => { root.render(createElement(Harness, { status })); });
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  root = createRoot(document.createElement("div"));
  client = {
    getRuntimeInfo: vi.fn(async () => info("current")),
    listModelProfiles: vi.fn(async () => [{ id: "profile" }] as RuntimeModelProfileSummary[]),
    listModelProviders: vi.fn(async () => []),
    getSupervisorStatus: vi.fn(async () => ({ status: "running" as const })),
    onSupervisorStatusChange: vi.fn(() => () => { }),
    notifyRendererReady: vi.fn(),
  };
});
afterEach(async () => { await act(async () => root.unmount()); vi.useRealTimers(); });

describe("runtime metadata refresh", () => {
  it("treats an attached Core as ready and refreshes runtime info", async () => {
    await render("external-attached");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.desktopReady).toBe(true);
    expect(result.desktopStarting).toBe(false);
    expect(client.getRuntimeInfo).toHaveBeenCalled();
    expect(client.listModelProfiles).toHaveBeenCalled();
    expect(client.listModelProviders).toHaveBeenCalled();
  });

  it("keeps starting until Core is running or attached", async () => {
    await render("starting");
    expect(result.desktopReady).toBe(false);
    expect(result.desktopStarting).toBe(true);
    expect(client.getRuntimeInfo).not.toHaveBeenCalled();
  });

  it("treats initial stopped as startup, then a later stopped Core as restart", async () => {
    await render("stopped");
    expect(result.desktopStarting).toBe(true);
    expect(result.desktopNeedsRestart).toBe(false);
    await render("running");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.desktopReady).toBe(true);
    await render("stopped");
    expect(result.desktopNeedsRestart).toBe(true);
    expect(result.desktopStarting).toBe(false);
  });

  it("shows a restart prompt when Core crashes before becoming ready", async () => {
    await render("crashed");
    expect(result.desktopNeedsRestart).toBe(true);
    expect(result.desktopStarting).toBe(false);
  });

  it("preserves the last model list on failure and distinguishes a successful empty list", async () => {
    await render();
    vi.mocked(client.listModelProfiles).mockRejectedValueOnce(new Error("offline"));
    await act(async () => { await result.refreshRuntimeInfoNow(); });
    expect(result.modelProfiles[0]?.id).toBe("profile");
    expect(result.runtimeRefreshError).toBeTruthy();
    expect(result.desktopReady).toBe(true);
    vi.mocked(client.listModelProfiles).mockResolvedValueOnce([]);
    await act(async () => { await result.refreshRuntimeInfoNow(); });
    expect(result.modelProfiles).toEqual([]);
    expect(result.runtimeRefreshError).toBeNull();
  });

  it("shares manual refresh with the pending poll and schedules the next poll after completion", async () => {
    const pending = deferred<RuntimeInfo>();
    vi.mocked(client.getRuntimeInfo).mockReturnValueOnce(pending.promise);
    await render();
    const first = result.refreshRuntimeInfoNow();
    expect(result.refreshRuntimeInfoNow()).toBe(first);
    await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
    expect(client.getRuntimeInfo).toHaveBeenCalledTimes(1);
    await act(async () => { pending.resolve(info("ready")); await first; });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(client.getRuntimeInfo).toHaveBeenCalledTimes(2);
  });

  it("does not let an old process response overwrite a restarted runtime", async () => {
    const old = deferred<RuntimeInfo>();
    vi.mocked(client.getRuntimeInfo).mockReturnValueOnce(old.promise);
    await render();
    await render("crashed");
    await render("running");
    expect(result.runtimeInfo?.defaultWorkspace).toBe("current");
    await act(async () => { old.resolve(info("obsolete")); });
    expect(result.runtimeInfo?.defaultWorkspace).toBe("current");
  });
});
