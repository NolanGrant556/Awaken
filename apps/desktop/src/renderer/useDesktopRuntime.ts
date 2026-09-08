import { useEffect, useRef, useState } from "react";
import type {
  RuntimeInfo,
  ModelProvider,
  RuntimeModelProfileSummary,
} from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeViewState, RuntimeAction } from "./runtime/reducer";

export function useDesktopRuntime(
  runtimeClient: RuntimeClient,
  supervisorStatus: RuntimeViewState["supervisorStatus"],
  dispatch: (action: RuntimeAction) => void,
) {
  const runtimeState = { supervisorStatus };

  const rendererReadyNotifiedRef = useRef(false);

  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);

  const [modelProfiles, setModelProfiles] = useState<
    RuntimeModelProfileSummary[]
  >([]);

  const [modelProviders, setModelProviders] = useState<ModelProvider[]>([]);

  const desktopReady =
    runtimeState.supervisorStatus === "running" && runtimeInfo !== null;

  const desktopNeedsRestart = runtimeState.supervisorStatus === "crashed";

  const desktopStarting = !desktopReady && !desktopNeedsRestart;

  useEffect(() => {
    let cancelled = false;

    async function poll(): Promise<void> {
      if (runtimeState.supervisorStatus !== "running") {
        setRuntimeInfo(null);
        setModelProfiles([]);
        setModelProviders([]);
        return;
      }
      try {
        const [info, profiles, providers] = await Promise.all([
          runtimeClient.getRuntimeInfo(),
          runtimeClient.listModelProfiles().catch(() => []),
          runtimeClient.listModelProviders().catch(() => []),
        ]);
        if (!cancelled) {
          setRuntimeInfo(info);
          setModelProfiles(profiles);
          setModelProviders(providers);
        }
      } catch {
        if (!cancelled) {
          setRuntimeInfo(null);
          setModelProfiles([]);
          setModelProviders([]);
        }
      }
    }

    void poll();
    const interval = setInterval(() => {
      void poll();
    }, 3000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [runtimeClient, runtimeState.supervisorStatus]);

  useEffect(() => {
    let cancelled = false;

    async function initSupervisor(): Promise<void> {
      try {
        const info = await runtimeClient.getSupervisorStatus();
        if (!cancelled) {
          dispatch({ type: "supervisor/status-changed", info });
        }
      } catch {
        // best-effort: supervisor may not be available yet
      }
    }

    void initSupervisor();

    const unsubscribeStatus = runtimeClient.onSupervisorStatusChange((info) => {
      if (!cancelled) {
        dispatch({ type: "supervisor/status-changed", info });
      }
    });

    return () => {
      cancelled = true;
      unsubscribeStatus();
    };
  }, [runtimeClient]);

  useEffect(() => {
    if (
      rendererReadyNotifiedRef.current ||
      (!desktopReady && !desktopNeedsRestart)
    ) {
      return;
    }

    rendererReadyNotifiedRef.current = true;
    let cancelled = false;

    async function notifyWhenStable(): Promise<void> {
      try {
        await document.fonts?.ready;
      } catch {
        // Font readiness is a polish signal only; rendering should continue.
      }
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
      if (!cancelled) {
        runtimeClient.notifyRendererReady();
      }
    }

    void notifyWhenStable();

    return () => {
      cancelled = true;
    };
  }, [desktopNeedsRestart, desktopReady, runtimeClient]);

  async function refreshRuntimeInfoNow(): Promise<void> {
    if (runtimeState.supervisorStatus !== "running") {
      setRuntimeInfo(null);
      setModelProfiles([]);
      setModelProviders([]);
      return;
    }
    try {
      const [info, profiles, providers] = await Promise.all([
        runtimeClient.getRuntimeInfo(),
        runtimeClient.listModelProfiles().catch(() => []),
        runtimeClient.listModelProviders().catch(() => []),
      ]);
      setRuntimeInfo(info);
      setModelProfiles(profiles);
      setModelProviders(providers);
    } catch {
      setRuntimeInfo(null);
      setModelProfiles([]);
      setModelProviders([]);
    }
  }
  return {
    runtimeInfo,
    modelProfiles,
    modelProviders,
    desktopReady,
    desktopNeedsRestart,
    desktopStarting,
    refreshRuntimeInfoNow,
  };
}
