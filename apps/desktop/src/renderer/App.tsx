import { useEffect, useReducer, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import {
  CalendarClock,
  Files,
  FolderPlus,
  GitBranch,
  Globe,
  ListFilter,
  PanelRightClose,
  PanelRightOpen,
  SlidersHorizontal,
  Terminal,
} from "lucide-react";
import {
  AwakenAppShell as AppShell,
  AwakenApprovalCard as ApprovalCard,
  AwakenAssistantMessage as AssistantMessage,
  AwakenComposerBar as ComposerBar,
  AwakenCopyButton as CopyButton,
  AwakenDisclosure as Disclosure,
  AwakenExecutionFeed as ExecutionFeed,
  AwakenFileChangeSummary as FileChangeSummary,
  AwakenLightInspectorRail as LightInspectorRail,
  AwakenSidebar as Sidebar,
  AwakenThoughtTrace as ThoughtTrace,
  AwakenToolTraceGroup as ToolTraceGroup,
  AwakenToolTraceLine as ToolTraceLine,
  AwakenUserPromptBlock as UserPromptBlock,
  AwakenWorkbenchChrome as WorkbenchChrome,
  AwakenWorkbenchShell as WorkbenchShell,
  AwakenWorkbenchSidebarFrame as WorkbenchSidebarFrame,
  AwakenRunStage as RunStage,
  AwakenComposerDock as ComposerDock,
  useAwakenAutoScrollTail as useAutoScrollTail,
  type ComponentTone,
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
} from "./runtime/reducer";
import type { ApprovalDecision } from "../shared/runtime";
import { A2UISurfaceView } from "./a2ui/A2UISurfaceView";
import type { InspectorSelection, RunNarrativeItem } from "./run-narrative";
import {
  diffLineStats,
  resolvedDecisionForApproval,
  shouldUseCenterComposer,
  type ResolvedApprovalState,
} from "./workbenchPresentation";

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
      return "terminal";
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

function rawApprovalId(selectionId: string): string {
  return selectionId.startsWith("approval:")
    ? selectionId.slice("approval:".length)
    : selectionId;
}

function rawFileChangeId(selectionId: string): string {
  return selectionId.startsWith("fileChange:")
    ? selectionId.slice("fileChange:".length)
    : selectionId;
}

const DEFAULT_SIDEBAR_WIDTH = 300;
const MIN_SIDEBAR_WIDTH = 228;
const MAX_SIDEBAR_WIDTH = 420;
const DEFAULT_INSPECTOR_WIDTH = 300;
const MIN_INSPECTOR_WIDTH = 260;
const MAX_INSPECTOR_WIDTH = 520;
const LAYOUT_STEP = 12;

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
  const threadLoadSeqRef = useRef(0);
  const runtimeClient = runtimeClientRef.current;

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const [composerValue, setComposerValue] = useState("");
  const [activeNavId, setActiveNavId] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | null>(null);
  const [resolvingApprovalId, setResolvingApprovalId] = useState<string | null>(null);
  const [resolvedApproval, setResolvedApproval] = useState<ResolvedApprovalState>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
  const [workspacePath, setWorkspacePath] = useState<string>("");
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
  const runTail = useAutoScrollTail<HTMLDivElement>({
    dependencies: [runtimeState.narrative],
    resetKeys: [runtimeState.runId, runtimeState.activeThreadId],
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
        let recoveredPrompt: string | undefined;
        try {
          const threads = await runtimeClient.listThreads();
          if (cancelled) return;
          recoveredPrompt = threads.find((thread) => thread.threadId === run.threadId)?.label;
          dispatch({ type: "run/threadsLoaded", threads });
        } catch {
          // Recovery can still replay the run history even if thread listing fails.
        }

        // Always replay full history (after_seq=0) regardless of run status.
        // For terminal runs, the replay restores timeline/artifacts/final text,
        // and the terminal event in the stream clears localStorage via onEvent.
        dispatch({ type: "run/recovered", run, prompt: recoveredPrompt });

        const unsubscribe = await runtimeClient.subscribeRun(
          run.runId,
          createSubscribeHandlers(),
          0,
        );
        if (cancelled) {
          unsubscribe();
          return;
        }
        unsubscribeRef.current = unsubscribe;
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
  const latestToolResult =
    runtimeState.toolResults.length > 0
      ? runtimeState.toolResults[runtimeState.toolResults.length - 1]
      : undefined;
  const narrativeItems = [
    ...runtimeState.narrative.committedItems,
    ...(runtimeState.narrative.activeItem ? [runtimeState.narrative.activeItem] : []),
  ];
  const visibleNarrativeItems = narrativeItems.filter((item) => item.kind !== "planning");
  const topbarStatusLabel = desktopNeedsRestart
    ? "Needs restart"
    : desktopStarting
      ? "Starting"
      : undefined;
  const repositoryThreadItems: ProjectItem[] = runtimeState.threads.map((thread) => {
    const sidebarStatus = sidebarStatusForThread(thread);
    const timeLabel =
      thread.threadId === runtimeState.activeThreadId
        ? "active"
        : sidebarStatus.label === "idle"
          ? undefined
          : sidebarStatus.label;
    return {
      id: `thread:${thread.threadId}`,
      label: thread.label,
      timeLabel,
    };
  });
  const repositoryProjects: ProjectItem[] = [
    {
      id: "awaken",
      label: "awaken",
      children: repositoryThreadItems.length > 0 ? repositoryThreadItems : undefined,
    },
  ];
  const activeProjectId = runtimeState.activeThreadId
    ? `thread:${runtimeState.activeThreadId}`
    : "awaken";
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
  const selectedFileChange =
    inspectorSelection?.kind === "fileChange"
      ? narrativeItems.find(
          (item): item is Extract<RunNarrativeItem, { kind: "fileChange" }> =>
            item.kind === "fileChange" &&
            (item.id === inspectorSelection.id ||
              item.files.some((file) => file.id === selectedArtifactId)),
        )
      : undefined;
  const selectedApprovalId =
    inspectorSelection?.kind === "approval"
      ? rawApprovalId(inspectorSelection.id)
      : undefined;
  const selectedApproval = selectedApprovalId
    ? runtimeState.approvals.find((approval) => approval.id === selectedApprovalId)
    : undefined;
  const selectedApprovalItem =
    inspectorSelection?.kind === "approval"
      ? narrativeItems.find(
          (item): item is Extract<RunNarrativeItem, { kind: "approval" }> =>
            item.kind === "approval" && item.id === inspectorSelection.id,
        )
      : undefined;
  const latestPatchPreview = runtimeState.patchPreviews[runtimeState.patchPreviews.length - 1];
  const changeStats = diffLineStats(runtimeState.turnDiff?.diff || latestPatchPreview?.patch);
  const changesBadge =
    runtimeState.artifacts.length +
    runtimeState.patchPreviews.length +
    (runtimeState.turnDiff?.diff ? 1 : 0);
  const inspectorTabs = [
    {
      id: "changes" as const,
      label: "Changes",
      badge: changesBadge || undefined,
      icon: <GitBranch size={16} strokeWidth={1.85} />,
      deltaAdded: changeStats.added || undefined,
      deltaRemoved: changeStats.removed || undefined,
      tone: changesBadge ? "success" as const : "neutral" as const,
    },
    {
      id: "browser" as const,
      label: "Browser",
      icon: <Globe size={16} strokeWidth={1.85} />,
    },
    {
      id: "terminal" as const,
      label: "Terminal",
      badge: runtimeState.toolResults.length || undefined,
      icon: <Terminal size={16} strokeWidth={1.85} />,
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

  function selectInspector(selection: InspectorSelection, tabId = defaultInspectorTab(selection)): void {
    setInspectorSelection(selection);
    setInspectorTab(tabId);
  }

  function handleInspectorTargetKeyDown(
    event: ReactKeyboardEvent,
    selection: InspectorSelection,
    tabId?: InspectorTabId,
  ): void {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    selectInspector(selection, tabId);
  }

  function createSubscribeHandlers(): SubscribeHandlers {
    return {
      onEvent: (event) => {
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
        dispatch({ type: "run/event", event });
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
    dispatch({ type: "run/requested", prompt });

    try {
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
      return;
    }
    await handleSubmit();
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
    dispatch({ type: "run/threadSwitch", threadId });

    const thread = runtimeState.threads.find((item) => item.threadId === threadId);
    if (!thread?.runId) {
      localStorage.removeItem("awaken:runId");
      return;
    }

    try {
      const run = await runtimeClient.getRun(thread.runId);
      if (threadLoadSeqRef.current !== loadSeq) {
        return;
      }
      dispatch({ type: "run/recovered", run, prompt: thread.label });
      if (run.status === "running" || run.status === "awaiting_approval" || run.status === "cancelling") {
        localStorage.setItem("awaken:runId", run.runId);
      } else {
        localStorage.removeItem("awaken:runId");
      }
      const unsubscribe = await runtimeClient.subscribeRun(
        run.runId,
        createSubscribeHandlers(),
        0,
      );
      if (threadLoadSeqRef.current !== loadSeq) {
        unsubscribe();
        return;
      }
      unsubscribeRef.current = unsubscribe;
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to load thread");
    }
  }

  function handleSelectProject(id: string): void {
    if (!id.startsWith("thread:")) {
      return;
    }
    void handleSelectConversation(id.slice("thread:".length));
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

  function renderToolDetail(tool = selectedToolResult ?? latestToolResult) {
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

  function renderPatchPreviewArticle() {
    if (!latestPatchPreview) {
      return null;
    }
    return (
      <article className="inspector-item inspector-item-selected">
        <strong>{latestPatchPreview.summary || latestPatchPreview.title}</strong>
        <span>{latestPatchPreview.affectedPaths.join(", ") || "No paths"}</span>
        {renderInspectorCopy(latestPatchPreview.patch, "Copy patch")}
        <pre>{latestPatchPreview.patch || "No patch captured."}</pre>
      </article>
    );
  }

  function renderTurnDiffArticle() {
    if (!runtimeState.turnDiff?.diff) {
      return null;
    }
    return (
      <article className="inspector-item inspector-item-selected">
        <strong>Turn diff</strong>
        <span>{runtimeState.turnDiff.truncated ? "truncated" : "ready"}</span>
        {renderInspectorCopy(runtimeState.turnDiff.diff, "Copy diff")}
        <pre>{runtimeState.turnDiff.diff}</pre>
      </article>
    );
  }

  function renderChangesPane() {
    const fileChangeFiles = selectedFileChange?.kind === "fileChange"
      ? selectedFileChange.files
      : [];
    const selectedChangePath = selectedArtifact?.path || fileChangeFiles[0]?.path || "";
    if (selectedApproval || selectedApprovalItem?.kind === "approval") {
      const affectedPaths = selectedApproval?.affectedPaths?.join("\n") || "";
      return (
        <section className="inspector-section">
          <div className="inspector-section-head">
            <h2>Approval</h2>
            <span>{selectedApprovalItem?.kind === "approval" ? selectedApprovalItem.status : "pending"}</span>
          </div>
          <article className="inspector-item inspector-item-selected">
            <strong>{selectedApproval?.title || selectedApprovalItem?.title || "Approval"}</strong>
            <span>{selectedApproval?.riskTone || "decision"}</span>
            {selectedApproval?.patchSummary || latestPatchPreview?.summary ? (
              <p>{selectedApproval?.patchSummary || latestPatchPreview?.summary}</p>
            ) : null}
            {affectedPaths ? (
              <>
                {renderInspectorCopy(affectedPaths, "Copy affected paths")}
                <pre>{affectedPaths}</pre>
              </>
            ) : (
              <p className="placeholder-copy">No affected paths captured.</p>
            )}
            {latestPatchPreview?.patch ? (
              <>
                {renderInspectorCopy(latestPatchPreview.patch, "Copy patch")}
                <pre>{latestPatchPreview.patch}</pre>
              </>
            ) : null}
          </article>
        </section>
      );
    }

    if (selectedArtifact || fileChangeFiles.length > 0) {
      return (
        <section className="inspector-section">
          <div className="inspector-section-head">
            <h2>Selected Change</h2>
            <span>{selectedArtifact?.status || fileChangeFiles[0]?.status || "ready"}</span>
          </div>
          <article className="inspector-item inspector-item-selected">
            <strong>{selectedArtifact?.title || fileChangeFiles[0]?.title || "File change"}</strong>
            <span>{selectedChangePath || "No path"}</span>
            {renderInspectorCopy(selectedChangePath, "Copy path")}
            <pre>
              {[
                selectedArtifact?.path ? `path: ${selectedArtifact.path}` : "",
                selectedArtifact?.kind ? `change: ${selectedArtifact.kind}` : "",
                selectedArtifact?.status ? `status: ${selectedArtifact.status}` : "",
                selectedArtifact?.description || fileChangeFiles[0]?.description || "",
                selectedArtifact?.content || "",
              ]
                .filter(Boolean)
                .join("\n") || "No change summary captured."}
            </pre>
          </article>
          {renderPatchPreviewArticle()}
          {renderTurnDiffArticle()}
        </section>
      );
    }

    if (latestPatchPreview || runtimeState.turnDiff?.diff) {
      return (
        <section className="inspector-section">
          <div className="inspector-section-head">
            <h2>Changes</h2>
            <span>
              {runtimeState.turnDiff?.diff
                ? runtimeState.turnDiff.truncated
                  ? "diff truncated"
                  : "diff ready"
                : "patch preview"}
            </span>
          </div>
          {renderPatchPreviewArticle()}
          {renderTurnDiffArticle()}
        </section>
      );
    }

    if (runtimeState.artifacts.length === 0) {
      return null;
    }

    return (
      <section className="inspector-section">
        <div className="inspector-section-head">
          <h2>Changes</h2>
          <span>{runtimeState.artifacts.length}</span>
        </div>
        <div className="inspector-list">
          {runtimeState.artifacts.map((artifact) => (
            <article key={artifact.id} className="inspector-item">
              <strong>{artifact.title}</strong>
              <span>{artifact.path || artifact.kind}</span>
            </article>
          ))}
        </div>
      </section>
    );
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
      case "terminal":
        return (
          <section className="inspector-section">
            <div className="inspector-section-head">
              <h2>Terminal</h2>
              <span>{selectedToolResult ? "selected" : "latest"}</span>
            </div>
            {renderToolDetail()}
          </section>
        );
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
                defaultExpandedIds={["awaken"]}
                profileName="Nolan"
                profileAvatarFallback="AW"
                profileSubtitle="Local"
                newTaskLabel="New Agent"
                conversationSectionLabel=""
                projectSectionLabel="Repositories"
                projectSectionActions={
                  <span className="sidebar-section-actions">
                    <button
                      type="button"
                      className="sidebar-section-action"
                      aria-label="Filter repositories"
                    >
                      <ListFilter size={13} strokeWidth={1.85} />
                    </button>
                    <button
                      type="button"
                      className="sidebar-section-action"
                      aria-label="Add repository"
                    >
                      <FolderPlus size={13} strokeWidth={1.85} />
                    </button>
                  </span>
                }
                showProjectCount={false}
                platform={platform}
                onSelectNav={setActiveNavId}
                onSelectProject={handleSelectProject}
                onNewTask={handleNewTask}
              />
            </WorkbenchSidebarFrame>
          }
          main={
            <div className="desktop-main">
              <WorkbenchShell
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
                stage={
                  <RunStage
                    scrollRef={showCenterComposer ? undefined : runTail.scrollRef}
                    onScroll={showCenterComposer ? undefined : runTail.onScroll}
                    composer={
                      showCenterComposer ? undefined : (
                        <ComposerDock>
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
                          />
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
                        />
                      </div>
                    ) : null}

                    {showCenterComposer ? null : (
                      <ExecutionFeed>
                        {visibleNarrativeItems.map((item, index) => {
                          const key = `${item.id}:${index}`;
                          if (item.kind === "prompt") {
                            return <UserPromptBlock key={key} text={item.text} />;
                          }
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
                            return (
                              <AssistantMessage
                                key={key}
                                markdown={item.markdown}
                                status={item.status}
                              />
                            );
                          }
                          if (item.kind === "exploring") {
                            return (
                              <ToolTraceGroup
                                key={key}
                                actions={item.actions}
                                onSelectAction={(action) => {
                                    const selection: InspectorSelection = {
                                      kind: "tool",
                                      id: action.id.startsWith("explore:")
                                        ? `tool:${action.id.slice("explore:".length)}`
                                        : action.id,
                                    };
                                    selectInspector(selection, "files");
                                  }}
                              />
                            );
                          }
                          if (item.kind === "tool") {
                            const selection: InspectorSelection = { kind: "tool", id: item.id };
                            return (
                              <ToolTraceLine
                                key={key}
                                title={item.title}
                                detail={item.detail}
                                status={item.status}
                                onSelect={() => selectInspector(selection, "terminal")}
                              />
                            );
                          }
                          if (item.kind === "approval") {
                            const approvalId = item.id.startsWith("approval:")
                              ? item.id.slice("approval:".length)
                              : item.id;
                            const itemApproval = runtimeState.approvals.find(
                              (approval) => approval.id === approvalId,
                            );
                            const canResolve =
                              item.status === "waiting" &&
                              activeApproval?.id === itemApproval?.id;
                            const resolvedDecision =
                              item.status === "approved" || item.status === "rejected"
                                ? item.status
                                : resolvedDecisionForApproval(itemApproval, resolvedApproval);
                            const selection: InspectorSelection = { kind: "approval", id: item.id };

                            return (
                              <div
                                key={key}
                                className="narrative-click-target"
                                onClick={() => selectInspector(selection, "changes")}
                              >
                                <ApprovalCard
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
                              </div>
                            );
                          }
                          if (item.kind === "fileChange") {
                            const selection: InspectorSelection = { kind: "fileChange", id: item.id };
                            return (
                              <div
                                key={key}
                                className="narrative-click-target"
                                role="button"
                                tabIndex={0}
                                onClick={() => selectInspector(selection, "changes")}
                                onKeyDown={(event) =>
                                  handleInspectorTargetKeyDown(event, selection, "changes")
                                }
                              >
                                <FileChangeSummary
                                  files={item.files}
                                  status={item.status}
                                />
                              </div>
                            );
                          }
                          return null;
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
    </>
  );
}
