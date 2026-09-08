import {
  BrowserWindow,
  ipcMain,
  shell
} from "electron";
import fs from "node:fs/promises";
import path from "node:path";
import {
  normalizeRuntimeThreadTranscript,
  type ArtifactOpenRequest,
  type ArtifactOpenResult,
  type ArtifactPreviewStartRequest,
  type ProjectFilePreviewActionRequest,
  type ProjectFilePreviewActionResult,
  type ProjectFilePreviewCloseRequest,
  type ProjectFilePreviewStartRequest,
  type ProjectFilePreviewStartResult,
  type RuntimeThreadTranscriptWire
} from "../shared/runtime.js";
import { ArtifactMediaManager } from "./artifact-media-manager.js";
import { openRecordedArtifact, revealRecordedArtifact } from "./artifact-open.js";
import { ProjectFilePreviewManager } from "./project-file-preview-manager.js";
import type { RuntimeRequester } from "./runtime-http.js";

type Dependencies = {
  readonly requestRuntime: RuntimeRequester["request"];
  readonly mainWindowRef: BrowserWindow | null;
  readonly projectFilePreviewManager: ProjectFilePreviewManager;
  readonly artifactMediaManager: ArtifactMediaManager;
};

export function registerFilesIpc(dependencies: Dependencies): void {
  ipcMain.handle(
    "awaken:start-project-file-preview",
    async (
      event,
      request: ProjectFilePreviewStartRequest,
    ): Promise<ProjectFilePreviewStartResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "文件预览请求来源无效。",
        };
      }

      return dependencies.projectFilePreviewManager.start(request);
    },
  );

  ipcMain.handle(
    "awaken:start-artifact-preview",
    async (
      event,
      request: ArtifactPreviewStartRequest,
    ): Promise<ProjectFilePreviewStartResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "交付物预览请求来源无效。",
        };
      }

      return dependencies.projectFilePreviewManager.startArtifact(request);
    },
  );

  ipcMain.handle(
    "awaken:close-project-file-preview",
    async (event, request: ProjectFilePreviewCloseRequest): Promise<void> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return;
      }
      dependencies.projectFilePreviewManager.cancel(request);
    },
  );

  ipcMain.handle(
    "awaken:open-project-file-externally",
    async (
      event,
      request: ProjectFilePreviewActionRequest,
    ): Promise<ProjectFilePreviewActionResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "文件打开请求来源无效。",
        };
      }
      return dependencies.projectFilePreviewManager.openExternally(request);
    },
  );

  ipcMain.handle(
    "awaken:reveal-project-file",
    async (
      event,
      request: ProjectFilePreviewActionRequest,
    ): Promise<ProjectFilePreviewActionResult> => {
      if (
        !dependencies.mainWindowRef || dependencies.mainWindowRef.isDestroyed() || event.sender.id !== dependencies.mainWindowRef.webContents.id
      ) {
        return {
          ok: false,
          code: "UNAUTHORIZED",
          message: "文件定位请求来源无效。",
        };
      }
      return dependencies.projectFilePreviewManager.reveal(request);
    },
  );

  ipcMain.handle(
    "awaken:open-artifact",
    async (event, request: ArtifactOpenRequest): Promise<ArtifactOpenResult> => {
      if (!dependencies.mainWindowRef || event.sender.id !== dependencies.mainWindowRef.webContents.id) {
        return {
          ok: false,
          code: "open_failed",
          message: "无法确认文件打开请求来自 Awaken 主窗口。",
        };
      }
      try {
        const transcript = await dependencies.requestRuntime<RuntimeThreadTranscriptWire>(
          `/v1/threads/${request.threadId}/transcript`,
          {
            method: "GET",
          },
        );
        return openRecordedArtifact(normalizeRuntimeThreadTranscript(transcript), request, (target) =>
          shell.openPath(target),
        );
      } catch (error) {
        console.error("artifact_open_failed", {
          threadId: request.threadId,
          runId: request.runId,
          artifactId: request.artifactId,
          error: error instanceof Error ? error.message : String(error),
        });
        return {
          ok: false,
          code: "runtime_unavailable",
          message: "暂时无法读取任务记录，请稍后重试。",
        };
      }
    },
  );

  ipcMain.handle(
    "awaken:get-artifact-media-source",
    async (event, request: ArtifactOpenRequest) => {
      if (!dependencies.mainWindowRef || event.sender.id !== dependencies.mainWindowRef.webContents.id) {
        return {
          ok: false as const,
          code: "open_failed" as const,
          message: "无法确认媒体预览请求来自 Awaken 主窗口。",
        };
      }

      return dependencies.artifactMediaManager.source(request);
    },
  );

  ipcMain.handle(
    "awaken:reveal-artifact",
    async (event, request: ArtifactOpenRequest): Promise<ArtifactOpenResult> => {
      if (!dependencies.mainWindowRef || event.sender.id !== dependencies.mainWindowRef.webContents.id) {
        return {
          ok: false,
          code: "open_failed",
          message: "无法确认文件定位请求来自 Awaken 主窗口。",
        };
      }
      try {
        const transcript = await dependencies.requestRuntime<RuntimeThreadTranscriptWire>(
          `/v1/threads/${request.threadId}/transcript`,
          {
            method: "GET",
          },
        );
        return revealRecordedArtifact(
          normalizeRuntimeThreadTranscript(transcript),
          request,
          (target) => shell.showItemInFolder(target),
        );
      } catch (error) {
        console.error("artifact_reveal_failed", {
          threadId: request.threadId,
          runId: request.runId,
          artifactId: request.artifactId,
          error: error instanceof Error ? error.message : String(error),
        });
        return {
          ok: false,
          code: "runtime_unavailable",
          message: "暂时无法读取任务记录，请稍后重试。",
        };
      }
    },
  );

  ipcMain.handle(
    "awaken:reveal-path",
    async (event, target: string): Promise<{ ok: boolean; message?: string; }> => {
      if (!dependencies.mainWindowRef || event.sender.id !== dependencies.mainWindowRef.webContents.id) {
        return { ok: false, message: "无法确认定位请求来自 Awaken 主窗口。" };
      }
      const folderPath = typeof target === "string" ? target.trim() : "";
      if (!folderPath || !path.isAbsolute(folderPath)) {
        return { ok: false, message: "需要一个绝对路径。" };
      }
      try {
        await fs.access(folderPath);
      } catch {
        return { ok: false, message: `目录已不存在：${folderPath}` };
      }
      shell.showItemInFolder(folderPath);
      return { ok: true };
    },
  );
}
