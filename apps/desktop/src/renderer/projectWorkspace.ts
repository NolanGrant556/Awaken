export type WorkspaceProject = {
  id: string;
  backendProjectId?: string;
  label: string;
  path: string;
};

export type TrackedTrashOperation = {
  operationId: string;
  projectId: string;
  backendProjectId: string;
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

export function canReconcileTrackedTrashOperation(
  operationState: ProjectOperationReconciliationState,
  projectState: "active" | "removed" | "trash_pending" | undefined,
): boolean {
  if (operationState === "failed") {
    return projectState === "active";
  }
  if (operationState === "succeeded") {
    return projectState !== "active" && projectState !== "trash_pending";
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
