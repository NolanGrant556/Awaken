import {
  BrowserWindow,
  clipboard,
  dialog,
  ipcMain,
  shell
} from "electron";
import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import {
  normalizeRuntimeProject,
  normalizeRuntimeProjectOperation,
  type ProjectAgentsCreateResult,
  type ProjectDirectoryListRequest,
  type ProjectDirectoryListResult,
  type ProjectDirectoryWatchRequest,
  type ProjectDirectoryWatchResult,
  type ProjectEntryActionRequest,
  type ProjectEntryActionResult,
  type ProjectEntryOpenWithRequest,
  type ProjectEntryResolveResult,
  type ProjectFilePreviewSaveRequest,
  type ProjectFilePreviewSaveResult,
  type ProjectFolderSelection,
  type ProjectFoldersPickResult,
  type ProjectMutationResult,
  type ProjectReorderResult,
  type RuntimeProjectOperation,
  type RuntimeProjectOperationWire,
  type RuntimeProjectWire,
  type UpsertProjectResult
} from "../shared/runtime.js";
import { ProjectDirectoryWatchManager } from "./project-directory-watch-manager.js";
import {
  openProjectEntryWithApplication,
  revealProjectEntryInFileManager,
} from "./project-entry-actions.js";
import { ProjectFilePreviewManager } from "./project-file-preview-manager.js";
import type { ResolveProjectEntryResult } from "./project-files.js";
import { listProjectDirectory } from "./project-files.js";
import type { RuntimeRequester } from "./runtime-http.js";
import { RuntimeDomainError } from "./runtime-http.js";

type Dependencies = {
  readonly requestRuntime: RuntimeRequester["request"];
  readonly mainWindowRef: BrowserWindow | null;
  readonly projectDirectoryWatchManager: ProjectDirectoryWatchManager;
  readonly resolveCurrentProjectEntry: (request: ProjectEntryActionRequest) => Promise<ResolveProjectEntryResult>;
  readonly projectFilePreviewManager: ProjectFilePreviewManager;
  readonly upsertProjectFailureMessage: (error: unknown) => string;
  readonly requestProjectWrite: (pathname: string, method: "POST" | "PATCH" | "PUT" | "DELETE", body: unknown, logContext: Record<string, unknown>) => Promise<UpsertProjectResult>;
  readonly runProjectMutation: (pathname: string, method: "DELETE" | "POST", projectId: string, idempotencyKey: string, body?: unknown) => Promise<ProjectMutationResult>;
  readonly processPendingProjectTrashOperations: () => Promise<void>;
};

export function registerProjectsIpc(dependencies: Dependencies): void {
  ipcMain.handle("awaken:trust-project-hook", async (_event, request) =>
    dependencies.requestRuntime("/v1/hooks/trust", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    }),
  );

  ipcMain.handle("awaken:revoke-project-hook", async (_event, request) =>
    dependencies.requestRuntime("/v1/hooks/revoke", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    }),
  );

  ipcMain.handle("awaken:list-projects", async (_event, includeRemoved = false) => {
    const projects = await dependencies.requestRuntime<RuntimeProjectWire[]>(
      `/v1/projects?include_removed=${includeRemoved ? "true" : "false"}`,
      { method: "GET" },
    );
    return projects.map(normalizeRuntimeProject);
  });

  ipcMain.handle(
    "awaken:list-project-directory",
    async (event, request: ProjectDirectoryListRequest): Promise<ProjectDirectoryListResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "文件目录请求来源无效。",
        };
      }
      const projects = await dependencies.requestRuntime<RuntimeProjectWire[]>(
        "/v1/projects?include_removed=false",
        {
          method: "GET",
        },
      );
      return listProjectDirectory(projects.map(normalizeRuntimeProject), request);
    },
  );

  ipcMain.handle(
    "awaken:watch-project-directory",
    async (event, request: ProjectDirectoryWatchRequest): Promise<ProjectDirectoryWatchResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "文件目录订阅来源无效。",
        };
      }
      const projects = await dependencies.requestRuntime<RuntimeProjectWire[]>(
        "/v1/projects?include_removed=false",
        { method: "GET" },
      );
      return dependencies.projectDirectoryWatchManager.subscribe(
        projects.map(normalizeRuntimeProject),
        request,
        event.sender.id,
      );
    },
  );

  ipcMain.handle("awaken:unwatch-project-directory", (event, subscriptionId: string): void => {
    if (
      (!dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id) || typeof subscriptionId !== "string"
    ) {
      return;
    }
    dependencies.projectDirectoryWatchManager.unsubscribe(subscriptionId, event.sender.id);
  });

  ipcMain.handle(
    "awaken:resolve-project-entry-for-task",
    async (event, request: ProjectEntryActionRequest): Promise<ProjectEntryResolveResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "添加任务请求来源无效。",
        };
      }
      const result = await dependencies.resolveCurrentProjectEntry(request);
      if (!result.ok) return result;
      return {
        ok: true,
        entry: {
          projectId: result.entry.projectId,
          folderPath: result.entry.registeredRoot,
          absolutePath: result.entry.absolutePath,
          relativePath: result.entry.relativePath,
          displayName: result.entry.displayName,
          kind: result.entry.kind,
        },
      };
    },
  );

  ipcMain.handle(
    "awaken:copy-project-entry-path",
    async (event, request: ProjectEntryActionRequest): Promise<ProjectEntryActionResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "复制路径请求来源无效。",
        };
      }
      const result = await dependencies.resolveCurrentProjectEntry(request);
      if (!result.ok) return result;
      clipboard.writeText(result.entry.absolutePath);
      return { ok: true };
    },
  );

  ipcMain.handle(
    "awaken:reveal-project-entry",
    async (event, request: ProjectEntryActionRequest): Promise<ProjectEntryActionResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "本地路径请求来源无效。",
        };
      }
      const result = await dependencies.resolveCurrentProjectEntry(request);
      if (!result.ok) return result;
      return revealProjectEntryInFileManager(result.entry.realPath, result.entry.kind, {
        openPath: (realPath) => shell.openPath(realPath),
        showItemInFolder: (realPath) => shell.showItemInFolder(realPath),
      });
    },
  );

  ipcMain.handle(
    "awaken:open-project-entry-with",
    async (event, request: ProjectEntryOpenWithRequest): Promise<ProjectEntryActionResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "指定应用打开请求来源无效。",
        };
      }
      const result = await dependencies.resolveCurrentProjectEntry(request);
      if (!result.ok) return result;
      return openProjectEntryWithApplication(
        request.application,
        result.entry.absolutePath,
        result.entry.kind,
      );
    },
  );

  ipcMain.handle(
    "awaken:save-project-markdown-file",
    async (
      event,
      request: ProjectFilePreviewSaveRequest,
    ): Promise<ProjectFilePreviewSaveResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "Markdown 保存请求来源无效。",
        };
      }
      return dependencies.projectFilePreviewManager.saveMarkdown(request);
    },
  );

  ipcMain.handle(
    "awaken:create-project-agents-file",
    async (event, projectId: string): Promise<ProjectAgentsCreateResult> => {
      if (!dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id) {
        return { ok: false, code: "UNAUTHORIZED", message: "项目指令创建请求来源无效。" };
      }
      try {
        const projects = (await dependencies.requestRuntime<RuntimeProjectWire[]>(
          "/v1/projects?include_removed=false",
          { method: "GET" },
        )).map(normalizeRuntimeProject);
        const project = projects.find((candidate) => candidate.projectId === projectId && candidate.state === "active");
        if (!project) return { ok: false, code: "PROJECT_NOT_FOUND", message: "当前项目不存在或已被移除。" };
        const primary = project.folders.find((folder) => folder.isPrimary)?.path ?? project.workspaceRoot;
        if (!primary || !path.isAbsolute(primary)) {
          return { ok: false, code: "PRIMARY_FOLDER_MISSING", message: "当前项目没有可用的主文件夹。" };
        }
        const primaryReal = await fs.realpath(primary);
        const agentsDirectory = path.join(primaryReal, ".awaken");
        await fs.mkdir(agentsDirectory, { recursive: true, mode: 0o700 });
        const directoryReal = await fs.realpath(agentsDirectory);
        const relativeDirectory = path.relative(primaryReal, directoryReal);
        if (relativeDirectory === ".." || relativeDirectory.startsWith(`..${path.sep}`) || path.isAbsolute(relativeDirectory)) {
          return { ok: false, code: "CREATE_FAILED", message: ".awaken 目录超出项目主文件夹。" };
        }
        const target = path.join(directoryReal, "AGENTS.md");
        const temporary = path.join(directoryReal, `.AGENTS.md.tmp-${randomUUID()}`);
        let handle: Awaited<ReturnType<typeof fs.open>> | undefined;
        try {
          handle = await fs.open(temporary, "wx", 0o600);
          await handle.writeFile("# 项目指令\n", "utf8");
          await handle.sync();
          await handle.close();
          handle = undefined;
          await fs.link(temporary, target);
          if (process.platform !== "win32") {
            const directoryHandle = await fs.open(directoryReal, "r");
            try { await directoryHandle.sync(); } finally { await directoryHandle.close(); }
          }
        } catch (error) {
          if (typeof error === "object" && error !== null && "code" in error && String(error.code) === "EEXIST") {
            return { ok: false, code: "FILE_EXISTS", message: "项目 AGENTS.md 已存在，未覆盖原文件。" };
          }
          throw error;
        } finally {
          await handle?.close().catch(() => undefined);
          await fs.rm(temporary, { force: true }).catch(() => undefined);
        }
        return { ok: true, projectId, folderPath: primary, relativePath: ".awaken/AGENTS.md", name: "AGENTS.md" };
      } catch {
        return { ok: false, code: "CREATE_FAILED", message: "暂时无法创建项目 AGENTS.md。" };
      }
    },
  );

  ipcMain.handle(
    "awaken:upsert-project",
    async (
      _event,
      request: { workspaceRoot: string; displayName?: string; },
    ): Promise<UpsertProjectResult> => {
      try {
        const project = await dependencies.requestRuntime<RuntimeProjectWire>("/v1/projects", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            workspace_root: request.workspaceRoot,
            display_name: request.displayName,
          }),
        });
        return { ok: true, project: normalizeRuntimeProject(project) };
      } catch (error) {
        const code = error instanceof RuntimeDomainError ? error.code : "INTERNAL_ERROR";
        console.error("upsert_project_failed", {
          workspaceRoot: request.workspaceRoot,
          code,
          error: error instanceof Error ? error.message : String(error),
        });
        return { ok: false, code, message: dependencies.upsertProjectFailureMessage(error) };
      }
    },
  );

  ipcMain.handle(
    "awaken:create-project",
    async (
      _event,
      request: { folders: ProjectFolderSelection[]; displayName?: string; },
    ): Promise<UpsertProjectResult> =>
      dependencies.requestProjectWrite(
        "/v1/projects",
        "POST",
        {
          folders: (request.folders ?? []).map((folder) => ({
            path: folder.path,
            primary: Boolean(folder.primary),
          })),
          display_name: request.displayName,
        },
        { folderCount: request.folders?.length ?? 0 },
      ),
  );

  ipcMain.handle(
    "awaken:rename-project",
    async (
      _event,
      request: { projectId: string; displayName: string; },
    ): Promise<UpsertProjectResult> =>
      dependencies.requestProjectWrite(
        `/v1/projects/${encodeURIComponent(request.projectId)}`,
        "PATCH",
        { display_name: request.displayName },
        { projectId: request.projectId },
      ),
  );

  ipcMain.handle(
    "awaken:attach-project-folder",
    async (_event, request: { projectId: string; path: string; }): Promise<UpsertProjectResult> =>
      dependencies.requestProjectWrite(
        `/v1/projects/${encodeURIComponent(request.projectId)}/folders`,
        "POST",
        { path: request.path },
        { projectId: request.projectId },
      ),
  );

  ipcMain.handle(
    "awaken:detach-project-folder",
    async (_event, request: { projectId: string; path: string; }): Promise<UpsertProjectResult> =>
      dependencies.requestProjectWrite(
        `/v1/projects/${encodeURIComponent(request.projectId)}/folders`,
        "DELETE",
        { path: request.path },
        { projectId: request.projectId },
      ),
  );

  ipcMain.handle(
    "awaken:set-primary-project-folder",
    async (_event, request: { projectId: string; path: string; }): Promise<UpsertProjectResult> =>
      dependencies.requestProjectWrite(
        `/v1/projects/${encodeURIComponent(request.projectId)}/primary-folder`,
        "PUT",
        { path: request.path },
        { projectId: request.projectId },
      ),
  );

  ipcMain.handle(
    "awaken:reorder-projects",
    async (_event, projectIds: string[]): Promise<ProjectReorderResult> => {
      try {
        const projects = await dependencies.requestRuntime<RuntimeProjectWire[]>("/v1/projects/order", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ project_ids: projectIds }),
        });
        return { ok: true, projects: projects.map(normalizeRuntimeProject) };
      } catch (error) {
        const code = error instanceof RuntimeDomainError ? error.code : "INTERNAL_ERROR";
        console.error("reorder_projects_failed", {
          projectIds,
          code,
          error: error instanceof Error ? error.message : String(error),
        });
        return {
          ok: false,
          code,
          message: "项目顺序暂时未保存，已恢复原顺序，请稍后重试。",
        };
      }
    },
  );

  ipcMain.handle(
    "awaken:detach-project",
    async (
      _event,
      request: { projectId: string; idempotencyKey: string; },
    ): Promise<ProjectMutationResult> =>
      dependencies.runProjectMutation(
        `/v1/projects/${encodeURIComponent(request.projectId)}`,
        "DELETE",
        request.projectId,
        request.idempotencyKey,
      ),
  );

  ipcMain.handle(
    "awaken:trash-project-folder",
    async (
      _event,
      request: { projectId: string; path: string; idempotencyKey: string; },
    ): Promise<ProjectMutationResult> => {
      const result = await dependencies.runProjectMutation(
        `/v1/projects/${encodeURIComponent(request.projectId)}/folders/trash`,
        "POST",
        request.projectId,
        request.idempotencyKey,
        { path: request.path },
      );
      if (result.ok) {
        void dependencies.processPendingProjectTrashOperations();
      }
      return result;
    },
  );

  ipcMain.handle(
    "awaken:get-project-operation",
    async (_event, operationId: string): Promise<RuntimeProjectOperation> => {
      const operation = await dependencies.requestRuntime<RuntimeProjectOperationWire>(
        `/v1/project-operations/${encodeURIComponent(operationId)}`,
        { method: "GET" },
      );
      return normalizeRuntimeProjectOperation(operation);
    },
  );

  ipcMain.handle(
    "awaken:pick-project-folders",
    async (_event, defaultPath?: string): Promise<ProjectFoldersPickResult> => {
      const result = await dialog.showOpenDialog({
        properties: ["openDirectory", "multiSelections"],
        defaultPath,
      });
      if (result.canceled || result.filePaths.length === 0) {
        return { canceled: true };
      }
      const paths: string[] = [];
      const errors: string[] = [];
      for (const selected of result.filePaths) {
        try {
          const stat = await fs.stat(selected);
          if (stat.isDirectory()) {
            paths.push(selected);
          } else {
            errors.push(`${selected} 不是文件夹。`);
          }
        } catch {
          errors.push(`无法访问 ${selected}。`);
        }
      }
      if (paths.length === 0) {
        return {
          canceled: true,
          error: errors[0] ?? "Cannot access selected directory",
        };
      }
      return {
        canceled: false,
        paths,
        errors: errors.length > 0 ? errors : undefined,
      };
    },
  );
}
