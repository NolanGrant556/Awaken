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

export function projectIdForWorkspacePath(
  path?: string,
  defaultWorkspacePath?: string,
): string {
  if (!path) {
    return DEFAULT_WORKSPACE_PROJECT_ID;
  }
  const normalized = normalizeWorkspacePath(path);
  if (!normalized || isSameWorkspacePath(normalized, defaultWorkspacePath)) {
    return DEFAULT_WORKSPACE_PROJECT_ID;
  }
  return workspaceProjectIdForPath(normalized);
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

export function normalizeWorkspaceProjects(
  projects: WorkspaceProject[],
  defaultWorkspacePath?: string,
): WorkspaceProject[] {
  const nextProjects: WorkspaceProject[] = [];
  const seenPaths = new Set<string>();

  for (const project of projects) {
    const normalized = normalizeWorkspacePath(project.path);
    if (
      !normalized ||
      isSameWorkspacePath(normalized, defaultWorkspacePath) ||
      seenPaths.has(normalized)
    ) {
      continue;
    }
    seenPaths.add(normalized);
    nextProjects.push({
      id: workspaceProjectIdForPath(normalized),
      label: workspaceLabelForPath(normalized),
      path: normalized,
    });
  }

  return nextProjects;
}

export function mergeWorkspaceProjects(
  projects: WorkspaceProject[],
  paths: string[],
  defaultWorkspacePath?: string,
): WorkspaceProject[] {
  let nextProjects = normalizeWorkspaceProjects(projects, defaultWorkspacePath);
  for (const path of paths) {
    nextProjects = upsertWorkspaceProject(
      nextProjects,
      path,
      defaultWorkspacePath,
    ).projects;
  }
  return normalizeWorkspaceProjects(nextProjects, defaultWorkspacePath);
}

export function projectIdForThread(
  thread:
    | string
    | {
        threadId: string;
        workspaceRoot?: string;
      },
  defaultWorkspacePath?: string,
): string {
  if (typeof thread === "string") {
    return DEFAULT_WORKSPACE_PROJECT_ID;
  }
  return projectIdForWorkspacePath(
    thread.workspaceRoot,
    defaultWorkspacePath,
  );
}

export function removeWorkspaceProject(
  projects: WorkspaceProject[],
  projectId: string,
): WorkspaceProject[] {
  return projects.filter((project) => project.id !== projectId);
}
