import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import type { AguiEvent } from "../shared/agui.js";
import {
  type AttachmentPickResult,
  type AwakenBridge,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type DesktopPlatform,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type MassMessageReviewDecision,
  type ModelProfileBalancesResult,
  type ModelProfileSaveRequest,
  type ModelProfileTestResult,
  type ModelProvider,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RunStreamHandlers,
  type RuntimeInfo,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeModelProfileSummary,
  type RuntimeRun,
  type RuntimeSkill,
  type RuntimeThreadTranscript,
  type RuntimeThread,
  type RuntimeToolInfo,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type UpdateMcpServerConfigRequest,
  type WorkspacePickResult,
  type WorkspaceRegistrySnapshot,
  type WorkspaceRegistryPersistRequest,
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
  resolveMassMessageReview: (runId: string, reviewId: string, decision: MassMessageReviewDecision) =>
    ipcRenderer.invoke("awaken:resolve-mass-message-review", {
      runId,
      reviewId,
      decision,
    }) as Promise<void>,
  answerUserQuestion: (runId: string, questionId: string, answer: string) =>
    ipcRenderer.invoke("awaken:answer-user-question", {
      runId,
      questionId,
      answer,
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
  getMcpServerConfig: (name: string) =>
    ipcRenderer.invoke("awaken:get-mcp-server-config", name) as Promise<RuntimeMcpServerConfigResponse>,
  updateMcpServerConfig: (name: string, request: UpdateMcpServerConfigRequest) =>
    ipcRenderer.invoke("awaken:update-mcp-server-config", name, request) as Promise<ConfigureMcpServersResponse>,
  getRuntimeLogs: () =>
    ipcRenderer.invoke("awaken:get-runtime-logs") as Promise<RuntimeLogEntry[]>,
  disableMcpServer: (name: string) =>
    ipcRenderer.invoke("awaken:disable-mcp-server", name) as Promise<void>,
  enableMcpServer: (name: string) =>
    ipcRenderer.invoke("awaken:enable-mcp-server", name) as Promise<void>,
  deleteMcpServer: (name: string) =>
    ipcRenderer.invoke("awaken:delete-mcp-server", name) as Promise<void>,
  listMcpTools: () =>
    ipcRenderer.invoke("awaken:list-mcp-tools") as Promise<RuntimeToolInfo[]>,
  listSkills: (projectRoot?: string) =>
    ipcRenderer.invoke("awaken:list-skills", projectRoot) as Promise<RuntimeSkill[]>,
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
  syncModelCapabilities: () =>
    ipcRenderer.invoke("awaken:sync-model-capabilities") as Promise<ModelProvider[]>,
  getModelProfileBalances: () =>
    ipcRenderer.invoke("awaken:get-model-profile-balances") as Promise<ModelProfileBalancesResult>,
  pickAttachments: (defaultPath?: string) =>
    ipcRenderer.invoke("awaken:pick-attachments", defaultPath) as Promise<AttachmentPickResult>,
  pickDirectory: (defaultPath?: string) =>
    ipcRenderer.invoke("awaken:pick-directory", defaultPath) as Promise<WorkspacePickResult>,
  pickWorkspace: (defaultPath?: string) =>
    ipcRenderer.invoke("awaken:pick-workspace", defaultPath) as Promise<WorkspacePickResult>,
  validateWorkspace: (path: string) =>
    ipcRenderer.invoke("awaken:validate-workspace", path) as Promise<WorkspaceValidationResult>,
  readWorkspaceRegistry: () =>
    ipcRenderer.invoke("awaken:read-workspace-registry") as Promise<WorkspaceRegistrySnapshot>,
  persistWorkspaceRegistry: (request: WorkspaceRegistryPersistRequest) =>
    ipcRenderer.invoke("awaken:persist-workspace-registry", request) as Promise<void>,
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
