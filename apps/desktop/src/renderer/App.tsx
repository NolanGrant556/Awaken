import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import {
  FolderPlus,
  GitBranch,
  ListFilter,
  MessageCirclePlus,
  Monitor,
  PanelRightClose,
  PanelRightOpen,
  Plug,
  Settings,
  Wrench,
} from "lucide-react";
import {
  AwakenAppShell as AppShell,
  AwakenApprovalCard as ApprovalCard,
  AwakenAssistantMessage as AssistantMessage,
  AwakenButton as Button,
  AwakenComposerBar as ComposerBar,
  AwakenDialog as Dialog,
  AwakenExecutionFeed as ExecutionFeed,
  AwakenFileChangeSummary as FileChangeSummary,
  AwakenLightInspectorRail as LightInspectorRail,
  AwakenSelectableCard as SelectableCard,
  AwakenSidebar as Sidebar,
  AwakenSidebarSectionAction as SidebarSectionAction,
  AwakenTaskWelcome as TaskWelcome,
  AwakenThoughtTrace as ThoughtTrace,
  AwakenToolTraceGroup as ToolTraceGroup,
  AwakenToolTraceLine as ToolTraceLine,
  AwakenUserPromptBlock as UserPromptBlock,
  AwakenWorkbenchChrome as WorkbenchChrome,
  AwakenWorkbenchShell as WorkbenchShell,
  AwakenWorkbenchSidebarFrame as WorkbenchSidebarFrame,
  AwakenRunStage as RunStage,
  AwakenComposerDock as ComposerDock,
  type ComponentTone,
  type ComposerBarMetaItem,
  type NavItem,
  type PlatformKind,
  type ProjectItem,
} from "@awaken/design-system";
import { createRuntimeClient, type SubscribeHandlers } from "./runtime/client";
import type {
  RuntimeInfo,
  RuntimeModelProfileSummary,
  RuntimeRunStatus,
  StartRunRequest,
} from "../shared/runtime";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
  type ThreadItem,
  type RuntimeAction,
} from "./runtime/reducer";
import type { ApprovalDecision } from "../shared/runtime";
import type { RunNarrativeItem } from "./run-narrative";
import {
  resolvedDecisionForApproval,
  shouldShowAssistantAnswerFooter,
  shouldUseCenterComposer,
  turnMarkerRailDisplay,
  type ResolvedApprovalState,
} from "./workbenchPresentation";
import { useRunStageScrollController } from "./useRunStageScrollController";
import {
  actionsForThreadTranscript,
  afterSeqForTranscriptRun,
} from "./runtime/thread-transcript";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  assignThreadToProject,
  isSameWorkspacePath,
  projectIdForThread,
  upsertWorkspaceProject,
  workspaceLabelForPath,
  type WorkspaceProject,
} from "./projectWorkspace";
import { ModelSettingsDialog } from "./ModelSettingsDialog";

type InspectorTabId = "changes";

function formatStatusLabel(
  phase: ReturnType<typeof createInitialRuntimeState>["phase"],
  status?: ReturnType<typeof createInitialRuntimeState>["status"],
): string {
  if (status === "awaiting_approval") {
    return "Awaiting approval";
  }
  switch (phase) {
    case "starting":
      return "Creating run";
    case "running":
      return "Streaming";
    case "reconnecting":
      return "Reconnecting...";
    case "cancelling":
      return "Cancelling";
    case "done":
      return "Completed";
    case "cancelled":
      return "Cancelled";
    case "error":
      return "Error";
    default:
      return "Idle";
  }
}

function sidebarStatusForThread(
  thread: ThreadItem,
): { label: string; tone: ComponentTone } {
  if ((thread.approvalCount ?? 0) > 0 || thread.runStatus === "awaiting_approval") {
    return { label: "needs input", tone: "permission" };
  }

  switch (thread.runStatus) {
    case "cancelling":
      return { label: "cancelling", tone: "warning" };
    case "cancelled":
      return { label: "cancelled", tone: "neutral" };
    case "error":
      return { label: "error", tone: "error" };
    case "finished":
      return { label: "done", tone: "success" };
    case "running":
      if ((thread.artifactCount ?? 0) > 0) {
        return { label: "ready for review", tone: "warning" };
      }
      return { label: "running", tone: "local" };
    default:
      if (thread.activeRunId) {
        return { label: "running", tone: "local" };
      }
      if ((thread.artifactCount ?? 0) > 0) {
        return { label: "ready for review", tone: "warning" };
      }
      return { label: "idle", tone: "neutral" };
  }
}

function formatTokenCount(value?: number): string | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  if (value >= 1_000_000) {
    const amount = value / 1_000_000;
    return `${Number.isInteger(amount) ? amount.toFixed(0) : amount.toFixed(1)}M`;
  }
  if (value >= 1000) {
    const amount = value / 1000;
    return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)}k`;
  }
  return String(value);
}

function rawToolId(selectionId: string): string {
  if (selectionId.startsWith("tool:")) {
    return selectionId.slice("tool:".length);
  }
  if (selectionId.startsWith("explore:")) {
    return selectionId.slice("explore:".length);
  }
  return selectionId;
}

function contextUsageMeta(
  contextWindow: ReturnType<typeof createInitialRuntimeState>["inspectorSummary"]["contextWindow"],
): Pick<ComposerBarMetaItem, "label" | "value" | "progress"> | undefined {
  const estimated = formatTokenCount(contextWindow?.estimatedTokens);
  const modelWindow = formatTokenCount(contextWindow?.modelWindow);
  if (estimated && modelWindow) {
    const progress =
      typeof contextWindow?.estimatedTokens === "number" &&
      typeof contextWindow?.modelWindow === "number" &&
      contextWindow.modelWindow > 0
        ? contextWindow.estimatedTokens / contextWindow.modelWindow
        : undefined;
    return {
      label: "context windows",
      value: `${estimated} / ${modelWindow}`,
      progress,
    };
  }
  if (estimated) {
    return {
      label: "context windows",
      value: estimated,
    };
  }
  return undefined;
}

function modelProfileLabel(profile: RuntimeModelProfileSummary): string {
  return profile.modelId || profile.displayName;
}

function modelProfileIcon(profile: RuntimeModelProfileSummary): string {
  return profile.displayName.trim().slice(0, 1) || "M";
}

function firstContentLine(value?: string): string | undefined {
  return value?.split("\n").map((line) => line.trim()).find(Boolean);
}

const DEFAULT_SIDEBAR_WIDTH = 300;
const MIN_SIDEBAR_WIDTH = 228;
const MAX_SIDEBAR_WIDTH = 420;
const DEFAULT_INSPECTOR_WIDTH = 300;
const MIN_INSPECTOR_WIDTH = 260;
const MAX_INSPECTOR_WIDTH = 520;
const LAYOUT_STEP = 12;
const INSPECTOR_RESPONSIVE_COLLAPSE_QUERY = "(max-width: 1180px)";

function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function readStoredNumber(
  key: string,
  fallback: number,
  min: number,
  max: number,
): number {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = Number(window.localStorage.getItem(key));
    if (!Number.isFinite(value)) {
      return fallback;
    }
    return clampNumber(value, min, max);
  } catch {
    return fallback;
  }
}

function readStoredBoolean(key: string, fallback: boolean): boolean {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = window.localStorage.getItem(key);
    if (value === null) {
      return fallback;
    }
    return value === "true";
  } catch {
    return fallback;
  }
}

function writeStoredValue(key: string, value: string): void {
  if (typeof window === "undefined" || !window.localStorage) {
    return;
  }
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Layout persistence is best-effort.
  }
}

export function App() {
  const runtimeClientRef = useRef(createRuntimeClient());
  const unsubscribeRef = useRef<null | (() => void)>(null);
  const terminalSeenRef = useRef(false);
  const resolvingApprovalIdRef = useRef<string | null>(null);
  const rendererReadyNotifiedRef = useRef(false);
  const threadLoadSeqRef = useRef(0);
  const inspectorCollapsedValueRef = useRef(false);
  const inspectorCollapsedBeforeResponsiveRef = useRef<boolean | null>(null);
  const runtimeClient = runtimeClientRef.current;

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const [composerValue, setComposerValue] = useState("");
  const [composerFocusRequest, setComposerFocusRequest] = useState(0);
  const [activeNavId, setActiveNavId] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | null>(null);
  const [resolvingApprovalId, setResolvingApprovalId] = useState<string | null>(null);
  const [resolvedApproval, setResolvedApproval] = useState<ResolvedApprovalState>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
  const [modelProfiles, setModelProfiles] = useState<RuntimeModelProfileSummary[]>([]);
  const [modelSettingsOpen, setModelSettingsOpen] = useState(false);
  const [workspacePath, setWorkspacePath] = useState<string>("");
  const [createProjectDialogOpen, setCreateProjectDialogOpen] = useState(false);
  const [pendingProjectMode, setPendingProjectMode] = useState<"local">("local");
  const [pickingWorkspace, setPickingWorkspace] = useState(false);
  const [projectsFromWorkspace, setProjectsFromWorkspace] = useState<WorkspaceProject[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState(DEFAULT_WORKSPACE_PROJECT_ID);
  const [expandedProjectIds, setExpandedProjectIds] = useState<string[]>([
    DEFAULT_WORKSPACE_PROJECT_ID,
  ]);
  const [threadProjectIds, setThreadProjectIds] = useState<Record<string, string>>({});
  const [inspectorTab, setInspectorTab] = useState<InspectorTabId>("changes");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(() =>
    readStoredNumber(
      "awaken:layout:sidebarWidth",
      DEFAULT_SIDEBAR_WIDTH,
      MIN_SIDEBAR_WIDTH,
      MAX_SIDEBAR_WIDTH,
    ),
  );
  const [inspectorWidth, setInspectorWidth] = useState(() =>
    readStoredNumber(
      "awaken:layout:inspectorWidth",
      DEFAULT_INSPECTOR_WIDTH,
      MIN_INSPECTOR_WIDTH,
      MAX_INSPECTOR_WIDTH,
    ),
  );
  const [inspectorCollapsed, setInspectorCollapsed] = useState(() =>
    readStoredBoolean("awaken:layout:inspectorCollapsed", false),
  );
  const visibleNarrativeItems = useMemo<RunNarrativeItem[]>(() => {
    const narrativeItems: RunNarrativeItem[] = [
      ...runtimeState.narrative.committedItems,
      ...(runtimeState.narrative.activeItem ? [runtimeState.narrative.activeItem] : []),
    ];
    return narrativeItems.filter((item) => item.kind !== "planning");
  }, [runtimeState.narrative]);

  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "reconnecting" ||
    runtimeState.phase === "cancelling";
  const desktopReady = runtimeState.supervisorStatus === "running" && runtimeInfo !== null;
  const desktopStarting =
    runtimeState.supervisorStatus === null ||
    runtimeState.supervisorStatus === "starting" ||
    runtimeState.supervisorStatus === "stopping" ||
    runtimeState.supervisorStatus === "stopped";
  const desktopNeedsRestart = runtimeState.supervisorStatus === "crashed";

  useEffect(() => {
    writeStoredValue("awaken:layout:sidebarWidth", String(sidebarWidth));
  }, [sidebarWidth]);

  useEffect(() => {
    writeStoredValue("awaken:layout:inspectorWidth", String(inspectorWidth));
  }, [inspectorWidth]);

  useEffect(() => {
    writeStoredValue("awaken:layout:inspectorCollapsed", String(inspectorCollapsed));
    inspectorCollapsedValueRef.current = inspectorCollapsed;
  }, [inspectorCollapsed]);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }

    const media = window.matchMedia(INSPECTOR_RESPONSIVE_COLLAPSE_QUERY);
    const handleResponsiveInspector = (matches: boolean) => {
      if (matches) {
        if (inspectorCollapsedBeforeResponsiveRef.current === null) {
          inspectorCollapsedBeforeResponsiveRef.current = inspectorCollapsedValueRef.current;
          setInspectorCollapsed(true);
        }
        return;
      }

      const previousCollapsed = inspectorCollapsedBeforeResponsiveRef.current;
      inspectorCollapsedBeforeResponsiveRef.current = null;
      if (previousCollapsed !== null) {
        setInspectorCollapsed(previousCollapsed);
      }
    };
    const handleChange = (event: MediaQueryListEvent) => {
      handleResponsiveInspector(event.matches);
    };

    handleResponsiveInspector(media.matches);
    media.addEventListener("change", handleChange);
    return () => {
      media.removeEventListener("change", handleChange);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function poll(): Promise<void> {
      if (runtimeState.supervisorStatus !== "running") {
        setRuntimeInfo(null);
        setModelProfiles([]);
        return;
      }
      try {
        const [info, profiles] = await Promise.all([
          runtimeClient.getRuntimeInfo(),
          runtimeClient.listModelProfiles().catch(() => []),
        ]);
        if (!cancelled) {
          setRuntimeInfo(info);
          setModelProfiles(profiles);
        }
      } catch {
        if (!cancelled) {
          setRuntimeInfo(null);
          setModelProfiles([]);
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
    return () => {
      unsubscribeRef.current?.();
      unsubscribeRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (rendererReadyNotifiedRef.current || (!desktopReady && !desktopNeedsRestart)) {
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
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      if (!cancelled) {
        runtimeClient.notifyRendererReady();
      }
    }

    void notifyWhenStable();

    return () => {
      cancelled = true;
    };
  }, [desktopNeedsRestart, desktopReady, runtimeClient]);

  useEffect(() => {
    if (runtimeInfo && workspacePath === "") {
      setWorkspacePath(runtimeInfo.defaultWorkspace);
    }
  }, [runtimeInfo, workspacePath]);

  useEffect(() => {
    setInspectorTab("changes");
  }, [runtimeState.runId, runtimeState.activeThreadId]);

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent): void {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "b") {
        event.preventDefault();
        setSidebarCollapsed((collapsed) => !collapsed);
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  async function refreshThreads(): Promise<void> {
    if (!desktopReady) {
      return;
    }
    try {
      const threads = await runtimeClient.listThreads();
      dispatch({ type: "run/threadsLoaded", threads });
    } catch {
      // Thread listing is best-effort; runtime reachability is already surfaced
      // by the runtime info poll.
    }
  }

  async function refreshRuntimeInfoNow(): Promise<void> {
    if (runtimeState.supervisorStatus !== "running") {
      setRuntimeInfo(null);
      setModelProfiles([]);
      return;
    }
    try {
      const [info, profiles] = await Promise.all([
        runtimeClient.getRuntimeInfo(),
        runtimeClient.listModelProfiles().catch(() => []),
      ]);
      setRuntimeInfo(info);
      setModelProfiles(profiles);
    } catch {
      setRuntimeInfo(null);
      setModelProfiles([]);
    }
  }

  async function handleSelectModelProfile(profileId: string): Promise<void> {
    try {
      setNotice(null);
      await runtimeClient.setDefaultModelProfile(profileId);
      await refreshRuntimeInfoNow();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to switch model");
    }
  }

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    void refreshThreads();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady, runtimeClient]);

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    const persistedRunId = localStorage.getItem("awaken:runId");
    if (!persistedRunId) return;

    let cancelled = false;

    async function recover(): Promise<void> {
      try {
        const run = await runtimeClient.getRun(persistedRunId!);
        if (cancelled) return;
        try {
          const threads = await runtimeClient.listThreads();
          if (cancelled) return;
          dispatch({ type: "run/threadsLoaded", threads });
        } catch {
          // Recovery can still rebuild the thread transcript even if thread
          // listing fails temporarily.
        }
        const loadSeq = threadLoadSeqRef.current + 1;
        threadLoadSeqRef.current = loadSeq;
        await replayThreadTranscript(run.threadId, loadSeq);
      } catch {
        if (!cancelled) {
          localStorage.removeItem("awaken:runId");
        }
      }
    }

    void recover();

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady, runtimeClient]);

  const isWorkspaceValid =
    workspacePath.trim() !== "" && /^([A-Za-z]:[\\/]|\/)/.test(workspacePath.trim());

  const activeApproval = runtimeState.approvals[0];
  const navItems: NavItem[] = [
    {
      id: "connectors",
      label: "连接器",
      icon: <Plug size={16} strokeWidth={1.9} />,
    },
    {
      id: "skills",
      label: "技能",
      icon: <Wrench size={16} strokeWidth={1.9} />,
    },
  ];
  const topbarStatusLabel = desktopNeedsRestart
    ? "Needs restart"
    : desktopStarting
      ? "Starting"
      : undefined;
  const defaultWorkspacePath = runtimeInfo?.defaultWorkspace ?? workspacePath;
  const projectLabelById = new Map<string, string>([
    [
      DEFAULT_WORKSPACE_PROJECT_ID,
      defaultWorkspacePath ? workspaceLabelForPath(defaultWorkspacePath) : "awaken",
    ],
    ...projectsFromWorkspace.map((project) => [project.id, project.label] as const),
  ]);
  const pathForSelectedProject =
    selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
      ? defaultWorkspacePath
      : projectsFromWorkspace.find((project) => project.id === selectedProjectId)?.path;
  const activeProjectLabel =
    projectLabelById.get(selectedProjectId) ??
    (pathForSelectedProject ? workspaceLabelForPath(pathForSelectedProject) : "awaken");
  const contextMeta = contextUsageMeta(runtimeState.inspectorSummary.contextWindow);
  const enabledModelProfiles = modelProfiles.filter((profile) => profile.enabled);
  const selectedModelProfile =
    enabledModelProfiles.find((profile) => profile.id === runtimeInfo?.modelProfile?.id) ??
    enabledModelProfiles.find((profile) => profile.isDefault) ??
    enabledModelProfiles[0];
  const modelMetaItem: ComposerBarMetaItem =
    enabledModelProfiles.length > 0 && selectedModelProfile
      ? {
          id: "model",
          label: modelProfileLabel(selectedModelProfile),
          menuItems: enabledModelProfiles.map((profile) => ({
            id: profile.id,
            label: modelProfileLabel(profile),
            icon: modelProfileIcon(profile),
            selected: profile.id === selectedModelProfile.id,
            onSelect: () => {
              if (profile.id !== selectedModelProfile.id) {
                void handleSelectModelProfile(profile.id);
              }
            },
          })),
          menuFooterAction: {
            label: "添加模型",
            icon: <Settings size={16} strokeWidth={1.9} />,
            onClick: () => setModelSettingsOpen(true),
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
    ...(contextMeta ? [{ id: "context", ...contextMeta }] : []),
  ];

  function projectAction(projectId: string, label: string) {
    return (
      <SidebarSectionAction
        label={`New session in ${label}`}
        onClick={(event) => {
          event.stopPropagation();
          handleNewProjectSession(projectId);
        }}
      >
        <MessageCirclePlus size={13} strokeWidth={1.85} />
      </SidebarSectionAction>
    );
  }

  function threadItemsForProject(projectId: string): ProjectItem[] {
    return runtimeState.threads
      .filter((thread) => projectIdForThread(thread.threadId, threadProjectIds) === projectId)
      .map((thread) => {
        const sidebarStatus = sidebarStatusForThread(thread);
        const timeLabel =
          thread.threadId === runtimeState.activeThreadId || sidebarStatus.label === "idle"
            ? undefined
            : sidebarStatus.label;
        return {
          id: `thread:${thread.threadId}`,
          kind: "thread",
          label: thread.label,
          timeLabel,
        };
      });
  }

  const defaultProjectThreads = threadItemsForProject(DEFAULT_WORKSPACE_PROJECT_ID);
  const repositoryProjects: ProjectItem[] = [
    {
      id: DEFAULT_WORKSPACE_PROJECT_ID,
      kind: "project",
      label: projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
      path: defaultWorkspacePath,
      action: projectAction(
        DEFAULT_WORKSPACE_PROJECT_ID,
        projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ?? "awaken",
      ),
      children: defaultProjectThreads.length > 0 ? defaultProjectThreads : undefined,
    },
    ...projectsFromWorkspace
      .filter((project) => !isSameWorkspacePath(project.path, defaultWorkspacePath))
      .map((project) => {
        const threads = threadItemsForProject(project.id);
        return {
          id: project.id,
          kind: "project" as const,
          label: project.label,
          path: project.path,
          action: projectAction(project.id, project.label),
          children: threads.length > 0 ? threads : undefined,
        };
      }),
  ];
  const activeProjectId = runtimeState.activeThreadId
    ? `thread:${runtimeState.activeThreadId}`
    : selectedProjectId;
  const inspectorTabs = [
    {
      id: "changes" as const,
      label: "Changes",
      icon: <GitBranch size={16} strokeWidth={1.85} />,
      tone: "neutral" as const,
    },
  ];
  const showCenterComposer = shouldUseCenterComposer({
    hasNarrative: visibleNarrativeItems.length > 0,
    hasRun: Boolean(runtimeState.runId),
    hasNotice: Boolean(notice),
    hasError: Boolean(runtimeState.errorMessage),
    needsRestart: desktopNeedsRestart,
  });
  const {
    activeWorkbenchTurnId,
    composerContentInsetPx,
    composerDockRef,
    composerSafeAreaPx,
    handleRunStageScroll,
    handleTurnMarkerClick,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeSections,
    prepareForNewTask,
    prepareForRunStart,
    prepareForThreadSwitch,
    runStageViewportHeightPx,
    setRunStageScrollElement,
  } = useRunStageScrollController({
    showCenterComposer,
    narrativeItems: visibleNarrativeItems,
  });
  const turnMarkerDisplay = turnMarkerRailDisplay({
    showCenterComposer,
    turnIds: narrativeSections.turns.map((turn) => turn.id),
    activeTurnId: activeWorkbenchTurnId,
    latestTurnId: latestPromptId,
    railHeight: Math.max(
      0,
      runStageViewportHeightPx + composerContentInsetPx - 28 - Math.max(28, composerSafeAreaPx),
    ),
  });
  const showTurnMarkerRail = turnMarkerDisplay.mode !== "hidden";

  function createSubscribeHandlers(): SubscribeHandlers {
    return {
      onEvent: (event) => {
        const receivedAtMs = Date.now();
        if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
          terminalSeenRef.current = true;
          localStorage.removeItem("awaken:runId");
        }
        if (event.type === "RUN_STATUS_CHANGED" && event.status === "cancelled") {
          localStorage.removeItem("awaken:runId");
        }
        if (event.type === "AWAKEN_APPROVAL_REQUESTED" && event.approvalId) {
          setResolvedApproval(null);
        }
        if (
          event.type === "AWAKEN_APPROVAL_RESOLVED" &&
          event.approvalId &&
          event.approvalId === resolvingApprovalIdRef.current
        ) {
          setResolvedApproval({
            approvalId: event.approvalId,
            decision: event.approvalDecision === "rejected" ? "rejected" : "approved",
          });
          resolvingApprovalIdRef.current = null;
          setResolvingApprovalId(null);
        }
        dispatch({ type: "run/event", event, receivedAtMs });
      },
      onError: (message) => dispatch({ type: "run/streamFailed", message }),
      onReconnect: () => dispatch({ type: "run/reconnecting" }),
      onClose: () => {
        // Non-terminal close is handled by the client layer's attemptReconnect
        // (which dispatches run/reconnecting and retries). This callback only
        // fires on terminal close; the give-up path (max retries exceeded) is
        // delivered through onError.
        unsubscribeRef.current = null;
      },
    };
  }

  function applyRuntimeActions(actions: RuntimeAction[]): void {
    for (const action of actions) {
      dispatch(action);
    }
  }

  function shouldResumeSubscription(status: RuntimeRunStatus): boolean {
    return status === "running" || status === "awaiting_approval" || status === "cancelling";
  }

  async function replayThreadTranscript(
    threadId: string,
    loadSeq: number,
  ): Promise<RuntimeRunStatus | undefined> {
    const transcript = await runtimeClient.getThreadTranscript(threadId);
    if (threadLoadSeqRef.current !== loadSeq) {
      return undefined;
    }

    applyRuntimeActions(actionsForThreadTranscript(transcript));

    const latestRun = transcript.runs[transcript.runs.length - 1]?.run;
    if (!latestRun) {
      localStorage.removeItem("awaken:runId");
      return undefined;
    }

    if (!shouldResumeSubscription(latestRun.status)) {
      localStorage.removeItem("awaken:runId");
      return latestRun.status;
    }

    localStorage.setItem("awaken:runId", latestRun.runId);
    const unsubscribe = await runtimeClient.subscribeRun(
      latestRun.runId,
      createSubscribeHandlers(),
      afterSeqForTranscriptRun(transcript, latestRun.runId),
    );
    if (threadLoadSeqRef.current !== loadSeq) {
      unsubscribe();
      return latestRun.status;
    }

    unsubscribeRef.current = unsubscribe;
    return latestRun.status;
  }

  async function handleSubmit(): Promise<void> {
    if (!desktopReady) {
      setNotice(desktopNeedsRestart ? "Awaken needs restart" : "Awaken is starting");
      return;
    }
    const prompt = composerValue.trim();
    if (!prompt) {
      setNotice("Input is required");
      return;
    }
    if (isBusy) {
      return;
    }
    if (!isWorkspaceValid) {
      setNotice("Please enter a valid absolute workspace path.");
      return;
    }

    const workspaceRoot = workspacePath.trim();
    const validation = await runtimeClient.validateWorkspace(workspaceRoot);
    if (!validation.valid) {
      setNotice(validation.error);
      return;
    }

    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    threadLoadSeqRef.current += 1;
    terminalSeenRef.current = false;
    setNotice(null);
    setResolvingApprovalId(null);
    setResolvedApproval(null);
    resolvingApprovalIdRef.current = null;
    prepareForRunStart();
    dispatch({ type: "run/requested", prompt, startedAtMs: Date.now() });
    setComposerValue("");

    try {
      const runProjectId = selectedProjectId;
      const startRunRequest: StartRunRequest = {
        input: prompt,
        threadId: runtimeState.activeThreadId || undefined,
        workspace: {
          workspaceRoot,
          writableRoots: workspaceRoot ? [workspaceRoot] : [],
        },
        client: {
          sessionId: window.crypto.randomUUID(),
          requestId: window.crypto.randomUUID(),
          entrypoint: "desktop",
        },
      };
      const run = await runtimeClient.startRun(startRunRequest);
      setThreadProjectIds((current) => assignThreadToProject(current, run.threadId, runProjectId));
      dispatch({ type: "run/created", run });
      dispatch({
        type: "run/threadAdded",
        threadId: run.threadId,
        label: prompt,
        runId: run.runId,
      });
      void refreshThreads();
      localStorage.setItem("awaken:runId", run.runId);
      const unsubscribe = await runtimeClient.subscribeRun(
        run.runId,
        createSubscribeHandlers(),
      );
      unsubscribeRef.current = unsubscribe;
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to create run";
      setComposerValue(prompt);
      dispatch({ type: "run/createFailed", message });
    }
  }

  async function handleComposerSubmit(): Promise<void> {
    if (isBusy) {
      await handleCancel();
      setComposerFocusRequest((request) => request + 1);
      return;
    }
    await handleSubmit();
    setComposerFocusRequest((request) => request + 1);
  }

  async function handleSelectConversation(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }
    if (threadId === runtimeState.activeThreadId) {
      return;
    }

    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    const loadSeq = threadLoadSeqRef.current + 1;
    threadLoadSeqRef.current = loadSeq;
    terminalSeenRef.current = false;
    setNotice(null);
    setResolvingApprovalId(null);
    setResolvedApproval(null);
    resolvingApprovalIdRef.current = null;
    prepareForThreadSwitch();
    dispatch({ type: "run/threadSwitch", threadId });

    const thread = runtimeState.threads.find((item) => item.threadId === threadId);
    if (!thread?.runId) {
      localStorage.removeItem("awaken:runId");
      return;
    }

    try {
      await replayThreadTranscript(threadId, loadSeq);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to load thread");
    }
  }

  function handleSelectProject(id: string): void {
    if (id.startsWith("thread:")) {
      void handleSelectConversation(id.slice("thread:".length));
      return;
    }

    const projectPath =
      id === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultWorkspacePath
        : projectsFromWorkspace.find((project) => project.id === id)?.path;
    setSelectedProjectId(id);
    if (projectPath) {
      setWorkspacePath(projectPath);
    }
  }

  function handleOpenCreateProject(): void {
    setPendingProjectMode("local");
    setCreateProjectDialogOpen(true);
    setNotice(null);
  }

  async function handleUseExistingFolder(): Promise<void> {
    if (pickingWorkspace) {
      return;
    }
    setPickingWorkspace(true);
    setNotice(null);

    try {
      const result = await runtimeClient.pickWorkspace(workspacePath || defaultWorkspacePath);
      if (result.canceled) {
        if (result.error) {
          setNotice(result.error);
        }
        return;
      }

      const validation = await runtimeClient.validateWorkspace(result.path);
      if (!validation.valid) {
        setNotice(validation.error);
        return;
      }

      const next = upsertWorkspaceProject(
        projectsFromWorkspace,
        result.path,
        defaultWorkspacePath,
      );
      setProjectsFromWorkspace(next.projects);
      setWorkspacePath(result.path);
      setSelectedProjectId(next.projectId);
      setExpandedProjectIds((current) => (
        current.includes(next.projectId) ? current : [...current, next.projectId]
      ));
      setCreateProjectDialogOpen(false);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to select folder");
    } finally {
      setPickingWorkspace(false);
    }
  }

  function handleNewProjectSession(projectId: string): void {
    handleSelectProject(projectId);
    setExpandedProjectIds((current) => (
      current.includes(projectId) ? current : [...current, projectId]
    ));
    handleNewTask();
    setComposerFocusRequest((request) => request + 1);
  }

  async function handleCancel(): Promise<void> {
    if (!runtimeState.runId || !isBusy) {
      return;
    }

    dispatch({ type: "run/cancelRequested" });

    try {
      const run = await runtimeClient.cancelRun(runtimeState.runId);
      dispatch({ type: "run/cancelSync", run });
      const terminalStatuses: RuntimeRunStatus[] = ["finished", "error", "cancelled"];
      if (terminalStatuses.includes(run.status)) {
        localStorage.removeItem("awaken:runId");
      }
    } catch (error) {
      dispatch({
        type: "run/streamFailed",
        message: error instanceof Error ? error.message : "Failed to cancel run",
      });
    }
  }

  async function handleResolveApproval(decision: ApprovalDecision): Promise<void> {
    if (!runtimeState.runId || !activeApproval) {
      return;
    }
    if (
      resolvingApprovalId === activeApproval.id ||
      resolvingApprovalIdRef.current === activeApproval.id
    ) {
      return;
    }

    setNotice(null);
    setResolvingApprovalId(activeApproval.id);
    resolvingApprovalIdRef.current = activeApproval.id;
    setResolvedApproval(null);

    try {
      await runtimeClient.resolveApproval(runtimeState.runId, activeApproval.id, decision);
    } catch (error) {
      setResolvingApprovalId(null);
      resolvingApprovalIdRef.current = null;
      setResolvedApproval(null);
      setNotice(error instanceof Error ? error.message : "Failed to resolve approval");
    }
  }

  function handleNewTask(): void {
    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    threadLoadSeqRef.current += 1;
    setComposerValue("");
    setNotice(null);
    setResolvingApprovalId(null);
    setResolvedApproval(null);
    resolvingApprovalIdRef.current = null;
    prepareForNewTask();
    terminalSeenRef.current = false;
    localStorage.removeItem("awaken:runId");
    dispatch({ type: "run/reset" });
  }

  function handleSelectNav(id: string): void {
    setActiveNavId(id);
  }

  function handleToggleInspectorCollapsed(): void {
    setInspectorCollapsed((collapsed) => {
      const nextCollapsed = !collapsed;
      if (inspectorCollapsedBeforeResponsiveRef.current !== null) {
        inspectorCollapsedBeforeResponsiveRef.current = nextCollapsed;
      }
      return nextCollapsed;
    });
  }

  function handleAddContext(): void {
    setNotice("Context picker is not wired yet.");
  }

  function handleSidebarResizeStart(event: ReactPointerEvent<HTMLDivElement>): void {
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture?.(event.pointerId);
    const startX = event.clientX;
    const startWidth = sidebarWidth;

    function handlePointerMove(moveEvent: PointerEvent): void {
      moveEvent.preventDefault();
      const nextWidth = clampNumber(
        startWidth + moveEvent.clientX - startX,
        MIN_SIDEBAR_WIDTH,
        MAX_SIDEBAR_WIDTH,
      );
      setSidebarWidth(nextWidth);
    }

    function handlePointerUp(): void {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      try {
        target.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released if the drag ends outside the window.
      }
      document.body.classList.remove("is-layout-resizing", "is-sidebar-resizing");
    }

    document.body.classList.add("is-layout-resizing", "is-sidebar-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleInspectorResizeStart(event: ReactPointerEvent<HTMLDivElement>): void {
    event.preventDefault();
    const target = event.currentTarget;
    target.setPointerCapture?.(event.pointerId);
    const startX = event.clientX;
    const startWidth = inspectorWidth;

    function handlePointerMove(moveEvent: PointerEvent): void {
      moveEvent.preventDefault();
      const nextWidth = clampNumber(
        startWidth - (moveEvent.clientX - startX),
        MIN_INSPECTOR_WIDTH,
        MAX_INSPECTOR_WIDTH,
      );
      setInspectorWidth(nextWidth);
    }

    function handlePointerUp(): void {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      try {
        target.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released if the drag ends outside the window.
      }
      document.body.classList.remove("is-layout-resizing", "is-inspector-resizing");
    }

    document.body.classList.add("is-layout-resizing", "is-inspector-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleSidebarResizeKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowRight" ? LAYOUT_STEP : -LAYOUT_STEP;
    setSidebarWidth((width) =>
      clampNumber(width + delta, MIN_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH),
    );
  }

  function handleInspectorResizeKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowLeft" ? LAYOUT_STEP : -LAYOUT_STEP;
    setInspectorWidth((width) =>
      clampNumber(width + delta, MIN_INSPECTOR_WIDTH, MAX_INSPECTOR_WIDTH),
    );
  }

  function renderNarrativeItem(item: RunNarrativeItem, key: string) {
    if (item.kind === "thinking") {
      return (
        <ThoughtTrace
          key={key}
          text={item.text}
          status={item.status}
        />
      );
    }
    if (item.kind === "answer") {
      const showAnswerFooter = shouldShowAssistantAnswerFooter({
        status: item.status,
        isFinal: item.isFinal,
      });
      return (
        <AssistantMessage
          key={key}
          markdown={item.markdown}
          status={item.status}
          elapsedMs={item.elapsedMs}
          showFooter={showAnswerFooter}
        />
      );
    }
    if (item.kind === "exploring") {
      const actions = item.actions.map((action) => {
        const id = action.id.startsWith("explore:")
          ? action.id.slice("explore:".length)
          : rawToolId(action.id);
        const tool = runtimeState.toolResults.find((record) => record.id === id);
        return {
          ...action,
          detail: toolTraceDetail(tool, action.detail),
        };
      });
      return (
        <ToolTraceGroup
          key={key}
          actions={actions}
        />
      );
    }
    if (item.kind === "tool") {
      const tool = runtimeState.toolResults.find((record) => record.id === rawToolId(item.id));
      return (
        <ToolTraceLine
          key={key}
          title={item.title}
          detail={toolTraceDetail(tool, item.detail)}
          status={item.status}
        />
      );
    }
    if (item.kind === "approval") {
      const approvalId = item.id.startsWith("approval:")
        ? item.id.slice("approval:".length)
        : item.id;
      const itemApproval = runtimeState.approvals.find(
        (approval) => approval.id === approvalId || approvalId.endsWith(`:${approval.id}`),
      );
      if (item.status === "waiting" && itemApproval?.id === activeApproval?.id) {
        return null;
      }
      const canResolve =
        item.status === "waiting" &&
        activeApproval?.id === itemApproval?.id;
      const resolvedDecision =
        item.status === "approved" || item.status === "rejected"
          ? item.status
          : resolvedDecisionForApproval(itemApproval, resolvedApproval);
      return (
        <ApprovalCard
          key={key}
          inline
          open
          title={itemApproval?.title || item.title}
          description={itemApproval?.description}
          affectedPaths={itemApproval?.affectedPaths}
          patchSummary={itemApproval?.patchSummary}
          riskTone={itemApproval?.riskTone}
          approveLabel={
            resolvingApprovalId === itemApproval?.id ? "Resolving..." : "Approve"
          }
          rejectLabel={
            resolvingApprovalId === itemApproval?.id ? "Resolving..." : "Reject"
          }
          resolving={resolvingApprovalId === itemApproval?.id}
          resolvedDecision={resolvedDecision}
          onApprove={
            canResolve
              ? () => {
                  void handleResolveApproval("approved");
                }
              : undefined
          }
          onReject={
            canResolve
              ? () => {
                  void handleResolveApproval("rejected");
                }
              : undefined
          }
        />
      );
    }
    if (item.kind === "fileChange") {
      return (
        <FileChangeSummary
          key={key}
          files={item.files}
          status={item.status}
        />
      );
    }
    return null;
  }

  function toolTraceDetail(
    tool: (typeof runtimeState.toolResults)[number] | undefined,
    fallback?: string,
  ): string | undefined {
    if (!tool) {
      return fallback;
    }
    const output = firstContentLine(tool.commandOutput);
    const result = firstContentLine(tool.content);
    if (tool.status === "done" || tool.status === "error") {
      return result || output || fallback;
    }
    return output || fallback || result;
  }

  function renderChangesPane() {
    return null;
  }

  function renderInspectorTabPanel() {
    switch (inspectorTab) {
      case "changes":
      default:
        return renderChangesPane();
    }
  }

  const platform = runtimeClient.platform as PlatformKind;
  const layoutStyle = {
    "--aw-sidebar-width": `${sidebarWidth}px`,
    "--aw-inspector-width": `${inspectorWidth}px`,
  } as CSSProperties;

  function renderActiveApprovalPrompt() {
    if (!activeApproval) {
      return null;
    }
    return (
      <div className="composer-approval-card">
        <ApprovalCard
          inline
          open
          title={activeApproval.title}
          description={activeApproval.description}
          affectedPaths={activeApproval.affectedPaths}
          patchSummary={activeApproval.patchSummary}
          riskTone={activeApproval.riskTone}
          approveLabel={
            resolvingApprovalId === activeApproval.id ? "Resolving..." : "Approve"
          }
          rejectLabel={
            resolvingApprovalId === activeApproval.id ? "Resolving..." : "Reject"
          }
          resolving={resolvingApprovalId === activeApproval.id}
          resolvedDecision={resolvedDecisionForApproval(activeApproval, resolvedApproval)}
          onApprove={() => {
            void handleResolveApproval("approved");
          }}
          onReject={() => {
            void handleResolveApproval("rejected");
          }}
        />
      </div>
    );
  }

  function renderComposerRuntimePanels() {
    const panels: Array<{
      id: string;
      title: string;
      message: string;
      tone?: "error";
    }> = [];

    if (desktopNeedsRestart) {
      panels.push({
        id: "restart",
        title: "Awaken needs restart",
        message: "Quit Awaken and open it again.",
        tone: "error",
      });
    }
    if (desktopStarting) {
      panels.push({
        id: "starting",
        title: "Starting Awaken",
        message: "Preparing the desktop app.",
      });
    }
    if (runtimeState.errorMessage) {
      panels.push({
        id: "run-error",
        title: "Run error",
        message: runtimeState.errorMessage,
        tone: "error",
      });
    }
    if (notice) {
      panels.push({
        id: "notice",
        title: "Notice",
        message: notice,
      });
    }

    if (panels.length === 0) {
      return null;
    }

    return (
      <div className="composer-runtime-panels">
        {panels.map((panel) => (
          <div
            key={panel.id}
            className={`runtime-panel composer-runtime-panel${
              panel.tone === "error" ? " runtime-panel-error" : ""
            }`}
            role={panel.tone === "error" ? "alert" : "status"}
          >
            <strong>{panel.title}</strong>
            <p>{panel.message}</p>
          </div>
        ))}
      </div>
    );
  }

  return (
    <>
      <div
        className={`desktop-root desktop-root-electron${
          inspectorCollapsed
            ? " desktop-root-inspector-collapsed"
            : " desktop-root-inspector-expanded"
        }`}
        style={layoutStyle}
      >
        <AppShell
          platform={platform}
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
          inspectorCollapsed={inspectorCollapsed}
          inspectorResizeHandleProps={{
            role: "separator",
            "aria-label": "Resize inspector",
            "aria-orientation": "vertical",
            "aria-valuemin": MIN_INSPECTOR_WIDTH,
            "aria-valuemax": MAX_INSPECTOR_WIDTH,
            "aria-valuenow": inspectorWidth,
            tabIndex: 0,
            onPointerDown: handleInspectorResizeStart,
            onKeyDown: handleInspectorResizeKeyDown,
          }}
          chrome={
            <WorkbenchChrome
              sidebarCollapsed={sidebarCollapsed}
              onToggleSidebar={() => setSidebarCollapsed((collapsed) => !collapsed)}
              onNewTask={handleNewTask}
              right={
                <div className="workbench-chrome-status">
                  {topbarStatusLabel ? (
                    <span className="workbench-chrome-state">{topbarStatusLabel}</span>
                  ) : null}
                  <button
                    type="button"
                    className="workbench-chrome-icon workbench-chrome-panel-toggle"
                    aria-label={inspectorCollapsed ? "Expand inspector" : "Collapse inspector"}
                    onClick={handleToggleInspectorCollapsed}
                  >
                    {inspectorCollapsed ? (
                      <PanelRightOpen size={14} strokeWidth={1.9} />
                    ) : (
                      <PanelRightClose size={14} strokeWidth={1.9} />
                    )}
                  </button>
                </div>
              }
            />
          }
          sidebar={
            <WorkbenchSidebarFrame>
              <Sidebar
                title="Awaken"
                showBrand={false}
                navItems={navItems}
                activeNavId={activeNavId}
                conversations={[]}
                projects={repositoryProjects}
                activeProjectId={activeProjectId}
                expandedIds={expandedProjectIds}
                defaultExpandedIds={[DEFAULT_WORKSPACE_PROJECT_ID]}
                profileName="Nolan"
                profileAvatarFallback="AW"
                profileSubtitle="Local"
                newTaskLabel="New Agent"
                conversationSectionLabel=""
                projectSectionLabel="Projects"
                projectSectionActions={
                  <span className="sidebar-section-actions">
                    <SidebarSectionAction
                      label="Filter projects"
                      onClick={() => setNotice("Project filters are not available yet.")}
                    >
                      <ListFilter size={13} strokeWidth={1.85} />
                    </SidebarSectionAction>
                    <SidebarSectionAction
                      label="Add folder"
                      onClick={handleOpenCreateProject}
                    >
                      <FolderPlus size={13} strokeWidth={1.85} />
                    </SidebarSectionAction>
                  </span>
                }
                showProjectCount={false}
                platform={platform}
                onSelectNav={handleSelectNav}
                onSelectProject={handleSelectProject}
                onExpandedChange={setExpandedProjectIds}
                onNewTask={handleNewTask}
                onProfileClick={() => setModelSettingsOpen(true)}
              />
            </WorkbenchSidebarFrame>
          }
          main={
            <div className="desktop-main">
              <WorkbenchShell
                inspectorCollapsed={inspectorCollapsed}
                stage={
                  <RunStage
                    scrollRef={showCenterComposer ? undefined : setRunStageScrollElement}
                    onScroll={showCenterComposer ? undefined : handleRunStageScroll}
                    overlay={
                      showTurnMarkerRail ? (
                        <nav
                          className={`turn-marker-rail turn-marker-rail-${turnMarkerDisplay.mode}`}
                          aria-label="Message shortcuts"
                          style={{ bottom: Math.max(28, composerSafeAreaPx) }}
                        >
                          {turnMarkerDisplay.items.map((marker) => (
                            <button
                              key={`${marker.id}:${marker.index}`}
                              type="button"
                              className={`turn-marker-button${
                                marker.id === activeWorkbenchTurnId ? " turn-marker-button-active" : ""
                              }`}
                              aria-current={marker.id === activeWorkbenchTurnId ? "true" : undefined}
                              aria-label={`跳转到第 ${marker.index + 1} 条消息`}
                              title={`第 ${marker.index + 1} 条消息`}
                              onClick={() => handleTurnMarkerClick(marker.id)}
                            >
                              <span className="turn-marker-bar" aria-hidden="true" />
                            </button>
                          ))}
                        </nav>
                      ) : undefined
                    }
                    composer={
                      showCenterComposer ? undefined : (
                        <ComposerDock ref={composerDockRef}>
                          <div className="composer-dock-stack">
                            {renderComposerRuntimePanels()}
                            {renderActiveApprovalPrompt()}
                            <ComposerBar
                              value={composerValue}
                              onChange={setComposerValue}
                              onSubmit={() => {
                                void handleComposerSubmit();
                              }}
                              onAddContext={handleAddContext}
                              placeholder=""
                              disabled={!desktopReady || isBusy || !isWorkspaceValid}
                              isReady={
                                isBusy ||
                                (desktopReady && composerValue.trim().length > 0 && isWorkspaceValid)
                              }
                              submitMode={isBusy ? "stop" : "send"}
                              variant="dock"
                              metaItems={composerMetaItems}
                              focusRequest={composerFocusRequest}
                            />
                          </div>
                        </ComposerDock>
                      )
                    }
                  >
                    {showCenterComposer ? (
                      <div className="run-stage-center-composer">
                        <TaskWelcome />
                        {renderComposerRuntimePanels()}
                        {renderActiveApprovalPrompt()}
                        <ComposerBar
                          value={composerValue}
                          onChange={setComposerValue}
                          onSubmit={() => {
                            void handleComposerSubmit();
                          }}
                          onAddContext={handleAddContext}
                          placeholder=""
                          disabled={!desktopReady || isBusy || !isWorkspaceValid}
                          isReady={
                            isBusy ||
                            (desktopReady && composerValue.trim().length > 0 && isWorkspaceValid)
                          }
                          submitMode={isBusy ? "stop" : "send"}
                          variant="center"
                          metaItems={composerMetaItems}
                          focusRequest={composerFocusRequest}
                        />
                      </div>
                    ) : null}

                    {showCenterComposer ? null : (
                      <ExecutionFeed>
                        {narrativeSections.leadItems.map((item, index) =>
                          renderNarrativeItem(item, `lead:${item.id}:${index}`),
                        )}
                        {narrativeSections.turns.map((turn, index) => {
                          const isLatestPrompt = turn.id === latestPromptId;
                          const isActiveTurn = turn.id === activeWorkbenchTurnId;
                          const turnStyle =
                            isLatestPrompt && latestTurnMinHeightPx > 0
                              ? ({
                                  "--aw-latest-turn-min-height": `${latestTurnMinHeightPx}px`,
                                } as CSSProperties)
                              : undefined;

                          return (
                            <section
                              key={`${turn.id}:${index}`}
                              data-execution-turn-id={turn.id}
                              className={`execution-turn-section${
                                isLatestPrompt ? " execution-turn-section-latest" : ""
                              }${
                                isActiveTurn ? " execution-turn-section-active" : ""
                              }`}
                              style={turnStyle}
                              aria-label={`Turn ${index + 1}`}
                            >
                              <div className="execution-turn-column">
                                <div className="execution-turn-header">
                                  <UserPromptBlock text={turn.prompt.text} />
                                </div>
                                <div className="execution-turn-body">
                                  {turn.items.map((item, itemIndex) =>
                                    renderNarrativeItem(
                                      item,
                                      `${turn.id}:${item.id}:${itemIndex}`,
                                    ),
                                  )}
                                </div>
                              </div>
                            </section>
                          );
                        })}
                      </ExecutionFeed>
                    )}
                  </RunStage>
                }
                inspector={
                  <LightInspectorRail
                    title="On Awaken"
                    status={desktopReady ? "Local" : "Offline"}
                    items={inspectorTabs}
                    activeId={inspectorTab}
                    onSelect={(id) => setInspectorTab(id as InspectorTabId)}
                  >
                    {renderInspectorTabPanel()}
                  </LightInspectorRail>
                }
              />
            </div>
          }
        />
      </div>
      <Dialog
        open={createProjectDialogOpen}
        title="Create project"
        onOpenChange={setCreateProjectDialogOpen}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              loading={pickingWorkspace}
              onClick={() => {
                void handleUseExistingFolder();
              }}
            >
              Use existing folder
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={pickingWorkspace}
              onClick={() => {
                void handleUseExistingFolder();
              }}
            >
              Next
            </Button>
          </>
        }
      >
        <div className="create-project-dialog-body">
          <SelectableCard
            selected={pendingProjectMode === "local"}
            icon={<Monitor size={18} strokeWidth={1.9} />}
            title="Local"
            description="Use a folder on this Mac as an Awaken project."
            onSelect={() => setPendingProjectMode("local")}
          />
        </div>
      </Dialog>
      <ModelSettingsDialog
        open={modelSettingsOpen}
        runtimeClient={runtimeClient}
        onOpenChange={setModelSettingsOpen}
        onProfilesChanged={() => {
          void refreshRuntimeInfoNow();
        }}
      />
    </>
  );
}
