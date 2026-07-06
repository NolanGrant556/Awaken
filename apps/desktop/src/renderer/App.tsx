import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import {
  ChevronDown,
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
  AwakenDisclosure as Disclosure,
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
  type ComposerBarMetaItem,
  type NavItem,
  type PlatformKind,
  type ProjectItem,
} from "@awaken/design-system";
import { createRuntimeClient, type SubscribeHandlers } from "./runtime/client";
import type {
  RuntimeInfo,
  RunInputAttachment,
  RuntimeModelProfileSummary,
  RuntimeRunStatus,
  RuntimeThread,
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
  projectNarrativeTurnBodySegments,
  resolvedDecisionForApproval,
  shouldShowAssistantAnswerFooter,
  shouldUseCenterComposer,
  turnMarkerRailDisplay,
  type NarrativeTurnBodySegment,
  type ResolvedApprovalState,
} from "./workbenchPresentation";
import { useComposerLifecycle } from "./useComposerLifecycle";
import { useRunStageScrollController } from "./useRunStageScrollController";
import {
  actionsForThreadTranscript,
  afterSeqForTranscriptRun,
} from "./runtime/thread-transcript";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  assignThreadToProject,
  isSameWorkspacePath,
  mergeWorkspaceProjects,
  projectIdForWorkspacePath,
  projectIdForThread,
  removeThreadProjectAssignments,
  removeWorkspaceProject,
  upsertWorkspaceProject,
  workspaceLabelForPath,
  type WorkspaceProject,
} from "./projectWorkspace";
import { ModelSettingsDialog } from "./ModelSettingsDialog";
import { ExtensionsView, type ExtensionSection } from "./ExtensionsView";
import {
  MAX_COMPOSER_IMAGE_ATTACHMENTS,
  MAX_COMPOSER_IMAGE_BYTES,
  SUPPORTED_COMPOSER_IMAGE_MIME_TYPES,
  type ComposerDraftAttachment,
  promptAttachmentViews,
  readComposerDraftImageAttachment,
  supportsVisionForProfile,
  toRunInputAttachment,
} from "./composerAttachments";

type InspectorTabId = "changes";

export function shouldShowSidebarThreadSpinner(thread: ThreadItem): boolean {
  if (
    thread.runStatus === "running" ||
    thread.runStatus === "cancelling"
  ) {
    return true;
  }
  if (thread.runStatus) {
    return false;
  }
  return Boolean(thread.activeRunId);
}

export function shouldResetActiveThreadForWorkspaceChange(
  activeThreadWorkspacePath?: string,
  nextWorkspacePath?: string,
): boolean {
  if (!activeThreadWorkspacePath || !nextWorkspacePath) {
    return false;
  }
  return !isSameWorkspacePath(activeThreadWorkspacePath, nextWorkspacePath);
}

export function resolveWorkspaceRootForRun(args: {
  workspacePath?: string;
  workspaceTargetPath?: string;
  selectedProjectId?: string;
  defaultWorkspacePath?: string;
  defaultProjectRemoved?: boolean;
  activeThreadWorkspacePath?: string;
  projectsFromWorkspace: WorkspaceProject[];
}): string {
  const {
    workspacePath,
    workspaceTargetPath,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  } = args;

  const explicitTarget = workspaceTargetPath?.trim();
  if (explicitTarget) {
    return explicitTarget;
  }

  const selectedProjectPath =
    selectedProjectId === undefined
      ? undefined
      : selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultProjectRemoved
          ? undefined
          : defaultWorkspacePath
        : projectsFromWorkspace.find(
            (project) => project.id === selectedProjectId,
          )?.path;

  return (
    [
      selectedProjectPath,
      activeThreadWorkspacePath,
      workspacePath,
      defaultProjectRemoved ? undefined : defaultWorkspacePath,
    ].find((candidate) => candidate?.trim())?.trim() ?? ""
  );
}

export function buildWorkspaceRegistryPersistRequest(args: {
  defaultWorkspacePath?: string;
  pathForSelectedProject?: string;
  activeThreadWorkspacePath?: string;
  projectsFromWorkspace: WorkspaceProject[];
  threadWorkspacePaths: string[];
  defaultProjectRemoved: boolean;
}): {
  defaultWorkspacePath: string;
  registeredWorkspacePaths: string[];
} {
  const {
    defaultWorkspacePath,
    pathForSelectedProject,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
    threadWorkspacePaths,
    defaultProjectRemoved,
  } = args;

  const shouldKeepDefaultWorkspace =
    !defaultProjectRemoved ||
    threadWorkspacePaths.some((path) =>
      isSameWorkspacePath(path, defaultWorkspacePath),
    );

  const registeredWorkspacePaths = dedupeWorkspacePaths([
    pathForSelectedProject,
    ...projectsFromWorkspace.map((project) => project.path),
    ...threadWorkspacePaths,
    shouldKeepDefaultWorkspace ? defaultWorkspacePath : undefined,
  ]);

  return {
    defaultWorkspacePath:
      activeThreadWorkspacePath ||
      pathForSelectedProject ||
      (shouldKeepDefaultWorkspace ? defaultWorkspacePath : undefined) ||
      registeredWorkspacePaths[0] ||
      "",
    registeredWorkspacePaths,
  };
}

function isThreadBusyForDelete(thread: ThreadItem): boolean {
  return Boolean(
    thread.activeRunId ||
    thread.runStatus === "running" ||
    thread.runStatus === "awaiting_approval" ||
    thread.runStatus === "cancelling",
  );
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
  contextWindow: ReturnType<
    typeof createInitialRuntimeState
  >["inspectorSummary"]["contextWindow"],
): Pick<ComposerBarMetaItem, "label" | "value" | "progress"> | undefined {
  const usageValue =
    typeof contextWindow?.actualTokens === "number"
      ? contextWindow.actualTokens
      : contextWindow?.estimatedTokens;
  const usage = formatTokenCount(usageValue);
  const modelWindow = formatTokenCount(contextWindow?.modelWindow);
  if (usage && modelWindow) {
    const progress =
      typeof usageValue === "number" &&
      typeof contextWindow?.modelWindow === "number" &&
      contextWindow.modelWindow > 0
        ? usageValue / contextWindow.modelWindow
        : undefined;
    return {
      label: "context windows",
      value: `${usage} / ${modelWindow}`,
      progress,
    };
  }
  if (usage) {
    return {
      label: "context windows",
      value: usage,
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
  return value
    ?.split("\n")
    .map((line) => line.trim())
    .find(Boolean);
}

function composerThreadLabel(
  prompt: string,
  attachments: Array<{ displayName?: string }>,
): string {
  const trimmed = prompt.trim();
  if (trimmed) {
    return trimmed;
  }
  const attachmentName = attachments
    .map((attachment) => attachment.displayName?.trim())
    .find(Boolean);
  return attachmentName || "Image prompt";
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

function readStoredStringArray(key: string): string[] {
  if (typeof window === "undefined" || !window.localStorage) {
    return [];
  }

  try {
    const parsed = JSON.parse(window.localStorage.getItem(key) ?? "[]");
    return Array.isArray(parsed)
      ? parsed.filter((item): item is string => typeof item === "string")
      : [];
  } catch {
    return [];
  }
}

function readStoredString(key: string, fallback = ""): string {
  if (typeof window === "undefined" || !window.localStorage) {
    return fallback;
  }
  try {
    const value = window.localStorage.getItem(key);
    return typeof value === "string" ? value : fallback;
  } catch {
    return fallback;
  }
}

function readStoredStringRecord(key: string): Record<string, string> {
  if (typeof window === "undefined" || !window.localStorage) {
    return {};
  }

  try {
    const parsed = JSON.parse(window.localStorage.getItem(key) ?? "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return {};
    }
    return Object.fromEntries(
      Object.entries(parsed).filter(
        (entry): entry is [string, string] =>
          typeof entry[0] === "string" && typeof entry[1] === "string",
      ),
    );
  } catch {
    return {};
  }
}

function readStoredWorkspaceProjects(key: string): WorkspaceProject[] {
  if (typeof window === "undefined" || !window.localStorage) {
    return [];
  }

  try {
    const parsed = JSON.parse(window.localStorage.getItem(key) ?? "[]");
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed
      .filter(
        (item): item is WorkspaceProject =>
          Boolean(item) &&
          typeof item === "object" &&
          typeof item.id === "string" &&
          typeof item.label === "string" &&
          typeof item.path === "string",
      )
      .map((project) => ({
        id: project.id,
        label: project.label,
        path: project.path,
      }));
  } catch {
    return [];
  }
}

function dedupeWorkspacePaths(paths: Array<string | undefined>): string[] {
  const nextPaths: string[] = [];
  for (const path of paths) {
    const trimmed = path?.trim();
    if (!trimmed || nextPaths.some((current) => isSameWorkspacePath(current, trimmed))) {
      continue;
    }
    nextPaths.push(trimmed);
  }
  return nextPaths;
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
  const persistedWorkspaceRegistryRef = useRef("");
  const workspaceTargetPathRef = useRef<string>(
    readStoredString("awaken:workspace:path"),
  );
  const runtimeClient = runtimeClientRef.current;

  const [runtimeState, dispatch] = useReducer(
    reduceRuntimeState,
    undefined,
    createInitialRuntimeState,
  );
  const [activeNavId, setActiveNavId] = useState<string | undefined>();
  const [notice, setNotice] = useState<string | null>(null);
  const [resolvingApprovalId, setResolvingApprovalId] = useState<string | null>(
    null,
  );
  const [resolvedApproval, setResolvedApproval] =
    useState<ResolvedApprovalState>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo | null>(null);
  const [modelProfiles, setModelProfiles] = useState<
    RuntimeModelProfileSummary[]
  >([]);
  const [modelSettingsOpen, setModelSettingsOpen] = useState(false);
  const [workspacePath, setWorkspacePath] = useState<string>(() =>
    readStoredString("awaken:workspace:path"),
  );
  const [createProjectDialogOpen, setCreateProjectDialogOpen] = useState(false);
  const [pendingProjectMode, setPendingProjectMode] =
    useState<"local">("local");
  const [pickingWorkspace, setPickingWorkspace] = useState(false);
  const [projectsFromWorkspace, setProjectsFromWorkspace] = useState<
    WorkspaceProject[]
  >(() => readStoredWorkspaceProjects("awaken:projects:workspace"));
  const [selectedProjectId, setSelectedProjectId] = useState<
    string | undefined
  >(undefined);
  const [expandedProjectIds, setExpandedProjectIds] = useState<string[]>(() => {
    const stored = readStoredStringArray("awaken:projects:expanded");
    return stored.includes(DEFAULT_WORKSPACE_PROJECT_ID)
      ? stored
      : [DEFAULT_WORKSPACE_PROJECT_ID, ...stored];
  });
  const [removedProjectIds, setRemovedProjectIds] = useState<string[]>(() =>
    readStoredStringArray("awaken:projects:removed"),
  );
  const defaultProjectRemoved = removedProjectIds.includes(
    DEFAULT_WORKSPACE_PROJECT_ID,
  );
  const [threadProjectIds, setThreadProjectIds] = useState<
    Record<string, string>
  >(() => readStoredStringRecord("awaken:threads:projects"));
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
  const [promptBubbleModeByTurnId, setPromptBubbleModeByTurnId] = useState<
    Record<string, "collapsed" | "expanded">
  >({});
  const [executionProcessOpenByTurnId, setExecutionProcessOpenByTurnId] =
    useState<Record<string, boolean>>({});
  const [composerAttachments, setComposerAttachments] = useState<
    ComposerDraftAttachment[]
  >([]);
  const visibleNarrativeItems = useMemo<RunNarrativeItem[]>(() => {
    const narrativeItems: RunNarrativeItem[] = [
      ...runtimeState.narrative.committedItems,
      ...(runtimeState.narrative.activeItem
        ? [runtimeState.narrative.activeItem]
        : []),
    ];
    return narrativeItems.filter((item) => item.kind !== "planning");
  }, [runtimeState.narrative]);

  const isBusy =
    runtimeState.phase === "starting" ||
    runtimeState.phase === "running" ||
    runtimeState.phase === "reconnecting" ||
    runtimeState.phase === "cancelling";
  const desktopReady =
    runtimeState.supervisorStatus === "running" && runtimeInfo !== null;
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
    writeStoredValue(
      "awaken:layout:inspectorCollapsed",
      String(inspectorCollapsed),
    );
    inspectorCollapsedValueRef.current = inspectorCollapsed;
  }, [inspectorCollapsed]);

  useEffect(() => {
    writeStoredValue(
      "awaken:projects:removed",
      JSON.stringify(removedProjectIds),
    );
  }, [removedProjectIds]);

  useEffect(() => {
    writeStoredValue("awaken:workspace:path", workspacePath);
    workspaceTargetPathRef.current = workspacePath;
  }, [workspacePath]);

  useEffect(() => {
    writeStoredValue(
      "awaken:projects:workspace",
      JSON.stringify(projectsFromWorkspace),
    );
  }, [projectsFromWorkspace]);

  useEffect(() => {
    writeStoredValue("awaken:projects:selected", selectedProjectId ?? "");
  }, [selectedProjectId]);

  useEffect(() => {
    writeStoredValue(
      "awaken:projects:expanded",
      JSON.stringify(expandedProjectIds),
    );
  }, [expandedProjectIds]);

  useEffect(() => {
    writeStoredValue(
      "awaken:threads:projects",
      JSON.stringify(threadProjectIds),
    );
  }, [threadProjectIds]);

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function"
    ) {
      return;
    }

    const media = window.matchMedia(INSPECTOR_RESPONSIVE_COLLAPSE_QUERY);
    const handleResponsiveInspector = (matches: boolean) => {
      if (matches) {
        if (inspectorCollapsedBeforeResponsiveRef.current === null) {
          inspectorCollapsedBeforeResponsiveRef.current =
            inspectorCollapsedValueRef.current;
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
    if (
      rendererReadyNotifiedRef.current ||
      (!desktopReady && !desktopNeedsRestart)
    ) {
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
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
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
    if (runtimeInfo && workspacePath === "" && !defaultProjectRemoved) {
      setWorkspacePath(runtimeInfo.defaultWorkspace);
    }
  }, [defaultProjectRemoved, runtimeInfo, workspacePath]);

  useEffect(() => {
    setInspectorTab("changes");
  }, [runtimeState.runId, runtimeState.activeThreadId]);

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

  async function loadThreadsIntoState(): Promise<RuntimeThread[]> {
    const threads = await runtimeClient.listThreads();
    dispatch({ type: "run/threadsLoaded", threads });
    return threads;
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
      setNotice(
        error instanceof Error ? error.message : "Failed to switch model",
      );
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
        prepareForThreadSwitch();
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

  function resetActiveThreadContext(): void {
    unsubscribeRef.current?.();
    unsubscribeRef.current = null;
    threadLoadSeqRef.current += 1;
    terminalSeenRef.current = false;
    setNotice(null);
    setResolvingApprovalId(null);
    setResolvedApproval(null);
    resolvingApprovalIdRef.current = null;
    localStorage.removeItem("awaken:runId");
    prepareForNewTask();
    dispatch({ type: "run/reset" });
  }

  const enabledModelProfiles = modelProfiles.filter(
    (profile) => profile.enabled,
  );
  const selectedModelProfile =
    enabledModelProfiles.find(
      (profile) => profile.id === runtimeInfo?.modelProfile?.id,
    ) ??
    enabledModelProfiles.find((profile) => profile.isDefault) ??
    enabledModelProfiles[0];
  const selectedModelSupportsVision =
    supportsVisionForProfile(selectedModelProfile);
  const composerAttachmentWarning =
    composerAttachments.length > 0 && !selectedModelSupportsVision
      ? "当前模型不支持图片输入，请切换模型或移除图片"
      : undefined;
  const defaultWorkspacePath = runtimeInfo?.defaultWorkspace ?? workspacePath;
  const activeThread = runtimeState.activeThreadId
    ? runtimeState.threads.find(
        (thread) => thread.threadId === runtimeState.activeThreadId,
      )
    : undefined;
  const activeThreadWorkspacePath = activeThread?.workspaceRoot;
  const activeThreadProjectId = activeThreadWorkspacePath
    ? projectIdForWorkspacePath(
        activeThreadWorkspacePath,
        defaultWorkspacePath,
      )
    : undefined;
  const pathForSelectedProject =
    selectedProjectId === undefined
      ? undefined
      : selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultProjectRemoved
          ? undefined
          : defaultWorkspacePath
        : projectsFromWorkspace.find(
            (project) => project.id === selectedProjectId,
          )?.path;
  const workspaceRootForRun = resolveWorkspaceRootForRun({
    workspacePath,
    workspaceTargetPath: workspaceTargetPathRef.current,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  });
  const isWorkspaceValid =
    workspaceRootForRun !== "" &&
    /^([A-Za-z]:[\\/]|\/)/.test(workspaceRootForRun);
  const {
    composerValue,
    setComposerValue,
    clearComposerValue,
    restoreComposerValue,
    trimmedComposerValue,
    composerFocusRequest,
    requestComposerFocus,
    composerBarState,
    composerRuntimePanels,
  } = useComposerLifecycle({
    desktopReady,
    desktopStarting,
    desktopNeedsRestart,
    isBusy,
    isWorkspaceValid,
    hasAttachments: composerAttachments.length > 0,
    inputAttachmentBlocked: Boolean(composerAttachmentWarning),
    runtimeErrorMessage: runtimeState.errorMessage,
  });

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
  const threadWorkspacePaths = useMemo(
    () =>
      runtimeState.threads
        .map((thread) => thread.workspaceRoot)
        .filter((path): path is string => Boolean(path)),
    [runtimeState.threads],
  );
  const workspaceRegistryRequest = useMemo(
    () =>
      buildWorkspaceRegistryPersistRequest({
        defaultWorkspacePath,
        pathForSelectedProject,
        activeThreadWorkspacePath,
        projectsFromWorkspace,
        threadWorkspacePaths,
        defaultProjectRemoved,
      }),
    [
      activeThreadWorkspacePath,
      defaultWorkspacePath,
      defaultProjectRemoved,
      pathForSelectedProject,
      projectsFromWorkspace,
      threadWorkspacePaths,
    ],
  );
  const workspaceRegistrySignature = JSON.stringify(workspaceRegistryRequest);

  useEffect(() => {
    setProjectsFromWorkspace((current) => {
      const next = mergeWorkspaceProjects(
        current,
        threadWorkspacePaths,
        defaultWorkspacePath,
      );
      const isSame =
        next.length === current.length &&
        next.every((project, index) => {
          const existing = current[index];
          return (
            existing?.id === project.id &&
            existing.path === project.path &&
            existing.label === project.label
          );
        });
      return isSame ? current : next;
    });
  }, [defaultWorkspacePath, runtimeState.threads]);

  useEffect(() => {
    const knownProjectIds = new Set([
      DEFAULT_WORKSPACE_PROJECT_ID,
      ...projectsFromWorkspace.map((project) => project.id),
    ]);
    setSelectedProjectId((current) =>
      current === undefined
        ? undefined
        : knownProjectIds.has(current)
          ? current
          : DEFAULT_WORKSPACE_PROJECT_ID,
    );
    setExpandedProjectIds((current) => {
      const next = current.filter((id) => knownProjectIds.has(id));
      return next.includes(DEFAULT_WORKSPACE_PROJECT_ID)
        ? next
        : [DEFAULT_WORKSPACE_PROJECT_ID, ...next];
    });
  }, [projectsFromWorkspace]);

  useEffect(() => {
    if (persistedWorkspaceRegistryRef.current === workspaceRegistrySignature) {
      return;
    }
    persistedWorkspaceRegistryRef.current = workspaceRegistrySignature;
    void runtimeClient.persistWorkspaceRegistry(workspaceRegistryRequest).catch(() => {
      // Workspace registry sync is best-effort.
    });
  }, [runtimeClient, workspaceRegistryRequest, workspaceRegistrySignature]);

  useEffect(() => {
    if (
      pathForSelectedProject &&
      !isSameWorkspacePath(workspacePath, pathForSelectedProject)
    ) {
      workspaceTargetPathRef.current = pathForSelectedProject;
      setWorkspacePath(pathForSelectedProject);
    }
  }, [pathForSelectedProject, workspacePath]);

  const projectLabelById = new Map<string, string>([
    [
      DEFAULT_WORKSPACE_PROJECT_ID,
      defaultWorkspacePath
        ? workspaceLabelForPath(defaultWorkspacePath)
        : "awaken",
    ],
    ...projectsFromWorkspace.map(
      (project) => [project.id, project.label] as const,
    ),
  ]);
  const removedProjectIdSet = new Set(removedProjectIds);
  const workspaceLabelProjectId = activeThreadProjectId ?? selectedProjectId;
  const workspaceLabelPath =
    activeThreadWorkspacePath ?? pathForSelectedProject;
  const activeProjectLabel =
    (workspaceLabelProjectId
      ? projectLabelById.get(workspaceLabelProjectId)
      : undefined) ??
    (workspaceLabelPath
      ? workspaceLabelForPath(workspaceLabelPath)
      : "awaken");
  const contextMeta = contextUsageMeta(
    runtimeState.inspectorSummary.contextWindow,
  );
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

  function projectMenuItems(projectId: string) {
    return [
      {
        id: "remove-folder",
        label: "移除文件夹",
        tone: "danger" as const,
        onSelect: () => {
          void deleteThreadsForProject(projectId, { removeProject: true });
        },
      },
      {
        id: "delete-folder-history",
        label: "全量删除历史会话",
        tone: "danger" as const,
        onSelect: () => {
          void deleteThreadsForProject(projectId, { removeProject: false });
        },
      },
    ];
  }

  function threadItemsForProject(projectId: string): ProjectItem[] {
    return threadsForProject(projectId).map((thread) => {
      return {
        id: `thread:${thread.threadId}`,
        kind: "thread",
        label: thread.label,
        isLoading: shouldShowSidebarThreadSpinner(thread),
        menuItems: [
          {
            id: "rename-thread",
            label: "重命名",
            onSelect: () => {
              void handleRenameThread(thread.threadId);
            },
          },
          {
            id: "delete-thread",
            label: "删除会话",
            tone: "danger",
            disabled: isThreadBusyForDelete(thread),
            onSelect: () => {
              void handleDeleteThread(thread.threadId);
            },
          },
        ],
      };
    });
  }

  const defaultProjectThreads = threadItemsForProject(
    DEFAULT_WORKSPACE_PROJECT_ID,
  );
  const repositoryProjects: ProjectItem[] = [
    ...(removedProjectIdSet.has(DEFAULT_WORKSPACE_PROJECT_ID)
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
            children:
              defaultProjectThreads.length > 0
                ? defaultProjectThreads
                : undefined,
          },
        ]),
    ...projectsFromWorkspace
      .filter(
        (project) => !isSameWorkspacePath(project.path, defaultWorkspacePath),
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
          children: threads.length > 0 ? threads : undefined,
        };
      }),
  ];
  const activeProjectId =
    runtimeState.activeThreadId
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
    setComposerDockElement,
    composerSafeAreaPx,
    handleReturnToLatestClick,
    handleRunStageScroll,
    handleTurnMarkerClick,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeSections,
    prepareForNewTask,
    prepareForRunStart,
    prepareForThreadSwitch,
    returnToLatestBottomPx,
    returnToLatestVisible,
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
      runStageViewportHeightPx +
        composerContentInsetPx -
        28 -
        Math.max(28, composerSafeAreaPx),
    ),
  });
  const showTurnMarkerRail = turnMarkerDisplay.mode !== "hidden";
  const activeExtensionSection: ExtensionSection | undefined =
    activeNavId === "connectors" || activeNavId === "skills"
      ? activeNavId
      : undefined;

  function createSubscribeHandlers(): SubscribeHandlers {
    return {
      onEvent: (event) => {
        const receivedAtMs = Date.now();
        if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
          terminalSeenRef.current = true;
          localStorage.removeItem("awaken:runId");
        }
        if (
          event.type === "RUN_STATUS_CHANGED" &&
          event.status === "cancelled"
        ) {
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
            decision:
              event.approvalDecision === "rejected" ? "rejected" : "approved",
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
    return (
      status === "running" ||
      status === "awaiting_approval" ||
      status === "cancelling"
    );
  }

  async function replayThreadTranscript(
    threadId: string,
    loadSeq: number,
  ): Promise<RuntimeRunStatus | undefined> {
    const transcript = await runtimeClient.getThreadTranscript(threadId);
    if (threadLoadSeqRef.current !== loadSeq) {
      return undefined;
    }

    syncProjectSelectionForWorkspace(
      transcript.thread.workspace.workspaceRoot,
    );
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
      setNotice(
        desktopNeedsRestart ? "Awaken needs restart" : "Awaken is starting",
      );
      return;
    }
    const prompt = trimmedComposerValue;
    const draftAttachments = composerAttachments.map((attachment) => ({
      ...attachment,
    }));
    const runInputAttachments = draftAttachments.map(toRunInputAttachment);
    if (!prompt && runInputAttachments.length === 0) {
      setNotice("请输入内容或粘贴图片");
      return;
    }
    if (isBusy) {
      return;
    }
    if (!isWorkspaceValid) {
      setNotice("Please enter a valid absolute workspace path.");
      return;
    }

    const workspaceRoot = resolveWorkspaceRootForRun({
      workspacePath,
      workspaceTargetPath: workspaceTargetPathRef.current,
      selectedProjectId,
      defaultWorkspacePath,
      activeThreadWorkspacePath,
      projectsFromWorkspace,
    });
    const validation = await runtimeClient.validateWorkspace(workspaceRoot);
    if (!validation.valid) {
      setNotice(validation.error);
      return;
    }
    if (composerAttachmentWarning) {
      setNotice(composerAttachmentWarning);
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
    dispatch({
      type: "run/requested",
      prompt,
      promptAttachments: runInputAttachments,
      startedAtMs: Date.now(),
    });
    clearComposerValue();
    setComposerAttachments([]);

    try {
      const runProjectId = selectedProjectId ?? DEFAULT_WORKSPACE_PROJECT_ID;
      const nextThreadLabel = composerThreadLabel(prompt, draftAttachments);
      const startRunRequest: StartRunRequest = {
        input: prompt,
        attachments: runInputAttachments,
        threadId: runtimeState.activeThreadId || undefined,
        workspace: {
          workspaceRoot,
          readableRoots: workspaceRoot ? [workspaceRoot] : [],
          writableRoots: workspaceRoot ? [workspaceRoot] : [],
          outputRoots: workspaceRoot ? [workspaceRoot] : [],
        },
        client: {
          sessionId: window.crypto.randomUUID(),
          requestId: window.crypto.randomUUID(),
          entrypoint: "desktop",
        },
      };
      const run = await runtimeClient.startRun(startRunRequest);
      setThreadProjectIds((current) =>
        assignThreadToProject(current, run.threadId, runProjectId),
      );
      if (runProjectId === DEFAULT_WORKSPACE_PROJECT_ID) {
        setRemovedProjectIds((current) =>
          current.filter(
            (projectId) => projectId !== DEFAULT_WORKSPACE_PROJECT_ID,
          ),
        );
      }
      dispatch({ type: "run/created", run });
      dispatch({
        type: "run/threadAdded",
        threadId: run.threadId,
        label: nextThreadLabel,
        runId: run.runId,
        workspaceRoot: workspaceRoot,
      });
      void refreshThreads();
      localStorage.setItem("awaken:runId", run.runId);
      const unsubscribe = await runtimeClient.subscribeRun(
        run.runId,
        createSubscribeHandlers(),
      );
      unsubscribeRef.current = unsubscribe;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create run";
      restoreComposerValue(prompt);
      setComposerAttachments(draftAttachments);
      dispatch({ type: "run/createFailed", message });
    }
  }

  async function handleComposerSubmit(): Promise<void> {
    if (isBusy) {
      await handleCancel();
      requestComposerFocus();
      return;
    }
    await handleSubmit();
    requestComposerFocus();
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

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    syncProjectSelectionForWorkspace(thread?.workspaceRoot);
    if (!thread?.runId) {
      localStorage.removeItem("awaken:runId");
      return;
    }

    try {
      await replayThreadTranscript(threadId, loadSeq);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to load thread",
      );
    }
  }

  function threadsForProject(projectId: string): ThreadItem[] {
    return runtimeState.threads.filter(
      (thread) =>
        projectIdForThread(
          thread,
          threadProjectIds,
          defaultWorkspacePath,
        ) === projectId,
    );
  }

  function syncProjectSelectionForWorkspace(
    nextWorkspacePath?: string,
  ): void {
    if (!nextWorkspacePath) {
      return;
    }

    const nextProjectId = projectIdForWorkspacePath(
      nextWorkspacePath,
      defaultWorkspacePath,
    );
    setProjectsFromWorkspace((current) =>
      mergeWorkspaceProjects(
        current,
        [nextWorkspacePath],
        defaultWorkspacePath,
      ),
    );
    setSelectedProjectId(nextProjectId);
    workspaceTargetPathRef.current = nextWorkspacePath;
    setWorkspacePath(nextWorkspacePath);
    setExpandedProjectIds((current) =>
      current.includes(nextProjectId)
        ? current
        : [...current, nextProjectId],
    );
  }

  async function refreshAfterThreadMutation(
    removedThreadIds: string[] = [],
  ): Promise<void> {
    if (removedThreadIds.includes(runtimeState.activeThreadId ?? "")) {
      resetActiveThreadContext();
    }

    if (removedThreadIds.length > 0) {
      setThreadProjectIds((current) =>
        removeThreadProjectAssignments(current, removedThreadIds),
      );
    }

    await loadThreadsIntoState();
  }

  async function deleteThreadsForProject(
    projectId: string,
    options: { removeProject: boolean },
  ): Promise<void> {
    if (!desktopReady) {
      return;
    }

    const projectLabel = projectLabelById.get(projectId) ?? "项目";
    const threads = threadsForProject(projectId);
    const shouldRemoveProject = options.removeProject;
    const busyThread = threads.find(isThreadBusyForDelete);
    if (busyThread) {
      setNotice(`"${busyThread.label}" 正在运行，先停止后再删除。`);
      return;
    }

    const hasThreads = threads.length > 0;
    const shouldProceed = window.confirm(
      hasThreads
        ? `确认删除 "${projectLabel}" 下的 ${threads.length} 条历史会话？`
        : shouldRemoveProject
          ? `确认移除 "${projectLabel}"？`
          : `"${projectLabel}" 暂无历史会话。`,
    );
    if (!shouldProceed || (!hasThreads && !shouldRemoveProject)) {
      return;
    }

    const removedThreadIds = threads.map((thread) => thread.threadId);
    try {
      await Promise.all(
        removedThreadIds.map((threadId) =>
          runtimeClient.deleteThread(threadId),
        ),
      );
      await refreshAfterThreadMutation(removedThreadIds);
      if (shouldRemoveProject) {
        if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
          setRemovedProjectIds((current) =>
            current.includes(projectId) ? current : [...current, projectId],
          );
        } else {
          setProjectsFromWorkspace((current) =>
            removeWorkspaceProject(current, projectId),
          );
        }
        setExpandedProjectIds((current) =>
          current.filter((id) => id !== projectId),
        );
        if (selectedProjectId === projectId) {
          const fallbackProject = projectsFromWorkspace.find(
            (project) =>
              project.id !== projectId &&
              !isSameWorkspacePath(project.path, defaultWorkspacePath),
          );
          if (fallbackProject) {
            setSelectedProjectId(fallbackProject.id);
            workspaceTargetPathRef.current = fallbackProject.path;
            setWorkspacePath(fallbackProject.path);
          } else {
            setSelectedProjectId(undefined);
            workspaceTargetPathRef.current = "";
            setWorkspacePath("");
          }
        }
      }
      setNotice(
        shouldRemoveProject
          ? hasThreads
            ? `已移除 "${projectLabel}" 并删除历史会话`
            : `已移除 "${projectLabel}"`
          : `已删除 "${projectLabel}" 下的历史会话`,
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "删除历史会话失败");
      await refreshThreads();
    }
  }

  async function handleRenameThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    if (!thread) {
      setNotice("会话不存在");
      return;
    }

    const nextLabel = window.prompt("重命名会话", thread.label)?.trim();
    if (!nextLabel || nextLabel === thread.label) {
      return;
    }

    try {
      await runtimeClient.renameThread(threadId, nextLabel);
      await loadThreadsIntoState();
      setNotice("已重命名会话");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "重命名会话失败");
    }
  }

  async function handleDeleteThread(threadId: string): Promise<void> {
    if (!desktopReady) {
      return;
    }

    const thread = runtimeState.threads.find(
      (item) => item.threadId === threadId,
    );
    if (!thread) {
      setNotice("会话不存在");
      return;
    }
    if (isThreadBusyForDelete(thread)) {
      setNotice(`"${thread.label}" 正在运行，先停止后再删除。`);
      return;
    }
    if (!window.confirm(`确认删除会话 "${thread.label}"？`)) {
      return;
    }

    try {
      await runtimeClient.deleteThread(threadId);
      await refreshAfterThreadMutation([threadId]);
      setNotice("已删除会话");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "删除会话失败");
      await refreshThreads();
    }
  }

  function handleSelectProject(id: string): void {
    setActiveNavId(undefined);
    if (id.startsWith("thread:")) {
      void handleSelectConversation(id.slice("thread:".length));
      return;
    }

    const projectPath =
      id === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultWorkspacePath
        : projectsFromWorkspace.find((project) => project.id === id)?.path;
    if (
      shouldResetActiveThreadForWorkspaceChange(
        activeThreadWorkspacePath,
        projectPath,
      )
    ) {
      resetActiveThreadContext();
    }
    setSelectedProjectId(id);
    if (projectPath) {
      workspaceTargetPathRef.current = projectPath;
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
      const result = await runtimeClient.pickWorkspace(
        workspacePath || defaultWorkspacePath,
      );
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
      if (next.projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
        setRemovedProjectIds((current) =>
          current.filter(
            (projectId) => projectId !== DEFAULT_WORKSPACE_PROJECT_ID,
          ),
        );
      }
      if (
        shouldResetActiveThreadForWorkspaceChange(
          activeThreadWorkspacePath,
          result.path,
        )
      ) {
        resetActiveThreadContext();
      }
      workspaceTargetPathRef.current = result.path;
      setWorkspacePath(result.path);
      setSelectedProjectId(next.projectId);
      setExpandedProjectIds((current) =>
        current.includes(next.projectId)
          ? current
          : [...current, next.projectId],
      );
      setCreateProjectDialogOpen(false);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to select folder",
      );
    } finally {
      setPickingWorkspace(false);
    }
  }

  function handleNewProjectSession(projectId: string): void {
    handleSelectProject(projectId);
    setExpandedProjectIds((current) =>
      current.includes(projectId) ? current : [...current, projectId],
    );
    handleNewTask();
    requestComposerFocus();
  }

  async function handleCancel(): Promise<void> {
    if (!runtimeState.runId || !isBusy) {
      return;
    }

    dispatch({ type: "run/cancelRequested" });

    try {
      const run = await runtimeClient.cancelRun(runtimeState.runId);
      dispatch({ type: "run/cancelSync", run });
      const terminalStatuses: RuntimeRunStatus[] = [
        "finished",
        "error",
        "cancelled",
      ];
      if (terminalStatuses.includes(run.status)) {
        localStorage.removeItem("awaken:runId");
      }
    } catch (error) {
      dispatch({
        type: "run/streamFailed",
        message:
          error instanceof Error ? error.message : "Failed to cancel run",
      });
    }
  }

  async function handleResolveApproval(
    decision: ApprovalDecision,
  ): Promise<void> {
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
      await runtimeClient.resolveApproval(
        runtimeState.runId,
        activeApproval.id,
        decision,
      );
    } catch (error) {
      setResolvingApprovalId(null);
      resolvingApprovalIdRef.current = null;
      setResolvedApproval(null);
      setNotice(
        error instanceof Error ? error.message : "Failed to resolve approval",
      );
    }
  }

  function handleNewTask(): void {
    setActiveNavId(undefined);
    clearComposerValue();
    setComposerAttachments([]);
    resetActiveThreadContext();
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

  async function handlePasteImages(files: File[]): Promise<void> {
    if (files.length === 0 || composerBarState.disabled) {
      return;
    }
    setNotice(null);
    if (
      composerAttachments.length + files.length >
      MAX_COMPOSER_IMAGE_ATTACHMENTS
    ) {
      setNotice(`最多只能添加 ${MAX_COMPOSER_IMAGE_ATTACHMENTS} 张图片`);
      return;
    }
    const unsupportedFile = files.find(
      (file) =>
        !SUPPORTED_COMPOSER_IMAGE_MIME_TYPES.includes(
          file.type as (typeof SUPPORTED_COMPOSER_IMAGE_MIME_TYPES)[number],
        ),
    );
    if (unsupportedFile) {
      setNotice("仅支持粘贴 PNG、JPEG 或 WebP 图片");
      return;
    }
    const oversizedFile = files.find(
      (file) => file.size > MAX_COMPOSER_IMAGE_BYTES,
    );
    if (oversizedFile) {
      setNotice(
        `单张图片不能超过 ${MAX_COMPOSER_IMAGE_BYTES / (1024 * 1024)} MiB`,
      );
      return;
    }

    try {
      const startIndex = composerAttachments.length;
      const nextAttachments = await Promise.all(
        files.map((file, index) =>
          readComposerDraftImageAttachment(file, startIndex + index),
        ),
      );
      setComposerAttachments((current) => [...current, ...nextAttachments]);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "粘贴图片失败");
    }
  }

  function handleRemoveComposerAttachment(attachmentId: string): void {
    setComposerAttachments((current) =>
      current.filter((attachment) => attachment.id !== attachmentId),
    );
    setNotice(null);
  }

  function handleSidebarResizeStart(
    event: ReactPointerEvent<HTMLDivElement>,
  ): void {
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
      document.body.classList.remove(
        "is-layout-resizing",
        "is-sidebar-resizing",
      );
    }

    document.body.classList.add("is-layout-resizing", "is-sidebar-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleInspectorResizeStart(
    event: ReactPointerEvent<HTMLDivElement>,
  ): void {
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
      document.body.classList.remove(
        "is-layout-resizing",
        "is-inspector-resizing",
      );
    }

    document.body.classList.add("is-layout-resizing", "is-inspector-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp, { once: true });
    window.addEventListener("pointercancel", handlePointerUp, { once: true });
  }

  function handleSidebarResizeKeyDown(
    event: ReactKeyboardEvent<HTMLDivElement>,
  ): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
      return;
    }
    event.preventDefault();
    const delta = event.key === "ArrowRight" ? LAYOUT_STEP : -LAYOUT_STEP;
    setSidebarWidth((width) =>
      clampNumber(width + delta, MIN_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH),
    );
  }

  function handleInspectorResizeKeyDown(
    event: ReactKeyboardEvent<HTMLDivElement>,
  ): void {
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
      return <ThoughtTrace key={key} text={item.text} status={item.status} />;
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
    if (item.kind === "compactBoundary") {
      return (
        <div
          key={key}
          className={`compact-boundary compact-boundary-${item.status}`}
          role="separator"
          aria-label={
            item.detail ? `${item.title}: ${item.detail}` : item.title
          }
        >
          <span className="compact-boundary-line" aria-hidden="true" />
          <span className="compact-boundary-copy">
            <span className="compact-boundary-title">{item.title}</span>
            {item.detail ? (
              <span className="compact-boundary-detail">{item.detail}</span>
            ) : null}
          </span>
          <span className="compact-boundary-line" aria-hidden="true" />
        </div>
      );
    }
    if (item.kind === "exploring") {
      const actions = item.actions.map((action) => {
        const id = action.id.startsWith("explore:")
          ? action.id.slice("explore:".length)
          : rawToolId(action.id);
        const tool = runtimeState.toolResults.find(
          (record) => record.id === id,
        );
        return {
          ...action,
          detail: toolTraceDetail(tool, action.detail),
        };
      });
      return <ToolTraceGroup key={key} actions={actions} />;
    }
    if (item.kind === "tool") {
      const tool = runtimeState.toolResults.find(
        (record) => record.id === rawToolId(item.id),
      );
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
        (approval) =>
          approval.id === approvalId || approvalId.endsWith(`:${approval.id}`),
      );
      if (
        item.status === "waiting" &&
        itemApproval?.id === activeApproval?.id
      ) {
        return null;
      }
      const canResolve =
        item.status === "waiting" && activeApproval?.id === itemApproval?.id;
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
            resolvingApprovalId === itemApproval?.id
              ? "Resolving..."
              : "Approve"
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
        <FileChangeSummary key={key} files={item.files} status={item.status} />
      );
    }
    return null;
  }

  function renderTurnBodySegment(
    turnId: string,
    segment: NarrativeTurnBodySegment,
  ) {
    if (segment.kind === "items") {
      return segment.items.map((item, itemIndex) =>
        renderNarrativeItem(
          item,
          `${turnId}:${segment.id}:${item.id}:${itemIndex}`,
        ),
      );
    }

    return [
      <div
        key={`${turnId}:${segment.id}`}
        className={`execution-process-disclosure${
          segment.open ? " execution-process-disclosure-open" : ""
        }`}
      >
        <Disclosure
          title={
            <span className="execution-process-disclosure-title">
              {segment.title}
            </span>
          }
          open={segment.open}
          onOpenChange={(open) => {
            setExecutionProcessOpenByTurnId((current) => ({
              ...current,
              [turnId]: open,
            }));
          }}
        >
          <div className="execution-process-disclosure-body">
            {segment.items.map((item, itemIndex) =>
              renderNarrativeItem(
                item,
                `${turnId}:${segment.id}:${item.id}:${itemIndex}`,
              ),
            )}
          </div>
        </Disclosure>
      </div>,
    ];
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
  const composerBarAttachments = composerAttachments.map((attachment) => ({
    id: attachment.id,
    kind: "image" as const,
    src: attachment.previewURL,
    alt: attachment.displayName || "Pasted image",
    mimeType: attachment.mimeType,
  }));
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
            resolvingApprovalId === activeApproval.id
              ? "Resolving..."
              : "Approve"
          }
          rejectLabel={
            resolvingApprovalId === activeApproval.id
              ? "Resolving..."
              : "Reject"
          }
          resolving={resolvingApprovalId === activeApproval.id}
          resolvedDecision={resolvedDecisionForApproval(
            activeApproval,
            resolvedApproval,
          )}
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
    if (composerRuntimePanels.length === 0) {
      return null;
    }

    return (
      <div className="composer-runtime-panels">
        {composerRuntimePanels.map((panel) => (
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
              onToggleSidebar={() =>
                setSidebarCollapsed((collapsed) => !collapsed)
              }
              onNewTask={handleNewTask}
              right={
                <div className="workbench-chrome-status">
                  {topbarStatusLabel ? (
                    <span className="workbench-chrome-state">
                      {topbarStatusLabel}
                    </span>
                  ) : null}
                  <button
                    type="button"
                    className="workbench-chrome-icon workbench-chrome-panel-toggle"
                    aria-label={
                      inspectorCollapsed
                        ? "Expand inspector"
                        : "Collapse inspector"
                    }
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
                newTaskShortcut=""
                conversationSectionLabel=""
                projectSectionLabel="Projects"
                projectSectionActions={
                  <span className="sidebar-section-actions">
                    <SidebarSectionAction
                      label="Filter projects"
                      onClick={() =>
                        setNotice("Project filters are not available yet.")
                      }
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
              {activeExtensionSection ? (
                <>
                  {renderNoticeToast()}
                  <ExtensionsView
                    section={activeExtensionSection}
                    desktopReady={desktopReady}
                    runtimeClient={runtimeClient}
                    onNotice={setNotice}
                  />
                </>
              ) : (
                <WorkbenchShell
                  inspectorCollapsed={inspectorCollapsed}
                  stage={
                    <RunStage
                      scrollRef={
                        showCenterComposer
                          ? undefined
                          : setRunStageScrollElement
                      }
                      onScroll={
                        showCenterComposer ? undefined : handleRunStageScroll
                      }
                      overlay={
                        <>
                          {renderNoticeToast()}
                          {showTurnMarkerRail ? (
                            <nav
                              className={`turn-marker-rail turn-marker-rail-${turnMarkerDisplay.mode}`}
                              aria-label="Message shortcuts"
                              style={{
                                bottom: Math.max(28, composerSafeAreaPx),
                              }}
                            >
                              {turnMarkerDisplay.items.map((marker) => (
                                <button
                                  key={`${marker.id}:${marker.index}`}
                                  type="button"
                                  className={`turn-marker-button${
                                    marker.id === activeWorkbenchTurnId
                                      ? " turn-marker-button-active"
                                      : ""
                                  }`}
                                  aria-current={
                                    marker.id === activeWorkbenchTurnId
                                      ? "true"
                                      : undefined
                                  }
                                  aria-label={`跳转到第 ${marker.index + 1} 条消息`}
                                  title={`第 ${marker.index + 1} 条消息`}
                                  onClick={() =>
                                    handleTurnMarkerClick(marker.id)
                                  }
                                >
                                  <span
                                    className="turn-marker-bar"
                                    aria-hidden="true"
                                  />
                                </button>
                              ))}
                            </nav>
                          ) : null}
                          {returnToLatestVisible ? (
                            <div
                              className="return-to-latest-overlay"
                              style={{ bottom: returnToLatestBottomPx }}
                            >
                              <button
                                type="button"
                                className="return-to-latest-button"
                                aria-label="回到最新输出"
                                title="回到最新输出"
                                onClick={handleReturnToLatestClick}
                              >
                                <ChevronDown
                                  size={16}
                                  strokeWidth={2.1}
                                  aria-hidden="true"
                                />
                              </button>
                            </div>
                          ) : null}
                        </>
                      }
                      composer={
                        showCenterComposer ? undefined : (
                          <ComposerDock ref={setComposerDockElement}>
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
                                disabled={composerBarState.disabled}
                                isReady={composerBarState.isReady}
                                submitMode={composerBarState.submitMode}
                                submitDisabled={composerBarState.submitDisabled}
                                variant="dock"
                                metaItems={composerMetaItems}
                                focusRequest={composerFocusRequest}
                                attachments={composerBarAttachments}
                                attachmentWarning={composerAttachmentWarning}
                                onRemoveAttachment={
                                  handleRemoveComposerAttachment
                                }
                                onPasteImages={(files) => {
                                  void handlePasteImages(files);
                                }}
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
                            disabled={composerBarState.disabled}
                            isReady={composerBarState.isReady}
                            submitMode={composerBarState.submitMode}
                            submitDisabled={composerBarState.submitDisabled}
                            variant="center"
                            metaItems={composerMetaItems}
                            focusRequest={composerFocusRequest}
                            attachments={composerBarAttachments}
                            attachmentWarning={composerAttachmentWarning}
                            onRemoveAttachment={handleRemoveComposerAttachment}
                            onPasteImages={(files) => {
                              void handlePasteImages(files);
                            }}
                          />
                        </div>
                      ) : null}

                      {showCenterComposer ? null : (
                        <ExecutionFeed>
                          {narrativeSections.leadItems.map((item, index) =>
                            renderNarrativeItem(
                              item,
                              `lead:${item.id}:${index}`,
                            ),
                          )}
                          {narrativeSections.turns.map((turn, index) => {
                            const isLatestPrompt = turn.id === latestPromptId;
                            const isActiveTurn =
                              turn.id === activeWorkbenchTurnId;
                            const turnBodySegments =
                              projectNarrativeTurnBodySegments({
                                turnId: turn.id,
                                items: turn.items,
                                executionProcessOpen:
                                  executionProcessOpenByTurnId[turn.id] ===
                                  true,
                              });
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
                                  isLatestPrompt
                                    ? " execution-turn-section-latest"
                                    : ""
                                }${
                                  isActiveTurn
                                    ? " execution-turn-section-active"
                                    : ""
                                }`}
                                style={turnStyle}
                                aria-label={`Turn ${index + 1}`}
                              >
                                <div className="execution-turn-column">
                                  <div className="execution-turn-header">
                                    <UserPromptBlock
                                      text={turn.prompt.text}
                                      attachments={promptAttachmentViews(
                                        turn.prompt.attachments,
                                      )}
                                      expanded={
                                        promptBubbleModeByTurnId[turn.id] ===
                                        "expanded"
                                      }
                                      onExpandedChange={(expanded) => {
                                        setPromptBubbleModeByTurnId(
                                          (current) => ({
                                            ...current,
                                            [turn.id]: expanded
                                              ? "expanded"
                                              : "collapsed",
                                          }),
                                        );
                                      }}
                                    />
                                  </div>
                                  <div className="execution-turn-body">
                                    {turnBodySegments.flatMap((segment) =>
                                      renderTurnBodySegment(turn.id, segment),
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
              )}
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
