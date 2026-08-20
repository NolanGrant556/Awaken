import { useEffect, useRef, useState } from "react";
import type { RunPlanView } from "./runtime/reducer";

/**
 * Local display delay before the live run-plan entry disappears after its run
 * reaches a terminal state (contract 运行计划与任务进度设计契约 §5.4). The
 * delay is pure Renderer behavior: it is never written to the journal and
 * replay/restart never shows the live entry for an already-terminal run —
 * the terminal history is owned by the narrative `runPlan` item.
 */
export const RUN_PLAN_TERMINAL_HIDE_DELAY_MS = 5000;

export type RunPlanEntryController = {
  /** Whether the compact trigger should render inside the composer dock stack. */
  entryVisible: boolean;
  /** Whether the expanded overlay panel is open. */
  panelOpen: boolean;
  /** Toggle from the trigger: opens, or closes with focus restore. */
  togglePanel: () => void;
  /** Close the panel; `returnFocus` moves focus back to the trigger. */
  closePanel: (returnFocus: boolean) => void;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  panelRef: React.RefObject<HTMLDivElement | null>;
};

/**
 * Owns the live run-plan entry state for the active run (contract §8.3:
 * App.tsx owns open state, trigger ref, overlay composition and focus
 * restore; the scroll controller is untouched because the trigger is a
 * passive layout change inside the measured composer dock stack).
 *
 * Visibility rules (contract §5.2/§5.4):
 * - the entry appears once the active run has a plan snapshot and the run is
 *   live (first successful update_plan);
 * - a fully completed plan keeps showing `M/M · 计划已完成` until run
 *   terminal;
 * - after a live-observed terminal the entry lingers
 *   RUN_PLAN_TERMINAL_HIDE_DELAY_MS and then hides;
 * - a terminal plan that was never observed live (thread reopen, transcript
 *   replay, restart) never shows the live entry.
 */
export function useRunPlanEntry({
  threadId,
  runId,
  plan,
  isBusy,
}: {
  threadId?: string;
  runId?: string;
  plan?: RunPlanView;
  isBusy: boolean;
}): RunPlanEntryController {
  const [panelOpen, setPanelOpen] = useState(false);
  const [entryHidden, setEntryHidden] = useState(false);
  const observedLiveRef = useRef(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  // Run-scoped and thread-scoped reset: a new run or a thread switch must not
  // inherit the previous entry's open state, hide timer or live observation.
  useEffect(() => {
    observedLiveRef.current = false;
    setEntryHidden(false);
    setPanelOpen(false);
  }, [threadId, runId]);

  // Only a plan seen while its run is live counts as live-observed; replayed
  // or restored terminal snapshots never pass through a busy phase.
  useEffect(() => {
    if (plan && isBusy) {
      observedLiveRef.current = true;
    }
  }, [plan, isBusy]);

  // Terminal grace period: hide the live entry 5s after a live-observed run
  // reaches terminal. Pure local setTimeout — nothing is journaled.
  useEffect(() => {
    if (!plan || isBusy || !observedLiveRef.current) {
      return;
    }
    const timer = window.setTimeout(() => {
      observedLiveRef.current = false;
      setEntryHidden(true);
      setPanelOpen(false);
    }, RUN_PLAN_TERMINAL_HIDE_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, [plan, isBusy]);

  const entryVisible =
    Boolean(plan) && !entryHidden && (isBusy || observedLiveRef.current);

  // The panel cannot outlive its entry (plan reset, terminal hide).
  useEffect(() => {
    if (!entryVisible) {
      setPanelOpen(false);
    }
  }, [entryVisible]);

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
    triggerRef,
    panelRef,
  };
}
