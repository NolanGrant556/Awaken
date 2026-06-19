import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import type { AguiEvent } from "../shared/agui.js";
import {
  type ApprovalDecision,
  type AwakenBridge,
  type DesktopPlatform,
  type RunStreamHandlers,
  type RuntimeInfo,
  type RuntimeLogEntry,
  type RuntimeRun,
  type RuntimeThread,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type WorkspacePickResult,
  type WorkspaceValidationResult,
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
  getRun: (runId: string) =>
    ipcRenderer.invoke("awaken:get-run", runId) as Promise<RuntimeRun>,
  listThreads: () =>
    ipcRenderer.invoke("awaken:list-threads") as Promise<RuntimeThread[]>,
  startRun: (request: StartRunRequest) =>
    ipcRenderer.invoke("awaken:start-run", request) as Promise<RuntimeRun>,
  cancelRun: (runId: string) =>
    ipcRenderer.invoke("awaken:cancel-run", runId) as Promise<RuntimeRun>,
  resolveApproval: (runId: string, approvalId: string, decision: ApprovalDecision) =>
    ipcRenderer.invoke("awaken:resolve-approval", {
      runId,
      approvalId,
      decision,
    }) as Promise<void>,
  subscribeRun: async (runId: string, handlers: RunStreamHandlers, afterSeq?: number) => {
    const subscriptionId = crypto.randomUUID();
    subscriptions.set(subscriptionId, handlers);

    try {
      await ipcRenderer.invoke("awaken:subscribe-run", {
        runId,
        subscriptionId,
        afterSeq,
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
  getRuntimeInfo: () =>
    ipcRenderer.invoke("awaken:get-runtime-info") as Promise<RuntimeInfo>,
  pickWorkspace: (defaultPath?: string) =>
    ipcRenderer.invoke("awaken:pick-workspace", defaultPath) as Promise<WorkspacePickResult>,
  validateWorkspace: (path: string) =>
    ipcRenderer.invoke("awaken:validate-workspace", path) as Promise<WorkspaceValidationResult>,
  getRuntimeLogs: () =>
    ipcRenderer.invoke("awaken:get-runtime-logs") as Promise<RuntimeLogEntry[]>,
  restartRuntime: () =>
    ipcRenderer.invoke("awaken:restart-runtime") as Promise<SupervisorStatusInfo>,
  getSupervisorStatus: () =>
    ipcRenderer.invoke("awaken:runtime-supervisor-status") as Promise<SupervisorStatusInfo>,
  onRuntimeLogs: (callback: (entry: RuntimeLogEntry) => void) => {
    const handler = (_event: IpcRendererEvent, entry: RuntimeLogEntry) => callback(entry);
    ipcRenderer.on("awaken:runtime-logs", handler);
    return () => ipcRenderer.removeListener("awaken:runtime-logs", handler);
  },
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => {
    const handler = (_event: IpcRendererEvent, info: SupervisorStatusInfo) => callback(info);
    ipcRenderer.on("awaken:supervisor-status-changed", handler);
    return () => ipcRenderer.removeListener("awaken:supervisor-status-changed", handler);
  },
};

contextBridge.exposeInMainWorld("awaken", bridge);
