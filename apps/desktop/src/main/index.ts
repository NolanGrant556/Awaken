import { randomUUID } from "node:crypto";
import { TextDecoder } from "node:util";
import {
  app,
  BrowserWindow,
  dialog,
  ipcMain,
  shell,
  webContents,
  type WebContents,
} from "electron";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import fs from "node:fs/promises";
import path from "node:path";
import type { AguiEvent } from "../shared/agui.js";
import {
  normalizeRuntimeInfo,
  normalizeRuntimeRun,
  normalizeRuntimeThread,
  normalizeStartRunRequest,
  type ApprovalDecision,
  type DesktopPlatform,
  type RuntimeInfoWire,
  type RuntimeRunWire,
  type RuntimeThreadWire,
  type StartRunRequest,
} from "../shared/runtime.js";
import { RuntimeSupervisor, type SupervisorStatusInfo } from "./runtime-supervisor.js";
import { ensureConfigDir } from "./config-bootstrap.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
let runtimeBaseUrl: string | null = null;
const isDevServer = process.argv.includes("--dev");

let supervisor: RuntimeSupervisor | null = null;
let isQuitting = false;

type StreamSubscription = {
  controller: AbortController;
  webContentsId: number;
};

const subscriptions = new Map<string, StreamSubscription>();

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
  const mainWindow = new BrowserWindow({
    width: 1440,
    height: 960,
    minWidth: 1120,
    minHeight: 760,
    backgroundColor: "#f8f8f7",
    autoHideMenuBar: true,
    title: "Awaken Desktop",
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
  mainWindow.on("closed", () => {
    cleanupWindowSubscriptions(webContentsId);
  });

  if (isDevServer) {
    void mainWindow.loadURL("http://127.0.0.1:5174");
  } else {
    void mainWindow.loadFile(join(__dirname, "../renderer/index.html"));
  }

  return mainWindow;
}

ipcMain.handle("awaken:get-runtime-info", async () => {
  try {
    const info = await requestRuntime<RuntimeInfoWire>("/v1/runtime/info", {
      method: "GET",
    });

    return normalizeRuntimeInfo(info);
  } catch {
    console.error("[awaken-desktop] failed to get runtime info");
    throw new Error("Runtime unavailable");
  }
});

ipcMain.handle("awaken:get-run", async (_event, runId: string) => {
  const run = await requestRuntime(`/v1/runs/${runId}`, {
    method: "GET",
  });

	return normalizeRuntimeRun(run);
});

ipcMain.handle("awaken:list-threads", async () => {
	const threads = await requestRuntime<RuntimeThreadWire[]>("/v1/threads", {
		method: "GET",
	});

	return threads.map(normalizeRuntimeThread);
});

ipcMain.handle("awaken:start-run", async (_event, request: StartRunRequest) => {
  const run = await requestRuntime("/v1/runs", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(normalizeStartRunRequest(request)),
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
  "awaken:resolve-approval",
  async (_event, payload: { runId: string; approvalId: string; decision: ApprovalDecision }) => {
    await requestRuntimeOK(`/v1/runs/${payload.runId}/approvals/${payload.approvalId}/resolve`, {
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

// Persist the last user-selected workspace so the supervisor can default to it
// on the next launch instead of always falling back to the home directory.
async function readLastWorkspace(): Promise<string | undefined> {
  try {
    const file = path.join(app.getPath("userData"), "last-workspace.json");
    const raw = await fs.readFile(file, "utf8");
    const parsed = JSON.parse(raw) as { path?: unknown };
    const value = typeof parsed.path === "string" ? parsed.path : "";
    if (!path.isAbsolute(value)) {
      return undefined;
    }
    return value;
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

app.whenReady().then(async () => {
  const configDir = await ensureConfigDir();
  const lastWorkspace = await readLastWorkspace();
  supervisor = new RuntimeSupervisor({
    configDir,
    defaultWorkspace: lastWorkspace ?? app.getPath("home"),
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
