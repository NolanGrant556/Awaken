import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  isSameWorkspacePath,
  normalizeWorkspacePath,
  workspaceLabelForPath,
  workspaceProjectForFolderPath,
  workspaceProjectRunRoots,
  type WorkspaceProject,
  type WorkspaceProjectFolder,
} from "./projectWorkspace";

export type ProjectDeleteAction = "detach" | "delete_history" | "trash";

export type PendingProjectDelete = {
  projectId: string;
  backendProjectId: string;
  label: string;
  action: ProjectDeleteAction;
  threadCount: number;
  /** 仅 trash 用：废纸篓是文件夹粒度，这里是本次要移走的那个目录。 */
  folderPath?: string;
};

export type PendingThreadDelete = {
  threadIds: string[];
  source: "single" | "batch";
};

export function shouldSubmitProjectNameInputKey(event: {
  key: string;
  isComposing?: boolean;
}): boolean {
  return event.key === "Enter" && !event.isComposing;
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
  conversationScope?: boolean;
  selectedProjectId?: string;
  defaultWorkspacePath?: string;
  defaultProjectRemoved?: boolean;
  activeThreadWorkspacePath?: string;
  projectsFromWorkspace: WorkspaceProject[];
}): string {
  const {
    workspacePath,
    workspaceTargetPath,
    conversationScope,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  } = args;

  // A project-less conversation is deliberately unbound. Core owns its
  // thread-keyed managed workspace, so neither a remembered default folder nor
  // an existing thread's resolved path may turn it back into a project run.
  if (conversationScope) {
    return "";
  }

  const explicitTarget = workspaceTargetPath?.trim();
  if (explicitTarget) {
    return explicitTarget;
  }

  const selectedProject =
    selectedProjectId === undefined ||
    selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
      ? undefined
      : projectsFromWorkspace.find(
          (project) => project.id === selectedProjectId,
        );

  // A project the user created without picking a folder anchors nothing on disk.
  // Returning empty lets Core resolve that project's session workspace, instead
  // of falling through and silently borrowing the previous thread's directory.
  if (selectedProject && selectedProject.folders.length === 0) {
    return "";
  }

  const selectedProjectPath =
    selectedProjectId === undefined
      ? undefined
      : selectedProjectId === DEFAULT_WORKSPACE_PROJECT_ID
        ? defaultProjectRemoved
          ? undefined
          : defaultWorkspacePath
        : selectedProject?.path;

  return (
    [
      selectedProjectPath,
      activeThreadWorkspacePath,
      workspacePath,
      defaultProjectRemoved ? undefined : defaultWorkspacePath,
    ]
      .find((candidate) => candidate?.trim())
      ?.trim() ?? ""
  );
}

/**
 * 决定一批选中的目录里哪些能加入项目。
 *
 * 文件夹选择器允许一次选中多个目录，所以整批因为其中一个不合格而失败是没必要的：
 * 不能加入的逐个说明原因，其余照常加入。已接受的目录也参与后续判重，这样同一批里
 * 指向同一位置的两个选择不会被加入两次。
 *
 * 目录属于哪些项目是用户的事，所以这里唯一会跳过的情况是它已经在当前这一组里，
 * 也就是加入了也不会产生任何变化。被别的项目持有不构成理由。
 */
export function partitionPickedProjectFolders(args: {
  candidates: string[];
  /** 已经挂在目标项目上的文件夹。 */
  attachedPaths: string[];
}): { accepted: string[]; skipped: string[] } {
  const accepted: string[] = [];
  const skipped: string[] = [];
  for (const candidate of args.candidates) {
    const label = workspaceLabelForPath(candidate);
    if (
      [...args.attachedPaths, ...accepted].some((taken) =>
        isSameWorkspacePath(taken, candidate),
      )
    ) {
      skipped.push(`“${label}”已经在这个项目里。`);
      continue;
    }
    accepted.push(candidate);
  }
  return { accepted, skipped };
}

/**
 * 废纸篓确认文案里的共享说明。
 *
 * 移到废纸篓真的会删掉磁盘目录，所以这个文件夹会同时从每一个持有它的项目里消失。
 * 别的项目也在用它时必须在确认之前说明，否则用户是在替另一个项目做决定而不自知。
 */
export function sharedFolderTrashWarning(args: {
  folderPath?: string;
  actingProjectId?: string;
  projects: { id: string; label: string; folders: { path: string }[] }[];
}): string {
  if (!args.folderPath) {
    return "";
  }
  const others = args.projects
    .filter(
      (project) =>
        project.id !== args.actingProjectId &&
        project.folders.some((folder) =>
          isSameWorkspacePath(folder.path, args.folderPath),
        ),
    )
    .map((project) => `“${project.label}”`);
  if (others.length === 0) {
    return "";
  }
  return `该文件夹同时属于${others.join("、")}，也会一并从这些项目中移除。`;
}

/**
 * run 的执行根集合：cwd 所属项目的全部文件夹。
 *
 * 匹配按「任意文件夹」而不是「主文件夹」进行。锚在次文件夹上的 run（继续旧线程、
 * 或切换过主文件夹之后）同样属于这个项目，只认主文件夹会让它塌回单根，把主文件夹
 * 和其余文件夹挤出 scope。cwd 排在首位，因为搜索类工具按这个顺序遍历根。
 *
 * 共享文件夹下同一个根会命中多个项目，这时候 projectId 决定用哪一组：run 的归属已经
 * 由 resolveRunProjectForRun 定过，roots 必须跟它取同一个项目，否则模型会拿到另一个
 * 项目的文件夹组。
 */
export function resolveRunRootsForRun(args: {
  workspaceRoot: string;
  projectsFromWorkspace: WorkspaceProject[];
  projectId?: string;
}): string[] {
  const root = normalizeWorkspacePath(args.workspaceRoot ?? "");
  if (!root) {
    return [];
  }
  const project =
    args.projectsFromWorkspace.find(
      (candidate) =>
        candidate.id === args.projectId &&
        candidate.folders.some((folder) =>
          isSameWorkspacePath(folder.path, root),
        ),
    ) ?? workspaceProjectForFolderPath(args.projectsFromWorkspace, root);
  if (!project) {
    return [root];
  }
  const roots = workspaceProjectRunRoots(project).filter(
    (candidate) => !isSameWorkspacePath(candidate, root),
  );
  return [root, ...roots];
}

/**
 * run 归属的项目身份与展示名。
 *
 * 两者一起解析，因为它们一起下发：`project_id` 让零文件夹项目找到自己的会话目录，
 * `project_name` 是模型侧「当前项目」那一行。只解析出 id 而漏掉名字，模型就只剩一堆
 * 路径，会重新退回到只认 cwd 的行为。
 *
 * 同一个目录可以同时属于多个项目，所以路径本身不再能确定身份：选中的项目只要确实
 * 持有这个根就优先采信，只有在选中项与这个根无关时（比如继续一条旧线程）才回退到
 * 按路径查找。匹配按「任意文件夹」进行，锚在次文件夹上的线程同样算这个项目。
 */
export function resolveRunProjectForRun(args: {
  workspaceRoot: string;
  selectedProjectId?: string;
  defaultWorkspacePath?: string;
  defaultProjectName?: string;
  effectiveDefaultWorkspacePath?: string;
  projectsFromWorkspace: WorkspaceProject[];
}): { projectId?: string; projectName?: string } {
  const {
    workspaceRoot,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectName,
    effectiveDefaultWorkspacePath,
    projectsFromWorkspace,
  } = args;
  const selectedHoldsRoot =
    selectedProjectId !== undefined &&
    selectedProjectId !== DEFAULT_WORKSPACE_PROJECT_ID &&
    projectsFromWorkspace
      .find((project) => project.id === selectedProjectId)
      ?.folders.some((folder) =>
        isSameWorkspacePath(folder.path, workspaceRoot),
      ) === true;
  // 没有根就只能靠选中项来定身份，那正是零文件夹项目的情形。
  const projectId = !workspaceRoot
    ? selectedProjectId
    : selectedHoldsRoot
      ? selectedProjectId
      : isSameWorkspacePath(workspaceRoot, effectiveDefaultWorkspacePath)
        ? DEFAULT_WORKSPACE_PROJECT_ID
        : workspaceProjectForFolderPath(projectsFromWorkspace, workspaceRoot)
            ?.id;
  if (!projectId) {
    return {};
  }
  if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
    return {
      projectId,
      projectName:
        defaultProjectName?.trim() ||
        (defaultWorkspacePath
          ? workspaceLabelForPath(defaultWorkspacePath)
          : "awaken"),
    };
  }
  return {
    projectId,
    projectName: projectsFromWorkspace.find(
      (project) => project.id === projectId,
    )?.label,
  };
}

export function sameWorkspaceProjectFolders(
  current: WorkspaceProjectFolder[],
  next: WorkspaceProjectFolder[],
): boolean {
  return (
    next.length === current.length &&
    next.every((folder, index) => {
      const existing = current[index];
      return (
        existing?.path === folder.path &&
        existing.role === folder.role &&
        existing.isPrimary === folder.isPrimary
      );
    })
  );
}

export function sameWorkspaceProjects(
  current: WorkspaceProject[],
  next: WorkspaceProject[],
): boolean {
  return (
    next.length === current.length &&
    next.every((project, index) => {
      const existing = current[index];
      return (
        existing?.id === project.id &&
        existing.backendProjectId === project.backendProjectId &&
        existing.displayName === project.displayName &&
        existing.path === project.path &&
        existing.label === project.label &&
        sameWorkspaceProjectFolders(existing.folders, project.folders)
      );
    })
  );
}

export function resolveBackendProjectOrder(
  ids: string[],
  defaultBackendProjectId: string | undefined,
  projects: WorkspaceProject[],
): string[] | undefined {
  const backendById = new Map(
    projects.map((project) => [project.id, project.backendProjectId]),
  );
  const backendIds = ids.map((id) =>
    id === DEFAULT_WORKSPACE_PROJECT_ID
      ? defaultBackendProjectId
      : backendById.get(id),
  );
  return backendIds.every((id): id is string => Boolean(id))
    ? backendIds
    : undefined;
}

/** Frozen project selection consumed by one run submission. */
export type RunSubmissionScope = {
  valid: boolean;
  projectId?: string;
  workspace: import("../shared/runtime").StartRunRequest["workspace"];
};
