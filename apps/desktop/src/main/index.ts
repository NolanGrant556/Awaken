import { createHash, randomUUID } from "node:crypto";
import { TextDecoder } from "node:util";
import {
  app,
  BrowserWindow,
  dialog,
  ipcMain,
  nativeImage,
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
  normalizeRuntimeInfo,
  normalizeRuntimeRun,
  normalizeRuntimeThreadTranscript,
  normalizeRuntimeThread,
  normalizeStartRunRequest,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type DesktopPlatform,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type MassMessageReviewDecision,
  type ModelProfileBalancesResultWire,
  type ModelProfileSaveRequest,
  type ModelProviderWire,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RuntimeInfoWire,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeRunWire,
  type RuntimeSkill,
  type RuntimeModelProfileSummaryWire,
  type RenameThreadRequest,
  type RuntimeThreadTranscriptWire,
  type RuntimeThreadWire,
  type RuntimeToolInfo,
  type StartRunRequest,
  type UpdateMcpServerConfigRequest,
  type WorkspaceRegistryPersistRequest,
} from "../shared/runtime.js";
import { RuntimeSupervisor, type SupervisorStatusInfo } from "./runtime-supervisor.js";
import { ensureConfigDir } from "./config-bootstrap.js";
import { ModelProfileStore } from "./model-profile-store.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
let runtimeBaseUrl: string | null = null;
const isDevServer = process.argv.includes("--dev");
const appName = "Awaken";

app.setName(appName);
app.setPath("userData", join(app.getPath("appData"), "@awaken", "desktop"));

let supervisor: RuntimeSupervisor | null = null;
let modelProfileStore: ModelProfileStore | null = null;
let isQuitting = false;

type StreamSubscription = {
  controller: AbortController;
  webContentsId: number;
};

const subscriptions = new Map<string, StreamSubscription>();

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

function loadDesktopIcon() {
  const icon = nativeImage.createFromPath(desktopIconPath());
  return icon.isEmpty() ? undefined : icon;
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
  afterSeq?: number,
): Promise<void> {
  const controller = new AbortController();
  const eventsUrl = afterSeq
    ? `/v1/runs/${runId}/events?after_seq=${afterSeq}`
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
  const icon = loadDesktopIcon();
  const mainWindow = new BrowserWindow({
    width: defaultWindowWidth,
    height: defaultWindowHeight,
    minWidth: minimumWindowWidth,
    minHeight: minimumWindowHeight,
    backgroundColor: "#f8f8f7",
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
  }

  ipcMain.on("awaken:renderer-ready", handleRendererReady);

  mainWindow.on("closed", () => {
    ipcMain.removeListener("awaken:renderer-ready", handleRendererReady);
    if (fallbackShowTimer) {
      clearTimeout(fallbackShowTimer);
      fallbackShowTimer = undefined;
    }
    cleanupWindowSubscriptions(webContentsId);
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
	return requestRuntime<RuntimeConnector[]>("/v1/connectors", {
		method: "GET",
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

ipcMain.handle("awaken:list-skills", async () => {
	return requestRuntime<RuntimeSkill[]>("/v1/skills", {
		method: "GET",
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
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  return normalizeModelProfileSummary(await modelProfileStore.saveProfile(request));
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
  if (!modelProfileStore) {
    throw new Error("Model profile store unavailable");
  }
  return modelProfileStore.testProfile(request);
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

ipcMain.handle("awaken:delete-thread", async (_event, threadId: string) => {
  await requestRuntimeOK(`/v1/threads/${threadId}`, {
    method: "DELETE",
  });
});

ipcMain.handle("awaken:start-run", async (_event, request: StartRunRequest) => {
  const body = normalizeStartRunRequest(request);
  const defaultProfile = await modelProfileStore?.resolveDefaultRunProfile();
  if (!defaultProfile) {
    throw new Error("Configure a model in Settings > Models before running Awaken");
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

ipcMain.handle("awaken:cancel-run", async (_event, runId: string) => {
  const run = await requestRuntime(`/v1/runs/${runId}/cancel`, {
    method: "POST",
  });

  return normalizeRuntimeRun(run);
});

ipcMain.handle(
  "awaken:resolve-mass-message-review",
  async (_event, payload: { runId: string; reviewId: string; decision: MassMessageReviewDecision }) => {
    await requestRuntimeOK(`/v1/runs/${payload.runId}/mass-message-reviews/${payload.reviewId}/resolve`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ decision: payload.decision }),
    });
  },
);

ipcMain.handle(
  "awaken:subscribe-run",
  async (event, payload: { runId: string; subscriptionId: string; afterSeq?: number }) => {
    await subscribeToRun(event.sender, payload.runId, payload.subscriptionId, payload.afterSeq);
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
      await writeLastWorkspace(selected);
      const registry = await readWorkspaceRegistry(selected);
      await writeWorkspaceRegistry(registry);
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

ipcMain.handle("awaken:read-workspace-registry", async () => {
  const lastWorkspacePath = await readLastWorkspace();
  const registeredWorkspacePaths = await readWorkspaceRegistry(lastWorkspacePath);
  return {
    lastWorkspacePath,
    registeredWorkspacePaths,
  };
});

ipcMain.handle(
  "awaken:persist-workspace-registry",
  async (_event, request: WorkspaceRegistryPersistRequest) => {
    const defaultWorkspace = normalizeStoredWorkspacePath(
      request.defaultWorkspacePath,
    );
    const workspaces = dedupeWorkspacePaths([
      defaultWorkspace,
      ...request.registeredWorkspacePaths,
    ]);

    await writeWorkspaceRegistry(workspaces);
    const nextLastWorkspace = defaultWorkspace ?? workspaces[0];
    if (nextLastWorkspace) {
      await writeLastWorkspace(nextLastWorkspace);
    } else {
      await clearLastWorkspace();
    }
  },
);

ipcMain.handle("awaken:runtime-supervisor-status", async () => {
  return toRendererSupervisorStatus(supervisor?.getStatus() ?? { status: "stopped" });
});

function normalizeStoredWorkspacePath(candidate: unknown): string | null {
  const value = typeof candidate === "string" ? candidate.trim() : "";
  if (!path.isAbsolute(value)) {
    return null;
  }
  return path.normalize(value);
}

function dedupeWorkspacePaths(paths: Iterable<unknown>): string[] {
  const nextPaths: string[] = [];
  for (const value of paths) {
    const normalized = normalizeStoredWorkspacePath(value);
    if (!normalized || nextPaths.includes(normalized)) {
      continue;
    }
    nextPaths.push(normalized);
  }
  return nextPaths;
}

// Persist the last user-selected workspace so the supervisor can default to it
// on the next launch instead of always falling back to the home directory.
async function readLastWorkspace(): Promise<string | undefined> {
  try {
    const file = path.join(app.getPath("userData"), "last-workspace.json");
    const raw = await fs.readFile(file, "utf8");
    const parsed = JSON.parse(raw) as { path?: unknown };
    return normalizeStoredWorkspacePath(parsed.path) ?? undefined;
  } catch {
    return undefined;
  }
}

async function writeLastWorkspace(workspacePath: string): Promise<void> {
  try {
    const file = path.join(app.getPath("userData"), "last-workspace.json");
    await fs.writeFile(file, JSON.stringify({ path: workspacePath }), "utf8");
  } catch {
    // Persistence is best-effort; a write failure must not block the picker.
  }
}

async function clearLastWorkspace(): Promise<void> {
  try {
    const file = path.join(app.getPath("userData"), "last-workspace.json");
    await fs.rm(file, { force: true });
  } catch {
    // Persistence is best-effort; a write failure must not block the picker.
  }
}

async function readWorkspaceRegistry(
  seedWorkspace?: string,
): Promise<string[]> {
  try {
    const file = path.join(app.getPath("userData"), "workspace-registry.json");
    const raw = await fs.readFile(file, "utf8");
    const parsed = JSON.parse(raw) as { workspaces?: unknown };
    const storedWorkspaces = Array.isArray(parsed.workspaces)
      ? parsed.workspaces
      : [];
    return dedupeWorkspacePaths([
      seedWorkspace,
      ...storedWorkspaces,
    ]);
  } catch {
    return dedupeWorkspacePaths([seedWorkspace]);
  }
}

async function writeWorkspaceRegistry(workspaces: string[]): Promise<void> {
  try {
    const file = path.join(app.getPath("userData"), "workspace-registry.json");
    const normalized = dedupeWorkspacePaths(workspaces);
    await fs.writeFile(
      file,
      JSON.stringify({ workspaces: normalized }),
      "utf8",
    );
  } catch {
    // Persistence is best-effort; a write failure must not block the UI.
  }
}

async function ensureDesktopDataDirs(): Promise<void> {
  await Promise.all([
    fs.mkdir(path.join(app.getPath("userData"), "config"), { recursive: true }),
    fs.mkdir(path.join(app.getPath("userData"), "data", "threads"), { recursive: true }),
    fs.mkdir(path.join(app.getPath("userData"), "data", "auth"), { recursive: true }),
    fs.mkdir(path.join(app.getPath("userData"), "data", "migrations"), { recursive: true }),
  ]);
}

function workspaceMigrationMarkerPath(workspacePath: string): string {
  const digest = createHash("sha256").update(workspacePath).digest("hex").slice(0, 16);
  return path.join(
    app.getPath("userData"),
    "data",
    "migrations",
    `workspace-transcripts-${digest}.json`,
  );
}

async function trashLegacyWorkspaceTranscripts(workspaces: Iterable<unknown>): Promise<void> {
  for (const workspacePath of dedupeWorkspacePaths(workspaces)) {
    const markerPath = workspaceMigrationMarkerPath(workspacePath);
    try {
      await fs.access(markerPath);
      continue;
    } catch {
      // Marker missing; continue.
    }

    const transcriptsDir = path.join(workspacePath, ".awaken", "transcripts");
    let exists = false;
    try {
      const stat = await fs.stat(transcriptsDir);
      exists = stat.isDirectory();
    } catch {
      exists = false;
    }

    if (exists) {
      try {
        await shell.trashItem(transcriptsDir);
      } catch (error) {
        console.warn("[awaken-desktop] failed to trash legacy transcripts", {
          workspacePath,
          transcriptsDir,
          error,
        });
        continue;
      }
    }

    const marker = {
      workspacePath,
      transcriptsDir,
      trashed: exists,
      migratedAt: new Date().toISOString(),
    };
    try {
      await fs.writeFile(markerPath, `${JSON.stringify(marker, null, 2)}\n`, "utf8");
    } catch {
      // Migration already completed; marker persistence is best-effort.
    }
  }
}

app.whenReady().then(async () => {
  setMacDockIcon();
  await ensureDesktopDataDirs();
  const configDir = await ensureConfigDir();
  modelProfileStore = new ModelProfileStore(configDir);
  await modelProfileStore.initialize();
  const lastWorkspace = await readLastWorkspace();
  const workspaceRegistry = await readWorkspaceRegistry(lastWorkspace);
  const defaultWorkspace = lastWorkspace ?? workspaceRegistry[0] ?? app.getPath("home");
  await trashLegacyWorkspaceTranscripts([
    defaultWorkspace,
    lastWorkspace,
    ...workspaceRegistry,
  ]);
  supervisor = new RuntimeSupervisor({
    defaultWorkspace,
  });
  supervisor.onStatusChange((info) => {
    if (info.runtimeBaseUrl) {
      runtimeBaseUrl = info.runtimeBaseUrl;
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
  void supervisor.start().then((statusInfo) => {
    if (statusInfo.runtimeBaseUrl) {
      runtimeBaseUrl = statusInfo.runtimeBaseUrl;
    }
  });

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createMainWindow();
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
  for (const subscription of subscriptions.values()) {
    subscription.controller.abort();
  }
  subscriptions.clear();
  void (async () => {
    try {
      if (supervisor) {
        await supervisor.stop();
      }
    } catch (err) {
      console.error("[awaken-desktop] supervisor stop failed", err);
    }
    app.quit();
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
