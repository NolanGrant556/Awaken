import { useEffect, useRef, useState } from "react";
import {
  expandedFileComposerCompletionKey,
  shouldCollapseExpandedFileComposer,
  type ExpandedFileComposerIdleMode,
  type ExpandedFileComposerTrackedRun,
} from "./ExpandedFileComposer";
import type { InspectorFileTab } from "./inspectorTabs";
import type { useWorkbenchPanels } from "./useWorkbenchPanels";
import type { WorkbenchNavSection } from "./desktopSurfacePresentation";
import {
  ExpandedWorkspaceSurface,
  fileWorkspacePanelActionForState,
} from "./desktopSurfacePresentation";
import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";
import type { ProjectFileWorkspaceTab } from "./projectFileTabs";
import { workspaceLabelForPath } from "./projectWorkspace";
import type { DeliverableFile } from "./run-narrative";
import type { RuntimeViewState } from "./runtime/reducer";
import { useProjectRegistry } from "./useProjectRegistry";
import { useWorkbenchLayout } from "./useWorkbenchLayout";
type Options = {
  inspectorTabs: ReturnType<typeof useWorkbenchPanels>["inspectorTabs"];
  activeNavSection?: WorkbenchNavSection;
  runtimeState: Pick<
    RuntimeViewState,
    | "activeThreadId"
    | "threadId"
    | "runId"
    | "status"
    | "runStartedAtMs"
    | "runCompletedAtMs"
  >;
  registry: Pick<
    ReturnType<typeof useProjectRegistry>,
    | "workspaceLabelProjectId"
    | "backendProjectIdFor"
    | "projectLabelById"
    | "foldersForProjectId"
  >;
  forceFull: boolean;
  isBusy: boolean;
  requestComposerFocus: () => void;
  onBeforeFileOpen: () => void;
  onBeforeFileExpand: () => void;
  onInspectorFileTabClosed?: (viewId: string) => void;
  onFileWorkspaceClosed?: () => void;
  onShowBrowserTab?: (viewId?: string) => void;
};
export function useFileWorkspaceSession(options: Options) {
  const {
    runtimeState,
    activeNavSection,
    isBusy,
    requestComposerFocus,
    onBeforeFileOpen,
    onBeforeFileExpand,
  } = options;
  const {
    workspaceLabelProjectId,
    backendProjectIdFor,
    projectLabelById,
    foldersForProjectId,
  } = options.registry;

  const [fileWorkspaceOpen, setFileWorkspaceOpen] = useState(false);
  const { state: tabsState, dispatch: dispatchTabs } = options.inspectorTabs;
  const selectedTab = tabsState.tabs.find(tab => tab.id === tabsState.activeId);
  const subagentPanelActive = selectedTab?.kind === "subagent";
  const inspectorChooserActive = tabsState.activeId === "chooser";
  const previewTab = tabsState.tabs.find(tab => tab.id === tabsState.previewId);
  const fileWorkspaceMode = previewTab?.kind === "file" ? previewTab.mode : "files";
  function setFileWorkspaceMode(mode: ProjectFileWorkspaceMode): void {
    dispatchTabs({ type: "set-mode", mode });
  }

  const [fileWorkspaceExpanded, setFileWorkspaceExpanded] = useState(false);

  const [expandedWorkspaceSurface, setExpandedWorkspaceSurface] =
    useState<ExpandedWorkspaceSurface>("file");

  const [expandedFileComposerIdleMode, setExpandedFileComposerIdleMode] =
    useState<ExpandedFileComposerIdleMode>("full");

  const [expandedFileComposerTrackedRun, setExpandedFileComposerTrackedRun] =
    useState<ExpandedFileComposerTrackedRun>();

  const deliverablePreview = previewTab?.kind === "artifact" ? previewTab.selection : undefined;

  useEffect(() => {
    dispatchTabs({ type: "project", projectId: workspaceLabelProjectId ? backendProjectIdFor(workspaceLabelProjectId) : undefined });
  }, [workspaceLabelProjectId]);

  const fileWorkspaceExpandedActive =
    fileWorkspaceOpen && fileWorkspaceExpanded && !subagentPanelActive && !inspectorChooserActive;

  const fileWorkspaceSurfaceActive =
    fileWorkspaceExpandedActive && expandedWorkspaceSurface === "file";

  const expandedFileComposerCompletionKeyValue =
    expandedFileComposerCompletionKey({
      trackedRun: expandedFileComposerTrackedRun,
      currentThreadId:
        runtimeState.activeThreadId ?? runtimeState.threadId ?? undefined,
      currentRunId: runtimeState.runId ?? undefined,
      status: runtimeState.status,
      completedAtMs: runtimeState.runCompletedAtMs,
    });

  const expandedFileComposerCompleted = Boolean(
    expandedFileComposerCompletionKeyValue,
  );

  const expandedFileComposerElapsedMs =
    expandedFileComposerCompletionKeyValue &&
    typeof runtimeState.runStartedAtMs === "number" &&
    typeof runtimeState.runCompletedAtMs === "number"
      ? Math.max(0, runtimeState.runCompletedAtMs - runtimeState.runStartedAtMs)
      : undefined;

  const expandedFileComposerForceFull = options.forceFull;

  const workbenchLayout = useWorkbenchLayout(
    fileWorkspaceOpen && !subagentPanelActive && !inspectorChooserActive,
    activeNavSection,
  );

  const {
    sidebarCollapsed,
    setSidebarCollapsed,
    sidebarWidth,
    inspectorWidth,
    fileWorkspaceInspectorWidth,
    setFileWorkspaceInspectorWidth,
    windowWidth,
    inspectorCollapsed,
    setInspectorCollapsed,
    inspectorCollapsedBeforeResponsiveRef,
    inspectorPresentation,
    inspectorCap,
    effectiveInspectorWidth,
    layoutStyle,
    handleSidebarResizeStart,
    handleInspectorResizeStart,
    handleSidebarResizeKeyDown,
    handleInspectorResizeKeyDown,
  } = workbenchLayout;

  const previousThreadIdRef = useRef(runtimeState.activeThreadId);
  const inspectorCollapsedRef = useRef(inspectorCollapsed);
  inspectorCollapsedRef.current = inspectorCollapsed;
  const inspectorCollapsedByThreadRef = useRef(new Map<string, boolean>());
  useEffect(() => {
    const previous = previousThreadIdRef.current;
    const next = runtimeState.activeThreadId;
    if (previous === next) return;
    if (previous) {
      inspectorCollapsedByThreadRef.current.set(
        previous,
        inspectorCollapsedRef.current,
      );
    }
    previousThreadIdRef.current = next;
    if (!previous) {
      if (next) {
        inspectorCollapsedByThreadRef.current.set(
          next,
          inspectorCollapsedRef.current,
        );
      }
      return;
    }
    setFileWorkspaceExpanded(false);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    const nextCollapsed = next
      ? (inspectorCollapsedByThreadRef.current.get(next) ?? true)
      : true;
    setInspectorCollapsed(nextCollapsed);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = nextCollapsed;
    }
  }, [runtimeState.activeThreadId, setInspectorCollapsed]);

  const fileWorkspacePanelAction = fileWorkspacePanelActionForState({
    fileWorkspaceOpen: fileWorkspaceOpen && !subagentPanelActive && !inspectorChooserActive,
    fileWorkspaceExpanded: fileWorkspaceExpandedActive,
    inspectorCollapsed: inspectorPresentation.effectiveCollapsed,
  });

  function handleToggleInspectorCollapsed(): void {
    if (fileWorkspaceExpanded) {
      setFileWorkspaceExpanded(false);
      setExpandedWorkspaceSurface("file");
      setExpandedFileComposerIdleMode("full");
    }
    setInspectorCollapsed((collapsed) => {
      const nextCollapsed = !collapsed;
      if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
        inspectorCollapsedBeforeResponsiveRef.current = nextCollapsed;
      }
      return nextCollapsed;
    });
  }

  function handleOpenFileWorkspace(
    mode: ProjectFileWorkspaceMode = "files",
  ): void {
    onBeforeFileOpen();
    setFileWorkspaceOpen(true);
    setExpandedWorkspaceSurface("file");
    if (selectedTab?.kind !== "file" && selectedTab?.kind !== "artifact") {
      dispatchTabs({ type: "add-file", projectId: fileWorkspaceProjectId, mode });
    } else setFileWorkspaceMode(mode);
    setFileWorkspaceInspectorWidth(undefined);
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
  }

  function handleChooseInspectorEntry(mode: ProjectFileWorkspaceMode = "files"): void {
    handleOpenFileWorkspace(mode);
  }

  function handleExpandFileWorkspace(): void {
    setFileWorkspaceExpanded(true);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    onBeforeFileExpand();
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
  }

  function handleContractFileWorkspace(): void {
    setFileWorkspaceExpanded(false);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    setExpandedFileComposerTrackedRun(undefined);
  }

  function handleShowConversationSurface(): void {
    setExpandedWorkspaceSurface("conversation");
  }

  function handleShowFileWorkspaceSurface(): void {
    onBeforeFileOpen();
    if (fileWorkspaceExpanded) {
      onBeforeFileExpand();
      setExpandedWorkspaceSurface("file");
      setExpandedFileComposerIdleMode("full");
    }
  }

  function handleShowExpandedFileComposerFull(): void {
    if (!isBusy) {
      setExpandedFileComposerTrackedRun(undefined);
    }
    setExpandedFileComposerIdleMode("full");
    requestComposerFocus();
  }

  function handleExpandedFilePreviewPrimaryClick(): void {
    if (
      !shouldCollapseExpandedFileComposer({
        active: fileWorkspaceSurfaceActive,
        forceFull: expandedFileComposerForceFull,
      })
    ) {
      return;
    }
    if (!isBusy) {
      setExpandedFileComposerTrackedRun(undefined);
    }
    setExpandedFileComposerIdleMode("rail");
  }

  function handleCloseFileWorkspace(): void {
    setFileWorkspaceOpen(false);
    setFileWorkspaceExpanded(false);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    setExpandedFileComposerTrackedRun(undefined);
    dispatchTabs({ type: "remove-workspace" });
    setFileWorkspaceInspectorWidth(undefined);
    options.onFileWorkspaceClosed?.();
  }

  function handleOpenProjectFileTab(tab: ProjectFileWorkspaceTab): void {
    dispatchTabs({ type: "open-file", tab });
    handleShowFileWorkspaceSurface();
  }

  function handleCreateProjectFileTab(mode: ProjectFileWorkspaceMode): void {
    onBeforeFileOpen();
    setFileWorkspaceOpen(true);
    setExpandedWorkspaceSurface("file");
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
    dispatchTabs({ type: "add-file", projectId: fileWorkspaceProjectId, mode });
  }

  function handleActivateInspectorTab(id: string): void {
    dispatchTabs({ type: "activate", id });
    handleShowFileWorkspaceSurface();
  }
  function handleActivateProjectFileTab(slot: InspectorFileTab): void {
    handleActivateInspectorTab(slot.id);
  }
  function handleCloseProjectFileTab(id: string): void {
    dispatchTabs({ type: "close", id });
    options.onInspectorFileTabClosed?.(id);
  }
  function handleShowBrowserTab(viewId?: string): void {
    onBeforeFileOpen();
    setFileWorkspaceOpen(true);
    setExpandedWorkspaceSurface("file");
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
    dispatchTabs({ type: "show-browser", projectId: fileWorkspaceProjectId, viewId });
  }

  function handleOpenDeliverablePreview(
    file: Pick<DeliverableFile, "artifactId" | "runId" | "title" | "fileType">,
    threadId: string,
  ): void {
    dispatchTabs({ type: "open-artifact", selection: {
      threadId, runId: file.runId, artifactId: file.artifactId, title: file.title,
      mediaKind: file.fileType === "image" || file.fileType === "video" ? file.fileType : undefined,
    } });
    onBeforeFileOpen();
    setFileWorkspaceOpen(true);
    setExpandedWorkspaceSurface("file");
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) inspectorCollapsedBeforeResponsiveRef.current = false;
  }

  const fileWorkspaceProjectId = workspaceLabelProjectId
    ? backendProjectIdFor(workspaceLabelProjectId)
    : undefined;

  const fileWorkspaceProjectLabel = workspaceLabelProjectId
    ? projectLabelById.get(workspaceLabelProjectId)
    : undefined;

  const fileWorkspaceFolders = workspaceLabelProjectId
    ? foldersForProjectId(workspaceLabelProjectId).map((folder) => ({
        path: folder.path,
        label: workspaceLabelForPath(folder.path),
        isPrimary: folder.isPrimary,
      }))
    : [];

  const activeFileWorkspaceTabSlot = previewTab?.kind === "file" ? previewTab : undefined;
  const activeFileWorkspaceTab = activeFileWorkspaceTabSlot?.tab;
  const visibleFileWorkspaceTabSlots = tabsState.tabs.filter((tab): tab is InspectorFileTab => tab.kind === "file");
  const showFileWorkspacePlaceholder = tabsState.tabs.length === 0;
  const fileWorkspaceTabsAreActive = !subagentPanelActive && !inspectorChooserActive && (!fileWorkspaceExpandedActive || expandedWorkspaceSurface === "file");
  return {
    inspectorTabs: options.inspectorTabs,
    handleActivateInspectorTab,
    inspectorChooserActive,
    handleChooseInspectorEntry,
    fileWorkspaceOpen,
    setFileWorkspaceOpen,
    fileWorkspaceExpanded,
    setFileWorkspaceExpanded,
    expandedWorkspaceSurface,
    setExpandedWorkspaceSurface,
    expandedFileComposerIdleMode,
    setExpandedFileComposerIdleMode,
    expandedFileComposerTrackedRun,
    setExpandedFileComposerTrackedRun,
    fileWorkspaceMode,
    setFileWorkspaceMode,
    deliverablePreview,
    fileWorkspaceExpandedActive,
    fileWorkspaceSurfaceActive,
    expandedFileComposerCompletionKeyValue,
    expandedFileComposerCompleted,
    expandedFileComposerElapsedMs,
    expandedFileComposerForceFull,
    workbenchLayout,
    sidebarCollapsed,
    setSidebarCollapsed,
    sidebarWidth,
    inspectorWidth,
    fileWorkspaceInspectorWidth,
    setFileWorkspaceInspectorWidth,
    windowWidth,
    inspectorCollapsed,
    setInspectorCollapsed,
    inspectorCollapsedBeforeResponsiveRef,
    inspectorPresentation,
    inspectorCap,
    effectiveInspectorWidth,
    layoutStyle,
    handleSidebarResizeStart,
    handleInspectorResizeStart,
    handleSidebarResizeKeyDown,
    handleInspectorResizeKeyDown,
    fileWorkspacePanelAction,
    handleToggleInspectorCollapsed,
    handleOpenFileWorkspace,
    handleExpandFileWorkspace,
    handleContractFileWorkspace,
    handleShowConversationSurface,
    handleShowFileWorkspaceSurface,
    handleShowExpandedFileComposerFull,
    handleExpandedFilePreviewPrimaryClick,
    handleCloseFileWorkspace,
    handleOpenProjectFileTab,
    handleCreateProjectFileTab,
    handleActivateProjectFileTab,
    handleCloseProjectFileTab,
    handleShowBrowserTab,
    handleOpenDeliverablePreview,
    fileWorkspaceProjectId,
    fileWorkspaceProjectLabel,
    fileWorkspaceFolders,
    activeFileWorkspaceTab,
    activeFileWorkspaceTabSlot,
    visibleFileWorkspaceTabSlots,
    showFileWorkspacePlaceholder,
    fileWorkspaceTabsAreActive,
  };
}
