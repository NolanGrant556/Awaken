import type { AguiEvent } from "./agui";
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
  type ArtifactPreviewStartRequest,
  type AwakenBridge,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type CommitPluginRequest,
  type CliActivation,
  type CliConnectorStateHandler,
  type CliConnectorView,
  type CliProviderId,
  type ComposerCatalogEvent,
  type ComputerHostStatus,
  type ComputerSettingsDocument,
  type ComputerSettingsSaveRequest,
  type DesktopPreferences,
  type DesktopPreferencesSaveRequest,
  type DesktopOperationIssue,
  type DesktopOperationResult,
  type DesktopOperationRecoveryAction,
  type DesktopPlatform,
  type DeleteSkillRequest,
  type DeleteSkillResponse,
  type GetSkillPreviewRequest,
	type GlobalAgentsDeleteRequest,
	type GlobalAgentsDocument,
	type GlobalAgentsSaveRequest,
	type HookRevokeRequest,
	type HookStatus,
	type HookTrustRequest,
  type InstallPluginRequest,
  type InstallPluginResponse,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type ModelProfileBalancesResult,
  type ModelCatalogRefreshResult,
  type ModelReasoningCapabilityTestResult,
  type UserQuestionAnswer,
  type ModelProfileSaveRequest,
  type ModelPurposeRouting,
  type ModelProfileTestResult,
  type ModelProvider,
  type MemoryDreamTask,
  type MemoryListRequest,
  type MemorySettingsDocument,
  type MemorySettingsPatch,
  type ProjectDirectoryChangedEvent,
  type ProjectFolderSelection,
  type ProjectDirectoryListRequest,
  type ProjectDirectoryListResult,
  type ProjectDirectoryWatchRequest,
  type ProjectDirectoryWatchResult,
  type ProjectEntryActionRequest,
  type ProjectEntryActionResult,
  type ProjectEntryOpenWithRequest,
  type ProjectEntryResolveResult,
  type ProjectFilePreviewActionRequest,
  type ProjectFilePreviewActionResult,
  type ProjectFilePreviewCloseRequest,
  type ProjectFilePreviewSaveRequest,
  type ProjectFilePreviewSaveResult,
  type ProjectFilePreviewStartRequest,
  type ProjectFilePreviewStartResult,
  type ProjectMutationResult,
	type ProjectAgentsCreateResult,
  type ProjectReorderResult,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RuntimeConnectorAuthSession,
  type RuntimeInfo,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeModelProfileSummary,
  type RuntimeMemory,
  type RuntimeRun,
  type RuntimeProject,
  type RuntimeProjectOperation,
  type RuntimePlugin,
  type RuntimePluginComponentDetail,
  type RuntimePluginComponentKind,
  type RuntimePluginComponentSourceResponse,
  type RuntimePluginComponentState,
  type RuntimePluginComponentStateResponse,
  type RuntimePluginHookReceipt,
  type RuntimePluginHookReceiptResponse,
  type RuntimePluginConfigurationResponse,
  type RuntimePluginAuthSession,
  type RuntimePluginSetupMutationRequest,
  type RuntimePluginSetupResponse,
  type RuntimePluginSource,
  type RuntimePluginValidation,
  type RuntimeSkill,
  type SkillProjectRoots,
  type SkillPreviewResponse,
  type RuntimeThreadTranscript,
  type RewindThreadRequest,
  type RewindThreadResult,
  type RuntimeThread,
  type RuntimeThreadListView,
  type SetThreadArchivedResult,
  type RuntimeToolInfo,
  type SearchSettings,
  type SearchSettingsSaveRequest,
  type SearchSettingsSaveResult,
  type SearchProviderTestRequest,
  type SearchProviderTestResult,
  type ScheduledTask,
  type ScheduledTaskCreateInput,
  type ScheduledTaskEvent,
  type ScheduledTaskExecution,
  type ScheduledTaskPatchInput,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type ThreadDeleteResult,
  type ThreadBatchDeleteResult,
  type ThreadModelPreferenceRequest,
  type UpdateMcpServerConfigRequest,
  type UpdatePluginComponentSourceRequest,
  type UpdatePluginConfigurationRequest,
  type ProjectFoldersPickResult,
  type UpsertProjectResult,
  type WorkspacePickResult,
  type WorkspaceValidationResult,
} from "../../shared/runtime";
import type { DesktopUpdateSnapshot } from "../../shared/desktop-update";
import type { ProjectFilePreviewInteraction } from "../../shared/project-file-preview-interaction";
import type {
  PetActivitySnapshot,
  PetImportReport,
  PetSettings,
  PetSummary,
} from "../../shared/pets";

export type SubscribeHandlers = {
  onEvent: (event: AguiEvent) => void;
  onError?: (message: string) => void;
  onReconnect?: () => void;
  onClose?: () => void;
};

const DESKTOP_OPERATION_ISSUE_CODES = new Set<DesktopOperationIssue["code"]>([
  "credential_unreadable",
  "composer_selection_invalid",
  "plugin_component_unavailable",
  "plugin_selection_invalid",
  "run_conflict",
  "run_input_invalid",
  "runtime_unavailable",
  "file_comment_stale",
  "invalid_file_comment",
  "operation_failed",
]);
const DESKTOP_OPERATION_RECOVERY_ACTIONS = new Set<DesktopOperationRecoveryAction>([
  "retry",
  "relaunch",
  "reenter",
]);

export function desktopOperationIssueFromError(error: unknown): DesktopOperationIssue | null {
  if (!(error instanceof Error)) return null;
  const candidate = error as Error & {
    code?: unknown;
    retryable?: unknown;
    actions?: unknown;
    commentId?: unknown;
    reason?: unknown;
  };
  if (
    typeof candidate.code !== "string" ||
    !DESKTOP_OPERATION_ISSUE_CODES.has(candidate.code as DesktopOperationIssue["code"])
  ) {
    return null;
  }
  const actions = Array.isArray(candidate.actions)
    ? candidate.actions.filter(
        (action): action is DesktopOperationRecoveryAction =>
          typeof action === "string" &&
          DESKTOP_OPERATION_RECOVERY_ACTIONS.has(action as DesktopOperationRecoveryAction),
      )
    : [];
  return {
    code: candidate.code as DesktopOperationIssue["code"],
    message: candidate.message,
    retryable: candidate.retryable === true,
    actions,
    commentId: typeof candidate.commentId === "string" ? candidate.commentId : undefined,
    reason: typeof candidate.reason === "string" ? candidate.reason : undefined,
  };
}

async function unwrapDesktopOperation<T>(
  resultPromise: Promise<DesktopOperationResult<T>>,
): Promise<T> {
  const result = await resultPromise;
  if (result.ok) return result.value;
  const error = Object.assign(new Error(result.issue.message), {
    name: "DesktopOperationError",
    code: result.issue.code,
    retryable: result.issue.retryable,
    actions: result.issue.actions,
    commentId: result.issue.commentId,
    reason: result.issue.reason,
  });
  throw error;
}

export type RuntimeClient = {
  source: "electron";
  platform: DesktopPlatform;
  relaunchDesktop: () => Promise<void>;
  getRuntimeInfo: () => Promise<RuntimeInfo>;
  getSearchSettings: () => Promise<SearchSettings>;
  saveSearchSettings: (request: SearchSettingsSaveRequest) => Promise<SearchSettingsSaveResult>;
  testSearchProvider: (request: SearchProviderTestRequest) => Promise<SearchProviderTestResult>;
  getACPRunners: () => Promise<ACPRunnersSettings>;
  createACPRunner: (request: ACPRunnerCreateRequest) => Promise<ACPRunnerView>;
  updateACPRunner: (runnerId: string, request: ACPRunnerUpdateRequest) => Promise<ACPRunnerView>;
  deleteACPRunner: (runnerId: string) => Promise<void>;
  probeACPRunner: (runnerId: string) => Promise<ACPRunnerProbeResult>;
  listConnectors: () => Promise<RuntimeConnector[]>;
  connectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  disconnectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  getConnectorAuthSession?: (sessionId: string) => Promise<RuntimeConnectorAuthSession>;
  cancelConnectorAuthSession?: (sessionId: string) => Promise<RuntimeConnectorAuthSession>;
  markConnectorAdminApprovalPending?: (sessionId: string) => Promise<RuntimeConnectorAuthSession>;
  listCliConnectors: () => Promise<CliConnectorView[]>;
  refreshCliConnector: (providerId: CliProviderId) => Promise<CliConnectorView>;
  installCliConnector: (providerId: CliProviderId) => Promise<CliConnectorView>;
  cancelCliInstall: (providerId: CliProviderId) => Promise<CliConnectorView>;
  startCliAuth: (providerId: CliProviderId) => Promise<CliConnectorView>;
  cancelCliAuth: (providerId: CliProviderId) => Promise<CliConnectorView>;
  retryCliAuth: (providerId: CliProviderId) => Promise<CliConnectorView>;
  setCliConnectorActivation: (
    providerId: CliProviderId,
    activation: CliActivation,
  ) => Promise<CliConnectorView>;
  logoutCliConnector: (providerId: CliProviderId) => Promise<CliConnectorView>;
  uninstallManagedCli: (providerId: CliProviderId) => Promise<CliConnectorView>;
  subscribeCliConnectorState: (handler: CliConnectorStateHandler) => () => void;
  subscribeComposerCatalogEvents: (handler: (event: ComposerCatalogEvent) => void) => () => void;
  listMcpServers: () => Promise<RuntimeMcpServer[]>;
  configureMcpServers: (
    request: ConfigureMcpServersRequest,
  ) => Promise<ConfigureMcpServersResponse>;
  getMcpServerConfig: (name: string) => Promise<RuntimeMcpServerConfigResponse>;
  updateMcpServerConfig: (
    name: string,
    request: UpdateMcpServerConfigRequest,
  ) => Promise<ConfigureMcpServersResponse>;
  getRuntimeLogs: () => Promise<RuntimeLogEntry[]>;
  disableMcpServer: (name: string) => Promise<void>;
  enableMcpServer: (name: string) => Promise<void>;
  deleteMcpServer: (name: string) => Promise<void>;
  listMcpTools: () => Promise<RuntimeToolInfo[]>;
  listSkills: (projectRoots?: SkillProjectRoots) => Promise<RuntimeSkill[]>;
  getSkillPreview: (request: GetSkillPreviewRequest) => Promise<SkillPreviewResponse>;
  installSkill: (request: InstallSkillRequest) => Promise<InstallSkillResponse>;
  deleteSkill: (request: DeleteSkillRequest) => Promise<DeleteSkillResponse>;
  disableSkill: (id: string) => Promise<void>;
  enableSkill: (id: string) => Promise<void>;
  listPlugins: () => Promise<RuntimePlugin[]>;
  getPlugin: (name: string) => Promise<RuntimePlugin>;
  validatePlugin: (source: RuntimePluginSource) => Promise<RuntimePluginValidation>;
  installPlugin: (request: InstallPluginRequest) => Promise<InstallPluginResponse>;
  commitPlugin: (name: string, request: CommitPluginRequest) => Promise<InstallPluginResponse>;
  enablePlugin: (name: string) => Promise<RuntimePlugin>;
  disablePlugin: (name: string) => Promise<RuntimePlugin>;
  uninstallPlugin: (name: string) => Promise<{ trashedPath?: string; message: string }>;
  enablePluginServer: (name: string, shortName: string) => Promise<RuntimePlugin>;
  disablePluginServer: (name: string, shortName: string) => Promise<RuntimePlugin>;
  listPluginComponents: (name: string) => Promise<RuntimePluginComponentState[]>;
  getPluginComponent: (
    name: string,
    kind: RuntimePluginComponentKind,
    logicalIdEncoded: string,
  ) => Promise<RuntimePluginComponentDetail>;
  updatePluginComponentSource: (
    name: string,
    kind: RuntimePluginComponentKind,
    logicalIdEncoded: string,
    request: UpdatePluginComponentSourceRequest,
  ) => Promise<RuntimePluginComponentSourceResponse>;
  setPluginComponentState: (
    name: string,
    kind: RuntimePluginComponentKind,
    logicalIdEncoded: string,
    enabled: boolean,
  ) => Promise<RuntimePluginComponentStateResponse>;
  setPluginMcpToolState: (
    name: string,
    logicalIdEncoded: string,
    toolName: string,
    enabled: boolean,
  ) => Promise<RuntimePluginComponentStateResponse>;
  setPluginRuleActivation: (
    name: string,
    logicalIdEncoded: string,
    activation: "always" | "file_globs" | "manual" | "model_decides",
  ) => Promise<{ plugin: RuntimePlugin; revision: number }>;
  listPluginHookReceipts: (
    name: string,
    logicalIdEncoded: string,
    revisionId?: string,
  ) => Promise<RuntimePluginHookReceiptResponse>;
  retryPluginHookReceipt: (
    name: string,
    logicalIdEncoded: string,
    receiptId: string,
  ) => Promise<{ receipt: RuntimePluginHookReceipt; diagnosticOnly: true }>;
  getPluginConfiguration: (name: string) => Promise<RuntimePluginConfigurationResponse>;
  updatePluginConfiguration: (
    name: string,
    request: UpdatePluginConfigurationRequest,
  ) => Promise<RuntimePluginConfigurationResponse>;
  getPluginSetup?: (name: string) => Promise<RuntimePluginSetupResponse>;
  startPluginSetup?: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginAuthSession>;
  getActivePluginAuthSession?: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginAuthSession | null>;
  getPluginAuthSession?: (name: string, sessionId: string) => Promise<RuntimePluginAuthSession>;
  cancelPluginAuthSession?: (name: string, sessionId: string) => Promise<RuntimePluginAuthSession>;
  markPluginAdminApprovalPending?: (name: string, sessionId: string) => Promise<RuntimePluginAuthSession>;
  probePluginSetup?: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginSetupResponse>;
  disconnectPluginSetup?: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginSetupResponse>;
  listModelProviders: () => Promise<ModelProvider[]>;
  listModelProfiles: () => Promise<RuntimeModelProfileSummary[]>;
  getModelPurposeRouting?: () => Promise<ModelPurposeRouting>;
  saveModelPurposeRouting?: (routing: ModelPurposeRouting) => Promise<ModelPurposeRouting>;
  getAgentPlanAccount: () => Promise<AgentPlanAccount>;
  saveAgentPlanAccount: (request: AgentPlanAccountSaveRequest) => Promise<AgentPlanAccount>;
  refreshAgentPlanModels: () => Promise<AgentPlanAccount>;
  deleteAgentPlanAccount: () => Promise<void>;
  getAgentPlanMediaTools: () => Promise<AgentPlanMediaTools>;
  saveAgentPlanMediaTool: (
    request: AgentPlanMediaToolSaveRequest,
  ) => Promise<AgentPlanMediaToolsSaveResult>;
  deleteAgentPlanMediaTool: (
    capability: AgentPlanMediaCapability,
  ) => Promise<AgentPlanMediaToolsSaveResult>;
  saveModelProfile: (
    request: ModelProfileSaveRequest,
  ) => Promise<RuntimeModelProfileSummary>;
  deleteModelProfile: (profileId: string) => Promise<void>;
  setDefaultModelProfile: (profileId: string) => Promise<RuntimeModelProfileSummary>;
  testModelProfile: (request: ModelProfileSaveRequest) => Promise<ModelProfileTestResult>;
  testAgentPlanReasoningCapability: (request: ModelProfileSaveRequest) => Promise<ModelReasoningCapabilityTestResult>;
  syncModelCapabilities: () => Promise<ModelProvider[]>;
  refreshModelCatalog: (
    request: ModelProfileSaveRequest,
  ) => Promise<ModelCatalogRefreshResult>;
  getModelProfileBalances: () => Promise<ModelProfileBalancesResult>;
  getRun: (runId: string) => Promise<RuntimeRun>;
  getThreadTranscript: (threadId: string) => Promise<RuntimeThreadTranscript>;
  rewindThread: (request: RewindThreadRequest) => Promise<RewindThreadResult>;
  listThreads: (view?: RuntimeThreadListView) => Promise<RuntimeThread[]>;
  renameThread: (threadId: string, label: string) => Promise<RuntimeThread>;
  setThreadArchived: (threadId: string, archived: boolean) => Promise<SetThreadArchivedResult>;
  updateThreadModelPreference: (
    threadId: string,
    request: ThreadModelPreferenceRequest,
  ) => Promise<RuntimeThread>;
  deleteThread: (threadId: string) => Promise<ThreadDeleteResult>;
  deleteThreads: (threadIds: string[]) => Promise<ThreadBatchDeleteResult>;
  listProjects: (includeRemoved?: boolean) => Promise<RuntimeProject[]>;
  listProjectDirectory: (
    request: ProjectDirectoryListRequest,
  ) => Promise<ProjectDirectoryListResult>;
  watchProjectDirectory: (
    request: ProjectDirectoryWatchRequest,
  ) => Promise<ProjectDirectoryWatchResult>;
  unwatchProjectDirectory: (subscriptionId: string) => Promise<void>;
  onProjectDirectoryChanged: (
    handler: (event: ProjectDirectoryChangedEvent) => void,
  ) => () => void;
  resolveProjectEntryForTask: (
    request: ProjectEntryActionRequest,
  ) => Promise<ProjectEntryResolveResult>;
  copyProjectEntryPath: (request: ProjectEntryActionRequest) => Promise<ProjectEntryActionResult>;
  revealProjectEntry: (request: ProjectEntryActionRequest) => Promise<ProjectEntryActionResult>;
  openProjectEntryWith: (request: ProjectEntryOpenWithRequest) => Promise<ProjectEntryActionResult>;
  startProjectFilePreview: (
    request: ProjectFilePreviewStartRequest,
  ) => Promise<ProjectFilePreviewStartResult>;
  startArtifactPreview: (
    request: ArtifactPreviewStartRequest,
  ) => Promise<ProjectFilePreviewStartResult>;
  closeProjectFilePreview: (request: ProjectFilePreviewCloseRequest) => Promise<void>;
  onProjectFilePreviewInteraction: (
    handler: (interaction: ProjectFilePreviewInteraction) => void,
  ) => () => void;
  openProjectFileExternally: (
    request: ProjectFilePreviewActionRequest,
  ) => Promise<ProjectFilePreviewActionResult>;
  revealProjectFile: (
    request: ProjectFilePreviewActionRequest,
  ) => Promise<ProjectFilePreviewActionResult>;
  saveProjectMarkdownFile: (
    request: ProjectFilePreviewSaveRequest,
  ) => Promise<ProjectFilePreviewSaveResult>;
	createProjectAgentsFile?: (projectId: string) => Promise<ProjectAgentsCreateResult>;
	getGlobalAgentsDocument?: () => Promise<GlobalAgentsDocument>;
	saveGlobalAgentsDocument?: (request: GlobalAgentsSaveRequest) => Promise<GlobalAgentsDocument>;
	deleteGlobalAgentsDocument?: (request: GlobalAgentsDeleteRequest) => Promise<GlobalAgentsDocument>;
  upsertProject: (workspaceRoot: string, displayName?: string) => Promise<UpsertProjectResult>;
  createProject: (
    folders: ProjectFolderSelection[],
    displayName?: string,
  ) => Promise<UpsertProjectResult>;
  renameProject: (projectId: string, displayName: string) => Promise<UpsertProjectResult>;
  attachProjectFolder: (projectId: string, path: string) => Promise<UpsertProjectResult>;
  detachProjectFolder: (projectId: string, path: string) => Promise<UpsertProjectResult>;
  setPrimaryProjectFolder: (projectId: string, path: string) => Promise<UpsertProjectResult>;
  reorderProjects: (projectIds: string[]) => Promise<ProjectReorderResult>;
  detachProject: (projectId: string, idempotencyKey: string) => Promise<ProjectMutationResult>;
  deleteProjectThreads: (
    projectId: string,
    idempotencyKey: string,
  ) => Promise<ProjectMutationResult>;
  trashProjectFolder: (
    projectId: string,
    path: string,
    idempotencyKey: string,
  ) => Promise<ProjectMutationResult>;
  getProjectOperation: (operationId: string) => Promise<RuntimeProjectOperation>;
  openArtifact: (request: ArtifactOpenRequest) => Promise<ArtifactOpenResult>;
  getArtifactMediaSource: (request: ArtifactOpenRequest) => Promise<ArtifactMediaSourceResult>;
  revealPath: (target: string) => Promise<{ ok: boolean; message?: string }>;
  revealArtifact: (request: ArtifactOpenRequest) => Promise<ArtifactOpenResult>;
  startRun: (request: StartRunRequest) => Promise<RuntimeRun>;
  cancelRun: (runId: string) => Promise<RuntimeRun>;
  cancelSubagent?: (runId: string, taskId: string, reason?: string) => Promise<void>;
  messageSubagent?: (
    runId: string,
    taskId: string,
    content: string,
    idempotencyKey?: string,
  ) => Promise<void>;
  answerUserQuestion: (runId: string, questionId: string, answer: string) => Promise<void>;
  cancelUserQuestion: (runId: string, questionId: string) => Promise<void>;
  resolveExternalInteraction: (
    runId: string,
    interactionId: string,
    resolution: "completed" | "cancelled" | "expired" | "failed",
    content?: Record<string, unknown>,
  ) => Promise<void>;
  resolveACPPermission: (
    runId: string,
    interactionId: string,
    optionId: string,
    cancelled: boolean,
  ) => Promise<void>;
  resolveComputerReview: (
    runId: string,
    reviewId: string,
    decision: "approved_once" | "denied",
  ) => Promise<void>;
  computerSessionControl: (
    runId: string,
    action: "takeover" | "return" | "pause" | "terminate",
  ) => Promise<void>;
  getComputerHostStatus: () => Promise<ComputerHostStatus>;
  getComputerSettings: () => Promise<ComputerSettingsDocument>;
  saveComputerSettings: (
    request: ComputerSettingsSaveRequest,
  ) => Promise<ComputerSettingsDocument>;
  prepareComputerRuntime: () => Promise<{
    node_ready: boolean;
    cua_host_ready: boolean;
    message?: string;
  }>;
  requestComputerPermissions: () => Promise<ComputerHostStatus>;
  openComputerPermissionSettings: (
    permission: "screen_recording" | "accessibility",
  ) => Promise<void>;
  getDesktopPreferences: () => Promise<DesktopPreferences>;
  saveDesktopPreferences: (request: DesktopPreferencesSaveRequest) => Promise<DesktopPreferences>;
  listMemories: (request?: MemoryListRequest) => Promise<RuntimeMemory[]>;
  getMemory: (memoryId: string, projectId?: string) => Promise<RuntimeMemory>;
  deleteMemory: (memoryId: string, expectedRevision: number, projectId?: string) => Promise<void>;
  getMemorySettings: () => Promise<MemorySettingsDocument>;
	getHookStatus?: (projectId?: string) => Promise<HookStatus>;
	trustProjectHook?: (request: HookTrustRequest) => Promise<HookStatus>;
	revokeProjectHook?: (request: HookRevokeRequest) => Promise<HookStatus>;
  updateMemorySettings: (request: MemorySettingsPatch) => Promise<MemorySettingsDocument>;
  startMemoryDream: (projectId?: string) => Promise<MemoryDreamTask>;
  getMemoryDream: (taskId: string) => Promise<MemoryDreamTask>;
  listMemoryDreams: (projectId?: string, limit?: number) => Promise<MemoryDreamTask[]>;
  cancelMemoryDream: (taskId: string) => Promise<void>;
  onMemoryDreamReceipt: (handler: (task: MemoryDreamTask) => void) => () => void;
  subscribeRun: (
    runId: string,
    handlers: SubscribeHandlers,
    afterSeq?: number,
  ) => Promise<() => void>;
  pickAttachments: (defaultPath?: string) => Promise<AttachmentPickResult>;
  pickDirectory: (defaultPath?: string) => Promise<WorkspacePickResult>;
  pickPluginSource: (defaultPath?: string) => Promise<WorkspacePickResult>;
  resolveDroppedPluginSource: (file: File) => Promise<WorkspacePickResult>;
  pickProjectFolders: (defaultPath?: string) => Promise<ProjectFoldersPickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
  notifyRendererReady: () => void;
  listScheduledTasks: () => Promise<ScheduledTask[]>;
  getScheduledTask: (taskId: string) => Promise<ScheduledTask>;
  createScheduledTask: (input: ScheduledTaskCreateInput) => Promise<ScheduledTask>;
  patchScheduledTask: (taskId: string, input: ScheduledTaskPatchInput) => Promise<ScheduledTask>;
  deleteScheduledTask: (
    taskId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ) => Promise<void>;
  pauseScheduledTask: (taskId: string, expectedVersion: number) => Promise<ScheduledTask>;
  resumeScheduledTask: (taskId: string, expectedVersion: number) => Promise<ScheduledTask>;
  runScheduledTaskNow: (taskId: string, idempotencyKey: string) => Promise<ScheduledTaskExecution>;
  cancelScheduledTaskExecution: (taskId: string) => Promise<void>;
  listScheduledTaskExecutions: (taskId: string) => Promise<ScheduledTaskExecution[]>;
  getSystemTimezone: () => Promise<string>;
  subscribeScheduledTaskEvents: (handler: (event: ScheduledTaskEvent) => void) => () => void;
  onScheduledTaskThreadActivation: (handler: (threadId: string) => void) => () => void;
  getDesktopUpdateState: () => Promise<DesktopUpdateSnapshot>;
  checkDesktopUpdate: () => Promise<DesktopUpdateSnapshot>;
  downloadDesktopUpdate: () => Promise<DesktopUpdateSnapshot>;
  restartAndInstallDesktopUpdate: () => Promise<DesktopUpdateSnapshot>;
  onDesktopUpdateStateChanged: (handler: (snapshot: DesktopUpdateSnapshot) => void) => () => void;
  listPets: () => Promise<PetSummary[]>;
  getPetSettings: () => Promise<PetSettings>;
  selectPet: (petId: string) => Promise<PetSettings>;
  refreshCodexPets: () => Promise<PetImportReport>;
  createPet: () => Promise<{ threadId: string }>;
  setPetOverlayOpen: (open: boolean) => Promise<PetSettings>;
  setPetImportPreferences: (input: { importCodexPets?: boolean }) => Promise<PetSettings>;
  updatePetActivity: (snapshot: PetActivitySnapshot) => void;
  onPetCatalogChanged: (callback: () => void) => () => void;
  onPetThreadActivationRequested: (callback: (threadId: string) => void) => () => void;
  openAwakenPetsDirectory: () => Promise<void>;
  openCodexPetsDirectory: () => Promise<void>;
};

function reconnectDelay(retryCount: number): number {
  return Math.min(1000 * Math.pow(2, retryCount), 10000);
}

function isTerminalEvent(event: AguiEvent): boolean {
  return event.type === "RUN_FINISHED" || event.type === "RUN_ERROR";
}

function isRunNotFoundError(message: string): boolean {
  const normalized = message.toLowerCase();
  return (
    normalized.includes("run_not_found") ||
    normalized.includes("404 page not found") ||
    normalized.includes("404 not found")
  );
}

function createElectronClient(bridge: AwakenBridge): RuntimeClient {
  return {
    source: "electron",
    platform: bridge.platform,
    relaunchDesktop: () => bridge.relaunchDesktop(),
    getRuntimeInfo: () => bridge.getRuntimeInfo(),
    getSearchSettings: () => bridge.getSearchSettings(),
    saveSearchSettings: (request) => bridge.saveSearchSettings(request),
    testSearchProvider: (request) => bridge.testSearchProvider(request),
    getACPRunners: () => bridge.getACPRunners(),
    createACPRunner: (request) => bridge.createACPRunner(request),
    updateACPRunner: (runnerId, request) => bridge.updateACPRunner(runnerId, request),
    deleteACPRunner: (runnerId) => bridge.deleteACPRunner(runnerId),
    probeACPRunner: (runnerId) => bridge.probeACPRunner(runnerId),
    listConnectors: () => bridge.listConnectors(),
    connectConnector: (connectorId) => bridge.connectConnector(connectorId),
    disconnectConnector: (connectorId) => bridge.disconnectConnector(connectorId),
    getConnectorAuthSession: bridge.getConnectorAuthSession
      ? (sessionId) => bridge.getConnectorAuthSession!(sessionId)
      : undefined,
    cancelConnectorAuthSession: bridge.cancelConnectorAuthSession
      ? (sessionId) => bridge.cancelConnectorAuthSession!(sessionId)
      : undefined,
    markConnectorAdminApprovalPending: bridge.markConnectorAdminApprovalPending
      ? (sessionId) => bridge.markConnectorAdminApprovalPending!(sessionId)
      : undefined,
    listCliConnectors: () => bridge.listCliConnectors(),
    refreshCliConnector: (providerId) => bridge.refreshCliConnector(providerId),
    installCliConnector: (providerId) => bridge.installCliConnector(providerId),
    cancelCliInstall: (providerId) => bridge.cancelCliInstall(providerId),
    startCliAuth: (providerId) => bridge.startCliAuth(providerId),
    cancelCliAuth: (providerId) => bridge.cancelCliAuth(providerId),
    retryCliAuth: (providerId) => bridge.retryCliAuth(providerId),
    setCliConnectorActivation: (providerId, activation) =>
      bridge.setCliConnectorActivation(providerId, activation),
    logoutCliConnector: (providerId) => bridge.logoutCliConnector(providerId),
    uninstallManagedCli: (providerId) => bridge.uninstallManagedCli(providerId),
    subscribeCliConnectorState: (handler) => bridge.subscribeCliConnectorState(handler),
    subscribeComposerCatalogEvents: (handler) => bridge.subscribeComposerCatalogEvents(handler),
    listMcpServers: () => bridge.listMcpServers(),
    configureMcpServers: (request) => bridge.configureMcpServers(request),
    getMcpServerConfig: (name) => bridge.getMcpServerConfig(name),
    updateMcpServerConfig: (name, request) => bridge.updateMcpServerConfig(name, request),
    getRuntimeLogs: () => bridge.getRuntimeLogs(),
    disableMcpServer: (name) => bridge.disableMcpServer(name),
    enableMcpServer: (name) => bridge.enableMcpServer(name),
    deleteMcpServer: (name) => bridge.deleteMcpServer(name),
    listMcpTools: () => bridge.listMcpTools(),
    listSkills: (projectRoot) => bridge.listSkills(projectRoot),
    getSkillPreview: (request) => bridge.getSkillPreview(request),
    installSkill: (request) => bridge.installSkill(request),
    deleteSkill: (request) => bridge.deleteSkill(request),
    disableSkill: (id) => bridge.disableSkill(id),
    enableSkill: (id) => bridge.enableSkill(id),
    listPlugins: () => bridge.listPlugins(),
    getPlugin: (name) => bridge.getPlugin(name),
    validatePlugin: (source) => bridge.validatePlugin(source),
    installPlugin: (request) => bridge.installPlugin(request),
    commitPlugin: (name, request) => bridge.commitPlugin(name, request),
    enablePlugin: (name) => bridge.enablePlugin(name),
    disablePlugin: (name) => bridge.disablePlugin(name),
    uninstallPlugin: (name) => bridge.uninstallPlugin(name),
    enablePluginServer: (name, shortName) => bridge.enablePluginServer(name, shortName),
    disablePluginServer: (name, shortName) => bridge.disablePluginServer(name, shortName),
    listPluginComponents: (name) => bridge.listPluginComponents(name),
    getPluginComponent: (name, kind, logicalIdEncoded) =>
      bridge.getPluginComponent(name, kind, logicalIdEncoded),
    updatePluginComponentSource: (name, kind, logicalIdEncoded, request) =>
      bridge.updatePluginComponentSource(name, kind, logicalIdEncoded, request),
    setPluginComponentState: (name, kind, logicalIdEncoded, enabled) =>
      bridge.setPluginComponentState(name, kind, logicalIdEncoded, enabled),
    setPluginMcpToolState: (name, logicalIdEncoded, toolName, enabled) =>
      bridge.setPluginMcpToolState(name, logicalIdEncoded, toolName, enabled),
    setPluginRuleActivation: (name, logicalIdEncoded, activation) =>
      bridge.setPluginRuleActivation(name, logicalIdEncoded, activation),
    listPluginHookReceipts: (name, logicalIdEncoded, revisionId) =>
      bridge.listPluginHookReceipts(name, logicalIdEncoded, revisionId),
    retryPluginHookReceipt: (name, logicalIdEncoded, receiptId) =>
      bridge.retryPluginHookReceipt(name, logicalIdEncoded, receiptId),
    getPluginConfiguration: (name) => bridge.getPluginConfiguration(name),
    updatePluginConfiguration: (name, request) =>
      bridge.updatePluginConfiguration(name, request),
    getPluginSetup: bridge.getPluginSetup
      ? (name) => bridge.getPluginSetup!(name)
      : undefined,
    startPluginSetup: bridge.startPluginSetup
      ? (name, setupId, request) => bridge.startPluginSetup!(name, setupId, request)
      : undefined,
    getActivePluginAuthSession: bridge.getActivePluginAuthSession
      ? (name, setupId, request) => bridge.getActivePluginAuthSession!(name, setupId, request)
      : undefined,
    getPluginAuthSession: bridge.getPluginAuthSession
      ? (name, sessionId) => bridge.getPluginAuthSession!(name, sessionId)
      : undefined,
    cancelPluginAuthSession: bridge.cancelPluginAuthSession
      ? (name, sessionId) => bridge.cancelPluginAuthSession!(name, sessionId)
      : undefined,
    markPluginAdminApprovalPending: bridge.markPluginAdminApprovalPending
      ? (name, sessionId) => bridge.markPluginAdminApprovalPending!(name, sessionId)
      : undefined,
    probePluginSetup: bridge.probePluginSetup
      ? (name, setupId, request) => bridge.probePluginSetup!(name, setupId, request)
      : undefined,
    disconnectPluginSetup: bridge.disconnectPluginSetup
      ? (name, setupId, request) => bridge.disconnectPluginSetup!(name, setupId, request)
      : undefined,
    listModelProviders: () => bridge.listModelProviders(),
    listModelProfiles: () => bridge.listModelProfiles(),
    getModelPurposeRouting: () => bridge.getModelPurposeRouting(),
    saveModelPurposeRouting: (routing) => bridge.saveModelPurposeRouting(routing),
    getAgentPlanAccount: () => bridge.getAgentPlanAccount(),
    saveAgentPlanAccount: (request) =>
      unwrapDesktopOperation(bridge.saveAgentPlanAccount(request)),
    refreshAgentPlanModels: () =>
      unwrapDesktopOperation(bridge.refreshAgentPlanModels()),
    deleteAgentPlanAccount: () => bridge.deleteAgentPlanAccount(),
    getAgentPlanMediaTools: () => bridge.getAgentPlanMediaTools(),
    saveAgentPlanMediaTool: (request) => bridge.saveAgentPlanMediaTool(request),
    deleteAgentPlanMediaTool: (capability) =>
      bridge.deleteAgentPlanMediaTool(capability),
    saveModelProfile: (request) =>
      unwrapDesktopOperation(bridge.saveModelProfile(request)),
    deleteModelProfile: (profileId) => bridge.deleteModelProfile(profileId),
    setDefaultModelProfile: (profileId) => bridge.setDefaultModelProfile(profileId),
    testModelProfile: (request) => unwrapDesktopOperation(bridge.testModelProfile(request)),
    testAgentPlanReasoningCapability: (request) =>
      unwrapDesktopOperation(bridge.testAgentPlanReasoningCapability(request)),
    syncModelCapabilities: () => bridge.syncModelCapabilities(),
    refreshModelCatalog: (request) =>
      unwrapDesktopOperation(bridge.refreshModelCatalog(request)),
    getModelProfileBalances: () => bridge.getModelProfileBalances(),
    getRun: (runId) => bridge.getRun(runId),
    getThreadTranscript: (threadId) => bridge.getThreadTranscript(threadId),
    rewindThread: (request) => bridge.rewindThread(request),
    listThreads: (view) => bridge.listThreads(view),
    renameThread: (threadId, label) => bridge.renameThread(threadId, label),
    setThreadArchived: (threadId, archived) => bridge.setThreadArchived(threadId, archived),
    updateThreadModelPreference: (threadId, request) =>
      bridge.updateThreadModelPreference(threadId, request),
    deleteThread: (threadId) => bridge.deleteThread(threadId),
    deleteThreads: (threadIds) => bridge.deleteThreads(threadIds),
    listProjects: (includeRemoved) => bridge.listProjects(includeRemoved),
    listProjectDirectory: (request) => bridge.listProjectDirectory(request),
    watchProjectDirectory: (request) => bridge.watchProjectDirectory(request),
    unwatchProjectDirectory: (subscriptionId) => bridge.unwatchProjectDirectory(subscriptionId),
    onProjectDirectoryChanged: (handler) => bridge.onProjectDirectoryChanged(handler),
    resolveProjectEntryForTask: (request) => bridge.resolveProjectEntryForTask(request),
    copyProjectEntryPath: (request) => bridge.copyProjectEntryPath(request),
    revealProjectEntry: (request) => bridge.revealProjectEntry(request),
    openProjectEntryWith: (request) => bridge.openProjectEntryWith(request),
    startProjectFilePreview: (request) => bridge.startProjectFilePreview(request),
    startArtifactPreview: (request) => bridge.startArtifactPreview(request),
    closeProjectFilePreview: (request) => bridge.closeProjectFilePreview(request),
    onProjectFilePreviewInteraction: (handler) => bridge.onProjectFilePreviewInteraction(handler),
    openProjectFileExternally: (request) => bridge.openProjectFileExternally(request),
    revealProjectFile: (request) => bridge.revealProjectFile(request),
    saveProjectMarkdownFile: (request) => bridge.saveProjectMarkdownFile(request),
		createProjectAgentsFile: bridge.createProjectAgentsFile
			? (projectId) => bridge.createProjectAgentsFile!(projectId)
			: undefined,
		getGlobalAgentsDocument: bridge.getGlobalAgentsDocument
			? () => bridge.getGlobalAgentsDocument!()
			: undefined,
		saveGlobalAgentsDocument: bridge.saveGlobalAgentsDocument
			? (request) => bridge.saveGlobalAgentsDocument!(request)
			: undefined,
		deleteGlobalAgentsDocument: bridge.deleteGlobalAgentsDocument
			? (request) => bridge.deleteGlobalAgentsDocument!(request)
			: undefined,
    upsertProject: (workspaceRoot, displayName) => bridge.upsertProject(workspaceRoot, displayName),
    createProject: (folders, displayName) => bridge.createProject(folders, displayName),
    renameProject: (projectId, displayName) => bridge.renameProject(projectId, displayName),
    attachProjectFolder: (projectId, path) => bridge.attachProjectFolder(projectId, path),
    detachProjectFolder: (projectId, path) => bridge.detachProjectFolder(projectId, path),
    setPrimaryProjectFolder: (projectId, path) => bridge.setPrimaryProjectFolder(projectId, path),
    reorderProjects: (projectIds) => bridge.reorderProjects(projectIds),
    detachProject: (projectId, idempotencyKey) => bridge.detachProject(projectId, idempotencyKey),
    deleteProjectThreads: (projectId, idempotencyKey) =>
      bridge.deleteProjectThreads(projectId, idempotencyKey),
    trashProjectFolder: (projectId, path, idempotencyKey) =>
      bridge.trashProjectFolder(projectId, path, idempotencyKey),
    getProjectOperation: (operationId) => bridge.getProjectOperation(operationId),
    openArtifact: (request) => bridge.openArtifact(request),
    getArtifactMediaSource: (request) => bridge.getArtifactMediaSource(request),
    revealPath: (target) => bridge.revealPath(target),
    revealArtifact: (request) => bridge.revealArtifact(request),
    startRun: (request) => unwrapDesktopOperation(bridge.startRun(request)),
    cancelRun: bridge.cancelRun,
    cancelSubagent: bridge.cancelSubagent
      ? (runId, taskId, reason) => bridge.cancelSubagent!(runId, taskId, reason)
      : undefined,
    messageSubagent: bridge.messageSubagent
      ? (runId, taskId, content, idempotencyKey) =>
          bridge.messageSubagent!(runId, taskId, content, idempotencyKey)
      : undefined,
    answerUserQuestion: bridge.answerUserQuestion,
    cancelUserQuestion: bridge.cancelUserQuestion,
    resolveExternalInteraction: bridge.resolveExternalInteraction,
    resolveACPPermission: bridge.resolveACPPermission,
    resolveComputerReview: bridge.resolveComputerReview,
    computerSessionControl: bridge.computerSessionControl,
    getComputerHostStatus: () => bridge.getComputerHostStatus(),
    getComputerSettings: () => bridge.getComputerSettings(),
    saveComputerSettings: (request) => bridge.saveComputerSettings(request),
    prepareComputerRuntime: () => bridge.prepareComputerRuntime(),
    requestComputerPermissions: () => bridge.requestComputerPermissions(),
    openComputerPermissionSettings: (permission) =>
      bridge.openComputerPermissionSettings(permission),
    getDesktopPreferences: () => bridge.getDesktopPreferences(),
    saveDesktopPreferences: (request) => bridge.saveDesktopPreferences(request),
    listMemories: (request) => bridge.listMemories(request),
    getMemory: (memoryId, projectId) => bridge.getMemory(memoryId, projectId),
    deleteMemory: (memoryId, expectedRevision, projectId) =>
      bridge.deleteMemory(memoryId, expectedRevision, projectId),
    getMemorySettings: () => bridge.getMemorySettings(),
		getHookStatus: bridge.getHookStatus
			? (projectId) => bridge.getHookStatus!(projectId)
			: undefined,
		trustProjectHook: bridge.trustProjectHook
			? (request) => bridge.trustProjectHook!(request)
			: undefined,
		revokeProjectHook: bridge.revokeProjectHook
			? (request) => bridge.revokeProjectHook!(request)
			: undefined,
    updateMemorySettings: (request) => bridge.updateMemorySettings(request),
    startMemoryDream: (projectId) => bridge.startMemoryDream(projectId),
    getMemoryDream: (taskId) => bridge.getMemoryDream(taskId),
    listMemoryDreams: (projectId, limit) => bridge.listMemoryDreams(projectId, limit),
    cancelMemoryDream: (taskId) => bridge.cancelMemoryDream(taskId),
    onMemoryDreamReceipt: (handler) => bridge.onMemoryDreamReceipt(handler),
    subscribeRun: async (runId, handlers, afterCursor) => {
      let lastCursor = afterCursor ?? 0;
      let terminalSeen = false;
      let retryCount = 0;
      let currentSubscriptionId: string | null = null;
      let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
      let disposed = false;

      const wrappedHandlers = {
        onEvent: (event: AguiEvent) => {
          const cursor = event.streamCursor ?? event.seq;
          if (cursor !== undefined && cursor > lastCursor) {
            lastCursor = cursor;
          }
          if (isTerminalEvent(event)) {
            terminalSeen = true;
          }
          handlers.onEvent(event);
        },
        onError: (message: string) => {
          if (disposed || terminalSeen) return;
          attemptReconnect(message);
        },
        onClose: () => {
          if (disposed) return;
          if (terminalSeen) {
            handlers.onClose?.();
            return;
          }
          attemptReconnect("Runtime stream closed before a terminal event");
        },
      };

      function attemptReconnect(reason: string): void {
        if (disposed || terminalSeen) return;
        // Reentrancy guard: if a reconnect is already pending, ignore
        // additional onError/onClose triggers that fire in the same window.
        if (reconnectTimer) return;
        if (isRunNotFoundError(reason)) {
          handlers.onError?.(
            `Run session was not found on the server (it may have been lost after restart): ${reason}`,
          );
          return;
        }

        handlers.onReconnect?.();
        const delay = reconnectDelay(retryCount);
        retryCount += 1;

        if (currentSubscriptionId) {
          bridge.unsubscribeRun(currentSubscriptionId);
          currentSubscriptionId = null;
        }

        reconnectTimer = setTimeout(() => {
          reconnectTimer = null;
          if (disposed || terminalSeen) return;
          void resubscribe();
        }, delay);
      }

      async function resubscribe(): Promise<void> {
        if (disposed || terminalSeen) return;
        try {
          currentSubscriptionId = await bridge.subscribeRun(runId, wrappedHandlers, lastCursor);
        } catch (error) {
          if (disposed || terminalSeen) return;
          attemptReconnect(error instanceof Error ? error.message : "Reconnection failed");
        }
      }

      try {
        currentSubscriptionId = await bridge.subscribeRun(runId, wrappedHandlers, lastCursor);
      } catch (error) {
        if (disposed) throw error;
        attemptReconnect(error instanceof Error ? error.message : "Initial subscription failed");
      }

      return () => {
        disposed = true;
        if (reconnectTimer) {
          clearTimeout(reconnectTimer);
          reconnectTimer = null;
        }
        if (currentSubscriptionId) {
          bridge.unsubscribeRun(currentSubscriptionId);
          currentSubscriptionId = null;
        }
      };
    },
    pickAttachments: (defaultPath) => bridge.pickAttachments(defaultPath),
    pickDirectory: (defaultPath) => bridge.pickDirectory(defaultPath),
    pickPluginSource: (defaultPath) => bridge.pickPluginSource(defaultPath),
    resolveDroppedPluginSource: (file) => bridge.resolveDroppedPluginSource(file),
    pickProjectFolders: (defaultPath) => bridge.pickProjectFolders(defaultPath),
    validateWorkspace: (path) => bridge.validateWorkspace(path),
    getSupervisorStatus: () => bridge.getSupervisorStatus(),
    onSupervisorStatusChange: (callback) => bridge.onSupervisorStatusChange(callback),
    notifyRendererReady: () => bridge.notifyRendererReady(),
    listScheduledTasks: () => bridge.listScheduledTasks(),
    getScheduledTask: (taskId) => bridge.getScheduledTask(taskId),
    createScheduledTask: (input) => bridge.createScheduledTask(input),
    patchScheduledTask: (taskId, input) => bridge.patchScheduledTask(taskId, input),
    deleteScheduledTask: (taskId, expectedVersion, idempotencyKey) =>
      bridge.deleteScheduledTask(taskId, expectedVersion, idempotencyKey),
    pauseScheduledTask: (taskId, expectedVersion) =>
      bridge.pauseScheduledTask(taskId, expectedVersion),
    resumeScheduledTask: (taskId, expectedVersion) =>
      bridge.resumeScheduledTask(taskId, expectedVersion),
    runScheduledTaskNow: (taskId, idempotencyKey) =>
      bridge.runScheduledTaskNow(taskId, idempotencyKey),
    cancelScheduledTaskExecution: (taskId) => bridge.cancelScheduledTaskExecution(taskId),
    listScheduledTaskExecutions: (taskId) => bridge.listScheduledTaskExecutions(taskId),
    getSystemTimezone: () => bridge.getSystemTimezone(),
    subscribeScheduledTaskEvents: (handler) => bridge.subscribeScheduledTaskEvents(handler),
    onScheduledTaskThreadActivation: (handler) => bridge.onScheduledTaskThreadActivation(handler),
    getDesktopUpdateState: () => bridge.getDesktopUpdateState(),
    checkDesktopUpdate: () => bridge.checkDesktopUpdate(),
    downloadDesktopUpdate: () => bridge.downloadDesktopUpdate(),
    restartAndInstallDesktopUpdate: () => bridge.restartAndInstallDesktopUpdate(),
    onDesktopUpdateStateChanged: (handler) => bridge.onDesktopUpdateStateChanged(handler),
    listPets: () => bridge.listPets(),
    getPetSettings: () => bridge.getPetSettings(),
    selectPet: (petId) => bridge.selectPet(petId),
    refreshCodexPets: () => bridge.refreshCodexPets(),
    createPet: () => bridge.createPet(),
    setPetOverlayOpen: (open) => bridge.setPetOverlayOpen(open),
    setPetImportPreferences: (input) => bridge.setPetImportPreferences(input),
    updatePetActivity: (snapshot) => bridge.updatePetActivity(snapshot),
    onPetCatalogChanged: (callback) => bridge.onPetCatalogChanged(callback),
    onPetThreadActivationRequested: (callback) => bridge.onPetThreadActivationRequested(callback),
    openAwakenPetsDirectory: () => bridge.openAwakenPetsDirectory(),
    openCodexPetsDirectory: () => bridge.openCodexPetsDirectory(),
  };
}

export function createRuntimeClient(): RuntimeClient {
  if (!window.awaken) {
    throw new Error("Awaken Desktop must run through the Electron window.awaken bridge");
  }
  return createElectronClient(window.awaken);
}
