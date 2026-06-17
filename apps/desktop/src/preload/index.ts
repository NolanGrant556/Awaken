import { contextBridge, ipcRenderer } from "electron";
import type { AguiEvent } from "../shared/agui.js";
import {
  type AwakenBridge,
  type DesktopPlatform,
  type RunStreamHandlers,
  type RuntimeRun,
} from "../shared/runtime.js";

const runtimeBaseUrl = process.env.AWAKEN_DESKTOP_RUNTIME_URL?.trim() || "http://127.0.0.1:8787";
const subscriptions = new Map<string, RunStreamHandlers>();

function toDesktopPlatform(platform: string): DesktopPlatform {
  switch (platform) {
    case "darwin":
      return "macos";
    case "win32":
      return "windows";
    default:
      return "linux";
  }
}

ipcRenderer.on(
  "awaken:run-event",
  (_event, payload: { subscriptionId: string; event: AguiEvent }) => {
    subscriptions.get(payload.subscriptionId)?.onEvent(payload.event);
  },
);

ipcRenderer.on(
  "awaken:run-error",
  (_event, payload: { subscriptionId: string; message: string }) => {
    subscriptions.get(payload.subscriptionId)?.onError?.(payload.message);
  },
);

ipcRenderer.on("awaken:run-closed", (_event, payload: { subscriptionId: string }) => {
  const handlers = subscriptions.get(payload.subscriptionId);
  if (!handlers) {
    return;
  }
  subscriptions.delete(payload.subscriptionId);
  handlers.onClose?.();
});

const bridge: AwakenBridge = {
  platform: toDesktopPlatform(process.platform),
  runtimeBaseUrl,
  createRun: (input: string) =>
    ipcRenderer.invoke("awaken:create-run", input) as Promise<RuntimeRun>,
  cancelRun: (runId: string) =>
    ipcRenderer.invoke("awaken:cancel-run", runId) as Promise<RuntimeRun>,
  subscribeRun: async (runId: string, handlers: RunStreamHandlers) => {
    const subscriptionId = crypto.randomUUID();
    subscriptions.set(subscriptionId, handlers);

    try {
      await ipcRenderer.invoke("awaken:subscribe-run", {
        runId,
        subscriptionId,
      });
      return subscriptionId;
    } catch (error) {
      subscriptions.delete(subscriptionId);
      throw error;
    }
  },
  unsubscribeRun: (subscriptionId: string) => {
    subscriptions.delete(subscriptionId);
    ipcRenderer.send("awaken:unsubscribe-run", subscriptionId);
  },
};

contextBridge.exposeInMainWorld("awaken", bridge);
