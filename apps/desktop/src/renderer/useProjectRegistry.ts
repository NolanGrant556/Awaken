import { useEffect, useRef, useState } from "react";
import type { RuntimeInfo, RuntimeProject } from "../shared/runtime";
import {
  resolveWorkspaceRootForRun,
  sameWorkspaceProjectFolders,
  sameWorkspaceProjects,
} from "./projectRunScope";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  isSameWorkspacePath,
  projectIdForThread,
  workspaceLabelForPath,
  type WorkspaceProject,
  type WorkspaceProjectFolder,
} from "./projectWorkspace";
import type { RuntimeClient } from "./runtime/client";
import { type ThreadItem } from "./runtime/reducer";

export function useProjectRegistry(
  runtimeClient: RuntimeClient,
  desktopReady: boolean,
  runtimeInfo: Pick<RuntimeInfo, "defaultWorkspace"> | null,
  activeThread: ThreadItem | undefined,
  refreshThreads: () => Promise<void>,
) {
  const workspaceTargetPathRef = useRef("");

  const [workspacePath, setWorkspacePath] = useState("");

  const [projectsFromWorkspace, setProjectsFromWorkspace] = useState<
    WorkspaceProject[]
  >([]);

  const [selectedProjectId, setSelectedProjectId] = useState<
    string | undefined
  >(undefined);

  const [conversationScope, setConversationScope] = useState(true);

  const [expandedProjectIds, setExpandedProjectIds] = useState<string[]>([]);

  const [conversationSectionExpanded, setConversationSectionExpanded] =
    useState(true);

  const [projectOrderIds, setProjectOrderIds] = useState<string[]>([]);

  const [removedProjectIds, setRemovedProjectIds] = useState<string[]>([]);

  const [defaultBackendProjectId, setDefaultBackendProjectId] = useState<
    string | undefined
  >(undefined);

  const [defaultProjectDisplayName, setDefaultProjectDisplayName] = useState<
    string | undefined
  >(undefined);

  const [defaultProjectFolders, setDefaultProjectFolders] = useState<
    WorkspaceProjectFolder[]
  >([]);

  const defaultProjectRemoved = removedProjectIds.includes(
    DEFAULT_WORKSPACE_PROJECT_ID,
  );

  useEffect(() => {
    workspaceTargetPathRef.current = workspacePath;
  }, [workspacePath]);

  useEffect(() => {
    window.localStorage?.removeItem("awaken:threads:projects");
  }, []);

  useEffect(() => {
    if (
      runtimeInfo &&
      runtimeInfo.defaultWorkspace &&
      workspacePath === "" &&
      !defaultProjectRemoved
    ) {
      setWorkspacePath(runtimeInfo.defaultWorkspace);
    }
  }, [defaultProjectRemoved, runtimeInfo, workspacePath]);

  function applyRuntimeProjects(projects: RuntimeProject[]): void {
    const configuredDefaultWorkspacePath =
      runtimeInfo?.defaultWorkspace ?? workspacePath;
    const defaultProject = projects.find((project) =>
      isSameWorkspacePath(
        project.workspaceRoot,
        configuredDefaultWorkspacePath,
      ),
    );
    setDefaultBackendProjectId(defaultProject?.projectId);
    setDefaultProjectDisplayName(
      defaultProject?.displayName.trim() || undefined,
    );
    const nextDefaultFolders =
      defaultProject?.folders.map<WorkspaceProjectFolder>((folder) => ({
        path: folder.path,
        role: folder.role,
        isPrimary: folder.isPrimary,
      })) ?? [];
    setDefaultProjectFolders((current) =>
      sameWorkspaceProjectFolders(current, nextDefaultFolders)
        ? current
        : nextDefaultFolders,
    );
    setRemovedProjectIds(
      defaultProject && defaultProject.state !== "active"
        ? [DEFAULT_WORKSPACE_PROJECT_ID]
        : [],
    );

    const nextProjectOrderIds = projects
      .filter((project) => project.state === "active")
      .map((project) =>
        isSameWorkspacePath(
          project.workspaceRoot,
          configuredDefaultWorkspacePath,
        )
          ? DEFAULT_WORKSPACE_PROJECT_ID
          : project.projectId,
      );
    setProjectOrderIds((current) =>
      current.length === nextProjectOrderIds.length &&
      current.every((id, index) => id === nextProjectOrderIds[index])
        ? current
        : nextProjectOrderIds,
    );

    const nextProjects = projects
      .filter(
        (project) =>
          project.state === "active" &&
          !isSameWorkspacePath(
            project.workspaceRoot,
            configuredDefaultWorkspacePath,
          ),
      )
      .map<WorkspaceProject>((project) => ({
        id: project.projectId,
        backendProjectId: project.projectId,
        displayName: project.displayName,
        label:
          project.displayName || workspaceLabelForPath(project.workspaceRoot),
        path: project.workspaceRoot,
        folders: project.folders.map<WorkspaceProjectFolder>((folder) => ({
          path: folder.path,
          role: folder.role,
          isPrimary: folder.isPrimary,
        })),
      }));
    setProjectsFromWorkspace((current) =>
      sameWorkspaceProjects(current, nextProjects) ? current : nextProjects,
    );
  }

  async function refreshProjects(): Promise<RuntimeProject[]> {
    const projects = await runtimeClient.listProjects(true);
    applyRuntimeProjects(projects);
    return projects;
  }

  useEffect(() => {
    if (!desktopReady) {
      return;
    }
    void refreshProjects().catch((error) => {
      console.error("project_list_refresh_failed", error);
    });
    void refreshThreads();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desktopReady, runtimeClient]);

  const defaultWorkspacePath = runtimeInfo?.defaultWorkspace ?? workspacePath;

  const effectiveDefaultWorkspacePath = defaultProjectRemoved
    ? undefined
    : defaultWorkspacePath;

  // Without a user-selected workspace there is no implicit default project;
  // 项目工作区 shows a true empty state until the user opens/adds a project.
  const hasDefaultWorkspaceProject = Boolean(
    effectiveDefaultWorkspacePath?.trim(),
  );

  const activeThreadWorkspacePath = activeThread?.workspaceRoot;

  const activeThreadProjectId = activeThread
    ? projectIdForThread(activeThread, effectiveDefaultWorkspacePath)
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
    conversationScope,
    selectedProjectId,
    defaultWorkspacePath,
    defaultProjectRemoved,
    activeThreadWorkspacePath,
    projectsFromWorkspace,
  });

  const selectedFolderlessProject =
    selectedProjectId !== undefined &&
    selectedProjectId !== DEFAULT_WORKSPACE_PROJECT_ID &&
    projectsFromWorkspace.find((project) => project.id === selectedProjectId)
      ?.folders.length === 0;

  const isWorkspaceValid =
    conversationScope ||
    selectedFolderlessProject ||
    (workspaceRootForRun !== "" &&
      /^([A-Za-z]:[\\/]|\/)/.test(workspaceRootForRun));

  useEffect(() => {
    const defaultProjectAvailable =
      !defaultProjectRemoved && hasDefaultWorkspaceProject;
    const knownProjectIds = new Set([
      ...(defaultProjectAvailable ? [DEFAULT_WORKSPACE_PROJECT_ID] : []),
      ...projectsFromWorkspace.map((project) => project.id),
    ]);
    const fallbackProjectId = defaultProjectAvailable
      ? DEFAULT_WORKSPACE_PROJECT_ID
      : projectsFromWorkspace[0]?.id;
    setSelectedProjectId((current) =>
      current === undefined
        ? undefined
        : knownProjectIds.has(current)
          ? current
          : fallbackProjectId,
    );
    setExpandedProjectIds((current) => {
      const next = current.filter((id) => knownProjectIds.has(id));
      return !defaultProjectAvailable ||
        next.includes(DEFAULT_WORKSPACE_PROJECT_ID)
        ? next
        : [DEFAULT_WORKSPACE_PROJECT_ID, ...next];
    });
  }, [
    defaultProjectRemoved,
    hasDefaultWorkspaceProject,
    projectsFromWorkspace,
  ]);

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
      defaultProjectDisplayName ||
        (defaultWorkspacePath
          ? workspaceLabelForPath(defaultWorkspacePath)
          : "awaken"),
    ],
    ...projectsFromWorkspace.map(
      (project) => [project.id, project.label] as const,
    ),
  ]);

  const projectDisplayNameById = new Map<string, string | undefined>([
    [DEFAULT_WORKSPACE_PROJECT_ID, defaultProjectDisplayName],
    ...projectsFromWorkspace.map(
      (project) => [project.id, project.displayName] as const,
    ),
  ]);

  const removedProjectIdSet = new Set(removedProjectIds);

  const workspaceLabelProjectId = activeThreadProjectId ?? selectedProjectId;

  const workspaceLabelPath =
    activeThreadWorkspacePath ?? pathForSelectedProject;

  const activeProjectLabel = conversationScope
    ? "选择项目"
    : ((workspaceLabelProjectId
        ? projectLabelById.get(workspaceLabelProjectId)
        : undefined) ??
      (workspaceLabelPath
        ? workspaceLabelForPath(workspaceLabelPath)
        : "awaken"));

  function backendProjectIdFor(projectId: string): string | undefined {
    if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
      return defaultBackendProjectId;
    }
    const project = projectsFromWorkspace.find((item) => item.id === projectId);
    return project?.backendProjectId;
  }

  function syncProjectSelectionForThread(
    thread?: Pick<ThreadItem, "threadId" | "projectId" | "workspaceRoot">,
  ): void {
    if (!thread) {
      return;
    }
    const nextProjectId = projectIdForThread(
      thread,
      effectiveDefaultWorkspacePath,
    );
    if (!nextProjectId) {
      setConversationScope(true);
      setSelectedProjectId(undefined);
      workspaceTargetPathRef.current = thread.workspaceRoot ?? "";
      setWorkspacePath(thread.workspaceRoot ?? "");
      setConversationSectionExpanded(true);
      return;
    }
    setConversationScope(false);
    setSelectedProjectId(nextProjectId);
    const projectHasNoFolders =
      nextProjectId !== DEFAULT_WORKSPACE_PROJECT_ID &&
      projectsFromWorkspace.find((project) => project.id === nextProjectId)
        ?.folders.length === 0;
    const nextWorkspacePath = projectHasNoFolders
      ? ""
      : (thread.workspaceRoot ?? "");
    workspaceTargetPathRef.current = nextWorkspacePath;
    setWorkspacePath(nextWorkspacePath);
    setExpandedProjectIds((current) =>
      current.includes(nextProjectId) ? current : [...current, nextProjectId],
    );
  }

  function foldersForProjectId(projectId: string): WorkspaceProjectFolder[] {
    if (projectId === DEFAULT_WORKSPACE_PROJECT_ID) {
      return defaultProjectFolders;
    }
    return (
      projectsFromWorkspace.find((project) => project.id === projectId)
        ?.folders ?? []
    );
  }
  return {
    setDefaultProjectDisplayName,
    setProjectsFromWorkspace,
    workspaceTargetPathRef,
    workspacePath,
    setWorkspacePath,
    projectsFromWorkspace,
    selectedProjectId,
    setSelectedProjectId,
    conversationScope,
    setConversationScope,
    expandedProjectIds,
    setExpandedProjectIds,
    conversationSectionExpanded,
    setConversationSectionExpanded,
    projectOrderIds,
    setProjectOrderIds,
    removedProjectIds,
    setRemovedProjectIds,
    defaultBackendProjectId,
    defaultProjectDisplayName,
    defaultProjectFolders,
    defaultProjectRemoved,
    applyRuntimeProjects,
    refreshProjects,
    defaultWorkspacePath,
    effectiveDefaultWorkspacePath,
    hasDefaultWorkspaceProject,
    activeThreadWorkspacePath,
    activeThreadProjectId,
    pathForSelectedProject,
    workspaceRootForRun,
    isWorkspaceValid,
    projectLabelById,
    projectDisplayNameById,
    removedProjectIdSet,
    workspaceLabelProjectId,
    workspaceLabelPath,
    activeProjectLabel,
    backendProjectIdFor,
    syncProjectSelectionForThread,
    foldersForProjectId,
  };
}
