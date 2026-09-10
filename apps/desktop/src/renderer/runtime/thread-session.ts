import type { RuntimeClient, SubscribeHandlers } from "./client";
import type { RuntimeThreadTranscript, RuntimeRun } from "../../shared/runtime";
import type { RuntimeAction } from "./reducer";
import {
  actionsForThreadTranscript,
  streamCursorForTranscriptRun,
} from "./thread-transcript";

export type ThreadSessionOptions = {
  client: Pick<RuntimeClient, "getThreadTranscript" | "subscribeRun" | "subscribeThreadActivity">;
  dispatch: (action: RuntimeAction) => void;
  storage: Pick<Storage, "getItem" | "setItem" | "removeItem">;
  onTranscript: (
    transcript: RuntimeThreadTranscript,
    isCurrent: () => boolean,
  ) => void;
};

const RUN_STORAGE_KEY = "awaken:runId";

export function runNeedsSubscription(status: RuntimeRun["status"]): boolean {
  return status === "running" || status === "cancelling";
}

/** Owns one selected thread's replay and live stream, including pending opens.
 * Switching invalidates callbacks immediately, before asynchronous IPC settles.
 * Background execution remains Core-owned and is never cancelled here.
 */
export class ThreadSession {
  private generation = 0;
  private unsubscribe: (() => void) | undefined;
  private activityUnsubscribe: (() => void) | undefined;
  private activityGeneration: number | undefined;
  private selectedThreadId: string | undefined;
  private selectedRunId: string | undefined;

  constructor(private readonly options: ThreadSessionOptions) {}

  get currentGeneration(): number {
    return this.generation;
  }

  isCurrent(generation: number): boolean {
    return generation === this.generation;
  }

  dispose(): void {
    this.generation += 1;
    this.unsubscribe?.();
    this.unsubscribe = undefined;
    this.activityUnsubscribe?.();
    this.activityUnsubscribe = undefined;
    this.activityGeneration = undefined;
  }

  begin(threadId?: string): number {
    this.dispose();
    this.selectedThreadId = threadId;
    this.selectedRunId = undefined;
    this.options.storage.removeItem(RUN_STORAGE_KEY);
    return this.generation;
  }

  private clearStoredRun(runId: string): void {
    if (this.options.storage.getItem(RUN_STORAGE_KEY) === runId) {
      this.options.storage.removeItem(RUN_STORAGE_KEY);
    }
  }

  async replay(
    threadId: string,
    generation = this.begin(threadId),
  ): Promise<void> {
    if (!this.isCurrent(generation)) return;
    const transcript = await this.options.client.getThreadTranscript(threadId);
    if (!this.isCurrent(generation)) return;
    if (transcript.thread.threadId !== threadId)
      throw new Error("会话恢复返回了不匹配的任务。");
    this.selectedThreadId = threadId;
    const latestRun = transcript.runs.at(-1)?.run;
    this.selectedRunId = latestRun?.runId;
    this.options.dispatch({
      type: "run/replayBatch",
      actions: actionsForThreadTranscript(transcript),
    });
    this.options.onTranscript(transcript, () => this.isCurrent(generation));
    if (this.options.client.subscribeThreadActivity) await this.subscribeActivity(threadId, generation, transcript.activityCursor);
    if (!latestRun || !runNeedsSubscription(latestRun.status)) {
      if (latestRun) this.clearStoredRun(latestRun.runId);
      return;
    }
    await this.subscribe(
      latestRun,
      generation,
      streamCursorForTranscriptRun(transcript, latestRun.runId),
    );
  }

  async subscribe(
    run: Pick<RuntimeRun, "runId" | "threadId">,
    generation: number,
    afterCursor?: number,
  ): Promise<void> {
    if (!this.isCurrent(generation)) return;
    this.selectedThreadId = run.threadId;
    this.selectedRunId = run.runId;
    if (this.options.client.subscribeThreadActivity) await this.subscribeActivity(run.threadId, generation);
    if (!this.isCurrent(generation)) return;
    this.options.storage.setItem(RUN_STORAGE_KEY, run.runId);
    const isCurrent = () =>
      this.isCurrent(generation) &&
      this.selectedThreadId === run.threadId &&
      this.selectedRunId === run.runId;
    let closed = false;
    const handlers: SubscribeHandlers = {
      onEvent: (event) => {
        if (this.activityGeneration === generation && event.subagentTaskId) return;
        if (
          !isCurrent() ||
          (event.threadId && event.threadId !== run.threadId) ||
          (event.runId && event.runId !== run.runId)
        )
          return;
        if (
          event.type === "RUN_FINISHED" ||
          event.type === "RUN_ERROR" ||
          (event.type === "RUN_STATUS_CHANGED" &&
            ["finished", "cancelled", "error"].includes(event.status ?? ""))
        ) {
          this.clearStoredRun(run.runId);
        }
        this.options.dispatch({
          type: "run/event",
          event,
          receivedAtMs: Date.now(),
        });
      },
      onReconnect: () => {
        if (isCurrent()) this.options.dispatch({ type: "run/reconnecting" });
      },
      onError: (message) => {
        if (!isCurrent()) return;
        // A missing stream is not evidence that the user cancelled the run.
        // Reconcile the canonical transcript; never synthesize cancellation.
        if (message.toLowerCase().includes("run session was not found")) {
          void this.options.client
            .getThreadTranscript(run.threadId)
            .then((transcript) => {
              if (!isCurrent()) return;
              if (transcript.thread.threadId !== run.threadId)
                throw new Error("会话恢复返回了不匹配的任务。");
              const latestRun = transcript.runs.at(-1)?.run;
              if (latestRun && !runNeedsSubscription(latestRun.status)) {
                this.clearStoredRun(run.runId);
                this.options.dispatch({
                  type: "run/replayBatch",
                  actions: actionsForThreadTranscript(transcript),
                });
                this.options.onTranscript(transcript, isCurrent);
              } else {
                this.options.dispatch({ type: "run/streamError", message });
              }
            })
            .catch(() => {
              if (isCurrent())
                this.options.dispatch({ type: "run/streamError", message });
            });
          return;
        }
        this.options.dispatch({ type: "run/streamFailed", message });
      },
      onClose: () => {
        closed = true;
        if (isCurrent()) this.unsubscribe = undefined;
      },
    };
    const unsubscribe = await this.options.client.subscribeRun(
      run.runId,
      handlers,
      afterCursor,
    );
    if (!isCurrent() || closed) unsubscribe();
    else this.unsubscribe = unsubscribe;
  }

  private async subscribeActivity(threadId: string, generation: number, afterCursor?: number): Promise<void> {
    const subscribe = this.options.client.subscribeThreadActivity;
    if (!subscribe || !this.isCurrent(generation) || this.activityGeneration === generation) return;
    this.activityGeneration = generation;
    const stop = await subscribe(threadId, {
      onEvent: (event) => {
        if (!this.isCurrent(generation) || this.selectedThreadId !== threadId || (event.threadId && event.threadId !== threadId)) return;
        this.options.dispatch({ type: "run/event", event, receivedAtMs: Date.now() });
      },
      onError: (message) => {
        if (this.isCurrent(generation) && message.includes("ACTIVITY_CURSOR_EXPIRED")) void this.replay(threadId);
      },
    }, afterCursor);
    if (!this.isCurrent(generation)) stop(); else this.activityUnsubscribe = stop;
  }
}
