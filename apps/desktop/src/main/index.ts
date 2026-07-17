import { randomUUID } from "node:crypto";
import { spawn } from "node:child_process";
import { TextDecoder } from "node:util";
import {
  app,
  BrowserWindow,
  dialog,
  ipcMain,
  nativeTheme,
  nativeImage,
  net,
  protocol,
  screen as electronScreen,
  shell,
  webContents,
  type IpcMainEvent,
  type Rectangle,
  type WebContents,
} from "electron";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import fs from "node:fs/promises";
import path from "node:path";
import type { AguiEvent } from "../shared/agui.js";
import {
  normalizeModelProfileBalancesResult,
  normalizeModelProvider,
  normalizeModelProfileRunConfig,
  normalizeModelProfileSummary,
  normalizeRuntimeProject,
  normalizeRuntimeProjectOperation,
  normalizeRuntimeInfo,
  normalizeRuntimeRun,
  normalizeRuntimeThreadTranscript,
  normalizeRuntimeThread,
  normalizeStartRunRequest,
  type AttachmentPickResult,
  type ArtifactOpenRequest,
  type ArtifactOpenResult,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type CliActivation,
  type CliConnectorReadiness,
  type CliConnectorView,
  type CliProviderId,
  type DesktopPreferencesSaveRequest,
  type DesktopPlatform,
  type DeleteSkillRequest,
  type DeleteSkillResponse,
  type GetSkillPreviewRequest,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type ModelProfileBalancesResultWire,
  type ModelProfileSaveRequest,
  type ModelProviderWire,
  type PickedAttachmentFile,
  type ProjectMutationResult,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RuntimeInfoWire,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeRunWire,
  type RuntimeProject,
  type RuntimeProjectOperation,
  type RuntimeProjectOperationWire,
  type RuntimeProjectWire,
  type RuntimeSkill,
  type SkillPreviewResponse,
  type RuntimeModelProfileSummaryWire,
  type RenameThreadRequest,
  type RuntimeThreadTranscriptWire,
  type RuntimeThreadWire,
  type RuntimeToolInfo,
  type SearchSettings,
  type SearchSettingsSaveRequest,
  type SearchSettingsSaveResult,
  type SearchProviderTestRequest,
  type SearchProviderTestResult,
  type StartRunRequest,
  type ThreadDeleteResult,
  type UpdateMcpServerConfigRequest,
  type UpsertProjectResult,
} from "../shared/runtime.js";
import {
  updateMainWindowBackground,
  windowBackgroundColor,
} from "./window-background.js";
import { RuntimeSupervisor, type SupervisorStatusInfo } from "./runtime-supervisor.js";
import { ensureConfigDir } from "./config-bootstrap.js";
import { DesktopPreferencesStore } from "./desktop-preferences-store.js";
import { ModelProfileStore } from "./model-profile-store.js";
import { SearchSettingsStore } from "./search-settings-store.js";
import { runDesktopOperation } from "./desktop-operation.js";
import { PetStore } from "./pets/pet-store.js";
import { detectCodexHome, importCodexPets } from "./pets/codex-pet-importer.js";
import { importBuiltinPets, resolveBuiltinPetsRoot } from "./pets/builtin-pet-importer.js";
import { PetWindowManager } from "./pets/pet-window-manager.js";
import { PET_IPC_CHANNELS } from "../shared/pets.js";
import { openRecordedArtifact, revealRecordedArtifact } from "./artifact-open.js";
import { executeProjectTrashOperation as executeClaimedProjectTrashOperation } from "./project-trash-worker.js";
import {
  desktopInstanceIdentity,
} from "./desktop-instance.js";
import { cleanupLegacyPrimaryRuntime } from "./legacy-runtime-cleanup.js";
import {
  resolveSystemSkillsSourceRoot,
  resolveSystemSkillsTargetRoot,
  syncSystemSkills,
  SystemSkillsSyncError,
} from "./system-skills-importer.js";
import { CliConnectorManager } from "./cli-connectors/manager.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
let runtimeBaseUrl: string | null = null;
const isDevServer = process.argv.includes("--dev");
const instanceIdentity = desktopInstanceIdentity(isDevServer);
app.setName(instanceIdentity.applicationName);
app.setPath(
  "userData",
  join(app.getPath("appData"), ...instanceIdentity.userDataPathSegments),
);
if (process.platform === "win32") {
  app.setAppUserModelId(instanceIdentity.applicationId);
}

const ownsSingleInstanceLock = app.requestSingleInstanceLock(
  instanceIdentity,
);
if (!ownsSingleInstanceLock) {
  app.quit();
} else {
  app.on("second-instance", (_event, _argv, _workingDirectory, identity) => {
    console.info("desktop_second_instance_redirected", { identity });
    if (app.isReady()) {
      focusOrCreateMainWindow();
    } else {
      app.once("ready", () => focusOrCreateMainWindow());
    }
  });
}
protocol.registerSchemesAsPrivileged([
  {
    scheme: "awaken-pet",
    privileges: {
      standard: true,
      secure: true,
      supportFetchAPI: true,
      corsEnabled: true,
    },
  },
  {
    scheme: "awaken-cli-auth",
    privileges: {
      standard: true,
      secure: true,
      supportFetchAPI: true,
      corsEnabled: false,
    },
  },
]);

let supervisor: RuntimeSupervisor | null = null;
let desktopPreferencesStore: DesktopPreferencesStore | null = null;
let modelProfileStore: ModelProfileStore | null = null;
let searchSettingsStore: SearchSettingsStore | null = null;
let petStore: PetStore | null = null;
let petWindowManager: PetWindowManager | null = null;
let cliConnectorManager: CliConnectorManager | null = null;
let mainWindowRef: BrowserWindow | null = null;
let pendingPetThreadId: string | null = null;
let isQuitting = false;
let projectTrashWorkerTimer: NodeJS.Timeout | null = null;
let projectTrashWorkerRunning = false;

type StreamSubscription = {
  controller: AbortController;
  webContentsId: number;
};

const subscriptions = new Map<string, StreamSubscription>();
const desktopControlToken = randomUUID();

const defaultWindowWidth = 1440;
const defaultWindowHeight = 960;
const minimumWindowWidth = 1120;
const minimumWindowHeight = 760;

function desktopIconPath(): string {
  if (app.isPackaged) {
    return join(process.resourcesPath, "icon.png");
  }
  return join(__dirname, "../../build/icon.png");
}

function cliConnectorManifestPath(): string {
  return app.isPackaged
    ? join(process.resourcesPath, "cli-connectors", "manifest.json")
    : join(app.getAppPath(), "resources", "cli-connectors", "manifest.json");
}

function broadcastCliConnectorState(connector: CliConnectorView): void {
  for (const wc of webContents.getAllWebContents()) {
    if (!wc.isDestroyed()) wc.send("awaken:cli-connector-state", { connector });
  }
}

async function publishCliReadiness(readiness: CliConnectorReadiness): Promise<void> {
  if (!runtimeBaseUrl) return;
  await requestRuntime(`/v1/connectors/${encodeURIComponent(readiness.providerId)}/cli-readiness`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      "X-Awaken-Control-Token": desktopControlToken,
    },
    body: JSON.stringify(readiness),
  });
}

function registerCliAuthProtocol(): void {
  protocol.handle("awaken-cli-auth", async (request) => {
    try {
      const url = new URL(request.url);
      const parts = url.pathname.split("/").filter(Boolean);
      if (parts.length !== 2 || parts[1] !== "qr.png") return new Response("Not found", { status: 404 });
      const assetPath = cliConnectorManager?.resolveAuthAsset(url.hostname, parts[0]);
      if (!assetPath) return new Response("Not found", { status: 404 });
      const bytes = await fs.readFile(assetPath);
      return new Response(bytes, {
        status: 200,
        headers: { "Content-Type": "image/png", "Cache-Control": "no-store" },
      });
    } catch {
      return new Response("Not found", { status: 404 });
    }
  });
}

function loadDesktopIcon() {
  const icon = nativeImage.createFromPath(desktopIconPath());
  return icon.isEmpty() ? undefined : icon;
}

const maxPickedImageBytes = 8 * 1024 * 1024;

const supportedAttachmentExtensions = [
  "png",
  "jpg",
  "jpeg",
  "webp",
  "pdf",
  "xls",
  "xlsx",
  "csv",
  "tsv",
  "md",
  "markdown",
  "doc",
  "docx",
  "txt",
  "json",
  "jsonl",
  "js",
  "jsx",
  "ts",
  "tsx",
  "css",
  "html",
  "xml",
  "yaml",
  "yml",
  "go",
  "py",
  "rb",
  "rs",
  "java",
  "c",
  "h",
  "cpp",
  "hpp",
  "swift",
  "kt",
  "sql",
  "sh",
];

const attachmentMimeTypesByExtension = new Map<string, string>([
  [".png", "image/png"],
  [".jpg", "image/jpeg"],
  [".jpeg", "image/jpeg"],
  [".webp", "image/webp"],
  [".pdf", "application/pdf"],
  [".csv", "text/csv"],
  [".tsv", "text/tab-separated-values"],
  [".md", "text/markdown"],
  [".markdown", "text/markdown"],
  [".txt", "text/plain"],
  [".json", "application/json"],
  [".jsonl", "application/jsonl"],
  [".doc", "application/msword"],
  [".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"],
  [".xls", "application/vnd.ms-excel"],
  [".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"],
  [".ppt", "application/vnd.ms-powerpoint"],
  [".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"],
  [".js", "text/javascript"],
  [".jsx", "text/javascript"],
  [".ts", "text/typescript"],
  [".tsx", "text/typescript"],
  [".css", "text/css"],
  [".scss", "text/x-scss"],
  [".html", "text/html"],
  [".xml", "application/xml"],
  [".yaml", "application/yaml"],
  [".yml", "application/yaml"],
  [".go", "text/x-go"],
  [".py", "text/x-python"],
  [".rb", "text/x-ruby"],
  [".rs", "text/x-rust"],
  [".java", "text/x-java"],
  [".c", "text/x-c"],
  [".h", "text/x-c"],
  [".cpp", "text/x-c++"],
  [".hpp", "text/x-c++"],
  [".swift", "text/x-swift"],
  [".kt", "text/x-kotlin"],
  [".sql", "application/sql"],
  [".sh", "application/x-sh"],
]);

function mimeTypeForAttachmentPath(filePath: string): string {
  return attachmentMimeTypesByExtension.get(path.extname(filePath).toLowerCase()) ??
    "application/octet-stream";
}

function isInlineImageMimeType(mimeType: string): boolean {
  return mimeType === "image/png" || mimeType === "image/jpeg" || mimeType === "image/webp";
}

async function pickedAttachmentForPath(filePath: string): Promise<PickedAttachmentFile> {
  const stat = await fs.stat(filePath);
  if (!stat.isFile()) {
    throw new Error("不是文件");
  }
  const mimeType = mimeTypeForAttachmentPath(filePath);
  const displayName = path.basename(filePath);
  const byteSize = Number(stat.size);

  if (!isInlineImageMimeType(mimeType)) {
    return {
      id: randomUUID(),
      kind: "file",
      path: filePath,
      mimeType,
      displayName,
      byteSize,
    };
  }

  if (byteSize > maxPickedImageBytes) {
    throw new Error(`图片超过 ${maxPickedImageBytes / (1024 * 1024)} MiB`);
  }

  const data = await fs.readFile(filePath);
  const image = nativeImage.createFromPath(filePath);
  const size = image.isEmpty() ? { width: 0, height: 0 } : image.getSize();
  return {
    id: randomUUID(),
    kind: "image",
    path: filePath,
    mimeType,
    displayName,
    byteSize,
    dataBase64: data.toString("base64"),
    width: size.width,
    height: size.height,
  };
}

function setMacDockIcon(): void {
  if (process.platform !== "darwin") {
    return;
  }
  const icon = loadDesktopIcon();
  if (icon) {
    app.dock?.setIcon(icon);
  }
}

function centeredBounds(width: number, height: number, workArea: Rectangle): Rectangle {
  return {
    width,
    height,
    x: workArea.x + Math.round((workArea.width - width) / 2),
    y: workArea.y + Math.round((workArea.height - height) / 2),
  };
}

function shouldResetWindowBounds(bounds: Rectangle, workArea: Rectangle): boolean {
  const hasUsableSize = bounds.width >= minimumWindowWidth && bounds.height >= minimumWindowHeight;
  const hasVisibleCenter =
    bounds.x + bounds.width > workArea.x + 120 &&
    bounds.x < workArea.x + workArea.width - 120 &&
    bounds.y + bounds.height > workArea.y + 120 &&
    bounds.y < workArea.y + workArea.height - 120;

  return !hasUsableSize || !hasVisibleCenter;
}

function ensureWindowVisible(window: BrowserWindow): void {
  if (window.isDestroyed()) {
    return;
  }

  if (window.isMinimized()) {
    window.restore();
  }

  const currentBounds = window.getBounds();
  const workArea = electronScreen.getDisplayMatching(currentBounds).workArea;
  const nextWidth = Math.min(Math.max(currentBounds.width, minimumWindowWidth), workArea.width);
  const nextHeight = Math.min(Math.max(currentBounds.height, minimumWindowHeight), workArea.height);

  if (shouldResetWindowBounds(currentBounds, workArea)) {
    window.setBounds(centeredBounds(nextWidth, nextHeight, workArea));
  }
}

function toDesktopPlatform(platform: NodeJS.Platform): DesktopPlatform {
  switch (platform) {
    case "darwin":
      return "macos";
    case "win32":
      return "windows";
    default:
      return "linux";
  }
}

function runtimeUrl(pathname: string): string {
  if (!runtimeBaseUrl) {
    throw new Error("Runtime unavailable");
  }
  return new URL(pathname, runtimeBaseUrl.endsWith("/") ? runtimeBaseUrl : `${runtimeBaseUrl}/`).toString();
}

function toRendererSupervisorStatus(info: SupervisorStatusInfo): { status: SupervisorStatusInfo["status"] } {
  return { status: info.status };
}

function hostFromBaseURL(baseURL: string): string {
  try {
    return new URL(baseURL).host;
  } catch {
    return "";
  }
}

async function readErrorMessage(response: Response): Promise<string> {
  const fallback = `Runtime request failed (${response.status})`;
  const body = (await response.text()).trim();
  if (!body) return fallback;
  try {
    const parsed = JSON.parse(body);
    const code = parsed.code ?? parsed.error;
    if (code && parsed.message) {
      return `${code}: ${parsed.message}`;
    }
  } catch {
    // fall through
  }
  return body;
}

async function requestRuntime<T = RuntimeRunWire>(pathname: string, init?: RequestInit): Promise<T> {
  const response = await fetch(runtimeUrl(pathname), init);
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  return (await response.json()) as T;
}

async function requestRuntimeOK(pathname: string, init?: RequestInit): Promise<void> {
  const response = await fetch(runtimeUrl(pathname), init);
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
}

class RuntimeDomainError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "RuntimeDomainError";
  }
}

async function requestRuntimeDomain<T>(pathname: string, init?: RequestInit): Promise<T> {
  const response = await fetch(runtimeUrl(pathname), init);
  if (!response.ok) {
    const fallback = `Runtime request failed (${response.status})`;
    const raw = (await response.text()).trim();
    let code = "INTERNAL_ERROR";
    let message = raw || fallback;
    if (raw) {
      try {
        const parsed = JSON.parse(raw) as { code?: string; error?: string; message?: string };
        code = parsed.code ?? parsed.error ?? code;
        message = parsed.message ?? message;
      } catch {
        // Keep the raw body for internal logs. Renderer-facing callers map it.
      }
    }
    throw new RuntimeDomainError(code, message, response.status);
  }
  return (await response.json()) as T;
}

function upsertProjectFailureMessage(error: unknown): string {
  if (!(error instanceof RuntimeDomainError)) {
    return error instanceof Error ? error.message : "打开项目失败，请稍后重试。";
  }
  const messages: Record<string, string> = {
    PROJECT_NOT_FOUND: "项目不存在或已经被移除。",
    PROJECT_OPERATION_CONFLICT:
      "该文件夹正在处理其他操作（例如移除），请稍后再试。",
    PROJECT_RUNS_BUSY: "项目中有正在运行的任务，请等待任务结束后再打开。",
    PROJECT_PATH_INVALID: "该文件夹不能作为 Awaken 项目。",
    PROJECT_PATH_FINGERPRINT_CHANGED:
      "文件夹内容已发生变化，Awaken 已停止本次操作以避免误操作。",
  };
  return messages[error.code] ?? "打开项目失败，请稍后重试。";
}

function projectMutationFailure(error: unknown): ProjectMutationResult {
  const code = error instanceof RuntimeDomainError ? error.code : "INTERNAL_ERROR";
  const messages: Record<string, string> = {
    PROJECT_NOT_FOUND: "项目不存在或已经被移除。",
    PROJECT_OPERATION_NOT_FOUND: "删除任务不存在或已经结束。",
    PROJECT_OPERATION_CONFLICT: "项目正在处理另一个操作，请稍后重试。",
    PROJECT_RUNS_BUSY: "正在等待项目中的任务停止，Awaken 会自动继续。",
    IDEMPOTENCY_KEY_REQUIRED: "删除请求缺少幂等标识，请重试。",
    IDEMPOTENCY_KEY_CONFLICT: "该删除请求已用于其他操作，请重试。",
    PROJECT_PATH_INVALID: "该文件夹不能被移到废纸篓。",
    PROJECT_PATH_FINGERPRINT_CHANGED:
      "文件夹身份发生变化，为避免误删新目录，Awaken 已停止本次删除。",
    PROJECT_TRASH_ATTEMPT_INVALID: "删除任务已由新的执行器接管，Awaken 会自动继续。",
  };
  return {
    ok: false,
    code,
    message: messages[code] ?? "项目操作暂时失败，请稍后重试。",
  };
}

async function failProjectTrashOperation(
  operationId: string,
  attemptId: string,
  code: string,
  message: string,
): Promise<void> {
  await requestRuntimeDomain<RuntimeProjectOperationWire>(
    `/v1/project-operations/${encodeURIComponent(operationId)}/fail-trash`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ attempt_id: attemptId, code, message }),
    },
  );
}

async function executeProjectTrashOperation(
  operation: RuntimeProjectOperationWire,
): Promise<void> {
  await executeClaimedProjectTrashOperation(operation, {
    claim: async (operationId) => {
      try {
        return await requestRuntimeDomain<RuntimeProjectOperationWire>(
          `/v1/project-operations/${encodeURIComponent(operationId)}/claim-trash`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ worker_id: "desktop-main", lease_seconds: 30 }),
          },
        );
      } catch (error) {
        if (
          error instanceof RuntimeDomainError &&
          (error.code === "PROJECT_RUNS_BUSY" ||
            error.code === "PROJECT_OPERATION_CONFLICT")
        ) {
          return null;
        }
        throw error;
      }
    },
    pathExists: async (targetPath) => {
      try {
        await fs.lstat(targetPath);
        return true;
      } catch (error) {
        if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
          return false;
        }
        throw error;
      }
    },
    stage: async (operationId, attemptId) => {
      return requestRuntimeDomain<RuntimeProjectOperationWire>(
        `/v1/project-operations/${encodeURIComponent(operationId)}/stage-trash`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ attempt_id: attemptId }),
        },
      );
    },
    beginInvocation: async (operationId, attemptId) => {
      await requestRuntimeDomain<RuntimeProjectOperationWire>(
        `/v1/project-operations/${encodeURIComponent(operationId)}/begin-trash`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ attempt_id: attemptId }),
        },
      );
    },
    trash: (workspaceRoot) => shell.trashItem(workspaceRoot),
    complete: async (operationId, attemptId) => {
      await requestRuntimeDomain<RuntimeProjectOperationWire>(
        `/v1/project-operations/${encodeURIComponent(operationId)}/complete-trash`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ attempt_id: attemptId }),
        },
      );
    },
    fail: failProjectTrashOperation,
    reportTrashFailure: ({ operationId, workspaceRoot, error }) => {
      console.error("project_trash_failed", { operationId, workspaceRoot, error });
    },
  });
}

async function processPendingProjectTrashOperations(): Promise<void> {
  if (projectTrashWorkerRunning || !runtimeBaseUrl) {
    return;
  }
  projectTrashWorkerRunning = true;
  try {
    const operations = await requestRuntimeDomain<RuntimeProjectOperationWire[]>(
      "/v1/project-operations",
      { method: "GET" },
    );
    for (const operation of operations) {
      if (
        operation.phase !== "waiting_for_runs" &&
        operation.phase !== "trash_claimed" &&
        operation.phase !== "trash_staged" &&
        operation.phase !== "trashing"
      ) {
        continue;
      }
      await executeProjectTrashOperation(operation);
    }
  } catch (error) {
    console.error("project_trash_worker_failed", {
      error: error instanceof Error ? error.message : String(error),
    });
  } finally {
    projectTrashWorkerRunning = false;
  }
}

function startProjectTrashWorker(): void {
  if (projectTrashWorkerTimer) {
    return;
  }
  void processPendingProjectTrashOperations();
  projectTrashWorkerTimer = setInterval(() => {
    void processPendingProjectTrashOperations();
  }, 750);
}

function stopProjectTrashWorker(): void {
  if (projectTrashWorkerTimer) {
    clearInterval(projectTrashWorkerTimer);
    projectTrashWorkerTimer = null;
  }
}

function sendToRenderer(channel: string, payload: unknown, targetId: number): void {
  const target = webContents.fromId(targetId);
  if (!target || target.isDestroyed()) {
    return;
  }
  target.send(channel, payload);
}

function consumeSSEFrame(frame: string, onEvent: (event: AguiEvent) => void): void {
  const dataLines = frame
    .split("\n")
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trimStart());

  if (dataLines.length === 0) {
    return;
  }

  const payload = dataLines.join("\n");
  onEvent(JSON.parse(payload) as AguiEvent);
}

async function pumpRunEvents(
  body: ReadableStream<Uint8Array>,
  subscriptionId: string,
  targetId: number,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }

      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split("\n\n");
      buffer = frames.pop() ?? "";

      for (const frame of frames) {
        if (!frame.trim()) {
          continue;
        }
        consumeSSEFrame(frame, (event) => {
          sendToRenderer(
            "awaken:run-event",
            {
              subscriptionId,
              event,
            },
            targetId,
          );
        });
      }
    }

    if (buffer.trim()) {
      consumeSSEFrame(buffer, (event) => {
        sendToRenderer(
          "awaken:run-event",
          {
            subscriptionId,
            event,
          },
          targetId,
        );
      });
    }
  } finally {
    reader.releaseLock();
  }

  // Normal completion: cleanup and notify renderer. On error, this path is
  // skipped (the rejection propagates to the .catch() handler in
  // subscribeToRun, which sends run-error BEFORE run-closed so the renderer
  // receives the real error message before the subscription is torn down).
  subscriptions.delete(subscriptionId);
  sendToRenderer("awaken:run-closed", { subscriptionId }, targetId);
}

async function subscribeToRun(
  target: WebContents,
  runId: string,
  subscriptionId: string,
  afterCursor?: number,
): Promise<void> {
  const controller = new AbortController();
  const eventsUrl = afterCursor
    ? `/v1/runs/${runId}/events?after_cursor=${afterCursor}`
    : `/v1/runs/${runId}/events`;
  const response = await fetch(runtimeUrl(eventsUrl), {
    headers: {
      Accept: "text/event-stream",
    },
    signal: controller.signal,
  });

  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
  if (!response.body) {
    throw new Error("Runtime stream body is missing");
  }

  subscriptions.set(subscriptionId, {
    controller,
    webContentsId: target.id,
  });

  void pumpRunEvents(response.body, subscriptionId, target.id).catch((error: unknown) => {
    if (controller.signal.aborted) {
      return;
    }

    // Send run-error BEFORE run-closed so the preload layer can still look up
    // the subscription handlers and deliver the real error message. The
    // run-closed handler in preload deletes the subscription entry, so sending
    // run-error after run-closed would be a no-op.
    sendToRenderer(
      "awaken:run-error",
      {
        subscriptionId,
        message: error instanceof Error ? error.message : "Runtime stream failed",
      },
      target.id,
    );
    sendToRenderer("awaken:run-closed", { subscriptionId }, target.id);
    subscriptions.delete(subscriptionId);
  });
}

function cleanupWindowSubscriptions(targetId: number): void {
  for (const [subscriptionId, subscription] of subscriptions.entries()) {
    if (subscription.webContentsId === targetId) {
      subscription.controller.abort();
      subscriptions.delete(subscriptionId);
    }
  }
}

function createMainWindow(): BrowserWindow {
  if (mainWindowRef && !mainWindowRef.isDestroyed()) {
    return mainWindowRef;
  }
  const icon = loadDesktopIcon();
  const mainWindow = new BrowserWindow({
    width: defaultWindowWidth,
    height: defaultWindowHeight,
    minWidth: minimumWindowWidth,
    minHeight: minimumWindowHeight,
    backgroundColor: windowBackgroundColor(
      desktopPreferencesStore?.snapshot().theme,
      nativeTheme.shouldUseDarkColors,
    ),
    autoHideMenuBar: true,
    show: false,
    title: "Awaken Desktop",
    ...(icon ? { icon } : {}),
    ...(process.platform === "darwin"
      ? {
          titleBarStyle: "hiddenInset" as const,
          trafficLightPosition: { x: 18, y: 18 },
        }
      : {}),
    webPreferences: {
      preload: join(__dirname, "../preload/index.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });
  mainWindowRef = mainWindow;

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url);
    return { action: "deny" };
  });

  const webContentsId = mainWindow.webContents.id;
  let electronReadyToShow = false;
  let rendererReadyToShow = false;
  let didShowWindow = false;
  let fallbackShowTimer: ReturnType<typeof setTimeout> | undefined;

  function showMainWindow(): void {
    if (didShowWindow || !electronReadyToShow || mainWindow.isDestroyed()) {
      return;
    }
    didShowWindow = true;
    if (fallbackShowTimer) {
      clearTimeout(fallbackShowTimer);
      fallbackShowTimer = undefined;
    }
    ensureWindowVisible(mainWindow);
    mainWindow.show();
    mainWindow.focus();
  }

  function handleRendererReady(event: IpcMainEvent): void {
    if (event.sender.id !== webContentsId) {
      return;
    }
    rendererReadyToShow = true;
    showMainWindow();
    if (pendingPetThreadId) {
      event.sender.send(
        PET_IPC_CHANNELS.petThreadActivationRequested,
        pendingPetThreadId,
      );
      pendingPetThreadId = null;
    }
  }

  ipcMain.on("awaken:renderer-ready", handleRendererReady);

  mainWindow.on("closed", () => {
    if (mainWindowRef === mainWindow) {
      mainWindowRef = null;
    }
    ipcMain.removeListener("awaken:renderer-ready", handleRendererReady);
    if (fallbackShowTimer) {
      clearTimeout(fallbackShowTimer);
      fallbackShowTimer = undefined;
    }
    cleanupWindowSubscriptions(webContentsId);
  });

  mainWindow.on("close", () => {
    if (process.platform !== "darwin" && !isQuitting) {
      app.quit();
    }
  });

  mainWindow.once("ready-to-show", () => {
    electronReadyToShow = true;
    if (rendererReadyToShow) {
      showMainWindow();
    }
  });

  mainWindow.on("show", () => {
    ensureWindowVisible(mainWindow);
  });

  mainWindow.on("restore", () => {
    ensureWindowVisible(mainWindow);
  });

  mainWindow.on("focus", () => {
    petWindowManager?.notifyMainWindowFocused();
  });

  const loadWindow = isDevServer
    ? mainWindow.loadURL("http://127.0.0.1:5174")
    : mainWindow.loadFile(join(__dirname, "../renderer/index.html"));

  void loadWindow.finally(() => {
    fallbackShowTimer = setTimeout(() => {
      rendererReadyToShow = true;
      showMainWindow();
    }, 2500);
  });

  return mainWindow;
}

function focusOrCreateMainWindow(): BrowserWindow {
  if (!mainWindowRef || mainWindowRef.isDestroyed()) {
    return createMainWindow();
  }
  const mainWindow = mainWindowRef;
  if (!mainWindow.isDestroyed()) {
    if (mainWindow.isMinimized()) {
      mainWindow.restore();
    }
    ensureWindowVisible(mainWindow);
    mainWindow.show();
    mainWindow.focus();
    mainWindow.moveTop();
  }
  return mainWindow;
}

function activatePetThread(threadId: string): void {
  pendingPetThreadId = threadId;
  const mainWindow = focusOrCreateMainWindow();
  if (!mainWindow.webContents.isLoadingMainFrame()) {
    mainWindow.webContents.send(
      PET_IPC_CHANNELS.petThreadActivationRequested,
      threadId,
    );
    pendingPetThreadId = null;
  }
}

function broadcastPetCatalogChanged(): void {
  for (const wc of webContents.getAllWebContents()) {
    if (!wc.isDestroyed()) {
      wc.send(PET_IPC_CHANNELS.petCatalogChanged);
    }
  }
}

function registerPetIpcHandlers(): void {
  ipcMain.handle(PET_IPC_CHANNELS.listPets, async () => {
    if (!petStore) throw new Error("Pet store unavailable");
    await petStore.refreshCatalog();
    return petStore.listPets();
  });

  ipcMain.handle(PET_IPC_CHANNELS.getPetSettings, async () => {
    if (!petStore) throw new Error("Pet store unavailable");
    return petStore.getSettings();
  });

  ipcMain.handle(PET_IPC_CHANNELS.selectPet, async (_event, petId: string) => {
    if (!petStore) throw new Error("Pet store unavailable");
    const settings = await petStore.selectPet(petId);
    if (settings.overlayOpen && petWindowManager) {
      await petWindowManager.show(petId);
    }
    return settings;
  });

  ipcMain.handle(PET_IPC_CHANNELS.refreshCodexPets, async () => {
    if (!petStore) throw new Error("Pet store unavailable");
    const codexHome = detectCodexHome(app.getPath("home"));
    const report = await importCodexPets(petStore, codexHome);
    broadcastPetCatalogChanged();
    const selectedPetId = petStore.getSettings().selectedPetId;
    if (selectedPetId && petWindowManager?.isOpen()) {
      await petWindowManager.setPet(selectedPetId);
    }
    return report;
  });

  ipcMain.handle(PET_IPC_CHANNELS.createPet, async () => {
    if (!petStore) throw new Error("Pet store unavailable");
    const now = new Date().toISOString();
    const result = await petStore.createLocalPet({
      sourceId: `my-pet-${Date.now()}`,
      displayName: "My Pet",
      description: `Awaken 本地宠物模板，创建于 ${now}。请替换 spritesheet.png 后刷新。`,
    });
    broadcastPetCatalogChanged();
    return { threadId: result.threadId };
  });

  ipcMain.handle(PET_IPC_CHANNELS.setPetOverlayOpen, async (_event, open: boolean) => {
    if (!petStore) throw new Error("Pet store unavailable");
    const settings = await petStore.setOverlayOpen(open);
    if (open) {
      await petWindowManager?.show();
    } else {
      petWindowManager?.hide();
    }
    return settings;
  });

  ipcMain.handle(
    PET_IPC_CHANNELS.setPetImportPreferences,
    async (_event, input: { importCodexPets?: boolean }) => {
      if (!petStore) throw new Error("Pet store unavailable");
      return petStore.setImportPreferences(input);
    },
  );

  ipcMain.handle(PET_IPC_CHANNELS.openAwakenPetsDirectory, async () => {
    if (!petStore) throw new Error("Pet store unavailable");
    const dir = path.join(app.getPath("userData"), "data", "pets");
    await fs.mkdir(dir, { recursive: true });
    const errorMessage = await shell.openPath(dir);
    if (errorMessage) {
      throw new Error(errorMessage);
    }
  });

  ipcMain.handle(PET_IPC_CHANNELS.openCodexPetsDirectory, async () => {
    const codexHome = detectCodexHome(app.getPath("home"));
    const dir = path.join(codexHome, "pets");
    try {
      await fs.access(dir);
    } catch {
      throw new Error("未检测到 Codex 自定义宠物目录");
    }
    const errorMessage = await shell.openPath(dir);
    if (errorMessage) {
      throw new Error(errorMessage);
    }
  });

  ipcMain.on(PET_IPC_CHANNELS.updatePetActivity, (_event, snapshot) => {
    petWindowManager?.updateActivity(snapshot);
  });
}

ipcMain.handle("awaken:get-desktop-preferences", async () => {
  if (!desktopPreferencesStore) {
    throw new Error("Desktop preferences store unavailable");
  }
  return desktopPreferencesStore.snapshot();
});

ipcMain.handle(
  "awaken:save-desktop-preferences",
  async (_event, request: DesktopPreferencesSaveRequest) => {
    if (!desktopPreferencesStore) {
      throw new Error("Desktop preferences store unavailable");
    }
    const preferences = await desktopPreferencesStore.savePreferences(request);
    updateMainWindowBackground(
      mainWindowRef,
      preferences.theme,
      nativeTheme.shouldUseDarkColors,
    );
    return preferences;
  },
);

ipcMain.handle("awaken:get-runtime-info", async () => {
  try {
    const info = await requestRuntime<RuntimeInfoWire>("/v1/runtime/info", {
      method: "GET",
    });
    const defaultProfile = await modelProfileStore?.defaultProfileSummary();
    if (defaultProfile) {
      info.configured_model = defaultProfile.model_id;
      info.protocol = defaultProfile.protocol;
      info.has_api_key = defaultProfile.has_key;
      info.base_url_host = hostFromBaseURL(defaultProfile.base_url);
      info.model_profile = defaultProfile;
    } else if (modelProfileStore) {
      info.configured_model = "";
      info.has_api_key = false;
      info.base_url_host = "";
      info.model_profile = undefined;
    }

    return normalizeRuntimeInfo(info);
  } catch {
    console.error("[awaken-desktop] failed to get runtime info");
    throw new Error("Runtime unavailable");
	}
});

ipcMain.handle("awaken:list-connectors", async () => {
	const connectors = await requestRuntime<RuntimeConnector[]>("/v1/connectors", {
		method: "GET",
	});
	const cliById = new Map((cliConnectorManager?.list() ?? []).map((entry) => [entry.providerId, entry]));
	return connectors.map((connector) => {
		const cli = cliById.get(connector.id as CliProviderId);
		return cli ? { ...connector, kind: "cli" as const, cliReadiness: cli.readiness } : connector;
	});
});

ipcMain.handle("awaken:connect-connector", async (_event, connectorId: string) => {
	return requestRuntime<RuntimeConnectorActionResponse>(`/v1/connectors/${connectorId}/connect`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({}),
	});
});

ipcMain.handle("awaken:disconnect-connector", async (_event, connectorId: string) => {
	return requestRuntime<RuntimeConnectorActionResponse>(`/v1/connectors/${connectorId}/disconnect`, {
		method: "POST",
	});
});

ipcMain.handle("awaken:list-cli-connectors", async () => cliConnectorManager?.list() ?? []);
ipcMain.handle("awaken:refresh-cli-connector", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.refreshProvider(providerId);
});
ipcMain.handle("awaken:install-cli-connector", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.install(providerId);
});
ipcMain.handle("awaken:cancel-cli-install", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.cancelInstall(providerId);
});
ipcMain.handle("awaken:select-cli-executable", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	const result = await dialog.showOpenDialog({
		title: providerId === "dingtalk-workspace" ? "选择 dws 可执行文件" : "选择 lark-cli 可执行文件",
		properties: ["openFile"],
	});
	if (result.canceled || result.filePaths.length !== 1) {
		const current = cliConnectorManager.list().find((entry) => entry.providerId === providerId);
		if (!current) throw new Error("CLI connector unavailable");
		return current;
	}
	return cliConnectorManager.selectExecutable(providerId, result.filePaths[0]);
});
ipcMain.handle("awaken:configure-cli-connector", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.configure(providerId);
});
ipcMain.handle("awaken:start-cli-auth", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.startAuth(providerId);
});
ipcMain.handle("awaken:cancel-cli-auth", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.cancelAuth(providerId);
});
ipcMain.handle("awaken:retry-cli-auth", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.retryAuth(providerId);
});
ipcMain.handle("awaken:set-cli-activation", async (_event, providerId: CliProviderId, activation: CliActivation) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.setActivation(providerId, activation);
});
ipcMain.handle("awaken:logout-cli-connector", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.logout(providerId);
});
ipcMain.handle("awaken:uninstall-managed-cli", async (_event, providerId: CliProviderId) => {
	if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
	return cliConnectorManager.uninstallManaged(providerId);
});

ipcMain.handle("awaken:list-mcp-servers", async () => {
	return requestRuntime<RuntimeMcpServer[]>("/v1/mcp/servers", {
		method: "GET",
	});
});

ipcMain.handle("awaken:configure-mcp-servers", async (_event, request: ConfigureMcpServersRequest) => {
	return requestRuntime<ConfigureMcpServersResponse>("/v1/mcp/servers", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(request),
	});
});

ipcMain.handle("awaken:get-mcp-server-config", async (_event, name: string) => {
	return requestRuntime<RuntimeMcpServerConfigResponse>(`/v1/mcp/servers/${encodeURIComponent(name)}/config`, {
		method: "GET",
	});
});

ipcMain.handle("awaken:update-mcp-server-config", async (_event, name: string, request: UpdateMcpServerConfigRequest) => {
	return requestRuntime<ConfigureMcpServersResponse>(`/v1/mcp/servers/${encodeURIComponent(name)}/config`, {
		method: "PATCH",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(request),
	});
});

ipcMain.handle("awaken:get-runtime-logs", async () => {
	return supervisor?.getLogs() ?? [] as RuntimeLogEntry[];
});

ipcMain.handle("awaken:list-mcp-tools", async () => {
	return requestRuntime<RuntimeToolInfo[]>("/v1/mcp/tools", {
		method: "GET",
	});
});

ipcMain.handle("awaken:disable-mcp-server", async (_event, name: string) => {
	return requestRuntimeOK(`/v1/mcp/servers/${encodeURIComponent(name)}/disable`, {
		method: "POST",
	});
});

ipcMain.handle("awaken:enable-mcp-server", async (_event, name: string) => {
	return requestRuntimeOK(`/v1/mcp/servers/${encodeURIComponent(name)}/enable`, {
		method: "POST",
	});
});

ipcMain.handle("awaken:delete-mcp-server", async (_event, name: string) => {
	return requestRuntimeOK(`/v1/mcp/servers/${encodeURIComponent(name)}`, {
		method: "DELETE",
	});
});

ipcMain.handle("awaken:list-skills", async (_event, projectRoot?: string) => {
	console.log("[main:list-skills] projectRoot:", projectRoot);
	const path = projectRoot
		? `/v1/skills?projectRoot=${encodeURIComponent(projectRoot)}`
		: "/v1/skills";
	const result = await requestRuntime<RuntimeSkill[]>(path, {
		method: "GET",
	});
	console.log("[main:list-skills] path:", path, "agents count:", result.filter((s) => s.sourceOrigin === "agents").length);
	return result;
});

ipcMain.handle("awaken:get-skill-preview", async (_event, request: GetSkillPreviewRequest) => {
	return requestRuntime<SkillPreviewResponse>("/v1/skills/preview", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(request),
	});
});

ipcMain.handle("awaken:install-skill", async (_event, request: InstallSkillRequest) => {
	return requestRuntime<InstallSkillResponse>("/v1/skills/install", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(request),
	});
});

ipcMain.handle("awaken:delete-skill", async (_event, request: DeleteSkillRequest) => {
	const id = request.id.trim();
	if (!id) {
		throw new Error("Skill id is required");
	}
	return requestRuntime<DeleteSkillResponse>(`/v1/skills/${encodeURIComponent(id)}`, {
		method: "DELETE",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({
			sourceOrigin: request.sourceOrigin,
			projectRoot: request.projectRoot,
		}),
	});
});

ipcMain.handle("awaken:disable-skill", async (_event, id: string) => {
	const trimmed = id.trim();
	if (!trimmed) {
		throw new Error("Skill id is required");
	}
	return requestRuntime<{ id: string; state: string }>(`/v1/skills/${encodeURIComponent(trimmed)}/disable`, {
		method: "POST",
	});
});

ipcMain.handle("awaken:enable-skill", async (_event, id: string) => {
	const trimmed = id.trim();
	if (!trimmed) {
		throw new Error("Skill id is required");
	}
	return requestRuntime<{ id: string; state: string }>(`/v1/skills/${encodeURIComponent(trimmed)}/enable`, {
		method: "POST",
	});
});

ipcMain.handle("awaken:list-model-providers", async () => {
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  const providers = modelProfileStore.listProviders() satisfies ModelProviderWire[];
  return providers.map(normalizeModelProvider);
});

ipcMain.handle("awaken:list-model-profiles", async () => {
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  const profiles = (await modelProfileStore.listProfiles()) satisfies RuntimeModelProfileSummaryWire[];
  return profiles.map(normalizeModelProfileSummary);
});

ipcMain.handle("awaken:save-model-profile", async (_event, request: ModelProfileSaveRequest) => {
  return runDesktopOperation(async () => {
    if (!modelProfileStore) {
      throw new Error("Model profile store unavailable");
    }
    return normalizeModelProfileSummary(await modelProfileStore.saveProfile(request));
  });
});

ipcMain.handle("awaken:delete-model-profile", async (_event, profileId: string) => {
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  await modelProfileStore.deleteProfile(profileId);
});

ipcMain.handle("awaken:set-default-model-profile", async (_event, profileId: string) => {
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  return normalizeModelProfileSummary(await modelProfileStore.setDefaultProfile(profileId));
});

ipcMain.handle("awaken:test-model-profile", async (_event, request: ModelProfileSaveRequest) => {
  return runDesktopOperation(async () => {
    if (!modelProfileStore) {
      throw new Error("Model profile store unavailable");
    }
    return modelProfileStore.testProfile(request);
  });
});

ipcMain.handle("awaken:sync-model-capabilities", async () => {
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  await modelProfileStore.syncCapabilities();
  const providers = modelProfileStore.listProviders() satisfies ModelProviderWire[];
  return providers.map(normalizeModelProvider);
});

ipcMain.handle("awaken:get-model-profile-balances", async () => {
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  const balances = await modelProfileStore.fetchBalances();
  const result: ModelProfileBalancesResultWire = { balances };
  return normalizeModelProfileBalancesResult(result);
});

ipcMain.handle("awaken:get-search-settings", async (): Promise<SearchSettings> => {
  if (!searchSettingsStore) {
    throw new Error("Search settings store unavailable");
  }
  return searchSettingsStore.snapshot();
});

ipcMain.handle(
  "awaken:test-search-provider",
  async (
    _event,
    request: SearchProviderTestRequest,
  ): Promise<SearchProviderTestResult> => {
    if (!searchSettingsStore) {
      throw new Error("Search settings store unavailable");
    }
    return searchSettingsStore.testProvider(request.provider);
  },
);

ipcMain.handle(
  "awaken:save-search-settings",
  async (
    _event,
    request: SearchSettingsSaveRequest,
  ): Promise<SearchSettingsSaveResult> => {
    if (!searchSettingsStore || !supervisor) {
      throw new Error("Search settings are unavailable");
    }
    const threads = await requestRuntime<RuntimeThreadWire[]>("/v1/threads");
    if (threads.some((thread) => Boolean(thread.active_run_id))) {
      throw new Error("当前有任务正在运行，请结束任务后重试。");
    }
    const settings = await searchSettingsStore.save(request);
    try {
      const status = await supervisor.restart();
      if (status.status !== "running" || !status.runtimeBaseUrl) {
        return {
          settings,
          restartStatus: "restart_failed",
          message: "搜索设置已保存，但 Core 重启失败。请稍后重试重启。",
        };
      }
      return { settings, restartStatus: "restarted" };
    } catch {
      return {
        settings,
        restartStatus: "restart_failed",
        message: "搜索设置已保存，但 Core 重启失败。请稍后重试重启。",
      };
    }
  },
);

ipcMain.handle("awaken:relaunch-desktop", () => {
  setImmediate(() => {
    if (isDevServer && process.platform === "darwin") {
      const repositoryRoot = path.resolve(__dirname, "../../../..");
      const helperPath = join(
        repositoryRoot,
        "scripts",
        "relaunch-managed-desktop.mjs",
      );
      const nodeExecutable = process.env.npm_node_execpath || "node";
      const helper = spawn(
        nodeExecutable,
        [helperPath, String(process.pid)],
        {
          cwd: repositoryRoot,
          detached: true,
          stdio: "ignore",
          env: process.env,
        },
      );
      helper.unref();
    } else {
      app.relaunch();
    }
    app.quit();
  });
});

ipcMain.handle("awaken:get-run", async (_event, runId: string) => {
  const run = await requestRuntime(`/v1/runs/${runId}`, {
    method: "GET",
  });

	return normalizeRuntimeRun(run);
});

ipcMain.handle("awaken:get-thread-transcript", async (_event, threadId: string) => {
  const transcript = await requestRuntime<RuntimeThreadTranscriptWire>(
    `/v1/threads/${threadId}/transcript`,
    {
      method: "GET",
    },
  );

  return normalizeRuntimeThreadTranscript(transcript);
});

ipcMain.handle("awaken:list-threads", async () => {
	const threads = await requestRuntime<RuntimeThreadWire[]>("/v1/threads", {
		method: "GET",
	});

	return threads.map(normalizeRuntimeThread);
});

ipcMain.handle("awaken:rename-thread", async (_event, threadId: string, label: string) => {
  const body: RenameThreadRequest = { label };
  const thread = await requestRuntime<RuntimeThreadWire>(`/v1/threads/${threadId}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });

  return normalizeRuntimeThread(thread);
});

ipcMain.handle("awaken:delete-thread", async (_event, threadId: string): Promise<ThreadDeleteResult> => {
  try {
    await requestRuntimeOK(`/v1/threads/${threadId}`, {
      method: "DELETE",
    });
    return { ok: true };
  } catch (error) {
    console.error("thread_delete_failed", {
      threadId,
      error: error instanceof Error ? error.message : String(error),
    });
    return {
      ok: false,
      code: "THREAD_DELETE_FAILED",
      message: "删除请求暂时未完成，Awaken 会保留当前会话，请稍后重试。",
    };
  }
});

ipcMain.handle("awaken:list-projects", async (_event, includeRemoved = false) => {
  const projects = await requestRuntimeDomain<RuntimeProjectWire[]>(
    `/v1/projects?include_removed=${includeRemoved ? "true" : "false"}`,
    { method: "GET" },
  );
  return projects.map(normalizeRuntimeProject);
});

ipcMain.handle(
  "awaken:upsert-project",
  async (
    _event,
    request: { workspaceRoot: string; displayName?: string },
  ): Promise<UpsertProjectResult> => {
    try {
      const project = await requestRuntimeDomain<RuntimeProjectWire>("/v1/projects", {
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
      return { ok: false, code, message: upsertProjectFailureMessage(error) };
    }
  },
);

async function runProjectMutation(
  pathname: string,
  method: "DELETE" | "POST",
  projectId: string,
  idempotencyKey: string,
): Promise<ProjectMutationResult> {
  try {
    const operation = await requestRuntimeDomain<RuntimeProjectOperationWire>(pathname, {
      method,
      headers: { "Idempotency-Key": idempotencyKey },
    });
    return { ok: true, operation: normalizeRuntimeProjectOperation(operation) };
  } catch (error) {
    console.error("project_mutation_failed", {
      projectId,
      pathname,
      code: error instanceof RuntimeDomainError ? error.code : "INTERNAL_ERROR",
      error: error instanceof Error ? error.message : String(error),
    });
    return projectMutationFailure(error);
  }
}

ipcMain.handle(
  "awaken:detach-project",
  async (
    _event,
    request: { projectId: string; idempotencyKey: string },
  ): Promise<ProjectMutationResult> =>
    runProjectMutation(
      `/v1/projects/${encodeURIComponent(request.projectId)}`,
      "DELETE",
      request.projectId,
      request.idempotencyKey,
    ),
);

ipcMain.handle(
  "awaken:delete-project-threads",
  async (
    _event,
    request: { projectId: string; idempotencyKey: string },
  ): Promise<ProjectMutationResult> =>
    runProjectMutation(
      `/v1/projects/${encodeURIComponent(request.projectId)}/threads`,
      "DELETE",
      request.projectId,
      request.idempotencyKey,
    ),
);

ipcMain.handle(
  "awaken:trash-project",
  async (
    _event,
    request: { projectId: string; idempotencyKey: string },
  ): Promise<ProjectMutationResult> => {
    const result = await runProjectMutation(
      `/v1/projects/${encodeURIComponent(request.projectId)}/trash`,
      "POST",
      request.projectId,
      request.idempotencyKey,
    );
    if (result.ok) {
      void processPendingProjectTrashOperations();
    }
    return result;
  },
);

ipcMain.handle(
  "awaken:get-project-operation",
  async (_event, operationId: string): Promise<RuntimeProjectOperation> => {
    const operation = await requestRuntimeDomain<RuntimeProjectOperationWire>(
      `/v1/project-operations/${encodeURIComponent(operationId)}`,
      { method: "GET" },
    );
    return normalizeRuntimeProjectOperation(operation);
  },
);

ipcMain.handle(
  "awaken:open-artifact",
  async (event, request: ArtifactOpenRequest): Promise<ArtifactOpenResult> => {
    if (!mainWindowRef || event.sender.id !== mainWindowRef.webContents.id) {
      return {
        ok: false,
        code: "open_failed",
        message: "无法确认文件打开请求来自 Awaken 主窗口。",
      };
    }
    try {
      const transcript = await requestRuntime<RuntimeThreadTranscriptWire>(
        `/v1/threads/${request.threadId}/transcript`,
        { method: "GET" },
      );
      return openRecordedArtifact(
        normalizeRuntimeThreadTranscript(transcript),
        request,
        (target) => shell.openPath(target),
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
  "awaken:reveal-artifact",
  async (event, request: ArtifactOpenRequest): Promise<ArtifactOpenResult> => {
    if (!mainWindowRef || event.sender.id !== mainWindowRef.webContents.id) {
      return {
        ok: false,
        code: "open_failed",
        message: "无法确认文件定位请求来自 Awaken 主窗口。",
      };
    }
    try {
      const transcript = await requestRuntime<RuntimeThreadTranscriptWire>(
        `/v1/threads/${request.threadId}/transcript`,
        { method: "GET" },
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

ipcMain.handle("awaken:start-run", async (_event, request: StartRunRequest) => {
  return runDesktopOperation(async () => {
    const body = normalizeStartRunRequest(request);
    const defaultProfile = await modelProfileStore?.resolveDefaultRunProfile();
    if (!defaultProfile) {
      throw new Error("请先在设置 > 模型中配置可用模型。");
    }
    body.model_profile = normalizeModelProfileRunConfig(defaultProfile);
    const run = await requestRuntime("/v1/runs", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });

    return normalizeRuntimeRun(run);
  });
});

ipcMain.handle("awaken:cancel-run", async (_event, runId: string) => {
  const run = await requestRuntime(`/v1/runs/${runId}/cancel`, {
    method: "POST",
  });

  return normalizeRuntimeRun(run);
});

ipcMain.handle(
  "awaken:answer-user-question",
  async (_event, payload: { runId: string; questionId: string; answer: string }) => {
    await requestRuntimeOK(`/v1/runs/${payload.runId}/user-questions/${payload.questionId}/answer`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ answer: payload.answer }),
    });
  },
);

ipcMain.handle(
  "awaken:resolve-external-interaction",
  async (
    _event,
    payload: {
      runId: string;
      interactionId: string;
      resolution: "completed" | "cancelled" | "expired" | "failed";
    },
  ) => {
    await requestRuntimeOK(
      `/v1/runs/${payload.runId}/external-interactions/${payload.interactionId}/resolve`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ resolution: payload.resolution }),
      },
    );
  },
);

ipcMain.handle(
  "awaken:subscribe-run",
  async (event, payload: { runId: string; subscriptionId: string; afterCursor?: number; afterSeq?: number }) => {
    await subscribeToRun(
      event.sender,
      payload.runId,
      payload.subscriptionId,
      payload.afterCursor ?? payload.afterSeq,
    );
    return payload.subscriptionId;
  },
);

ipcMain.on("awaken:unsubscribe-run", (_event, subscriptionId: string) => {
  const subscription = subscriptions.get(subscriptionId);
  if (!subscription) {
    return;
  }

  subscription.controller.abort();
  subscriptions.delete(subscriptionId);
});

ipcMain.handle(
  "awaken:pick-attachments",
  async (_event, defaultPath?: string): Promise<AttachmentPickResult> => {
    const result = await dialog.showOpenDialog({
      properties: ["openFile", "multiSelections"],
      defaultPath,
      filters: [
        {
          name: "Supported attachments",
          extensions: supportedAttachmentExtensions,
        },
        { name: "Images", extensions: ["png", "jpg", "jpeg", "webp"] },
        {
          name: "Documents and code",
          extensions: supportedAttachmentExtensions.filter(
            (extension) => !["png", "jpg", "jpeg", "webp"].includes(extension),
          ),
        },
        { name: "All Files", extensions: ["*"] },
      ],
    });
    if (result.canceled || result.filePaths.length === 0) {
      return { canceled: true };
    }

    const files: PickedAttachmentFile[] = [];
    const errors: string[] = [];
    for (const filePath of result.filePaths) {
      try {
        files.push(await pickedAttachmentForPath(filePath));
      } catch (error) {
        const reason = error instanceof Error ? error.message : "无法读取文件";
        errors.push(`${path.basename(filePath)}: ${reason}`);
      }
    }

    if (files.length === 0) {
      return {
        canceled: true,
        error: errors[0] ?? "没有可用附件",
      };
    }
    return {
      canceled: false,
      files,
      errors: errors.length > 0 ? errors : undefined,
    };
  },
);

ipcMain.handle(
  "awaken:pick-directory",
  async (_event, defaultPath?: string) => {
    const result = await dialog.showOpenDialog({
      properties: ["openDirectory"],
      defaultPath,
    });
    if (result.canceled || result.filePaths.length === 0) {
      return { canceled: true };
    }
    const selected = result.filePaths[0];
    try {
      const stat = await fs.stat(selected);
      if (!stat.isDirectory()) {
        return { canceled: true, error: "Selected path is not a directory" };
      }
      return { canceled: false, path: selected };
    } catch {
      return { canceled: true, error: "Cannot access selected directory" };
    }
  },
);

ipcMain.handle(
  "awaken:pick-workspace",
  async (_event, defaultPath?: string) => {
    const result = await dialog.showOpenDialog({
      properties: ["openDirectory"],
      defaultPath,
    });
    if (result.canceled || result.filePaths.length === 0) {
      return { canceled: true };
    }
    const selected = result.filePaths[0];
    try {
      const stat = await fs.stat(selected);
      if (!stat.isDirectory()) {
        return { canceled: true, error: "Selected path is not a directory" };
      }
      return { canceled: false, path: selected };
    } catch {
      return { canceled: true, error: "Cannot access selected directory" };
    }
  },
);

ipcMain.handle("awaken:validate-workspace", async (_event, candidate: string) => {
  const trimmed = typeof candidate === "string" ? candidate.trim() : "";
  if (trimmed === "") {
    return { valid: false, error: "Workspace path is required" };
  }
  if (!path.isAbsolute(trimmed)) {
    return { valid: false, error: "Workspace path must be absolute" };
  }
  try {
    const stat = await fs.stat(trimmed);
    if (!stat.isDirectory()) {
      return { valid: false, error: "Workspace path is not a directory" };
    }
    return { valid: true };
  } catch {
    return { valid: false, error: "Workspace directory does not exist or is not accessible" };
  }
});

ipcMain.handle("awaken:runtime-supervisor-status", async () => {
  return toRendererSupervisorStatus(supervisor?.getStatus() ?? { status: "stopped" });
});

app.whenReady().then(async () => {
  if (!ownsSingleInstanceLock) {
    return;
  }
  setMacDockIcon();
  try {
    const cleanup = await cleanupLegacyPrimaryRuntime(app.getPath("userData"));
    if (cleanup.status === "removed") {
      console.info("legacy_primary_runtime_removed", { root: cleanup.root });
    } else if (cleanup.status === "refused") {
      console.warn("legacy_primary_runtime_cleanup_refused", cleanup);
    }
  } catch (error) {
    // Retired cache cleanup must never block Desktop startup.
    console.warn("legacy_primary_runtime_cleanup_failed", {
      error: error instanceof Error ? error.message : String(error),
    });
  }
  const systemSkillsSourceRoot = resolveSystemSkillsSourceRoot({
    packaged: app.isPackaged,
    resourcesPath: process.resourcesPath,
    appPath: app.getAppPath(),
  });
  const systemSkillsTargetRoot = resolveSystemSkillsTargetRoot(
    app.getPath("home"),
  );
  try {
    const report = await syncSystemSkills({
      sourceRoot: systemSkillsSourceRoot,
      targetRoot: systemSkillsTargetRoot,
      appVersion: app.getVersion(),
    });
    console.info("system_skills_sync_complete", report);
  } catch (error) {
    // Bundled Skills are an optional startup input. A missing or invalid
    // packaged resource must not prevent Desktop/Core from starting.
    console.warn("system_skills_sync_failed", {
      code:
        error instanceof SystemSkillsSyncError
          ? error.code
          : "SYSTEM_SKILLS_SYNC_FAILED",
      sourceRoot: systemSkillsSourceRoot,
      targetRoot: systemSkillsTargetRoot,
      error: error instanceof Error ? error.message : String(error),
    });
  }
  const configDir = await ensureConfigDir();
  desktopPreferencesStore = new DesktopPreferencesStore(configDir);
  await desktopPreferencesStore.initialize();
  nativeTheme.on("updated", () => {
    if (desktopPreferencesStore?.snapshot().theme === "system") {
      updateMainWindowBackground(
        mainWindowRef,
        "system",
        nativeTheme.shouldUseDarkColors,
      );
    }
  });
  const awakenHome = join(app.getPath("home"), ".awaken");
  modelProfileStore = new ModelProfileStore(awakenHome);
  await modelProfileStore.initialize();
  searchSettingsStore = new SearchSettingsStore(configDir);
  cliConnectorManager = new CliConnectorManager({
    homeDir: app.getPath("home"),
    platform: process.platform,
    architecture: process.arch,
    manifestPath: cliConnectorManifestPath(),
    fetchAsset: (url, signal) => net.fetch(url, { signal }),
    publishReadiness: publishCliReadiness,
    onStateChanged: broadcastCliConnectorState,
  });
  await cliConnectorManager.initialize();
  registerCliAuthProtocol();
  petStore = new PetStore(configDir, app.getPath("userData"));
  await petStore.initialize();
  const builtinPetReport = await importBuiltinPets(
    petStore,
    resolveBuiltinPetsRoot({
      packaged: app.isPackaged,
      resourcesPath: process.resourcesPath,
      appPath: app.getAppPath(),
    }),
  );
  if (builtinPetReport.failed.length > 0 || builtinPetReport.conflicts.length > 0) {
    console.error("[awaken-desktop] built-in pet import incomplete", {
      failed: builtinPetReport.failed,
      conflicts: builtinPetReport.conflicts,
    });
  }
  petWindowManager = new PetWindowManager({
    petStore,
    focusMainWindow: () => {
      focusOrCreateMainWindow();
    },
    activateThread: activatePetThread,
    getRuntimeBaseUrl: () => runtimeBaseUrl,
  });
  await petWindowManager.initialize();

  if (petStore.getSettings().importCodexPets) {
    const codexHome = detectCodexHome(app.getPath("home"));
    void importCodexPets(petStore, codexHome)
      .then(() => {
        broadcastPetCatalogChanged();
        const selectedPetId = petStore?.getSettings().selectedPetId;
        if (selectedPetId && petWindowManager?.isOpen()) {
          return petWindowManager.setPet(selectedPetId);
        }
      })
      .catch((error: unknown) => {
        console.error("[awaken-desktop] codex pet import failed", error);
      });
  }

  registerPetIpcHandlers();

  supervisor = new RuntimeSupervisor(
    {},
    {
      resolveProductEnvironment: async () => {
        try {
          const searchEnvironment = (await searchSettingsStore?.runtimeEnvironment()) ?? {
            AWAKEN_WEB_SEARCH_PROVIDERS: "",
          };
          const managedBin = cliConnectorManager?.managedBinPath() ?? "";
          const inheritedPath = process.env.PATH ?? "";
          return {
            ...searchEnvironment,
            AWAKEN_DESKTOP_CONTROL_TOKEN: desktopControlToken,
            PATH: managedBin ? `${managedBin}${path.delimiter}${inheritedPath}` : inheritedPath,
          };
        } catch (error) {
          console.error("[awaken-desktop] search settings unavailable", {
            message: error instanceof Error ? error.message : String(error),
          });
          return { AWAKEN_WEB_SEARCH_PROVIDERS: "" };
        }
      },
    },
  );
  supervisor.onStatusChange((info) => {
    if (info.runtimeBaseUrl) {
      runtimeBaseUrl = info.runtimeBaseUrl;
      startProjectTrashWorker();
      void cliConnectorManager?.refreshAll();
    } else if (info.status !== "running") {
      runtimeBaseUrl = null;
    }
    for (const wc of webContents.getAllWebContents()) {
      if (!wc.isDestroyed()) {
        wc.send("awaken:supervisor-status-changed", toRendererSupervisorStatus(info));
      }
    }
  });
  createMainWindow();
  if (petStore.getSettings().overlayOpen) {
    void petWindowManager.show().catch((error: unknown) => {
      console.error("[awaken-desktop] failed to restore pet overlay", error);
    });
  }
  void supervisor.start().then((statusInfo) => {
    if (statusInfo.runtimeBaseUrl) {
      runtimeBaseUrl = statusInfo.runtimeBaseUrl;
      startProjectTrashWorker();
      void cliConnectorManager?.refreshAll();
    }
  });

  app.on("activate", () => {
    if (!mainWindowRef || mainWindowRef.isDestroyed()) {
      createMainWindow();
    } else {
      focusOrCreateMainWindow();
    }
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});

app.on("before-quit", (event) => {
  if (isQuitting) return;
  event.preventDefault();
  isQuitting = true;
  stopProjectTrashWorker();
  for (const subscription of subscriptions.values()) {
    subscription.controller.abort();
  }
  subscriptions.clear();
  void (async () => {
    try {
      if (supervisor) {
        await supervisor.stop();
      }
      await cliConnectorManager?.dispose();
      await petWindowManager?.dispose();
    } catch (err) {
      console.error("[awaken-desktop] supervisor stop failed", err);
    }
    app.exit();
  })();
});

process.on("uncaughtException", (error) => {
  console.error("[awaken-desktop] uncaughtException", error);
});

process.on("unhandledRejection", (error) => {
  console.error("[awaken-desktop] unhandledRejection", error);
});

console.info(
  `[awaken-desktop] platform=${toDesktopPlatform(process.platform)}`,
);
