import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import {
  AppWindow,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  FileText,
  Folder,
  FolderOpen,
  Globe2,
  Maximize2,
  Plus,
  Search,
} from "lucide-react";
import { AwakenPopover as Popover, AwakenTooltip as Tooltip } from "@awaken/design-system";
import type {
  ProjectDirectoryEntry,
  ProjectEntryActionRequest,
  ProjectEntryOpenApplication,
} from "../shared/runtime";
import cursorLogo from "./assets/open-with/cursor.png";
import microsoftOfficeLogo from "./assets/open-with/microsoft-office.png";
import traeLogo from "./assets/open-with/trae.png";
import vscodeLogo from "./assets/open-with/vscode.png";
import wpsOfficeLogo from "./assets/open-with/wps-office.png";
import xcodeLogo from "./assets/open-with/xcode.png";
import type { RuntimeClient } from "./runtime/client";
import ArtifactPreviewHost, {
  type ArtifactPreviewSelection,
} from "./file-preview/ArtifactPreviewHost";
import FilePreviewHost, { type ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";
import { projectFileTabId, type ProjectFileWorkspaceTab } from "./projectFileTabs";
import { projectFileIconKind, projectFileTreeRows, type ProjectFileTreeRow } from "./projectFiles";
import ProjectFileIcon, { type ProjectFileVisualIconKind } from "./ProjectFileIcon";
import "./styles/project-files.css";

export type ProjectFilesFolder = {
  path: string;
  label: string;
  isPrimary: boolean;
};

export type ProjectFilesWorkspaceProps = {
  runtimeClient: Pick<
    RuntimeClient,
    | "listProjectDirectory"
    | "watchProjectDirectory"
    | "unwatchProjectDirectory"
    | "onProjectDirectoryChanged"
    | "copyProjectEntryPath"
    | "openProjectEntryWith"
    | "startArtifactPreview"
    | "startProjectFilePreview"
    | "closeProjectFilePreview"
    | "openProjectFileExternally"
    | "revealProjectFile"
    | "saveProjectMarkdownFile"
    | "openArtifact"
    | "revealArtifact"
  >;
  projectId?: string;
  projectLabel?: string;
  folders: ProjectFilesFolder[];
  initialMode?: ProjectFileWorkspaceMode;
  activeTab?: ProjectFileWorkspaceTab;
  artifactPreview?: ArtifactPreviewSelection;
  expanded?: boolean;
  onWorkspaceModeChange?: (mode: ProjectFileWorkspaceMode) => void;
  onOpenTab?: (tab: ProjectFileWorkspaceTab) => void;
  onExpand?: () => void;
  onPreviewPrimaryClick?: () => void;
  addTaskDisabled?: boolean;
  onAddTaskContext?: (
    request: ProjectEntryActionRequest,
  ) => Promise<{ ok: true } | { ok: false; message: string }>;
};

const MIN_FILE_LIST_WIDTH = 240;
const MAX_FILE_LIST_WIDTH = 400;
const MIN_FILE_PREVIEW_WIDTH = 240;
const DEFAULT_FILE_LIST_WIDTH = MIN_FILE_LIST_WIDTH;
const DEFAULT_FILE_WORKSPACE_WIDTH = 720;
const FILE_LIST_RESIZE_STEP = 12;
const MINIMUM_WIDTH_FLING_SAMPLE_WINDOW = 140;
const MINIMUM_WIDTH_FLING_DISTANCE = 120;
const MINIMUM_WIDTH_FLING_VELOCITY = 1.2;
const PREVIEW_PRIMARY_CLICK_DISTANCE = 6;
const CONTEXT_MENU_MARGIN = 8;
const CONTEXT_SUBMENU_GAP = 6;
const CONTEXT_SUBMENU_DEFAULT_TOP = -7;

type PreviewPrimaryPointerGesture = {
  pointerId: number;
  startX: number;
  startY: number;
  moved: boolean;
};

type ProjectEntryContextMenuState = {
  request: ProjectEntryActionRequest;
  name: string;
  x: number;
  y: number;
  openWithMenuOpen: boolean;
  submenuSide: "left" | "right";
  submenuTop: number;
};

type FloatingRect = {
  left: number;
  right: number;
  top: number;
  bottom: number;
};

type FloatingSize = {
  width: number;
  height: number;
};

type FloatingViewport = {
  width: number;
  height: number;
};

type ProjectFilesContextStatus = {
  message: string;
  tone: "info" | "error";
};

const PROJECT_ENTRY_OPEN_APPLICATIONS: ReadonlyArray<{
  id: ProjectEntryOpenApplication;
  label: string;
  icon: string;
}> = [
  { id: "wps", label: "WPS Office", icon: wpsOfficeLogo },
  { id: "office", label: "Microsoft Office", icon: microsoftOfficeLogo },
  { id: "vscode", label: "VS Code", icon: vscodeLogo },
  { id: "cursor", label: "Cursor", icon: cursorLogo },
  { id: "trae", label: "TRAE", icon: traeLogo },
  { id: "xcode", label: "Xcode", icon: xcodeLogo },
];

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

export function projectFilesMenuViewportOffset(
  rect: FloatingRect,
  viewport: FloatingViewport,
  margin = CONTEXT_MENU_MARGIN,
): { x: number; y: number } {
  const availableRight = viewport.width - margin;
  const availableBottom = viewport.height - margin;
  const rectWidth = rect.right - rect.left;
  const rectHeight = rect.bottom - rect.top;
  const availableWidth = Math.max(0, viewport.width - margin * 2);
  const availableHeight = Math.max(0, viewport.height - margin * 2);
  return {
    x:
      rectWidth > availableWidth || rect.left < margin
        ? margin - rect.left
        : rect.right > availableRight
          ? availableRight - rect.right
          : 0,
    y:
      rectHeight > availableHeight || rect.top < margin
        ? margin - rect.top
        : rect.bottom > availableBottom
          ? availableBottom - rect.bottom
          : 0,
  };
}

export function projectFilesSubmenuPlacement(
  anchor: Pick<FloatingRect, "left" | "right" | "top">,
  submenu: FloatingSize,
  viewport: FloatingViewport,
  margin = CONTEXT_MENU_MARGIN,
  gap = CONTEXT_SUBMENU_GAP,
): { side: "left" | "right"; top: number } {
  const spaceRight = viewport.width - margin - anchor.right;
  const spaceLeft = anchor.left - margin;
  const side =
    spaceRight >= submenu.width + gap || spaceRight >= spaceLeft ? "right" : "left";
  const preferredTop = anchor.top + CONTEXT_SUBMENU_DEFAULT_TOP;
  const maximumTop = Math.max(margin, viewport.height - submenu.height - margin);
  const viewportTop = clamp(preferredTop, margin, maximumTop);
  return {
    side,
    top: viewportTop - anchor.top,
  };
}

type PointerSample = {
  x: number;
  time: number;
};

function isMinimumWidthRightwardFling(samples: PointerSample[]): boolean {
  const finish = samples[samples.length - 1];
  if (!finish || samples.length < 2) return false;
  const start = samples.reduce((leftmost, sample) => (sample.x < leftmost.x ? sample : leftmost));
  const distance = finish.x - start.x;
  const duration = Math.max(1, finish.time - start.time);
  return (
    distance >= MINIMUM_WIDTH_FLING_DISTANCE && distance / duration >= MINIMUM_WIDTH_FLING_VELOCITY
  );
}

function preferredFolderPath(folders: ProjectFilesFolder[]): string {
  return folders.find((folder) => folder.isPrimary)?.path ?? folders[0]?.path ?? "";
}

function rowIconKind(entry: ProjectFileTreeRow): ProjectFileVisualIconKind {
  if (entry.kind === "directory" && entry.expanded) return "folder-open";
  return projectFileIconKind(entry);
}

export function ProjectFilesWorkspace({
  runtimeClient,
  projectId,
  projectLabel,
  folders,
  initialMode = "files",
  activeTab,
  artifactPreview,
  expanded = false,
  onWorkspaceModeChange,
  onOpenTab,
  onExpand,
  onPreviewPrimaryClick,
  addTaskDisabled = false,
  onAddTaskContext,
}: ProjectFilesWorkspaceProps) {
  const workspaceBodyRef = useRef<HTMLDivElement>(null);
  const fileNavigatorRef = useRef<HTMLElement>(null);
  const [fileListExpanded, setFileListExpanded] = useState(true);
  const [fileListWidth, setFileListWidth] = useState<number>();
  const [folderMenuOpen, setFolderMenuOpen] = useState(false);
  const [selectedFolderPath, setSelectedFolderPath] = useState(() => preferredFolderPath(folders));
  const [filter, setFilter] = useState("");
  const [listings, setListings] = useState<Record<string, ProjectDirectoryEntry[]>>({});
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set());
  const [workspaceMode, setWorkspaceMode] = useState<ProjectFileWorkspaceMode>(initialMode);
  const [errorMessage, setErrorMessage] = useState<string>();
  const [contextStatus, setContextStatus] = useState<ProjectFilesContextStatus>();
  const [contextMenu, setContextMenu] = useState<ProjectEntryContextMenuState>();
  const contextMenuRef = useRef<HTMLDivElement>(null);
  const contextOpenWithRef = useRef<HTMLDivElement>(null);
  const contextSubmenuRef = useRef<HTMLDivElement>(null);
  const requestGenerationRef = useRef(0);
  const directoryRefreshSequenceRef = useRef(0);
  const listingsRef = useRef(listings);
  const onWorkspaceModeChangeRef = useRef(onWorkspaceModeChange);
  const previewPrimaryPointerGestureRef = useRef<PreviewPrimaryPointerGesture | undefined>(
    undefined,
  );

  onWorkspaceModeChangeRef.current = onWorkspaceModeChange;

  const selectedFolder = folders.find((folder) => folder.path === selectedFolderPath) ?? folders[0];

  const changeWorkspaceMode = useCallback(
    (mode: ProjectFileWorkspaceMode): void => {
      setWorkspaceMode(mode);
      onWorkspaceModeChangeRef.current?.(mode);
    },
    [],
  );

  useEffect(() => {
    setWorkspaceMode(initialMode);
  }, [initialMode]);

  useEffect(() => {
    listingsRef.current = listings;
  }, [listings]);

  useEffect(() => {
    if (folders.some((folder) => folder.path === selectedFolderPath)) {
      return;
    }
    setSelectedFolderPath(preferredFolderPath(folders));
  }, [folders, selectedFolderPath]);

  useEffect(() => {
    setContextMenu(undefined);
  }, [projectId, selectedFolderPath]);

  useEffect(() => {
    if (!contextStatus) return;
    const timeout = window.setTimeout(() => setContextStatus(undefined), 2_200);
    return () => window.clearTimeout(timeout);
  }, [contextStatus]);

  useEffect(() => {
    if (!contextMenu) return;
    const close = (): void => setContextMenu(undefined);
    const handlePointerDown = (event: PointerEvent): void => {
      if (!contextMenuRef.current?.contains(event.target as Node)) close();
    };
    const handleKeyDown = (event: KeyboardEvent): void => {
      if (event.key === "Escape") close();
    };
    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    window.addEventListener("blur", close);
    window.addEventListener("resize", close);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("blur", close);
      window.removeEventListener("resize", close);
    };
  }, [contextMenu]);

  useLayoutEffect(() => {
    const menu = contextMenuRef.current;
    if (!contextMenu || !menu) return;
    const rect = menu.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return;
    const offset = projectFilesMenuViewportOffset(rect, {
      width: window.innerWidth,
      height: window.innerHeight,
    });
    if (offset.x === 0 && offset.y === 0) return;
    setContextMenu((current) =>
      current
        ? {
            ...current,
            x: current.x + offset.x,
            y: current.y + offset.y,
          }
        : current,
    );
  }, [contextMenu]);

  useLayoutEffect(() => {
    const anchor = contextOpenWithRef.current;
    const submenu = contextSubmenuRef.current;
    if (!contextMenu?.openWithMenuOpen || !anchor || !submenu) return;
    const anchorRect = anchor.getBoundingClientRect();
    const submenuRect = submenu.getBoundingClientRect();
    if (submenuRect.width === 0 || submenuRect.height === 0) return;
    const placement = projectFilesSubmenuPlacement(
      anchorRect,
      { width: submenuRect.width, height: submenuRect.height },
      { width: window.innerWidth, height: window.innerHeight },
    );
    if (
      placement.side === contextMenu.submenuSide &&
      placement.top === contextMenu.submenuTop
    ) {
      return;
    }
    setContextMenu((current) =>
      current
        ? {
            ...current,
            submenuSide: placement.side,
            submenuTop: placement.top,
          }
        : current,
    );
  }, [contextMenu]);

  useEffect(() => {
    const generation = requestGenerationRef.current + 1;
    requestGenerationRef.current = generation;
    directoryRefreshSequenceRef.current += 1;
    setListings({});
    setExpandedPaths(new Set());
    setLoadingPaths(new Set());
    setErrorMessage(undefined);

    if (!projectId || !selectedFolder?.path) {
      return;
    }

    setLoadingPaths(new Set([""]));
    void runtimeClient
      .listProjectDirectory({
        projectId,
        folderPath: selectedFolder.path,
      })
      .then((result) => {
        if (requestGenerationRef.current !== generation) return;
        if (!result.ok) {
          setErrorMessage(result.message);
          return;
        }
        setListings({ "": result.entries });
      })
      .catch(() => {
        if (requestGenerationRef.current === generation) {
          setErrorMessage("文件目录暂时无法读取。");
        }
      })
      .finally(() => {
        if (requestGenerationRef.current === generation) {
          setLoadingPaths(new Set());
        }
      });
  }, [projectId, runtimeClient, selectedFolder?.path]);

  const refreshLoadedDirectories = useCallback(async (): Promise<void> => {
    if (!projectId || !selectedFolder?.path) return;
    const generation = requestGenerationRef.current;
    const refreshSequence = directoryRefreshSequenceRef.current + 1;
    directoryRefreshSequenceRef.current = refreshSequence;
    const loadedPaths = Object.keys(listingsRef.current);
    const paths = loadedPaths.includes("") ? loadedPaths : ["", ...loadedPaths];
    const refreshed = await Promise.all(
      paths.map(async (relativePath) => {
        try {
          const result = await runtimeClient.listProjectDirectory({
            projectId,
            folderPath: selectedFolder.path,
            ...(relativePath ? { relativePath } : {}),
          });
          return { relativePath, result };
        } catch {
          return { relativePath, result: undefined };
        }
      }),
    );
    if (
      requestGenerationRef.current !== generation ||
      directoryRefreshSequenceRef.current !== refreshSequence
    ) {
      return;
    }

    const root = refreshed.find((entry) => entry.relativePath === "");
    const rootResult = root?.result;
    if (!rootResult || !rootResult.ok) {
      setListings({});
      setExpandedPaths(new Set());
      setErrorMessage(rootResult?.message ?? "文件目录暂时无法读取。");
      return;
    }

    const nextListings: Record<string, ProjectDirectoryEntry[]> = {};
    const availableDirectoryPaths = new Set<string>();
    const successfullyLoadedPaths = new Set<string>();
    for (const entry of refreshed) {
      if (!entry.result?.ok) continue;
      successfullyLoadedPaths.add(entry.relativePath);
      nextListings[entry.relativePath] = entry.result.entries;
      for (const child of entry.result.entries) {
        if (child.kind === "directory") availableDirectoryPaths.add(child.relativePath);
      }
    }
    for (const relativePath of Object.keys(nextListings)) {
      if (
        relativePath &&
        (!availableDirectoryPaths.has(relativePath) || !successfullyLoadedPaths.has(relativePath))
      ) {
        delete nextListings[relativePath];
      }
    }
    setListings(nextListings);
    setExpandedPaths((current) =>
      new Set(
        [...current].filter(
          (relativePath) =>
            availableDirectoryPaths.has(relativePath) &&
            successfullyLoadedPaths.has(relativePath),
        ),
      ),
    );
    setErrorMessage(undefined);
  }, [projectId, runtimeClient, selectedFolder?.path]);

  useEffect(() => {
    if (!projectId || !selectedFolder?.path) return;
    const generation = requestGenerationRef.current;
    let disposed = false;
    let subscriptionId: string | undefined;
    const removeChangeListener = runtimeClient.onProjectDirectoryChanged((event) => {
      if (event.subscriptionId !== subscriptionId) return;
      void refreshLoadedDirectories();
    });

    void runtimeClient
      .watchProjectDirectory({
        projectId,
        folderPath: selectedFolder.path,
      })
      .then((result) => {
        if (disposed || requestGenerationRef.current !== generation) {
          if (result.ok) void runtimeClient.unwatchProjectDirectory(result.subscriptionId);
          return;
        }
        if (!result.ok) {
          setErrorMessage(result.message);
          return;
        }
        subscriptionId = result.subscriptionId;
        // Refresh once after the watcher is active to close the small setup gap
        // between the initial directory snapshot and subscription readiness.
        void refreshLoadedDirectories();
      })
      .catch(() => {
        if (!disposed && requestGenerationRef.current === generation) {
          setErrorMessage("文件变化暂时无法自动同步。");
        }
      });

    return () => {
      disposed = true;
      removeChangeListener();
      if (subscriptionId) void runtimeClient.unwatchProjectDirectory(subscriptionId);
    };
  }, [projectId, refreshLoadedDirectories, runtimeClient, selectedFolder?.path]);

  async function loadDirectory(relativePath: string): Promise<void> {
    if (!projectId || !selectedFolder?.path || loadingPaths.has(relativePath)) {
      return;
    }
    const generation = requestGenerationRef.current;
    setLoadingPaths((current) => new Set(current).add(relativePath));
    setErrorMessage(undefined);
    try {
      const result = await runtimeClient.listProjectDirectory({
        projectId,
        folderPath: selectedFolder.path,
        relativePath,
      });
      if (requestGenerationRef.current !== generation) return;
      if (!result.ok) {
        setErrorMessage(result.message);
        return;
      }
      setListings((current) => ({
        ...current,
        [relativePath]: result.entries,
      }));
    } catch {
      if (requestGenerationRef.current === generation) {
        setErrorMessage("文件目录暂时无法读取。");
      }
    } finally {
      if (requestGenerationRef.current === generation) {
        setLoadingPaths((current) => {
          const next = new Set(current);
          next.delete(relativePath);
          return next;
        });
      }
    }
  }

  function toggleDirectory(relativePath: string): void {
    if (expandedPaths.has(relativePath)) {
      setExpandedPaths((current) => {
        const next = new Set(current);
        next.delete(relativePath);
        return next;
      });
      return;
    }
    setExpandedPaths((current) => new Set(current).add(relativePath));
    if (!listings[relativePath]) {
      void loadDirectory(relativePath);
    }
  }

  function fileListWidthBounds(): { minimum: number; maximum: number } {
    const bodyWidth = workspaceBodyRef.current?.getBoundingClientRect().width;
    return {
      minimum: MIN_FILE_LIST_WIDTH,
      maximum: Math.max(
        MIN_FILE_LIST_WIDTH,
        Math.min(
          MAX_FILE_LIST_WIDTH,
          (bodyWidth ?? DEFAULT_FILE_WORKSPACE_WIDTH) - MIN_FILE_PREVIEW_WIDTH,
        ),
      ),
    };
  }

  function currentFileListWidth(): number {
    if (fileListWidth !== undefined) return fileListWidth;
    const measuredWidth = fileNavigatorRef.current?.getBoundingClientRect().width ?? 0;
    return measuredWidth > 0 ? measuredWidth : DEFAULT_FILE_LIST_WIDTH;
  }

  function handleFileListResizeStart(event: ReactPointerEvent<HTMLDivElement>): void {
    if (!fileListExpanded || !workspaceBodyRef.current) return;
    event.preventDefault();
    const target = event.currentTarget;
    const pointerId = event.pointerId;
    target.setPointerCapture?.(pointerId);
    const bodyRight = workspaceBodyRef.current.getBoundingClientRect().right;
    const { minimum, maximum } = fileListWidthBounds();
    let minimumWidthSamples: PointerSample[] =
      currentFileListWidth() <= minimum ? [{ x: event.clientX, time: event.timeStamp }] : [];

    function recordMinimumWidthSample(clientX: number, timeStamp: number): boolean {
      minimumWidthSamples.push({ x: clientX, time: timeStamp });
      const cutoff = timeStamp - MINIMUM_WIDTH_FLING_SAMPLE_WINDOW;
      minimumWidthSamples = minimumWidthSamples.filter((sample) => sample.time >= cutoff);
      return isMinimumWidthRightwardFling(minimumWidthSamples);
    }

    function handlePointerMove(moveEvent: PointerEvent): void {
      if (moveEvent.pointerId !== pointerId) return;
      moveEvent.preventDefault();
      const nextWidth = clamp(bodyRight - moveEvent.clientX, minimum, maximum);
      setFileListWidth(nextWidth);
      if (nextWidth > minimum) {
        minimumWidthSamples = [];
        return;
      }
      if (minimumWidthSamples.length === 0) {
        minimumWidthSamples = [{ x: moveEvent.clientX, time: moveEvent.timeStamp }];
        return;
      }
      if (recordMinimumWidthSample(moveEvent.clientX, moveEvent.timeStamp)) {
        finishPointerGesture(moveEvent);
        setFolderMenuOpen(false);
        setFileListExpanded(false);
      }
    }

    function finishPointerGesture(finishEvent: PointerEvent): void {
      if (finishEvent.pointerId !== pointerId) return;
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerCancel);
      try {
        target.releasePointerCapture(pointerId);
      } catch {
        // Pointer capture can already be released when dragging ends outside.
      }
      document.body.classList.remove("is-project-file-list-resizing");
    }

    function handlePointerUp(upEvent: PointerEvent): void {
      finishPointerGesture(upEvent);
    }

    function handlePointerCancel(cancelEvent: PointerEvent): void {
      finishPointerGesture(cancelEvent);
    }

    document.body.classList.add("is-project-file-list-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
    window.addEventListener("pointercancel", handlePointerCancel);
  }

  function handleFileListResizeKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const { minimum, maximum } = fileListWidthBounds();
    const delta = event.key === "ArrowLeft" ? FILE_LIST_RESIZE_STEP : -FILE_LIST_RESIZE_STEP;
    setFileListWidth(clamp(currentFileListWidth() + delta, minimum, maximum));
  }

  const rows = useMemo(
    () =>
      projectFileTreeRows({
        listings,
        expandedPaths,
        selectedPath:
          activeTab && activeTab.folderPath === selectedFolder?.path
            ? activeTab.relativePath
            : undefined,
        filter,
      }),
    [activeTab, expandedPaths, filter, listings, selectedFolder?.path],
  );

  const emptyTitle = !projectId ? "选择项目" : folders.length === 0 ? "项目没有文件夹" : "打开文件";
  const emptyDescription = !projectId
    ? "先从左侧选择一个项目"
    : folders.length === 0
      ? "为当前项目添加文件夹后即可浏览"
      : "从工作区目录树中选择文件";

  function isInteractivePreviewTarget(target: EventTarget | null): boolean {
    return (
      target instanceof Element &&
      Boolean(
        target.closest(
          "button, a, input, textarea, select, [role='button'], [contenteditable='true']",
        ),
      )
    );
  }

  function handlePreviewPointerDown(event: ReactPointerEvent<HTMLDivElement>): void {
    if (event.button !== 0 || !onPreviewPrimaryClick || isInteractivePreviewTarget(event.target)) {
      return;
    }
    previewPrimaryPointerGestureRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      moved: false,
    };
  }

  function handlePreviewPointerMove(event: ReactPointerEvent<HTMLDivElement>): void {
    const gesture = previewPrimaryPointerGestureRef.current;
    if (!gesture || gesture.pointerId !== event.pointerId || gesture.moved) {
      return;
    }
    const distanceX = event.clientX - gesture.startX;
    const distanceY = event.clientY - gesture.startY;
    if (
      distanceX * distanceX + distanceY * distanceY >
      PREVIEW_PRIMARY_CLICK_DISTANCE * PREVIEW_PRIMARY_CLICK_DISTANCE
    ) {
      gesture.moved = true;
    }
  }

  function handlePreviewPointerUp(event: ReactPointerEvent<HTMLDivElement>): void {
    const gesture = previewPrimaryPointerGestureRef.current;
    if (!gesture || gesture.pointerId !== event.pointerId) return;
    previewPrimaryPointerGestureRef.current = undefined;
    if (!gesture.moved && !isInteractivePreviewTarget(event.target)) {
      onPreviewPrimaryClick?.();
    }
  }

  function handlePreviewPointerCancel(event: ReactPointerEvent<HTMLDivElement>): void {
    if (previewPrimaryPointerGestureRef.current?.pointerId === event.pointerId) {
      previewPrimaryPointerGestureRef.current = undefined;
    }
  }

  function showProjectEntryContextMenu(
    event: ReactMouseEvent<HTMLButtonElement>,
    row: ProjectFileTreeRow,
  ): void {
    if (!projectId || !selectedFolder?.path || row.kind === "symlink") return;
    event.preventDefault();
    event.stopPropagation();
    const menuWidth = 224;
    const menuHeight = 152;
    setContextMenu({
      request: {
        projectId,
        folderPath: selectedFolder.path,
        relativePath: row.relativePath,
        kind: row.kind,
      },
      name: row.name,
      x: clamp(
        event.clientX,
        CONTEXT_MENU_MARGIN,
        Math.max(CONTEXT_MENU_MARGIN, window.innerWidth - menuWidth - CONTEXT_MENU_MARGIN),
      ),
      y: clamp(
        event.clientY,
        CONTEXT_MENU_MARGIN,
        Math.max(CONTEXT_MENU_MARGIN, window.innerHeight - menuHeight - CONTEXT_MENU_MARGIN),
      ),
      openWithMenuOpen: false,
      submenuSide: "right",
      submenuTop: CONTEXT_SUBMENU_DEFAULT_TOP,
    });
  }

  async function addContextMenuEntryToTask(): Promise<void> {
    const current = contextMenu;
    setContextMenu(undefined);
    if (!current || !onAddTaskContext || addTaskDisabled) return;
    try {
      const result = await onAddTaskContext(current.request);
      setContextStatus({
        message: result.ok ? `已添加到任务：${current.name}` : result.message,
        tone: result.ok ? "info" : "error",
      });
    } catch {
      setContextStatus({ message: "暂时无法把这个条目添加到任务。", tone: "error" });
    }
  }

  async function copyContextMenuEntryPath(): Promise<void> {
    const current = contextMenu;
    setContextMenu(undefined);
    if (!current) return;
    try {
      const result = await runtimeClient.copyProjectEntryPath(current.request);
      setContextStatus({
        message: result.ok ? "已复制路径" : result.message,
        tone: result.ok ? "info" : "error",
      });
    } catch {
      setContextStatus({ message: "暂时无法复制这个路径。", tone: "error" });
    }
  }

  async function openContextMenuEntryWith(
    application: ProjectEntryOpenApplication,
    label: string,
  ): Promise<void> {
    const current = contextMenu;
    setContextMenu(undefined);
    if (!current) return;
    try {
      const result = await runtimeClient.openProjectEntryWith({
        ...current.request,
        application,
      });
      setContextStatus({
        message: result.ok ? `已用 ${label} 打开` : result.message,
        tone: result.ok ? "info" : "error",
      });
    } catch {
      setContextStatus({
        message: `暂时无法用 ${label} 打开这个条目。`,
        tone: "error",
      });
    }
  }

  return (
    <section className="project-files-workspace" aria-label="项目文件">
      <header className="project-files-pathbar">
        <div className="project-files-mode-switch" role="tablist" aria-label="文件工作区">
          <button
            type="button"
            role="tab"
            aria-selected={workspaceMode === "files"}
            onClick={() => changeWorkspaceMode("files")}
          >
            <FileText size={15} strokeWidth={1.8} aria-hidden="true" />
            文件
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={workspaceMode === "browser"}
            onClick={() => changeWorkspaceMode("browser")}
          >
            <Globe2 size={15} strokeWidth={1.8} aria-hidden="true" />
            浏览器
          </button>
        </div>
        <div className="project-files-pathbar__actions">
          {!expanded && onExpand ? (
            <Tooltip label="展开面板" placement="bottom">
              <button
                type="button"
                className="project-files-pathbar__icon"
                aria-label="展开面板"
                onClick={onExpand}
              >
                <Maximize2 size={17} strokeWidth={1.8} />
              </button>
            </Tooltip>
          ) : null}
          <button
            type="button"
            className={`project-files-pathbar__icon${fileListExpanded ? " is-expanded" : ""}`}
            aria-label={fileListExpanded ? "收起文件列表" : "展开文件列表"}
            aria-expanded={fileListExpanded}
            aria-controls="awaken-project-files-navigator"
            onClick={() => {
              setFolderMenuOpen(false);
              setFileListExpanded((current) => !current);
            }}
          >
            <FolderOpen size={18} strokeWidth={1.8} />
          </button>
        </div>
      </header>

      <div
        ref={workspaceBodyRef}
        className={`project-files-workspace__body${
          fileListExpanded ? "" : " is-file-list-collapsed"
        }`}
        style={
          fileListWidth === undefined
            ? undefined
            : ({
                "--project-file-list-width": `${fileListWidth}px`,
              } as CSSProperties)
        }
      >
        <div
          className="project-files-preview"
          onPointerDown={handlePreviewPointerDown}
          onPointerMove={handlePreviewPointerMove}
          onPointerUp={handlePreviewPointerUp}
          onPointerCancel={handlePreviewPointerCancel}
        >
          {artifactPreview && workspaceMode === "files" ? (
            <ArtifactPreviewHost
              runtimeClient={runtimeClient}
              selection={artifactPreview}
            />
          ) : activeTab || workspaceMode === "browser" ? (
            <FilePreviewHost
              runtimeClient={runtimeClient}
              projectId={projectId}
              folderPath={activeTab?.folderPath}
              selection={activeTab}
              mode={workspaceMode}
              onModeChange={changeWorkspaceMode}
              onPreviewPrimaryClick={onPreviewPrimaryClick}
            />
          ) : (
            <div className="project-files-preview-empty is-centered">
              <FolderOpen size={42} strokeWidth={1.6} aria-hidden="true" />
              <strong>{emptyTitle}</strong>
              <span>{emptyDescription}</span>
            </div>
          )}
          {contextStatus ? (
            <div
              className={`project-files-context-status${
                contextStatus.tone === "error" ? " is-error" : ""
              }`}
              role={contextStatus.tone === "error" ? "alert" : "status"}
              aria-live={contextStatus.tone === "error" ? "assertive" : "polite"}
            >
              {contextStatus.message}
            </div>
          ) : null}
        </div>

        <div
          className="project-files-divider"
          role="separator"
          aria-label="调整文件列表宽度"
          aria-orientation="vertical"
          aria-valuemin={MIN_FILE_LIST_WIDTH}
          aria-valuemax={Math.round(fileListWidthBounds().maximum)}
          aria-valuenow={Math.round(fileListWidth ?? DEFAULT_FILE_LIST_WIDTH)}
          aria-hidden={!fileListExpanded}
          tabIndex={fileListExpanded ? 0 : -1}
          onPointerDown={handleFileListResizeStart}
          onKeyDown={handleFileListResizeKeyDown}
        />

        <aside
          ref={fileNavigatorRef}
          id="awaken-project-files-navigator"
          className="project-files-navigator"
          aria-label="项目文件目录"
          aria-hidden={!fileListExpanded}
        >
          <div className="project-files-folder-picker">
            <Popover
              open={folderMenuOpen}
              onOpenChange={setFolderMenuOpen}
              placement="bottom-start"
              matchTriggerWidth
              trigger={
                <button
                  type="button"
                  className="project-files-folder-trigger"
                  disabled={folders.length === 0}
                  aria-label="选择项目文件夹组"
                >
                  <Folder size={17} strokeWidth={1.8} aria-hidden="true" />
                  <span>{selectedFolder?.label ?? projectLabel ?? "选择文件夹"}</span>
                  {folders.length > 1 ? (
                    <ChevronDown size={15} strokeWidth={1.8} aria-hidden="true" />
                  ) : null}
                </button>
              }
              content={
                <div className="project-files-folder-menu" role="listbox">
                  {folders.map((folder) => {
                    const selected = folder.path === selectedFolder?.path;
                    return (
                      <button
                        key={folder.path}
                        type="button"
                        className={`project-files-folder-option${selected ? " is-selected" : ""}`}
                        role="option"
                        aria-selected={selected}
                        onClick={() => {
                          setSelectedFolderPath(folder.path);
                          setFolderMenuOpen(false);
                        }}
                      >
                        <Folder size={16} strokeWidth={1.75} aria-hidden="true" />
                        <span>
                          <strong>{folder.label}</strong>
                        </span>
                        {selected ? <Check size={16} strokeWidth={1.9} aria-hidden="true" /> : null}
                      </button>
                    );
                  })}
                </div>
              }
            />
          </div>

          <label className="project-files-filter">
            <Search size={16} strokeWidth={1.75} aria-hidden="true" />
            <input
              type="search"
              value={filter}
              disabled={!selectedFolder}
              placeholder="筛选文件…"
              aria-label="筛选文件"
              onChange={(event) => setFilter(event.currentTarget.value)}
            />
          </label>

          <div className="project-files-tree-wrap">
            {loadingPaths.has("") ? (
              <div className="project-files-tree-state">正在读取文件…</div>
            ) : errorMessage && !listings[""] ? (
              <div className="project-files-tree-state is-error">{errorMessage}</div>
            ) : selectedFolder && rows.length === 0 ? (
              <div className="project-files-tree-state">
                {filter ? "没有匹配的文件" : "此文件夹为空"}
              </div>
            ) : (
              <div className="project-files-tree" role="tree">
                {rows.map((row) => {
                  const directory = row.kind === "directory";
                  const loading = loadingPaths.has(row.relativePath);
                  const iconKind = rowIconKind(row);
                  return (
                    <button
                      key={row.relativePath}
                      type="button"
                      className={`project-files-tree-row${
                        row.selected ? " is-selected" : ""
                      }${row.depth > 0 ? " is-nested" : ""}${loading ? " is-loading" : ""}`}
                      style={{ "--project-file-depth": row.depth } as CSSProperties}
                      role="treeitem"
                      aria-level={row.depth + 1}
                      aria-expanded={directory ? row.expanded : undefined}
                      aria-busy={loading || undefined}
                      onContextMenu={(event) => showProjectEntryContextMenu(event, row)}
                      onClick={() => {
                        if (directory) {
                          toggleDirectory(row.relativePath);
                        } else {
                          const mode = /\.html?$/i.test(row.name) ? "browser" : "files";
                          if (projectId && selectedFolder?.path) {
                            onOpenTab?.({
                              id: projectFileTabId(
                                projectId,
                                selectedFolder.path,
                                row.relativePath,
                              ),
                              projectId,
                              folderPath: selectedFolder.path,
                              name: row.name,
                              relativePath: row.relativePath,
                              mode,
                            });
                          }
                          changeWorkspaceMode(mode);
                        }
                      }}
                    >
                      <span className="project-files-tree-row__chevron">
                        {directory ? (
                          <ChevronRight size={15} strokeWidth={2} aria-hidden="true" />
                        ) : null}
                      </span>
                      <span className="project-files-tree-row__icon" data-file-icon={iconKind}>
                        <ProjectFileIcon
                          kind={iconKind}
                          size={iconKind === "folder" || iconKind === "folder-open" ? 16 : 15}
                        />
                      </span>
                      <span className="project-files-tree-row__label">{row.name}</span>
                    </button>
                  );
                })}
              </div>
            )}
            {errorMessage && listings[""] ? (
              <div className="project-files-tree-inline-error">{errorMessage}</div>
            ) : null}
          </div>
        </aside>
      </div>
      {contextMenu ? (
        <div
          ref={contextMenuRef}
          className="project-files-context-menu"
          role="menu"
          aria-label={`${contextMenu.name} 操作`}
          style={{ left: contextMenu.x, top: contextMenu.y }}
        >
          <button
            type="button"
            role="menuitem"
            disabled={addTaskDisabled || !onAddTaskContext}
            onClick={() => void addContextMenuEntryToTask()}
          >
            <Plus size={16} strokeWidth={1.9} aria-hidden="true" />
            <span>添加任务</span>
          </button>
          <button type="button" role="menuitem" onClick={() => void copyContextMenuEntryPath()}>
            <Copy size={15} strokeWidth={1.9} aria-hidden="true" />
            <span>复制路径</span>
          </button>
          <div className="project-files-context-menu__separator" role="separator" />
          <div
            ref={contextOpenWithRef}
            className="project-files-context-menu__open-with"
            onMouseEnter={() =>
              setContextMenu((current) =>
                current ? { ...current, openWithMenuOpen: true } : current,
              )
            }
          >
            <button
              type="button"
              role="menuitem"
              aria-haspopup="menu"
              aria-expanded={contextMenu.openWithMenuOpen}
              onFocus={() =>
                setContextMenu((current) =>
                  current ? { ...current, openWithMenuOpen: true } : current,
                )
              }
              onClick={() =>
                setContextMenu((current) =>
                  current
                    ? {
                        ...current,
                        openWithMenuOpen: true,
                      }
                    : current,
                )
              }
            >
              <AppWindow size={15} strokeWidth={1.8} aria-hidden="true" />
              <span>打开方式</span>
              <ChevronRight
                className="project-files-context-menu__submenu-chevron"
                size={15}
                strokeWidth={2}
                aria-hidden="true"
              />
            </button>
            {contextMenu.openWithMenuOpen ? (
              <div
                ref={contextSubmenuRef}
                className={`project-files-context-submenu${
                  contextMenu.submenuSide === "left" ? " is-left" : ""
                }`}
                role="menu"
                aria-label="选择打开应用"
                style={{ top: contextMenu.submenuTop }}
              >
                {PROJECT_ENTRY_OPEN_APPLICATIONS.map((application) => {
                  const officeApplication = application.id === "wps" || application.id === "office";
                  return (
                    <button
                      key={application.id}
                      type="button"
                      role="menuitem"
                      disabled={contextMenu.request.kind === "directory" && officeApplication}
                      onClick={() =>
                        void openContextMenuEntryWith(application.id, application.label)
                      }
                    >
                      <img
                        className="project-files-context-submenu__application-icon"
                        src={application.icon}
                        alt=""
                        aria-hidden="true"
                        draggable={false}
                      />
                      <span>{application.label}</span>
                    </button>
                  );
                })}
              </div>
            ) : null}
          </div>
        </div>
      ) : null}
    </section>
  );
}
