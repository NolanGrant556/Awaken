import { useEffect, useRef, useState } from "react";
import {
  MediaArtifactPreviewItem,
  MediaArtifactPreviewState,
} from "./artifactMediaPresentation";
import { useRunPlanEntry } from "./useRunPlanEntry";

export function useWorkbenchPanels(
  activeThreadId: string | undefined,
  runPlanEntry: Pick<ReturnType<typeof useRunPlanEntry>, "closePanel">,
  fallbackSubagentTaskId: () => string | undefined,
  onThreadReset: () => void,
) {
  const runtimeState = { activeThreadId };

  const [mediaPreview, setMediaPreview] =
    useState<MediaArtifactPreviewState | null>(null);

  const [contextUsagePanelOpen, setContextUsagePanelOpen] = useState(false);

  const contextUsageTriggerRef = useRef<HTMLButtonElement>(null);

  const contextUsagePanelRef = useRef<HTMLDivElement>(null);

  const [selectedSubagentTaskId, setSelectedSubagentTaskId] = useState<
    string | undefined
  >();

  const [openSubagentTaskId, setOpenSubagentTaskId] = useState<
    string | undefined
  >();

  useEffect(() => {
    setMediaPreview(null);
  }, [runtimeState.activeThreadId]);

  function closeContextUsagePanel(returnFocus: boolean): void {
    setContextUsagePanelOpen(false);
    if (returnFocus && typeof window !== "undefined") {
      window.requestAnimationFrame(() =>
        contextUsageTriggerRef.current?.focus(),
      );
    }
  }

  useEffect(() => {
    if (!contextUsagePanelOpen) {
      return;
    }
    function handlePointerDown(event: MouseEvent): void {
      const target = event.target as Node;
      if (
        !contextUsagePanelRef.current?.contains(target) &&
        !contextUsageTriggerRef.current?.contains(target)
      ) {
        closeContextUsagePanel(true);
      }
    }
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        event.preventDefault();
        closeContextUsagePanel(true);
      }
    }
    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [contextUsagePanelOpen]);

  useEffect(() => {
    setContextUsagePanelOpen(false);
    setSelectedSubagentTaskId(undefined);
    setOpenSubagentTaskId(undefined);
    onThreadReset();
  }, [runtimeState.activeThreadId]);

  function handleOpenSubagent(taskId?: string): void {
    setContextUsagePanelOpen(false);
    runPlanEntry.closePanel(false);
    setOpenSubagentTaskId(
      taskId ?? openSubagentTaskId ?? fallbackSubagentTaskId(),
    );
  }

  function openMediaPreview(
    items: MediaArtifactPreviewItem[],
    activeId: string,
  ): void {
    if (items.length === 0) return;
    setMediaPreview({ items, activeId });
    setContextUsagePanelOpen(false);
    setOpenSubagentTaskId(undefined);
    runPlanEntry.closePanel(false);
  }
  return {
    contextUsagePanelOpen,
    setContextUsagePanelOpen,
    contextUsageTriggerRef,
    contextUsagePanelRef,
    mediaPreview,
    setMediaPreview,
    selectedSubagentTaskId,
    setSelectedSubagentTaskId,
    openSubagentTaskId,
    setOpenSubagentTaskId,
    closeContextUsagePanel,
    openMediaPreview,
    handleOpenSubagent,
  };
}
