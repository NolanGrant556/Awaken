import {
  AwakenIcon,
  AwakenSidebarSectionAction as SidebarSectionAction,
  type ProjectFolderRow,
  type ProjectHoverCard,
} from "@awaken/design-system";
import { useEffect, useRef, useState } from "react";
import type {
  RuntimeInfo,
  RuntimeThread,
  UpsertProjectResult,
} from "../shared/runtime";
import { readStoredString, writeStoredValue } from "./desktopStorage";
import {
  PendingProjectDelete,
  ProjectDeleteAction,
  partitionPickedProjectFolders,
  resolveBackendProjectOrder,
  sharedFolderTrashWarning,
  shouldResetActiveThreadForWorkspaceChange,
} from "./projectRunScope";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  TRACKED_TRASH_OPERATIONS_STORAGE_KEY,
  canReconcileTrackedTrashOperation,
  isSameWorkspacePath,
  parseTrackedTrashOperations,
  projectOperationReconciliationState,
  upsertTrackedTrashOperation,
  workspaceLabelForPath,
  type TrackedTrashOperation,
} from "./projectWorkspace";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeAction } from "./runtime/reducer";
import { useProjectRegistry } from "./useProjectRegistry";
import type { ThreadItem } from "./runtime/reducer";
type Options = {
  handleNewProjectSession: (id: string) => void;
  threadsForProject: (id: string) => ThreadItem[];
  runtimeClient: RuntimeClient;
  desktopReady: boolean;
  registry: Pick<
    ReturnType<typeof useProjectRegistry>,
    | "setDefaultProjectDisplayName"
    | "setProjectsFromWorkspace"
    | "workspaceTargetPathRef"
    | "workspacePath"
    | "setWorkspacePath"
    | "projectsFromWorkspace"
    | "setSelectedProjectId"
    | "setConversationScope"
    | "expandedProjectIds"
    | "setExpandedProjectIds"
    | "projectOrderIds"
    | "setProjectOrderIds"
    | "defaultBackendProjectId"
    | "defaultProjectFolders"
    | "applyRuntimeProjects"
    | "refreshProjects"
    | "defaultWorkspacePath"
    | "effectiveDefaultWorkspacePath"
    | "activeThreadWorkspacePath"
    | "projectLabelById"
    | "projectDisplayNameById"
    | "backendProjectIdFor"
    | "foldersForProjectId"
  >;
  runtimeInfo: Pick<RuntimeInfo, "defaultWorkspace"> | null;
  setNotice: (message: string | null) => void;
  refreshThreads: () => Promise<void>;
  loadThreadsIntoState: () => Promise<RuntimeThread[]>;
  resetActiveThreadContext: () => void;
  handleSelectProject: (id: string) => void;
  dispatch: (action: RuntimeAction) => void;
};
export function useProjectOperations(options: Options) {
  const {
    handleNewProjectSession,
    threadsForProject,
    runtimeClient,
    desktopReady,
    runtimeInfo,
    setNotice,
    refreshThreads,
    loadThreadsIntoState,
    resetActiveThreadContext,
    handleSelectProject,
    dispatch,
  } = options;
  const {
    setDefaultProjectDisplayName,
    setProjectsFromWorkspace,
    workspaceTargetPathRef,
    workspacePath,
    setWorkspacePath,
    projectsFromWorkspace,
    setSelectedProjectId,
    setConversationScope,
    expandedProjectIds,
    setExpandedProjectIds,
    projectOrderIds,
    setProjectOrderIds,
    defaultBackendProjectId,
    defaultProjectFolders,
    applyRuntimeProjects,
    refreshProjects,
    defaultWorkspacePath,
    effectiveDefaultWorkspacePath,
    activeThreadWorkspacePath,
    projectLabelById,
    projectDisplayNameById,
    backendProjectIdFor,
    foldersForProjectId,
  } = options.registry;

  const projectOperationPollFailuresRef = useRef<Set<string>>(new Set());

  const [createProjectDialogOpen, setCreateProjectDialogOpen] = useState(false);

  const [pendingProjectMode, setPendingProjectMode] =
    useState<"local">("local");

  const [pickingWorkspace, setPickingWorkspace] = useState(false);

  const [draftProjectFolders, setDraftProjectFolders] = useState<string[]>([]);

  const [draftPrimaryFolder, setDraftPrimaryFolder] = useState<string>("");

  const [draftProjectName, setDraftProjectName] = useState<string>("");

  const [createProjectSubmitting, setCreateProjectSubmitting] = useState(false);

  const [editingProjectId, setEditingProjectId] = useState<string | null>(null);

  const [editingProjectName, setEditingProjectName] = useState("");

  const [projectRenameSubmitting, setProjectRenameSubmitting] = useState(false);

  const [projectFolderBusyPath, setProjectFolderBusyPath] = useState<
    string | undefined
  >(undefined);

  const [projectReorderSubmitting, setProjectReorderSubmitting] =
    useState(false);

  const projectReorderSubmittingRef = useRef(false);

  const [pendingProjectDelete, setPendingProjectDelete] =
    useState<PendingProjectDelete | null>(null);

  const [projectDeleteSubmitting, setProjectDeleteSubmitting] = useState(false);

  const [trackedTrashOperations, setTrackedTrashOperations] = useState<
    TrackedTrashOperation[]
  >(() =>
    parseTrackedTrashOperations(
      readStoredString(TRACKED_TRASH_OPERATIONS_STORAGE_KEY),
    ),
  );

  useEffect(() => {
    writeStoredValue(
      TRACKED_TRASH_OPERATIONS_STORAGE_KEY,
      JSON.stringify(trackedTrashOperations),
    );
  }, [trackedTrashOperations]);

  useEffect(() => {
    if (!desktopReady || trackedTrashOperations.length === 0) {
      return;
    }

    let cancelled = false;
    let timer: number | undefined;

    async function pollTrackedTrashOperations(): Promise<void> {
      const checked = await Promise.all(
        trackedTrashOperations.map(async (tracked) => {
          try {
            const operation = await runtimeClient.getProjectOperation(
              tracked.operationId,
            );
            projectOperationPollFailuresRef.current.delete(tracked.operationId);
            return {
              tracked,
              operation,
              state: projectOperationReconciliationState(operation),
            };
          } catch (error) {
            if (
              !projectOperationPollFailuresRef.current.has(tracked.operationId)
            ) {
              projectOperationPollFailuresRef.current.add(tracked.operationId);
              console.warn("project_operation_poll_failed", {
                operationId: tracked.operationId,
                error,
              });
            }
            return undefined;
          }
        }),
      );
      if (cancelled) {
        return;
      }

      const terminal = checked.filter(
        (result): result is Exclude<(typeof checked)[number], undefined> =>
          Boolean(result && result.state !== "pending"),
      );
      if (terminal.length > 0) {
        try {
          const projects = await runtimeClient.listProjects(true);
          if (cancelled) {
            return;
          }
          const reconciled = terminal.filter(({ tracked, state }) => {
            const project = projects.find(
              (candidate) => candidate.projectId === tracked.backendProjectId,
            );
            return canReconcileTrackedTrashOperation(
              state,
              project,
              tracked.folderPath,
            );
          });

          if (reconciled.length > 0) {
            applyRuntimeProjects(projects);
            const reconciledIds = new Set(
              reconciled.map(({ tracked }) => tracked.operationId),
            );
            const failed = reconciled.filter(({ state }) => state === "failed");
            if (failed.length > 0) {
              setExpandedProjectIds((current) => {
                const next = [...current];
                for (const { tracked } of failed) {
                  if (
                    tracked.wasExpanded &&
                    !next.includes(tracked.projectId)
                  ) {
                    next.push(tracked.projectId);
                  }
                }
                return next;
              });
              setNotice(
                `未能将“${failed[0].tracked.label}”移到废纸篓，文件夹、项目历史和磁盘文件均保持不变。`,
              );
            } else {
              setNotice(
                `已将“${reconciled[0].tracked.label}”移到废纸篓，项目的其余文件夹不受影响。`,
              );
            }
            setTrackedTrashOperations((current) =>
              current.filter(
                (tracked) => !reconciledIds.has(tracked.operationId),
              ),
            );
            for (const operationId of reconciledIds) {
              projectOperationPollFailuresRef.current.delete(operationId);
            }

            void runtimeClient
              .listThreads()
              .then((threads) => {
                dispatch({ type: "run/threadsLoaded", threads });
              })
              .catch((error) => {
                console.warn("project_operation_thread_refresh_failed", error);
              });
          }
        } catch (error) {
          console.warn("project_operation_reconcile_failed", error);
        }
      }

      if (!cancelled) {
        timer = window.setTimeout(() => {
          void pollTrackedTrashOperations();
        }, 1000);
      }
    }

    void pollTrackedTrashOperations();
    return () => {
      cancelled = true;
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
    };
  }, [
    desktopReady,
    runtimeClient,
    runtimeInfo?.defaultWorkspace,
    trackedTrashOperations,
    workspacePath,
  ]);

  function projectAction(projectId: string, label: string) {
    return (
      <SidebarSectionAction
        label={`New session in ${label}`}
        onClick={(event) => {
          event.stopPropagation();
          handleNewProjectSession(projectId);
        }}
      >
        <AwakenIcon concept="thread-new" size={16} />
      </SidebarSectionAction>
    );
  }

  function openProjectEditor(projectId: string): void {
    setEditingProjectId(projectId);
    setEditingProjectName(projectLabelById.get(projectId) ?? "");
    setNotice(null);
  }

  function closeProjectEditor(): void {
    setEditingProjectId(null);
    setEditingProjectName("");
  }

  async function handleFinishEditingProject(): Promise<void> {
    const projectId = editingProjectId;
    if (!projectId || projectRenameSubmitting || projectFolderBusyPath) {
      return;
    }
    const displayName = editingProjectName.trim();
    if (!displayName) {
      setNotice("项目名称不能为空。");
      return;
    }
    if (displayName === projectDisplayNameById.get(projectId)?.trim()) {
      closeProjectEditor();
      return;
    }
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }

    setProjectRenameSubmitting(true);
    setNotice(null);
    try {
      const result = await runtimeClient.renameProject(
        backendProjectId,
        displayName,
      );
      if (!result.ok) {
        setNotice(result.message);
        return;
      }

      const renamedName = result.project.displayName;
      if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
        setDefaultProjectDisplayName(renamedName);
      } else {
        setProjectsFromWorkspace((current) =>
          current.map((project) =>
            project.id === projectId
              ? { ...project, displayName: renamedName, label: renamedName }
              : project,
          ),
        );
      }
      closeProjectEditor();
      setNotice(`项目已重命名为“${renamedName}”。`);
      await refreshProjects().catch(() => []);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "项目重命名暂时失败。",
      );
    } finally {
      setProjectRenameSubmitting(false);
    }
  }

  function requestProjectDelete(
    projectId: string,
    action: ProjectDeleteAction,
  ): void {
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    setPendingProjectDelete({
      projectId,
      backendProjectId,
      label: projectLabelById.get(projectId) ?? "项目",
      action,
      threadCount: threadsForProject(projectId).length,
    });
  }

  async function handleRevealFolder(folderPath: string): Promise<void> {
    const result = await runtimeClient.revealPath(folderPath);
    if (!result.ok) {
      setNotice(result.message ?? "无法在文件管理器中定位该目录。");
    }
  }

  /**
   * 悬停项目行时展示这个项目由哪些文件夹组成。侧栏树本身不展开文件夹行，
   * 文件夹的完整视图和写操作都留在编辑项目弹窗里。
   */
  function projectHoverCard(
    projectId: string,
    label: string,
    threadCount: number,
  ): ProjectHoverCard {
    return {
      title: label,
      summary: threadCount > 0 ? `${threadCount} 个对话串` : "暂无对话串",
      folders: foldersForProjectId(projectId).map((folder) => ({
        path: folder.path,
        label: workspaceLabelForPath(folder.path),
      })),
      emptyFoldersLabel: "还没有文件夹，对话会在这个项目自己的工作目录里进行",
      revealHint: "在文件管理器中显示：",
      actionLabel: "编辑项目",
      onRevealFolder: (path) => {
        void handleRevealFolder(path);
      },
      onAction: () => {
        openProjectEditor(projectId);
      },
    };
  }

  /**
   * 项目层菜单只负责项目自身：加文件夹、编辑文件夹列表、从 Awaken 移除、删除历史。
   * 「移到废纸篓」是文件夹粒度的，只出现在编辑项目弹窗里的文件夹行菜单。
   */
  function projectMenuItems(projectId: string) {
    return [
      {
        id: "add-project-folder",
        label: "添加文件夹",
        onSelect: () => {
          void handleAddProjectFolder(projectId);
        },
      },
      {
        id: "edit-project",
        label: "编辑项目",
        onSelect: () => {
          openProjectEditor(projectId);
        },
      },
      {
        id: "remove-project",
        label: "从 Awaken 移除",
        onSelect: () => {
          requestProjectDelete(projectId, "detach");
        },
      },
      {
        id: "delete-project-history",
        label: "删除 Awaken 历史",
        tone: "danger" as const,
        onSelect: () => {
          requestProjectDelete(projectId, "delete_history");
        },
      },
    ];
  }

  async function handleConfirmProjectDelete(): Promise<void> {
    const pending = pendingProjectDelete;
    if (!pending || projectDeleteSubmitting || !desktopReady) {
      return;
    }
    setProjectDeleteSubmitting(true);
    setNotice(null);
    const idempotencyKey = window.crypto.randomUUID();
    try {
      const result =
        pending.action === "detach"
          ? await runtimeClient.detachProject(
              pending.backendProjectId,
              idempotencyKey,
            )
          : pending.action === "delete_history"
            ? await runtimeClient.deleteProjectThreads(
                pending.backendProjectId,
                idempotencyKey,
              )
            : await runtimeClient.trashProjectFolder(
                pending.backendProjectId,
                pending.folderPath ?? "",
                idempotencyKey,
              );
      if (!result.ok) {
        setNotice(result.message);
        return;
      }

      const operation = result.operation;
      const operationState = projectOperationReconciliationState(operation);
      if (operationState === "failed") {
        await Promise.all([refreshProjects(), loadThreadsIntoState()]);
        setNotice(
          pending.action === "trash"
            ? "未能移到废纸篓，项目、历史和磁盘文件均保持不变。"
            : "项目操作未完成，请稍后重试。",
        );
        setPendingProjectDelete(null);
        return;
      }

      if (pending.action === "trash" && operationState === "pending") {
        setTrackedTrashOperations((current) =>
          upsertTrackedTrashOperation(current, {
            operationId: operation.operationId,
            projectId: pending.projectId,
            backendProjectId: pending.backendProjectId,
            folderPath: pending.folderPath ?? "",
            label: pending.label,
            wasExpanded: expandedProjectIds.includes(pending.projectId),
          }),
        );
        if (
          isSameWorkspacePath(activeThreadWorkspacePath, pending.folderPath)
        ) {
          resetActiveThreadContext();
        }
        setPendingProjectDelete(null);
        setNotice(`正在将“${pending.label}”移到废纸篓，Awaken 会自动完成。`);
        const refreshResults = await Promise.allSettled([
          refreshProjects(),
          loadThreadsIntoState(),
        ]);
        for (const refreshResult of refreshResults) {
          if (refreshResult.status === "rejected") {
            console.warn(
              "project_operation_initial_refresh_failed",
              refreshResult.reason,
            );
          }
        }
        return;
      }

      if (
        pending.action === "detach" ||
        (pending.action === "trash" &&
          isSameWorkspacePath(activeThreadWorkspacePath, pending.folderPath))
      ) {
        resetActiveThreadContext();
      }
      await Promise.all([refreshProjects(), loadThreadsIntoState()]);
      setExpandedProjectIds((current) =>
        pending.action === "detach"
          ? current.filter((id) => id !== pending.projectId)
          : current,
      );
      setPendingProjectDelete(null);
      setNotice(
        pending.action === "detach"
          ? `已从 Awaken 移除“${pending.label}”，磁盘文件和历史均已保留。`
          : pending.action === "delete_history"
            ? `已删除“${pending.label}”的 Awaken 历史，磁盘文件未改变。`
            : `已将“${pending.label}”移到废纸篓，项目的其余文件夹不受影响。`,
      );
    } catch (error) {
      console.error("project_delete_ui_failed", error);
      setNotice("项目操作暂时失败，请稍后重试。");
      await Promise.all([refreshProjects().catch(() => []), refreshThreads()]);
    } finally {
      setProjectDeleteSubmitting(false);
    }
  }

  async function handleReorderProjects(ids: string[]): Promise<void> {
    if (!desktopReady || projectReorderSubmittingRef.current) return;
    const backendIds = resolveBackendProjectOrder(
      ids,
      defaultBackendProjectId,
      projectsFromWorkspace,
    );
    if (!backendIds) {
      setNotice("项目顺序暂时无法保存，请刷新后重试。");
      await refreshProjects();
      return;
    }

    const previousOrder = projectOrderIds;
    projectReorderSubmittingRef.current = true;
    setProjectReorderSubmitting(true);
    setProjectOrderIds(ids);
    setNotice(null);
    try {
      const result = await runtimeClient.reorderProjects(backendIds);
      if (!result.ok) {
        setProjectOrderIds(previousOrder);
        setNotice(result.message);
        await refreshProjects();
        return;
      }
      applyRuntimeProjects(result.projects);
    } catch (error) {
      setProjectOrderIds(previousOrder);
      setNotice(
        error instanceof Error
          ? error.message
          : "项目顺序暂时未保存，已恢复原顺序，请稍后重试。",
      );
      await refreshProjects();
    } finally {
      projectReorderSubmittingRef.current = false;
      setProjectReorderSubmitting(false);
    }
  }

  function handleOpenCreateProject(): void {
    setPendingProjectMode("local");
    setDraftProjectFolders([]);
    setDraftPrimaryFolder("");
    setDraftProjectName("");
    setCreateProjectDialogOpen(true);
    setNotice(null);
  }

  /** 目标项目之外，所有已经占用了某个目录的项目。 */
  /** 选一个或多个目录，逐个通过 Core 校验后再判定哪些能加入。 */
  async function pickProjectFolders(
    excludedPaths: string[],
  ): Promise<{ paths: string[]; skipped: string[] }> {
    const result = await runtimeClient.pickProjectFolders(
      workspacePath || defaultWorkspacePath,
    );
    if (result.canceled) {
      if (result.error) {
        setNotice(result.error);
      }
      return { paths: [], skipped: [] };
    }
    const readable: string[] = [];
    const skipped: string[] = [...(result.errors ?? [])];
    for (const candidate of result.paths) {
      const validation = await runtimeClient.validateWorkspace(candidate);
      if (validation.valid) {
        readable.push(candidate);
      } else {
        skipped.push(
          `“${workspaceLabelForPath(candidate)}”：${validation.error}`,
        );
      }
    }
    const partition = partitionPickedProjectFolders({
      candidates: readable,
      attachedPaths: excludedPaths,
    });
    return {
      paths: partition.accepted,
      skipped: [...skipped, ...partition.skipped],
    };
  }

  async function handleAddDraftProjectFolder(): Promise<void> {
    if (pickingWorkspace) {
      return;
    }
    setPickingWorkspace(true);
    setNotice(null);
    try {
      const { paths, skipped } = await pickProjectFolders(draftProjectFolders);
      if (skipped.length > 0) {
        setNotice(skipped.join(" "));
      }
      if (paths.length === 0) {
        return;
      }
      setDraftProjectFolders((current) => [...current, ...paths]);
      setDraftPrimaryFolder((current) => current || paths[0]);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to select folder",
      );
    } finally {
      setPickingWorkspace(false);
    }
  }

  function handleRemoveDraftProjectFolder(path: string): void {
    setDraftProjectFolders((current) => {
      const next = current.filter(
        (candidate) => !isSameWorkspacePath(candidate, path),
      );
      setDraftPrimaryFolder((primary) =>
        isSameWorkspacePath(primary, path) ? (next[0] ?? "") : primary,
      );
      return next;
    });
  }

  async function handleCreateProject(): Promise<void> {
    if (createProjectSubmitting || !desktopReady) {
      return;
    }
    setCreateProjectSubmitting(true);
    setNotice(null);
    try {
      const folders = draftProjectFolders.map((path) => ({
        path,
        primary: isSameWorkspacePath(path, draftPrimaryFolder),
      }));
      // The name the user typed wins. Without one, a bound folder still supplies
      // a sensible label, and a project with neither is named by the backend.
      const displayName =
        draftProjectName.trim() ||
        (draftPrimaryFolder
          ? workspaceLabelForPath(draftPrimaryFolder)
          : undefined);
      const result = await runtimeClient.createProject(folders, displayName);
      if (!result.ok) {
        setNotice(
          result.code === "PROJECT_OPERATION_CONFLICT"
            ? "该项目正在移除中"
            : result.message,
        );
        return;
      }

      const project = result.project;
      const nextProjectId = isSameWorkspacePath(
        project.workspaceRoot,
        effectiveDefaultWorkspacePath,
      )
        ? DEFAULT_WORKSPACE_PROJECT_ID
        : project.projectId;
      await refreshProjects();
      if (
        shouldResetActiveThreadForWorkspaceChange(
          activeThreadWorkspacePath,
          project.workspaceRoot,
        )
      ) {
        resetActiveThreadContext();
      }
      // A folder-less project has no path to select. Leaving the target and the
      // workspace path untouched would keep pointing at the previous project, so
      // both are cleared and the selection carries the identity instead.
      workspaceTargetPathRef.current = project.workspaceRoot;
      setWorkspacePath(project.workspaceRoot);
      setConversationScope(false);
      setSelectedProjectId(nextProjectId);
      setExpandedProjectIds((current) =>
        current.includes(nextProjectId) ? current : [...current, nextProjectId],
      );
      setCreateProjectDialogOpen(false);
      setDraftProjectFolders([]);
      setDraftPrimaryFolder("");
      setDraftProjectName("");
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to create project",
      );
    } finally {
      setCreateProjectSubmitting(false);
    }
  }

  async function runProjectFolderMutation(
    projectId: string,
    folderPath: string,
    mutate: (backendProjectId: string) => Promise<UpsertProjectResult>,
    successNotice: string,
  ): Promise<void> {
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    if (projectFolderBusyPath) {
      return;
    }
    setProjectFolderBusyPath(folderPath);
    setNotice(null);
    try {
      const result = await mutate(backendProjectId);
      if (!result.ok) {
        setNotice(
          result.code === "PROJECT_FOLDER_NOT_FOUND"
            ? "该文件夹已经不在这个项目里。"
            : result.message,
        );
        await refreshProjects();
        return;
      }
      await refreshProjects();
      setNotice(successNotice);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "项目文件夹操作暂时失败。",
      );
      await refreshProjects().catch(() => []);
    } finally {
      setProjectFolderBusyPath(undefined);
    }
  }

  /**
   * 逐个挂载并汇总结果。走 runProjectFolderMutation 的循环会为每个目录刷新一次项目
   * 列表并覆盖一次 notice，选中五个目录时用户只会看到最后一条。
   */
  async function attachProjectFolders(
    backendProjectId: string,
    paths: string[],
    skipped: string[],
  ): Promise<void> {
    const attached: string[] = [];
    const failures: string[] = [...skipped];
    for (const path of paths) {
      const label = workspaceLabelForPath(path);
      setProjectFolderBusyPath(path);
      try {
        const result = await runtimeClient.attachProjectFolder(
          backendProjectId,
          path,
        );
        if (result.ok) {
          attached.push(label);
        } else {
          failures.push(`“${label}”：${result.message}`);
        }
      } catch (error) {
        failures.push(
          `“${label}”：${error instanceof Error ? error.message : "加入失败"}`,
        );
      }
    }
    setProjectFolderBusyPath(undefined);
    await refreshProjects().catch(() => []);
    const success =
      attached.length > 0
        ? `已把${attached.map((label) => `“${label}”`).join("、")}加入项目。`
        : "";
    setNotice([success, ...failures].filter(Boolean).join(" ") || null);
  }

  async function handleAddProjectFolder(projectId: string): Promise<void> {
    if (pickingWorkspace || projectFolderBusyPath) {
      return;
    }
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    setPickingWorkspace(true);
    setNotice(null);
    try {
      const { paths, skipped } = await pickProjectFolders(
        foldersForProjectId(projectId).map((folder) => folder.path),
      );
      if (paths.length === 0) {
        if (skipped.length > 0) {
          setNotice(skipped.join(" "));
        }
        return;
      }
      await attachProjectFolders(backendProjectId, paths, skipped);
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "Failed to select folder",
      );
    } finally {
      setPickingWorkspace(false);
    }
  }

  async function handleSetPrimaryProjectFolder(
    projectId: string,
    path: string,
  ): Promise<void> {
    await runProjectFolderMutation(
      projectId,
      path,
      (id) => runtimeClient.setPrimaryProjectFolder(id, path),
      `已把“${workspaceLabelForPath(path)}”设为主文件夹。`,
    );
  }

  async function handleDetachProjectFolder(
    projectId: string,
    path: string,
  ): Promise<void> {
    await runProjectFolderMutation(
      projectId,
      path,
      (id) => runtimeClient.detachProjectFolder(id, path),
      `已把“${workspaceLabelForPath(path)}”从项目移除，磁盘文件保留。`,
    );
  }

  function requestProjectFolderTrash(projectId: string, path: string): void {
    const backendProjectId = backendProjectIdFor(projectId);
    if (!backendProjectId) {
      setNotice("项目正在初始化，请稍后重试。");
      return;
    }
    setPendingProjectDelete({
      projectId,
      backendProjectId,
      label: workspaceLabelForPath(path),
      action: "trash",
      threadCount: threadsForProject(projectId).length,
      folderPath: path,
    });
  }

  function projectFolderRows(projectId: string): ProjectFolderRow[] {
    // Every folder is one the user picked, so every row offers the same actions.
    // Removing the last one is allowed: the project keeps its name and threads
    // and goes back to running without a folder.
    return foldersForProjectId(projectId).map((folder) => ({
      path: folder.path,
      isPrimary: folder.isPrimary,
      onMakePrimary: (path: string) => {
        void handleSetPrimaryProjectFolder(projectId, path);
      },
      onRemove: (path: string) => {
        void handleDetachProjectFolder(projectId, path);
      },
      // × 只把文件夹移出项目，磁盘文件保留。真正动磁盘的废纸篓留在菜单里，
      // 不和这个非破坏性动作放在同一个一键位置。
      actions: [
        {
          id: "trash-folder",
          label: "移到废纸篓",
          tone: "danger" as const,
          onSelect: (path: string) => {
            requestProjectFolderTrash(projectId, path);
          },
        },
      ],
    }));
  }

  const projectDeleteDialogTitle =
    pendingProjectDelete?.action === "detach"
      ? "从 Awaken 移除项目"
      : pendingProjectDelete?.action === "delete_history"
        ? "删除 Awaken 历史"
        : "将文件夹移到废纸篓";

  const projectDeleteDialogBody = !pendingProjectDelete
    ? ""
    : pendingProjectDelete.action === "detach"
      ? `“${pendingProjectDelete.label}”会从侧栏移除。${pendingProjectDelete.threadCount} 条历史会话和磁盘文件都会保留。`
      : pendingProjectDelete.action === "delete_history"
        ? `将删除“${pendingProjectDelete.label}”的 ${pendingProjectDelete.threadCount} 条 Awaken 历史会话。项目和磁盘目录都会保留；运行中的任务会先自动停止。`
        : [
            `“${pendingProjectDelete.label}”的磁盘目录将进入系统废纸篓，并从项目中解绑。项目和它的 Awaken 历史都会保留；失败时文件夹、项目和文件都保持原样。`,
            sharedFolderTrashWarning({
              folderPath: pendingProjectDelete.folderPath,
              actingProjectId: pendingProjectDelete.projectId,
              projects: [
                ...projectsFromWorkspace,
                {
                  id: DEFAULT_WORKSPACE_PROJECT_ID,
                  label:
                    projectLabelById.get(DEFAULT_WORKSPACE_PROJECT_ID) ??
                    "awaken",
                  folders: defaultProjectFolders,
                },
              ],
            }),
          ]
            .filter(Boolean)
            .join(" ");

  const projectDeleteConfirmLabel =
    pendingProjectDelete?.action === "detach"
      ? "确认移除"
      : pendingProjectDelete?.action === "delete_history"
        ? "确认删除历史"
        : "移到废纸篓";
  return {
    projectOperationPollFailuresRef,
    createProjectDialogOpen,
    setCreateProjectDialogOpen,
    pendingProjectMode,
    setPendingProjectMode,
    pickingWorkspace,
    setPickingWorkspace,
    draftProjectFolders,
    setDraftProjectFolders,
    draftPrimaryFolder,
    setDraftPrimaryFolder,
    draftProjectName,
    setDraftProjectName,
    createProjectSubmitting,
    setCreateProjectSubmitting,
    editingProjectId,
    setEditingProjectId,
    editingProjectName,
    setEditingProjectName,
    projectRenameSubmitting,
    setProjectRenameSubmitting,
    projectFolderBusyPath,
    setProjectFolderBusyPath,
    projectReorderSubmitting,
    setProjectReorderSubmitting,
    projectReorderSubmittingRef,
    pendingProjectDelete,
    setPendingProjectDelete,
    projectDeleteSubmitting,
    setProjectDeleteSubmitting,
    trackedTrashOperations,
    setTrackedTrashOperations,
    projectAction,
    openProjectEditor,
    closeProjectEditor,
    handleFinishEditingProject,
    requestProjectDelete,
    handleRevealFolder,
    projectHoverCard,
    projectMenuItems,
    handleConfirmProjectDelete,
    handleReorderProjects,
    handleOpenCreateProject,
    pickProjectFolders,
    handleAddDraftProjectFolder,
    handleRemoveDraftProjectFolder,
    handleCreateProject,
    runProjectFolderMutation,
    attachProjectFolders,
    handleAddProjectFolder,
    handleSetPrimaryProjectFolder,
    handleDetachProjectFolder,
    requestProjectFolderTrash,
    projectFolderRows,
    projectDeleteDialogTitle,
    projectDeleteDialogBody,
    projectDeleteConfirmLabel,
  };
}
