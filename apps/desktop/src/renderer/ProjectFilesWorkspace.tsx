import { useEffect, useMemo, useRef, useState } from "react";
import type {
  CSSProperties,
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import {
  Check,
  ChevronDown,
  ChevronRight,
  File,
  FileArchive,
  FileAudio2,
  FileCode2,
  FileImage,
  FileJson2,
  FileSpreadsheet,
  FileText,
  FileType,
  FileType2,
  FileVideo2,
  Folder,
  FolderOpen,
  Link2,
  Presentation,
  Search,
} from "lucide-react";
import { AwakenPopover as Popover } from "@awaken/design-system";
import type { ProjectDirectoryEntry } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import {
  projectFileIconKind,
  projectFileTreeRows,
  type ProjectFileIconKind,
  type ProjectFileTreeRow,
} from "./projectFiles";
import "./styles/project-files.css";

export type ProjectFilesFolder = {
  path: string;
  label: string;
  isPrimary: boolean;
};

export type ProjectFilesWorkspaceProps = {
  runtimeClient: Pick<RuntimeClient, "listProjectDirectory">;
  projectId?: string;
  projectLabel?: string;
  folders: ProjectFilesFolder[];
};

const MIN_FILE_LIST_WIDTH = 240;
const MAX_FILE_LIST_WIDTH = 400;
const MIN_FILE_PREVIEW_WIDTH = 240;
const DEFAULT_FILE_LIST_WIDTH = 360;
const DEFAULT_FILE_WORKSPACE_WIDTH = 720;
const FILE_LIST_RESIZE_STEP = 12;
const MINIMUM_WIDTH_FLING_SAMPLE_WINDOW = 160;
const MINIMUM_WIDTH_FLING_DISTANCE = 56;
const MINIMUM_WIDTH_FLING_VELOCITY = 0.5;

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

type PointerSample = {
  x: number;
  time: number;
};

function isMinimumWidthRightwardFling(samples: PointerSample[]): boolean {
  const finish = samples[samples.length - 1];
  if (!finish || samples.length < 2) return false;
  const start = samples.reduce((leftmost, sample) =>
    sample.x < leftmost.x ? sample : leftmost,
  );
  const distance = finish.x - start.x;
  const duration = Math.max(1, finish.time - start.time);
  return (
    distance >= MINIMUM_WIDTH_FLING_DISTANCE &&
    distance / duration >= MINIMUM_WIDTH_FLING_VELOCITY
  );
}

function preferredFolderPath(folders: ProjectFilesFolder[]): string {
  return folders.find((folder) => folder.isPrimary)?.path ?? folders[0]?.path ?? "";
}

type ProjectFileRowIconKind = ProjectFileIconKind | "folder-open";

function rowIconKind(entry: ProjectFileTreeRow): ProjectFileRowIconKind {
  if (entry.kind === "directory" && entry.expanded) return "folder-open";
  return projectFileIconKind(entry);
}

function entryIcon(entry: ProjectFileTreeRow) {
  const kind = rowIconKind(entry);
  if (kind === "folder-open") {
    return <FolderOpen size={16} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "folder") {
    return <Folder size={16} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "symlink") {
    return <Link2 size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "excel") {
    return <FileSpreadsheet size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "word" || kind === "text") {
    return <FileText size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "powerpoint") {
    return <Presentation size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "pdf") {
    return <FileType2 size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "markdown") {
    return <FileType size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "code") {
    return <FileCode2 size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "data") {
    return <FileJson2 size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "image") {
    return <FileImage size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "archive") {
    return <FileArchive size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "audio") {
    return <FileAudio2 size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  if (kind === "video") {
    return <FileVideo2 size={15} strokeWidth={1.75} aria-hidden="true" />;
  }
  return <File size={15} strokeWidth={1.75} aria-hidden="true" />;
}

export function ProjectFilesWorkspace({
  runtimeClient,
  projectId,
  projectLabel,
  folders,
}: ProjectFilesWorkspaceProps) {
  const workspaceBodyRef = useRef<HTMLDivElement>(null);
  const fileNavigatorRef = useRef<HTMLElement>(null);
  const [fileListExpanded, setFileListExpanded] = useState(true);
  const [fileListWidth, setFileListWidth] = useState<number>();
  const [folderMenuOpen, setFolderMenuOpen] = useState(false);
  const [selectedFolderPath, setSelectedFolderPath] = useState(() =>
    preferredFolderPath(folders),
  );
  const [filter, setFilter] = useState("");
  const [listings, setListings] = useState<
    Record<string, ProjectDirectoryEntry[]>
  >({});
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set());
  const [selectedFilePath, setSelectedFilePath] = useState<string>();
  const [errorMessage, setErrorMessage] = useState<string>();
  const requestGenerationRef = useRef(0);

  const selectedFolder =
    folders.find((folder) => folder.path === selectedFolderPath) ?? folders[0];

  useEffect(() => {
    if (folders.some((folder) => folder.path === selectedFolderPath)) {
      return;
    }
    setSelectedFolderPath(preferredFolderPath(folders));
  }, [folders, selectedFolderPath]);

  useEffect(() => {
    const generation = requestGenerationRef.current + 1;
    requestGenerationRef.current = generation;
    setListings({});
    setExpandedPaths(new Set());
    setLoadingPaths(new Set());
    setSelectedFilePath(undefined);
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
          (bodyWidth ?? DEFAULT_FILE_WORKSPACE_WIDTH) -
            MIN_FILE_PREVIEW_WIDTH,
        ),
      ),
    };
  }

  function currentFileListWidth(): number {
    if (fileListWidth !== undefined) return fileListWidth;
    const measuredWidth =
      fileNavigatorRef.current?.getBoundingClientRect().width ?? 0;
    return measuredWidth > 0 ? measuredWidth : DEFAULT_FILE_LIST_WIDTH;
  }

  function handleFileListResizeStart(
    event: ReactPointerEvent<HTMLDivElement>,
  ): void {
    if (!fileListExpanded || !workspaceBodyRef.current) return;
    event.preventDefault();
    const target = event.currentTarget;
    const pointerId = event.pointerId;
    target.setPointerCapture?.(pointerId);
    const bodyRight = workspaceBodyRef.current.getBoundingClientRect().right;
    const { minimum, maximum } = fileListWidthBounds();
    let minimumWidthSamples: PointerSample[] =
      currentFileListWidth() <= minimum
        ? [{ x: event.clientX, time: event.timeStamp }]
        : [];

    function recordMinimumWidthSample(
      clientX: number,
      timeStamp: number,
    ): boolean {
      minimumWidthSamples.push({ x: clientX, time: timeStamp });
      const cutoff = timeStamp - MINIMUM_WIDTH_FLING_SAMPLE_WINDOW;
      minimumWidthSamples = minimumWidthSamples.filter(
        (sample) => sample.time >= cutoff,
      );
      return isMinimumWidthRightwardFling(minimumWidthSamples);
    }

    function handlePointerMove(moveEvent: PointerEvent): void {
      if (moveEvent.pointerId !== pointerId) return;
      moveEvent.preventDefault();
      const nextWidth = clamp(
        bodyRight - moveEvent.clientX,
        minimum,
        maximum,
      );
      setFileListWidth(nextWidth);
      if (nextWidth > minimum) {
        minimumWidthSamples = [];
        return;
      }
      if (minimumWidthSamples.length === 0) {
        minimumWidthSamples = [
          { x: moveEvent.clientX, time: moveEvent.timeStamp },
        ];
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

  function handleFileListResizeKeyDown(
    event: ReactKeyboardEvent<HTMLDivElement>,
  ): void {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const { minimum, maximum } = fileListWidthBounds();
    const delta =
      event.key === "ArrowLeft"
        ? FILE_LIST_RESIZE_STEP
        : -FILE_LIST_RESIZE_STEP;
    setFileListWidth(
      clamp(currentFileListWidth() + delta, minimum, maximum),
    );
  }

  const rows = useMemo(
    () =>
      projectFileTreeRows({
        listings,
        expandedPaths,
        selectedPath: selectedFilePath,
        filter,
      }),
    [expandedPaths, filter, listings, selectedFilePath],
  );

  const emptyTitle = !projectId
    ? "选择项目"
    : folders.length === 0
      ? "项目没有文件夹"
      : "打开文件";
  const emptyDescription = !projectId
    ? "先从左侧选择一个项目"
    : folders.length === 0
      ? "为当前项目添加文件夹后即可浏览"
      : "从工作区目录树中选择文件";

  return (
    <section className="project-files-workspace" aria-label="项目文件">
      <header className="project-files-pathbar">
        <span className="project-files-pathbar__path">/</span>
        <button
          type="button"
          className={`project-files-pathbar__icon${
            fileListExpanded ? " is-expanded" : ""
          }`}
          aria-label={fileListExpanded ? "收起文件列表" : "展开文件列表"}
          aria-expanded={fileListExpanded}
          aria-controls="awaken-project-files-navigator"
          onClick={() => {
            setFolderMenuOpen(false);
            setFileListExpanded((expanded) => !expanded);
          }}
        >
          <FolderOpen size={18} strokeWidth={1.8} />
        </button>
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
        <div className="project-files-preview-empty">
          <FolderOpen size={42} strokeWidth={1.6} aria-hidden="true" />
          <strong>{emptyTitle}</strong>
          <span>{emptyDescription}</span>
        </div>

        <div
          className="project-files-divider"
          role="separator"
          aria-label="调整文件列表宽度"
          aria-orientation="vertical"
          aria-valuemin={MIN_FILE_LIST_WIDTH}
          aria-valuemax={Math.round(fileListWidthBounds().maximum)}
          aria-valuenow={Math.round(
            fileListWidth ?? DEFAULT_FILE_LIST_WIDTH,
          )}
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
                        className={`project-files-folder-option${
                          selected ? " is-selected" : ""
                        }`}
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
                        {selected ? (
                          <Check size={16} strokeWidth={1.9} aria-hidden="true" />
                        ) : null}
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
              <div className="project-files-tree-state is-error">
                {errorMessage}
              </div>
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
                      }${row.depth > 0 ? " is-nested" : ""}${
                        loading ? " is-loading" : ""
                      }`}
                      style={{ "--project-file-depth": row.depth } as CSSProperties}
                      role="treeitem"
                      aria-level={row.depth + 1}
                      aria-expanded={directory ? row.expanded : undefined}
                      aria-busy={loading || undefined}
                      onClick={() => {
                        if (directory) {
                          toggleDirectory(row.relativePath);
                        } else {
                          setSelectedFilePath(row.relativePath);
                        }
                      }}
                    >
                      <span className="project-files-tree-row__chevron">
                        {directory ? (
                          <ChevronRight
                            size={15}
                            strokeWidth={2}
                            aria-hidden="true"
                          />
                        ) : null}
                      </span>
                      <span
                        className="project-files-tree-row__icon"
                        data-file-icon={iconKind}
                      >
                        {entryIcon(row)}
                      </span>
                      <span className="project-files-tree-row__label">
                        {row.name}
                      </span>
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
    </section>
  );
}
