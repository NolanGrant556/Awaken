import { useEffect, useRef, useState } from "react";
import type { RunPlanView } from "./runtime/reducer";

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
 * Owns the task-plan entry state for the active thread (contract §8.3:
 * App.tsx owns open state, trigger ref, overlay composition and focus
 * restore; the scroll controller is untouched because the trigger is a
 * passive layout change inside the measured composer dock stack).
 *
 * The entry is the sole task-plan projection. Once a thread has a durable
 * plan snapshot it remains anchored above the composer through pause,
 * continuation, terminal replay and restart. A later snapshot updates the
 * same surface; plan history never moves into the narrative feed.
 */
export function useRunPlanEntry({
  threadId,
  plan,
}: {
  threadId?: string;
  plan?: RunPlanView;
}): RunPlanEntryController {
  const [panelOpen, setPanelOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  // The plan itself is thread-scoped, but an overlay opened in one thread must
  // never remain open after switching to another thread.
  useEffect(() => {
    setPanelOpen(false);
  }, [threadId]);

  const entryVisible = Boolean(plan);

  // The panel cannot outlive its entry (for example when switching to a
  // thread that has never created a plan).
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
