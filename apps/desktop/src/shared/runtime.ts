import type { AguiEvent } from "./agui.js";
import type {
  PetActivitySnapshot,
  PetBridge,
} from "./pets.js";

export type DesktopPlatform = "macos" | "windows" | "linux";

export type DesktopOperationIssueCode =
  | "credential_unreadable"
  | "operation_failed";

export type DesktopOperationRecoveryAction =
  | "retry"
  | "relaunch"
  | "reenter";

export type DesktopOperationIssue = {
  code: DesktopOperationIssueCode;
  message: string;
  retryable: boolean;
  actions: DesktopOperationRecoveryAction[];
};

export type DesktopOperationResult<T> =
  | { ok: true; value: T }
  | { ok: false; issue: DesktopOperationIssue };

export type AwakenTheme = "light" | "dark";
export type AwakenThemePreference = "system" | AwakenTheme;

export type DesktopPreferences = {
  theme: AwakenThemePreference;
};

export type DesktopPreferencesSaveRequest = {
  theme?: AwakenThemePreference;
};

export type MemoryScope = "user" | "project";
export type MemoryType = "user" | "feedback" | "project" | "reference";

export type RuntimeMemory = {
  memoryId: string;
  name: string;
  description: string;
  type: MemoryType;
  status: "active" | "superseded";
  revision: number;
  updatedAt: string;
  sourceThreadId: string;
  sourceRecordId: string;
  scope: MemoryScope;
  content?: string;
};

export type MemoryListRequest = {
  query?: string;
  scope?: MemoryScope;
  projectId?: string;
  limit?: number;
  type?: MemoryType;
  status?: "active" | "superseded";
};

export type MemorySettings = {
  enabled: boolean;
  automaticRecall: boolean;
  proactiveMemory: boolean;
  autoDream: boolean;
};

export type MemorySettingsDocument = {
  revision: number;
  value: MemorySettings;
};

export type MemorySettingsPatch = Partial<MemorySettings> & {
  expectedRevision: number;
};

export type MemoryDreamStatus =
  | "queued"
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "interrupted";

export type MemoryDreamTask = {
  taskId: string;
  status: MemoryDreamStatus;
  phase?: string;
  projectId?: string;
  scopes: MemoryScope[];
  reports?: Partial<Record<MemoryScope, { updated: number; superseded: number; conflicts: number; skipped: number }>>;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type RuntimeRunStatus =
  | "running"
  | "cancelling"
  | "finished"
  | "error"
  | "cancelled";

export type ThreadTitleSource = "first_user" | "local" | "llm" | "manual";

export type ThreadTitleStatus = "pending" | "ready" | "failed";

export type RunInputAttachmentKind = "image" | "file";

export type RunInputAttachmentWire = {
  id: string;
  kind: RunInputAttachmentKind;
  mime_type: string;
  path?: string;
  data_base64?: string;
  display_name?: string;
  width?: number;
  height?: number;
  byte_size?: number;
};

export type RunInputAttachment = {
  id: string;
  kind: RunInputAttachmentKind;
  mimeType: string;
  path?: string;
  dataBase64?: string;
  displayName?: string;
  width?: number;
  height?: number;
  byteSize?: number;
};

export type PickedAttachmentFile = {
  id: string;
  kind: RunInputAttachmentKind;
  path: string;
  mimeType: string;
  displayName: string;
  byteSize: number;
  dataBase64?: string;
  width?: number;
  height?: number;
};

export type AttachmentPickResult =
  | { canceled: true; error?: string }
  | { canceled: false; files: PickedAttachmentFile[]; errors?: string[] };

export type RuntimeRunWire = {
  run_id: string;
  thread_id: string;
  input?: string;
  input_attachments?: RunInputAttachmentWire[];
  composer_tokens?: ComposerToken[];
  status: RuntimeRunStatus;
  result?: string;
  error?: string;
  cancel_requested?: boolean;
  created_at: string;
  updated_at: string;
  finished_at?: string | null;
};

export type RuntimeRun = {
  runId: string;
  threadId: string;
  input?: string;
  inputAttachments?: RunInputAttachment[];
  composerTokens?: ComposerToken[];
  status: RuntimeRunStatus;
  result?: string;
  error?: string;
  cancelRequested: boolean;
  createdAt: string;
  updatedAt: string;
  finishedAt?: string;
};

export type RuntimeThreadWire = {
  thread_id: string;
  project_id?: string;
  label: string;
  title_source?: ThreadTitleSource;
  title_status?: ThreadTitleStatus;
  title_updated_at?: string;
  title_seed_run_id?: string;
  run_id?: string;
  active_run_id?: string;
  workspace: WorkspaceConfigWire;
  created_at: string;
  updated_at: string;
};

export type RuntimeThread = {
  threadId: string;
  projectId?: string;
  label: string;
  titleSource?: ThreadTitleSource;
  titleStatus?: ThreadTitleStatus;
  titleUpdatedAt?: string;
  titleSeedRunId?: string;
  runId?: string;
  activeRunId?: string;
  workspace: WorkspaceConfig;
  createdAt: string;
  updatedAt: string;
};

export type RuntimeProjectState = "active" | "removed" | "trash_pending";

/** A folder Awaken created and owns cannot be detached or moved to the trash. */
export type RuntimeProjectFolderRole = "user" | "managed";

export type RuntimeProjectFolderWire = {
  path: string;
  role: RuntimeProjectFolderRole;
  is_primary: boolean;
};

export type RuntimeProjectFolder = {
  path: string;
  role: RuntimeProjectFolderRole;
  isPrimary: boolean;
};

export type RuntimeProjectWire = {
  project_id: string;
  /** Mirrors the primary folder path. `folders` is authoritative. */
  workspace_root: string;
  folders?: RuntimeProjectFolderWire[];
  display_name: string;
  state: RuntimeProjectState;
  removal_reason?: "detached" | "trashed" | string;
  removed_at?: string;
  created_at: string;
  updated_at: string;
  version: number;
};

export type RuntimeProject = {
  projectId: string;
  workspaceRoot: string;
  folders: RuntimeProjectFolder[];
  displayName: string;
  state: RuntimeProjectState;
  removalReason?: string;
  removedAt?: string;
  createdAt: string;
  updatedAt: string;
  version: number;
};

export type ProjectDirectoryEntryKind = "directory" | "file" | "symlink";

export type ProjectDirectoryEntry = {
  name: string;
  /** Project-folder-relative path using forward slashes on every platform. */
  relativePath: string;
  kind: ProjectDirectoryEntryKind;
};

export type ProjectDirectoryListRequest = {
  projectId: string;
  folderPath: string;
  /** Empty or omitted means the selected project-folder root. */
  relativePath?: string;
};

export type ProjectDirectoryListResult =
  | {
      ok: true;
      relativePath: string;
      entries: ProjectDirectoryEntry[];
    }
  | {
      ok: false;
      code:
        | "INVALID_REQUEST"
        | "UNAUTHORIZED"
        | "PROJECT_NOT_FOUND"
        | "FOLDER_NOT_ATTACHED"
        | "PATH_OUTSIDE_PROJECT"
        | "DIRECTORY_UNREADABLE";
      message: string;
    };

/** Folder list submitted when creating a project. An empty list is legal: the
 * project exists under its own name and binds folders later, if ever. */
export type ProjectFolderSelection = {
  path: string;
  primary?: boolean;
};

export type RuntimeProjectOperationAction =
  | "detach"
  | "delete_history"
  | "trash_and_purge"
  | "delete_thread";

export type RuntimeProjectOperationStatus =
  | "accepted"
  | "running"
  | "succeeded"
  | "failed"
  | "blocked";

export type RuntimeProjectOperationWire = {
  operation_id: string;
  idempotency_key: string;
  project_id?: string;
  action: RuntimeProjectOperationAction;
  status: RuntimeProjectOperationStatus;
  phase: string;
  previous_project_state?: RuntimeProjectState;
  workspace_root?: string;
  path_fingerprint?: string;
  attempt_id?: string;
  trash_staging_path?: string;
  trash_invoked_at?: string;
  matched_threads: number;
  pending_cleanup: number;
  logical_committed_at?: string;
  physical_completed_at?: string;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
};

export type RuntimeProjectOperation = {
  operationId: string;
  idempotencyKey: string;
  projectId?: string;
  action: RuntimeProjectOperationAction;
  status: RuntimeProjectOperationStatus;
  phase: string;
  previousProjectState?: RuntimeProjectState;
  workspaceRoot?: string;
  pathFingerprint?: string;
  attemptId?: string;
  trashStagingPath?: string;
  trashInvokedAt?: string;
  matchedThreads: number;
  pendingCleanup: number;
  logicalCommittedAt?: string;
  physicalCompletedAt?: string;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
};

export type ProjectMutationResult =
  | { ok: true; operation: RuntimeProjectOperation }
  | { ok: false; code: string; message: string; operationId?: string };

export type UpsertProjectResult =
  | { ok: true; project: RuntimeProject }
  | { ok: false; code: string; message: string };

export type ProjectReorderResult =
  | { ok: true; projects: RuntimeProject[] }
  | { ok: false; code: string; message: string };

export type ThreadDeleteResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export type RenameThreadRequest = {
  label: string;
};

export type RuntimeThreadTranscriptRunWire = {
  run: RuntimeRunWire;
  events: AguiEvent[];
  stream_cursor?: number;
};

export type RuntimeThreadTranscriptWire = {
  thread: RuntimeThreadWire;
  runs: RuntimeThreadTranscriptRunWire[];
};

export type RuntimeThreadTranscriptRun = {
  run: RuntimeRun;
  events: AguiEvent[];
  streamCursor?: number;
};

export type RuntimeThreadTranscript = {
  thread: RuntimeThread;
  runs: RuntimeThreadTranscriptRun[];
};

export type RuntimeInfoWire = {
  runtime_version: string;
  api_version: string;
  configured_model: string;
  protocol: string;
  has_api_key: boolean;
  default_workspace: string;
  base_url_host: string;
  model_profile?: RuntimeModelProfileSummaryWire;
};

export type RuntimeInfo = {
  runtimeVersion: string;
  apiVersion: string;
  configuredModel: string;
  protocol: string;
  hasApiKey: boolean;
  defaultWorkspace: string;
  baseUrlHost: string;
  modelProfile?: RuntimeModelProfileSummary;
};

export type RuntimeConnectorState =
  | "disconnected"
  | "pending"
  | "connected"
  | "needs-auth"
  | "failed";

export type RuntimeConnectorKind = "mcp" | "cli";

export type CliProviderId = "dingtalk-workspace" | "lark-cli";
export type CliActivation = "enabled" | "disabled";
export type CliInstallState = "missing" | "installing" | "installed" | "unsupported" | "failed";
export type CliExecutableState = "unknown" | "missing" | "available" | "invalid";
export type CliConfigurationState = "unknown" | "required" | "ready" | "failed";
export type CliAuthenticationState = "unknown" | "required" | "in_progress" | "authorized" | "failed";
export type CliOverallState = "disabled" | "not_ready" | "action_required" | "working" | "ready" | "failed";
export type CliInstallOwnership = "awaken-managed" | "user-managed" | "system-managed";

export type CliInstallPhase =
  | "preparing"
  | "downloading_cli"
  | "verifying_cli"
  | "installing_cli"
  | "downloading_skills"
  | "verifying_skills"
  | "installing_skills"
  | "finishing";

export type CliInstallProgress = {
  phase: CliInstallPhase;
  percent: number;
  label: string;
};

export type CliSafeError = {
  code: string;
  message: string;
  recovery?: string;
};

/** Non-sensitive account projection for connected CLI detail UI. */
export type CliAccountProfile = {
  displayName?: string;
  organization?: string;
};

export type CliConnectorReadiness = {
  providerId: CliProviderId;
  activation: CliActivation;
  install: CliInstallState;
  executable: CliExecutableState;
  configuration: CliConfigurationState;
  authentication: CliAuthenticationState;
  overall: CliOverallState;
  version?: string;
  executablePath?: string;
  ownership?: CliInstallOwnership;
  installMethod?: "managed-download" | "npm" | "homebrew" | "system" | "unknown";
  account?: CliAccountProfile;
  safeError?: CliSafeError;
  updatedAt: string;
};

export type CliAuthPhase =
  | "starting"
  | "waiting_user"
  | "confirming"
  | "authorized"
  | "failed"
  | "cancelled";

export type CliAuthSessionView = {
  id: string;
  provider: CliProviderId;
  phase: CliAuthPhase;
  safeMessage?: string;
};

export type CliConnectorView = {
  providerId: CliProviderId;
  displayName: string;
  readiness: CliConnectorReadiness;
  authSession?: CliAuthSessionView;
  installProgress?: CliInstallProgress;
};

export type CliConnectorStateEvent = {
  connector: CliConnectorView;
};

export type CliConnectorStateHandler = (event: CliConnectorStateEvent) => void;

export type RuntimeConnector = {
  id: string;
  kind: RuntimeConnectorKind;
  displayName: string;
  description?: string;
  category: string;
  provider?: string;
  state: RuntimeConnectorState;
  sourceOrigin: string;
  authType: string;
  lastError?: string;
  serverNames?: string[];
  shadowedBy?: string;
  cliReadiness?: CliConnectorReadiness;
  legacyMcpReviewRequired?: boolean;
};

export type RuntimeConnectorActionResponse = {
  connectorId: string;
  connectorState: RuntimeConnectorState | string;
  message?: string;
};

export type RuntimeMcpServerState =
  | "pending"
  | "connected"
  | "failed"
  | "needs-auth"
  | "disabled";

export type RuntimeMcpServer = {
  name: string;
  state: RuntimeMcpServerState;
  transport: string;
  scope: string;
  errorReason?: string;
  lastConnectedAt?: number;
  sourceConnectorId?: string;
  shadowedBy?: string;
};

export type ConfigureMcpServersRequest = {
  configText: string;
};

export type ConfigureMcpServersResponse = {
  serverNames: string[];
  configPath?: string;
  message: string;
};

export type RuntimeMcpServerConfigResponse = {
  name: string;
  configText: string;
  scope: string;
};

export type UpdateMcpServerConfigRequest = {
  configText: string;
};

export type RuntimeLogEntry = {
  id: number;
  timestamp: number;
  stream: "stdout" | "stderr";
  line: string;
};

export type RuntimeToolInfo = {
  name: string;
  source: string;
  description: string;
  readOnly: boolean;
};

export type RuntimeSkillState =
  | "loaded"
  | "invalid"
  | "shadowed"
  | "disabled_by_policy";

export type RuntimeSkillDiagnostic =
  | string
  | {
      code?: string;
      message?: string;
      severity?: "warning" | "error" | string;
    };

export type RuntimeSkill = {
  id: string;
  catalogEntryId?: string;
  name: string;
  displayName?: string;
  description: string;
  whenToUse?: string;
  state: RuntimeSkillState;
  sourceOrigin: string;
  scope?: "global" | "project" | "mcp" | string;
  rootDir?: string;
  skillFilePath?: string;
  projectPath?: string;
  allowedTools?: string[];
  context: "inline" | "fork" | string;
  shadowedBy?: string;
  requiredMcpServers?: string[];
  userInvocable?: boolean;
  modelInvocable?: boolean;
  diagnostics?: RuntimeSkillDiagnostic[];
  lastInvokedAt?: string;
};

export type SkillProjectRoots = string | string[];

export type ComposerTokenType = "skill" | "file" | "workflow" | "prompt" | "connector";

export type SkillComposerTokenPayload = {
  skillId: string;
  sourceOrigin: "project" | "user" | string;
  skillFilePath: string;
  rootDir?: string;
};

export type ComposerToken = {
  id: string;
  type: ComposerTokenType | string;
  label: string;
  /**
   * Prompt text segment immediately before this token. Durable so canonical
   * user_message reflow and thread replay keep the token's inline position.
   */
  leadingText?: string;
  payload?: Record<string, unknown>;
};

export type InstallSkillRequest = {
  fileName: string;
  content?: string;
  contentBase64?: string;
  scope?: "project" | "global";
  projectRoot?: string;
};

export type InstallSkillResponse = {
  skill: RuntimeSkill;
  path: string;
  message: string;
};

export type DeleteSkillRequest = {
  id: string;
  sourceOrigin: "project" | "user" | string;
  projectRoot?: string;
};

export type DeleteSkillResponse = {
  id: string;
  sourceOrigin: string;
  originalPath: string;
  trashedPath?: string;
  message: string;
};

export type GetSkillPreviewRequest = {
  catalogEntryId: string;
  projectRoot?: string;
};

export type SkillPreviewResponse = {
  frontmatterRaw: string;
  bodyMarkdown: string;
};

export type ModelProviderWire = {
  id: string;
  display_name: string;
  base_url: string;
  protocol: string;
  auth_scheme: string;
  key_env_name: string;
  default_model: string;
  models: string[];
  docs_url: string;
  api_docs_url: string;
  coding_preset?: boolean;
  endpoint_model?: boolean;
  allow_custom_model?: boolean;
  model_capabilities?: Record<string, ModelCapabilityWire>;
};

export type ModelCapabilityWire = {
  context_window?: number;
  max_output_tokens?: number;
  default_max_tokens?: number;
  default_temperature?: number;
  supports_vision?: boolean;
  supports_native_pdf?: boolean;
  supports_video?: boolean;
};

export type ModelProvider = {
  id: string;
  displayName: string;
  baseURL: string;
  protocol: string;
  authScheme: string;
  keyEnvName: string;
  defaultModel: string;
  models: string[];
  docsURL: string;
  apiDocsURL: string;
  codingPreset: boolean;
  endpointModel: boolean;
  allowCustomModel: boolean;
  modelCapabilities: Record<string, ModelCapability>;
};

export type ModelCapability = {
  contextWindow?: number;
  maxOutputTokens?: number;
  defaultMaxTokens?: number;
  defaultTemperature?: number;
  supportsVision?: boolean;
  supportsNativePDF?: boolean;
  supportsVideo?: boolean;
};

export const DEFAULT_CONTEXT_OUTPUT_RESERVE = 16_000;

export function effectiveContextWindow(
  contextWindow?: number,
  outputReserve: number = DEFAULT_CONTEXT_OUTPUT_RESERVE,
): number {
  const normalizedContextWindow =
    typeof contextWindow === "number" && Number.isFinite(contextWindow)
      ? Math.max(0, Math.round(contextWindow))
      : 0;
  const normalizedOutputReserve =
    typeof outputReserve === "number" && Number.isFinite(outputReserve)
      ? Math.max(0, Math.round(outputReserve))
      : DEFAULT_CONTEXT_OUTPUT_RESERVE;
  return Math.max(0, normalizedContextWindow - normalizedOutputReserve);
}

export function calculateAutoCompactThreshold(
  contextWindow?: number,
  outputReserve: number = DEFAULT_CONTEXT_OUTPUT_RESERVE,
): number {
  return Math.floor(effectiveContextWindow(contextWindow, outputReserve) * 0.9);
}

export type ModelProfileAdvancedWire = {
  temperature?: number;
  max_tokens?: number;
  endpoint_id?: string;
  prompt_cache_key?: string;
  context_window?: number;
  supports_vision?: boolean;
  supports_native_pdf?: boolean;
  supports_video?: boolean;
};

export type ModelProfileAdvanced = {
  temperature?: number;
  maxTokens?: number;
  endpointId?: string;
  promptCacheKey?: string;
  contextWindow?: number;
  supportsVision?: boolean;
  supportsNativePDF?: boolean;
  supportsVideo?: boolean;
};

export type ModelCredentialStatus =
  | "ready"
  | "missing_key"
  | "disabled"
  | "unreadable";

export type RuntimeModelProfileSummaryWire = {
  id: string;
  provider_id: string;
  display_name: string;
  model_id: string;
  base_url: string;
  protocol: string;
  auth_scheme: string;
  has_key: boolean;
  masked_key?: string;
  enabled: boolean;
  is_default: boolean;
  profile_status: ModelCredentialStatus;
  credential_revision?: number;
  advanced?: ModelProfileAdvancedWire;
};

export type RuntimeModelProfileSummary = {
  id: string;
  providerId: string;
  displayName: string;
  credentialRevision?: number;
  modelId: string;
  baseURL: string;
  protocol: string;
  authScheme: string;
  hasKey: boolean;
  maskedKey?: string;
  enabled: boolean;
  isDefault: boolean;
  profileStatus: ModelCredentialStatus;
  advanced?: ModelProfileAdvanced;
};

export type ModelProfileSaveRequest = {
  id?: string;
  templateProfileId?: string;
  providerId: string;
  displayName?: string;
  modelId: string;
  baseURL?: string;
  protocol?: string;
  authScheme?: string;
  apiKey?: string;
  enabled?: boolean;
  isDefault?: boolean;
  advanced?: ModelProfileAdvanced;
};

export type ModelProfileTestResult = {
  ok: boolean;
  message: string;
};

export type ModelBalanceStatusWire =
  | "available"
  | "unavailable"
  | "missing_key"
  | "error"
  | "unsupported";

export type ModelBalanceAmountWire = {
  currency: string;
  total: number;
  granted?: number;
  topped_up?: number;
};

export type ModelBalanceQuotaWire = {
  name: string;
  limit: number;
  used: number;
  remaining: number;
  reset_at?: string;
};

export type ModelBalanceDetailWire =
  | { kind: "amount"; amounts: ModelBalanceAmountWire[] }
  | { kind: "quota"; quotas: ModelBalanceQuotaWire[] };

export type ModelProfileBalanceWire = {
  profile_id: string;
  provider_id: string;
  model_id: string;
  display_name?: string;
  status: ModelBalanceStatusWire;
  fetched_at?: string;
  detail?: ModelBalanceDetailWire;
  error?: string;
  credential_issue?: "unreadable";
};

export type ModelProfileBalancesResultWire = {
  balances: ModelProfileBalanceWire[];
};

export type ModelBalanceStatus =
  | "available"
  | "unavailable"
  | "missing_key"
  | "error"
  | "unsupported";

export type ModelBalanceAmount = {
  currency: string;
  total: number;
  granted?: number;
  toppedUp?: number;
};

export type ModelBalanceQuota = {
  name: string;
  limit: number;
  used: number;
  remaining: number;
  resetAt?: string;
};

export type ModelBalanceDetail =
  | { kind: "amount"; amounts: ModelBalanceAmount[] }
  | { kind: "quota"; quotas: ModelBalanceQuota[] };

export type ModelProfileBalance = {
  profileId: string;
  providerId: string;
  modelId: string;
  displayName?: string;
  status: ModelBalanceStatus;
  fetchedAt?: string;
  detail?: ModelBalanceDetail;
  error?: string;
  credentialIssue?: "unreadable";
};

export type ModelProfileBalancesResult = {
  balances: ModelProfileBalance[];
};

export type TaskScopeFileWire = {
  path: string;
  display_name?: string;
};

export type TaskScopeFile = {
  path: string;
  displayName?: string;
};

export type WorkspaceConfigWire = {
  project_id?: string;
  project_name?: string;
  workspace_root: string;
  readable_roots?: string[];
  writable_roots?: string[];
  output_roots?: string[];
  attached_files?: TaskScopeFileWire[];
};

export type WorkspaceConfig = {
  /** Identifies the project a run belongs to. A folder-less project uses it to
   * resolve a project session workspace. When both this field and workspaceRoot
   * are empty, Core resolves a project-less conversation by thread ID. */
  projectId?: string;
  /** Display name of the project, sent so the agent can name where it is
   * working. Core derives no path or permission from it. */
  projectName?: string;
  workspaceRoot: string;
  readableRoots?: string[];
  writableRoots?: string[];
  outputRoots?: string[];
  attachedFiles?: TaskScopeFile[];
};

export type ClientInfoWire = {
  session_id: string;
  request_id: string;
  entrypoint: string;
};

export type ClientInfo = {
  sessionId: string;
  requestId: string;
  entrypoint: string;
};

export type StartRunRequestWire = {
  input: string;
  attachments?: RunInputAttachmentWire[];
  composer_tokens?: ComposerToken[];
  thread_id?: string;
  workspace: WorkspaceConfigWire;
  client: ClientInfoWire;
  model_profile?: ModelProfileRunConfigWire;
};

export type StartRunRequest = {
  input: string;
  attachments?: RunInputAttachment[];
  composerTokens?: ComposerToken[];
  threadId?: string;
  workspace: WorkspaceConfig;
  client: ClientInfo;
  modelProfile?: ModelProfileRunConfig;
};

export type ModelProfileRunConfigWire = {
  profile_id?: string;
  provider_id?: string;
  display_name?: string;
  model_id?: string;
  base_url?: string;
  protocol?: string;
  auth_scheme?: string;
  api_key?: string;
  temperature?: number;
  max_tokens?: number;
  context_window?: number;
  prompt_cache_key?: string;
  credential_revision?: number;
  supports_vision?: boolean;
  supports_native_pdf?: boolean;
  supports_video?: boolean;
};

export type ModelProfileRunConfig = {
  profileId?: string;
  providerId?: string;
  displayName?: string;
  modelId?: string;
  baseURL?: string;
  protocol?: string;
  authScheme?: string;
  apiKey?: string;
  temperature?: number;
  maxTokens?: number;
  contextWindow?: number;
  promptCacheKey?: string;
  credentialRevision?: number;
  supportsVision?: boolean;
  supportsNativePDF?: boolean;
  supportsVideo?: boolean;
};

export type WorkspacePickResult =
  | { canceled: true; error?: string }
  | { canceled: false; path: string };

/** Result of picking project folders. The picker allows several directories at
 * once, so `errors` reports the ones that could not be read while `paths` still
 * carries whatever was readable. */
export type ProjectFoldersPickResult =
  | { canceled: true; error?: string }
  | { canceled: false; paths: string[]; errors?: string[] };

export type WorkspaceValidationResult =
  | { valid: true }
  | { valid: false; error: string };

export type UserQuestionAnswer = {
  questionId: string;
  answer: string;
};

export type ArtifactOpenRequest = {
  threadId: string;
  runId: string;
  artifactId: string;
};

export type ArtifactOpenErrorCode =
  | "runtime_unavailable"
  | "artifact_not_recorded"
  | "run_not_terminal"
  | "artifact_not_deliverable"
  | "unsupported_type"
  | "outside_output_roots"
  | "file_not_found"
  | "not_a_file"
  | "no_default_application"
  | "open_failed";

export type ArtifactOpenResult =
  | { ok: true }
  | { ok: false; code: ArtifactOpenErrorCode; message: string };

export type RunStreamHandlers = {
  onEvent: (event: AguiEvent) => void;
  onError?: (message: string) => void;
  onClose?: () => void;
};

export type SupervisorStatus = "stopped" | "starting" | "running" | "crashed" | "stopping";

export type SupervisorStatusInfo = {
  status: SupervisorStatus;
};

export type SearchProviderId = "exa" | "tavily";

export type SearchSettings = {
  providers: Record<SearchProviderId, { enabled: boolean }>;
};

export type SearchSettingsSaveRequest = {
  provider: SearchProviderId;
  enabled: boolean;
};

export type SearchSettingsSaveResult = {
  settings: SearchSettings;
  restartStatus: "restarted" | "restart_failed";
  message?: string;
};

export type SearchProviderTestRequest = {
  provider: SearchProviderId;
};

export type SearchProviderTestResult = {
  ok: boolean;
  message: string;
};

export type ACPPermissionMode = "ask" | "deny";

export type ACPRunnerEnvVar = {
  key: string;
  has_value: boolean;
};

// ACPRunnerView is the redacted runner DTO returned by Core. Secret env values
// are never sent to the Renderer; only the key and a has_value flag are exposed.
export type ACPRunnerView = {
  id: string;
  enabled: boolean;
  command: string;
  args: string[];
  env: ACPRunnerEnvVar[];
  permission_mode: ACPPermissionMode;
  startup_timeout_seconds: number;
  turn_timeout_seconds: number;
  max_frame_bytes: number;
};

// ACPRunnerCreateRequest is the write DTO used when creating a runner.
// It carries full env values because it originates from user input in the
// settings UI, not from a redacted API response.
export type ACPRunnerCreateRequest = {
  id: string;
  enabled: boolean;
  command: string;
  args: string[];
  env: Record<string, string>;
  permission_mode: ACPPermissionMode;
  startup_timeout_seconds: number;
  turn_timeout_seconds: number;
  max_frame_bytes: number;
};

export type ACPRunnersSettings = {
  runners: ACPRunnerView[];
};

export type ACPRunnerProbeResult = {
  runner_id: string;
  ready: boolean;
};

export type ACPRunnerUpdateRequest = Partial<Omit<ACPRunnerCreateRequest, "id" | "env">> & {
  env_set?: Record<string, string>;
  env_remove?: string[];
};

// ---------------------------------------------------------------------------
// Scheduled tasks (contract docs/12_定时任务 §10). Wire shapes mirror the Go
// json tags of src/scheduledtask; the camelCase projections are the only
// shapes Renderer code may touch.
// ---------------------------------------------------------------------------

export type ScheduledTaskState = "enabled" | "paused" | "running" | "completed" | "invalid";

export type ScheduledTaskActionKind = "reminder" | "agent";

export type ScheduledTaskActionWire = {
  kind: ScheduledTaskActionKind;
  content: string;
};

export type ScheduledTaskAction = {
  kind: ScheduledTaskActionKind;
  content: string;
};

export type ScheduledTaskExecutionOptionsWire = {
  profile_id: string;
  timeout_seconds: number;
  misfire_grace_seconds: number;
};

export type ScheduledTaskExecutionOptions = {
  profileId: string;
  timeoutSeconds: number;
  misfireGraceSeconds: number;
};

export type ScheduledTaskTemplateOriginWire = {
  template_id: string;
  source: string;
};

export type ScheduledTaskTemplateOrigin = {
  templateId: string;
  source: string;
};

export type ScheduledTaskRepeatEndKind = "never" | "until" | "count";

export type ScheduledTaskRepeatEndWire = {
  kind: ScheduledTaskRepeatEndKind;
  until?: string;
  count?: number;
};

export type ScheduledTaskRepeatEnd = {
  kind: ScheduledTaskRepeatEndKind;
  until?: string;
  count?: number;
};

export type ScheduledTaskScheduleWire = {
  kind: "cron" | "calendar";
  timezone: string;
  cron?: string;
  run_at?: string;
  repeat_every_days?: number;
  repeat_end?: ScheduledTaskRepeatEndWire;
};

export type ScheduledTaskSchedule =
  | { kind: "cron"; cron: string; timezone: string }
  | {
      kind: "calendar";
      runAt: string;
      timezone: string;
      repeatEveryDays?: number;
      repeatEnd?: ScheduledTaskRepeatEnd;
    };

export type ScheduledTaskLastExecutionWire = {
  execution_id: string;
  status: string;
  started_at?: string;
  finished_at?: string;
};

export type ScheduledTaskLastExecution = {
  executionId: string;
  status: string;
  startedAt?: string;
  finishedAt?: string;
};

export type ScheduledTaskWire = {
  task_id: string;
  version: number;
  name: string;
  enabled: boolean;
  project_id: string;
  thread_id?: string;
  schedule: ScheduledTaskScheduleWire;
  action: ScheduledTaskActionWire;
  execution: ScheduledTaskExecutionOptionsWire;
  template_origin?: ScheduledTaskTemplateOriginWire;
  state: ScheduledTaskState;
  invalid_reason?: string;
  next_run_at?: string | null;
  last_execution?: ScheduledTaskLastExecutionWire | null;
  created_at: string;
  updated_at: string;
};

export type ScheduledTask = {
  taskId: string;
  version: number;
  name: string;
  enabled: boolean;
  projectId: string;
  threadId?: string;
  schedule: ScheduledTaskSchedule;
  action: ScheduledTaskAction;
  execution: ScheduledTaskExecutionOptions;
  templateOrigin?: ScheduledTaskTemplateOrigin;
  state: ScheduledTaskState;
  invalidReason?: string;
  nextRunAt?: string | null;
  lastExecution?: ScheduledTaskLastExecution | null;
  createdAt: string;
  updatedAt: string;
};

export type ScheduledTaskExecutionTrigger = "scheduled" | "manual" | "misfire";

export type ScheduledTaskExecutionStatus =
  | "queued"
  | "running"
  | "success"
  | "error"
  | "skipped"
  | "cancelled"
  | "interrupted";

export type ScheduledTaskExecutionWire = {
  execution_id: string;
  task_id: string;
  task_version: number;
  trigger: ScheduledTaskExecutionTrigger;
  scheduled_for: string;
  started_at?: string;
  finished_at?: string;
  status: ScheduledTaskExecutionStatus;
  run_id?: string;
  thread_id?: string;
  error_code?: string;
  error_message?: string;
};

export type ScheduledTaskExecution = {
  executionId: string;
  taskId: string;
  taskVersion: number;
  trigger: ScheduledTaskExecutionTrigger;
  scheduledFor: string;
  startedAt?: string;
  finishedAt?: string;
  status: ScheduledTaskExecutionStatus;
  runId?: string;
  threadId?: string;
  errorCode?: string;
  errorMessage?: string;
};

export type ScheduledTaskEventType =
  | "execution_started"
  | "execution_finished"
  | "notice"
  | "task_changed";

export type ScheduledTaskEventWire = {
  type: ScheduledTaskEventType;
  task_id: string;
  execution_id?: string;
  thread_id?: string;
  task_name?: string;
  status?: string;
  content?: string;
};

export type ScheduledTaskEvent = {
  type: ScheduledTaskEventType;
  taskId: string;
  executionId?: string;
  threadId?: string;
  taskName?: string;
  status?: string;
  content?: string;
};

export type ScheduledTaskCreateInput = {
  name: string;
  enabled: boolean;
  projectId: string;
  schedule: ScheduledTaskSchedule;
  action: ScheduledTaskAction;
  execution: ScheduledTaskExecutionOptions;
  templateOrigin?: ScheduledTaskTemplateOrigin;
};

export type ScheduledTaskCreateWire = {
  name: string;
  enabled: boolean;
  project_id: string;
  schedule: ScheduledTaskScheduleWire;
  action: ScheduledTaskActionWire;
  execution: ScheduledTaskExecutionOptionsWire;
  template_origin?: ScheduledTaskTemplateOriginWire;
};

export type ScheduledTaskPatchInput = {
  expectedVersion: number;
  name?: string;
  enabled?: boolean;
  projectId?: string;
  schedule?: ScheduledTaskSchedule;
  action?: ScheduledTaskAction;
  execution?: ScheduledTaskExecutionOptions;
  templateOrigin?: ScheduledTaskTemplateOrigin;
};

export type ScheduledTaskPatchWire = {
  expected_version: number;
  name?: string;
  enabled?: boolean;
  project_id?: string;
  schedule?: ScheduledTaskScheduleWire;
  action?: ScheduledTaskActionWire;
  execution?: ScheduledTaskExecutionOptionsWire;
  template_origin?: ScheduledTaskTemplateOriginWire;
};

export type ScheduledTaskListWire = {
  tasks: ScheduledTaskWire[];
};

export type ScheduledTaskExecutionListWire = {
  executions: ScheduledTaskExecutionWire[];
};

export type AwakenBridge = {
  platform: DesktopPlatform;
  onSelectAllRequested: (callback: () => void) => () => void;
  relaunchDesktop: () => Promise<void>;
  getSearchSettings: () => Promise<SearchSettings>;
  saveSearchSettings: (
    request: SearchSettingsSaveRequest,
  ) => Promise<SearchSettingsSaveResult>;
  testSearchProvider: (
    request: SearchProviderTestRequest,
  ) => Promise<SearchProviderTestResult>;
  getACPRunners: () => Promise<ACPRunnersSettings>;
  createACPRunner: (
    request: ACPRunnerCreateRequest,
  ) => Promise<ACPRunnerView>;
  updateACPRunner: (
    runnerId: string,
    request: ACPRunnerUpdateRequest,
  ) => Promise<ACPRunnerView>;
  deleteACPRunner: (runnerId: string) => Promise<void>;
  probeACPRunner: (runnerId: string) => Promise<ACPRunnerProbeResult>;
  getRun: (runId: string) => Promise<RuntimeRun>;
  getThreadTranscript: (threadId: string) => Promise<RuntimeThreadTranscript>;
  listThreads: () => Promise<RuntimeThread[]>;
  renameThread: (threadId: string, label: string) => Promise<RuntimeThread>;
  deleteThread: (threadId: string) => Promise<ThreadDeleteResult>;
  listProjects: (includeRemoved?: boolean) => Promise<RuntimeProject[]>;
  listProjectDirectory: (
    request: ProjectDirectoryListRequest,
  ) => Promise<ProjectDirectoryListResult>;
  upsertProject: (workspaceRoot: string, displayName?: string) => Promise<UpsertProjectResult>;
  createProject: (
    folders: ProjectFolderSelection[],
    displayName?: string,
  ) => Promise<UpsertProjectResult>;
  attachProjectFolder: (projectId: string, path: string) => Promise<UpsertProjectResult>;
  detachProjectFolder: (projectId: string, path: string) => Promise<UpsertProjectResult>;
  setPrimaryProjectFolder: (projectId: string, path: string) => Promise<UpsertProjectResult>;
  reorderProjects: (projectIds: string[]) => Promise<ProjectReorderResult>;
  detachProject: (projectId: string, idempotencyKey: string) => Promise<ProjectMutationResult>;
  deleteProjectThreads: (projectId: string, idempotencyKey: string) => Promise<ProjectMutationResult>;
  trashProjectFolder: (
    projectId: string,
    path: string,
    idempotencyKey: string,
  ) => Promise<ProjectMutationResult>;
  getProjectOperation: (operationId: string) => Promise<RuntimeProjectOperation>;
  openArtifact: (request: ArtifactOpenRequest) => Promise<ArtifactOpenResult>;
  revealPath: (target: string) => Promise<{ ok: boolean; message?: string }>;
  revealArtifact: (request: ArtifactOpenRequest) => Promise<ArtifactOpenResult>;
  startRun: (
    request: StartRunRequest,
  ) => Promise<DesktopOperationResult<RuntimeRun>>;
  cancelRun: (runId: string) => Promise<RuntimeRun>;
  answerUserQuestion: (
    runId: string,
    questionId: string,
    answer: string,
  ) => Promise<void>;
  resolveExternalInteraction: (
    runId: string,
    interactionId: string,
    resolution: "completed" | "cancelled" | "expired" | "failed",
  ) => Promise<void>;
  resolveACPPermission: (
    runId: string,
    interactionId: string,
    optionId: string,
    cancelled: boolean,
  ) => Promise<void>;
  subscribeRun: (
    runId: string,
    handlers: RunStreamHandlers,
    afterSeq?: number,
  ) => Promise<string>;
  unsubscribeRun: (subscriptionId: string) => void;
  getDesktopPreferences: () => Promise<DesktopPreferences>;
  saveDesktopPreferences: (request: DesktopPreferencesSaveRequest) => Promise<DesktopPreferences>;
  listMemories: (request?: MemoryListRequest) => Promise<RuntimeMemory[]>;
  getMemory: (memoryId: string, projectId?: string) => Promise<RuntimeMemory>;
  deleteMemory: (memoryId: string, expectedRevision: number, projectId?: string) => Promise<void>;
  getMemorySettings: () => Promise<MemorySettingsDocument>;
  updateMemorySettings: (request: MemorySettingsPatch) => Promise<MemorySettingsDocument>;
  startMemoryDream: (projectId?: string) => Promise<MemoryDreamTask>;
  getMemoryDream: (taskId: string) => Promise<MemoryDreamTask>;
  listMemoryDreams: (projectId?: string, limit?: number) => Promise<MemoryDreamTask[]>;
  cancelMemoryDream: (taskId: string) => Promise<void>;
  onMemoryDreamReceipt: (handler: (task: MemoryDreamTask) => void) => () => void;
  getRuntimeInfo: () => Promise<RuntimeInfo>;
  listConnectors: () => Promise<RuntimeConnector[]>;
  connectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  disconnectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  listCliConnectors: () => Promise<CliConnectorView[]>;
  refreshCliConnector: (providerId: CliProviderId) => Promise<CliConnectorView>;
  installCliConnector: (providerId: CliProviderId) => Promise<CliConnectorView>;
  cancelCliInstall: (providerId: CliProviderId) => Promise<CliConnectorView>;
  startCliAuth: (providerId: CliProviderId) => Promise<CliConnectorView>;
  cancelCliAuth: (providerId: CliProviderId) => Promise<CliConnectorView>;
  retryCliAuth: (providerId: CliProviderId) => Promise<CliConnectorView>;
  setCliConnectorActivation: (providerId: CliProviderId, activation: CliActivation) => Promise<CliConnectorView>;
  logoutCliConnector: (providerId: CliProviderId) => Promise<CliConnectorView>;
  uninstallManagedCli: (providerId: CliProviderId) => Promise<CliConnectorView>;
  subscribeCliConnectorState: (handler: CliConnectorStateHandler) => () => void;
  listMcpServers: () => Promise<RuntimeMcpServer[]>;
  configureMcpServers: (request: ConfigureMcpServersRequest) => Promise<ConfigureMcpServersResponse>;
  getMcpServerConfig: (name: string) => Promise<RuntimeMcpServerConfigResponse>;
  updateMcpServerConfig: (name: string, request: UpdateMcpServerConfigRequest) => Promise<ConfigureMcpServersResponse>;
  disableMcpServer: (name: string) => Promise<void>;
  enableMcpServer: (name: string) => Promise<void>;
  deleteMcpServer: (name: string) => Promise<void>;
  getRuntimeLogs: () => Promise<RuntimeLogEntry[]>;
  listMcpTools: () => Promise<RuntimeToolInfo[]>;
  listSkills: (projectRoots?: SkillProjectRoots) => Promise<RuntimeSkill[]>;
  getSkillPreview: (request: GetSkillPreviewRequest) => Promise<SkillPreviewResponse>;
  installSkill: (request: InstallSkillRequest) => Promise<InstallSkillResponse>;
  deleteSkill: (request: DeleteSkillRequest) => Promise<DeleteSkillResponse>;
  disableSkill: (id: string) => Promise<void>;
  enableSkill: (id: string) => Promise<void>;
  listModelProviders: () => Promise<ModelProvider[]>;
  listModelProfiles: () => Promise<RuntimeModelProfileSummary[]>;
  saveModelProfile: (
    request: ModelProfileSaveRequest,
  ) => Promise<DesktopOperationResult<RuntimeModelProfileSummary>>;
  deleteModelProfile: (profileId: string) => Promise<void>;
  setDefaultModelProfile: (profileId: string) => Promise<RuntimeModelProfileSummary>;
  testModelProfile: (
    request: ModelProfileSaveRequest,
  ) => Promise<DesktopOperationResult<ModelProfileTestResult>>;
  syncModelCapabilities: () => Promise<ModelProvider[]>;
  getModelProfileBalances: () => Promise<ModelProfileBalancesResult>;
  pickAttachments: (defaultPath?: string) => Promise<AttachmentPickResult>;
  pickDirectory: (defaultPath?: string) => Promise<WorkspacePickResult>;
  pickProjectFolders: (defaultPath?: string) => Promise<ProjectFoldersPickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
  notifyRendererReady: () => void;
  listScheduledTasks: () => Promise<ScheduledTask[]>;
  getScheduledTask: (taskId: string) => Promise<ScheduledTask>;
  createScheduledTask: (input: ScheduledTaskCreateInput) => Promise<ScheduledTask>;
  patchScheduledTask: (taskId: string, input: ScheduledTaskPatchInput) => Promise<ScheduledTask>;
  deleteScheduledTask: (taskId: string, expectedVersion: number, idempotencyKey: string) => Promise<void>;
  pauseScheduledTask: (taskId: string, expectedVersion: number) => Promise<ScheduledTask>;
  resumeScheduledTask: (taskId: string, expectedVersion: number) => Promise<ScheduledTask>;
  runScheduledTaskNow: (taskId: string, idempotencyKey: string) => Promise<ScheduledTaskExecution>;
  cancelScheduledTaskExecution: (taskId: string) => Promise<void>;
  listScheduledTaskExecutions: (taskId: string) => Promise<ScheduledTaskExecution[]>;
  getSystemTimezone: () => Promise<string>;
  subscribeScheduledTaskEvents: (handler: (event: ScheduledTaskEvent) => void) => () => void;
  onScheduledTaskThreadActivation: (handler: (threadId: string) => void) => () => void;
} & PetBridge;

export function normalizeRuntimeRun(run: RuntimeRunWire): RuntimeRun {
  return {
    runId: run.run_id,
    threadId: run.thread_id,
    input: run.input?.trim() || undefined,
    inputAttachments: run.input_attachments?.map(normalizeRunInputAttachment),
    composerTokens: run.composer_tokens?.map((token) => ({
      ...token,
      payload: token.payload ? { ...token.payload } : undefined,
    })),
    status: run.status,
    result: run.result,
    error: run.error,
    cancelRequested: Boolean(run.cancel_requested),
    createdAt: run.created_at,
    updatedAt: run.updated_at,
    finishedAt: run.finished_at ?? undefined,
  };
}

export function normalizeRuntimeThread(thread: RuntimeThreadWire): RuntimeThread {
  return {
    threadId: thread.thread_id,
    projectId: thread.project_id || undefined,
    label: thread.label,
    titleSource: thread.title_source || undefined,
    titleStatus: thread.title_status || undefined,
    titleUpdatedAt: thread.title_updated_at || undefined,
    titleSeedRunId: thread.title_seed_run_id || undefined,
    runId: thread.run_id || undefined,
    activeRunId: thread.active_run_id || undefined,
    workspace: {
      workspaceRoot: thread.workspace.workspace_root,
      readableRoots: thread.workspace.readable_roots ?? [],
      writableRoots: thread.workspace.writable_roots ?? [],
      outputRoots: thread.workspace.output_roots ?? [],
      attachedFiles: (thread.workspace.attached_files ?? []).map((file) => ({
        path: file.path,
        displayName: file.display_name,
      })),
    },
    createdAt: thread.created_at,
    updatedAt: thread.updated_at,
  };
}

export function normalizeRuntimeProject(project: RuntimeProjectWire): RuntimeProject {
  // A project persisted before multi-folder support has no folders array; its
  // workspace_root is the single primary user folder. A project created without
  // picking a folder has neither, and stays folder-less.
  const folders: RuntimeProjectFolder[] = (project.folders ?? []).map((folder) => ({
    path: folder.path,
    role: folder.role === "managed" ? "managed" : "user",
    isPrimary: Boolean(folder.is_primary),
  }));
  if (folders.length === 0 && project.workspace_root) {
    folders.push({ path: project.workspace_root, role: "user", isPrimary: true });
  }
  return {
    projectId: project.project_id,
    workspaceRoot: project.workspace_root,
    folders,
    displayName: project.display_name,
    state: project.state,
    removalReason: project.removal_reason || undefined,
    removedAt: project.removed_at || undefined,
    createdAt: project.created_at,
    updatedAt: project.updated_at,
    version: project.version,
  };
}

export function normalizeRuntimeProjectOperation(
  operation: RuntimeProjectOperationWire,
): RuntimeProjectOperation {
  return {
    operationId: operation.operation_id,
    idempotencyKey: operation.idempotency_key,
    projectId: operation.project_id || undefined,
    action: operation.action,
    status: operation.status,
    phase: operation.phase,
    previousProjectState: operation.previous_project_state || undefined,
    workspaceRoot: operation.workspace_root || undefined,
    pathFingerprint: operation.path_fingerprint || undefined,
    attemptId: operation.attempt_id || undefined,
    trashStagingPath: operation.trash_staging_path || undefined,
    trashInvokedAt: operation.trash_invoked_at || undefined,
    matchedThreads: operation.matched_threads,
    pendingCleanup: operation.pending_cleanup,
    logicalCommittedAt: operation.logical_committed_at || undefined,
    physicalCompletedAt: operation.physical_completed_at || undefined,
    errorCode: operation.error_code || undefined,
    errorMessage: operation.error_message || undefined,
    createdAt: operation.created_at,
    updatedAt: operation.updated_at,
  };
}

export function normalizeRuntimeThreadTranscript(
  transcript: RuntimeThreadTranscriptWire,
): RuntimeThreadTranscript {
  return {
    thread: normalizeRuntimeThread(transcript.thread),
    runs: transcript.runs.map((entry) => ({
      run: normalizeRuntimeRun(entry.run),
      events: entry.events,
      streamCursor: entry.stream_cursor ?? 0,
    })),
  };
}

export function normalizeRuntimeInfo(info: RuntimeInfoWire): RuntimeInfo {
  return {
    runtimeVersion: info.runtime_version,
    apiVersion: info.api_version,
    configuredModel: info.configured_model,
    protocol: info.protocol,
    hasApiKey: info.has_api_key,
    defaultWorkspace: info.default_workspace,
    baseUrlHost: info.base_url_host,
    modelProfile: info.model_profile ? normalizeModelProfileSummary(info.model_profile) : undefined,
  };
}

export function normalizeStartRunRequest(request: StartRunRequest): StartRunRequestWire {
  return {
    input: request.input,
    attachments: request.attachments?.map(toRunInputAttachmentWire),
    composer_tokens: request.composerTokens,
    thread_id: request.threadId,
    workspace: {
      project_id: request.workspace.projectId,
      project_name: request.workspace.projectName,
      workspace_root: request.workspace.workspaceRoot,
      readable_roots: request.workspace.readableRoots,
      writable_roots: request.workspace.writableRoots,
      output_roots: request.workspace.outputRoots,
      attached_files: request.workspace.attachedFiles?.map((file) => ({
        path: file.path,
        display_name: file.displayName,
      })),
    },
    client: {
      session_id: request.client.sessionId,
      request_id: request.client.requestId,
      entrypoint: request.client.entrypoint,
    },
    model_profile: request.modelProfile ? normalizeModelProfileRunConfig(request.modelProfile) : undefined,
  };
}

export function normalizeModelProvider(provider: ModelProviderWire): ModelProvider {
  return {
    id: provider.id,
    displayName: provider.display_name,
    baseURL: provider.base_url,
    protocol: provider.protocol,
    authScheme: provider.auth_scheme,
    keyEnvName: provider.key_env_name,
    defaultModel: provider.default_model,
    models: provider.models,
    docsURL: provider.docs_url,
    apiDocsURL: provider.api_docs_url,
    codingPreset: Boolean(provider.coding_preset),
    endpointModel: Boolean(provider.endpoint_model),
    allowCustomModel: Boolean(provider.allow_custom_model),
    modelCapabilities: normalizeModelCapabilities(provider.model_capabilities),
  };
}

export function normalizeModelCapabilities(
  capabilities?: Record<string, ModelCapabilityWire>,
): Record<string, ModelCapability> {
  if (!capabilities) return {};
  return Object.fromEntries(
    Object.entries(capabilities).map(([modelId, capability]) => [
      modelId,
      {
        contextWindow: capability.context_window,
        maxOutputTokens: capability.max_output_tokens,
        defaultMaxTokens: capability.default_max_tokens,
        defaultTemperature: capability.default_temperature,
        supportsVision: capability.supports_vision,
        supportsNativePDF: capability.supports_native_pdf,
        supportsVideo: capability.supports_video,
      },
    ]),
  );
}

export function normalizeModelProfileAdvanced(
  advanced?: ModelProfileAdvancedWire,
): ModelProfileAdvanced | undefined {
  if (!advanced) return undefined;
  return {
    temperature: advanced.temperature,
    maxTokens: advanced.max_tokens,
    endpointId: advanced.endpoint_id,
    promptCacheKey: advanced.prompt_cache_key,
    contextWindow: advanced.context_window,
    supportsVision: advanced.supports_vision,
    supportsNativePDF: advanced.supports_native_pdf,
    supportsVideo: advanced.supports_video,
  };
}

export function toModelProfileAdvancedWire(
  advanced?: ModelProfileAdvanced,
): ModelProfileAdvancedWire | undefined {
  if (!advanced) return undefined;
  return {
    temperature: advanced.temperature,
    max_tokens: advanced.maxTokens,
    endpoint_id: advanced.endpointId,
    prompt_cache_key: advanced.promptCacheKey,
    context_window: advanced.contextWindow,
    supports_vision: advanced.supportsVision,
    supports_native_pdf: advanced.supportsNativePDF,
    supports_video: advanced.supportsVideo,
  };
}

export function normalizeModelProfileSummary(
  profile: RuntimeModelProfileSummaryWire,
): RuntimeModelProfileSummary {
  return {
    id: profile.id,
    providerId: profile.provider_id,
    displayName: profile.display_name,
    credentialRevision: profile.credential_revision ?? 1,
    modelId: profile.model_id,
    baseURL: profile.base_url,
    protocol: profile.protocol,
    authScheme: profile.auth_scheme,
    hasKey: profile.has_key,
    maskedKey: profile.masked_key,
    enabled: profile.enabled,
    isDefault: profile.is_default,
    profileStatus: profile.profile_status,
    advanced: normalizeModelProfileAdvanced(profile.advanced),
  };
}

export function normalizeModelProfileRunConfig(
  profile: ModelProfileRunConfig,
): ModelProfileRunConfigWire {
  return {
    profile_id: profile.profileId,
    provider_id: profile.providerId,
    display_name: profile.displayName,
    model_id: profile.modelId,
    base_url: profile.baseURL,
    protocol: profile.protocol,
    auth_scheme: profile.authScheme,
    api_key: profile.apiKey,
    temperature: profile.temperature,
    max_tokens: profile.maxTokens,
    context_window: profile.contextWindow,
    prompt_cache_key: profile.promptCacheKey,
    credential_revision: profile.credentialRevision,
    supports_vision: profile.supportsVision,
    supports_native_pdf: profile.supportsNativePDF,
    supports_video: profile.supportsVideo,
  };
}

export function normalizeRunInputAttachment(
  attachment: RunInputAttachmentWire,
): RunInputAttachment {
  return {
    id: attachment.id,
    kind: attachment.kind,
    mimeType: attachment.mime_type,
    path: attachment.path,
    dataBase64: attachment.data_base64,
    displayName: attachment.display_name,
    width: attachment.width,
    height: attachment.height,
    byteSize: attachment.byte_size,
  };
}

export function toRunInputAttachmentWire(
  attachment: RunInputAttachment,
): RunInputAttachmentWire {
  return {
    id: attachment.id,
    kind: attachment.kind,
    mime_type: attachment.mimeType,
    path: attachment.path,
    data_base64: attachment.dataBase64,
    display_name: attachment.displayName,
    width: attachment.width,
    height: attachment.height,
    byte_size: attachment.byteSize,
  };
}

export function normalizeModelBalanceStatus(
  status: ModelBalanceStatusWire,
): ModelBalanceStatus {
  return status;
}

export function normalizeModelBalanceAmount(
  amount: ModelBalanceAmountWire,
): ModelBalanceAmount {
  return {
    currency: amount.currency,
    total: amount.total,
    granted: amount.granted,
    toppedUp: amount.topped_up,
  };
}

export function normalizeModelBalanceQuota(
  quota: ModelBalanceQuotaWire,
): ModelBalanceQuota {
  return {
    name: quota.name,
    limit: quota.limit,
    used: quota.used,
    remaining: quota.remaining,
    resetAt: quota.reset_at,
  };
}

export function normalizeModelBalanceDetail(
  detail: ModelBalanceDetailWire,
): ModelBalanceDetail {
  if (detail.kind === "amount") {
    return {
      kind: "amount",
      amounts: detail.amounts.map(normalizeModelBalanceAmount),
    };
  }
  return {
    kind: "quota",
    quotas: detail.quotas.map(normalizeModelBalanceQuota),
  };
}

export function normalizeModelProfileBalance(
  balance: ModelProfileBalanceWire,
): ModelProfileBalance {
  return {
    profileId: balance.profile_id,
    providerId: balance.provider_id,
    modelId: balance.model_id,
    displayName: balance.display_name,
    status: normalizeModelBalanceStatus(balance.status),
    fetchedAt: balance.fetched_at,
    detail: balance.detail ? normalizeModelBalanceDetail(balance.detail) : undefined,
    error: balance.error,
    credentialIssue: balance.credential_issue,
  };
}

export function normalizeModelProfileBalancesResult(
  result: ModelProfileBalancesResultWire,
): ModelProfileBalancesResult {
  return {
    balances: result.balances.map(normalizeModelProfileBalance),
  };
}

// ---------------------------------------------------------------------------
// Scheduled-task normalization (snake_case wire ↔ camelCase projection).
// ---------------------------------------------------------------------------

/** Go encodes an unset time.Time as the year-1 zero value; treat it as absent. */
function isZeroTimeValue(value?: string): boolean {
  return !value || value.startsWith("0001-01-01");
}

function presentTime(value?: string): string | undefined {
  return isZeroTimeValue(value) ? undefined : value;
}

export function normalizeScheduledTaskRepeatEnd(
  repeatEnd: ScheduledTaskRepeatEndWire,
): ScheduledTaskRepeatEnd {
  return {
    kind: repeatEnd.kind,
    until: presentTime(repeatEnd.until),
    count: repeatEnd.count,
  };
}

export function toScheduledTaskRepeatEndWire(
  repeatEnd: ScheduledTaskRepeatEnd,
): ScheduledTaskRepeatEndWire {
  return {
    kind: repeatEnd.kind,
    until: repeatEnd.until,
    count: repeatEnd.count,
  };
}

export function normalizeScheduledTaskSchedule(
  schedule: ScheduledTaskScheduleWire,
): ScheduledTaskSchedule {
  if (schedule.kind === "calendar") {
    return {
      kind: "calendar",
      runAt: schedule.run_at ?? "",
      timezone: schedule.timezone,
      repeatEveryDays:
        schedule.repeat_every_days && schedule.repeat_every_days > 0
          ? schedule.repeat_every_days
          : undefined,
      repeatEnd: schedule.repeat_end
        ? normalizeScheduledTaskRepeatEnd(schedule.repeat_end)
        : undefined,
    };
  }
  return {
    kind: "cron",
    cron: schedule.cron ?? "",
    timezone: schedule.timezone,
  };
}

export function toScheduledTaskScheduleWire(
  schedule: ScheduledTaskSchedule,
): ScheduledTaskScheduleWire {
  if (schedule.kind === "calendar") {
    return {
      kind: "calendar",
      timezone: schedule.timezone,
      run_at: schedule.runAt,
      repeat_every_days: schedule.repeatEveryDays,
      repeat_end: schedule.repeatEnd
        ? toScheduledTaskRepeatEndWire(schedule.repeatEnd)
        : undefined,
    };
  }
  return {
    kind: "cron",
    timezone: schedule.timezone,
    cron: schedule.cron,
  };
}

export function normalizeScheduledTaskExecutionOptions(
  options: ScheduledTaskExecutionOptionsWire,
): ScheduledTaskExecutionOptions {
  return {
    profileId: options.profile_id,
    timeoutSeconds: options.timeout_seconds,
    misfireGraceSeconds: options.misfire_grace_seconds,
  };
}

export function toScheduledTaskExecutionOptionsWire(
  options: ScheduledTaskExecutionOptions,
): ScheduledTaskExecutionOptionsWire {
  return {
    profile_id: options.profileId,
    timeout_seconds: options.timeoutSeconds,
    misfire_grace_seconds: options.misfireGraceSeconds,
  };
}

export function normalizeScheduledTaskTemplateOrigin(
  origin: ScheduledTaskTemplateOriginWire,
): ScheduledTaskTemplateOrigin {
  return {
    templateId: origin.template_id,
    source: origin.source,
  };
}

export function toScheduledTaskTemplateOriginWire(
  origin: ScheduledTaskTemplateOrigin,
): ScheduledTaskTemplateOriginWire {
  return {
    template_id: origin.templateId,
    source: origin.source,
  };
}

export function normalizeScheduledTaskLastExecution(
  execution: ScheduledTaskLastExecutionWire,
): ScheduledTaskLastExecution {
  return {
    executionId: execution.execution_id,
    status: execution.status,
    startedAt: presentTime(execution.started_at),
    finishedAt: presentTime(execution.finished_at),
  };
}

export function normalizeScheduledTask(task: ScheduledTaskWire): ScheduledTask {
  return {
    taskId: task.task_id,
    version: task.version,
    name: task.name,
    enabled: Boolean(task.enabled),
    projectId: task.project_id,
    threadId: task.thread_id || undefined,
    schedule: normalizeScheduledTaskSchedule(task.schedule),
    action: { kind: task.action.kind, content: task.action.content },
    execution: normalizeScheduledTaskExecutionOptions(task.execution),
    templateOrigin: task.template_origin
      ? normalizeScheduledTaskTemplateOrigin(task.template_origin)
      : undefined,
    state: task.state,
    invalidReason: task.invalid_reason || undefined,
    nextRunAt: task.next_run_at ?? null,
    lastExecution: task.last_execution
      ? normalizeScheduledTaskLastExecution(task.last_execution)
      : null,
    createdAt: task.created_at,
    updatedAt: task.updated_at,
  };
}

export function normalizeScheduledTaskExecution(
  execution: ScheduledTaskExecutionWire,
): ScheduledTaskExecution {
  return {
    executionId: execution.execution_id,
    taskId: execution.task_id,
    taskVersion: execution.task_version,
    trigger: execution.trigger,
    scheduledFor: execution.scheduled_for,
    startedAt: presentTime(execution.started_at),
    finishedAt: presentTime(execution.finished_at),
    status: execution.status,
    runId: execution.run_id || undefined,
    threadId: execution.thread_id || undefined,
    errorCode: execution.error_code || undefined,
    errorMessage: execution.error_message || undefined,
  };
}

export function normalizeScheduledTaskEvent(
  event: ScheduledTaskEventWire,
): ScheduledTaskEvent {
  return {
    type: event.type,
    taskId: event.task_id,
    executionId: event.execution_id || undefined,
    threadId: event.thread_id || undefined,
    taskName: event.task_name || undefined,
    status: event.status || undefined,
    content: event.content || undefined,
  };
}

export function toScheduledTaskCreateWire(
  input: ScheduledTaskCreateInput,
): ScheduledTaskCreateWire {
  return {
    name: input.name,
    enabled: input.enabled,
    project_id: input.projectId,
    schedule: toScheduledTaskScheduleWire(input.schedule),
    action: { kind: input.action.kind, content: input.action.content },
    execution: toScheduledTaskExecutionOptionsWire(input.execution),
    template_origin: input.templateOrigin
      ? toScheduledTaskTemplateOriginWire(input.templateOrigin)
      : undefined,
  };
}

export function toScheduledTaskPatchWire(
  input: ScheduledTaskPatchInput,
): ScheduledTaskPatchWire {
  return {
    expected_version: input.expectedVersion,
    name: input.name,
    enabled: input.enabled,
    project_id: input.projectId,
    schedule: input.schedule ? toScheduledTaskScheduleWire(input.schedule) : undefined,
    action: input.action
      ? { kind: input.action.kind, content: input.action.content }
      : undefined,
    execution: input.execution
      ? toScheduledTaskExecutionOptionsWire(input.execution)
      : undefined,
    template_origin: input.templateOrigin
      ? toScheduledTaskTemplateOriginWire(input.templateOrigin)
      : undefined,
  };
}
