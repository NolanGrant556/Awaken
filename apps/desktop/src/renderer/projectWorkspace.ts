export type WorkspaceProjectFolderRole = "user" | "managed";

export type WorkspaceProjectFolder = {
  path: string;
  role: WorkspaceProjectFolderRole;
  isPrimary: boolean;
};

export type WorkspaceProject = {
  id: string;
  backendProjectId?: string;
  label: string;
  /** 主文件夹路径；Core 始终保证项目恰好有一个主文件夹。 */
  path: string;
  folders: WorkspaceProjectFolder[];
};

export type TrackedTrashOperation = {
  operationId: string;
  projectId: string;
  backendProjectId: string;
  /** 本次进废纸篓的文件夹；废纸篓是文件夹粒度而不是项目粒度。 */
  folderPath: string;
  label: string;
  wasExpanded: boolean;
};

export type ProjectOperationReconciliationState =
  | "pending"
  | "succeeded"
  | "failed";

export const TRACKED_TRASH_OPERATIONS_STORAGE_KEY =
  "awaken:projects:trash-operations";

const failedOperationPhases = new Set(["failed", "cancelled"]);
const committedOperationPhases = new Set(["cleaning", "completed"]);

export const DEFAULT_WORKSPACE_PROJECT_ID = "awaken";

export function projectOperationReconciliationState(operation: {
  status: "accepted" | "running" | "succeeded" | "failed" | "blocked";
  phase: string;
}): ProjectOperationReconciliationState {
  if (
    operation.status === "failed" ||
    operation.status === "blocked" ||
    failedOperationPhases.has(operation.phase)
  ) {
    return "failed";
  }
  if (
    operation.status === "succeeded" ||
    committedOperationPhases.has(operation.phase)
  ) {
    return "succeeded";
  }
  return "pending";
}

export type TrackedTrashProjectSnapshot = {
  state?: "active" | "removed" | "trash_pending";
  folders?: { path: string }[];
};

/**
 * 文件夹级废纸篓成功后项目仍然存活，只是这个文件夹被解绑；失败后文件夹原样挂回。
 * 因此对账只看「项目是否还挂着这个文件夹」，不再看项目是否被整体移除。
 */
export function canReconcileTrackedTrashOperation(
  operationState: ProjectOperationReconciliationState,
  project: TrackedTrashProjectSnapshot | undefined,
  folderPath: string,
): boolean {
  const stillAttached = Boolean(
    project?.folders?.some((folder) =>
      isSameWorkspacePath(folder.path, folderPath),
    ),
  );
  if (operationState === "failed") {
    return project?.state === "active" && stillAttached;
  }
  if (operationState === "succeeded") {
    return project?.state !== "trash_pending" && !stillAttached;
  }
  return false;
}

export function parseTrackedTrashOperations(
  serialized: string,
): TrackedTrashOperation[] {
  if (!serialized.trim()) {
    return [];
  }
  try {
    const parsed: unknown = JSON.parse(serialized);
    if (!Array.isArray(parsed)) {
      return [];
    }
    const operations: TrackedTrashOperation[] = [];
    const seen = new Set<string>();
    for (const candidate of parsed) {
      if (
        !candidate ||
        typeof candidate !== "object" ||
        !("operationId" in candidate) ||
        typeof candidate.operationId !== "string" ||
        !("projectId" in candidate) ||
        typeof candidate.projectId !== "string" ||
        !("backendProjectId" in candidate) ||
        typeof candidate.backendProjectId !== "string" ||
        !("folderPath" in candidate) ||
        typeof candidate.folderPath !== "string" ||
        !candidate.folderPath ||
        !("label" in candidate) ||
        typeof candidate.label !== "string" ||
        !("wasExpanded" in candidate) ||
        typeof candidate.wasExpanded !== "boolean" ||
        !candidate.operationId ||
        seen.has(candidate.operationId)
      ) {
        continue;
      }
      seen.add(candidate.operationId);
      operations.push({
        operationId: candidate.operationId,
        projectId: candidate.projectId,
        backendProjectId: candidate.backendProjectId,
        folderPath: candidate.folderPath,
        label: candidate.label,
        wasExpanded: candidate.wasExpanded,
      });
    }
    return operations.slice(-100);
  } catch {
    return [];
  }
}

export function upsertTrackedTrashOperation(
  operations: TrackedTrashOperation[],
  next: TrackedTrashOperation,
): TrackedTrashOperation[] {
  return [
    ...operations.filter(
      (operation) => operation.operationId !== next.operationId,
    ),
    next,
  ].slice(-100);
}

export function normalizeWorkspacePath(path: string): string {
  const trimmed = path.trim();
  if (trimmed === "/" || /^[A-Za-z]:[\\/]?$/.test(trimmed)) {
    return trimmed;
  }
  return trimmed.replace(/[\\/]+$/, "");
}

export function workspaceLabelForPath(path: string): string {
  const normalized = normalizeWorkspacePath(path);
  if (!normalized) {
    return "Workspace";
  }
  const segments = normalized.split(/[\\/]/).filter(Boolean);
  return segments[segments.length - 1] ?? normalized;
}

export function isSameWorkspacePath(a?: string, b?: string): boolean {
  if (!a || !b) {
    return false;
  }
  return normalizeWorkspacePath(a) === normalizeWorkspacePath(b);
}

export function projectIdForThread(
  thread: {
    threadId: string;
    projectId?: string;
    workspaceRoot?: string;
  },
  defaultWorkspacePath?: string,
): string | undefined {
  if (thread.projectId) {
    return thread.projectId;
  }
  return isSameWorkspacePath(thread.workspaceRoot, defaultWorkspacePath)
    ? DEFAULT_WORKSPACE_PROJECT_ID
    : undefined;
}

export function removeWorkspaceProject(
  projects: WorkspaceProject[],
  projectId: string,
): WorkspaceProject[] {
  return projects.filter((project) => project.id !== projectId);
}

export function primaryWorkspaceProjectFolder(
  project: Pick<WorkspaceProject, "path" | "folders">,
): WorkspaceProjectFolder | undefined {
  return (
    project.folders.find((folder) => folder.isPrimary) ?? project.folders[0]
  );
}

/**
 * run 的执行根：主文件夹在前，其余文件夹按项目里的顺序跟随。
 * TaskScope 的三个 roots 数组都用这个顺序，主文件夹同时是 cwd 与相对路径基准。
 */
export function workspaceProjectRunRoots(
  project: Pick<WorkspaceProject, "path" | "folders">,
): string[] {
  const primary = normalizeWorkspacePath(
    primaryWorkspaceProjectFolder(project)?.path ?? project.path,
  );
  const roots = primary ? [primary] : [];
  for (const folder of project.folders) {
    const path = normalizeWorkspacePath(folder.path);
    if (path && !roots.includes(path)) {
      roots.push(path);
    }
  }
  return roots;
}

export function workspaceProjectForFolderPath(
  projects: WorkspaceProject[],
  folderPath: string | undefined,
): WorkspaceProject | undefined {
  if (!folderPath) {
    return undefined;
  }
  return projects.find((project) =>
    project.folders.some((folder) =>
      isSameWorkspacePath(folder.path, folderPath),
    ),
  );
}
