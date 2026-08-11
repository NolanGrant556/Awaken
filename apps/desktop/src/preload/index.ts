import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import type { AguiEvent } from "../shared/agui.js";
import { DESKTOP_SELECT_ALL_REQUESTED_CHANNEL } from "../shared/desktop-selection.js";
import {
  PROJECT_FILE_PREVIEW_INTERACTION_CHANNEL,
  type ProjectFilePreviewInteraction,
} from "../shared/project-file-preview-interaction.js";
import {
  type ACPRunnerCreateRequest,
  type ACPRunnerProbeResult,
  type ACPRunnersSettings,
  type ACPRunnerUpdateRequest,
  type ACPRunnerView,
  type AgentPlanAccount,
  type AgentPlanAccountSaveRequest,
  type AgentPlanMediaCapability,
  type AgentPlanMediaToolSaveRequest,
  type AgentPlanMediaTools,
  type AgentPlanMediaToolsSaveResult,
  type AttachmentPickResult,
  type ArtifactOpenRequest,
  type ArtifactOpenResult,
  type ArtifactMediaSourceResult,
  type AwakenBridge,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type CliActivation,
  type CliConnectorStateEvent,
  type CliConnectorView,
  type CliProviderId,
  type ComputerHostStatus,
  type ComputerPermission,
  type ComputerPermissionStatus,
  type ComputerRuntimeSetupResult,
  type ComputerSettingsDocument,
  type ComputerSettingsSaveRequest,
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
  type ModelCatalogRefreshResult,
  type ModelReasoningCapabilityTestResult,
  type ModelProfileSaveRequest,
  type ModelPurposeRouting,
  type ModelProfileTestResult,
  type ModelProvider,
  type MemoryDreamTask,
  type MemoryListRequest,
  type MemorySettingsDocument,
  type MemorySettingsPatch,
  type ProjectFolderSelection,
  type ProjectDirectoryListRequest,
  type ProjectDirectoryListResult,
  type ProjectEntryActionRequest,
  type ProjectEntryActionResult,
  type ProjectEntryOpenWithRequest,
  type ProjectEntryResolveResult,
  type ProjectFilePreviewActionRequest,
  type ProjectFilePreviewActionResult,
  type ProjectFilePreviewCloseRequest,
  type ProjectFilePreviewStartRequest,
  type ProjectFilePreviewStartResult,
  type ProjectMutationResult,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RunStreamHandlers,
  type RuntimeInfo,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeModelProfileSummary,
  type RuntimeMemory,
  type RuntimeRun,
  type RuntimeProject,
  type ProjectReorderResult,
  type RuntimeProjectOperation,
  type RuntimeSkill,
  type SkillProjectRoots,
  type SkillPreviewResponse,
  type RuntimeThreadTranscript,
  type RuntimeThread,
  type RuntimeToolInfo,
  type ScheduledTask,
  type ScheduledTaskCreateInput,
  type ScheduledTaskEvent,
  type ScheduledTaskExecution,
  type ScheduledTaskPatchInput,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type ThreadDeleteResult,
  type ThreadModelPreferenceRequest,
  type UpdateMcpServerConfigRequest,
  type ProjectFoldersPickResult,
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
const cliConnectorStateHandlers = new Set<(event: CliConnectorStateEvent) => void>();
const memoryDreamReceiptHandlers = new Set<(task: MemoryDreamTask) => void>();
const scheduledTaskEventHandlers = new Set<(event: ScheduledTaskEvent) => void>();
const scheduledTaskThreadActivationHandlers = new Set<(threadId: string) => void>();

ipcRenderer.on("awaken:cli-connector-state", (_event, payload: CliConnectorStateEvent) => {
  for (const handler of cliConnectorStateHandlers) handler(payload);
});
ipcRenderer.on("awaken:memory-dream-receipt", (_event, task: MemoryDreamTask) => {
  for (const handler of memoryDreamReceiptHandlers) handler(task);
});
ipcRenderer.on("awaken:scheduled-task-event", (_event, payload: ScheduledTaskEvent) => {
  for (const handler of scheduledTaskEventHandlers) handler(payload);
});
ipcRenderer.on(
  "awaken:scheduled-task-thread-activation",
  (_event, payload: { threadId?: string }) => {
    const threadId = typeof payload?.threadId === "string" ? payload.threadId : "";
    if (!threadId) return;
    for (const handler of scheduledTaskThreadActivationHandlers) handler(threadId);
  },
);

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
  onSelectAllRequested: (callback) => {
    const handler = () => callback();
    ipcRenderer.on(DESKTOP_SELECT_ALL_REQUESTED_CHANNEL, handler);
    return () => ipcRenderer.removeListener(DESKTOP_SELECT_ALL_REQUESTED_CHANNEL, handler);
  },
  relaunchDesktop: () => ipcRenderer.invoke("awaken:relaunch-desktop") as Promise<void>,
  getSearchSettings: () => ipcRenderer.invoke("awaken:get-search-settings"),
  saveSearchSettings: (request) => ipcRenderer.invoke("awaken:save-search-settings", request),
  testSearchProvider: (request) => ipcRenderer.invoke("awaken:test-search-provider", request),
  getACPRunners: () => ipcRenderer.invoke("awaken:get-acp-runners") as Promise<ACPRunnersSettings>,
  createACPRunner: (request: ACPRunnerCreateRequest) =>
    ipcRenderer.invoke("awaken:create-acp-runner", request) as Promise<ACPRunnerView>,
  updateACPRunner: (runnerId: string, request: ACPRunnerUpdateRequest) =>
    ipcRenderer.invoke("awaken:update-acp-runner", runnerId, request) as Promise<ACPRunnerView>,
  deleteACPRunner: (runnerId: string) =>
    ipcRenderer.invoke("awaken:delete-acp-runner", runnerId) as Promise<void>,
  probeACPRunner: (runnerId: string) =>
    ipcRenderer.invoke("awaken:probe-acp-runner", runnerId) as Promise<ACPRunnerProbeResult>,
  listMemories: (request?: MemoryListRequest) =>
    ipcRenderer.invoke("awaken:list-memories", request) as Promise<RuntimeMemory[]>,
  getMemory: (memoryId: string, projectId?: string) =>
    ipcRenderer.invoke("awaken:get-memory", memoryId, projectId) as Promise<RuntimeMemory>,
  deleteMemory: (memoryId: string, expectedRevision: number, projectId?: string) =>
    ipcRenderer.invoke(
      "awaken:delete-memory",
      memoryId,
      expectedRevision,
      projectId,
    ) as Promise<void>,
  getMemorySettings: () =>
    ipcRenderer.invoke("awaken:get-memory-settings") as Promise<MemorySettingsDocument>,
  updateMemorySettings: (request: MemorySettingsPatch) =>
    ipcRenderer.invoke("awaken:update-memory-settings", request) as Promise<MemorySettingsDocument>,
  startMemoryDream: (projectId?: string) =>
    ipcRenderer.invoke("awaken:start-memory-dream", projectId) as Promise<MemoryDreamTask>,
  getMemoryDream: (taskId: string) =>
    ipcRenderer.invoke("awaken:get-memory-dream", taskId) as Promise<MemoryDreamTask>,
  listMemoryDreams: (projectId?: string, limit = 5) =>
    ipcRenderer.invoke("awaken:list-memory-dreams", projectId, limit) as Promise<MemoryDreamTask[]>,
  cancelMemoryDream: (taskId: string) =>
    ipcRenderer.invoke("awaken:cancel-memory-dream", taskId) as Promise<void>,
  onMemoryDreamReceipt: (handler) => {
    memoryDreamReceiptHandlers.add(handler);
    return () => memoryDreamReceiptHandlers.delete(handler);
  },
  getRun: (runId: string) => ipcRenderer.invoke("awaken:get-run", runId) as Promise<RuntimeRun>,
  getThreadTranscript: (threadId: string) =>
    ipcRenderer.invoke(
      "awaken:get-thread-transcript",
      threadId,
    ) as Promise<RuntimeThreadTranscript>,
  listThreads: () => ipcRenderer.invoke("awaken:list-threads") as Promise<RuntimeThread[]>,
  renameThread: (threadId: string, label: string) =>
    ipcRenderer.invoke("awaken:rename-thread", threadId, label) as Promise<RuntimeThread>,
  updateThreadModelPreference: (threadId: string, request: ThreadModelPreferenceRequest) =>
    ipcRenderer.invoke(
      "awaken:update-thread-model-preference",
      threadId,
      request,
    ) as Promise<RuntimeThread>,
  deleteThread: (threadId: string) =>
    ipcRenderer.invoke("awaken:delete-thread", threadId) as Promise<ThreadDeleteResult>,
  listProjects: (includeRemoved = false) =>
    ipcRenderer.invoke("awaken:list-projects", includeRemoved) as Promise<RuntimeProject[]>,
  listProjectDirectory: (request: ProjectDirectoryListRequest) =>
    ipcRenderer.invoke(
      "awaken:list-project-directory",
      request,
    ) as Promise<ProjectDirectoryListResult>,
  resolveProjectEntryForTask: (request: ProjectEntryActionRequest) =>
    ipcRenderer.invoke(
      "awaken:resolve-project-entry-for-task",
      request,
    ) as Promise<ProjectEntryResolveResult>,
  copyProjectEntryPath: (request: ProjectEntryActionRequest) =>
    ipcRenderer.invoke(
      "awaken:copy-project-entry-path",
      request,
    ) as Promise<ProjectEntryActionResult>,
  openProjectEntryWith: (request: ProjectEntryOpenWithRequest) =>
    ipcRenderer.invoke(
      "awaken:open-project-entry-with",
      request,
    ) as Promise<ProjectEntryActionResult>,
  startProjectFilePreview: (request: ProjectFilePreviewStartRequest) =>
    ipcRenderer.invoke(
      "awaken:start-project-file-preview",
      request,
    ) as Promise<ProjectFilePreviewStartResult>,
  closeProjectFilePreview: (request: ProjectFilePreviewCloseRequest) =>
    ipcRenderer.invoke("awaken:close-project-file-preview", request) as Promise<void>,
  onProjectFilePreviewInteraction: (callback) => {
    const handler = (_event: IpcRendererEvent, interaction: ProjectFilePreviewInteraction) =>
      callback(interaction);
    ipcRenderer.on(PROJECT_FILE_PREVIEW_INTERACTION_CHANNEL, handler);
    return () => ipcRenderer.removeListener(PROJECT_FILE_PREVIEW_INTERACTION_CHANNEL, handler);
  },
  openProjectFileExternally: (request: ProjectFilePreviewActionRequest) =>
    ipcRenderer.invoke(
      "awaken:open-project-file-externally",
      request,
    ) as Promise<ProjectFilePreviewActionResult>,
  revealProjectFile: (request: ProjectFilePreviewActionRequest) =>
    ipcRenderer.invoke(
      "awaken:reveal-project-file",
      request,
    ) as Promise<ProjectFilePreviewActionResult>,
  upsertProject: (workspaceRoot: string, displayName?: string) =>
    ipcRenderer.invoke("awaken:upsert-project", {
      workspaceRoot,
      displayName,
    }) as Promise<UpsertProjectResult>,
  createProject: (folders: ProjectFolderSelection[], displayName?: string) =>
    ipcRenderer.invoke("awaken:create-project", {
      folders,
      displayName,
    }) as Promise<UpsertProjectResult>,
  attachProjectFolder: (projectId: string, path: string) =>
    ipcRenderer.invoke("awaken:attach-project-folder", {
      projectId,
      path,
    }) as Promise<UpsertProjectResult>,
  detachProjectFolder: (projectId: string, path: string) =>
    ipcRenderer.invoke("awaken:detach-project-folder", {
      projectId,
      path,
    }) as Promise<UpsertProjectResult>,
  setPrimaryProjectFolder: (projectId: string, path: string) =>
    ipcRenderer.invoke("awaken:set-primary-project-folder", {
      projectId,
      path,
    }) as Promise<UpsertProjectResult>,
  reorderProjects: (projectIds: string[]) =>
    ipcRenderer.invoke("awaken:reorder-projects", projectIds) as Promise<ProjectReorderResult>,
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
  trashProjectFolder: (projectId: string, path: string, idempotencyKey: string) =>
    ipcRenderer.invoke("awaken:trash-project-folder", {
      projectId,
      path,
      idempotencyKey,
    }) as Promise<ProjectMutationResult>,
  getProjectOperation: (operationId: string) =>
    ipcRenderer.invoke(
      "awaken:get-project-operation",
      operationId,
    ) as Promise<RuntimeProjectOperation>,
  openArtifact: (request: ArtifactOpenRequest) =>
    ipcRenderer.invoke("awaken:open-artifact", request) as Promise<ArtifactOpenResult>,
  getArtifactMediaSource: (request: ArtifactOpenRequest) =>
    ipcRenderer.invoke("awaken:get-artifact-media-source", request) as Promise<ArtifactMediaSourceResult>,
  revealPath: (target: string) =>
    ipcRenderer.invoke("awaken:reveal-path", target) as Promise<{
      ok: boolean;
      message?: string;
    }>,
  revealArtifact: (request: ArtifactOpenRequest) =>
    ipcRenderer.invoke("awaken:reveal-artifact", request) as Promise<ArtifactOpenResult>,
  startRun: (request: StartRunRequest) =>
    ipcRenderer.invoke("awaken:start-run", request) as Promise<DesktopOperationResult<RuntimeRun>>,
  cancelRun: (runId: string) =>
    ipcRenderer.invoke("awaken:cancel-run", runId) as Promise<RuntimeRun>,
  cancelSubagent: (runId: string, taskId: string, reason?: string) =>
    ipcRenderer.invoke("awaken:cancel-subagent", {
      runId,
      taskId,
      reason,
    }) as Promise<void>,
  answerUserQuestion: (runId: string, questionId: string, answer: string) =>
    ipcRenderer.invoke("awaken:answer-user-question", {
      runId,
      questionId,
      answer,
    }) as Promise<void>,
  resolveExternalInteraction: (
    runId: string,
    interactionId: string,
    resolution: "completed" | "cancelled" | "expired" | "failed",
  ) =>
    ipcRenderer.invoke("awaken:resolve-external-interaction", {
      runId,
      interactionId,
      resolution,
    }) as Promise<void>,
  resolveACPPermission: (
    runId: string,
    interactionId: string,
    optionId: string,
    cancelled: boolean,
  ) =>
    ipcRenderer.invoke("awaken:resolve-acp-permission", {
      runId,
      interactionId,
      optionId,
      cancelled,
    }) as Promise<void>,
  resolveComputerReview: (runId: string, reviewId: string, decision: "approved_once" | "denied") =>
    ipcRenderer.invoke("awaken:resolve-computer-review", {
      runId,
      reviewId,
      decision,
    }) as Promise<void>,
  computerSessionControl: (runId: string, action: "takeover" | "return" | "pause" | "terminate") =>
    ipcRenderer.invoke("awaken:computer-session-control", {
      runId,
      action,
    }) as Promise<void>,
  getComputerHostStatus: () =>
    ipcRenderer.invoke("awaken:get-computer-host-status") as Promise<ComputerHostStatus>,
  getComputerSettings: () =>
    ipcRenderer.invoke("awaken:get-computer-settings") as Promise<ComputerSettingsDocument>,
  saveComputerSettings: (request: ComputerSettingsSaveRequest) =>
    ipcRenderer.invoke("awaken:save-computer-settings", request) as Promise<ComputerSettingsDocument>,
  prepareComputerRuntime: () =>
    ipcRenderer.invoke("awaken:prepare-computer-runtime") as Promise<ComputerRuntimeSetupResult>,
  getComputerPermissionStatus: (permission: ComputerPermission) =>
    ipcRenderer.invoke("awaken:get-computer-permission-status", permission) as Promise<ComputerPermissionStatus>,
  requestComputerPermission: (permission: ComputerPermission) =>
    ipcRenderer.invoke("awaken:request-computer-permission", permission) as Promise<ComputerPermissionStatus>,
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
  getRuntimeInfo: () => ipcRenderer.invoke("awaken:get-runtime-info") as Promise<RuntimeInfo>,
  listConnectors: () => ipcRenderer.invoke("awaken:list-connectors") as Promise<RuntimeConnector[]>,
  connectConnector: (connectorId: string) =>
    ipcRenderer.invoke(
      "awaken:connect-connector",
      connectorId,
    ) as Promise<RuntimeConnectorActionResponse>,
  disconnectConnector: (connectorId: string) =>
    ipcRenderer.invoke(
      "awaken:disconnect-connector",
      connectorId,
    ) as Promise<RuntimeConnectorActionResponse>,
  listCliConnectors: () =>
    ipcRenderer.invoke("awaken:list-cli-connectors") as Promise<CliConnectorView[]>,
  refreshCliConnector: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:refresh-cli-connector", providerId) as Promise<CliConnectorView>,
  installCliConnector: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:install-cli-connector", providerId) as Promise<CliConnectorView>,
  cancelCliInstall: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:cancel-cli-install", providerId) as Promise<CliConnectorView>,
  startCliAuth: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:start-cli-auth", providerId) as Promise<CliConnectorView>,
  cancelCliAuth: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:cancel-cli-auth", providerId) as Promise<CliConnectorView>,
  retryCliAuth: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:retry-cli-auth", providerId) as Promise<CliConnectorView>,
  setCliConnectorActivation: (providerId: CliProviderId, activation: CliActivation) =>
    ipcRenderer.invoke(
      "awaken:set-cli-activation",
      providerId,
      activation,
    ) as Promise<CliConnectorView>,
  logoutCliConnector: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:logout-cli-connector", providerId) as Promise<CliConnectorView>,
  uninstallManagedCli: (providerId: CliProviderId) =>
    ipcRenderer.invoke("awaken:uninstall-managed-cli", providerId) as Promise<CliConnectorView>,
  subscribeCliConnectorState: (handler) => {
    cliConnectorStateHandlers.add(handler);
    return () => cliConnectorStateHandlers.delete(handler);
  },
  listMcpServers: () =>
    ipcRenderer.invoke("awaken:list-mcp-servers") as Promise<RuntimeMcpServer[]>,
  configureMcpServers: (request: ConfigureMcpServersRequest) =>
    ipcRenderer.invoke(
      "awaken:configure-mcp-servers",
      request,
    ) as Promise<ConfigureMcpServersResponse>,
  getMcpServerConfig: (name: string) =>
    ipcRenderer.invoke(
      "awaken:get-mcp-server-config",
      name,
    ) as Promise<RuntimeMcpServerConfigResponse>,
  updateMcpServerConfig: (name: string, request: UpdateMcpServerConfigRequest) =>
    ipcRenderer.invoke(
      "awaken:update-mcp-server-config",
      name,
      request,
    ) as Promise<ConfigureMcpServersResponse>,
  getRuntimeLogs: () => ipcRenderer.invoke("awaken:get-runtime-logs") as Promise<RuntimeLogEntry[]>,
  disableMcpServer: (name: string) =>
    ipcRenderer.invoke("awaken:disable-mcp-server", name) as Promise<void>,
  enableMcpServer: (name: string) =>
    ipcRenderer.invoke("awaken:enable-mcp-server", name) as Promise<void>,
  deleteMcpServer: (name: string) =>
    ipcRenderer.invoke("awaken:delete-mcp-server", name) as Promise<void>,
  listMcpTools: () => ipcRenderer.invoke("awaken:list-mcp-tools") as Promise<RuntimeToolInfo[]>,
  listSkills: (projectRoots?: SkillProjectRoots) =>
    ipcRenderer.invoke("awaken:list-skills", projectRoots) as Promise<RuntimeSkill[]>,
  getSkillPreview: (request: GetSkillPreviewRequest) =>
    ipcRenderer.invoke("awaken:get-skill-preview", request) as Promise<SkillPreviewResponse>,
  installSkill: (request: InstallSkillRequest) =>
    ipcRenderer.invoke("awaken:install-skill", request) as Promise<InstallSkillResponse>,
  deleteSkill: (request: DeleteSkillRequest) =>
    ipcRenderer.invoke("awaken:delete-skill", request) as Promise<DeleteSkillResponse>,
  disableSkill: (id: string) => ipcRenderer.invoke("awaken:disable-skill", id) as Promise<void>,
  enableSkill: (id: string) => ipcRenderer.invoke("awaken:enable-skill", id) as Promise<void>,
  listModelProviders: () =>
    ipcRenderer.invoke("awaken:list-model-providers") as Promise<ModelProvider[]>,
  listModelProfiles: () =>
    ipcRenderer.invoke("awaken:list-model-profiles") as Promise<RuntimeModelProfileSummary[]>,
  getModelPurposeRouting: () =>
    ipcRenderer.invoke("awaken:get-model-purpose-routing") as Promise<ModelPurposeRouting>,
  saveModelPurposeRouting: (routing: ModelPurposeRouting) =>
    ipcRenderer.invoke("awaken:save-model-purpose-routing", routing) as Promise<ModelPurposeRouting>,
  getAgentPlanAccount: () =>
    ipcRenderer.invoke("awaken:get-agent-plan-account") as Promise<AgentPlanAccount>,
  saveAgentPlanAccount: (request: AgentPlanAccountSaveRequest) =>
    ipcRenderer.invoke("awaken:save-agent-plan-account", request) as Promise<
      DesktopOperationResult<AgentPlanAccount>
    >,
  refreshAgentPlanModels: () =>
    ipcRenderer.invoke("awaken:refresh-agent-plan-models") as Promise<
      DesktopOperationResult<AgentPlanAccount>
    >,
  deleteAgentPlanAccount: () =>
    ipcRenderer.invoke("awaken:delete-agent-plan-account") as Promise<void>,
  getAgentPlanMediaTools: () =>
    ipcRenderer.invoke(
      "awaken:get-agent-plan-media-tools",
    ) as Promise<AgentPlanMediaTools>,
  saveAgentPlanMediaTool: (request: AgentPlanMediaToolSaveRequest) =>
    ipcRenderer.invoke(
      "awaken:save-agent-plan-media-tool",
      request,
    ) as Promise<AgentPlanMediaToolsSaveResult>,
  deleteAgentPlanMediaTool: (capability: AgentPlanMediaCapability) =>
    ipcRenderer.invoke(
      "awaken:delete-agent-plan-media-tool",
      capability,
    ) as Promise<AgentPlanMediaToolsSaveResult>,
  saveModelProfile: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:save-model-profile", request) as Promise<
      DesktopOperationResult<RuntimeModelProfileSummary>
    >,
  deleteModelProfile: (profileId: string) =>
    ipcRenderer.invoke("awaken:delete-model-profile", profileId) as Promise<void>,
  setDefaultModelProfile: (profileId: string) =>
    ipcRenderer.invoke(
      "awaken:set-default-model-profile",
      profileId,
    ) as Promise<RuntimeModelProfileSummary>,
  testModelProfile: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:test-model-profile", request) as Promise<
      DesktopOperationResult<ModelProfileTestResult>
    >,
  testAgentPlanReasoningCapability: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:test-agent-plan-reasoning-capability", request) as Promise<
      DesktopOperationResult<ModelReasoningCapabilityTestResult>
    >,
  syncModelCapabilities: () =>
    ipcRenderer.invoke("awaken:sync-model-capabilities") as Promise<ModelProvider[]>,
  refreshModelCatalog: (request: ModelProfileSaveRequest) =>
    ipcRenderer.invoke("awaken:refresh-model-catalog", request) as Promise<
      DesktopOperationResult<ModelCatalogRefreshResult>
    >,
  getModelProfileBalances: () =>
    ipcRenderer.invoke("awaken:get-model-profile-balances") as Promise<ModelProfileBalancesResult>,
  pickAttachments: (defaultPath?: string) =>
    ipcRenderer.invoke("awaken:pick-attachments", defaultPath) as Promise<AttachmentPickResult>,
  pickDirectory: (defaultPath?: string) =>
    ipcRenderer.invoke("awaken:pick-directory", defaultPath) as Promise<WorkspacePickResult>,
  pickProjectFolders: (defaultPath?: string) =>
    ipcRenderer.invoke(
      "awaken:pick-project-folders",
      defaultPath,
    ) as Promise<ProjectFoldersPickResult>,
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
  listScheduledTasks: () =>
    ipcRenderer.invoke("awaken:list-scheduled-tasks") as Promise<ScheduledTask[]>,
  getScheduledTask: (taskId: string) =>
    ipcRenderer.invoke("awaken:get-scheduled-task", taskId) as Promise<ScheduledTask>,
  createScheduledTask: (input: ScheduledTaskCreateInput) =>
    ipcRenderer.invoke("awaken:create-scheduled-task", input) as Promise<ScheduledTask>,
  patchScheduledTask: (taskId: string, input: ScheduledTaskPatchInput) =>
    ipcRenderer.invoke("awaken:patch-scheduled-task", taskId, input) as Promise<ScheduledTask>,
  deleteScheduledTask: (taskId: string, expectedVersion: number, idempotencyKey: string) =>
    ipcRenderer.invoke("awaken:delete-scheduled-task", {
      taskId,
      expectedVersion,
      idempotencyKey,
    }) as Promise<void>,
  pauseScheduledTask: (taskId: string, expectedVersion: number) =>
    ipcRenderer.invoke("awaken:pause-scheduled-task", {
      taskId,
      expectedVersion,
    }) as Promise<ScheduledTask>,
  resumeScheduledTask: (taskId: string, expectedVersion: number) =>
    ipcRenderer.invoke("awaken:resume-scheduled-task", {
      taskId,
      expectedVersion,
    }) as Promise<ScheduledTask>,
  runScheduledTaskNow: (taskId: string, idempotencyKey: string) =>
    ipcRenderer.invoke("awaken:run-scheduled-task-now", {
      taskId,
      idempotencyKey,
    }) as Promise<ScheduledTaskExecution>,
  cancelScheduledTaskExecution: (taskId: string) =>
    ipcRenderer.invoke("awaken:cancel-scheduled-task-execution", taskId) as Promise<void>,
  listScheduledTaskExecutions: (taskId: string) =>
    ipcRenderer.invoke("awaken:list-scheduled-task-executions", taskId) as Promise<
      ScheduledTaskExecution[]
    >,
  getSystemTimezone: () => ipcRenderer.invoke("awaken:get-system-timezone") as Promise<string>,
  subscribeScheduledTaskEvents: (handler) => {
    scheduledTaskEventHandlers.add(handler);
    return () => scheduledTaskEventHandlers.delete(handler);
  },
  onScheduledTaskThreadActivation: (handler) => {
    scheduledTaskThreadActivationHandlers.add(handler);
    return () => scheduledTaskThreadActivationHandlers.delete(handler);
  },
  listPets: () => ipcRenderer.invoke(PET_IPC_CHANNELS.listPets) as Promise<PetSummary[]>,
  getPetSettings: () => ipcRenderer.invoke(PET_IPC_CHANNELS.getPetSettings) as Promise<PetSettings>,
  selectPet: (petId: string) =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.selectPet, petId) as Promise<PetSettings>,
  refreshCodexPets: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.refreshCodexPets) as Promise<PetImportReport>,
  createPet: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.createPet) as Promise<{
      threadId: string;
    }>,
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
    return () => ipcRenderer.removeListener(PET_IPC_CHANNELS.petThreadActivationRequested, handler);
  },
  openAwakenPetsDirectory: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.openAwakenPetsDirectory) as Promise<void>,
  openCodexPetsDirectory: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.openCodexPetsDirectory) as Promise<void>,
};

contextBridge.exposeInMainWorld("awaken", bridge);
