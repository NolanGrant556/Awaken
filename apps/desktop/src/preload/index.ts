import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import type { AguiEvent } from "../shared/agui.js";
import {
  type ApprovalDecision,
  type AwakenBridge,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type DesktopPlatform,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type ModelProfileSaveRequest,
  type ModelProfileTestResult,
  type ModelProvider,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RunStreamHandlers,
  type RuntimeInfo,
  type RuntimeMcpServer,
  type RuntimeModelProfileSummary,
  type RuntimeRun,
  type RuntimeSkill,
  type RuntimeThreadTranscript,
  type RuntimeThread,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type WorkspacePickResult,
  type WorkspaceRegistrySyncRequest,
  type WorkspaceValidationResult,
} from "../shared/runtime.js";

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
  getRun: (runId: string) =>
    ipcRenderer.invoke("awaken:get-run", runId) as Promise<RuntimeRun>,
  getThreadTranscript: (threadId: string) =>
    ipcRenderer.invoke("awaken:get-thread-transcript", threadId) as Promise<RuntimeThreadTranscript>,
  listThreads: () =>
    ipcRenderer.invoke("awaken:list-threads") as Promise<RuntimeThread[]>,
  renameThread: (threadId: string, label: string) =>
    ipcRenderer.invoke("awaken:rename-thread", threadId, label) as Promise<RuntimeThread>,
  deleteThread: (threadId: string) =>
    ipcRenderer.invoke("awaken:delete-thread", threadId) as Promise<void>,
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
  listConnectors: () =>
    ipcRenderer.invoke("awaken:list-connectors") as Promise<RuntimeConnector[]>,
  connectConnector: (connectorId: string) =>
    ipcRenderer.invoke("awaken:connect-connector", connectorId) as Promise<RuntimeConnectorActionResponse>,
  disconnectConnector: (connectorId: string) =>
    ipcRenderer.invoke("awaken:disconnect-connector", connectorId) as Promise<RuntimeConnectorActionResponse>,
  listMcpServers: () =>
    ipcRenderer.invoke("awaken:list-mcp-servers") as Promise<RuntimeMcpServer[]>,
  configureMcpServers: (request: ConfigureMcpServersRequest) =>
    ipcRenderer.invoke("awaken:configure-mcp-servers", request) as Promise<ConfigureMcpServersResponse>,
  listSkills: () =>
    ipcRenderer.invoke("awaken:list-skills") as Promise<RuntimeSkill[]>,
  installSkill: (request: InstallSkillRequest) =>
    ipcRenderer.invoke("awaken:install-skill", request) as Promise<InstallSkillResponse>,
  listModelProviders: () =>
    ipcRenderer.invoke("awaken:list-model-providers") as Promise<ModelProvider[]>,
  listModelProfiles: () =>
    ipcRenderer.invoke("awaken:list-model-profiles") as Promise<RuntimeModelProfileSummary[]>,
  saveModelProfile: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:save-model-profile", request) as Promise<RuntimeModelProfileSummary>,
  deleteModelProfile: (profileId: string) =>
    ipcRenderer.invoke("awaken:delete-model-profile", profileId) as Promise<void>,
  setDefaultModelProfile: (profileId: string) =>
    ipcRenderer.invoke("awaken:set-default-model-profile", profileId) as Promise<RuntimeModelProfileSummary>,
  testModelProfile: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:test-model-profile", request) as Promise<ModelProfileTestResult>,
  pickWorkspace: (defaultPath?: string) =>
    ipcRenderer.invoke("awaken:pick-workspace", defaultPath) as Promise<WorkspacePickResult>,
  validateWorkspace: (path: string) =>
    ipcRenderer.invoke("awaken:validate-workspace", path) as Promise<WorkspaceValidationResult>,
  syncWorkspaceRegistry: (request: WorkspaceRegistrySyncRequest) =>
    ipcRenderer.invoke("awaken:sync-workspace-registry", request) as Promise<void>,
  getSupervisorStatus: () =>
    ipcRenderer.invoke("awaken:runtime-supervisor-status") as Promise<SupervisorStatusInfo>,
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => {
    const handler = (_event: IpcRendererEvent, info: SupervisorStatusInfo) => callback(info);
    ipcRenderer.on("awaken:supervisor-status-changed", handler);
    return () => ipcRenderer.removeListener("awaken:supervisor-status-changed", handler);
  },
  notifyRendererReady: () => {
    ipcRenderer.send("awaken:renderer-ready");
  },
};

contextBridge.exposeInMainWorld("awaken", bridge);
