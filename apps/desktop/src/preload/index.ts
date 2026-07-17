import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import type { AguiEvent } from "../shared/agui.js";
import {
  type AttachmentPickResult,
  type ArtifactOpenRequest,
  type ArtifactOpenResult,
  type AwakenBridge,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type DesktopPreferences,
  type DesktopPreferencesSaveRequest,
  type DesktopOperationResult,
  type DesktopPlatform,
  type DeleteSkillRequest,
  type DeleteSkillResponse,
  type GetSkillPreviewRequest,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type ModelProfileBalancesResult,
  type ModelProfileSaveRequest,
  type ModelProfileTestResult,
  type ModelProvider,
  type ProjectMutationResult,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RunStreamHandlers,
  type RuntimeInfo,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeModelProfileSummary,
  type RuntimeRun,
  type RuntimeProject,
  type RuntimeProjectOperation,
  type RuntimeSkill,
  type SkillPreviewResponse,
  type RuntimeThreadTranscript,
  type RuntimeThread,
  type RuntimeToolInfo,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type ThreadDeleteResult,
  type UpdateMcpServerConfigRequest,
  type UpsertProjectResult,
  type WorkspacePickResult,
  type WorkspaceValidationResult,
} from "../shared/runtime.js";
import {
  PET_IPC_CHANNELS,
  type PetActivitySnapshot,
  type PetImportReport,
  type PetSettings,
  type PetSummary,
} from "../shared/pets.js";

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
  relaunchDesktop: () =>
    ipcRenderer.invoke("awaken:relaunch-desktop") as Promise<void>,
  getSearchSettings: () =>
    ipcRenderer.invoke("awaken:get-search-settings"),
  saveSearchSettings: (request) =>
    ipcRenderer.invoke("awaken:save-search-settings", request),
  testSearchProvider: (request) =>
    ipcRenderer.invoke("awaken:test-search-provider", request),
  getRun: (runId: string) =>
    ipcRenderer.invoke("awaken:get-run", runId) as Promise<RuntimeRun>,
  getThreadTranscript: (threadId: string) =>
    ipcRenderer.invoke("awaken:get-thread-transcript", threadId) as Promise<RuntimeThreadTranscript>,
  listThreads: () =>
    ipcRenderer.invoke("awaken:list-threads") as Promise<RuntimeThread[]>,
  renameThread: (threadId: string, label: string) =>
    ipcRenderer.invoke("awaken:rename-thread", threadId, label) as Promise<RuntimeThread>,
  deleteThread: (threadId: string) =>
    ipcRenderer.invoke("awaken:delete-thread", threadId) as Promise<ThreadDeleteResult>,
  listProjects: (includeRemoved = false) =>
    ipcRenderer.invoke("awaken:list-projects", includeRemoved) as Promise<RuntimeProject[]>,
  upsertProject: (workspaceRoot: string, displayName?: string) =>
    ipcRenderer.invoke("awaken:upsert-project", {
      workspaceRoot,
      displayName,
    }) as Promise<UpsertProjectResult>,
  detachProject: (projectId: string, idempotencyKey: string) =>
    ipcRenderer.invoke("awaken:detach-project", {
      projectId,
      idempotencyKey,
    }) as Promise<ProjectMutationResult>,
  deleteProjectThreads: (projectId: string, idempotencyKey: string) =>
    ipcRenderer.invoke("awaken:delete-project-threads", {
      projectId,
      idempotencyKey,
    }) as Promise<ProjectMutationResult>,
  trashProject: (projectId: string, idempotencyKey: string) =>
    ipcRenderer.invoke("awaken:trash-project", {
      projectId,
      idempotencyKey,
    }) as Promise<ProjectMutationResult>,
  getProjectOperation: (operationId: string) =>
    ipcRenderer.invoke(
      "awaken:get-project-operation",
      operationId,
    ) as Promise<RuntimeProjectOperation>,
  openArtifact: (request: ArtifactOpenRequest) =>
    ipcRenderer.invoke("awaken:open-artifact", request) as Promise<ArtifactOpenResult>,
  revealArtifact: (request: ArtifactOpenRequest) =>
    ipcRenderer.invoke("awaken:reveal-artifact", request) as Promise<ArtifactOpenResult>,
  startRun: (request: StartRunRequest) =>
    ipcRenderer.invoke("awaken:start-run", request) as Promise<
      DesktopOperationResult<RuntimeRun>
    >,
  cancelRun: (runId: string) =>
    ipcRenderer.invoke("awaken:cancel-run", runId) as Promise<RuntimeRun>,
  answerUserQuestion: (runId: string, questionId: string, answer: string) =>
    ipcRenderer.invoke("awaken:answer-user-question", {
      runId,
      questionId,
      answer,
    }) as Promise<void>,
  subscribeRun: async (runId: string, handlers: RunStreamHandlers, afterCursor?: number) => {
    const subscriptionId = crypto.randomUUID();
    subscriptions.set(subscriptionId, handlers);

    try {
      await ipcRenderer.invoke("awaken:subscribe-run", {
        runId,
        subscriptionId,
        afterCursor,
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
  getDesktopPreferences: () =>
    ipcRenderer.invoke("awaken:get-desktop-preferences") as Promise<DesktopPreferences>,
  saveDesktopPreferences: (request: DesktopPreferencesSaveRequest) =>
    ipcRenderer.invoke("awaken:save-desktop-preferences", request) as Promise<DesktopPreferences>,
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
  getSkillPreview: (request: GetSkillPreviewRequest) =>
    ipcRenderer.invoke("awaken:get-skill-preview", request) as Promise<SkillPreviewResponse>,
  installSkill: (request: InstallSkillRequest) =>
    ipcRenderer.invoke("awaken:install-skill", request) as Promise<InstallSkillResponse>,
  deleteSkill: (request: DeleteSkillRequest) =>
    ipcRenderer.invoke("awaken:delete-skill", request) as Promise<DeleteSkillResponse>,
  disableSkill: (id: string) =>
    ipcRenderer.invoke("awaken:disable-skill", id) as Promise<void>,
  enableSkill: (id: string) =>
    ipcRenderer.invoke("awaken:enable-skill", id) as Promise<void>,
  listModelProviders: () =>
    ipcRenderer.invoke("awaken:list-model-providers") as Promise<ModelProvider[]>,
  listModelProfiles: () =>
    ipcRenderer.invoke("awaken:list-model-profiles") as Promise<RuntimeModelProfileSummary[]>,
  saveModelProfile: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:save-model-profile", request) as Promise<
      DesktopOperationResult<RuntimeModelProfileSummary>
    >,
  deleteModelProfile: (profileId: string) =>
    ipcRenderer.invoke("awaken:delete-model-profile", profileId) as Promise<void>,
  setDefaultModelProfile: (profileId: string) =>
    ipcRenderer.invoke("awaken:set-default-model-profile", profileId) as Promise<RuntimeModelProfileSummary>,
  testModelProfile: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:test-model-profile", request) as Promise<
      DesktopOperationResult<ModelProfileTestResult>
    >,
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
  listPets: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.listPets) as Promise<PetSummary[]>,
  getPetSettings: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.getPetSettings) as Promise<PetSettings>,
  selectPet: (petId: string) =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.selectPet, petId) as Promise<PetSettings>,
  refreshCodexPets: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.refreshCodexPets) as Promise<PetImportReport>,
  createPet: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.createPet) as Promise<{ threadId: string }>,
  setPetOverlayOpen: (open: boolean) =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.setPetOverlayOpen, open) as Promise<PetSettings>,
  setPetImportPreferences: (input: { importCodexPets?: boolean }) =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.setPetImportPreferences, input) as Promise<PetSettings>,
  updatePetActivity: (snapshot: PetActivitySnapshot) => {
    ipcRenderer.send(PET_IPC_CHANNELS.updatePetActivity, snapshot);
  },
  onPetCatalogChanged: (callback: () => void) => {
    const handler = () => callback();
    ipcRenderer.on(PET_IPC_CHANNELS.petCatalogChanged, handler);
    return () => ipcRenderer.removeListener(PET_IPC_CHANNELS.petCatalogChanged, handler);
  },
  onPetThreadActivationRequested: (callback: (threadId: string) => void) => {
    const handler = (_event: IpcRendererEvent, threadId: string) => callback(threadId);
    ipcRenderer.on(PET_IPC_CHANNELS.petThreadActivationRequested, handler);
    return () =>
      ipcRenderer.removeListener(PET_IPC_CHANNELS.petThreadActivationRequested, handler);
  },
  openAwakenPetsDirectory: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.openAwakenPetsDirectory) as Promise<void>,
  openCodexPetsDirectory: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.openCodexPetsDirectory) as Promise<void>,
};

contextBridge.exposeInMainWorld("awaken", bridge);
