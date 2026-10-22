import type { EmbeddedBrowserViewController } from "./controller.js";

export type BrowserIdentity = {
  threadId: string;
  runId: string;
  agentId: string;
  taskId?: string;
  turnId: string;
  requestId: string;
};

export type BrowserControl = {
  owner: string;
  cancellation: AbortController;
  controller?: EmbeddedBrowserViewController;
  snapshotToken?: string;
  documentRevision?: number;
  expiresAt: number;
  assertCurrent: () => void;
};

function ownerKey(id: BrowserIdentity): string {
  return JSON.stringify([id.threadId, id.runId, id.agentId, id.taskId ?? ""]);
}

// One visible page is shared by the whole Desktop window. Retain ownership
// between tool calls, not just while loadURL is pending.
export class BrowserControlGate {
  private current?: BrowserControl;
  private executing = false;
  private interruption = 0;
  private readonly ended = new Set<string>();

  invalidate(): void {
    this.interruption++;
    this.revoke();
  }

  private revoke(): void {
    const control = this.current;
    this.current = undefined;
    control?.cancellation.abort();
    if (this.executing) control?.controller?.stop();
  }

  release(id: BrowserIdentity, ended = false): void {
    const owner = ownerKey(id);
    if (ended) {
      this.ended.add(owner);
      // Tombstones reject late requests from completed assignments.
      if (this.ended.size > 2048) this.ended.delete(this.ended.values().next().value!);
    }
    if (this.current?.owner === owner) this.revoke();
  }

  async run<T>(id: BrowserIdentity, action: string, execute: (control: BrowserControl) => Promise<T>, signal?: AbortSignal): Promise<T> {
    const owner = ownerKey(id);
    const interruption = this.interruption;
    const deadline = Date.now() + 30_000;
    const check = () => {
      if (interruption !== this.interruption) throw new Error("browser_control_lost");
      if (signal?.aborted) throw new Error("browser_action_cancelled");
      if (this.ended.has(owner)) throw new Error("browser_owner_ended");
    };
    check();
    while (true) {
      check();
      if (!this.executing && this.current && this.current.expiresAt <= Date.now()) this.revoke();
      if (!this.executing && (!this.current || this.current.owner === owner)) break;
      if (action !== "navigate" || Date.now() >= deadline) throw new Error("browser_busy");
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    check();
    if (!this.current && action !== "navigate") throw new Error("browser_control_required");
    // A waiting navigation may acquire a released lease, but a cancelled HTTP
    // request must never navigate after its caller has gone away.
    const control = this.current ?? {
      owner,
      cancellation: new AbortController(),
      expiresAt: 0,
      assertCurrent: () => {
        check();
        if (this.current !== control) throw new Error("browser_control_lost");
      },
    };
    control.assertCurrent = () => {
      check();
      if (this.current !== control) throw new Error("browser_control_lost");
    };
    this.current = control;
    control.expiresAt = Date.now() + 120_000;
    this.executing = true;
    const abort = () => {
      if (this.current !== control) return;
      this.revoke();
    };
    signal?.addEventListener("abort", abort, { once: true });
    try {
      control.assertCurrent();
      const result = await execute(control);
      control.assertCurrent();
      return result;
    } catch (error) {
      if (this.current === control) this.revoke();
      throw error;
    } finally {
      signal?.removeEventListener("abort", abort);
      this.executing = false;
      if (this.current === control) control.expiresAt = Date.now() + 120_000;
    }
  }
}
