import { useEffect, useRef, useState } from "react";
import type { RuntimeThreadTranscript } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeAction, RuntimeViewState } from "./runtime/reducer";
import { ThreadSession } from "./runtime/thread-session";

type Options = {
  client: RuntimeClient;
  ready: boolean;
  state: RuntimeViewState;
  dispatch: (action: RuntimeAction) => void;
  onTranscript: (
    transcript: RuntimeThreadTranscript,
    isCurrent: () => boolean,
  ) => void;
  onError: (message: string) => void;
};

export function useThreadSession(options: Options): ThreadSession {
  const latest = useRef(options);
  latest.current = options;
  const [session] = useState(
    () =>
      new ThreadSession({
        client: options.client,
        dispatch: options.dispatch,
        storage: window.localStorage,
        onTranscript: (transcript, isCurrent) =>
          latest.current.onTranscript(transcript, isCurrent),
      }),
  );

  useEffect(
    () => () => {
      session.dispose();
    },
    [session],
  );

  useEffect(() => {
    if (!options.ready) return;
    const runId = window.localStorage.getItem("awaken:runId");
    if (!runId) return;
    let disposed = false;
    let generation = session.currentGeneration;
    void options.client
      .getRun(runId)
      .then(async (run) => {
        if (disposed || !session.isCurrent(generation)) return;
        generation = session.begin(run.threadId);
        await session.replay(run.threadId, generation);
      })
      .catch((error) => {
        if (!disposed && session.isCurrent(generation)) {
          latest.current.onError(
            error instanceof Error
              ? error.message
              : "无法恢复任务，请重新选择会话。",
          );
        }
      });
    return () => {
      disposed = true;
    };
  }, [options.ready, options.client, session]);

  useEffect(() => {
    if (!options.ready) return;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const reconcile = async () => {
      const generation = session.currentGeneration;
      try {
        const threads = await options.client.listThreads();
        if (disposed || !session.isCurrent(generation)) return;
        const { state, dispatch } = latest.current;
        dispatch({ type: "run/threadsLoaded", threads });
        const selected = threads.find(
          (thread) => thread.threadId === state.activeThreadId,
        );
        if (
          selected &&
          state.runId &&
          (selected.runId !== state.runId ||
            state.phase === "reconnecting" ||
            (!selected.activeRunId &&
              (state.status === "running" || state.status === "cancelling")))
        ) {
          await session.replay(selected.threadId);
        }
      } catch {
        // Preserve the last view during transient outages; the next pass and
        // stream reconnect retry independently, without inventing a terminal.
      } finally {
        if (!disposed)
          timer = setTimeout(() => {
            void reconcile();
          }, 3000);
      }
    };
    // Do not race first-render recovery with a second initial transcript load.
    timer = setTimeout(() => {
      void reconcile();
    }, 3000);
    return () => {
      disposed = true;
      if (timer) clearTimeout(timer);
    };
  }, [options.ready, options.client, session]);

  return session;
}
