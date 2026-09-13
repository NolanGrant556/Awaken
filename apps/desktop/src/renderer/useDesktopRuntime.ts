import { useCallback, useEffect, useRef, useState } from "react";
import {
  isDesktopRuntimeAvailable,
  type RuntimeInfo,
  type ModelProvider,
  type RuntimeModelProfileSummary,
} from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeViewState, RuntimeAction } from "./runtime/reducer";

export function useDesktopRuntime(
  runtimeClient: Pick<RuntimeClient,
    "getRuntimeInfo" | "listModelProfiles" | "listModelProviders" |
    "getSupervisorStatus" | "onSupervisorStatusChange" | "notifyRendererReady">,
  supervisorStatus: RuntimeViewState["supervisorStatus"],
  dispatch: (action: RuntimeAction) => void,
) {

  const rendererReadyNotifiedRef = useRef(false);

  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);

  const [modelProfiles, setModelProfiles] = useState<
    RuntimeModelProfileSummary[]
  >([]);

  const [modelProviders, setModelProviders] = useState<ModelProvider[]>([]);

  const desktopReady =
    isDesktopRuntimeAvailable(supervisorStatus) && runtimeInfo !== null;

  const desktopNeedsRestart = supervisorStatus === "crashed";

  const desktopStarting = !desktopReady && !desktopNeedsRestart;

  const [runtimeRefreshError, setRuntimeRefreshError] = useState<string | null>(null);
  const generationRef = useRef(0);
  const inFlightRef = useRef<Promise<void> | null>(null);

  const refreshRuntimeInfoNow = useCallback((): Promise<void> => {
    if (!isDesktopRuntimeAvailable(supervisorStatus)) return Promise.resolve();
    if (inFlightRef.current) return inFlightRef.current;
    const generation = generationRef.current;
    const request = Promise.allSettled([
      runtimeClient.getRuntimeInfo(),
      runtimeClient.listModelProfiles(),
      runtimeClient.listModelProviders(),
    ]).then(([info, profiles, providers]) => {
      if (generation !== generationRef.current) return;
      if (info.status === "fulfilled") setRuntimeInfo(info.value);
      if (profiles.status === "fulfilled") setModelProfiles(profiles.value);
      if (providers.status === "fulfilled") setModelProviders(providers.value);
      setRuntimeRefreshError(
        [info, profiles, providers].some((result) => result.status === "rejected")
          ? "运行信息暂时无法刷新，请稍后重试。"
          : null,
      );
    }).finally(() => {
      if (inFlightRef.current === request) inFlightRef.current = null;
    });
    inFlightRef.current = request;
    return request;
  }, [runtimeClient, supervisorStatus]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    if (!isDesktopRuntimeAvailable(supervisorStatus)) {
      setRuntimeInfo(null);
      setModelProfiles([]);
      setModelProviders([]);
      setRuntimeRefreshError(null);
    } else {
      const poll = async () => {
        await refreshRuntimeInfoNow();
        if (!cancelled) timer = setTimeout(() => { void poll(); }, 3000);
      };
      void poll();
    }
    return () => {
      cancelled = true;
      generationRef.current += 1;
      inFlightRef.current = null;
      clearTimeout(timer);
    };
  }, [refreshRuntimeInfoNow, supervisorStatus]);

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
  }, [runtimeClient, dispatch]);

  useEffect(() => {
    if (
      rendererReadyNotifiedRef.current ||
      (!desktopReady && !desktopNeedsRestart)
    ) {
      return;
    }

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
        rendererReadyNotifiedRef.current = true;
        runtimeClient.notifyRendererReady();
      }
    }

    void notifyWhenStable();

    return () => {
      cancelled = true;
    };
  }, [desktopNeedsRestart, desktopReady, runtimeClient]);

  return {
    runtimeInfo,
    runtimeRefreshError,
    modelProfiles,
    modelProviders,
    desktopReady,
    desktopNeedsRestart,
    desktopStarting,
    refreshRuntimeInfoNow,
  };
}
