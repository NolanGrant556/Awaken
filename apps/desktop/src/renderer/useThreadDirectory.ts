import { useEffect, useRef } from "react";
import type { RuntimeThread } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeAction } from "./runtime/reducer";

export function useThreadDirectory(
  runtimeClient: RuntimeClient,
  desktopReady: boolean,
  dispatch: (action: RuntimeAction) => void,
) {
  const threadTitlePollsRef = useRef<Map<string, () => void>>(new Map());

  useEffect(() => {
    return () => {
      stopThreadTitlePoll();
    };
  }, []);

  async function refreshThreads(): Promise<void> {
    if (!desktopReady) {
      return;
    }
    try {
      const threads = await runtimeClient.listThreads();
      dispatch({ type: "run/threadsLoaded", threads });
    } catch {
      // Thread listing is best-effort; runtime reachability is already surfaced
      // by the runtime info poll.
    }
  }

  async function loadThreadsIntoState(): Promise<RuntimeThread[]> {
    const threads = await runtimeClient.listThreads();
    dispatch({ type: "run/threadsLoaded", threads });
    return threads;
  }

  function stopThreadTitlePoll(threadId?: string): void {
    if (threadId) {
      const cancel = threadTitlePollsRef.current.get(threadId);
      cancel?.();
      threadTitlePollsRef.current.delete(threadId);
      return;
    }
    for (const cancel of threadTitlePollsRef.current.values()) {
      cancel();
    }
    threadTitlePollsRef.current.clear();
  }

  function startThreadTitlePoll(threadId: string): void {
    stopThreadTitlePoll(threadId);

    let cancelled = false;
    let activeTimeoutId: ReturnType<typeof setTimeout> | null = null;

    const cancel = () => {
      cancelled = true;
      if (activeTimeoutId !== null) {
        clearTimeout(activeTimeoutId);
        activeTimeoutId = null;
      }
    };

    threadTitlePollsRef.current.set(threadId, cancel);

    const delays = [
      500, 1000, 2000, 4000, 8000, 12000, 20000, 30000, 60000, 90000,
    ];

    async function pollOnce(): Promise<void> {
      for (const delay of delays) {
        if (cancelled) {
          return;
        }
        await new Promise<void>((resolve) => {
          activeTimeoutId = setTimeout(() => {
            activeTimeoutId = null;
            resolve();
          }, delay);
        });
        if (cancelled) {
          return;
        }
        try {
          const threads = await runtimeClient.listThreads();
          if (cancelled) {
            return;
          }
          const thread = threads.find((t) => t.threadId === threadId);
          if (!thread) {
            continue;
          }
          if (
            thread.titleSource === "manual" ||
            (thread.titleStatus === "ready" && thread.titleSource === "llm")
          ) {
            dispatch({ type: "run/threadsLoaded", threads });
            stopThreadTitlePoll(threadId);
            return;
          }
          if (thread.titleStatus === "failed") {
            stopThreadTitlePoll(threadId);
            return;
          }
        } catch {
          // Best-effort polling; transient failures are ignored.
        }
      }
    }

    void pollOnce().finally(() => {
      if (threadTitlePollsRef.current.get(threadId) === cancel) {
        threadTitlePollsRef.current.delete(threadId);
      }
    });
  }
  return {
    refreshThreads,
    loadThreadsIntoState,
    stopThreadTitlePoll,
    startThreadTitlePoll,
  };
}
