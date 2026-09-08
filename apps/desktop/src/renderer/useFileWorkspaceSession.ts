import { useEffect, useReducer, useState } from "react";
import {
  expandedFileComposerCompletionKey,
  shouldCollapseExpandedFileComposer,
  type ExpandedFileComposerIdleMode,
  type ExpandedFileComposerTrackedRun,
} from "./ExpandedFileComposer";
import {
  activeArtifactPreviewTab,
  artifactPreviewTabsReducer,
  createArtifactPreviewTabsState,
  type ArtifactPreviewTab,
} from "./artifactPreviewTabs";
import type { WorkbenchNavSection } from "./desktopSurfacePresentation";
import {
  ExpandedWorkspaceSurface,
  fileWorkspacePanelActionForState,
} from "./desktopSurfacePresentation";
import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";
import {
  activeProjectFileTab,
  activeProjectFileTabSlot,
  createProjectFileTabsState,
  projectFileTabsReducer,
  type ProjectFileTabSlot,
  type ProjectFileWorkspaceTab,
} from "./projectFileTabs";
import { workspaceLabelForPath } from "./projectWorkspace";
import type { DeliverableFile } from "./run-narrative";
import type { RuntimeViewState } from "./runtime/reducer";
import { useProjectRegistry } from "./useProjectRegistry";
import { useWorkbenchLayout } from "./useWorkbenchLayout";
type Options = {
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
  hasDraft: boolean;
  isBusy: boolean;
  requestComposerFocus: () => void;
  onBeforeFileOpen: () => void;
  onBeforeFileExpand: () => void;
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

  const [fileWorkspaceExpanded, setFileWorkspaceExpanded] = useState(false);

  const [expandedWorkspaceSurface, setExpandedWorkspaceSurface] =
    useState<ExpandedWorkspaceSurface>("file");

  const [expandedFileComposerIdleMode, setExpandedFileComposerIdleMode] =
    useState<ExpandedFileComposerIdleMode>("full");

  const [expandedFileComposerTrackedRun, setExpandedFileComposerTrackedRun] =
    useState<ExpandedFileComposerTrackedRun>();

  const [fileWorkspaceMode, setFileWorkspaceMode] =
    useState<ProjectFileWorkspaceMode>("files");

  const [fileWorkspaceTabsState, dispatchFileWorkspaceTabs] = useReducer(
    projectFileTabsReducer,
    undefined,
    createProjectFileTabsState,
  );

  const [artifactPreviewTabsState, dispatchArtifactPreviewTabs] = useReducer(
    artifactPreviewTabsReducer,
    undefined,
    createArtifactPreviewTabsState,
  );

  const deliverablePreview = activeArtifactPreviewTab(
    artifactPreviewTabsState,
  )?.selection;

  useEffect(() => {
    dispatchFileWorkspaceTabs({ type: "reset" });
  }, [workspaceLabelProjectId]);

  const fileWorkspaceExpandedActive =
    fileWorkspaceOpen && fileWorkspaceExpanded;

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

  const expandedFileComposerHasDraft = options.hasDraft;

  const workbenchLayout = useWorkbenchLayout(
    fileWorkspaceOpen,
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

  const fileWorkspacePanelAction = fileWorkspacePanelActionForState({
    fileWorkspaceOpen,
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
    setFileWorkspaceMode(mode);
    setFileWorkspaceInspectorWidth(undefined);
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
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
    if (fileWorkspaceExpanded) {
      onBeforeFileExpand();
      setExpandedWorkspaceSurface("file");
      setExpandedFileComposerIdleMode("full");
    }
  }

  function handleShowExpandedFileComposerFull(): void {
    setExpandedFileComposerTrackedRun(undefined);
    setExpandedFileComposerIdleMode("full");
    requestComposerFocus();
  }

  function handleExpandedFilePreviewPrimaryClick(): void {
    if (
      !shouldCollapseExpandedFileComposer({
        active: fileWorkspaceSurfaceActive,
        busy: isBusy,
        forceFull: expandedFileComposerForceFull,
        hasDraft: expandedFileComposerHasDraft,
      })
    ) {
      return;
    }
    setExpandedFileComposerTrackedRun(undefined);
    setExpandedFileComposerIdleMode("rail");
  }

  function handleCloseFileWorkspace(): void {
    setFileWorkspaceOpen(false);
    setFileWorkspaceExpanded(false);
    setExpandedWorkspaceSurface("file");
    setExpandedFileComposerIdleMode("full");
    setExpandedFileComposerTrackedRun(undefined);
    dispatchFileWorkspaceTabs({ type: "reset" });
    dispatchArtifactPreviewTabs({ type: "reset" });
    setFileWorkspaceInspectorWidth(undefined);
  }

  function handleOpenProjectFileTab(tab: ProjectFileWorkspaceTab): void {
    dispatchArtifactPreviewTabs({ type: "deactivate" });
    dispatchFileWorkspaceTabs({ type: "open", tab });
    setFileWorkspaceMode(tab.mode);
    handleShowFileWorkspaceSurface();
  }

  function handleAddProjectFileTab(): void {
    if (!fileWorkspaceProjectId) return;
    dispatchArtifactPreviewTabs({ type: "deactivate" });
    dispatchFileWorkspaceTabs({
      type: "add",
      projectId: fileWorkspaceProjectId,
    });
    setFileWorkspaceMode("files");
    handleShowFileWorkspaceSurface();
  }

  function handleActivateProjectFileTab(slot: ProjectFileTabSlot): void {
    dispatchArtifactPreviewTabs({ type: "deactivate" });
    dispatchFileWorkspaceTabs({ type: "activate", tabId: slot.id });
    setFileWorkspaceMode(slot.tab?.mode ?? "files");
    handleShowFileWorkspaceSurface();
  }

  function handleCloseProjectFileTab(tabId: string): void {
    const nextState = projectFileTabsReducer(fileWorkspaceTabsState, {
      type: "close",
      tabId,
    });
    dispatchFileWorkspaceTabs({ type: "close", tabId });
    const nextProjectSlot = activeProjectFileTabSlot(nextState);
    if (nextProjectSlot) {
      setFileWorkspaceMode(nextProjectSlot.tab?.mode ?? "files");
      return;
    }
    const previousArtifactTab = artifactPreviewTabsState.tabs.at(-1);
    if (previousArtifactTab) {
      dispatchArtifactPreviewTabs({
        type: "activate",
        tabId: previousArtifactTab.id,
      });
    }
    setFileWorkspaceMode("files");
  }

  function handleActivateArtifactPreviewTab(tab: ArtifactPreviewTab): void {
    dispatchArtifactPreviewTabs({ type: "activate", tabId: tab.id });
    setFileWorkspaceMode("files");
    handleShowFileWorkspaceSurface();
  }

  function handleCloseArtifactPreviewTab(tabId: string): void {
    const closingIndex = artifactPreviewTabsState.tabs.findIndex(
      (tab) => tab.id === tabId,
    );
    const closingActive = artifactPreviewTabsState.activeTabId === tabId;
    const hasRightArtifact =
      closingIndex >= 0 &&
      Boolean(artifactPreviewTabsState.tabs[closingIndex + 1]);
    const firstProjectSlot = fileWorkspaceTabsState.tabs.find(
      (slot) => slot.projectId === fileWorkspaceProjectId,
    );
    const nextState = artifactPreviewTabsReducer(artifactPreviewTabsState, {
      type: "close",
      tabId,
    });
    dispatchArtifactPreviewTabs({ type: "close", tabId });
    if (closingActive && !hasRightArtifact && firstProjectSlot) {
      dispatchArtifactPreviewTabs({ type: "deactivate" });
      dispatchFileWorkspaceTabs({
        type: "activate",
        tabId: firstProjectSlot.id,
      });
      setFileWorkspaceMode(firstProjectSlot.tab?.mode ?? "files");
      return;
    }
    if (!activeArtifactPreviewTab(nextState)) {
      setFileWorkspaceMode(
        activeProjectFileTab(fileWorkspaceTabsState)?.mode ?? "files",
      );
    }
  }

  function handleOpenDeliverablePreview(
    file: Pick<DeliverableFile, "artifactId" | "runId" | "title" | "fileType">,
    threadId: string,
  ): void {
    dispatchArtifactPreviewTabs({
      type: "open",
      selection: {
        threadId,
        runId: file.runId,
        artifactId: file.artifactId,
        title: file.title,
        mediaKind:
          file.fileType === "image" || file.fileType === "video"
            ? file.fileType
            : undefined,
      },
    });
    handleOpenFileWorkspace("files");
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

  const activeFileWorkspaceTabSlotCandidate = activeProjectFileTabSlot(
    fileWorkspaceTabsState,
  );

  const activeFileWorkspaceTabCandidate = activeProjectFileTab(
    fileWorkspaceTabsState,
  );

  const activeFileWorkspaceTab =
    activeFileWorkspaceTabCandidate?.projectId === fileWorkspaceProjectId
      ? activeFileWorkspaceTabCandidate
      : undefined;

  const activeFileWorkspaceTabSlot =
    activeFileWorkspaceTabSlotCandidate?.projectId === fileWorkspaceProjectId
      ? activeFileWorkspaceTabSlotCandidate
      : undefined;

  const visibleFileWorkspaceTabSlots = fileWorkspaceTabsState.tabs.filter(
    (slot) => slot.projectId === fileWorkspaceProjectId,
  );

  const showFileWorkspacePlaceholder =
    artifactPreviewTabsState.tabs.length === 0 &&
    visibleFileWorkspaceTabSlots.length === 0;

  const fileWorkspaceTabsAreActive =
    !fileWorkspaceExpandedActive || expandedWorkspaceSurface === "file";
  useEffect(() => {
    dispatchArtifactPreviewTabs({ type: "reset" });
  }, [runtimeState.activeThreadId]);
  return {
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
    fileWorkspaceTabsState,
    dispatchFileWorkspaceTabs,
    artifactPreviewTabsState,
    dispatchArtifactPreviewTabs,
    deliverablePreview,
    fileWorkspaceExpandedActive,
    fileWorkspaceSurfaceActive,
    expandedFileComposerCompletionKeyValue,
    expandedFileComposerCompleted,
    expandedFileComposerElapsedMs,
    expandedFileComposerForceFull,
    expandedFileComposerHasDraft,
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
    handleAddProjectFileTab,
    handleActivateProjectFileTab,
    handleCloseProjectFileTab,
    handleActivateArtifactPreviewTab,
    handleCloseArtifactPreviewTab,
    handleOpenDeliverablePreview,
    fileWorkspaceProjectId,
    fileWorkspaceProjectLabel,
    fileWorkspaceFolders,
    activeFileWorkspaceTabSlotCandidate,
    activeFileWorkspaceTabCandidate,
    activeFileWorkspaceTab,
    activeFileWorkspaceTabSlot,
    visibleFileWorkspaceTabSlots,
    showFileWorkspacePlaceholder,
    fileWorkspaceTabsAreActive,
  };
}
