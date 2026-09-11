import { useRuntimeInteractions } from "./useRuntimeInteractions";
import {
  AwakenAppShell as AppShell,
  AwakenIcon,
  AwakenComposerDock as ComposerDock,
  AwakenRunPlanTrigger as RunPlanTrigger,
  AwakenRunStage as RunStage,
  AwakenTaskWelcome as TaskWelcome,
  AwakenWorkbenchShell as WorkbenchShell,
  type ComposerBarMetaItem,
  type NavItem,
  type PlatformKind,
  type ProjectItem,
} from "@awaken/design-system";
import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import type {
  ComposerToken,
  ModelAccessType,
  ProjectEntryActionRequest,
  ProjectPathComposerTokenPayload,
  ReasoningEffortLevel,
  RuntimePlugin,
  RuntimePluginComponent,
  RuntimeThread,
} from "../shared/runtime";
import { MACOS_WINDOW_CHROME_GEOMETRY } from "../shared/window-chrome";
import { ComposerSurface } from "./ComposerSurface";
import { ComputerHandoffIndicator } from "./ComputerHandoffIndicator";
import { ComputerReviewGate } from "./ComputerReviewGate";
import { DesktopExecutionFeed } from "./DesktopExecutionFeed";
import { DesktopInspectorRail } from "./DesktopInspectorRail";
import { SubagentPanel } from "./SubagentPanel";
import { DesktopSidebar } from "./DesktopSidebar";
import { DesktopStartupStatus } from "./DesktopStartupStatus";
import { DesktopUpdateDialog } from "./DesktopUpdateDialog";
import { DesktopWorkbenchChrome } from "./DesktopWorkbenchChrome";
import {
  ExpandedFileComposer,
  hasExpandedFileComposerDraft,
} from "./ExpandedFileComposer";
import { ExtensionsView, type ExtensionSection } from "./ExtensionsView";
import { ExternalActionGate } from "./ExternalActionGate";
import { ProfileBalancePopover } from "./ProfileBalancePopover";
import { ProjectFilesWorkspace } from "./ProjectFilesWorkspace";
import { ProjectManagementDialogs } from "./ProjectManagementDialogs";
import { PromptRewindDialog } from "./PromptRewindDialog";
import { ScheduledTasksView } from "./ScheduledTasksView";
import { ThreadManagementDialogs } from "./ThreadManagementDialogs";
import { UserQuestionPrompt } from "./UserQuestionPrompt";
import { WorkbenchOverlays } from "./WorkbenchOverlays";
import {
  shouldRenderWorkbenchNarrative,
  shouldShowRunPlanSurface,
} from "./WorkbenchSurfacePolicy";
import {
  artifactMediaSourceKey,
  mediaRevealLabelForPlatform,
} from "./artifactMediaPresentation";
import {
  pluginAgentComposerTokenFromAction,
  pluginCommandComposerTokenFromAction,
  pluginComposerTokenFromAction,
  pluginRuleComposerTokenFromAction,
  pluginSkillComposerTokenFromAction,
} from "./composerSlash";
import {
  composerStructuredDraftLength,
  insertComposerTokenAtOffset,
} from "./composerStructuredDraft";
import {
  contextUsageMeta,
  contextUsagePanelData,
} from "./contextUsagePresentation";
import { selectAllInFocusedEditor } from "./desktop-selection";
import { MAX_SIDEBAR_WIDTH, MIN_SIDEBAR_WIDTH } from "./desktopLayoutConstants";
import { readStoredString, writeStoredValue } from "./desktopStorage";
import {
  composerMetaItemsForFileWorkspaceSurface,
  shouldEnableInspectorResizeHandle,
  shouldUseCenterComposerWithFileWorkspace,
  workbenchNavSectionForNavId,
} from "./desktopSurfacePresentation";
import {
  DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY,
  dreamTaskReceiptEvent,
} from "./dreamReceiptPresentation";
import {
  assignDreamReceiptToThread,
  dreamTasksForThreadReplay,
  parseDreamReceiptThreadAssignments,
  type DreamReceiptThreadAssignments,
} from "./memoryDreamReceipts";
import {
  modelPreferenceChangeForOption,
  modelPreferenceRecoveryKey,
  modelProfileIcon,
  modelProfileLabel,
} from "./modelPreferencePresentation";
import { projectPetActivity } from "./pets/pet-activity-projector";
import { shouldResetActiveThreadForWorkspaceChange } from "./projectRunScope";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  isSameWorkspacePath,
  projectIdForThread,
} from "./projectWorkspace";
import {
  composerTokenEndOffset,
  threadSupportsMessageRewind,
} from "./promptPresentation";
import type { RunNarrativeItem } from "./run-narrative";
import { createRuntimeClient } from "./runtime/client";
import {
  createInitialRuntimeState,
  latestThreadRunPlan,
  reduceRuntimeState,
  threadItemFromRuntime,
  type SubagentTaskView,
  type ThreadItem,
} from "./runtime/reducer";
import { DesktopSettingsDialog } from "./settings/DesktopSettingsDialog";
import { useArtifactMediaSources } from "./useArtifactMediaSources";
import { useComposerInput } from "./useComposerInput";
import { useComposerLifecycle } from "./useComposerLifecycle";
import { useDesktopAppearance } from "./useDesktopAppearance";
import { useDesktopRuntime } from "./useDesktopRuntime";
import { useDesktopUpdate } from "./useDesktopUpdate";
import { useFileWorkspaceSession } from "./useFileWorkspaceSession";
import { useNarrativeRenderer } from "./useNarrativeRenderer";
import { useProjectOperations } from "./useProjectOperations";
import { useProjectRegistry } from "./useProjectRegistry";
import { usePromptRewind } from "./usePromptRewind";
import { useRunPlanEntry } from "./useRunPlanEntry";
import { useRunStageScrollController } from "./useRunStageScrollController";
import { useRunSubmission } from "./useRunSubmission";
import {
  composerDraftOwnerKey,
  useThreadComposerDraft,
} from "./useThreadComposerDraft";
import { useThreadDirectory } from "./useThreadDirectory";
import { useThreadNavigation } from "./useThreadNavigation";
import { useThreadSession } from "./useThreadSession";
import { useWorkbenchPanels } from "./useWorkbenchPanels";
import { rendererWorkbenchDisclosureStore } from "./workbenchDisclosureStore";
import { MIN_INSPECTOR_WIDTH } from "./workbenchLayout";
import {
  projectVisibleNarrativeItems,
  runPlanStepStatusForDisplay,
  shouldUseCenterComposer,
  turnMarkerRailDisplay,
  turnMarkerRailHeight,
  workbenchHasRun,
} from "./workbenchPresentation";

export function App() {
  const [runtimeClient] = useState(createRuntimeClient);
  const [promptReplacementPending, setPromptReplacementPending] =
    useState(false);
  const workbenchDisclosureStore = rendererWorkbenchDisclosureStore;

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const {
    runtimeInfo,
    runtimeRefreshError,
    modelProfiles,
    modelProviders,
    desktopReady,
    desktopNeedsRestart,
    desktopStarting,
    refreshRuntimeInfoNow,
  } = useDesktopRuntime(runtimeClient, runtimeState.supervisorStatus, dispatch);

  const activeThread = runtimeState.activeThreadId
    ? runtimeState.threads.find(
        (thread) => thread.threadId === runtimeState.activeThreadId,
      )
    : undefined;
  const threadDirectory = useThreadDirectory(
    runtimeClient,
    desktopReady,
    dispatch,
  );
  const { refreshThreads, loadThreadsIntoState, startThreadTitlePoll } =
    threadDirectory;

  const activeThreadIdRef = useRef(runtimeState.activeThreadId);
  activeThreadIdRef.current = runtimeState.activeThreadId;
  const dreamReceiptThreadAssignmentsRef =
    useRef<DreamReceiptThreadAssignments>(
      parseDreamReceiptThreadAssignments(
        readStoredString(DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY),
      ),
    );
  const backgroundThreadActivationRef = useRef<(threadId: string) => void>(
    () => {},
  );
  backgroundThreadActivationRef.current = async (threadId: string) => {
    // Refresh the thread authority before selecting: scheduled tasks can create
    // a dedicated thread in the background while other pages are mounted.
    let runtimeThreads: RuntimeThread[] | undefined;
    try {
      runtimeThreads = await loadThreadsIntoState();
    } catch {
      setNotice("无法加载线程列表");
      return;
    }
    const threads = runtimeThreads.map(threadItemFromRuntime);
    // A scheduled-task thread can already be the reducer's active thread while
    // its cached view is still the empty shell created by a background thread
    // list refresh. Force a transcript replay when activation comes from a
    // notification or execution history.
    await handleSelectConversation(threadId, threads, true);
  };

  useEffect(() => {
    const snapshot = projectPetActivity(runtimeState);
    runtimeClient.updatePetActivity(snapshot);
  }, [runtimeState, runtimeClient]);

  useEffect(
    () =>
      runtimeClient.onPetThreadActivationRequested((threadId) => {
        backgroundThreadActivationRef.current(threadId);
      }),
    [runtimeClient],
  );

  useEffect(
    () =>
      runtimeClient.onScheduledTaskThreadActivation((threadId) => {
        backgroundThreadActivationRef.current(threadId);
      }),
    [runtimeClient],
  );
  const desktopUpdate = useDesktopUpdate(runtimeClient);
  const { sidebarUpdateAction } = desktopUpdate;

  const [activeNavId, setActiveNavId] = useState<string | undefined>();

  const activeNavSection = workbenchNavSectionForNavId(activeNavId);

  const activeExtensionSection: ExtensionSection | undefined =
    activeNavSection === "connectors" ||
    activeNavSection === "skills" ||
    activeNavSection === "plugins"
      ? activeNavSection
      : undefined;

  const scheduledTasksNavActive = activeNavSection === "scheduled-tasks";

  const workbenchMediaSurfaceActive =
    !activeExtensionSection && !scheduledTasksNavActive;
  const [extensionNavigationEpoch, setExtensionNavigationEpoch] = useState(0);
  const [notice, setNotice] = useState<string | null>(null);
  useEffect(() => {
    if (runtimeRefreshError) setNotice(runtimeRefreshError);
  }, [runtimeRefreshError]);

  const [resolvingUserQuestionId, setResolvingUserQuestionId] = useState<
    string | null
  >(null);
  const [newThreadModelPreference, setNewThreadModelPreference] = useState<{
    selectedModelProfileId?: string;
    reasoningEffortByProfile: Record<string, ReasoningEffortLevel>;
  }>({ reasoningEffortByProfile: {} });
  const recoveredModelPreferenceRef = useRef<string>("");
  const [modelSettingsOpen, setModelSettingsOpen] = useState(false);
  const [
    modelSettingsInitialModelSection,
    setModelSettingsInitialModelSection,
  ] = useState<ModelAccessType>("coding_plan");
  const {
    appearanceThemePreference,
    appearanceTheme,
    appearanceSaving,
    appearanceMessage,
    handleAppearanceThemeChange,
  } = useDesktopAppearance(runtimeClient);

  const [profilePopoverOpen, setProfilePopoverOpen] = useState(false);
  const profileButtonRef = useRef<HTMLButtonElement>(null);
  const projectRegistry = useProjectRegistry(
    runtimeClient,
    desktopReady,
    runtimeInfo,
    activeThread,
    refreshThreads,
  );
  const {
    workspaceTargetPathRef,
    workspacePath,
    setWorkspacePath,
    projectsFromWorkspace,
    selectedProjectId,
    setSelectedProjectId,
    conversationScope,
    setConversationScope,
    expandedProjectIds,
    setExpandedProjectIds,
    setConversationSectionExpanded,
    projectOrderIds,
    defaultWorkspacePath,
    effectiveDefaultWorkspacePath,
    hasDefaultWorkspaceProject,
    activeThreadWorkspacePath,
    activeThreadProjectId,
    workspaceRootForRun,
    isWorkspaceValid,
    projectLabelById,
    removedProjectIdSet,
    activeProjectLabel,
    syncProjectSelectionForThread,
  } = projectRegistry;
  const projectOperations = useProjectOperations({
    handleNewProjectSession,
    threadsForProject,
    runtimeClient,
    desktopReady,
    registry: projectRegistry,
    runtimeInfo,
    setNotice,
    refreshThreads,
    loadThreadsIntoState,
    resetActiveThreadContext,
    dispatch,
  });
  const {
    createProjectSubmitting,
    projectAction,
    projectHoverCard,
    projectMenuItems,
    handleOpenCreateProject,
  } = projectOperations;
  useEffect(
    () =>
      runtimeClient.onMemoryDreamReceipt((task) => {
        if (task.projectId && task.projectId !== selectedProjectId) return;
        const threadId = activeThreadIdRef.current;
        if (!threadId) return;
        const assignments = assignDreamReceiptToThread(
          dreamReceiptThreadAssignmentsRef.current,
          task.taskId,
          threadId,
        );
        dreamReceiptThreadAssignmentsRef.current = assignments;
        writeStoredValue(
          DREAM_RECEIPT_THREAD_ASSIGNMENTS_STORAGE_KEY,
          JSON.stringify(assignments),
        );
        const event = dreamTaskReceiptEvent(task);
        if (event)
          dispatch({ type: "run/event", event, receivedAtMs: Date.now() });
      }),
    [runtimeClient, selectedProjectId],
  );
  const [cancellingSubagentTaskId, setCancellingSubagentTaskId] = useState<
    string | undefined
  >();
  const [subagentNowMs, setSubagentNowMs] = useState(() => Date.now());
  const composerDraftKey = composerDraftOwnerKey(runtimeState.activeThreadId);
  const composerDraftController = useThreadComposerDraft(composerDraftKey);
  const {
    draft: composerDraft,
    setValue: setComposerValue,
    setTokens: setComposerTokens,
    setTokenPlacements: setComposerTokenPlacements,
    setTokenLeadingTexts: setComposerTokenLeadingTexts,
    setSelection: setComposerInputSelection,
    clearDraft: clearThreadComposerDraft,
  } = composerDraftController;
  const composerValue = composerDraft.value;
  const composerAttachments = composerDraft.attachments;
  const composerTokens = composerDraft.tokens;
  const composerComments = composerDraft.comments;
  const composerTokenPlacements = composerDraft.tokenPlacements;
  const composerTokenLeadingTexts = composerDraft.tokenLeadingTexts;
  const composerInputSelection = composerDraft.selection;
  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "reconnecting" ||
    runtimeState.phase === "cancelling";
  // The task surface follows the newest durable plan across runs in the
  // active thread. Pause, continuation and replay update this one ComposerDock
  // projection; plans never move into the narrative execution process.
  const activeRunPlan = useMemo(
    () => latestThreadRunPlan(runtimeState.runPlansById),
    [runtimeState.runPlansById],
  );
  const runPlanEntry = useRunPlanEntry({
    threadId: runtimeState.activeThreadId,
    plan: activeRunPlan,
    currentRunId: runtimeState.runId,
    phase: runtimeState.phase,
    runStartedAtMs: runtimeState.runStartedAtMs,
    runCompletedAtMs: runtimeState.runCompletedAtMs,
  });
  const runPlanPaused = Boolean(
    activeRunPlan &&
      !activeRunPlan.allCompleted &&
      (runtimeState.phase === "cancelling" || !isBusy),
  );
  const activeRunPlanItems = useMemo(
    () =>
      (activeRunPlan?.items ?? []).map((item) => ({
        step: item.step,
        activeForm: item.activeForm,
        status: runPlanStepStatusForDisplay(item.status),
      })),
    [activeRunPlan],
  );
  const runPlanPanelId = "awaken-run-plan-panel";
  const visibleNarrativeItems = useMemo<RunNarrativeItem[]>(() => {
    const narrativeItems: RunNarrativeItem[] = [
      ...runtimeState.narrative.committedItems,
      ...(runtimeState.narrative.activeItem
        ? [runtimeState.narrative.activeItem]
        : []),
    ];
    return projectVisibleNarrativeItems({ items: narrativeItems, isBusy });
  }, [isBusy, runtimeState.narrative]);
  const artifactMedia = useArtifactMediaSources(
    runtimeClient,
    {
      activeThreadId: runtimeState.activeThreadId,
      threadId: runtimeState.threadId,
      artifacts: runtimeState.artifacts,
    },
    visibleNarrativeItems,
    workbenchMediaSurfaceActive,
    setNotice,
  );
  const {
    artifactMediaSources,
    artifactMediaMetadata,
    openingArtifactId,
    recordArtifactMediaMetadata,
    refreshArtifactMediaSource,
    openArtifactExternally,
    revealArtifact,
  } = artifactMedia;

  const replayedNarrativeItemIds = useMemo(
    () => new Set(runtimeState.narrative.replayedItemIds),
    [runtimeState.narrative.replayedItemIds],
  );
  const workbenchPanels = useWorkbenchPanels(
    runtimeState.activeThreadId,
    runPlanEntry,
    () => setCancellingSubagentTaskId(undefined),
  );
  const {
    contextUsagePanelOpen,
    setContextUsagePanelOpen,
    contextUsageTriggerRef,
    mediaPreview,
    subagentTabs,
    resetSubagentTabs,
    openSubagentTaskId,
    openMediaPreview,
    openSubagentTab,
  } = workbenchPanels;

  const subagentTasks = useMemo<SubagentTaskView[]>(
    () =>
      runtimeState.subagentOrder.flatMap((taskId) => {
        const task = runtimeState.subagentsById[taskId];
        return task ? [task] : [];
      }).sort((a, b) => (a.createdAt ?? "").localeCompare(b.createdAt ?? "") || a.taskId.localeCompare(b.taskId)),
    [runtimeState.subagentOrder, runtimeState.subagentsById],
  );
  const openSubagent = openSubagentTaskId
    ? runtimeState.subagentsById[openSubagentTaskId]
    : undefined;
  const openSubagentTabs = subagentTabs.taskIds.flatMap(taskId => {
    const task = runtimeState.subagentsById[taskId];
    return task ? [task] : [];
  });
  const runningSubagentCount = subagentTasks.filter((task) =>
    ["queued", "running", "cancelling"].includes(task.status),
  ).length;

  useEffect(() => {
    setSubagentNowMs(Date.now());
    if (runningSubagentCount === 0) return undefined;
    const interval = window.setInterval(
      () => setSubagentNowMs(Date.now()),
      1000,
    );
    return () => window.clearInterval(interval);
  }, [runningSubagentCount]);

  useEffect(() => {
    if (!notice) {
      return;
    }

    const timer = window.setTimeout(() => {
      setNotice(null);
    }, 2000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    return window.awaken?.onSelectAllRequested(() => {
      selectAllInFocusedEditor(document);
    });
  }, []);

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    let cancelled = false;
    const reconcileScheduledThreads = async (startedThreadId?: string) => {
      try {
        const threads = await runtimeClient.listThreads();
        if (!cancelled) {
          dispatch({ type: "run/threadsLoaded", threads });
          if (startedThreadId) {
            const startedThread = threads.find(
              (thread) => thread.threadId === startedThreadId,
            );
            if (startedThread) {
              const projectId = startedThread.projectId;
              if (projectId) {
                setExpandedProjectIds((current) =>
                  current.includes(projectId)
                    ? current
                    : [...current, projectId],
                );
              } else {
                setConversationSectionExpanded(true);
              }
            }
          }
        }
      } catch {
        // The runtime reachability surface owns the visible error. Reconnect
        // or the next scheduled event retries this durable reconciliation.
      }
    };
    void reconcileScheduledThreads();
    const unsubscribe = runtimeClient.subscribeScheduledTaskEvents((event) => {
      if (
        (event.type === "execution_started" ||
          event.type === "execution_finished") &&
        event.threadId
      ) {
        void reconcileScheduledThreads(event.threadId);
      }
    });
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [desktopReady, runtimeClient]);

  async function handleApplyModelPreference(
    profileId: string,
    reasoningEffort?: ReasoningEffortLevel,
  ): Promise<void> {
    if (isBusy) return;
    try {
      setNotice(null);
      if (activeThread?.threadId) {
        await runtimeClient.updateThreadModelPreference(activeThread.threadId, {
          modelProfileId: profileId,
          reasoningEffort,
        });
        await refreshThreads();
      } else {
        setNewThreadModelPreference((current) => ({
          selectedModelProfileId: profileId,
          reasoningEffortByProfile: {
            ...current.reasoningEffortByProfile,
            ...(reasoningEffort ? { [profileId]: reasoningEffort } : {}),
          },
        }));
      }
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to switch model",
      );
    }
  }

  function resetActiveThreadContext(): void {
    threadSession.begin();
    setNotice(null);
    setEditingPrompt(undefined);
    setPendingPromptRewind(undefined);
    setRewindDialogHidden(false);
    setMessageActionPendingRunId(undefined);
    setNewThreadModelPreference({ reasoningEffortByProfile: {} });
    localStorage.removeItem("awaken:runId");
    prepareForNewTask();
    dispatch({ type: "run/reset" });
  }

  const enabledModelProfiles = modelProfiles.filter(
    (profile) => profile.enabled,
  );
  const activeThreadSupportsMessageRewind =
    threadSupportsMessageRewind(activeThread);
  const requestedModelProfileId =
    activeThread?.selectedModelProfileId ??
    newThreadModelPreference.selectedModelProfileId;
  const selectedModelProfile =
    enabledModelProfiles.find(
      (profile) => profile.id === requestedModelProfileId,
    ) ??
    enabledModelProfiles.find(
      (profile) => profile.id === runtimeInfo?.modelProfile?.id,
    ) ??
    enabledModelProfiles.find((profile) => profile.isDefault) ??
    enabledModelProfiles[0];
  const selectedModelProvider = selectedModelProfile
    ? modelProviders.find(
        (provider) => provider.id === selectedModelProfile.providerId,
      )
    : undefined;
  const selectedReasoningCapability = selectedModelProfile
    ? (selectedModelProfile.effectiveCapability?.reasoningEffort ??
      selectedModelProvider?.modelCapabilities[selectedModelProfile.modelId]
        ?.reasoningEffort)
    : undefined;
  const persistedThreadReasoningEffort = selectedModelProfile
    ? activeThread?.reasoningEffortByProfile?.[selectedModelProfile.id]
    : undefined;
  const storedReasoningEffort = selectedModelProfile
    ? (persistedThreadReasoningEffort ??
      newThreadModelPreference.reasoningEffortByProfile[
        selectedModelProfile.id
      ] ??
      selectedModelProfile.advanced?.defaultReasoningEffort)
    : undefined;
  const selectedReasoningEffort =
    storedReasoningEffort &&
    selectedReasoningCapability?.levels.includes(storedReasoningEffort)
      ? storedReasoningEffort
      : selectedReasoningCapability?.defaultLevel;
  const selectedReasoningEffortLabel = selectedReasoningEffort
    ? (selectedReasoningCapability?.displayLabels?.[selectedReasoningEffort] ??
      selectedReasoningEffort)
    : undefined;
  const recoveredPreferenceKey = modelPreferenceRecoveryKey(
    activeThread?.threadId,
    activeThread?.selectedModelProfileId,
    selectedModelProfile?.id,
    persistedThreadReasoningEffort,
    selectedReasoningEffort,
  );
  useEffect(() => {
    if (
      !recoveredPreferenceKey ||
      recoveredModelPreferenceRef.current === recoveredPreferenceKey
    ) {
      return;
    }
    recoveredModelPreferenceRef.current = recoveredPreferenceKey;
    setNotice("原会话的模型或 effort 已失效，已恢复为当前可用默认值。");
  }, [recoveredPreferenceKey]);
  const composerLifecycle = useComposerLifecycle({
    composerValue,
    desktopReady,
    isBusy,
    isWorkspaceValid,
    hasAttachments:
      composerAttachments.length > 0 ||
      composerTokens.length > 0 ||
      composerComments.length > 0,
    inputAttachmentBlocked: false,
  });
  const { requestComposerFocus, composerBarState } = composerLifecycle;
  const composerInput = useComposerInput({
    runtimeClient,
    desktopReady,
    isBusy,
    isWorkspaceValid,
    workspaceRootForRun,
    defaultWorkspacePath,
    disabled: composerBarState.disabled,
    draft: composerDraftController,
    requestComposerFocus,
    setNotice,
  });
  const {
    composerPluginCatalog,
    composerPluginBrandByName,
    composerCatalogRevision,
    trimmedEffectiveComposerValue,
    handleCreateFileLineComment,
    handleAddFileTextSelection,
    handleRemoveFileLineComment,
  } = composerInput;

  const activeUserQuestion = runtimeState.userQuestions[0];
  const activeExternalInteraction = runtimeState.externalInteractions[0];
  const activeComputerReview = runtimeState.computerReviews[0];
  const activeComputerHandoff = runtimeState.computerHandoffs.find(
    (item) => item.active,
  );
  const { resolvingExternalInteractionId, resolvingComputerReviewId, controllingComputerHandoffId, resolveActiveExternalInteraction, resolveActiveACPPermission, resolveActiveComputerReview, returnComputerControl } = useRuntimeInteractions({
    runId: runtimeState.runId, activeExternalInteraction, activeComputerReview,
    activeComputerHandoff, runtimeClient, setNotice,
  });
  const navItems: NavItem[] = [
    {
      id: "connectors",
      label: "连接器",
      icon: <AwakenIcon concept="connector" size={20} />,
    },
    {
      id: "skills",
      label: "技能",
      icon: <AwakenIcon concept="skill" size={20} />,
    },
    {
      id: "plugins",
      label: "插件",
      icon: <AwakenIcon concept="plugin" size={20} />,
    },
    {
      id: "scheduled-tasks",
      label: "定时任务",
      icon: <AwakenIcon concept="scheduled-task" size={20} />,
    },
  ];
  const selectedEndpointSelector =
    selectedModelProfile?.advanced?.endpointId?.trim();
  const selectedRuntimeModelIdentity = selectedModelProfile
    ? {
        ...selectedModelProfile,
        modelId: selectedEndpointSelector || selectedModelProfile.modelId,
        capabilityModelId: selectedModelProfile.modelId,
        endpointSelector: selectedEndpointSelector,
        modelSelectorKind: selectedEndpointSelector
          ? ("endpoint" as const)
          : ("model" as const),
        reasoningEffort: selectedReasoningEffort,
      }
    : undefined;
  const contextMeta = contextUsageMeta(
    runtimeState.inspectorSummary.contextWindow,
    selectedRuntimeModelIdentity,
  );
  const contextPanelData = contextUsagePanelData(
    runtimeState.inspectorSummary.contextWindow,
    selectedRuntimeModelIdentity,
  );
  const contextUsagePanelId = "awaken-context-usage-panel";

  useEffect(() => {
    if (!contextPanelData) {
      setContextUsagePanelOpen(false);
    }
  }, [contextPanelData]);
  const modelMetaItem: ComposerBarMetaItem =
    enabledModelProfiles.length > 0 && selectedModelProfile
      ? {
          id: "model",
          label: `${modelProfileLabel(selectedModelProfile)}${selectedReasoningEffortLabel ? ` · ${selectedReasoningEffortLabel}` : ""}`,
          icon: modelProfileIcon(selectedModelProfile),
          disabled: isBusy,
          menuItems: enabledModelProfiles.map((profile) => {
            const provider = modelProviders.find(
              (item) => item.id === profile.providerId,
            );
            const capability =
              profile.effectiveCapability?.reasoningEffort ??
              provider?.modelCapabilities[profile.modelId]?.reasoningEffort;
            const storedEffort =
              activeThread?.reasoningEffortByProfile?.[profile.id] ??
              newThreadModelPreference.reasoningEffortByProfile[profile.id] ??
              profile.advanced?.defaultReasoningEffort;
            const effectiveEffort =
              storedEffort && capability?.levels.includes(storedEffort)
                ? storedEffort
                : capability?.defaultLevel;
            return {
              id: profile.id,
              label: modelProfileLabel(profile),
              icon: modelProfileIcon(profile),
              selected: profile.id === selectedModelProfile.id,
              disabled: isBusy,
              onSelect: () => {
                const change = modelPreferenceChangeForOption(
                  profile.id,
                  selectedModelProfile.id,
                  effectiveEffort,
                );
                if (change) {
                  void handleApplyModelPreference(
                    change.modelProfileId,
                    change.reasoningEffort,
                  );
                }
              },
              effort:
                capability && effectiveEffort
                  ? {
                      levels: capability.levels,
                      selectedLevel: effectiveEffort,
                      displayLabels: capability.displayLabels,
                      note: activeThread?.runId
                        ? "切换模型或推理强度会重建对话缓存，下次发送消息时生效。"
                        : undefined,
                      onChange: (level: ReasoningEffortLevel) => {
                        void handleApplyModelPreference(profile.id, level);
                      },
                    }
                  : undefined,
            };
          }),
          menuFooterAction: {
            label: "添加模型",
            icon: <AwakenIcon concept="settings" size={16} />,
            onClick: () => {
              setModelSettingsInitialModelSection("coding_plan");
              setModelSettingsOpen(true);
            },
          },
        }
      : {
          id: "model",
          label: "请点击头像配置模型",
          disabled: true,
        };
  const composerMetaItems: ComposerBarMetaItem[] = [
    modelMetaItem,
    { id: "workspace", label: activeProjectLabel },
    ...(contextMeta
      ? [
          {
            id: "context",
            ...contextMeta,
            onClick: () => {
              runPlanEntry.closePanel(false);
              setContextUsagePanelOpen((current) => !current);
            },
            buttonRef: contextUsageTriggerRef,
            ariaHaspopup: "dialog" as const,
            ariaExpanded: contextUsagePanelOpen,
            ariaControls: contextUsagePanelId,
          },
        ]
      : []),
  ];
  const threadNavigation = useThreadNavigation({
    dispatch,
    runtimeClient,
    desktopReady,
    runtimeState: {
      threads: runtimeState.threads,
      activeThreadId: runtimeState.activeThreadId,
    },
    directory: threadDirectory,
    effectiveDefaultWorkspacePath,
    setConversationSectionExpanded,
    clearThreadComposerDraft,
    setNotice,
    resetActiveThreadContext,
    threadsForProject,
    onActiveThreadArchived: () => {
      handleCloseFileWorkspace();
      resetSubagentTabs();
    },
  });
  const { threadItemsForProject, exitConversationBatchMode } = threadNavigation;

  const defaultProjectThreads = threadItemsForProject(
    DEFAULT_WORKSPACE_PROJECT_ID,
  );
  const unorderedRepositoryProjects: ProjectItem[] = [
    ...(removedProjectIdSet.has(DEFAULT_WORKSPACE_PROJECT_ID) ||
    !hasDefaultWorkspaceProject
      ? []
      : [
          {
            id: DEFAULT_WORKSPACE_PROJECT_ID,
            kind: "project" as const,
            label:
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
            path: defaultWorkspacePath,
            action: projectAction(
              DEFAULT_WORKSPACE_PROJECT_ID,
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
            ),
            menuItems: projectMenuItems(DEFAULT_WORKSPACE_PROJECT_ID),
            hoverCard: projectHoverCard(
              DEFAULT_WORKSPACE_PROJECT_ID,
              projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
              defaultProjectThreads.length,
            ),
            children:
              defaultProjectThreads.length > 0
                ? defaultProjectThreads
                : undefined,
          },
        ]),
    ...projectsFromWorkspace
      .filter(
        (project) =>
          !isSameWorkspacePath(project.path, effectiveDefaultWorkspacePath),
      )
      .map((project) => {
        const threads = threadItemsForProject(project.id);
        return {
          id: project.id,
          kind: "project" as const,
          label: project.label,
          path: project.path,
          action: projectAction(project.id, project.label),
          menuItems: projectMenuItems(project.id),
          hoverCard: projectHoverCard(
            project.id,
            project.label,
            threads.length,
          ),
          children: threads.length > 0 ? threads : undefined,
        };
      }),
  ];
  const repositoryProjectById = new Map(
    unorderedRepositoryProjects.map((project) => [project.id, project]),
  );
  const orderedRepositoryProjectIds = new Set(projectOrderIds);
  const repositoryProjects: ProjectItem[] = [
    ...projectOrderIds.flatMap((id) => {
      const project = repositoryProjectById.get(id);
      return project ? [project] : [];
    }),
    ...unorderedRepositoryProjects.filter(
      (project) => !orderedRepositoryProjectIds.has(project.id),
    ),
  ];
  const allProjectsExpanded =
    repositoryProjects.length > 0 &&
    repositoryProjects.every((project) =>
      expandedProjectIds.includes(project.id),
    );
  const composerProjectPicker = {
    label: "选择项目",
    placeholder: "选择项目",
    options: [
      {
        id: "conversation",
        label: "选择项目",
        icon: null,
        selected: conversationScope,
        onSelect: handleSelectConversationScope,
      },
      ...repositoryProjects.map((project) => ({
        id: project.id,
        label: project.label,
        selected: !conversationScope && project.id === selectedProjectId,
        disabled: project.disabled,
        onSelect: () => handleSelectProject(project.id),
      })),
    ],
    emptyLabel: "未选择项目时，对话在电脑全局范围中运行。",
    clearAction: conversationScope
      ? undefined
      : {
          label: "取消选择项目",
          onClick: handleSelectConversationScope,
        },
    createAction: {
      label: "创建项目",
      disabled: !desktopReady || createProjectSubmitting,
      onClick: handleOpenCreateProject,
    },
  };
  const activeProjectId = runtimeState.activeThreadId
    ? `thread:${runtimeState.activeThreadId}`
    : selectedProjectId;
  const fileWorkspace = useFileWorkspaceSession({
    inspectorTabs: workbenchPanels.inspectorTabs,
    activeNavSection,
    runtimeState: {
      activeThreadId: runtimeState.activeThreadId,
      threadId: runtimeState.threadId,
      runId: runtimeState.runId,
      status: runtimeState.status,
      runStartedAtMs: runtimeState.runStartedAtMs,
      runCompletedAtMs: runtimeState.runCompletedAtMs,
    },
    registry: projectRegistry,
    forceFull: Boolean(
      desktopNeedsRestart || activeUserQuestion || activeExternalInteraction,
    ),
    hasDraft: hasExpandedFileComposerDraft({
      effectiveText: trimmedEffectiveComposerValue,
      attachmentCount: composerAttachments.length,
      tokenCount: composerTokens.length,
      commentCount: composerComments.length,
    }),
    isBusy,
    requestComposerFocus,
    onBeforeFileOpen: () => {
      runPlanEntry.closePanel(false);
    },
    onBeforeFileExpand: () => {
      setContextUsagePanelOpen(false);
      runPlanEntry.closePanel(false);
    },
  });
  const {
    fileWorkspaceOpen,
    fileWorkspaceExpanded,
    expandedWorkspaceSurface,
    expandedFileComposerIdleMode,
    setExpandedFileComposerIdleMode,
    setExpandedFileComposerTrackedRun,
    fileWorkspaceMode,
    setFileWorkspaceMode,
    deliverablePreview,
    fileWorkspaceExpandedActive,
    fileWorkspaceSurfaceActive,
    expandedFileComposerCompleted,
    expandedFileComposerElapsedMs,
    expandedFileComposerForceFull,
    sidebarCollapsed,
    sidebarWidth,
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
    handleShowExpandedFileComposerFull,
    handleExpandedFilePreviewPrimaryClick,
    handleCloseFileWorkspace,
    handleOpenProjectFileTab,
    handleOpenDeliverablePreview,
    fileWorkspaceProjectId,
    fileWorkspaceProjectLabel,
    fileWorkspaceFolders,
    activeFileWorkspaceTab,
  } = fileWorkspace;

  const dockComposerMetaItems = composerMetaItemsForFileWorkspaceSurface(
    composerMetaItems,
    fileWorkspaceSurfaceActive,
  );
  const showCenterComposer = shouldUseCenterComposerWithFileWorkspace({
    standardCenterComposer: shouldUseCenterComposer({
      hasNarrative: visibleNarrativeItems.length > 0,
      hasRun: workbenchHasRun({
        runId: runtimeState.runId,
        isBusy,
      }),
      hasNotice: Boolean(notice),
      hasError: Boolean(runtimeState.errorMessage),
      needsRestart: desktopNeedsRestart,
      holdWorkbench: promptReplacementPending,
    }),
    fileWorkspaceOpen,
    fileWorkspaceExpanded,
    expandedWorkspaceSurface,
  });
  const scrollController = useRunStageScrollController({
    showCenterComposer,
    narrativeItems: visibleNarrativeItems,
  });
  const {
    activeWorkbenchTurnId,
    composerBarSafeAreaPx,
    composerContentInsetPx,
    setComposerDockElement,
    handleRunStageScroll,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeSections,
    prepareForNewTask,
    prepareForRunStart,
    prepareForThreadSwitch,
    runStageViewportHeightPx,
    setRunStageScrollElement,
  } = scrollController;
  const turnMarkerDisplay = turnMarkerRailDisplay({
    showCenterComposer,
    turnIds: narrativeSections.turns.map((turn) => turn.id),
    activeTurnId: activeWorkbenchTurnId,
    latestTurnId: latestPromptId,
    railHeight: turnMarkerRailHeight({
      scrollViewportHeight: runStageViewportHeightPx,
      composerContentInset: composerContentInsetPx,
      bottomSafeArea: composerBarSafeAreaPx,
    }),
  });
  const showTurnMarkerRail =
    !fileWorkspaceSurfaceActive && turnMarkerDisplay.mode !== "hidden";

  const threadSession = useThreadSession({
    client: runtimeClient,
    ready: desktopReady,
    state: runtimeState,
    dispatch,
    onError: setNotice,
    onTranscript: (transcript, isCurrent) => {
      syncProjectSelectionForThread({
        threadId: transcript.thread.threadId,
        projectId: transcript.thread.projectId,
        workspaceRoot: transcript.thread.workspace.workspaceRoot,
      });
      void runtimeClient
        .listMemoryDreams(transcript.thread.projectId, 5)
        .then((tasks) => {
          if (!isCurrent()) return;
          const assignedTasks = dreamTasksForThreadReplay(
            tasks,
            transcript.thread.threadId,
            dreamReceiptThreadAssignmentsRef.current,
          );
          for (const task of assignedTasks.slice().reverse()) {
            const event = dreamTaskReceiptEvent(task);
            if (event)
              dispatch({ type: "run/event", event, receivedAtMs: Date.now() });
          }
        })
        .catch(() => {
          // Dream receipts are auxiliary to transcript recovery; runtime errors
          // remain visible through the Settings task status.
        });
    },
  });

  async function handleAddProjectEntryToTask(
    request: ProjectEntryActionRequest,
  ): Promise<{ ok: true } | { ok: false; message: string }> {
    if (isBusy) {
      return { ok: false, message: "任务执行中，结束后再添加文件上下文。" };
    }
    const result = await runtimeClient.resolveProjectEntryForTask(request);
    if (!result.ok) {
      setNotice(result.message);
      return { ok: false, message: result.message };
    }

    const existingToken = composerTokens.find(
      (token) =>
        token.type === "file" &&
        token.payload?.path === result.entry.absolutePath &&
        token.payload?.projectId === result.entry.projectId &&
        token.payload?.folderPath === result.entry.folderPath,
    );
    if (existingToken) {
      const offset = composerTokenEndOffset(
        composerTokens,
        composerTokenLeadingTexts,
        existingToken.id,
      );
      setExpandedFileComposerTrackedRun(undefined);
      setExpandedFileComposerIdleMode("full");
      if (offset !== undefined) {
        setComposerInputSelection({ anchor: offset, focus: offset });
      }
      requestComposerFocus(offset);
      return { ok: true };
    }

    const payload: ProjectPathComposerTokenPayload = {
      path: result.entry.absolutePath,
      entryKind: result.entry.kind,
      projectId: result.entry.projectId,
      folderPath: result.entry.folderPath,
      relativePath: result.entry.relativePath,
    };
    const token: ComposerToken = {
      id: `project-path:${window.crypto.randomUUID()}`,
      type: "file",
      label: result.entry.displayName,
      payload: payload as unknown as Record<string, unknown>,
    };
    const draftLength = composerStructuredDraftLength(
      composerTokens,
      composerTokenLeadingTexts,
      composerValue,
    );
    const offset = composerInputSelection?.focus ?? draftLength;
    const nextDraft = insertComposerTokenAtOffset({
      tokens: composerTokens,
      placements: composerTokenPlacements,
      leadingTexts: composerTokenLeadingTexts,
      value: composerValue,
      offset,
      token,
    });
    setComposerTokens(nextDraft.tokens);
    setComposerTokenPlacements(nextDraft.placements);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerValue(nextDraft.value);
    setExpandedFileComposerTrackedRun(undefined);
    setExpandedFileComposerIdleMode("full");
    setComposerInputSelection({
      anchor: nextDraft.focusOffset,
      focus: nextDraft.focusOffset,
    });
    requestComposerFocus(nextDraft.focusOffset);
    setNotice(null);
    return { ok: true };
  }

  function insertComposerTokenFromExtensions(token: ComposerToken): void {
    const existingTokenOffset = composerTokenEndOffset(
      composerTokens,
      composerTokenLeadingTexts,
      token.id,
    );
    if (existingTokenOffset !== undefined) {
      setActiveNavId(undefined);
      setComposerInputSelection({
        anchor: existingTokenOffset,
        focus: existingTokenOffset,
      });
      requestComposerFocus(existingTokenOffset);
      return;
    }
    const draftLength = composerStructuredDraftLength(
      composerTokens,
      composerTokenLeadingTexts,
      composerValue,
    );
    const offset = composerInputSelection?.focus ?? draftLength;
    const nextDraft = insertComposerTokenAtOffset({
      tokens: composerTokens,
      placements: composerTokenPlacements,
      leadingTexts: composerTokenLeadingTexts,
      value: composerValue,
      offset,
      token,
    });
    setComposerTokens(nextDraft.tokens);
    setComposerTokenPlacements(nextDraft.placements);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerValue(nextDraft.value);
    setComposerInputSelection({
      anchor: nextDraft.focusOffset,
      focus: nextDraft.focusOffset,
    });
    setActiveNavId(undefined);
    requestComposerFocus(nextDraft.focusOffset);
  }

  function handleUsePluginFromExtensions(plugin: RuntimePlugin): void {
    insertComposerTokenFromExtensions(pluginComposerTokenFromAction(plugin));
  }

  function handleUsePluginComponentFromExtensions(
    plugin: RuntimePlugin,
    component: RuntimePluginComponent,
    commandArguments: string[],
  ): void {
    if (component.kind === "skill") {
      insertComposerTokenFromExtensions(
        pluginSkillComposerTokenFromAction(component),
      );
      return;
    }
    if (component.kind === "rule") {
      insertComposerTokenFromExtensions(
        pluginRuleComposerTokenFromAction(plugin, component),
      );
      return;
    }
    if (component.kind === "command") {
      insertComposerTokenFromExtensions(
        pluginCommandComposerTokenFromAction(
          plugin,
          component,
          commandArguments,
        ),
      );
      return;
    }
    if (component.kind === "agent") {
      insertComposerTokenFromExtensions(
        pluginAgentComposerTokenFromAction(plugin, component),
      );
    }
  }
  const { handleSubmitWithPrompt, handleCancel } = useRunSubmission({
    runtimeClient,
    threadSession,
    dispatch,
    runtimeState: {
      activeThreadId: runtimeState.activeThreadId,
      runId: runtimeState.runId,
    },
    desktopReady,
    desktopNeedsRestart,
    isBusy,
    getSubmissionScope: projectRegistry.getSubmissionScope,
    onRunStarted: projectRegistry.onRunStarted,
    draft: composerDraftController,
    composerDraftKey,
    selectedModelProfile,
    selectedReasoningEffort,
    fileWorkspaceSurfaceActive,
    setExpandedFileComposerTrackedRun,
    setExpandedFileComposerIdleMode,
    prepareForRunStart,
    refreshThreads,
    startThreadTitlePoll,
    setNotice,
  });
  const promptActions = usePromptRewind({
    setPromptReplacementPending,
    runtimeClient,
    threadSession,
    dispatch,
    runtimeState: { activeThreadId: runtimeState.activeThreadId },
    isBusy,
    narrativeSections,
    draft: composerDraftController,
    handleSubmitWithPrompt,
    requestComposerFocus,
    refreshThreads,
    setNotice,
  });
  const {
    editingPrompt,
    setEditingPrompt,
    setPendingPromptRewind,
    setMessageActionPendingRunId,
    setRewindDialogHidden,
  } = promptActions;

  async function handleComposerSubmit(): Promise<void> {
    if (editingPrompt) {
      return;
    }
    if (isBusy) {
      await handleCancel();
      requestComposerFocus();
      return;
    }
    await handleSubmitWithPrompt();
    requestComposerFocus();
  }

  async function handleSelectConversation(
    threadId: string,
    prefetchedThreads?: ThreadItem[],
    _forceReplay = false,
  ): Promise<void> {
    if (!desktopReady) {
      return;
    }
    // Exit any extension-level page (scheduled tasks, connectors, skills) so
    // that activation from system notifications or execution history always
    // lands on the workbench thread view.
    setActiveNavId(undefined);

    const loadSeq = threadSession.begin(threadId);
    setNotice(null);
    setEditingPrompt(undefined);
    setPendingPromptRewind(undefined);
    setRewindDialogHidden(false);
    setMessageActionPendingRunId(undefined);
    prepareForThreadSwitch();
    dispatch({ type: "run/threadSwitch", threadId });

    const threads = prefetchedThreads ?? runtimeState.threads;
    const thread = threads.find((item) => item.threadId === threadId);
    syncProjectSelectionForThread(thread);
    try {
      await threadSession.replay(threadId, loadSeq);
    } catch (error) {
      if (threadSession.isCurrent(loadSeq))
        setNotice(
          error instanceof Error
            ? error.message
            : "无法加载任务，请重新选择会话。",
        );
    }
  }

  function threadsForProject(projectId: string): ThreadItem[] {
    return runtimeState.threads.filter(
      (thread) =>
        projectIdForThread(thread, effectiveDefaultWorkspacePath) === projectId,
    );
  }

  function handleSelectProject(id: string): void {
    exitConversationBatchMode();
    setActiveNavId(undefined);
    if (id.startsWith("thread:")) {
      void handleSelectConversation(id.slice("thread:".length));
      return;
    }

    setConversationScope(false);

    const projectPath =
      id === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultWorkspacePath
        : projectsFromWorkspace.find((project) => project.id === id)?.path;
    if (
      (runtimeState.activeThreadId && activeThreadProjectId !== id) ||
      shouldResetActiveThreadForWorkspaceChange(
        activeThreadWorkspacePath,
        projectPath,
      )
    ) {
      resetActiveThreadContext();
    }
    setSelectedProjectId(id);
    workspaceTargetPathRef.current = projectPath ?? "";
    setWorkspacePath(projectPath ?? "");
  }

  function handleNewProjectSession(projectId: string): void {
    handleSelectProject(projectId);
    setExpandedProjectIds((current) =>
      current.includes(projectId) ? current : [...current, projectId],
    );
    resetNewTaskDraft();
    requestComposerFocus();
  }

  async function handleAnswerUserQuestion(answer: string): Promise<void> {
    if (!runtimeState.runId || !activeUserQuestion) {
      return;
    }
    if (resolvingUserQuestionId === activeUserQuestion.id) {
      return;
    }

    const trimmed = answer.trim();
    if (!trimmed) {
      return;
    }

    setNotice(null);
    setResolvingUserQuestionId(activeUserQuestion.id);
    try {
      await runtimeClient.answerUserQuestion(
        activeUserQuestion.runId ?? runtimeState.runId,
        activeUserQuestion.id,
        trimmed,
      );
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to answer user question",
      );
    } finally {
      setResolvingUserQuestionId(null);
    }
  }

  async function handleCancelUserQuestion(): Promise<void> {
    if (!runtimeState.runId || !activeUserQuestion) {
      return;
    }
    if (resolvingUserQuestionId === activeUserQuestion.id) {
      return;
    }

    setNotice(null);
    setResolvingUserQuestionId(activeUserQuestion.id);
    try {
      await runtimeClient.cancelUserQuestion(
        activeUserQuestion.runId ?? runtimeState.runId,
        activeUserQuestion.id,
      );
    } catch (error) {
      setNotice(
        error instanceof Error
          ? error.message
          : "Failed to cancel user question",
      );
    } finally {
      setResolvingUserQuestionId(null);
    }
  }

  function resetNewTaskDraft(): void {
    setActiveNavId(undefined);
    clearThreadComposerDraft(composerDraftOwnerKey());
    resetActiveThreadContext();
  }

  function handleSelectConversationScope(): void {
    exitConversationBatchMode();
    if (runtimeState.activeThreadId && activeThreadProjectId !== undefined) {
      resetActiveThreadContext();
    }
    setConversationScope(true);
    setSelectedProjectId(undefined);
    workspaceTargetPathRef.current = "";
    setWorkspacePath("");
    setConversationSectionExpanded(true);
  }

  function handleNewTask(): void {
    exitConversationBatchMode();
    setConversationScope(true);
    setSelectedProjectId(undefined);
    workspaceTargetPathRef.current = "";
    setWorkspacePath("");
    setConversationSectionExpanded(true);
    resetNewTaskDraft();
  }

  function handleSelectNav(id: string): void {
    exitConversationBatchMode();
    if (id === activeNavId && workbenchNavSectionForNavId(id)) {
      setExtensionNavigationEpoch((current) => current + 1);
    }
    setActiveNavId(id);
  }

  function handleOpenSubagent(taskId?: string): void {
    const id = taskId ?? subagentTasks.at(-1)?.taskId;
    if (!id || !runtimeState.subagentsById[id]) return;
    openSubagentTab(id);
    setInspectorCollapsed(false);
    if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
      inspectorCollapsedBeforeResponsiveRef.current = false;
    }
  }

  async function handleCancelSubagent(taskId: string): Promise<void> {
    const ownerRunId = runtimeState.subagentsById[taskId]?.runId;
    if (!ownerRunId) return;
    setCancellingSubagentTaskId(taskId);
    setNotice(null);
    try {
      await runtimeClient.cancelSubagent(
        ownerRunId,
        taskId,
        "desktop_user",
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "取消子任务失败");
    } finally {
      setCancellingSubagentTaskId(undefined);
    }
  }

  async function handleStopAllAgents(): Promise<void> {
    const threadId = runtimeState.activeThreadId;
    if (!threadId || !runtimeClient.stopThreadAgents) return;
    try {
      // Core cancels active subagents only and injects the user stop into the
      // root Agent. Composer stop-run still cancels the whole run tree.
      await runtimeClient.stopThreadAgents(threadId);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "停止全部子任务失败");
    }
  }

  const platform = runtimeClient.platform as PlatformKind;
  const mediaRevealLabel = mediaRevealLabelForPlatform(platform);
  const narrativeRenderer = useNarrativeRenderer({
    runtimeState: {
      activeThreadId: runtimeState.activeThreadId,
      threadId: runtimeState.threadId,
      artifacts: runtimeState.artifacts,
      subagentsById: runtimeState.subagentsById,
      toolResults: runtimeState.toolResults,
      patchPreviews: runtimeState.patchPreviews,
    },
    appearanceTheme,
    platform,
    replayedNarrativeItemIds,
    media: {
      artifactMediaSources,
      artifactMediaMetadata,
      openingArtifactId,
      recordArtifactMediaMetadata,
      refreshArtifactMediaSource,
      openArtifactExternally,
      revealArtifact,
    },
    subagentNowMs,
    openSubagentTaskId,
    handleOpenSubagent,
    onStopAllAgents:
      runningSubagentCount > 0 ? handleStopAllAgents : undefined,
    openMediaPreview,
    handleOpenDeliverablePreview,
    setNotice,
    workbenchDisclosureStore,
  });
  const { renderNarrativeItem, renderTurnBodySegment } = narrativeRenderer;

  const activeMediaPreviewIndex = mediaPreview
    ? Math.max(
        0,
        mediaPreview.items.findIndex(
          (item) => item.id === mediaPreview.activeId,
        ),
      )
    : -1;
  const activeMediaPreviewRecord =
    mediaPreview && activeMediaPreviewIndex >= 0
      ? mediaPreview.items[activeMediaPreviewIndex]
      : undefined;
  const activeMediaPreviewSourceState = activeMediaPreviewRecord
    ? artifactMediaSources[activeMediaPreviewRecord.sourceKey]
    : undefined;
  const activeMediaPreviewItem = activeMediaPreviewRecord
    ? {
        ...activeMediaPreviewRecord,
        src:
          typeof activeMediaPreviewSourceState === "string"
            ? activeMediaPreviewSourceState
            : undefined,
        metadata: artifactMediaMetadata[activeMediaPreviewRecord.sourceKey],
      }
    : undefined;
  const deliverablePreviewMediaSourceKey = deliverablePreview?.mediaKind
    ? artifactMediaSourceKey(deliverablePreview)
    : undefined;
  const deliverablePreviewMediaSourceState = deliverablePreviewMediaSourceKey
    ? artifactMediaSources[deliverablePreviewMediaSourceKey]
    : undefined;
  const deliverablePreviewMediaMetadata = deliverablePreviewMediaSourceKey
    ? artifactMediaMetadata[deliverablePreviewMediaSourceKey]
    : undefined;
  const currentConversationLabel = activeThread?.label?.trim() || "新建任务";

  function renderExternalActionGate() {
    if (activeExternalInteraction) {
      const resolving =
        resolvingExternalInteractionId === activeExternalInteraction.id;
      const isACPPermission =
        activeExternalInteraction.kind === "acp-permission";
      return (
        <ExternalActionGate
          compact
          interaction={activeExternalInteraction}
          resolving={resolving}
          onConfirmDone={() => {
            void resolveActiveExternalInteraction("completed");
          }}
          onSubmitForm={(content) => {
            void resolveActiveExternalInteraction("completed", content);
          }}
          onCancel={() => {
            if (isACPPermission) {
              void resolveActiveACPPermission("", true);
            } else {
              void resolveActiveExternalInteraction("cancelled");
            }
          }}
          onResolveACPOption={(optionId) => {
            void resolveActiveACPPermission(optionId);
          }}
        />
      );
    }
    return null;
  }

  function renderUserQuestionPrompt() {
    if (!activeUserQuestion) {
      return null;
    }
    const resolving = resolvingUserQuestionId === activeUserQuestion.id;
    return (
      <UserQuestionPrompt
        question={activeUserQuestion}
        disabled={resolving}
        onCancel={() => {
          void handleCancelUserQuestion();
        }}
        onSubmit={(answer) => {
          void handleAnswerUserQuestion(answer);
        }}
      />
    );
  }

  function renderComputerReviewGate() {
    if (!activeComputerReview) {
      return null;
    }
    const resolving = resolvingComputerReviewId === activeComputerReview.id;
    return (
      <ComputerReviewGate
        review={activeComputerReview}
        resolving={resolving}
        onResolve={(decision) => {
          void resolveActiveComputerReview(decision);
        }}
      />
    );
  }

  function renderComputerHandoffIndicator() {
    if (!activeComputerHandoff) {
      return null;
    }
    const busy = controllingComputerHandoffId === activeComputerHandoff.id;
    return (
      <ComputerHandoffIndicator
        handoff={activeComputerHandoff}
        busy={busy}
        onReturn={() => {
          void returnComputerControl();
        }}
      />
    );
  }

  function renderNoticeToast() {
    if (!notice) {
      return null;
    }

    return (
      <div className="desktop-toast" role="status" aria-live="polite">
        {notice}
      </div>
    );
  }

  return (
    <>
      <div
        className={`desktop-root desktop-root-electron ${inspectorPresentation.rootClassName}`}
        style={layoutStyle}
        aria-hidden={modelSettingsOpen || undefined}
        inert={modelSettingsOpen || undefined}
      >
        <AppShell
          platform={platform}
          windowChromeGeometry={
            platform === "macos" ? MACOS_WINDOW_CHROME_GEOMETRY : undefined
          }
          sidebarCollapsed={sidebarCollapsed}
          sidebarResizeHandleProps={{
            role: "separator",
            "aria-label": "Resize sidebar",
            "aria-orientation": "vertical",
            "aria-valuemin": MIN_SIDEBAR_WIDTH,
            "aria-valuemax": MAX_SIDEBAR_WIDTH,
            "aria-valuenow": sidebarWidth,
            tabIndex: 0,
            onPointerDown: handleSidebarResizeStart,
            onKeyDown: handleSidebarResizeKeyDown,
          }}
          inspectorCollapsed={inspectorPresentation.effectiveCollapsed}
          inspectorResizeHandleProps={
            shouldEnableInspectorResizeHandle({
              inspectorAvailable: inspectorPresentation.available,
              fileWorkspaceExpanded: fileWorkspaceExpandedActive,
            })
              ? {
                  role: "separator",
                  "aria-label": "Resize inspector",
                  "aria-orientation": "vertical",
                  "aria-valuemin": MIN_INSPECTOR_WIDTH,
                  "aria-valuemax": inspectorCap,
                  "aria-valuenow": effectiveInspectorWidth,
                  tabIndex: 0,
                  onPointerDown: handleInspectorResizeStart,
                  onKeyDown: handleInspectorResizeKeyDown,
                }
              : undefined
          }
          chrome={
            <DesktopWorkbenchChrome
              fileWorkspace={fileWorkspace}
              subagentTabs={openSubagentTabs}
              onOpenSubagent={handleOpenSubagent}
              platform={platform}
              handleNewTask={handleNewTask}
              currentConversationLabel={currentConversationLabel}
            />
          }
          sidebar={
            <DesktopSidebar
              projectRegistry={projectRegistry}
              projectOperations={projectOperations}
              threadNavigation={threadNavigation}
              navItems={navItems}
              activeNavId={activeNavId}
              repositoryProjects={repositoryProjects}
              activeProjectId={activeProjectId}
              allProjectsExpanded={allProjectsExpanded}
              handleNewTask={handleNewTask}
              handleSelectNav={handleSelectNav}
              handleSelectProject={handleSelectProject}
              desktopReady={desktopReady}
              profilePopoverOpen={profilePopoverOpen}
              setProfilePopoverOpen={setProfilePopoverOpen}
              profileButtonRef={profileButtonRef}
              sidebarUpdateAction={sidebarUpdateAction}
            />
          }
          main={
            <div className="desktop-main">
              {activeExtensionSection ? (
                <>
                  {renderNoticeToast()}
                  <ExtensionsView
                    section={activeExtensionSection}
                    desktopReady={desktopReady}
                    runtimeClient={runtimeClient}
                    initialPlugins={composerPluginCatalog}
                    onNotice={setNotice}
                    activeWorkspaceRoot={workspacePath}
                    composerCatalogRevision={composerCatalogRevision}
                    navigationEpoch={extensionNavigationEpoch}
                    onNavigateSection={setActiveNavId}
                    onUsePlugin={handleUsePluginFromExtensions}
                    onUsePluginComponent={
                      handleUsePluginComponentFromExtensions
                    }
                  />
                </>
              ) : scheduledTasksNavActive ? (
                <ScheduledTasksView
                  runtimeClient={runtimeClient}
                  desktopReady={desktopReady}
                  onOpenThread={(threadId) => {
                    backgroundThreadActivationRef.current(threadId);
                  }}
                />
              ) : (
                <WorkbenchShell
                  inspectorCollapsed={inspectorCollapsed}
                  inspectorExpanded={fileWorkspaceExpandedActive}
                  inspectorNarrowLayout={openSubagent ? "split" : "overlay"}
                  inspectorSurfaceActive={
                    !fileWorkspaceExpandedActive || fileWorkspaceSurfaceActive
                  }
                  stage={
                    <RunStage
                      contentInactive={
                        fileWorkspaceSurfaceActive ||
                        Boolean(activeMediaPreviewItem)
                      }
                      scrollRef={
                        showCenterComposer
                          ? undefined
                          : setRunStageScrollElement
                      }
                      onScroll={
                        showCenterComposer ? undefined : handleRunStageScroll
                      }
                      overlay={
                        <WorkbenchOverlays
                          workbenchPanels={workbenchPanels}
                          artifactMedia={artifactMedia}
                          fileWorkspace={fileWorkspace}
                          scrollController={scrollController}
                          renderNoticeToast={renderNoticeToast}
                          showTurnMarkerRail={showTurnMarkerRail}
                          turnMarkerDisplay={turnMarkerDisplay}
                          contextPanelData={contextPanelData}
                          contextUsagePanelId={contextUsagePanelId}
                          activeMediaPreviewItem={activeMediaPreviewItem}
                          runPlanEntry={runPlanEntry}
                          activeRunPlan={activeRunPlan}
                          runPlanPanelId={runPlanPanelId}
                          activeRunPlanItems={activeRunPlanItems}
                          runPlanPaused={runPlanPaused}
                          activeMediaPreviewIndex={activeMediaPreviewIndex}
                          activeMediaPreviewSourceState={
                            activeMediaPreviewSourceState
                          }
                          mediaRevealLabel={mediaRevealLabel}
                        />
                      }
                      composer={
                        showCenterComposer ? undefined : (
                          <ComposerDock
                            ref={setComposerDockElement}
                            interactionInactive={Boolean(
                              activeMediaPreviewItem,
                            )}
                          >
                            <div className="composer-dock-stack">
                              <DesktopStartupStatus
                                starting={desktopStarting}
                                needsRestart={desktopNeedsRestart}
                                placement="inline"
                              />
                              {renderUserQuestionPrompt()}
                              {renderExternalActionGate()}
                              {renderComputerReviewGate()}
                              {renderComputerHandoffIndicator()}
                              {shouldShowRunPlanSurface({
                                fileWorkspaceSurfaceActive,
                                mediaPreviewActive: Boolean(
                                  activeMediaPreviewItem,
                                ),
                              }) &&
                              runPlanEntry.entryVisible &&
                              activeRunPlan ? (
                                <RunPlanTrigger
                                  ref={runPlanEntry.triggerRef}
                                  items={activeRunPlanItems}
                                  completedCount={activeRunPlan.completedCount}
                                  totalCount={activeRunPlan.totalCount}
                                  currentStep={activeRunPlan.currentStep}
                                  allCompleted={activeRunPlan.allCompleted}
                                  paused={runPlanPaused}
                                  open={runPlanEntry.panelOpen}
                                  onDismiss={
                                    activeRunPlan.allCompleted
                                      ? runPlanEntry.dismissCompletedEntry
                                      : undefined
                                  }
                                  onToggle={() => {
                                    if (!runPlanEntry.panelOpen) {
                                      setContextUsagePanelOpen(false);
                                                            }
                                    runPlanEntry.togglePanel();
                                  }}
                                />
                              ) : null}
                              <ExpandedFileComposer
                                active={fileWorkspaceSurfaceActive}
                                idleMode={expandedFileComposerIdleMode}
                                running={isBusy}
                                completed={expandedFileComposerCompleted}
                                completedElapsedMs={
                                  expandedFileComposerElapsedMs
                                }
                                forceFull={expandedFileComposerForceFull}
                                onShowCompact={() =>
                                  setExpandedFileComposerIdleMode("compact")
                                }
                                onShowFull={handleShowExpandedFileComposerFull}
                                onStop={() => {
                                  void handleComposerSubmit();
                                }}
                              >
                                <ComposerSurface
                                  variant="dock"
                                  draft={composerDraftController}
                                  input={composerInput}
                                  lifecycle={composerLifecycle}
                                  editing={Boolean(editingPrompt)}
                                  onSubmit={handleComposerSubmit}
                                  metaItems={dockComposerMetaItems}
                                />
                              </ExpandedFileComposer>
                            </div>
                          </ComposerDock>
                        )
                      }
                    >
                      {showCenterComposer ? (
                        <div className="run-stage-center-composer">
                          <div className="desktop-task-welcome">
                            <DesktopStartupStatus
                              starting={desktopStarting}
                              needsRestart={desktopNeedsRestart}
                            />
                            <TaskWelcome />
                          </div>
                          {renderUserQuestionPrompt()}
                          {renderExternalActionGate()}
                          {renderComputerReviewGate()}
                          {renderComputerHandoffIndicator()}
                          <ComposerSurface
                            variant="center"
                            draft={composerDraftController}
                            input={composerInput}
                            lifecycle={composerLifecycle}
                            editing={Boolean(editingPrompt)}
                            onSubmit={handleComposerSubmit}
                            metaItems={composerMetaItems}
                            projectPicker={composerProjectPicker}
                          />
                        </div>
                      ) : null}

                      {shouldRenderWorkbenchNarrative({
                        showCenterComposer,
                        fileWorkspaceSurfaceActive,
                      }) ? (
                        <DesktopExecutionFeed
                          promptActions={promptActions}
                          narrativeSections={narrativeSections}
                          renderNarrativeItem={renderNarrativeItem}
                          runtimeState={runtimeState}
                          latestPromptId={latestPromptId}
                          activeWorkbenchTurnId={activeWorkbenchTurnId}
                          isBusy={isBusy}
                          latestTurnMinHeightPx={latestTurnMinHeightPx}
                          composerPluginBrandByName={composerPluginBrandByName}
                          activeThreadSupportsMessageRewind={
                            activeThreadSupportsMessageRewind
                          }
                          renderTurnBodySegment={renderTurnBodySegment}
                        />
                      ) : null}
                    </RunStage>
                  }
                  inspector={
                    <div className="inspector-surfaces">
                    {fileWorkspaceOpen ? <div className="inspector-surface" hidden={Boolean(openSubagent) || fileWorkspace.inspectorChooserActive}>
                      <ProjectFilesWorkspace
                        key={fileWorkspaceProjectId ?? "no-project"}
                        runtimeClient={runtimeClient}
                        projectId={fileWorkspaceProjectId}
                        projectLabel={fileWorkspaceProjectLabel}
                        folders={fileWorkspaceFolders}
                        initialMode={fileWorkspaceMode}
                        activeTab={activeFileWorkspaceTab}
                        artifactPreview={deliverablePreview}
                        artifactMediaSource={
                          typeof deliverablePreviewMediaSourceState === "string"
                            ? deliverablePreviewMediaSourceState
                            : undefined
                        }
                        artifactMediaSourceUnavailable={
                          deliverablePreviewMediaSourceState === null
                        }
                        artifactMediaMetadata={deliverablePreviewMediaMetadata}
                        revealLabel={mediaRevealLabel}
                        onArtifactMediaError={
                          deliverablePreviewMediaSourceKey && deliverablePreview
                            ? () =>
                                refreshArtifactMediaSource(
                                  deliverablePreviewMediaSourceKey,
                                )
                            : undefined
                        }
                        onArtifactMediaMetadata={
                          deliverablePreviewMediaSourceKey
                            ? (metadata) =>
                                recordArtifactMediaMetadata(
                                  deliverablePreviewMediaSourceKey,
                                  metadata,
                                )
                            : undefined
                        }
                        onWorkspaceModeChange={(mode) => {
                          setFileWorkspaceMode(mode);
                        }}
                        onOpenTab={handleOpenProjectFileTab}
                        onPreviewPrimaryClick={
                          handleExpandedFilePreviewPrimaryClick
                        }
                        addTaskDisabled={isBusy}
                        fileComments={composerComments}
                        onCreateFileComment={handleCreateFileLineComment}
                        onAddFileTextSelection={handleAddFileTextSelection}
                        onDeleteFileComment={handleRemoveFileLineComment}
                        onAddTaskContext={handleAddProjectEntryToTask}
                      />
                    </div> : null}
                    {openSubagent ? <SubagentPanel
                      key={openSubagent.taskId}
                      task={openSubagent}
                      nowMs={subagentNowMs}
                      cancelling={cancellingSubagentTaskId === openSubagent.taskId}
                      onCancel={() => void handleCancelSubagent(openSubagent.taskId)}
                      renderActivity={narrativeRenderer.renderNarrativeActivityItems}
                    /> : !fileWorkspaceOpen || fileWorkspace.inspectorChooserActive ? <div className="inspector-entry-page"><DesktopInspectorRail
                      handleOpenFileWorkspace={fileWorkspace.handleChooseInspectorEntry}
                    /></div> : null}
                    </div>
                  }
                />
              )}
            </div>
          }
        />
      </div>
      <ProjectManagementDialogs projectOperations={projectOperations} />

      <ThreadManagementDialogs threadNavigation={threadNavigation} />
      <PromptRewindDialog
        promptReplacementPending={promptReplacementPending}
        promptActions={promptActions}
      />

      <DesktopSettingsDialog
        open={modelSettingsOpen}
        runtimeClient={runtimeClient}
        currentProjectId={activeThreadProjectId ?? selectedProjectId}
        theme={appearanceTheme}
        themePreference={appearanceThemePreference}
        themeSaving={appearanceSaving}
        themeMessage={appearanceMessage}
        onThemePreferenceChange={(theme) => {
          void handleAppearanceThemeChange(theme);
        }}
        onOpenChange={setModelSettingsOpen}
        initialModelSection={modelSettingsInitialModelSection}
        onProfilesChanged={() => {
          void refreshRuntimeInfoNow();
        }}
        onThreadsChanged={() => {
          void refreshThreads();
        }}
      />
      <ProfileBalancePopover
        open={profilePopoverOpen}
        anchorRef={profileButtonRef}
        runtimeClient={runtimeClient}
        themePreference={appearanceThemePreference}
        themeSaving={appearanceSaving}
        onThemePreferenceChange={(theme) => {
          void handleAppearanceThemeChange(theme);
        }}
        onOpenSettings={() => {
          setProfilePopoverOpen(false);
          setModelSettingsInitialModelSection("api");
          setModelSettingsOpen(true);
        }}
        onClose={() => setProfilePopoverOpen(false)}
      />
      <DesktopUpdateDialog desktopUpdate={desktopUpdate} />
    </>
  );
}
