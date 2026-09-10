import { useEffect, useReducer, useRef, useState } from "react";
import {
  MediaArtifactPreviewItem,
  MediaArtifactPreviewState,
} from "./artifactMediaPresentation";
import { useRunPlanEntry } from "./useRunPlanEntry";
import { createInspectorTabs, inspectorTabsForThread, inspectorTabsReducer, subagentTabId, type InspectorTabsAction, type InspectorTabsCommand } from "./inspectorTabs";

export function useWorkbenchPanels(
  activeThreadId: string | undefined,
  runPlanEntry: Pick<ReturnType<typeof useRunPlanEntry>, "closePanel">,
  onThreadReset: () => void,
) {
  const runtimeState = { activeThreadId };

  const [mediaPreview, setMediaPreview] =
    useState<MediaArtifactPreviewState | null>(null);

  const [contextUsagePanelOpen, setContextUsagePanelOpen] = useState(false);

  const contextUsageTriggerRef = useRef<HTMLButtonElement>(null);

  const contextUsagePanelRef = useRef<HTMLDivElement>(null);

  const [tabsState, dispatchTabs] = useReducer(inspectorTabsReducer, activeThreadId, createInspectorTabs);
  const state = inspectorTabsForThread(tabsState, activeThreadId);
  const dispatch = (action: InspectorTabsCommand) => dispatchTabs({ ...action, threadId: activeThreadId } as InspectorTabsAction);
  const inspectorTabs = { state, dispatch };
  const activeTab = state.tabs.find(tab => tab.id === state.activeId);
  const openSubagentTaskId = activeTab?.kind === "subagent" ? activeTab.taskId : undefined;
  const subagentTabs = { taskIds: state.tabs.flatMap(tab => tab.kind === "subagent" ? [tab.taskId] : []) };

  function closeSubagentTab(taskId: string): void {
    dispatchTabs({ type: "close", threadId: activeThreadId, id: subagentTabId(taskId) });
  }
  function resetSubagentTabs(): void {
    dispatchTabs({ type: "remove-subagents", threadId: activeThreadId });
  }

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
    dispatchTabs({ type: "sync-thread", threadId: activeThreadId });
    onThreadReset();
  }, [runtimeState.activeThreadId]);

  function openSubagentTab(taskId: string): void {
    setContextUsagePanelOpen(false);
    runPlanEntry.closePanel(false);
    dispatchTabs({ type: "open-subagent", threadId: activeThreadId, taskId });
  }

  function openMediaPreview(
    items: MediaArtifactPreviewItem[],
    activeId: string,
  ): void {
    if (items.length === 0) return;
    setMediaPreview({ items, activeId });
    setContextUsagePanelOpen(false);
    runPlanEntry.closePanel(false);
  }
  return {
    inspectorTabs,
    contextUsagePanelOpen,
    setContextUsagePanelOpen,
    contextUsageTriggerRef,
    contextUsagePanelRef,
    mediaPreview,
    setMediaPreview,
    subagentTabs,
    openSubagentTaskId,
    closeSubagentTab,
    resetSubagentTabs,
    closeContextUsagePanel,
    openMediaPreview,
    openSubagentTab,
  };
}
