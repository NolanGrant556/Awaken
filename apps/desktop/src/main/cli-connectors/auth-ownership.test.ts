import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { expect, it, vi } from "vitest";
import { CliConnectorManager } from "./manager.js";
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

async function fixture() {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-audit-auth-"));
  const manager = new CliConnectorManager({
    homeDir: root,
    platform: "darwin",
    architecture: "arm64",
    manifestPath: path.join(root, "missing.json"),
    fetchAsset: vi.fn(),
    publishReadiness: async () => {},
    onStateChanged: () => {},
    openExternal: async () => {},
  });
  // Inject transport boundaries while exercising the public auth lifecycle.
  const m = manager as any;
  m.requireExecutable = () => "unused-fake-cli";
  return { root, manager, m };
}

it("late failure from a cancelled Lark login must not kill the replacement login", async () => {
  const { root, manager, m } = await fixture();
  const oldResult = deferred<any>();
  const oldStarted = deferred<void>();
  const newerResult = deferred<any>();
  const newStarted = deferred<void>();
  let n = 0;
  const workers: Promise<void>[] = [];
  const original = m.startLarkSplitAuth.bind(m);
  m.startLarkSplitAuth = (...args: any[]) => { const work = original(...args); workers.push(work); return work; };
  m.runCommand = () => {
    if (++n === 1) {
      oldStarted.resolve();
      return oldResult.promise;
    }
    newStarted.resolve();
    return newerResult.promise;
  };
  try {
    await manager.startAuth("lark-cli");
    await oldStarted.promise;
    await manager.cancelAuth("lark-cli");
    await manager.startAuth("lark-cli");
    await newStarted.promise;
    const replacement = m.authSessions.get("lark-cli");
    const kill = vi.fn();
    replacement.child = { kill };
    oldResult.reject(new Error("old request transport timeout"));
    await workers[0]; // wait for the old callback, independent of its outcome
    expect(kill, "cancelled auth callback killed newer child").not.toHaveBeenCalled();
  }
  finally {
    await manager.cancelAuth("lark-cli");
    newerResult.resolve({ exitCode: 0, stdout: "{}", stderr: "" });
    await fs.rm(root, { recursive: true, force: true });
  }
});
it("concurrent startAuth must create one provider session", async () => {
  const { root, manager, m } = await fixture();
  const release = deferred<void>();
  let calls = 0;
  m.prepareNetworkOperation = () => release.promise;
  m.startLarkSplitAuth = vi.fn(async () => { calls++; });
  try {
    const a = manager.startAuth("lark-cli"), b = manager.startAuth("lark-cli");
    release.resolve();
    const results = await Promise.all([a, b]);
    expect(calls, "two auth workers were launched").toBe(1);
    expect(results[0].authSession?.id).toBe(results[1].authSession?.id);
  }
  finally {
    await manager.cancelAuth("lark-cli");
    await fs.rm(root, { recursive: true, force: true });
  }
});
it.each(["cancel", "dispose"])("%s during initialization cannot launch a late auth worker", async (action) => {
  const { root, manager, m } = await fixture();
  const release = deferred<void>();
  m.prepareNetworkOperation = () => release.promise;
  m.startLarkSplitAuth = vi.fn(async () => { });
  const start = manager.startAuth("lark-cli");
  try {
    if (action === "cancel")
      await manager.cancelAuth("lark-cli");
    else
      await manager.dispose();
    release.resolve();
    await start;
    expect(m.startLarkSplitAuth).not.toHaveBeenCalled();
    expect(m.authSessions.size).toBe(0);
  }
  finally {
    release.resolve();
    await start;
    await fs.rm(root, { recursive: true, force: true });
  }
});
it("late successful verification cannot delete a newer auth session", async () => {
  const { EventEmitter } = await import("node:events");
  const { root, manager, m } = await fixture();
  const refresh = deferred<any>();
  const refreshing = deferred<void>();
  m.startLarkSplitAuth = vi.fn(async () => { });
  m.refresh = () => { refreshing.resolve(); return refresh.promise; };
  const child = new EventEmitter();
  (child as any).kill = vi.fn();
  try {
    await manager.startAuth("lark-cli");
    const oldSession = m.authSessions.get("lark-cli");
    oldSession.child = child;
    const finishing = m.finishBlockingSession(oldSession, child, false);
    child.emit("exit", 0);
    await refreshing.promise;
    await manager.cancelAuth("lark-cli");
    await manager.startAuth("lark-cli");
    const newer = m.authSessions.get("lark-cli");
    refresh.resolve({ readiness: { authentication: "authorized" } });
    await finishing;
    expect(m.authSessions.get("lark-cli")).toBe(newer);
    expect(newer.view.phase).toBe("starting");
  }
  finally {
    await manager.cancelAuth("lark-cli");
    await fs.rm(root, { recursive: true, force: true });
  }
});
