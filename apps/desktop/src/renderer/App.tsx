import { useCallback, useEffect, useLayoutEffect, useReducer, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
  UIEvent as ReactUIEvent,
} from "react";
import {
  CalendarClock,
  Files,
  FolderPlus,
  GitBranch,
  Globe,
  ListFilter,
  MessageCirclePlus,
  Monitor,
  PanelRightClose,
  PanelRightOpen,
  SlidersHorizontal,
} from "lucide-react";
import {
  AwakenAppShell as AppShell,
  AwakenApprovalCard as ApprovalCard,
  AwakenAssistantMessage as AssistantMessage,
  AwakenButton as Button,
  AwakenComposerBar as ComposerBar,
  AwakenCopyButton as CopyButton,
  AwakenDialog as Dialog,
  AwakenDisclosure as Disclosure,
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
  type InspectorTabId,
  type NavItem,
  type PlatformKind,
  type ProjectItem,
} from "@awaken/design-system";
import { createRuntimeClient, type SubscribeHandlers } from "./runtime/client";
import type { RuntimeInfo, RuntimeRunStatus, StartRunRequest } from "../shared/runtime";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
  type ThreadItem,
  type RuntimeAction,
} from "./runtime/reducer";
import type { ApprovalDecision } from "../shared/runtime";
import { A2UISurfaceView } from "./a2ui/A2UISurfaceView";
import type { InspectorSelection, RunNarrativeItem } from "./run-narrative";
import {
  anchoredTurnMinHeight,
  activeTurnIdForAnchor,
  composerBottomSafeArea,
  composerContentBottomInset,
  groupNarrativeByTurns,
  isNearLatestTurnOutputTail,
  resolvedDecisionForApproval,
  scrollTopForLatestTurnOutputTail,
  scrollTopForPromptAnchor,
  shouldPauseLatestOutputFollowForTouchMove,
  shouldPauseLatestOutputFollowForWheel,
  shouldRequestLatestOutputFollowResumeForTouchMove,
  shouldRequestLatestOutputFollowResumeForWheel,
  shouldResumeLatestOutputFollow,
  shouldUseCenterComposer,
  turnMarkerRailDisplay,
  type ResolvedApprovalState,
} from "./workbenchPresentation";
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

function defaultInspectorTab(selection?: InspectorSelection): InspectorTabId {
  switch (selection?.kind) {
    case "tool":
      return "files";
    case "artifact":
    case "fileChange":
    case "approval":
      return "changes";
    default:
      return "changes";
  }
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

function rawFileChangeId(selectionId: string): string {
  return selectionId.startsWith("fileChange:")
    ? selectionId.slice("fileChange:".length)
    : selectionId;
}

function formatTokenCount(value?: number): string | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  if (value >= 1000) {
    const amount = value / 1000;
    return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)}k`;
  }
  return String(value);
}

function contextUsageMeta(
  contextWindow: ReturnType<typeof createInitialRuntimeState>["inspectorSummary"]["contextWindow"],
): Pick<ComposerBarMetaItem, "label" | "value" | "progress"> | undefined {
  const estimated = formatTokenCount(contextWindow?.estimatedTokens);
  const effective = formatTokenCount(contextWindow?.effectiveWindow);
  if (estimated && effective) {
    const progress =
      typeof contextWindow?.estimatedTokens === "number" &&
      typeof contextWindow?.effectiveWindow === "number" &&
      contextWindow.effectiveWindow > 0
        ? contextWindow.estimatedTokens / contextWindow.effectiveWindow
        : undefined;
    return {
      label: "context windows",
      value: `${estimated} / ${effective}`,
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

function latestDoneAnswerId(items: RunNarrativeItem[]): string | undefined {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index];
    if (item.kind === "answer" && item.status === "done") {
      return item.id;
    }
  }
  return undefined;
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
const DEFAULT_PROMPT_ANCHOR_OFFSET = 0;
const RUN_STAGE_COMPOSER_SAFE_GAP = 72;
const LATEST_OUTPUT_FOLLOW_THRESHOLD = 48;

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
  const pendingPromptAnchorRef = useRef(false);
  const followLatestOutputRef = useRef(false);
  const latestOutputFollowPausedByUserRef = useRef(false);
  const latestOutputFollowResumeIntentRef = useRef(false);
  const runStageTouchStartYRef = useRef<number | null>(null);
  const runStageInputIntentCleanupRef = useRef<(() => void) | null>(null);
  const runStageResizeObserverRef = useRef<ResizeObserver | null>(null);
  const rendererReadyNotifiedRef = useRef(false);
  const threadLoadSeqRef = useRef(0);
  const runStageScrollRef = useRef<HTMLDivElement | null>(null);
  const composerDockRef = useRef<HTMLDivElement | null>(null);
  const runtimeClient = runtimeClientRef.current;

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const [composerValue, setComposerValue] = useState("");
  const [composerFocusRequest, setComposerFocusRequest] = useState(0);
  const [activeTurnMarkerId, setActiveTurnMarkerId] = useState<string | undefined>();
  const [activeNavId, setActiveNavId] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | null>(null);
  const [latestTurnMinHeightPx, setLatestTurnMinHeightPx] = useState(0);
  const [composerContentBottomInsetPx, setComposerContentBottomInsetPx] = useState(0);
  const [runStageViewportHeightPx, setRunStageViewportHeightPx] = useState(0);
  const [resolvingApprovalId, setResolvingApprovalId] = useState<string | null>(null);
  const [resolvedApproval, setResolvedApproval] = useState<ResolvedApprovalState>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
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
  const [inspectorSelection, setInspectorSelection] = useState<InspectorSelection | undefined>();
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
  const narrativeItems: RunNarrativeItem[] = [
    ...runtimeState.narrative.committedItems,
    ...(runtimeState.narrative.activeItem ? [runtimeState.narrative.activeItem] : []),
  ];
  const visibleNarrativeItems = narrativeItems.filter((item) => item.kind !== "planning");
  const narrativeSections = groupNarrativeByTurns(visibleNarrativeItems);
  const latestTurn = narrativeSections.turns[narrativeSections.turns.length - 1];
  const latestPromptId = latestTurn ? latestTurn.id : undefined;
  const hasPromptNarrative = narrativeSections.turns.length > 0;
  const fallbackFooterAnswerId = latestDoneAnswerId(visibleNarrativeItems);
  const composerSafeAreaPx = composerBottomSafeArea({
    composerHeight: hasPromptNarrative ? composerContentBottomInsetPx : 0,
    safeGap: hasPromptNarrative ? RUN_STAGE_COMPOSER_SAFE_GAP : 0,
  });

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
  }, [inspectorCollapsed]);

  useEffect(() => {
    let cancelled = false;

    async function poll(): Promise<void> {
      if (runtimeState.supervisorStatus !== "running") {
        setRuntimeInfo(null);
        return;
      }
      try {
        const info = await runtimeClient.getRuntimeInfo();
        if (!cancelled) {
          setRuntimeInfo(info);
        }
      } catch {
        if (!cancelled) {
          setRuntimeInfo(null);
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
    setInspectorSelection(undefined);
    setInspectorTab("changes");
  }, [runtimeState.runId, runtimeState.activeThreadId]);

  useEffect(() => {
    if (!runtimeState.narrative.inspectorSelection) {
      return;
    }
    setInspectorSelection(runtimeState.narrative.inspectorSelection);
    setInspectorTab(defaultInspectorTab(runtimeState.narrative.inspectorSelection));
  }, [runtimeState.narrative.inspectorSelection]);

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
  const a2uiArtifacts = runtimeState.artifacts.filter(
    (artifact) => artifact.kind === "a2ui_surface",
  );
  const navItems: NavItem[] = [
    {
      id: "automations",
      label: "Automations",
      icon: <CalendarClock size={16} strokeWidth={1.9} />,
    },
    {
      id: "customize",
      label: "Customize",
      icon: <SlidersHorizontal size={16} strokeWidth={1.9} />,
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
  const composerMetaItems: ComposerBarMetaItem[] = [
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
  const selectedToolResult =
    inspectorSelection?.kind === "tool"
      ? runtimeState.toolResults.find((tool) => tool.id === rawToolId(inspectorSelection.id))
      : undefined;
  const selectedArtifactId =
    inspectorSelection?.kind === "artifact"
      ? inspectorSelection.id
      : inspectorSelection?.kind === "fileChange"
        ? rawFileChangeId(inspectorSelection.id)
        : undefined;
  const selectedArtifact = selectedArtifactId
    ? runtimeState.artifacts.find((artifact) => artifact.id === selectedArtifactId)
    : undefined;
  const inspectorTabs = [
    {
      id: "changes" as const,
      label: "Changes",
      icon: <GitBranch size={16} strokeWidth={1.85} />,
      tone: "neutral" as const,
    },
    {
      id: "browser" as const,
      label: "Browser",
      icon: <Globe size={16} strokeWidth={1.85} />,
    },
    {
      id: "files" as const,
      label: "Files",
      badge: runtimeState.artifacts.length || undefined,
      icon: <Files size={16} strokeWidth={1.85} />,
    },
  ];
  const showCenterComposer = shouldUseCenterComposer({
    hasNarrative: visibleNarrativeItems.length > 0,
    hasRun: Boolean(runtimeState.runId),
    hasNotice: Boolean(notice),
    hasError: Boolean(runtimeState.errorMessage),
    needsRestart: desktopNeedsRestart,
  });
  const markerActiveTurnId =
    activeTurnMarkerId &&
    narrativeSections.turns.some((turn) => turn.id === activeTurnMarkerId)
      ? activeTurnMarkerId
      : latestPromptId;
  const turnMarkerDisplay = turnMarkerRailDisplay({
    showCenterComposer,
    turnIds: narrativeSections.turns.map((turn) => turn.id),
    activeTurnId: markerActiveTurnId,
    latestTurnId: latestPromptId,
    railHeight: Math.max(0, runStageViewportHeightPx - 28 - Math.max(28, composerSafeAreaPx)),
  });
  const showTurnMarkerRail = turnMarkerDisplay.mode !== "hidden";

  function selectInspector(selection: InspectorSelection, tabId = defaultInspectorTab(selection)): void {
    setInspectorSelection(selection);
    setInspectorTab(tabId);
  }

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

  const setRunStageScrollElement = useCallback((element: HTMLDivElement | null) => {
    runStageInputIntentCleanupRef.current?.();
    runStageInputIntentCleanupRef.current = null;
    runStageResizeObserverRef.current?.disconnect();
    runStageResizeObserverRef.current = null;
    runStageScrollRef.current = element;
    if (!element) {
      setRunStageViewportHeightPx((current) => current === 0 ? current : 0);
      return;
    }
    element.style.setProperty("--aw-composer-safe-area", `${composerSafeAreaPx}px`);

    const syncRunStageViewportHeight = () => {
      const nextHeight = element.clientHeight;
      setRunStageViewportHeightPx((current) => current === nextHeight ? current : nextHeight);
    };
    syncRunStageViewportHeight();
    if (typeof ResizeObserver !== "undefined") {
      const resizeObserver = new ResizeObserver(syncRunStageViewportHeight);
      resizeObserver.observe(element);
      runStageResizeObserverRef.current = resizeObserver;
    }

    const handleWheel = (event: WheelEvent) => {
      if (shouldPauseLatestOutputFollowForWheel({ deltaY: event.deltaY })) {
        latestOutputFollowPausedByUserRef.current = true;
        latestOutputFollowResumeIntentRef.current = false;
        followLatestOutputRef.current = false;
        return;
      }
      if (shouldRequestLatestOutputFollowResumeForWheel({ deltaY: event.deltaY })) {
        latestOutputFollowResumeIntentRef.current = true;
      }
    };
    const handleTouchStart = (event: TouchEvent) => {
      runStageTouchStartYRef.current = event.touches[0]?.clientY ?? null;
    };
    const handleTouchMove = (event: TouchEvent) => {
      const startY = runStageTouchStartYRef.current;
      const currentY = event.touches[0]?.clientY;
      if (
        startY !== null &&
        currentY !== undefined &&
        shouldPauseLatestOutputFollowForTouchMove({ startY, currentY })
      ) {
        latestOutputFollowPausedByUserRef.current = true;
        latestOutputFollowResumeIntentRef.current = false;
        followLatestOutputRef.current = false;
        return;
      }
      if (
        startY !== null &&
        currentY !== undefined &&
        shouldRequestLatestOutputFollowResumeForTouchMove({ startY, currentY })
      ) {
        latestOutputFollowResumeIntentRef.current = true;
      }
    };

    element.addEventListener("wheel", handleWheel, { passive: true });
    element.addEventListener("touchstart", handleTouchStart, { passive: true });
    element.addEventListener("touchmove", handleTouchMove, { passive: true });
    runStageInputIntentCleanupRef.current = () => {
      element.removeEventListener("wheel", handleWheel);
      element.removeEventListener("touchstart", handleTouchStart);
      element.removeEventListener("touchmove", handleTouchMove);
    };
  }, [composerSafeAreaPx]);

  function promptAnchorOffset(element: HTMLElement): number {
    const value = window
      .getComputedStyle(element)
      .getPropertyValue("--aw-prompt-anchor-offset")
      .trim();
    const parsed = Number.parseFloat(value);
    return Number.isFinite(parsed) ? parsed : DEFAULT_PROMPT_ANCHOR_OFFSET;
  }

  function turnElements(element: HTMLElement): HTMLElement[] {
    return Array.from(
      element.querySelectorAll<HTMLElement>("[data-execution-turn-id]"),
    );
  }

  function findTurnElement(element: HTMLElement, turnId: string): HTMLElement | undefined {
    return turnElements(element).find((turn) => turn.dataset.executionTurnId === turnId);
  }

  function activeTurnMarkerFromScroll(element: HTMLElement): string | undefined {
    const containerTop = element.getBoundingClientRect().top;
    const anchorY = containerTop + promptAnchorOffset(element) + 1;
    return activeTurnIdForAnchor({
      anchorY,
      turns: turnElements(element)
        .map((turn) => ({
          id: turn.dataset.executionTurnId ?? "",
          top: turn.getBoundingClientRect().top,
        }))
        .filter((turn) => turn.id.length > 0),
    });
  }

  function syncActiveTurnMarker(element: HTMLElement): void {
    const nextActiveId = activeTurnMarkerFromScroll(element);
    if (!nextActiveId) {
      return;
    }
    setActiveTurnMarkerId((current) => current === nextActiveId ? current : nextActiveId);
  }

  function latestTurnOutputBottom(turn: HTMLElement): number {
    const body = turn.querySelector<HTMLElement>(".execution-turn-body");
    const latestBodyItem = body?.lastElementChild;
    if (latestBodyItem instanceof HTMLElement) {
      return latestBodyItem.getBoundingClientRect().bottom;
    }

    const header = turn.querySelector<HTMLElement>(".execution-turn-header");
    return (header ?? turn).getBoundingClientRect().bottom;
  }

  function measureLatestTurnMinHeight(promptId: string): void {
    const element = runStageScrollRef.current;
    if (!element || !findTurnElement(element, promptId)) {
      setLatestTurnMinHeightPx(0);
      return;
    }

    setLatestTurnMinHeightPx(
      Math.ceil(
        anchoredTurnMinHeight({
          containerHeight: element.clientHeight,
          anchorOffset: promptAnchorOffset(element),
          bottomReserve: composerSafeAreaPx,
        }),
      ),
    );
  }

  function scrollPromptToAnchor(promptId: string): void {
    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }

    const turn = findTurnElement(element, promptId);
    if (!turn) {
      return;
    }

    const elementRect = element.getBoundingClientRect();
    const turnRect = turn.getBoundingClientRect();
    element.scrollTo({
      top: scrollTopForPromptAnchor({
        currentScrollTop: element.scrollTop,
        containerTop: elementRect.top,
        promptTop: turnRect.top,
        anchorOffset: promptAnchorOffset(element),
      }),
      behavior: "auto",
    });
  }

  function scrollLatestTurnOutputToTail(promptId: string): void {
    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }

    const turn = findTurnElement(element, promptId);
    if (!turn) {
      return;
    }

    const elementRect = element.getBoundingClientRect();
    const turnRect = turn.getBoundingClientRect();
    const anchorScrollTop = scrollTopForPromptAnchor({
      currentScrollTop: element.scrollTop,
      containerTop: elementRect.top,
      promptTop: turnRect.top,
      anchorOffset: promptAnchorOffset(element),
    });
    const nextScrollTop = scrollTopForLatestTurnOutputTail({
      currentScrollTop: element.scrollTop,
      containerTop: elementRect.top,
      outputBottom: latestTurnOutputBottom(turn),
      containerHeight: element.clientHeight,
      bottomReserve: composerSafeAreaPx,
      anchorScrollTop,
    });

    if (Math.abs(nextScrollTop - element.scrollTop) < 1) {
      return;
    }

    element.scrollTo({
      top: nextScrollTop,
      behavior: "auto",
    });
  }

  function handleTurnMarkerClick(turnId: string): void {
    latestOutputFollowPausedByUserRef.current = true;
    latestOutputFollowResumeIntentRef.current = false;
    followLatestOutputRef.current = false;
    setActiveTurnMarkerId(turnId);
    scrollPromptToAnchor(turnId);
  }

  function handleRunStageScroll(event: ReactUIEvent<HTMLDivElement>): void {
    const element = event.currentTarget;
    if (showTurnMarkerRail) {
      syncActiveTurnMarker(element);
    }

    if (!latestPromptId) {
      followLatestOutputRef.current = false;
      return;
    }

    const turn = findTurnElement(element, latestPromptId);
    if (!turn) {
      followLatestOutputRef.current = false;
      return;
    }

    const elementRect = element.getBoundingClientRect();
    const isNearTail = isNearLatestTurnOutputTail({
      containerTop: elementRect.top,
      outputBottom: latestTurnOutputBottom(turn),
      containerHeight: element.clientHeight,
      bottomReserve: composerSafeAreaPx,
      threshold: LATEST_OUTPUT_FOLLOW_THRESHOLD,
    });
    const shouldResume = shouldResumeLatestOutputFollow({
      isPausedByUser: latestOutputFollowPausedByUserRef.current,
      isNearTail,
      hasResumeIntent: latestOutputFollowResumeIntentRef.current,
    });

    if (shouldResume) {
      latestOutputFollowPausedByUserRef.current = false;
      latestOutputFollowResumeIntentRef.current = false;
    }
    followLatestOutputRef.current = shouldResume;
  }

  useLayoutEffect(() => {
    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }
    element.style.setProperty("--aw-composer-safe-area", `${composerSafeAreaPx}px`);
  }, [composerSafeAreaPx]);

  useLayoutEffect(() => {
    if (showCenterComposer) {
      setComposerContentBottomInsetPx(0);
      return;
    }

    const dockElement = composerDockRef.current;
    if (!dockElement) {
      return;
    }
    const measuredDockElement: HTMLDivElement = dockElement;

    function syncComposerBottomInset(): void {
      const dockRect = measuredDockElement.getBoundingClientRect();
      const contentElement =
        measuredDockElement.querySelector<HTMLElement>(".composer-dock-stack") ??
        measuredDockElement.firstElementChild;
      const contentTop =
        contentElement instanceof HTMLElement
          ? contentElement.getBoundingClientRect().top
          : undefined;
      setComposerContentBottomInsetPx(
        Math.ceil(
          composerContentBottomInset({
            dockBottom: dockRect.bottom,
            contentTop,
            fallbackInset: dockRect.height,
          }),
        ),
      );
    }

    syncComposerBottomInset();
    const resizeObserver = new ResizeObserver(syncComposerBottomInset);
    resizeObserver.observe(measuredDockElement);

    return () => {
      resizeObserver.disconnect();
    };
  }, [showCenterComposer]);

  useLayoutEffect(() => {
    const element = runStageScrollRef.current;
    if (!latestPromptId || !element) {
      pendingPromptAnchorRef.current = false;
      setLatestTurnMinHeightPx((current) => current === 0 ? current : 0);
      return;
    }

    const turn = findTurnElement(element, latestPromptId);
    if (!turn) {
      setLatestTurnMinHeightPx((current) => current === 0 ? current : 0);
      return;
    }

    const nextMinHeight = Math.ceil(
      anchoredTurnMinHeight({
        containerHeight: element.clientHeight,
        anchorOffset: promptAnchorOffset(element),
        bottomReserve: composerSafeAreaPx,
      }),
    );
    if (nextMinHeight !== latestTurnMinHeightPx) {
      setLatestTurnMinHeightPx(nextMinHeight);
      return;
    }

    if (pendingPromptAnchorRef.current) {
      pendingPromptAnchorRef.current = false;
      scrollPromptToAnchor(latestPromptId);
      return;
    }

    if (followLatestOutputRef.current && !latestOutputFollowPausedByUserRef.current) {
      scrollLatestTurnOutputToTail(latestPromptId);
    }
  }, [
    composerSafeAreaPx,
    latestPromptId,
    latestTurnMinHeightPx,
    runtimeState.narrative,
  ]);

  useLayoutEffect(() => {
    if (!latestPromptId) {
      return;
    }

    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }

    const resizeObserver = new ResizeObserver(() => {
      measureLatestTurnMinHeight(latestPromptId);
    });
    resizeObserver.observe(element);

    return () => {
      resizeObserver.disconnect();
    };
  }, [composerSafeAreaPx, latestPromptId]);

  useEffect(() => {
    return () => {
      runStageInputIntentCleanupRef.current?.();
      runStageInputIntentCleanupRef.current = null;
      runStageResizeObserverRef.current?.disconnect();
      runStageResizeObserverRef.current = null;
    };
  }, []);

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
    pendingPromptAnchorRef.current = true;
    followLatestOutputRef.current = true;
    latestOutputFollowPausedByUserRef.current = false;
    latestOutputFollowResumeIntentRef.current = false;
    setLatestTurnMinHeightPx(0);
    dispatch({ type: "run/requested", prompt, startedAtMs: Date.now() });

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
      setComposerValue("");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to create run";
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
    pendingPromptAnchorRef.current = false;
    followLatestOutputRef.current = false;
    latestOutputFollowPausedByUserRef.current = false;
    latestOutputFollowResumeIntentRef.current = false;
    setLatestTurnMinHeightPx(0);
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
    pendingPromptAnchorRef.current = false;
    followLatestOutputRef.current = false;
    latestOutputFollowPausedByUserRef.current = false;
    latestOutputFollowResumeIntentRef.current = false;
    setLatestTurnMinHeightPx(0);
    terminalSeenRef.current = false;
    localStorage.removeItem("awaken:runId");
    dispatch({ type: "run/reset" });
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
      const showAnswerFooter =
        item.status === "done" &&
        (item.isFinal === true || (!isBusy && item.id === fallbackFooterAnswerId));
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

  function renderInspectorEmpty(title: string, detail: string) {
    return (
      <div className="inspector-empty-state inspector-empty-state-quiet">
        <strong>{title}</strong>
        <p>{detail}</p>
      </div>
    );
  }

  function renderInspectorCopy(text: string, label: string) {
    if (!text) {
      return null;
    }
    return (
      <div className="inspector-copy-row">
        <CopyButton text={text} label={label} />
      </div>
    );
  }

  function renderToolDetail(tool = selectedToolResult) {
    if (!tool) {
      return null;
    }
    const toolOutput = [
      "args:",
      tool.args || "No tool args captured.",
      "",
      "command output:",
      tool.commandOutput || "No command output captured.",
      tool.commandOutputTruncated ? "[command output truncated]" : "",
      "",
      "result:",
      tool.content || "No tool result captured.",
    ]
      .filter((line, index, lines) => line !== "" || lines[index - 1] !== "")
      .join("\n");
    return (
      <article className="inspector-item inspector-item-selected">
        <Disclosure title={tool.title} meta={tool.status || "done"} defaultOpen>
          {renderInspectorCopy(toolOutput, "Copy tool output")}
          <pre>{toolOutput}</pre>
        </Disclosure>
      </article>
    );
  }

  function renderChangesPane() {
    return null;
  }

  function renderBrowserPane() {
    return renderInspectorEmpty(
      "Browser unavailable",
      "No browser runtime is attached to this run yet.",
    );
  }

  function renderFilesPane() {
    if (selectedToolResult) {
      return (
        <section className="inspector-section">
          <div className="inspector-section-head">
            <h2>Tool File Output</h2>
            <span>{selectedToolResult.status || "done"}</span>
          </div>
          {renderToolDetail(selectedToolResult)}
        </section>
      );
    }

    if (selectedArtifact) {
      return (
        <section className="inspector-section">
          <div className="inspector-section-head">
            <h2>File</h2>
            <span>{selectedArtifact.status}</span>
          </div>
          <article className="inspector-item inspector-item-selected">
            <strong>{selectedArtifact.title}</strong>
            <span>{selectedArtifact.path || selectedArtifact.kind}</span>
            {renderInspectorCopy(selectedArtifact.path || "", "Copy path")}
            <pre>{selectedArtifact.content || "No file content captured."}</pre>
          </article>
        </section>
      );
    }

    return null;
  }

  function renderInspectorTabPanel() {
    switch (inspectorTab) {
      case "browser":
        return renderBrowserPane();
      case "files":
        return renderFilesPane();
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
                    onClick={() => setInspectorCollapsed((collapsed) => !collapsed)}
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
                onSelectNav={setActiveNavId}
                onSelectProject={handleSelectProject}
                onExpandedChange={setExpandedProjectIds}
                onNewTask={handleNewTask}
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
                                marker.id === markerActiveTurnId ? " turn-marker-button-active" : ""
                              }`}
                              aria-current={marker.id === markerActiveTurnId ? "true" : undefined}
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
                    {desktopNeedsRestart ? (
                      <div className="runtime-panel runtime-panel-error">
                        <strong>Awaken needs restart</strong>
                        <p>Quit Awaken and open it again.</p>
                      </div>
                    ) : null}

                    {desktopStarting ? (
                      <div className="runtime-panel">
                        <strong>Starting Awaken</strong>
                        <p>Preparing the desktop app.</p>
                      </div>
                    ) : null}

                    {runtimeState.errorMessage ? (
                      <div className="runtime-panel runtime-panel-error">
                        <strong>Run error</strong>
                        <p>{runtimeState.errorMessage}</p>
                      </div>
                    ) : null}

                    {notice ? (
                      <div className="runtime-panel">
                        <strong>Notice</strong>
                        <p>{notice}</p>
                      </div>
                    ) : null}

                    {showCenterComposer ? (
                      <div className="run-stage-center-composer">
                        <TaskWelcome />
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

                    {a2uiArtifacts.length > 0 ? (
                      <section className="inspector-section">
                        <div className="inspector-section-head">
                          <h2>A2UI</h2>
                          <span>{a2uiArtifacts.length}</span>
                        </div>
                        <div className="a2ui-stack">
                          {a2uiArtifacts.map((artifact) => (
                            <A2UISurfaceView
                              key={artifact.id}
                              title={artifact.title}
                              content={artifact.content}
                            />
                          ))}
                        </div>
                      </section>
                    ) : null}
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
    </>
  );
}
