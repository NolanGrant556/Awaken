export type WorkspaceProject = {
  id: string;
  label: string;
  path: string;
};

export const DEFAULT_WORKSPACE_PROJECT_ID = "awaken";

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

export function workspaceProjectIdForPath(path: string): string {
  return `workspace:${encodeURIComponent(normalizeWorkspacePath(path))}`;
}

export function isSameWorkspacePath(a?: string, b?: string): boolean {
  if (!a || !b) {
    return false;
  }
  return normalizeWorkspacePath(a) === normalizeWorkspacePath(b);
}

export function upsertWorkspaceProject(
  projects: WorkspaceProject[],
  path: string,
  defaultWorkspacePath?: string,
): { projects: WorkspaceProject[]; projectId: string } {
  const normalized = normalizeWorkspacePath(path);
  if (!normalized || isSameWorkspacePath(normalized, defaultWorkspacePath)) {
    return { projects, projectId: DEFAULT_WORKSPACE_PROJECT_ID };
  }

  const existing = projects.find((project) => isSameWorkspacePath(project.path, normalized));
  if (existing) {
    return { projects, projectId: existing.id };
  }

  const nextProject: WorkspaceProject = {
    id: workspaceProjectIdForPath(normalized),
    label: workspaceLabelForPath(normalized),
    path: normalized,
  };

  return { projects: [...projects, nextProject], projectId: nextProject.id };
}

export function projectIdForThread(
  threadId: string,
  threadProjectIds: Record<string, string>,
): string {
  return threadProjectIds[threadId] ?? DEFAULT_WORKSPACE_PROJECT_ID;
}

export function assignThreadToProject(
  threadProjectIds: Record<string, string>,
  threadId: string,
  projectId: string,
): Record<string, string> {
  return {
    ...threadProjectIds,
    [threadId]: projectId,
  };
}
