import {
  app,
  BrowserWindow,
  dialog,
  screen as electronScreen,
  ipcMain,
  nativeImage,
  nativeTheme,
  net,
  Notification,
  powerMonitor,
  protocol,
  session,
  shell,
  webContents,
  type IpcMainEvent,
  type IpcMainInvokeEvent,
  type Rectangle,
  type WebContents
} from "electron";
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path, { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { TextDecoder } from "node:util";
import type { AguiEvent } from "../shared/agui.js";
import {
  DESKTOP_SELECT_ALL_REQUESTED_CHANNEL,
  isDesktopSelectAllShortcut,
  isDesktopTabNavigation,
  isDesktopZoomShortcut,
} from "../shared/desktop-selection.js";
import { PET_IPC_CHANNELS } from "../shared/pets.js";
import { PROJECT_FILE_PREVIEW_INTERACTION_CHANNEL } from "../shared/project-file-preview-interaction.js";
import {
  normalizeAgentPlanMediaTools,
  normalizeModelProfileRunConfig,
  normalizeRuntimeProject,
  normalizeRuntimeProjectOperation,
  normalizeRuntimeThreadTranscript,
  normalizeScheduledTask,
  normalizeScheduledTaskExecution,
  PROJECT_DIRECTORY_CHANGED_CHANNEL,
  toScheduledTaskCreateWire,
  toScheduledTaskPatchWire,
  type AgentPlanMediaCapability,
  type AgentPlanMediaToolsSaveResult,
  type AgentPlanMediaToolsWire,
  type AttachmentPickResult,
  type CliConnectorReadiness,
  type CliConnectorView,
  type CliProviderId,
  type ComposerCatalogEvent,
  type DesktopPlatform,
  type DesktopPreferencesSaveRequest,
  type PickedAttachmentFile,
  type ProjectEntryActionRequest,
  type ProjectMutationResult,
  type RuntimeProjectOperationWire,
  type RuntimeProjectWire,
  type RuntimeThreadTranscriptWire,
  type RuntimeThreadWire,
  type ScheduledTask,
  type ScheduledTaskCreateInput,
  type ScheduledTaskEvent,
  type ScheduledTaskExecution,
  type ScheduledTaskExecutionListWire,
  type ScheduledTaskExecutionWire,
  type ScheduledTaskListWire,
  type ScheduledTaskPatchInput,
  type ScheduledTaskWire,
  type UpsertProjectResult
} from "../shared/runtime.js";
import { MACOS_WINDOW_CHROME_GEOMETRY } from "../shared/window-chrome.js";
import { ArtifactMediaManager } from "./artifact-media-manager.js";
import { CliConnectorManager } from "./cli-connectors/manager.js";
import {
  startComposerCatalogEventStream,
  type ComposerCatalogEventStream,
} from "./composer-catalog.js";
import { registerComputerIpc } from "./computer-ipc.js";
import { ComputerOverlayManager } from "./computer-overlay-manager.js";
import { ensureConfigDir } from "./config-bootstrap.js";
import { desktopInstanceIdentity } from "./desktop-instance.js";
import { DesktopPreferencesStore } from "./desktop-preferences-store.js";
import { registerExtensionsIpc } from "./extensions-ipc.js";
import { registerFilesIpc } from "./files-ipc.js";
import { cleanupLegacyPrimaryRuntime } from "./legacy-runtime-cleanup.js";
import { MemoryDreamSchedulerStore } from "./memory-dream-scheduler-store.js";
import { ModelProfileStore } from "./model-profile-store.js";
import { registerModelsIpc } from "./models-ipc.js";
import {
  NetworkRouteManager,
  type NetworkRouteSnapshot,
} from "./network-route-manager.js";
import { NetworkRouteObserver } from "./network-route-observer.js";
import { importBuiltinPets, resolveBuiltinPetsRoot } from "./pets/builtin-pet-importer.js";
import { detectCodexHome, importCodexPets } from "./pets/codex-pet-importer.js";
import { PetStore } from "./pets/pet-store.js";
import { PetWindowManager } from "./pets/pet-window-manager.js";
import { registerPluginsIpc } from "./plugins-ipc.js";
import { ProjectDirectoryWatchManager } from "./project-directory-watch-manager.js";
import { ProjectFilePreviewManager } from "./project-file-preview-manager.js";
import { resolveProjectEntry } from "./project-files.js";
import { executeProjectTrashOperation as executeClaimedProjectTrashOperation } from "./project-trash-worker.js";
import { registerProjectsIpc } from "./projects-ipc.js";
import {
  safelySendToRenderer,
  shouldRecoverMainRenderer,
} from "./renderer-delivery.js";
import { registerRunsIpc } from "./runs-ipc.js";
import { createRuntimeRequester, requireRuntimeSuccess, RuntimeDomainError } from "./runtime-http.js";
import { registerRuntimeRequest } from "./runtime-request.js";
import { RuntimeSupervisor, type SupervisorStatusInfo } from "./runtime-supervisor.js";
import {
  scheduledTaskNotificationContent,
  startScheduledTaskEventStream,
  type ScheduledTaskEventStream,
} from "./scheduled-tasks.js";
import { SearchSettingsStore } from "./search-settings-store.js";
import {
  resolveSystemSkillsSourceRoot,
  resolveSystemSkillsTargetRoot,
  syncSystemSkills,
  SystemSkillsSyncError,
} from "./system-skills-importer.js";
import { updateMainWindowBackground, windowBackgroundColor } from "./window-background.js";
import { enforceMainWindowZoom } from "./window-zoom.js";
// electron-updater 是 CommonJS；按 electron-builder 官方 interop 形式取 autoUpdater，
// 只在 index.ts 获取一次并注入 DesktopUpdateManager，不在多个模块重复绑定 listener。
import electronUpdaterPkg from "electron-updater";
import {
  DESKTOP_UPDATE_CHANNELS,
  disabledDesktopUpdateSnapshot,
  isStableDesktopUpdateIdentity,
  type DesktopRuntimeActivity,
} from "../shared/desktop-update.js";
import { DesktopShutdownCoordinator } from "./desktop-shutdown-coordinator.js";
import { DesktopUpdateManager } from "./desktop-update-manager.js";
import { EMBEDDED_BROWSER_CHANNELS } from "../shared/embedded-browser.js";
import { EmbeddedBrowserHost } from "./embedded-browser/host.js";
import { registerEmbeddedBrowserIpc } from "./embedded-browser/ipc.js";
import { EmbeddedBrowserViewManager } from "./embedded-browser/manager.js";

const { autoUpdater } = electronUpdaterPkg;

const __dirname = dirname(fileURLToPath(import.meta.url));
let runtimeBaseUrl: string | null = null;
const isDevServer = process.argv.includes("--dev");
const instanceIdentity = desktopInstanceIdentity(isDevServer);
app.setName(instanceIdentity.applicationName);
app.setPath("userData", join(app.getPath("appData"), ...instanceIdentity.userDataPathSegments));
if (process.platform === "win32") {
  app.setAppUserModelId(instanceIdentity.applicationId);
}

const ownsSingleInstanceLock = app.requestSingleInstanceLock(instanceIdentity);
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
    scheme: "awaken-preview",
    privileges: {
      standard: true,
      secure: true,
      supportFetchAPI: true,
      corsEnabled: true,
      stream: true,
    },
  },
  {
    scheme: "awaken-artifact",
    privileges: {
      standard: true,
      secure: true,
      supportFetchAPI: true,
      corsEnabled: true,
      stream: true,
    },
  },
]);

let supervisor: RuntimeSupervisor | null = null;
let desktopPreferencesStore: DesktopPreferencesStore | null = null;
let modelProfileStore: ModelProfileStore | null = null;
let searchSettingsStore: SearchSettingsStore | null = null;
let petStore: PetStore | null = null;
let petWindowManager: PetWindowManager | null = null;
let computerOverlayManager: ComputerOverlayManager | null = null;
let cliConnectorManager: CliConnectorManager | null = null;
let networkRouteManager: NetworkRouteManager | null = null;
let networkRouteObserver: NetworkRouteObserver | null = null;
let networkRoutePowerListener: (() => void) | null = null;
let mainWindowRef: BrowserWindow | null = null;
let pendingPetThreadId: string | null = null;
let isQuitting = false;
let desktopUpdateManager: DesktopUpdateManager | null = null;
let desktopShutdownCoordinator: DesktopShutdownCoordinator | null = null;
let updateStartupCheckTimer: NodeJS.Timeout | null = null;
let updateIntervalTimer: NodeJS.Timeout | null = null;
let updatePowerResumeListener: (() => void) | null = null;
let projectTrashWorkerTimer: NodeJS.Timeout | null = null;
let projectTrashWorkerRunning = false;
let memoryDreamSchedulerStore: MemoryDreamSchedulerStore | null = null;
let projectFilePreviewManager: ProjectFilePreviewManager | null = null;
let embeddedBrowserManager: EmbeddedBrowserViewManager | null = null;
let embeddedBrowserHost: EmbeddedBrowserHost | null = null;
let artifactMediaManager: ArtifactMediaManager | null = null;
let memoryDreamSchedulerTimer: NodeJS.Timeout | null = null;
let automaticDreamRunning = false;
const activeDreamTaskIds = new Set<string>();
const completedDreamTaskIds = new Set<string>();
const emittedDreamReceiptTaskIds = new Set<string>();

type StreamSubscription = {
  controller: AbortController;
  webContentsId: number;
};

const subscriptions = new Map<string, StreamSubscription>();
const computerRunMonitors = new Map<string, AbortController>();
const desktopControlToken = randomUUID();
const projectDirectoryWatchManager = new ProjectDirectoryWatchManager(
  ({ webContentsId, event }) => {
    const target = webContents.fromId(webContentsId);
    if (!target || target.isDestroyed()) return;
    target.send(PROJECT_DIRECTORY_CHANGED_CHANNEL, event);
  },
);

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

function networkRouteObserverBinaryPath(): string {
  const targetDir = `${process.platform}-${process.arch}`;
  const binaryName = process.platform === "win32"
    ? "awaken-network-watch.exe"
    : "awaken-network-watch";
  return app.isPackaged
    ? join(process.resourcesPath, "bin", targetDir, binaryName)
    : join(__dirname, "../../resources/bin", targetDir, binaryName);
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

async function synchronizeNetworkRoute(snapshot?: NetworkRouteSnapshot): Promise<void> {
  const client = supervisor?.getControlClient();
  const current = snapshot ?? networkRouteManager?.currentSnapshot();
  if (!client || !current) return;
  try {
    await client.updateNetworkRoute(current);
  } catch (error) {
    console.warn("network_route_sync_failed", {
      generation: current.generation,
      platform: process.platform,
      error: error instanceof Error ? error.message : String(error),
    });
  }
}

async function prepareNetworkRoute(surface: string): Promise<void> {
  await networkRouteManager?.prepareForNetworkOperation(surface);
}

async function registerEmbeddedBrowserHostWithCore(): Promise<void> {
  const client = supervisor?.getControlClient();
  if (!client || !embeddedBrowserHost) return;
  try {
    const registration = await embeddedBrowserHost.start();
    await client.registerBrowserHost(registration);
  } catch (error) {
    console.warn("embedded_browser_host_register_failed", {
      error: error instanceof Error ? error.message : String(error),
    });
  }
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
  return (
    attachmentMimeTypesByExtension.get(path.extname(filePath).toLowerCase()) ??
    "application/octet-stream"
  );
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
  return new URL(
    pathname,
    runtimeBaseUrl.endsWith("/") ? runtimeBaseUrl : `${runtimeBaseUrl}/`,
  ).toString();
}

function toRendererSupervisorStatus(info: SupervisorStatusInfo): {
  status: SupervisorStatusInfo["status"];
} {
  return { status: info.status };
}

function hostFromBaseURL(baseURL: string): string {
  try {
    return new URL(baseURL).host;
  } catch {
    return "";
  }
}

const { request: requestRuntime, requestVoid: requestRuntimeOK } = createRuntimeRequester(runtimeUrl);

function upsertProjectFailureMessage(error: unknown): string {
  if (!(error instanceof RuntimeDomainError)) {
    return error instanceof Error ? error.message : "打开项目失败，请稍后重试。";
  }
  const messages: Record<string, string> = {
    PROJECT_NOT_FOUND: "项目不存在或已经被移除。",
    PROJECT_OPERATION_CONFLICT: "该文件夹正在处理其他操作（例如移除），请稍后再试。",
    PROJECT_RUNS_BUSY: "项目中有正在运行的任务，请等待任务结束后再打开。",
    PROJECT_PATH_INVALID: "该文件夹不能作为 Awaken 项目。",
    PROJECT_NAME_INVALID: "项目名称不能为空。",
    PROJECT_PATH_FINGERPRINT_CHANGED: "文件夹内容已发生变化，Awaken 已停止本次操作以避免误操作。",
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
  await requestRuntime<RuntimeProjectOperationWire>(
    `/v1/project-operations/${encodeURIComponent(operationId)}/fail-trash`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ attempt_id: attemptId, code, message }),
    },
  );
}

async function executeProjectTrashOperation(operation: RuntimeProjectOperationWire): Promise<void> {
  await executeClaimedProjectTrashOperation(operation, {
    claim: async (operationId) => {
      try {
        return await requestRuntime<RuntimeProjectOperationWire>(
          `/v1/project-operations/${encodeURIComponent(operationId)}/claim-trash`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              worker_id: "desktop-main",
              lease_seconds: 30,
            }),
          },
        );
      } catch (error) {
        if (
          error instanceof RuntimeDomainError &&
          (error.code === "PROJECT_RUNS_BUSY" || error.code === "PROJECT_OPERATION_CONFLICT")
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
      return requestRuntime<RuntimeProjectOperationWire>(
        `/v1/project-operations/${encodeURIComponent(operationId)}/stage-trash`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ attempt_id: attemptId }),
        },
      );
    },
    beginInvocation: async (operationId, attemptId) => {
      await requestRuntime<RuntimeProjectOperationWire>(
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
      await requestRuntime<RuntimeProjectOperationWire>(
        `/v1/project-operations/${encodeURIComponent(operationId)}/complete-trash`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ attempt_id: attemptId }),
        },
      );
    },
    fail: failProjectTrashOperation,
    reportTrashFailure: ({ operationId, folderPath, error }) => {
      console.error("project_trash_failed", { operationId, folderPath, error });
    },
  });
}

async function processPendingProjectTrashOperations(): Promise<void> {
  if (projectTrashWorkerRunning || !runtimeBaseUrl) {
    return;
  }
  projectTrashWorkerRunning = true;
  try {
    const operations = await requestRuntime<RuntimeProjectOperationWire[]>(
      "/v1/project-operations",
      {
        method: "GET",
      },
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

function sendToRenderer(
  channel: string,
  payload: unknown,
  targetId: number,
): boolean {
  const target = webContents.fromId(targetId);
  return safelySendToRenderer(target, channel, payload);
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
  const event = JSON.parse(payload) as AguiEvent;
  if (event.instructionSnapshot) {
    event.instructionSnapshot = {
      ...event.instructionSnapshot,
      sources: event.instructionSnapshot.sources ?? [],
      diagnostics: event.instructionSnapshot.diagnostics ?? [],
    };
  }
  onEvent(event);
}

async function pumpComputerOverlayEvents(
  body: ReadableStream<Uint8Array>,
  runId: string,
  controller: AbortController,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (!controller.signal.aborted) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split("\n\n");
      buffer = frames.pop() ?? "";
      for (const frame of frames) {
        if (!frame.trim()) continue;
        consumeSSEFrame(frame, (event) => computerOverlayManager?.handleRunEvent(event, runId));
      }
    }
    if (buffer.trim() && !controller.signal.aborted) {
      consumeSSEFrame(buffer, (event) => computerOverlayManager?.handleRunEvent(event, runId));
    }
  } finally {
    reader.releaseLock();
    computerRunMonitors.delete(runId);
    computerOverlayManager?.handleRunStreamClosed(runId);
  }
}

function ensureComputerRunMonitor(runId: string): void {
  const normalizedRunId = runId.trim();
  if (!normalizedRunId || computerRunMonitors.has(normalizedRunId) || !runtimeBaseUrl) return;
  const controller = new AbortController();
  computerRunMonitors.set(normalizedRunId, controller);
  void fetch(runtimeUrl(`/v1/runs/${encodeURIComponent(normalizedRunId)}/events`), {
    headers: { Accept: "text/event-stream" },
    signal: controller.signal,
  })
    .then(async (response) => {
      await requireRuntimeSuccess(response);
      if (!response.body) throw new Error("Runtime stream body is missing");
      await pumpComputerOverlayEvents(response.body, normalizedRunId, controller);
    })
    .catch((error: unknown) => {
      computerRunMonitors.delete(normalizedRunId);
      computerOverlayManager?.handleRunStreamClosed(normalizedRunId);
      if (!controller.signal.aborted) {
        console.error("computer_overlay_event_stream_failed", {
          runId: normalizedRunId,
          error: error instanceof Error ? error.message : String(error),
        });
      }
    });
}

function stopComputerRunMonitors(): void {
  for (const controller of computerRunMonitors.values()) controller.abort();
  computerRunMonitors.clear();
}

async function pumpRunEvents(
  body: ReadableStream<Uint8Array>,
  subscriptionId: string,
  targetId: number,
  controller: AbortController,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (!controller.signal.aborted) {
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
        let rendererAvailable = true;
        consumeSSEFrame(frame, (event) => {
          trackMemoryActivity(event);
          rendererAvailable = sendToRenderer(
            "awaken:run-event",
            {
              subscriptionId,
              event,
            },
            targetId,
          );
        });
        if (!rendererAvailable) {
          controller.abort();
          return;
        }
      }
    }

    if (buffer.trim() && !controller.signal.aborted) {
      let rendererAvailable = true;
      consumeSSEFrame(buffer, (event) => {
        trackMemoryActivity(event);
        rendererAvailable = sendToRenderer(
          "awaken:run-event",
          {
            subscriptionId,
            event,
          },
          targetId,
        );
      });
      if (!rendererAvailable) {
        controller.abort();
        return;
      }
    }
  } finally {
    reader.releaseLock();
    subscriptions.delete(subscriptionId);
  }

  // Normal completion: cleanup and notify renderer. On error, this path is
  // skipped (the rejection propagates to the .catch() handler in
  // subscribeToRun, which sends run-error BEFORE run-closed so the renderer
  // receives the real error message before the subscription is torn down).
  if (!controller.signal.aborted) {
    sendToRenderer("awaken:run-closed", { subscriptionId }, targetId);
  }
}

async function subscribeToRun(
  target: WebContents,
  runId: string,
  subscriptionId: string,
  afterCursor?: number,
  activityThreadId?: string,
): Promise<void> {
  const controller = new AbortController();
  const path = activityThreadId ? `/v1/threads/${encodeURIComponent(activityThreadId)}/activity` : `/v1/runs/${encodeURIComponent(runId)}/events`;
  const eventsUrl = afterCursor ? `${path}?after_cursor=${afterCursor}` : path;
  const response = await fetch(runtimeUrl(eventsUrl), {
    headers: {
      Accept: "text/event-stream",
    },
    signal: controller.signal,
  });

  await requireRuntimeSuccess(response);
  if (!response.body) {
    throw new Error("Runtime stream body is missing");
  }

  subscriptions.set(subscriptionId, {
    controller,
    webContentsId: target.id,
  });

  void pumpRunEvents(response.body, subscriptionId, target.id, controller).catch((error: unknown) => {
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
  projectDirectoryWatchManager.releaseForWebContents(targetId);
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
          trafficLightPosition: MACOS_WINDOW_CHROME_GEOMETRY.trafficLightPosition,
        }
      : {}),
    webPreferences: {
      preload: join(__dirname, "../preload/index.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      webviewTag: true,
    },
  });
  mainWindowRef = mainWindow;

  const resetMainWindowZoom = () => {
    void enforceMainWindowZoom(mainWindow.webContents).catch((error) => {
      console.warn("Failed to enforce the Awaken main-window zoom baseline", error);
    });
  };
  resetMainWindowZoom();
  mainWindow.webContents.on("did-finish-load", resetMainWindowZoom);
  mainWindow.webContents.on("zoom-changed", (event) => {
    event.preventDefault();
    resetMainWindowZoom();
  });

  mainWindow.webContents.on("before-input-event", (event, input) => {
    if (isDesktopTabNavigation(input)) {
      event.preventDefault();
      return;
    }
    if (isDesktopZoomShortcut(input)) {
      event.preventDefault();
      resetMainWindowZoom();
      return;
    }
    if (!isDesktopSelectAllShortcut(input)) {
      return;
    }
    event.preventDefault();
    mainWindow.webContents.send(DESKTOP_SELECT_ALL_REQUESTED_CHANNEL);
  });

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url);
    return { action: "deny" };
  });

  mainWindow.webContents.on("will-attach-webview", (event, webPreferences, params) => {
    if (!projectFilePreviewManager?.authorizeWebview(webPreferences, params)) {
      event.preventDefault();
    }
  });
  mainWindow.webContents.on("did-attach-webview", (_event, guest) => {
    guest.on("before-input-event", (event, input) => {
      if (isDesktopTabNavigation(input)) {
        event.preventDefault();
      }
    });
    projectFilePreviewManager?.attachGuest(guest);
  });

  const webContentsId = mainWindow.webContents.id;
  let electronReadyToShow = false;
  let rendererReadyToShow = false;
  let didShowWindow = false;
  let fallbackShowTimer: ReturnType<typeof setTimeout> | undefined;
  let rendererRecoveryAttempts = 0;
  let rendererRecoveryTimer: ReturnType<typeof setTimeout> | undefined;

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
    scheduleDesktopUpdateChecks();
    if (pendingPetThreadId) {
      event.sender.send(PET_IPC_CHANNELS.petThreadActivationRequested, pendingPetThreadId);
      pendingPetThreadId = null;
    }
  }

  ipcMain.on("awaken:renderer-ready", handleRendererReady);

  const hideEmbeddedBrowserViews = () => {
    embeddedBrowserManager?.hideAll();
  };
  mainWindow.webContents.on("did-start-loading", hideEmbeddedBrowserViews);

  mainWindow.webContents.on("render-process-gone", (_event, details) => {
    hideEmbeddedBrowserViews();
    cleanupWindowSubscriptions(webContentsId);
    if (
      isQuitting ||
      mainWindow.isDestroyed() ||
      !shouldRecoverMainRenderer(details.reason, rendererRecoveryAttempts)
    ) {
      return;
    }

    rendererRecoveryAttempts += 1;
    console.error("main_renderer_process_gone", {
      reason: details.reason,
      exitCode: details.exitCode,
      recoveryAttempt: rendererRecoveryAttempts,
    });
    if (rendererRecoveryTimer) {
      clearTimeout(rendererRecoveryTimer);
    }
    rendererRecoveryTimer = setTimeout(() => {
      rendererRecoveryTimer = undefined;
      if (!mainWindow.isDestroyed()) {
        mainWindow.webContents.reloadIgnoringCache();
      }
    }, 150);
  });

  mainWindow.webContents.on("did-finish-load", () => {
    if (rendererRecoveryAttempts === 0) {
      return;
    }
    if (rendererRecoveryTimer) {
      clearTimeout(rendererRecoveryTimer);
    }
    rendererRecoveryTimer = setTimeout(() => {
      rendererRecoveryTimer = undefined;
      rendererRecoveryAttempts = 0;
    }, 30_000);
  });

  mainWindow.on("closed", () => {
    if (mainWindowRef === mainWindow) {
      mainWindowRef = null;
    }
    ipcMain.removeListener("awaken:renderer-ready", handleRendererReady);
    if (fallbackShowTimer) {
      clearTimeout(fallbackShowTimer);
      fallbackShowTimer = undefined;
    }
    if (rendererRecoveryTimer) {
      clearTimeout(rendererRecoveryTimer);
      rendererRecoveryTimer = undefined;
    }
    cleanupWindowSubscriptions(webContentsId);
    embeddedBrowserManager?.hideAll();
    projectFilePreviewManager?.releaseAll();
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
    mainWindow.webContents.send(PET_IPC_CHANNELS.petThreadActivationRequested, threadId);
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

// ---------------------------------------------------------------------------
// Scheduled tasks (contract [HTTP、IPC 与事件](docs/12_定时任务/定时执行与提醒/实现方案.md#httpipc-与事件)): typed IPC forwards plus a
// resident SSE subscription that feeds renderer refreshes and native
// notifications. Renderer pages being closed never affects this pump, so a
// notice is never lost with the page.
// ---------------------------------------------------------------------------

let scheduledTaskEventStream: ScheduledTaskEventStream | null = null;
let composerCatalogEventStream: ComposerCatalogEventStream | null = null;

function broadcastComposerCatalogEvent(event: ComposerCatalogEvent): void {
  for (const wc of webContents.getAllWebContents()) {
    if (!wc.isDestroyed()) {
      wc.send("awaken:composer-catalog-event", event);
    }
  }
}

function startComposerCatalogEventStreamIfNeeded(): void {
  if (composerCatalogEventStream || !runtimeBaseUrl) {
    return;
  }
  composerCatalogEventStream = startComposerCatalogEventStream({
    baseUrl: runtimeBaseUrl,
    onEvent: broadcastComposerCatalogEvent,
    onError: (message) => {
      console.error("[awaken-desktop] composer catalog event stream", { message });
    },
  });
}

function stopComposerCatalogEventStream(): void {
  composerCatalogEventStream?.stop();
  composerCatalogEventStream = null;
}

function broadcastScheduledTaskEvent(event: ScheduledTaskEvent): void {
  for (const wc of webContents.getAllWebContents()) {
    if (!wc.isDestroyed()) {
      wc.send("awaken:scheduled-task-event", event);
    }
  }
}

function handleScheduledTaskEvent(event: ScheduledTaskEvent): void {
  broadcastScheduledTaskEvent(event);
  // Only live notice events become native notifications; Core emits a notice
  // exactly once when it fires, so reconnects never re-notify.
  const content = scheduledTaskNotificationContent(event);
  if (!content || !Notification.isSupported()) {
    return;
  }
  const notification = new Notification({
    title: content.title,
    body: content.body,
  });
  notification.on("click", () => {
    const mainWindow = focusOrCreateMainWindow();
    const threadId = event.threadId ?? "";
    if (threadId && !mainWindow.webContents.isDestroyed()) {
      mainWindow.webContents.send("awaken:scheduled-task-thread-activation", {
        threadId,
      });
    }
  });
  notification.show();
}

function startScheduledTaskEventStreamIfNeeded(): void {
  if (scheduledTaskEventStream || !runtimeBaseUrl) {
    return;
  }
  scheduledTaskEventStream = startScheduledTaskEventStream({
    baseUrl: runtimeBaseUrl,
    onEvent: handleScheduledTaskEvent,
    onError: (message) => {
      console.error("[awaken-desktop] scheduled task event stream", {
        message,
      });
    },
  });
}

function stopScheduledTaskEventStream(): void {
  scheduledTaskEventStream?.stop();
  scheduledTaskEventStream = null;
}

/** Scheduled-task IPC is accepted from the main window only, mirroring the
 * artifact handler sender check. */
function assertMainWindowSender(event: IpcMainInvokeEvent): void {
  if (
    !mainWindowRef ||
    mainWindowRef.isDestroyed() ||
    event.sender.id !== mainWindowRef.webContents.id
  ) {
    throw new Error("无法确认定时任务请求来自 Awaken 主窗口。");
  }
}

function scheduledTaskPath(taskId: string, suffix = ""): string {
  return `/v1/scheduled-tasks/${encodeURIComponent(taskId)}${suffix}`;
}

/**
 * 更新命令只接受当前主窗口；payload 不含 URL/路径/版本/资产选择。
 * 快照广播只发往当前主窗口，窗口销毁后由 closed 处理自然终止。
 */
function requireMainWindowSender(event: IpcMainInvokeEvent): void {
  if (
    !mainWindowRef ||
    mainWindowRef.isDestroyed() ||
    event.sender.id !== mainWindowRef.webContents.id
  ) {
    throw new Error("无法确认更新请求来自 Awaken 主窗口。");
  }
}

async function fetchRuntimeActivitySnapshot(): Promise<DesktopRuntimeActivity | null> {
  if (!runtimeBaseUrl) {
    return null;
  }
  try {
    return await requestRuntime<DesktopRuntimeActivity>("/v1/runtime/activity", {
      method: "GET",
    });
  } catch (error) {
    console.warn("desktop_update_activity_unavailable", {
      component: "desktop-updater",
      error: error instanceof Error ? error.message : String(error),
    });
    return null;
  }
}

async function readDesktopUpdateBuildChannel(): Promise<unknown> {
  if (!app.isPackaged) return undefined;
  try {
    const metadata = JSON.parse(
      await fs.readFile(join(app.getAppPath(), "package.json"), "utf8"),
    ) as { awakenUpdateChannel?: unknown };
    return metadata.awakenUpdateChannel;
  } catch (error) {
    console.warn("desktop_update_identity_unavailable", {
      component: "desktop-updater",
      error_name: error instanceof Error ? error.name : "unknown",
    });
    return undefined;
  }
}

function subscribeDesktopUpdateStateBroadcast(): void {
  if (!desktopUpdateManager) return;
  desktopUpdateManager.subscribe((snapshot) => {
    if (!mainWindowRef || mainWindowRef.isDestroyed()) return;
    mainWindowRef.webContents.send(DESKTOP_UPDATE_CHANNELS.stateChanged, snapshot);
  });
}

function registerDesktopUpdateIpcHandlers(): void {
  ipcMain.handle(DESKTOP_UPDATE_CHANNELS.getState, (event) => {
    requireMainWindowSender(event);
    return (
      desktopUpdateManager?.getSnapshot() ?? disabledDesktopUpdateSnapshot(app.getVersion())
    );
  });
  ipcMain.handle(DESKTOP_UPDATE_CHANNELS.check, async (event) => {
    requireMainWindowSender(event);
    return (
      desktopUpdateManager?.check("retry") ?? disabledDesktopUpdateSnapshot(app.getVersion())
    );
  });
  ipcMain.handle(DESKTOP_UPDATE_CHANNELS.download, async (event) => {
    requireMainWindowSender(event);
    return (
      desktopUpdateManager?.download() ?? disabledDesktopUpdateSnapshot(app.getVersion())
    );
  });
  ipcMain.handle(DESKTOP_UPDATE_CHANNELS.restartAndInstall, async (event) => {
    requireMainWindowSender(event);
    return (
      desktopUpdateManager?.restartAndInstall() ??
      disabledDesktopUpdateSnapshot(app.getVersion())
    );
  });
}

/** Renderer ready 后延迟首次检查；之后每 6 小时最多自动检查一次（manager 内部节流）。 */
function scheduleDesktopUpdateChecks(): void {
  if (!desktopUpdateManager || updateStartupCheckTimer || updateIntervalTimer) {
    return;
  }
  updateStartupCheckTimer = setTimeout(() => {
    updateStartupCheckTimer = null;
    void desktopUpdateManager?.check("startup");
    if (!updateIntervalTimer) {
      updateIntervalTimer = setInterval(
        () => void desktopUpdateManager?.check("interval"),
        6 * 60 * 60 * 1000,
      );
    }
  }, 20_000);
}

function stopDesktopUpdateScheduling(): void {
  if (updateStartupCheckTimer) {
    clearTimeout(updateStartupCheckTimer);
    updateStartupCheckTimer = null;
  }
  if (updateIntervalTimer) {
    clearInterval(updateIntervalTimer);
    updateIntervalTimer = null;
  }
  if (updatePowerResumeListener) {
    powerMonitor.removeListener("resume", updatePowerResumeListener);
    updatePowerResumeListener = null;
  }
}

function registerScheduledTaskIpcHandlers(): void {
  registerRuntimeRequest("awaken:list-scheduled-tasks", async (event): Promise<ScheduledTask[]> => {
    assertMainWindowSender(event);
    const result = await requestRuntime<ScheduledTaskListWire>("/v1/scheduled-tasks", {
      method: "GET",
    });
    return (result.tasks ?? []).map(normalizeScheduledTask);
  });

  registerRuntimeRequest(
    "awaken:get-scheduled-task",
    async (event, taskId: string): Promise<ScheduledTask> => {
      assertMainWindowSender(event);
      const task = await requestRuntime<ScheduledTaskWire>(scheduledTaskPath(taskId), {
        method: "GET",
      });
      return normalizeScheduledTask(task);
    },
  );

  registerRuntimeRequest(
    "awaken:create-scheduled-task",
    async (event, input: ScheduledTaskCreateInput): Promise<ScheduledTask> => {
      assertMainWindowSender(event);
      const task = await requestRuntime<ScheduledTaskWire>("/v1/scheduled-tasks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(toScheduledTaskCreateWire(input)),
      });
      return normalizeScheduledTask(task);
    },
  );

  registerRuntimeRequest(
    "awaken:patch-scheduled-task",
    async (event, taskId: string, input: ScheduledTaskPatchInput): Promise<ScheduledTask> => {
      assertMainWindowSender(event);
      const task = await requestRuntime<ScheduledTaskWire>(scheduledTaskPath(taskId), {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(toScheduledTaskPatchWire(input)),
      });
      return normalizeScheduledTask(task);
    },
  );

  registerRuntimeRequest(
    "awaken:delete-scheduled-task",
    async (
      event,
      request: {
        taskId: string;
        expectedVersion: number;
        idempotencyKey: string;
      },
    ): Promise<void> => {
      assertMainWindowSender(event);
      await requestRuntimeOK(
        `${scheduledTaskPath(request.taskId)}?expected_version=${encodeURIComponent(String(request.expectedVersion))}`,
        {
          method: "DELETE",
          headers: { "Idempotency-Key": request.idempotencyKey },
        },
      );
    },
  );

  const handleSetEnabled =
    (enabled: boolean) =>
    async (
      event: IpcMainInvokeEvent,
      request: { taskId: string; expectedVersion: number },
    ): Promise<ScheduledTask> => {
      assertMainWindowSender(event);
      const task = await requestRuntime<ScheduledTaskWire>(
        scheduledTaskPath(request.taskId, enabled ? "/resume" : "/pause"),
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ expected_version: request.expectedVersion }),
        },
      );
      return normalizeScheduledTask(task);
    };
  registerRuntimeRequest("awaken:pause-scheduled-task", handleSetEnabled(false));
  registerRuntimeRequest("awaken:resume-scheduled-task", handleSetEnabled(true));

  registerRuntimeRequest(
    "awaken:run-scheduled-task-now",
    async (
      event,
      request: { taskId: string; idempotencyKey: string },
    ): Promise<ScheduledTaskExecution> => {
      assertMainWindowSender(event);
      const execution = await requestRuntime<ScheduledTaskExecutionWire>(
        scheduledTaskPath(request.taskId, "/run"),
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ idempotency_key: request.idempotencyKey }),
        },
      );
      return normalizeScheduledTaskExecution(execution);
    },
  );

  registerRuntimeRequest(
    "awaken:cancel-scheduled-task-execution",
    async (event, taskId: string): Promise<void> => {
      assertMainWindowSender(event);
      await requestRuntimeOK(scheduledTaskPath(taskId, "/cancel"), {
        method: "POST",
      });
    },
  );

  registerRuntimeRequest(
    "awaken:list-scheduled-task-executions",
    async (event, taskId: string): Promise<ScheduledTaskExecution[]> => {
      assertMainWindowSender(event);
      const result = await requestRuntime<ScheduledTaskExecutionListWire>(
        scheduledTaskPath(taskId, "/executions"),
        { method: "GET" },
      );
      return (result.executions ?? []).map(normalizeScheduledTaskExecution);
    },
  );

  registerRuntimeRequest("awaken:get-system-timezone", async (event): Promise<string> => {
    assertMainWindowSender(event);
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  });
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
    updateMainWindowBackground(mainWindowRef, preferences.theme, nativeTheme.shouldUseDarkColors);
    return preferences;
  },
);

type MemoryWire = {
  memory_id: string;
  name: string;
  description: string;
  type: "user" | "feedback" | "project" | "reference";
  status: "active" | "superseded";
  revision: number;
  updated_at: string;
  source_thread_id: string;
  source_record_id: string;
  scope: "user" | "project";
  content?: string;
};

type MemorySettingsDocumentWire = {
  revision: number;
  value: {
    enabled: boolean;
    automatic_recall: boolean;
    proactive_memory: boolean;
    auto_dream: boolean;
  };
};

type MemoryDreamTaskWire = {
  task_id: string;
  status: "queued" | "running" | "completed" | "failed" | "cancelled" | "interrupted";
  phase: string;
  project_id?: string;
  scopes: Array<"user" | "project">;
  reports?: Record<
    string,
    { updated: number; superseded: number; conflicts: number; skipped: number }
  >;
  error_code?: string;
  error_message?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
};

const normalizeMemory = (item: MemoryWire) => ({
  memoryId: item.memory_id,
  name: item.name,
  description: item.description,
  type: item.type,
  status: item.status,
  revision: item.revision,
  updatedAt: item.updated_at,
  sourceThreadId: item.source_thread_id,
  sourceRecordId: item.source_record_id,
  scope: item.scope,
  content: item.content,
});
const normalizeMemorySettings = (document: MemorySettingsDocumentWire) => ({
  revision: document.revision,
  value: {
    enabled: document.value.enabled,
    automaticRecall: document.value.automatic_recall,
    proactiveMemory: document.value.proactive_memory,
    autoDream: document.value.auto_dream,
  },
});
const normalizeMemoryDream = (task: MemoryDreamTaskWire) => ({
  taskId: task.task_id,
  status: task.status,
  phase: task.phase,
  projectId: task.project_id,
  scopes: task.scopes,
  reports: task.reports,
  errorCode: task.error_code,
  errorMessage: task.error_message,
  createdAt: task.created_at,
  startedAt: task.started_at,
  finishedAt: task.finished_at,
});

async function startMemoryDreamTask(
  projectId: string | undefined,
  scopes: Array<"user" | "project">,
): Promise<ReturnType<typeof normalizeMemoryDream>> {
  const profile = await modelProfileStore?.resolveDefaultRunProfile();
  if (!profile) throw new Error("请先配置并启用主模型。");
  const task = normalizeMemoryDream(
    await requestRuntime<MemoryDreamTaskWire>("/v1/memory/dream", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        project_id: projectId,
        scopes,
        model_profile: normalizeModelProfileRunConfig(profile),
      }),
    }),
  );
  activeDreamTaskIds.add(task.taskId);
  return task;
}

async function getMemoryDreamTask(
  taskId: string,
): Promise<ReturnType<typeof normalizeMemoryDream>> {
  const task = normalizeMemoryDream(
    await requestRuntime<MemoryDreamTaskWire>(`/v1/memory/dream/${encodeURIComponent(taskId)}`),
  );
  if (["completed", "failed", "cancelled", "interrupted"].includes(task.status)) {
    activeDreamTaskIds.delete(taskId);
    if (!emittedDreamReceiptTaskIds.has(taskId)) {
      emittedDreamReceiptTaskIds.add(taskId);
      mainWindowRef?.webContents.send("awaken:memory-dream-receipt", task);
    }
  }
  if (task.status === "completed" && !completedDreamTaskIds.has(taskId)) {
    completedDreamTaskIds.add(taskId);
    memoryDreamSchedulerStore?.recordSuccessfulDream();
  }
  return task;
}

async function waitForMemoryDream(taskId: string): Promise<void> {
  for (;;) {
    const task = await getMemoryDreamTask(taskId);
    if (task.status === "completed") return;
    if (["failed", "cancelled", "interrupted"].includes(task.status))
      throw new Error(task.errorMessage || `Dream ${task.status}`);
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
}

async function cancelActiveDreams(): Promise<void> {
  const tasks = [...activeDreamTaskIds];
  await Promise.allSettled(
    tasks.map((taskId) =>
      requestRuntimeOK(`/v1/memory/dream/${encodeURIComponent(taskId)}/cancel`, { method: "POST" }),
    ),
  );
  for (const taskId of tasks) activeDreamTaskIds.delete(taskId);
}

async function runAutomaticDreamIfEligible(): Promise<void> {
  if (
    automaticDreamRunning ||
    !runtimeBaseUrl ||
    !memoryDreamSchedulerStore?.eligible() ||
    powerMonitor.getSystemIdleTime() < 5 * 60
  )
    return;
  automaticDreamRunning = true;
  try {
    const settings = normalizeMemorySettings(
      await requestRuntime<MemorySettingsDocumentWire>("/v1/memory/settings"),
    );
    if (!settings.value.enabled || !settings.value.autoDream) return;
    const threads = await requestRuntime<RuntimeThreadWire[]>("/v1/threads");
    if (threads.some((thread) => Boolean(thread.active_run_id)) || activeDreamTaskIds.size > 0)
      return;
    const projects = await requestRuntime<RuntimeProjectWire[]>(
      "/v1/projects?include_removed=false",
    );
    const userTask = await startMemoryDreamTask(undefined, ["user"]);
    await waitForMemoryDream(userTask.taskId);
    for (const project of projects.filter((item) => item.state === "active")) {
      const projectTask = await startMemoryDreamTask(project.project_id, ["project"]);
      await waitForMemoryDream(projectTask.taskId);
    }
  } catch (error) {
    console.error("automatic_memory_dream_failed", error);
  } finally {
    automaticDreamRunning = false;
  }
}

function trackMemoryActivity(event: AguiEvent): void {
  if (event.type === "RUN_FINISHED" && event.runId)
    memoryDreamSchedulerStore?.recordCompletedRun(event.runId);
  if (
    [
      "AWAKEN_MEMORY_CREATED",
      "AWAKEN_MEMORY_UPDATED",
      "AWAKEN_MEMORY_SUPERSEDED",
      "AWAKEN_MEMORY_DELETED",
    ].includes(event.type)
  ) {
    memoryDreamSchedulerStore?.recordMemoryMutation(
      event.recordId || `${event.runId ?? "run"}:${event.seq ?? event.streamCursor ?? 0}`,
    );
  }
}

ipcMain.handle("awaken:list-memories", async (_event, request = {}) => {
  const input = request as {
    query?: string;
    scope?: string;
    projectId?: string;
    limit?: number;
    type?: string;
    status?: string;
  };
  const params = new URLSearchParams();
  if (input.query) params.set("query", input.query);
  if (input.scope) params.set("scope", input.scope);
  if (input.projectId) params.set("project_id", input.projectId);
  if (input.limit) params.set("limit", String(input.limit));
  if (input.type) params.set("type", input.type);
  if (input.status) params.set("status", input.status);
  const items = await requestRuntime<MemoryWire[]>(`/v1/memories?${params.toString()}`);
  return items.map(normalizeMemory);
});
ipcMain.handle("awaken:get-memory", async (_event, memoryId: string, projectId?: string) => {
  const suffix = projectId ? `?project_id=${encodeURIComponent(projectId)}` : "";
  return normalizeMemory(
    await requestRuntime<MemoryWire>(`/v1/memories/${encodeURIComponent(memoryId)}${suffix}`),
  );
});
ipcMain.handle(
  "awaken:delete-memory",
  async (_event, memoryId: string, expectedRevision: number, projectId?: string) => {
    const suffix = projectId ? `?project_id=${encodeURIComponent(projectId)}` : "";
    await requestRuntimeOK(`/v1/memories/${encodeURIComponent(memoryId)}${suffix}`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expected_revision: expectedRevision }),
    });
    memoryDreamSchedulerStore?.recordMemoryMutation(
      `settings-delete:${memoryId}:${expectedRevision}`,
    );
  },
);
ipcMain.handle("awaken:get-memory-settings", async () =>
  normalizeMemorySettings(await requestRuntime<MemorySettingsDocumentWire>("/v1/memory/settings")),
);
ipcMain.handle("awaken:update-memory-settings", async (_event, request) => {
  const input = request as {
    expectedRevision: number;
    enabled?: boolean;
    automaticRecall?: boolean;
    proactiveMemory?: boolean;
    autoDream?: boolean;
  };
  return normalizeMemorySettings(
    await requestRuntime<MemorySettingsDocumentWire>("/v1/memory/settings", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        expected_revision: input.expectedRevision,
        enabled: input.enabled,
        automatic_recall: input.automaticRecall,
        proactive_memory: input.proactiveMemory,
        auto_dream: input.autoDream,
      }),
    }),
  );
});
ipcMain.handle("awaken:start-memory-dream", async (_event, projectId?: string) => {
  const scopes: Array<"user" | "project"> = projectId ? ["user", "project"] : ["user"];
  return startMemoryDreamTask(projectId, scopes);
});
ipcMain.handle("awaken:get-memory-dream", async (_event, taskId: string) =>
  getMemoryDreamTask(taskId),
);
ipcMain.handle("awaken:list-memory-dreams", async (_event, projectId?: string, limit = 5) => {
  const params = new URLSearchParams({ limit: String(limit) });
  if (projectId) params.set("project_id", projectId);
  const tasks = await requestRuntime<MemoryDreamTaskWire[]>(
    `/v1/memory/dream?${params.toString()}`,
  );
  return tasks.map(normalizeMemoryDream);
});
ipcMain.handle("awaken:cancel-memory-dream", async (_event, taskId: string) => {
  await requestRuntimeOK(`/v1/memory/dream/${encodeURIComponent(taskId)}/cancel`, {
    method: "POST",
  });
  activeDreamTaskIds.delete(taskId);
});
ipcMain.handle("awaken:uninstall-managed-cli", async (_event, providerId: CliProviderId) => {
  if (!cliConnectorManager) throw new Error("CLI connector manager unavailable");
  return cliConnectorManager.uninstallManaged(providerId);
});

async function ensureNoActiveRunsForSettingsChange(): Promise<void> {
  const threads = await requestRuntime<RuntimeThreadWire[]>("/v1/threads");
  if (threads.some((thread) => Boolean(thread.active_run_id))) {
    throw new Error("当前有任务正在运行，请结束任务后重试。");
  }
}

async function hasAgentPlanMediaToolReferences(): Promise<boolean> {
  if (!modelProfileStore) return false;
  return (
    (await modelProfileStore.getAgentPlanMediaTools()).configurations.length > 0
  );
}

async function restartCoreForAgentPlanTools(): Promise<void> {
  if (!supervisor) throw new Error("Core supervisor unavailable");
  const status = await supervisor.restart();
  if (status.status !== "running" || !status.runtimeBaseUrl) {
    throw new Error(
      "Agent Plan 配置已保存，但 Core 重启失败。请重启 Awaken 后再使用生成工具。",
    );
  }
}

async function mutateAgentPlanMediaTools(
  capability: AgentPlanMediaCapability,
  operation: () => Promise<AgentPlanMediaToolsWire>,
): Promise<AgentPlanMediaToolsSaveResult> {
  if (!supervisor) throw new Error("Core supervisor unavailable");
  await ensureNoActiveRunsForSettingsChange();
  const settings = normalizeAgentPlanMediaTools(await operation());
  try {
    const client = supervisor.getControlClient();
    if (!client) throw new Error("Core control client unavailable");
    await client.reloadAgentPlanMediaTool(capability);
    return { settings, applyStatus: "applied" };
  } catch {
    return {
      settings,
      applyStatus: "apply_failed",
      message: "生成工具配置已保存，但运行时刷新失败。请重启 Awaken 后再使用该工具。",
    };
  }
}

ipcMain.handle("awaken:relaunch-desktop", () => {
  setImmediate(() => {
    if (isDevServer && process.platform === "darwin") {
      const repositoryRoot = path.resolve(__dirname, "../../../..");
      const helperPath = join(repositoryRoot, "scripts", "relaunch-managed-desktop.mjs");
      const nodeExecutable = process.env.npm_node_execpath || "node";
      const helper = spawn(nodeExecutable, [helperPath, String(process.pid)], {
        cwd: repositoryRoot,
        detached: true,
        stdio: "ignore",
        env: process.env,
      });
      helper.unref();
    } else {
      app.relaunch();
    }
    app.quit();
  });
});

async function resolveCurrentProjectEntry(request: ProjectEntryActionRequest) {
  const projects = await requestRuntime<RuntimeProjectWire[]>(
    "/v1/projects?include_removed=false",
    {
      method: "GET",
    },
  );
  return resolveProjectEntry(projects.map(normalizeRuntimeProject), request);
}

// upsertProjectFolders POSTs the multi-folder create form. Core mints a new
// project identity for it, and an empty folder list yields a project backed by
// an Awaken-managed working directory.
async function requestProjectWrite(
  pathname: string,
  method: "POST" | "PATCH" | "PUT" | "DELETE",
  body: unknown,
  logContext: Record<string, unknown>,
): Promise<UpsertProjectResult> {
  try {
    const project = await requestRuntime<RuntimeProjectWire>(pathname, {
      method,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return { ok: true, project: normalizeRuntimeProject(project) };
  } catch (error) {
    const code = error instanceof RuntimeDomainError ? error.code : "INTERNAL_ERROR";
    console.error("project_folder_write_failed", {
      ...logContext,
      pathname,
      code,
      error: error instanceof Error ? error.message : String(error),
    });
    return { ok: false, code, message: upsertProjectFailureMessage(error) };
  }
}

async function runProjectMutation(
  pathname: string,
  method: "DELETE" | "POST",
  projectId: string,
  idempotencyKey: string,
  body?: unknown,
): Promise<ProjectMutationResult> {
  try {
    const operation = await requestRuntime<RuntimeProjectOperationWire>(pathname, {
      method,
      headers:
        body === undefined
          ? { "Idempotency-Key": idempotencyKey }
          : {
              "Idempotency-Key": idempotencyKey,
              "Content-Type": "application/json",
            },
      body: body === undefined ? undefined : JSON.stringify(body),
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

ipcMain.handle("awaken:pick-directory", async (_event, defaultPath?: string) => {
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
});

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
    return {
      valid: false,
      error: "Workspace directory does not exist or is not accessible",
    };
  }
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
  const systemSkillsTargetRoot = resolveSystemSkillsTargetRoot(app.getPath("home"));
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
      code: error instanceof SystemSkillsSyncError ? error.code : "SYSTEM_SKILLS_SYNC_FAILED",
      sourceRoot: systemSkillsSourceRoot,
      targetRoot: systemSkillsTargetRoot,
      error: error instanceof Error ? error.message : String(error),
    });
  }
  const configDir = await ensureConfigDir();
  memoryDreamSchedulerStore = new MemoryDreamSchedulerStore(configDir);
  await memoryDreamSchedulerStore.initialize();
  desktopPreferencesStore = new DesktopPreferencesStore(configDir);
  await desktopPreferencesStore.initialize();
  nativeTheme.on("updated", () => {
    if (desktopPreferencesStore?.snapshot().theme === "system") {
      updateMainWindowBackground(mainWindowRef, "system", nativeTheme.shouldUseDarkColors);
    }
  });
  const awakenHome = join(app.getPath("home"), ".awaken");
  modelProfileStore = new ModelProfileStore(awakenHome);
  await modelProfileStore.initialize();
  searchSettingsStore = new SearchSettingsStore(configDir);
  networkRouteManager = new NetworkRouteManager({
    platform: process.platform,
    resolveProxy: (url) => session.defaultSession.resolveProxy(url),
    forceReloadProxyConfig: () => session.defaultSession.forceReloadProxyConfig(),
    onSnapshotChanged: (snapshot) => {
      void cliConnectorManager?.refreshAll();
      return synchronizeNetworkRoute(snapshot);
    },
    onError: (error) => {
      console.warn("network_route_refresh_failed", {
        platform: process.platform,
        error: error instanceof Error ? error.message : String(error),
      });
    },
  });
  await networkRouteManager.initialize();
  networkRouteObserver = new NetworkRouteObserver({
    platform: process.platform,
    windowsHelperPath: process.platform === "win32" ? networkRouteObserverBinaryPath() : undefined,
    onChange: (reason) => networkRouteManager?.notifySystemChange(reason),
    onError: (error) => {
      console.warn("network_route_observer_failed", {
        platform: process.platform,
        error: error instanceof Error ? error.message : String(error),
      });
    },
  });
  networkRouteObserver.start();
  networkRoutePowerListener = () => networkRouteManager?.notifySystemChange("power-resume");
  powerMonitor.on("resume", networkRoutePowerListener);
  powerMonitor.on("unlock-screen", networkRoutePowerListener);
  cliConnectorManager = new CliConnectorManager({
    homeDir: app.getPath("home"),
    platform: process.platform,
    architecture: process.arch,
    manifestPath: cliConnectorManifestPath(),
    fetchAsset: (url, signal) => net.fetch(url, { signal }),
    openExternal: (url) => shell.openExternal(url),
    publishReadiness: publishCliReadiness,
    onStateChanged: broadcastCliConnectorState,
    environment: () => networkRouteManager?.childEnvironment(process.env) ?? process.env,
    beforeNetworkOperation: prepareNetworkRoute,
    onTransportFailure: (surface) => networkRouteManager?.notifyTransportFailure(surface),
  });
  await cliConnectorManager.initialize();
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
  registerScheduledTaskIpcHandlers();

  desktopShutdownCoordinator = new DesktopShutdownCoordinator({
    stopSyncOwners: stopSyncShutdownOwners,
    stopAsyncOwners: stopAsyncShutdownOwners,
    finalizeOwners: finalizeShutdownOwners,
  });
  const desktopUpdateBuildChannel = await readDesktopUpdateBuildChannel();
  desktopUpdateManager = new DesktopUpdateManager({
    updater: autoUpdater,
    // 只有安装版稳定身份启用更新器；Awaken Dev、Gallery、ad-hoc 包不访问 GitHub。
    enabled: isStableDesktopUpdateIdentity({
      packaged: app.isPackaged,
      devServer: isDevServer,
      channel: desktopUpdateBuildChannel,
      version: app.getVersion(),
      platform: process.platform,
      arch: process.arch,
    }),
    currentVersion: app.getVersion(),
    platform: process.platform,
    arch: process.arch,
    receiptPath: join(app.getPath("userData"), "desktop-update-receipt.json"),
    fetchActivity: fetchRuntimeActivitySnapshot,
    requestShutdown: (mode) => {
      if (!desktopShutdownCoordinator) {
        return Promise.resolve({ ok: false, reason: "shutdown coordinator unavailable" });
      }
      return desktopShutdownCoordinator.stop(mode);
    },
  });
  desktopUpdateManager.initialize();
  subscribeDesktopUpdateStateBroadcast();
  registerDesktopUpdateIpcHandlers();
  updatePowerResumeListener = () => void desktopUpdateManager?.check("resume");
  powerMonitor.on("resume", updatePowerResumeListener);

  projectFilePreviewManager = new ProjectFilePreviewManager({
    getProjects: async () => {
      const projects = await requestRuntime<RuntimeProjectWire[]>(
        "/v1/projects?include_removed=false",
        {
          method: "GET",
        },
      );
      return projects.map(normalizeRuntimeProject);
    },
    getTranscript: async (threadId) => {
      const transcript = await requestRuntime<RuntimeThreadTranscriptWire>(
        `/v1/threads/${encodeURIComponent(threadId)}/transcript`,
        { method: "GET" },
      );
      return normalizeRuntimeThreadTranscript(transcript);
    },
    runtimePaths: {
      packaged: app.isPackaged,
      resourcesPath: process.resourcesPath,
      appPath: app.getAppPath(),
      cachePath: join(app.getPath("userData"), "cache"),
    },
    defaultSession: session.defaultSession,
    sessionFromPartition: (partition) => session.fromPartition(partition, { cache: false }),
    openExternal: (url) => shell.openExternal(url),
    openPath: (filePath) => shell.openPath(filePath),
    showItemInFolder: (filePath) => shell.showItemInFolder(filePath),
    savePdfAs: async (fileName) => {
      const options = {
        title: "保存 PDF",
        defaultPath: join(app.getPath("downloads"), fileName),
        filters: [{ name: "PDF", extensions: ["pdf"] }],
      };
      const result =
        mainWindowRef && !mainWindowRef.isDestroyed()
          ? await dialog.showSaveDialog(mainWindowRef, options)
          : await dialog.showSaveDialog(options);
      return result.canceled ? undefined : result.filePath;
    },
    onPreviewInteraction: (interaction) => {
      if (!mainWindowRef || mainWindowRef.isDestroyed()) return;
      mainWindowRef.webContents.send(PROJECT_FILE_PREVIEW_INTERACTION_CHANNEL, interaction);
    },
    onGuestHttpNavigate: (payload) => {
      if (!mainWindowRef || mainWindowRef.isDestroyed()) return;
      mainWindowRef.webContents.send(EMBEDDED_BROWSER_CHANNELS.guestHttpNavigate, payload);
    },
  });
  projectFilePreviewManager.initialize();
  embeddedBrowserManager = new EmbeddedBrowserViewManager({
    getWindow: () => mainWindowRef,
    onState: (viewId, threadId, state) => {
      if (!mainWindowRef || mainWindowRef.isDestroyed()) return;
      mainWindowRef.webContents.send(EMBEDDED_BROWSER_CHANNELS.stateChanged, {
        viewId,
        threadId,
        state,
      });
    },
    onRequestShow: (threadId, viewId) => {
      if (!mainWindowRef || mainWindowRef.isDestroyed()) return;
      mainWindowRef.webContents.send(EMBEDDED_BROWSER_CHANNELS.requestShow, {
        threadId,
        viewId,
      });
    },
  });
  embeddedBrowserHost = new EmbeddedBrowserHost(embeddedBrowserManager);
  registerEmbeddedBrowserIpc({
    getWindow: () => mainWindowRef,
    manager: embeddedBrowserManager,
  });
  artifactMediaManager = new ArtifactMediaManager({
    defaultSession: session.defaultSession,
    getTranscript: async (threadId) => {
      const transcript = await requestRuntime<RuntimeThreadTranscriptWire>(
        `/v1/threads/${encodeURIComponent(threadId)}/transcript`,
        { method: "GET" },
      );
      return normalizeRuntimeThreadTranscript(transcript);
    },
  });
  artifactMediaManager.initialize();

  supervisor = new RuntimeSupervisor(
    {},
    {
      resolveProductEnvironment: async () => {
        let searchEnvironment: NodeJS.ProcessEnv = {
          AWAKEN_WEB_SEARCH_PROVIDERS: "",
        };
        try {
          searchEnvironment = (await searchSettingsStore?.runtimeEnvironment()) ?? {
            AWAKEN_WEB_SEARCH_PROVIDERS: "",
          };
        } catch (error) {
          console.error("[awaken-desktop] search settings unavailable", {
            message: error instanceof Error ? error.message : String(error),
          });
        }
        const managedBin = cliConnectorManager?.managedBinPath() ?? "";
        const inheritedPath = process.env.PATH ?? "";
        return {
          ...searchEnvironment,
          ...(networkRouteManager?.runtimeEnvironment() ?? {}),
          AWAKEN_DESKTOP_CONTROL_TOKEN: desktopControlToken,
          ...(process.platform === "win32"
            ? {
                // The CUA N-API module runs inside a DPI-virtualized node.exe,
                // while its SendInput path consumes physical screen pixels.
                // Electron owns the authoritative primary-display scale.
                AWAKEN_CUA_WINDOWS_SCALE_FACTOR: String(
                  electronScreen.getPrimaryDisplay().scaleFactor,
                ),
              }
            : {}),
          PATH: managedBin ? `${managedBin}${path.delimiter}${inheritedPath}` : inheritedPath,
        };
      },
    },
  );
  computerOverlayManager = new ComputerOverlayManager({
    stopComputerUse: async (runId) => {
      const client = supervisor?.getControlClient();
      if (!client) {
        throw new Error("Computer Use 控制服务当前不可用。");
      }
      await client.computerSessionControl(runId, "terminate");
    },
  });
  computerOverlayManager.initialize();
  supervisor.onStatusChange((info) => {
    if (info.runtimeBaseUrl) {
      runtimeBaseUrl = info.runtimeBaseUrl;
      startProjectTrashWorker();
      startScheduledTaskEventStreamIfNeeded();
      startComposerCatalogEventStreamIfNeeded();
      void cliConnectorManager?.refreshAll();
      void synchronizeNetworkRoute();
      void registerEmbeddedBrowserHostWithCore();
    } else if (info.status !== "running") {
      runtimeBaseUrl = null;
      stopScheduledTaskEventStream();
      stopComposerCatalogEventStream();
      stopComputerRunMonitors();
    }
    for (const wc of webContents.getAllWebContents()) {
      if (!wc.isDestroyed()) {
        wc.send("awaken:supervisor-status-changed", toRendererSupervisorStatus(info));
      }
    }
  });
  registerRunsIpc({
    requestRuntimeOK,
    requestRuntime,
    modelProfileStore,
    hostFromBaseURL,
    supervisor,
    runProjectMutation,
    cancelActiveDreams,
    prepareNetworkRoute,
    ensureComputerRunMonitor,
    subscribeToRun,
    subscribeToThreadActivity: (target, threadId, subscriptionId, afterCursor) => subscribeToRun(target, "", subscriptionId, afterCursor, threadId),
    toRendererSupervisorStatus,
  });
  registerExtensionsIpc({
    requestRuntimeOK,
    requestRuntime,
    cliConnectorManager,
    prepareNetworkRoute,
  });
  registerProjectsIpc({
    requestRuntime,
    get mainWindowRef() { return mainWindowRef; },
    projectDirectoryWatchManager,
    resolveCurrentProjectEntry,
    projectFilePreviewManager,
    upsertProjectFailureMessage,
    requestProjectWrite,
    runProjectMutation,
    processPendingProjectTrashOperations,
  });
  registerPluginsIpc({
    requestRuntime,
  });
  registerModelsIpc({
    requestRuntime,
    modelProfileStore,
    hasAgentPlanMediaToolReferences,
    ensureNoActiveRunsForSettingsChange,
    restartCoreForAgentPlanTools,
    mutateAgentPlanMediaTools,
    prepareNetworkRoute,
    searchSettingsStore,
    supervisor,
  });
  registerFilesIpc({
    requestRuntime,
    get mainWindowRef() { return mainWindowRef; },
    projectFilePreviewManager,
    artifactMediaManager,
  });
  registerComputerIpc({
    supervisor,
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
      startScheduledTaskEventStreamIfNeeded();
      startComposerCatalogEventStreamIfNeeded();
      void cliConnectorManager?.refreshAll();
      void registerEmbeddedBrowserHostWithCore();
    }
  });
  memoryDreamSchedulerTimer = setInterval(() => void runAutomaticDreamIfEligible(), 60_000);

  app.on("activate", () => {
    networkRouteManager?.notifySystemChange("app-activate");
    void desktopUpdateManager?.check("activate");
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

/** 退出清理的同步 owner：与 before-quit 原顺序一致，供 shutdown coordinator 调用。 */
function stopSyncShutdownOwners(): void {
  stopProjectTrashWorker();
  stopScheduledTaskEventStream();
  stopComposerCatalogEventStream();
  stopComputerRunMonitors();
  if (memoryDreamSchedulerTimer) clearInterval(memoryDreamSchedulerTimer);
  memoryDreamSchedulerTimer = null;
  for (const subscription of subscriptions.values()) {
    subscription.controller.abort();
  }
  subscriptions.clear();
  projectDirectoryWatchManager.dispose();
  projectFilePreviewManager?.dispose();
  projectFilePreviewManager = null;
  embeddedBrowserManager?.dispose();
  embeddedBrowserManager = null;
  artifactMediaManager?.dispose();
  artifactMediaManager = null;
  if (networkRoutePowerListener) {
    powerMonitor.removeListener("resume", networkRoutePowerListener);
    powerMonitor.removeListener("unlock-screen", networkRoutePowerListener);
  }
  networkRoutePowerListener = null;
  networkRouteObserver?.dispose();
  networkRouteObserver = null;
  networkRouteManager?.dispose();
  networkRouteManager = null;
}

/** 只在全部 owner 已成功停止后销毁 updater/scheduler；失败时仍可向窗口回报并重试。 */
function finalizeShutdownOwners(): void {
  stopDesktopUpdateScheduling();
  desktopUpdateManager?.dispose();
  desktopUpdateManager = null;
}

/** 退出清理的异步 owner：supervisor/Core、CLI connector、pet/computer overlay。 */
async function stopAsyncShutdownOwners(): Promise<void> {
  if (supervisor) {
    await supervisor.stop();
  }
  await embeddedBrowserHost?.stop();
  embeddedBrowserHost = null;
  await cliConnectorManager?.dispose();
  await petWindowManager?.dispose();
  await computerOverlayManager?.dispose();
}

app.on("before-quit", (event) => {
  if (isQuitting) return;
  if (desktopShutdownCoordinator?.updateInstallReady) {
    // update-install 清理已完成，放行让 quitAndInstall 的安装器接管；
    // before-quit-for-update 不再发起第二次清理，也不调用 app.exit()。
    isQuitting = true;
    return;
  }
  event.preventDefault();
  isQuitting = true;
  const coordinator = desktopShutdownCoordinator;
  if (!coordinator) {
    app.exit();
    return;
  }
  void coordinator.stop("normal").then((result) => {
    if (!result.ok) {
      console.error("[awaken-desktop] shutdown cleanup failed", { reason: result.reason });
    }
    if (coordinator.updateInstallReady) {
      // 清理期间已被更新安装路径接管，普通退出不得抢跑安装器。
      return;
    }
    app.exit();
  });
});

process.on("uncaughtException", (error) => {
  console.error("[awaken-desktop] uncaughtException", error);
});

process.on("unhandledRejection", (error) => {
  console.error("[awaken-desktop] unhandledRejection", error);
});

console.info(`[awaken-desktop] platform=${toDesktopPlatform(process.platform)}`);
