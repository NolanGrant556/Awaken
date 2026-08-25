import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { RunPlanView, RuntimePhase } from "./runtime/reducer";

export const RUN_PLAN_COMPLETED_GRACE_MS = 5_000;

export type RunPlanEntryController = {
  /** Whether the compact trigger should render inside the composer dock stack. */
  entryVisible: boolean;
  /** Whether the expanded overlay panel is open. */
  panelOpen: boolean;
  /** Toggle from the trigger: opens, or closes with focus restore. */
  togglePanel: () => void;
  /** Close the panel; `returnFocus` moves focus back to the trigger. */
  closePanel: (returnFocus: boolean) => void;
  /** Hide the current completed snapshot until a newer version replaces it. */
  dismissCompletedEntry: () => void;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  panelRef: React.RefObject<HTMLDivElement | null>;
};

/**
 * Owns the task-plan entry state for the active thread (contract §8.3:
 * App.tsx owns open state, trigger ref, overlay composition and focus
 * restore; the scroll controller is untouched because the trigger is a
 * passive layout change inside the measured composer dock stack).
 *
 * The entry is the sole live task-plan projection. Durable snapshots remain in
 * runtime state for replay and audit, while this hook owns whether the newest
 * snapshot still belongs above the active composer: incomplete plans survive
 * pause/error/continuation, completed plans stay visible until a successful
 * terminal event and then receive a short grace period before disappearing.
 * A newer plan version always becomes visible again. Plan history never moves
 * into the narrative feed.
 */
export function useRunPlanEntry({
  threadId,
  plan,
  currentRunId,
  phase,
  runStartedAtMs,
  runCompletedAtMs,
}: {
  threadId?: string;
  plan?: RunPlanView;
  currentRunId?: string;
  phase: RuntimePhase;
  runStartedAtMs?: number;
  runCompletedAtMs?: number;
}): RunPlanEntryController {
  const [panelOpen, setPanelOpen] = useState(false);
  const [hiddenCompletedPlanKey, setHiddenCompletedPlanKey] = useState<string>();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  const planKey =
    threadId && plan
      ? `${threadId}:${plan.runId}:${plan.planId}:${plan.version}`
      : undefined;
  const completedPlanBelongsToOlderRun = Boolean(
    plan?.allCompleted &&
      ((currentRunId && plan.runId !== currentRunId) ||
        (!currentRunId && phase === "starting") ||
        (!currentRunId &&
          runStartedAtMs !== undefined &&
          (plan.updatedAtMs ?? 0) < runStartedAtMs)),
  );
  const entryVisible = Boolean(
    planKey &&
      hiddenCompletedPlanKey !== planKey &&
      !completedPlanBelongsToOlderRun,
  );

  // The plan itself is thread-scoped, but an overlay opened in one thread must
  // never remain open after switching to another thread.
  useEffect(() => {
    setPanelOpen(false);
  }, [threadId]);

  // The panel cannot outlive its entry (for example when switching to a
  // thread that has never created a plan or after completion grace expires).
  useEffect(() => {
    if (!entryVisible) {
      setPanelOpen(false);
    }
  }, [entryVisible]);

  const dismissCompletedEntry = useCallback((): void => {
    if (!planKey || !plan?.allCompleted) {
      return;
    }
    setHiddenCompletedPlanKey(planKey);
    setPanelOpen(false);
  }, [plan?.allCompleted, planKey]);

  // A completed plan remains visible while the agent is still producing the
  // final answer. Only a successful terminal state starts the five-second
  // completion grace. `useLayoutEffect` makes an already-expired replay hide
  // before paint instead of flashing an old M/M entry during thread restore.
  useLayoutEffect(() => {
    const isCurrentCompletedTerminalPlan = Boolean(
      planKey &&
        plan?.allCompleted &&
        phase === "done" &&
        (!currentRunId || plan.runId === currentRunId),
    );
    if (
      !planKey ||
      !isCurrentCompletedTerminalPlan ||
      hiddenCompletedPlanKey === planKey
    ) {
      return;
    }

    const completedPresentationStartedAtMs = Math.max(
      plan?.updatedAtMs ?? 0,
      runCompletedAtMs ?? 0,
    );
    const remainingMs =
      completedPresentationStartedAtMs > 0
        ? completedPresentationStartedAtMs + RUN_PLAN_COMPLETED_GRACE_MS - Date.now()
        : RUN_PLAN_COMPLETED_GRACE_MS;

    if (remainingMs <= 0) {
      setHiddenCompletedPlanKey(planKey);
      setPanelOpen(false);
      return;
    }

    const timeoutId = window.setTimeout(() => {
      setHiddenCompletedPlanKey(planKey);
      setPanelOpen(false);
    }, remainingMs);
    return () => window.clearTimeout(timeoutId);
  }, [
    currentRunId,
    hiddenCompletedPlanKey,
    phase,
    plan?.allCompleted,
    plan?.runId,
    plan?.updatedAtMs,
    planKey,
    runCompletedAtMs,
  ]);

  function closePanel(returnFocus: boolean): void {
    setPanelOpen(false);
    if (returnFocus && typeof window !== "undefined") {
      window.requestAnimationFrame(() => triggerRef.current?.focus());
    }
  }

  function togglePanel(): void {
    if (panelOpen) {
      closePanel(true);
    } else {
      setPanelOpen(true);
    }
  }

  // Escape and outside-pointer close, mirroring the context usage panel;
  // closing always restores focus to the trigger (contract §5.3/§5.5).
  useEffect(() => {
    if (!panelOpen) {
      return;
    }
    function handlePointerDown(event: MouseEvent): void {
      const target = event.target as Node;
      if (
        !panelRef.current?.contains(target) &&
        !triggerRef.current?.contains(target)
      ) {
        closePanel(true);
      }
    }
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        event.preventDefault();
        closePanel(true);
      }
    }
    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [panelOpen]);

  return {
    entryVisible,
    panelOpen,
    togglePanel,
    closePanel,
    dismissCompletedEntry,
    triggerRef,
    panelRef,
  };
}
