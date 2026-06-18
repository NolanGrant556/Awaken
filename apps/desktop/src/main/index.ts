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
  normalizeStartRunRequest,
  type ApprovalDecision,
  type DesktopPlatform,
  type RuntimeInfoWire,
  type RuntimeRunWire,
  type StartRunRequest,
} from "../shared/runtime.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const runtimeBaseUrl = process.env.AWAKEN_DESKTOP_RUNTIME_URL?.trim() || "http://127.0.0.1:8787";
const isDevServer = process.argv.includes("--dev");

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
  return new URL(pathname, runtimeBaseUrl.endsWith("/") ? runtimeBaseUrl : `${runtimeBaseUrl}/`).toString();
}

async function readErrorMessage(response: Response): Promise<string> {
  const fallback = `Runtime request failed (${response.status})`;
  const body = (await response.text()).trim();
  if (!body) return fallback;
  try {
    const parsed = JSON.parse(body);
    if (parsed.code && parsed.message) {
      return `${parsed.code}: ${parsed.message}`;
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
    subscriptions.delete(subscriptionId);
    sendToRenderer("awaken:run-closed", { subscriptionId }, targetId);
  }
}

async function subscribeToRun(
  target: WebContents,
  runId: string,
  subscriptionId: string,
): Promise<void> {
  const controller = new AbortController();
  const response = await fetch(runtimeUrl(`/v1/runs/${runId}/events`), {
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

    subscriptions.delete(subscriptionId);
    sendToRenderer(
      "awaken:run-error",
      {
        subscriptionId,
        message: error instanceof Error ? error.message : "Runtime stream failed",
      },
      target.id,
    );
    sendToRenderer("awaken:run-closed", { subscriptionId }, target.id);
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

  mainWindow.on("closed", () => {
    cleanupWindowSubscriptions(mainWindow.webContents.id);
  });

  if (isDevServer) {
    void mainWindow.loadURL("http://127.0.0.1:5174");
    mainWindow.webContents.openDevTools({ mode: "detach" });
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
  async (event, payload: { runId: string; subscriptionId: string }) => {
    await subscribeToRun(event.sender, payload.runId, payload.subscriptionId);
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

app.whenReady().then(() => {
  createMainWindow();

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

app.on("before-quit", () => {
  for (const subscription of subscriptions.values()) {
    subscription.controller.abort();
  }
  subscriptions.clear();
});

process.on("uncaughtException", (error) => {
  console.error("[awaken-desktop] uncaughtException", error);
});

process.on("unhandledRejection", (error) => {
  console.error("[awaken-desktop] unhandledRejection", error);
});

console.info(
  `[awaken-desktop] platform=${toDesktopPlatform(process.platform)} runtime=${runtimeBaseUrl}`,
);
