import { runtimeRequestIssue, unwrapRuntimeRequest } from "../../shared/runtime-request";
import { reconnectingSubscription } from "./reconnecting-subscription";
import { threadActivitySubscriber } from "./activity-client";
import type { DesktopUpdateSnapshot } from "../../shared/desktop-update";
import type {
  PetActivitySnapshot,
  PetImportReport,
  PetSettings,
  PetSummary,
} from "../../shared/pets";
import type { ProjectFilePreviewInteraction } from "../../shared/project-file-preview-interaction";
import type { EmbeddedBrowserBridge } from "../../shared/embedded-browser";
import {
  type ACPRunnerCreateRequest,
  type ACPRunnerProbeResult,
  type ACPRunnersSettings,
  type ACPRunnerUpdateRequest,
  type ACPRunnerView,
  type AgentPlanAccount,
  type AgentPlanAccountSaveRequest,
  type AgentPlanMediaCapability,
  type AgentPlanMediaTools,
  type AgentPlanMediaToolSaveRequest,
  type AgentPlanMediaToolsSaveResult,
  type ArtifactMediaSourceResult,
  type ArtifactOpenRequest,
  type ArtifactOpenResult,
  type ArtifactPreviewStartRequest,
  type AttachmentPickResult,
  type AwakenBridge,
  type CliActivation,
  type CliConnectorStateHandler,
  type CliConnectorView,
  type CliProviderId,
  type CommitPluginRequest,
  type ComposerCatalogEvent,
  type ComputerHostStatus,
  type ComputerSettingsDocument,
  type ComputerSettingsSaveRequest,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type DeleteSkillRequest,
  type DeleteSkillResponse,
  type DesktopOperationIssue,
  type DesktopOperationRecoveryAction,
  type DesktopOperationResult,
  type DesktopPlatform,
  type DesktopPreferences,
  type DesktopPreferencesSaveRequest,
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
  type MemoryDreamTask,
  type MemoryListRequest,
  type MemorySettingsDocument,
  type MemorySettingsPatch,
  type ModelCatalogRefreshResult,
  type ModelProfileBalancesResult,
  type ModelProfileSaveRequest,
  type ModelProfileTestResult,
  type ModelProvider,
  type ModelPurposeRouting,
  type ModelReasoningCapabilityTestResult,
  type ProjectAgentsCreateResult,
  type ProjectDirectoryChangedEvent,
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
  type ProjectFolderSelection,
  type ProjectFoldersPickResult,
  type ProjectMutationResult,
  type ProjectReorderResult,
  type RewindThreadRequest,
  type RewindThreadResult,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RuntimeConnectorAuthSession,
  type RuntimeInfo,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeMemory,
  type RuntimeModelProfileSummary,
  type RuntimePlugin,
  type RuntimePluginAuthSession,
  type RuntimePluginComponentDetail,
  type RuntimePluginComponentKind,
  type RuntimePluginComponentSourceResponse,
  type RuntimePluginComponentState,
  type RuntimePluginComponentStateResponse,
  type RuntimePluginConfigurationResponse,
  type RuntimePluginHookReceipt,
  type RuntimePluginHookReceiptResponse,
  type RuntimePluginSetupMutationRequest,
  type RuntimePluginSetupResponse,
  type RuntimePluginSource,
  type RuntimePluginValidation,
  type RuntimeProject,
  type RuntimeProjectOperation,
  type RuntimeRun,
  type RuntimeSkill,
  type RuntimeThread,
  type RuntimeThreadListView,
  type RuntimeThreadTranscript,
  type RuntimeToolInfo,
  type ScheduledTask,
  type ScheduledTaskCreateInput,
  type ScheduledTaskEvent,
  type ScheduledTaskExecution,
  type ScheduledTaskPatchInput,
  type SearchProviderTestRequest,
  type SearchProviderTestResult,
  type SearchSettings,
  type SearchSettingsSaveRequest,
  type SearchSettingsSaveResult,
  type SetThreadArchivedResult,
  type SkillPreviewResponse,
  type SkillProjectRoots,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type ThreadBatchDeleteResult,
  type ThreadDeleteResult,
  type ThreadModelPreferenceRequest,
  type UpdateMcpServerConfigRequest,
  type UpdatePluginComponentSourceRequest,
  type UpdatePluginConfigurationRequest,
  type UpsertProjectResult,
  type WorkspacePickResult,
  type WorkspaceValidationResult
} from "../../shared/runtime";
import type { AguiEvent } from "./agui";

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
  getConnectorAuthSession: (sessionId: string) => Promise<RuntimeConnectorAuthSession>;
  cancelConnectorAuthSession: (sessionId: string) => Promise<RuntimeConnectorAuthSession>;
  markConnectorAdminApprovalPending: (sessionId: string) => Promise<RuntimeConnectorAuthSession>;
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
  getPluginSetup: (name: string) => Promise<RuntimePluginSetupResponse>;
  startPluginSetup: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginAuthSession>;
  getActivePluginAuthSession: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginAuthSession | null>;
  getPluginAuthSession: (name: string, sessionId: string) => Promise<RuntimePluginAuthSession>;
  cancelPluginAuthSession: (name: string, sessionId: string) => Promise<RuntimePluginAuthSession>;
  markPluginAdminApprovalPending: (name: string, sessionId: string) => Promise<RuntimePluginAuthSession>;
  probePluginSetup: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginSetupResponse>;
  disconnectPluginSetup: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<RuntimePluginSetupResponse>;
  listModelProviders: () => Promise<ModelProvider[]>;
  listModelProfiles: () => Promise<RuntimeModelProfileSummary[]>;
  getModelPurposeRouting: () => Promise<ModelPurposeRouting>;
  saveModelPurposeRouting: (routing: ModelPurposeRouting) => Promise<ModelPurposeRouting>;
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
  embeddedBrowser: EmbeddedBrowserBridge;
  openProjectFileExternally: (
    request: ProjectFilePreviewActionRequest,
  ) => Promise<ProjectFilePreviewActionResult>;
  revealProjectFile: (
    request: ProjectFilePreviewActionRequest,
  ) => Promise<ProjectFilePreviewActionResult>;
  saveProjectMarkdownFile: (
    request: ProjectFilePreviewSaveRequest,
  ) => Promise<ProjectFilePreviewSaveResult>;
	createProjectAgentsFile: (projectId: string) => Promise<ProjectAgentsCreateResult>;
	getGlobalAgentsDocument: () => Promise<GlobalAgentsDocument>;
	saveGlobalAgentsDocument: (request: GlobalAgentsSaveRequest) => Promise<GlobalAgentsDocument>;
	deleteGlobalAgentsDocument: (request: GlobalAgentsDeleteRequest) => Promise<GlobalAgentsDocument>;
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
  cancelSubagent: (runId: string, taskId: string, reason?: string) => Promise<void>;
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
    sessionId?: string,
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
	getHookStatus: (projectId?: string) => Promise<HookStatus>;
	trustProjectHook: (request: HookTrustRequest) => Promise<HookStatus>;
	revokeProjectHook: (request: HookRevokeRequest) => Promise<HookStatus>;
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
  subscribeThreadActivity?: (threadId: string, handlers: SubscribeHandlers, afterCursor?: number) => Promise<() => void>;
  stopThreadAgents?: (threadId: string) => Promise<void>;
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

function isTerminalEvent(event: AguiEvent): boolean {
  return event.type === "RUN_FINISHED" || event.type === "RUN_ERROR"
    || (event.type === "RUN_STATUS_CHANGED" && (event.status === "cancelled" || event.status === "finished" || event.status === "error"));
}

function createElectronClient(bridge: AwakenBridge): RuntimeClient {
  return {
    subscribeThreadActivity: threadActivitySubscriber(bridge),
    stopThreadAgents: bridge.stopThreadAgents ? (threadId) => unwrapRuntimeRequest(bridge.stopThreadAgents!(threadId)) : undefined,
    source: "electron",
    platform: bridge.platform,
    relaunchDesktop: () => bridge.relaunchDesktop(),
    getRuntimeInfo: () => bridge.getRuntimeInfo(),
    getSearchSettings: () => bridge.getSearchSettings(),
    saveSearchSettings: (request) => bridge.saveSearchSettings(request),
    testSearchProvider: (request) => bridge.testSearchProvider(request),
    getACPRunners: () => unwrapRuntimeRequest(bridge.getACPRunners()),
    createACPRunner: (request) => unwrapRuntimeRequest(bridge.createACPRunner(request)),
    updateACPRunner: (runnerId, request) => unwrapRuntimeRequest(bridge.updateACPRunner(runnerId, request)),
    deleteACPRunner: (runnerId) => unwrapRuntimeRequest(bridge.deleteACPRunner(runnerId)),
    probeACPRunner: (runnerId) => unwrapRuntimeRequest(bridge.probeACPRunner(runnerId)),
    listConnectors: () => bridge.listConnectors(),
    connectConnector: (connectorId) => bridge.connectConnector(connectorId),
    disconnectConnector: (connectorId) => bridge.disconnectConnector(connectorId),
    getConnectorAuthSession: (sessionId) => bridge.getConnectorAuthSession(sessionId),
    cancelConnectorAuthSession: (sessionId) => bridge.cancelConnectorAuthSession(sessionId),
    markConnectorAdminApprovalPending: (sessionId) => bridge.markConnectorAdminApprovalPending(sessionId),
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
    listPlugins: () => unwrapRuntimeRequest(bridge.listPlugins()),
    getPlugin: (name) => unwrapRuntimeRequest(bridge.getPlugin(name)),
    validatePlugin: (source) => unwrapRuntimeRequest(bridge.validatePlugin(source)),
    installPlugin: (request) => unwrapRuntimeRequest(bridge.installPlugin(request)),
    commitPlugin: (name, request) => unwrapRuntimeRequest(bridge.commitPlugin(name, request)),
    enablePlugin: (name) => unwrapRuntimeRequest(bridge.enablePlugin(name)),
    disablePlugin: (name) => unwrapRuntimeRequest(bridge.disablePlugin(name)),
    uninstallPlugin: (name) => unwrapRuntimeRequest(bridge.uninstallPlugin(name)),
    enablePluginServer: (name, shortName) => unwrapRuntimeRequest(bridge.enablePluginServer(name, shortName)),
    disablePluginServer: (name, shortName) => unwrapRuntimeRequest(bridge.disablePluginServer(name, shortName)),
    listPluginComponents: (name) => unwrapRuntimeRequest(bridge.listPluginComponents(name)),
    getPluginComponent: (name, kind, logicalIdEncoded) =>
      unwrapRuntimeRequest(bridge.getPluginComponent(name, kind, logicalIdEncoded)),
    updatePluginComponentSource: (name, kind, logicalIdEncoded, request) =>
      unwrapRuntimeRequest(bridge.updatePluginComponentSource(name, kind, logicalIdEncoded, request)),
    setPluginComponentState: (name, kind, logicalIdEncoded, enabled) =>
      unwrapRuntimeRequest(bridge.setPluginComponentState(name, kind, logicalIdEncoded, enabled)),
    setPluginMcpToolState: (name, logicalIdEncoded, toolName, enabled) =>
      unwrapRuntimeRequest(bridge.setPluginMcpToolState(name, logicalIdEncoded, toolName, enabled)),
    setPluginRuleActivation: (name, logicalIdEncoded, activation) =>
      unwrapRuntimeRequest(bridge.setPluginRuleActivation(name, logicalIdEncoded, activation)),
    listPluginHookReceipts: (name, logicalIdEncoded, revisionId) =>
      unwrapRuntimeRequest(bridge.listPluginHookReceipts(name, logicalIdEncoded, revisionId)),
    retryPluginHookReceipt: (name, logicalIdEncoded, receiptId) =>
      unwrapRuntimeRequest(bridge.retryPluginHookReceipt(name, logicalIdEncoded, receiptId)),
    getPluginConfiguration: (name) => unwrapRuntimeRequest(bridge.getPluginConfiguration(name)),
    updatePluginConfiguration: (name, request) =>
      unwrapRuntimeRequest(bridge.updatePluginConfiguration(name, request)),
    getPluginSetup: (name) => unwrapRuntimeRequest(bridge.getPluginSetup(name)),
    startPluginSetup: (name, setupId, request) => unwrapRuntimeRequest(bridge.startPluginSetup(name, setupId, request)),
    getActivePluginAuthSession: (name, setupId, request) => unwrapRuntimeRequest(bridge.getActivePluginAuthSession(name, setupId, request)),
    getPluginAuthSession: (name, sessionId) => unwrapRuntimeRequest(bridge.getPluginAuthSession(name, sessionId)),
    cancelPluginAuthSession: (name, sessionId) => unwrapRuntimeRequest(bridge.cancelPluginAuthSession(name, sessionId)),
    markPluginAdminApprovalPending: (name, sessionId) => unwrapRuntimeRequest(bridge.markPluginAdminApprovalPending(name, sessionId)),
    probePluginSetup: (name, setupId, request) => unwrapRuntimeRequest(bridge.probePluginSetup(name, setupId, request)),
    disconnectPluginSetup: (name, setupId, request) => unwrapRuntimeRequest(bridge.disconnectPluginSetup(name, setupId, request)),
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
    getRun: (runId) => unwrapRuntimeRequest(bridge.getRun(runId)),
    getThreadTranscript: (threadId) => unwrapRuntimeRequest(bridge.getThreadTranscript(threadId)),
    rewindThread: (request) => unwrapRuntimeRequest(bridge.rewindThread(request)),
    listThreads: (view) => unwrapRuntimeRequest(bridge.listThreads(view)),
    renameThread: (threadId, label) => unwrapRuntimeRequest(bridge.renameThread(threadId, label)),
    setThreadArchived: (threadId, archived) => unwrapRuntimeRequest(bridge.setThreadArchived(threadId, archived)),
    updateThreadModelPreference: (threadId, request) =>
      unwrapRuntimeRequest(bridge.updateThreadModelPreference(threadId, request)),
    deleteThread: (threadId) => unwrapRuntimeRequest(bridge.deleteThread(threadId)),
    deleteThreads: (threadIds) => unwrapRuntimeRequest(bridge.deleteThreads(threadIds)),
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
    embeddedBrowser: bridge.embeddedBrowser,
    openProjectFileExternally: (request) => bridge.openProjectFileExternally(request),
    revealProjectFile: (request) => bridge.revealProjectFile(request),
    saveProjectMarkdownFile: (request) => bridge.saveProjectMarkdownFile(request),
		createProjectAgentsFile: (projectId) => bridge.createProjectAgentsFile(projectId),
		getGlobalAgentsDocument: () => bridge.getGlobalAgentsDocument(),
		saveGlobalAgentsDocument: (request) => bridge.saveGlobalAgentsDocument(request),
		deleteGlobalAgentsDocument: (request) => bridge.deleteGlobalAgentsDocument(request),
    upsertProject: (workspaceRoot, displayName) => bridge.upsertProject(workspaceRoot, displayName),
    createProject: (folders, displayName) => bridge.createProject(folders, displayName),
    renameProject: (projectId, displayName) => bridge.renameProject(projectId, displayName),
    attachProjectFolder: (projectId, path) => bridge.attachProjectFolder(projectId, path),
    detachProjectFolder: (projectId, path) => bridge.detachProjectFolder(projectId, path),
    setPrimaryProjectFolder: (projectId, path) => bridge.setPrimaryProjectFolder(projectId, path),
    reorderProjects: (projectIds) => bridge.reorderProjects(projectIds),
    detachProject: (projectId, idempotencyKey) => bridge.detachProject(projectId, idempotencyKey),
    deleteProjectThreads: (projectId, idempotencyKey) =>
      unwrapRuntimeRequest(bridge.deleteProjectThreads(projectId, idempotencyKey)),
    trashProjectFolder: (projectId, path, idempotencyKey) =>
      bridge.trashProjectFolder(projectId, path, idempotencyKey),
    getProjectOperation: (operationId) => bridge.getProjectOperation(operationId),
    openArtifact: (request) => bridge.openArtifact(request),
    getArtifactMediaSource: (request) => bridge.getArtifactMediaSource(request),
    revealPath: (target) => bridge.revealPath(target),
    revealArtifact: (request) => bridge.revealArtifact(request),
    startRun: (request) => unwrapDesktopOperation(bridge.startRun(request)),
    cancelRun: (...args) => unwrapRuntimeRequest(bridge.cancelRun(...args)),
    cancelSubagent: (runId, taskId, reason) => unwrapRuntimeRequest(bridge.cancelSubagent(runId, taskId, reason)),
    answerUserQuestion: (...args) => unwrapRuntimeRequest(bridge.answerUserQuestion(...args)),
    cancelUserQuestion: (...args) => unwrapRuntimeRequest(bridge.cancelUserQuestion(...args)),
    resolveExternalInteraction: (...args) => unwrapRuntimeRequest(bridge.resolveExternalInteraction(...args)),
    resolveACPPermission: (...args) => unwrapRuntimeRequest(bridge.resolveACPPermission(...args)),
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
		getHookStatus: (projectId) => bridge.getHookStatus(projectId),
		trustProjectHook: (request) => bridge.trustProjectHook(request),
		revokeProjectHook: (request) => bridge.revokeProjectHook(request),
    updateMemorySettings: (request) => bridge.updateMemorySettings(request),
    startMemoryDream: (projectId) => bridge.startMemoryDream(projectId),
    getMemoryDream: (taskId) => bridge.getMemoryDream(taskId),
    listMemoryDreams: (projectId, limit) => bridge.listMemoryDreams(projectId, limit),
    cancelMemoryDream: (taskId) => bridge.cancelMemoryDream(taskId),
    onMemoryDreamReceipt: (handler) => bridge.onMemoryDreamReceipt(handler),
    subscribeRun: (runId, handlers, afterCursor) => reconnectingSubscription({
      connect: (callbacks, cursor) => unwrapRuntimeRequest(bridge.subscribeRun(runId, callbacks, cursor)),
      disconnect: id => bridge.unsubscribeRun(id),
      handlers,
      cursor: afterCursor,
      eventCursor: event => event.streamCursor ?? event.seq,
      terminalEvent: isTerminalEvent,
      terminalError: error => {
        const issue = runtimeRequestIssue(error);
        return issue.code === "RUN_NOT_FOUND"
          ? `Run session was not found on the server (it may have been lost after restart): ${issue.message}`
          : undefined;
      },
    }),
    pickAttachments: (defaultPath) => bridge.pickAttachments(defaultPath),
    pickDirectory: (defaultPath) => bridge.pickDirectory(defaultPath),
    pickPluginSource: (defaultPath) => unwrapRuntimeRequest(bridge.pickPluginSource(defaultPath)),
    resolveDroppedPluginSource: (file) => unwrapRuntimeRequest(bridge.resolveDroppedPluginSource(file)),
    pickProjectFolders: (defaultPath) => bridge.pickProjectFolders(defaultPath),
    validateWorkspace: (path) => bridge.validateWorkspace(path),
    getSupervisorStatus: () => bridge.getSupervisorStatus(),
    onSupervisorStatusChange: (callback) => bridge.onSupervisorStatusChange(callback),
    notifyRendererReady: () => bridge.notifyRendererReady(),
    listScheduledTasks: () => unwrapRuntimeRequest(bridge.listScheduledTasks()),
    getScheduledTask: (taskId) => unwrapRuntimeRequest(bridge.getScheduledTask(taskId)),
    createScheduledTask: (input) => unwrapRuntimeRequest(bridge.createScheduledTask(input)),
    patchScheduledTask: (taskId, input) => unwrapRuntimeRequest(bridge.patchScheduledTask(taskId, input)),
    deleteScheduledTask: (taskId, expectedVersion, idempotencyKey) =>
      unwrapRuntimeRequest(bridge.deleteScheduledTask(taskId, expectedVersion, idempotencyKey)),
    pauseScheduledTask: (taskId, expectedVersion) =>
      unwrapRuntimeRequest(bridge.pauseScheduledTask(taskId, expectedVersion)),
    resumeScheduledTask: (taskId, expectedVersion) =>
      unwrapRuntimeRequest(bridge.resumeScheduledTask(taskId, expectedVersion)),
    runScheduledTaskNow: (taskId, idempotencyKey) =>
      unwrapRuntimeRequest(bridge.runScheduledTaskNow(taskId, idempotencyKey)),
    cancelScheduledTaskExecution: (taskId) => unwrapRuntimeRequest(bridge.cancelScheduledTaskExecution(taskId)),
    listScheduledTaskExecutions: (taskId) => unwrapRuntimeRequest(bridge.listScheduledTaskExecutions(taskId)),
    getSystemTimezone: () => unwrapRuntimeRequest(bridge.getSystemTimezone()),
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
