import type { DesktopRequestResult } from "./runtime-request.js";
import type { AguiEvent, InstructionSnapshotSummary } from "./agui.js";
import type { DesktopUpdateSnapshot } from "./desktop-update.js";
import type { PetBridge } from "./pets.js";
import type { ProjectFilePreviewInteraction } from "./project-file-preview-interaction.js";

export type DesktopPlatform = "macos" | "windows" | "linux";

export type DesktopOperationIssueCode =
  | "credential_unreadable"
  | "composer_selection_invalid"
  | "plugin_component_unavailable"
  | "plugin_selection_invalid"
  | "run_conflict"
  | "run_input_invalid"
  | "runtime_unavailable"
  | "file_comment_stale"
  | "invalid_file_comment"
  | "operation_failed";

export type DesktopOperationRecoveryAction = "retry" | "relaunch" | "reenter";

export type DesktopOperationIssue = {
  code: DesktopOperationIssueCode;
  message: string;
  retryable: boolean;
  actions: DesktopOperationRecoveryAction[];
  commentId?: string;
  reason?: string;
};

export type DesktopOperationResult<T, Issue = DesktopOperationIssue> =
  { ok: true; value: T } | { ok: false; issue: Issue };

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
  "queued" | "running" | "completed" | "failed" | "cancelled" | "interrupted";

export type MemoryDreamTask = {
  taskId: string;
  status: MemoryDreamStatus;
  phase?: string;
  projectId?: string;
  scopes: MemoryScope[];
  reports?: Partial<
    Record<
      MemoryScope,
      {
        updated: number;
        superseded: number;
        conflicts: number;
        skipped: number;
      }
    >
  >;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type RuntimeRunStatus = "running" | "cancelling" | "finished" | "error" | "cancelled";

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

export type FileLineCommentWire = {
  id: string;
  kind?: "comment" | "selection";
  project_id: string;
  folder_path: string;
  relative_path: string;
  display_name: string;
  content_digest: string;
  start_line: number;
  end_line: number;
  selected_text: string;
  comment: string;
  created_at: string;
};

export type FileLineComment = {
  id: string;
  kind?: "comment" | "selection";
  projectId: string;
  folderPath: string;
  relativePath: string;
  displayName: string;
  contentDigest: string;
  startLine: number;
  endLine: number;
  selectedText: string;
  comment: string;
  createdAt: string;
  validationState?: "ready" | "stale";
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
  file_comments?: FileLineCommentWire[];
  status: RuntimeRunStatus;
  result?: string;
  error?: string;
  cancel_requested?: boolean;
  created_at: string;
  updated_at: string;
  finished_at?: string | null;
  model_profile_id?: string;
  reasoning_effort?: ReasoningEffortLevel;
  hook_snapshot_id?: string;
	instruction_snapshot?: InstructionSnapshotSummaryWire;
};

export type InstructionSnapshotSummaryWire = {
	snapshot_id: string;
	captured_at: string;
	partial: boolean;
	sources?: Array<{
		scope: "global" | "project";
		display_path: string;
		scope_root?: string;
		digest?: string;
		original_bytes: number;
		applied_bytes: number;
		truncated: boolean;
		status: "applied" | "skipped";
	}>;
	diagnostics?: Array<{
		code: string;
		message: string;
		scope: "global" | "project";
		display_path: string;
	}>;
};

export type GlobalAgentsDocumentWire = {
	exists: boolean;
	revision: string;
	digest: string;
	content: string;
	byte_count: number;
	updated_at?: string;
	max_bytes: number;
};

export type GlobalAgentsDocument = {
	exists: boolean;
	revision: string;
	digest: string;
	content: string;
	byteCount: number;
	updatedAt?: string;
	maxBytes: number;
};

export type GlobalAgentsSaveRequest = { expectedRevision: string; content: string };
export type GlobalAgentsDeleteRequest = { expectedRevision: string };

export type ProjectAgentsCreateResult =
	| { ok: true; projectId: string; folderPath: string; relativePath: ".awaken/AGENTS.md"; name: "AGENTS.md" }
	| { ok: false; code: "UNAUTHORIZED" | "PROJECT_NOT_FOUND" | "PRIMARY_FOLDER_MISSING" | "FILE_EXISTS" | "CREATE_FAILED"; message: string };

export type RuntimeRun = {
  runId: string;
  threadId: string;
  input?: string;
  inputAttachments?: RunInputAttachment[];
  composerTokens?: ComposerToken[];
  fileComments?: FileLineComment[];
  status: RuntimeRunStatus;
  result?: string;
  error?: string;
  cancelRequested: boolean;
  createdAt: string;
  updatedAt: string;
  finishedAt?: string;
  modelProfileId?: string;
  reasoningEffort?: ReasoningEffortLevel;
  hookSnapshotId?: string;
	instructionSnapshot?: InstructionSnapshotSummary;
};

export type HookSourceStatus = "enabled" | "disabled" | "untrusted" | "invalid";
export type HookReviewItem = {
  id: string;
  event: string;
  type: "command" | "http" | "prompt" | "agent";
  toolNames?: string[];
  command?: string[];
  cwd?: string;
  url?: string;
  allowLoopback?: boolean;
  secretHeaderNames?: string[];
  prompt?: string;
  model?: string;
  maxTurns?: number;
  timeoutMs: number;
  maxOutputBytes: number;
  onFailure: "continue" | "block";
};
export type HookSourceReceipt = {
  kind: "user" | "project" | "project_local";
  path: string;
  projectRoot?: string;
  digest?: string;
  status: HookSourceStatus;
  hookCount: number;
  diagnostic?: string;
  hooks: HookReviewItem[];
};
export type HookDiagnostic = { code: string; message: string; sourcePath?: string };
export type HookStatus = {
  snapshotId: string;
  schemaVersion: number;
  createdAt: string;
  enabledHookCount: number;
  sources: HookSourceReceipt[];
  diagnostics?: HookDiagnostic[];
};
export type HookTrustRequest = { projectId: string; sourcePath: string; digest: string };
export type HookRevokeRequest = { projectId: string; sourcePath: string };

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
  selected_model_profile_id?: string;
  reasoning_effort_by_profile?: Record<string, ReasoningEffortLevel>;
  workspace: WorkspaceConfigWire;
  created_at: string;
  updated_at: string;
  archived?: boolean;
  /** Go zero time serializes as "0001-01-01T00:00:00Z" and means absent. */
  archived_at?: string;
  source?: string;
  acp_session_id?: string;
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
  selectedModelProfileId?: string;
  reasoningEffortByProfile?: Record<string, ReasoningEffortLevel>;
  workspace: WorkspaceConfig;
  createdAt: string;
  updatedAt: string;
  archived: boolean;
  archivedAt?: string;
  source?: string;
  acpSessionId?: string;
};

export type RuntimeThreadListView = "active" | "archived";

export type SetThreadArchivedResult =
  | { ok: true; thread: RuntimeThread }
  | { ok: false; code: string; message: string };

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

export const PROJECT_DIRECTORY_CHANGED_CHANNEL = "awaken:project-directory-changed";

export type ProjectDirectoryWatchRequest = {
  projectId: string;
  folderPath: string;
};

export type ProjectDirectoryWatchResult =
  | {
      ok: true;
      subscriptionId: string;
    }
  | {
      ok: false;
      code:
        | Exclude<ProjectDirectoryListResult, { ok: true }>["code"]
        | "WATCH_UNAVAILABLE";
      message: string;
    };

export type ProjectDirectoryChangedEvent = {
  subscriptionId: string;
};

export type ProjectEntryKind = "directory" | "file";

export type ProjectEntryActionRequest = {
  projectId: string;
  folderPath: string;
  /** Project-folder-relative path using forward slashes on every platform. */
  relativePath: string;
  kind: ProjectEntryKind;
};

export type ProjectEntryActionErrorCode =
  | "INVALID_REQUEST"
  | "UNAUTHORIZED"
  | "PROJECT_NOT_FOUND"
  | "FOLDER_NOT_ATTACHED"
  | "PATH_OUTSIDE_PROJECT"
  | "ENTRY_NOT_FOUND"
  | "ENTRY_UNREADABLE"
  | "ENTRY_TYPE_MISMATCH"
  | "UNSUPPORTED_PLATFORM"
  | "UNSUPPORTED_APPLICATION"
  | "APPLICATION_UNAVAILABLE"
  | "APPLICATION_OPEN_FAILED"
  | "ENTRY_REVEAL_FAILED";

export type ProjectEntryReference = {
  projectId: string;
  folderPath: string;
  absolutePath: string;
  relativePath: string;
  displayName: string;
  kind: ProjectEntryKind;
};

export type ProjectEntryResolveResult =
  | { ok: true; entry: ProjectEntryReference }
  | {
      ok: false;
      code: ProjectEntryActionErrorCode;
      message: string;
    };

export type ProjectEntryActionResult =
  | { ok: true }
  | {
      ok: false;
      code: ProjectEntryActionErrorCode;
      message: string;
    };

export type ProjectEntryOpenApplication = "wps" | "office" | "vscode" | "cursor" | "trae" | "xcode";

export type ProjectEntryOpenWithRequest = ProjectEntryActionRequest & {
  application: ProjectEntryOpenApplication;
};

export type ProjectFilePreviewKind = "office" | "markdown" | "code" | "pdf" | "image" | "html";

export type ProjectFilePreviewStartRequest = {
  /** Renderer-generated identity used to cancel work before a preview handle exists. */
  requestId: string;
  projectId: string;
  folderPath: string;
  /** Project-folder-relative path using forward slashes on every platform. */
  relativePath: string;
};

export type ProjectFilePreviewTextHandle = {
  previewId: string;
  requestId: string;
  kind: "markdown" | "code";
  transport: "text";
  displayName: string;
  relativePath: string;
  byteLength: number;
  content: string;
	contentDigest?: string;
  language?: string;
};

export type ProjectFilePreviewUrlHandle = {
  previewId: string;
  requestId: string;
  kind: "office" | "pdf" | "image" | "html";
  transport: "url";
  displayName: string;
  relativePath: string;
  byteLength: number;
  src: string;
  /** Only webview-backed previews receive an isolated, non-persistent partition. */
  partition?: string;
};

export type ProjectFilePreviewHandle = ProjectFilePreviewTextHandle | ProjectFilePreviewUrlHandle;

export type ProjectFilePreviewErrorCode =
  | "INVALID_REQUEST"
  | "UNAUTHORIZED"
  | "PROJECT_NOT_FOUND"
  | "FOLDER_NOT_ATTACHED"
  | "PATH_OUTSIDE_PROJECT"
  | "FILE_NOT_FOUND"
  | "FILE_UNREADABLE"
  | "FILE_UNWRITABLE"
	| "FILE_CHANGED"
  | "UNSUPPORTED_FILE_TYPE"
  | "FILE_TOO_LARGE"
  | "IMAGE_TOO_LARGE"
  | "TEXT_BINARY_CONTENT"
  | "OFFICECLI_UNAVAILABLE"
  | "OFFICECLI_VERSION_MISMATCH"
  | "OFFICE_RENDER_FAILED"
  | "OFFICE_RENDER_TIMEOUT"
  | "PREVIEW_CANCELLED"
  | "PREVIEW_RESOURCE_LIMIT"
  | "PREVIEW_UNAVAILABLE";

export type ProjectFilePreviewStartResult =
  | { ok: true; preview: ProjectFilePreviewHandle }
  | {
      ok: false;
      code: ProjectFilePreviewErrorCode;
      message: string;
      byteLength?: number;
      limitBytes?: number;
    };

export type ProjectFilePreviewCloseRequest = {
  previewId?: string;
  requestId?: string;
};

export type ProjectFilePreviewActionRequest = {
  projectId: string;
  folderPath: string;
  relativePath: string;
};

export type ProjectFilePreviewActionResult =
  | { ok: true }
  | {
      ok: false;
      code: ProjectFilePreviewErrorCode;
      message: string;
    };

export type ProjectFilePreviewSaveRequest = ProjectFilePreviewActionRequest & {
  content: string;
	expectedDigest?: string;
};

export type ProjectFilePreviewSaveResult =
  | { ok: true; byteLength: number; digest?: string }
  | {
      ok: false;
      code: ProjectFilePreviewErrorCode;
      message: string;
      byteLength?: number;
      limitBytes?: number;
		currentContent?: string;
		currentDigest?: string;
		submittedContent?: string;
    };

/** Folder list submitted when creating a project. An empty list is legal: the
 * project exists under its own name and binds folders later, if ever. */
export type ProjectFolderSelection = {
  path: string;
  primary?: boolean;
};

export type RuntimeProjectOperationAction =
  "detach" | "delete_history" | "trash_and_purge" | "delete_thread";

export type RuntimeProjectOperationStatus =
  "accepted" | "running" | "succeeded" | "failed" | "blocked";

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
  { ok: true; project: RuntimeProject } | { ok: false; code: string; message: string };

export type ProjectReorderResult =
  { ok: true; projects: RuntimeProject[] } | { ok: false; code: string; message: string };

export type ThreadDeleteResult = { ok: true } | { ok: false; code: string; message: string };

export type ThreadBatchDeleteFailure = {
  threadId: string;
  code: string;
  message: string;
};

export type ThreadBatchDeleteResult =
  | {
      ok: true;
      deletedThreadIds: string[];
      failures: ThreadBatchDeleteFailure[];
    }
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
  activity_cursor?: number;
  thread: RuntimeThreadWire;
  runs: RuntimeThreadTranscriptRunWire[];
};

export type RuntimeThreadTranscriptRun = {
  run: RuntimeRun;
  events: AguiEvent[];
  streamCursor?: number;
};

export type RuntimeThreadTranscript = {
  activityCursor?: number;
  thread: RuntimeThread;
  runs: RuntimeThreadTranscriptRun[];
};

export type RewindThreadRequest = {
  threadId: string;
  targetRunId: string;
  expectedHeadRunId: string;
};

export type RewindThreadResultWire = {
  thread: RuntimeThreadWire;
  target_run: RuntimeRunWire;
  removed_run_ids: string[];
};

export type RewindThreadResult = {
  thread: RuntimeThread;
  targetRun: RuntimeRun;
  removedRunIds: string[];
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
  "disconnected" | "pending" | "connected" | "needs-auth" | "approval-required" | "failed";

export type RuntimeConnectorKind = "mcp" | "cli";
export type RuntimeConnectorCatalogPlacement = "connectors" | "plugin_only";

export type CliProviderId = "dingtalk-workspace" | "lark-cli";
export type CliActivation = "enabled" | "disabled";
export type CliInstallState = "missing" | "installing" | "installed" | "unsupported" | "failed";
export type CliExecutableState = "unknown" | "missing" | "available" | "invalid";
export type CliConfigurationState = "unknown" | "required" | "ready" | "failed";
export type CliAuthenticationState =
  "unknown" | "required" | "in_progress" | "authorized" | "failed";
export type CliOverallState =
  "disabled" | "not_ready" | "action_required" | "working" | "ready" | "failed";
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
  "starting" | "waiting_user" | "confirming" | "authorized" | "failed" | "cancelled";

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
  catalogPlacement?: RuntimeConnectorCatalogPlacement;
  authType: string;
  lastError?: string;
  serverNames?: string[];
  shadowedBy?: string;
  cliReadiness?: CliConnectorReadiness;
  legacyMcpReviewRequired?: boolean;
  account?: {
    displayName?: string;
    tenantName?: string;
  };
};

export type RuntimeConnectorActionResponse = {
  connectorId: string;
  connectorState: RuntimeConnectorState | string;
  sessionId?: string;
  authStatus?: RuntimeConnectorAuthStatus;
  expiresAt?: string;
  message?: string;
};

export type RuntimeConnectorAuthStatus =
  | "authorizing"
  | "verifying"
  | "pending_admin_approval"
  | "ready"
  | "failed"
  | "expired"
  | "cancelled";

export type RuntimeConnectorAuthSession = {
  sessionId: string;
  connectorId: string;
  status: RuntimeConnectorAuthStatus;
  errorMessage?: string;
  expiresAt?: string;
};

export type RuntimeMcpServerState = "pending" | "connected" | "failed" | "needs-auth" | "disabled";

export type RuntimeMcpServer = {
  name: string;
  displayName?: string;
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

export type RuntimeSkillState = "loaded" | "invalid" | "shadowed" | "disabled_by_policy";

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

export type RuntimePluginDiagnostic = {
  code?: string;
  message?: string;
  severity?: string;
  phase?: string;
  componentKind?: RuntimePluginComponentKind;
  componentId?: string;
  sourcePath?: string;
  sourceDialect?: RuntimePluginDialect;
  remediation?: string;
  rawField?: string;
};

export type RuntimePluginDialect = "agent_plugins" | "codex" | "claude_code" | "cursor" | string;

export type RuntimePluginComponentKind =
  | "skill"
  | "mcp_server"
  | "rule"
  | "agent"
  | "command"
  | "hook"
  | "variable"
  | "app"
  | "unknown"
  | string;

export type RuntimePluginMappingState =
  | "native"
  | "adapted"
  | "recognized_unsupported"
  | "ignored_by_dialect"
  | "invalid"
  | string;

export type RuntimePluginSource = {
  kind: "local_path";
  path: string;
  dialectHint?: RuntimePluginDialect;
  selectedDialect?: RuntimePluginDialect;
};

export type RuntimePluginVariableSpec = {
  key: string;
  type: string;
  title?: string;
  description?: string;
  required?: boolean;
  default?: unknown;
  enum?: unknown[];
  const?: unknown;
  secret?: boolean;
  multiple?: boolean;
  minimum?: number;
  maximum?: number;
};

export type RuntimePluginComponent = {
  logicalId: string;
  logicalIdEncoded: string;
  revisionId?: string;
  kind: RuntimePluginComponentKind;
  declaredName?: string;
  derivedName?: string;
  displayName?: string;
  description?: string;
  sourceDialect: RuntimePluginDialect;
  sourceManifestPath?: string;
  sourcePath?: string;
  sourceDigest?: string;
  mappingState: RuntimePluginMappingState;
  validation: string;
  canonicalProvenance?: string;
  secondaryProvenance?: string[];
  diagnostics?: RuntimePluginDiagnostic[];
  skill?: { path: string; catalogId?: string };
  mcp?: { shortName?: string; transport?: string; connectionState?: string; toolsLoaded?: boolean; toolCount?: number; hostManaged?: boolean; connectorId?: string };
  rule?: { activation?: string; globs?: string[]; scope?: string };
  agent?: { model?: string; readOnly?: boolean; maxTurns?: number; timeoutSeconds?: number };
  command?: { invocationName?: string; argumentHint?: string; acceptsArguments?: boolean; argumentsRequired?: boolean };
  hook?: { sourceEvent?: string; mappedEvent?: string; matcher?: string; condition?: string; handlerType?: string; scope?: string; timeoutSeconds?: number; async?: boolean; asyncRewake?: boolean };
  variable?: RuntimePluginVariableSpec;
  app?: Record<string, unknown>;
};

export type RuntimePluginComponentState = {
  component: RuntimePluginComponent;
  desiredState: string;
  ruleActivation?: "always" | "file_globs" | "manual" | "model_decides" | string;
  configurationState: string;
  authenticationState?: string;
  runtimeState: string;
  integrityState: string;
  mappingState: RuntimePluginMappingState;
  switchable: boolean;
};

export type RuntimePluginSetupKind =
  | "configuration"
  | "secret"
  | "oauth_pkce"
  | "oauth_device"
  | "mcp_oauth"
  | "cli_login"
  | "external_setup"
  | "host_connector"
  | "app_connector";

export type RuntimePluginSetupStatus =
  | "ready"
  | "needs_configuration"
  | "needs_auth"
  | "authorizing"
  | "verifying"
  | "pending_admin_approval"
  | "expired"
  | "failed"
  | "unsupported";

export type RuntimePluginSetupAction =
  | "configure"
  | "authorize"
  | "cancel"
  | "mark_admin_approval"
  | "retry"
  | "probe"
  | "disconnect"
  | "inspect_only";

export type RuntimePluginSetupRequirement = {
  id: string;
  fingerprint: string;
  kind: RuntimePluginSetupKind;
  title?: string;
  description?: string;
  required?: boolean;
  componentIds?: string[];
  mcpServerName?: string;
  field?: RuntimePluginVariableSpec;
  providerId?: string;
  externalUrl?: string;
  connectorId?: string;
  sourceDialect: RuntimePluginDialect;
  sourcePath?: string;
};

export type RuntimePluginSetupItem = {
  requirement: RuntimePluginSetupRequirement;
  status: RuntimePluginSetupStatus;
  reasonCode?: string;
  message?: string;
  actions: RuntimePluginSetupAction[];
};

export type RuntimePluginSetupSummary = {
  total: number;
  ready: number;
  needsConfiguration: number;
  needsAuth: number;
  needsAdminApproval: number;
  inProgress: number;
  failed: number;
  unsupported: number;
};

export type RuntimePluginSetupResponse = {
  items: RuntimePluginSetupItem[];
  summary: RuntimePluginSetupSummary;
};

export type RuntimePluginSetupMutationRequest = {
  pluginDigest: string;
  requirementFingerprint: string;
  force?: boolean;
};

export type RuntimePluginAuthSession = {
  sessionId: string;
  pluginName: string;
  setupId: string;
  status: RuntimePluginSetupStatus | "cancelled";
  errorMessage?: string;
  expiresAt?: string;
};

export type RuntimePluginComponentDetail = {
  state: RuntimePluginComponentState;
  preview?: string;
  source?: string;
  sourceEditable: boolean;
  properties?: Record<string, unknown>;
  resources?: string[];
  hookReceipts?: RuntimePluginHookReceipt[];
  hookReceiptRetention?: string;
};

export type UpdatePluginComponentSourceRequest = {
  expectedDigest: string;
  source: string;
};

export type RuntimePluginComponentSourceResponse = {
  plugin: RuntimePlugin;
  revision: number;
  changed: boolean;
};

export type RuntimePluginHookReceipt = {
  id: string;
  pluginName: string;
  pluginDigest: string;
  logicalId: string;
  revisionId: string;
  event: string;
  status: string;
  action: string;
  reasonCode?: string;
  durationMs: number;
  createdAt: string;
  retryOf?: string;
};

export type RuntimePluginHookReceiptResponse = {
  items: RuntimePluginHookReceipt[];
  nextCursor?: string;
  retention: "memory_only" | string;
};

export type RuntimePluginComponentTotals = {
  total: number;
  runnable: number;
  needsConfiguration: number;
  invalid: number;
  unsupported: number;
  ignored: number;
};

export type RuntimePluginVariableField = {
  key: string;
  schema: RuntimePluginVariableSpec;
  configured: boolean;
  masked: boolean;
  value?: unknown;
  dependencies?: Array<{ logicalId: string; kind: RuntimePluginComponentKind; name: string }>;
};

export type RuntimePluginServer = {
  shortName: string;
  identity: string;
  transport?: string;
  state?: string;
  enabled: boolean;
  errorReason?: string;
  invalid?: RuntimePluginDiagnostic;
};

export type RuntimePlugin = {
  name: string;
  displayName?: string;
  builtin?: boolean;
  brandIcon?: string;
  builtinOrder?: number;
  version?: string;
  description?: string;
  author?: { name?: string; email?: string; url?: string };
  homepage?: string;
  repository?: string;
  license?: string;
  keywords?: string[];
  enabled: boolean;
  dialect?: RuntimePluginDialect;
  manifestPath?: string;
  installationId?: string;
  configurationState?: string;
  authenticationState?: string;
  compatibilityState?: string;
  setupItems?: RuntimePluginSetupItem[];
  setupSummary?: RuntimePluginSetupSummary;
  componentTotals?: RuntimePluginComponentTotals;
  componentCounts?: Record<string, number>;
  source: RuntimePluginSource;
  digest?: string;
  installedAt?: string;
  skillRoot?: string;
  skills?: RuntimeSkill[];
  servers?: RuntimePluginServer[];
  diagnostics?: RuntimePluginDiagnostic[];
  skillCount: number;
  serverCount: number;
};

export type RuntimePluginValidation = {
  valid: boolean;
  source: RuntimePluginSource;
  name: string;
  version?: string;
  description?: string;
  digest: string;
  targetRoot: string;
  skillCount: number;
  serverCount: number;
  dialect?: RuntimePluginDialect;
  manifestPath?: string;
  components?: RuntimePluginComponent[];
  componentTotals?: RuntimePluginComponentTotals;
  candidates?: Array<{
    dialect: RuntimePluginDialect;
    manifestPath: string;
    manifestless?: boolean;
    identityHint?: string;
    valid: boolean;
    reason?: string;
  }>;
  requiresSelection?: boolean;
  selectedDialect?: RuntimePluginDialect;
  requiredVariables?: string[];
  processIntents?: Array<{ componentId: string; kind: string; summary: string }>;
  diagnostics: RuntimePluginDiagnostic[];
};

export type InstallPluginRequest = {
  source: RuntimePluginSource;
  replace?: boolean;
  expectedDigest?: string;
  installName?: string;
};

export type CommitPluginRequest = {
  expectedDigest: string;
};

export type InstallPluginResponse = {
  plugin: RuntimePlugin;
  message: string;
};

export type RuntimePluginConfigurationResponse = {
  fields: RuntimePluginVariableField[];
  revision: number;
};

export type UpdatePluginConfigurationRequest = {
  values: Record<string, unknown>;
  reset?: string[];
};

export type RuntimePluginComponentStateResponse = {
  plugin: RuntimePlugin;
  revision: number;
};

export type RuntimePluginMcpTool = {
  name: string;
  description?: string;
  enabled: boolean;
};

export type ComposerTokenType =
  | "skill"
  | "plugin"
  | "rule"
  | "command"
  | "agent"
  | "file"
  | "workflow"
  | "prompt"
  | "connector"
  | "capability";

export type SkillComposerTokenPayload = {
  skillId: string;
  sourceOrigin: "project" | "user" | string;
  skillFilePath: string;
  rootDir?: string;
};

export type PluginComposerTokenPayload = {
  pluginName: string;
  pluginDigest: string;
};

export type PluginRuleComposerTokenPayload = PluginComposerTokenPayload & {
  ruleLogicalId: string;
};

export type PluginCommandComposerTokenPayload = PluginComposerTokenPayload & {
  commandLogicalId: string;
  commandArguments?: string;
};

export type PluginAgentComposerTokenPayload = PluginComposerTokenPayload & {
  agentLogicalId: string;
};

export type ProjectPathComposerTokenPayload = {
  path: string;
  entryKind: ProjectEntryKind;
  projectId: string;
  folderPath: string;
  relativePath: string;
};

export type ComputerUseComposerTokenPayload = {
  capabilityId: "computer-use";
};

export type WorkflowComposerTokenPayload = {
  workflowId: "deep-research" | string;
};

export type ComposerCatalogEvent = {
  revision: number;
  kind: "snapshot" | "skills" | "plugins" | string;
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
  sourcePath?: string;
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
  resources?: SkillPreviewResource[];
  resourcesTruncated?: boolean;
};

export type SkillPreviewResource = {
  path: string;
  kind: "file" | "directory" | "symlink" | "other";
  sizeBytes: number;
};

export type ModelProviderWire = {
  id: string;
  display_name: string;
  base_url: string;
  protocol: string;
  supported_protocols?: string[];
  official_protocols?: string[];
  protocol_base_urls?: Record<string, string>;
  protocol_auth_schemes?: Record<string, string>;
  catalog_strategy?: ModelCatalogStrategy;
  catalog_url?: string;
  catalog_auth_scheme?: string;
  catalog_source?: ModelCatalogSource;
  catalog_observed_at?: string;
  auth_scheme: string;
  key_env_name: string;
  default_model: string;
  models: string[];
  model_display_names?: Record<string, string>;
  docs_url: string;
  api_docs_url: string;
  access_type?: ModelAccessType;
  coding_preset?: boolean;
  endpoint_model?: boolean;
  allow_custom_model?: boolean;
  model_capabilities?: Record<string, ModelCapabilityWire>;
};

export type ModelCapabilityWire = {
  supported_protocols?: string[];
  max_input_tokens?: number;
  context_window?: number;
  max_output_tokens?: number;
  default_max_tokens?: number;
  default_temperature?: number;
  supports_vision?: boolean;
  supports_native_pdf?: boolean;
  supports_video?: boolean;
  supports_tools?: ModelToolCapabilityStatus;
  supports_prompt_cache?: boolean;
  reasoning_effort?: ReasoningEffortCapabilityWire;
  field_sources?: Partial<Record<ModelCapabilityField, ModelCapabilityFieldSourceWire>>;
};

export type ModelCatalogStrategy = "openai" | "anthropic" | "google" | "none";

export type ModelCatalogSource = "provider_api" | "official_manifest" | "unavailable";

export type ModelCapabilityField =
  | "supportedProtocols"
  | "maxInputTokens"
  | "contextWindow"
  | "maxOutputTokens"
  | "supportsVision"
  | "supportsNativePDF"
  | "supportsVideo"
  | "supportsTools"
  | "supportsPromptCache"
  | "reasoningEffort";

export type ModelCapabilitySource = "provider_api" | "official_manifest" | "user_override" | "unknown";

export type ModelCapabilityFieldSourceWire = {
  source: ModelCapabilitySource;
  observed_at?: string;
};

export type ModelCapabilityFieldSource = {
  source: ModelCapabilitySource;
  observedAt?: string;
};

export type ModelAccessType = "coding_plan" | "api" | "agent_plan" | "custom";

export type ModelSelectorKind = "model" | "endpoint";

export type ModelPurpose =
  | "thread_title"
  | "compact_summary"
  | "subagent"
  | "scheduled_task"
  | "hook_prompt"
  | "hook_agent";

export type ModelPurposeRouting = Partial<Record<ModelPurpose, string>>;

export type ReasoningEffortLevel = "low" | "high" | "max";

export type ReasoningEffortCapabilityWire = {
  levels: ReasoningEffortLevel[];
  default_level: ReasoningEffortLevel;
  display_labels?: Partial<Record<ReasoningEffortLevel, string>>;
};

export type ReasoningEffortCapability = {
  levels: ReasoningEffortLevel[];
  defaultLevel: ReasoningEffortLevel;
  displayLabels?: Partial<Record<ReasoningEffortLevel, string>>;
};

export type ModelToolCapabilityStatus = "ready" | "unsupported" | "unknown";

export type ModelProvider = {
  id: string;
  displayName: string;
  baseURL: string;
  protocol: string;
  supportedProtocols?: string[];
  officialProtocols: string[];
  protocolBaseURLs: Record<string, string>;
  protocolAuthSchemes: Record<string, string>;
  catalogStrategy?: ModelCatalogStrategy;
  catalogSource: ModelCatalogSource;
  catalogObservedAt?: string;
  authScheme: string;
  keyEnvName: string;
  defaultModel: string;
  models: string[];
  modelDisplayNames?: Record<string, string>;
  docsURL: string;
  apiDocsURL: string;
  accessType?: ModelAccessType;
  codingPreset: boolean;
  endpointModel: boolean;
  allowCustomModel: boolean;
  modelCapabilities: Record<string, ModelCapability>;
};

export type ModelCapability = {
  supportedProtocols?: string[];
  maxInputTokens?: number;
  contextWindow?: number;
  maxOutputTokens?: number;
  defaultMaxTokens?: number;
  defaultTemperature?: number;
  supportsVision?: boolean;
  supportsNativePDF?: boolean;
  supportsVideo?: boolean;
  supportsTools?: ModelToolCapabilityStatus;
  supportsPromptCache?: boolean;
  reasoningEffort?: ReasoningEffortCapability;
  fieldSources?: Partial<Record<ModelCapabilityField, ModelCapabilityFieldSource>>;
};

export function effectiveContextWindow(contextWindow?: number): number {
  const normalizedContextWindow =
    typeof contextWindow === "number" && Number.isFinite(contextWindow)
      ? Math.max(0, Math.round(contextWindow))
      : 0;
  return normalizedContextWindow;
}

export function calculateAutoCompactThreshold(contextWindow?: number): number {
  return Math.floor(effectiveContextWindow(contextWindow) * 0.9);
}

export type ModelProfileAdvancedWire = {
  temperature?: number;
  max_tokens?: number;
  endpoint_id?: string;
  prompt_cache_key?: string;
  inference_endpoint?: string;
  max_input_tokens?: number;
  context_window?: number;
  supports_vision?: boolean;
  supports_native_pdf?: boolean;
  supports_video?: boolean;
  supports_tools?: ModelToolCapabilityStatus;
  supports_prompt_cache?: boolean;
  default_reasoning_effort?: ReasoningEffortLevel;
};

export type ModelProfileAdvanced = {
  temperature?: number;
  maxTokens?: number;
  endpointId?: string;
  promptCacheKey?: string;
  inferenceEndpoint?: string;
  maxInputTokens?: number;
  contextWindow?: number;
  supportsVision?: boolean;
  supportsNativePDF?: boolean;
  supportsVideo?: boolean;
  supportsTools?: ModelToolCapabilityStatus;
  supportsPromptCache?: boolean;
  defaultReasoningEffort?: ReasoningEffortLevel;
};

export type ModelCredentialReferenceWire =
  | { kind: "profile_secret"; profile_id: string }
  | {
      kind: "provider_account";
      provider_id: "volcengine-agent-plan";
      account_id: string;
    };

export type ModelCredentialReference =
  | { kind: "profile_secret"; profileId: string }
  | {
      kind: "provider_account";
      providerId: "volcengine-agent-plan";
      accountId: string;
    };

export type ModelCredentialStatus = "ready" | "missing_key" | "disabled" | "unreadable";

export type RuntimeModelProfileSummaryWire = {
  id: string;
  provider_id: string;
  display_name: string;
  model_id: string;
  model_display_name?: string;
  base_url: string;
  protocol: string;
  auth_scheme: string;
  has_key: boolean;
  masked_key?: string;
  has_management_key?: boolean;
  masked_management_key?: string;
  enabled: boolean;
  is_default: boolean;
  profile_status: ModelCredentialStatus;
  credential_revision?: number;
  access_type?: ModelAccessType;
  credential_reference?: ModelCredentialReferenceWire;
  advanced?: ModelProfileAdvancedWire;
  effective_capability?: ModelCapabilityWire;
};

export type RuntimeModelProfileSummary = {
  id: string;
  providerId: string;
  displayName: string;
  credentialRevision?: number;
  accessType?: ModelAccessType;
  credentialReference?: ModelCredentialReference;
  modelId: string;
  modelDisplayName?: string;
  baseURL: string;
  protocol: string;
  authScheme: string;
  hasKey: boolean;
  maskedKey?: string;
  hasManagementKey?: boolean;
  maskedManagementKey?: string;
  enabled: boolean;
  isDefault: boolean;
  profileStatus: ModelCredentialStatus;
  advanced?: ModelProfileAdvanced;
  effectiveCapability?: ModelCapability;
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
  managementApiKey?: string;
  enabled?: boolean;
  isDefault?: boolean;
  advanced?: ModelProfileAdvanced;
  credentialReference?: ModelCredentialReference;
};

export type ModelProfileTestResult = {
  ok: boolean;
  message: string;
};

export type ModelCatalogRefreshResult = {
  provider: ModelProvider;
  message: string;
  discoveredModelIds?: string[];
};

export type ModelReasoningCapabilityTestResult = ModelProfileTestResult & {
  reasoningEffort?: ReasoningEffortCapability;
};

export const VOLCENGINE_AGENT_PLAN_ACCOUNT_ID = "volcengine-agent-plan-default";

export type AgentPlanCatalogKind =
  "llm" | "image" | "video" | "unsupported" | "unknown";

export type AgentPlanCatalogEntryWire = {
  id: string;
  kind: AgentPlanCatalogKind;
  display_name?: string;
};

export type AgentPlanCatalogEntry = {
  id: string;
  kind: AgentPlanCatalogKind;
  displayName?: string;
};

export type AgentPlanAccountWire = {
  account_id: typeof VOLCENGINE_AGENT_PLAN_ACCOUNT_ID;
  provider_id: "volcengine-agent-plan";
  has_key: boolean;
  masked_key?: string;
  credential_revision: number;
  model_ids: string[];
  catalog: AgentPlanCatalogEntryWire[];
  catalog_updated_at?: string;
};

export type AgentPlanAccount = {
  accountId: typeof VOLCENGINE_AGENT_PLAN_ACCOUNT_ID;
  providerId: "volcengine-agent-plan";
  hasKey: boolean;
  maskedKey?: string;
  credentialRevision: number;
  modelIds: string[];
  catalog: AgentPlanCatalogEntry[];
  catalogUpdatedAt?: string;
};

export type AgentPlanAccountSaveRequest = {
  apiKey?: string;
};

export function normalizeAgentPlanAccount(account: AgentPlanAccountWire): AgentPlanAccount {
  return {
    accountId: account.account_id,
    providerId: account.provider_id,
    hasKey: account.has_key,
    maskedKey: account.masked_key,
    credentialRevision: account.credential_revision,
    modelIds: [...account.model_ids],
    catalog: account.catalog.map((entry) => ({
      id: entry.id,
      kind: entry.kind,
      displayName: entry.display_name,
    })),
    catalogUpdatedAt: account.catalog_updated_at,
  };
}

export type AgentPlanMediaCapability =
  "media.image.generate" | "media.video.generate";

export type AgentPlanMediaModelKind = "image" | "video";

export type AgentPlanMediaDisplayState =
  | "disabled"
  | "needs_auth"
  | "unknown"
  | "ready_unverified"
  | "ready"
  | "denied"
  | "unavailable";

export type AgentPlanMediaToolConfigurationWire = {
  id: string;
  provider_id: "volcengine-agent-plan";
  account_id: typeof VOLCENGINE_AGENT_PLAN_ACCOUNT_ID;
  capability: AgentPlanMediaCapability;
  model_id: string;
  model_kind: AgentPlanMediaModelKind;
  enabled: boolean;
  configuration_revision: number;
  credential_revision: number;
  display_state: AgentPlanMediaDisplayState;
};

export type AgentPlanMediaToolConfiguration = {
  id: string;
  providerId: "volcengine-agent-plan";
  accountId: typeof VOLCENGINE_AGENT_PLAN_ACCOUNT_ID;
  capability: AgentPlanMediaCapability;
  modelId: string;
  modelKind: AgentPlanMediaModelKind;
  enabled: boolean;
  configurationRevision: number;
  credentialRevision: number;
  displayState: AgentPlanMediaDisplayState;
};

export type AgentPlanMediaToolsWire = {
  configurations: AgentPlanMediaToolConfigurationWire[];
};

export type AgentPlanMediaTools = {
  configurations: AgentPlanMediaToolConfiguration[];
};

export type AgentPlanMediaToolSaveRequest = {
  capability: AgentPlanMediaCapability;
  modelId: string;
  enabled?: boolean;
};

export type AgentPlanMediaToolsSaveResult = {
  settings: AgentPlanMediaTools;
  applyStatus: "applied" | "apply_failed";
  message?: string;
};

export function normalizeAgentPlanMediaTools(
  value: AgentPlanMediaToolsWire,
): AgentPlanMediaTools {
  return {
    configurations: value.configurations.map((configuration) => ({
      id: configuration.id,
      providerId: configuration.provider_id,
      accountId: configuration.account_id,
      capability: configuration.capability,
      modelId: configuration.model_id,
      modelKind: configuration.model_kind,
      enabled: configuration.enabled,
      configurationRevision: configuration.configuration_revision,
      credentialRevision: configuration.credential_revision,
      displayState: configuration.display_state,
    })),
  };
}

export type ModelBalanceStatusWire =
  | "available"
  | "unavailable"
  | "missing_key"
  | "error"
  | "unsupported"
  | "requires_setup";

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
  unit?: "requests" | "percent";
  unlimited?: boolean;
  reset_at?: string;
};

export type ModelUsageMetricWire = {
  name: string;
  value: number;
  unit: "tokens" | "requests" | "usd";
};

export type ModelBalanceDetailWire =
  | { kind: "amount"; amounts: ModelBalanceAmountWire[] }
  | { kind: "quota"; quotas: ModelBalanceQuotaWire[] }
  | { kind: "usage"; metrics: ModelUsageMetricWire[]; console_url?: string };

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
  setup_url?: string;
  setup_label?: string;
};

export type ModelProfileBalancesResultWire = {
  balances: ModelProfileBalanceWire[];
};

export type ModelBalanceStatus =
  | "available"
  | "unavailable"
  | "missing_key"
  | "error"
  | "unsupported"
  | "requires_setup";

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
  unit?: "requests" | "percent";
  unlimited?: boolean;
  resetAt?: string;
};

export type ModelBalanceDetail =
  | { kind: "amount"; amounts: ModelBalanceAmount[] }
  | { kind: "quota"; quotas: ModelBalanceQuota[] }
  | { kind: "usage"; metrics: Array<{ name: string; value: number; unit: "tokens" | "requests" | "usd" }>; consoleURL?: string };

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
  setupURL?: string;
  setupLabel?: string;
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
  file_comments?: FileLineCommentWire[];
  thread_id?: string;
  workspace: WorkspaceConfigWire;
  client: ClientInfoWire;
  model_profile_id?: string;
  reasoning_effort?: ReasoningEffortLevel;
  model_profile?: ModelProfileRunConfigWire;
};

export type StartRunRequest = {
  input: string;
  attachments?: RunInputAttachment[];
  composerTokens?: ComposerToken[];
  fileComments?: FileLineComment[];
  threadId?: string;
  workspace: WorkspaceConfig;
  client: ClientInfo;
  modelProfileId?: string;
  reasoningEffort?: ReasoningEffortLevel;
  /** Main-process/Core internal field. Renderer callers must use modelProfileId. */
  modelProfile?: ModelProfileRunConfig;
};

export type ThreadModelPreferenceRequest = {
  modelProfileId: string;
  reasoningEffort?: ReasoningEffortLevel;
};

export type ModelProfileRunConfigWire = {
  profile_id?: string;
  provider_id?: string;
  display_name?: string;
  model_id?: string;
  capability_model_id?: string;
  endpoint_selector?: string;
  model_selector_kind?: ModelSelectorKind;
  base_url?: string;
  inference_endpoint?: string;
  protocol?: string;
  auth_scheme?: string;
  api_key?: string;
  temperature?: number;
  max_tokens?: number;
  context_window?: number;
  max_input_tokens?: number;
  prompt_cache_key?: string;
  credential_revision?: number;
  supports_vision?: boolean;
  supports_native_pdf?: boolean;
  supports_video?: boolean;
  supports_tools?: ModelToolCapabilityStatus;
  supports_prompt_cache?: boolean;
  reasoning_effort?: ReasoningEffortLevel;
  purpose_profiles?: Partial<Record<ModelPurpose, ModelProfileRunConfigWire>>;
  purpose_profile_sources?: Partial<Record<ModelPurpose, "inherited" | "host_route" | "optional_route_fallback">>;
  config_snapshot_revision?: string;
};

export type ModelProfileRunConfig = {
  profileId?: string;
  providerId?: string;
  displayName?: string;
  modelId?: string;
  capabilityModelId?: string;
  endpointSelector?: string;
  modelSelectorKind?: ModelSelectorKind;
  baseURL?: string;
  inferenceEndpoint?: string;
  protocol?: string;
  authScheme?: string;
  apiKey?: string;
  temperature?: number;
  maxTokens?: number;
  contextWindow?: number;
  maxInputTokens?: number;
  promptCacheKey?: string;
  credentialRevision?: number;
  supportsVision?: boolean;
  supportsNativePDF?: boolean;
  supportsVideo?: boolean;
  supportsTools?: ModelToolCapabilityStatus;
  supportsPromptCache?: boolean;
  reasoningEffort?: ReasoningEffortLevel;
  purposeProfiles?: Partial<Record<ModelPurpose, ModelProfileRunConfig>>;
  purposeProfileSources?: Partial<Record<ModelPurpose, "inherited" | "host_route" | "optional_route_fallback">>;
  configSnapshotRevision?: string;
};

export type WorkspacePickResult =
  { canceled: true; error?: string } | { canceled: false; path: string };

/** Result of picking project folders. The picker allows several directories at
 * once, so `errors` reports the ones that could not be read while `paths` still
 * carries whatever was readable. */
export type ProjectFoldersPickResult =
  { canceled: true; error?: string } | { canceled: false; paths: string[]; errors?: string[] };

export type WorkspaceValidationResult = { valid: true } | { valid: false; error: string };

export type UserQuestionAnswer = {
  questionId: string;
  answer: string;
};

export type ArtifactOpenRequest = {
  threadId: string;
  runId: string;
  artifactId: string;
  /**
   * Compatibility locator for media paths recorded by image/video tool results
   * before the runtime emitted first-class Artifact events. Electron Main must
   * verify the exact path against the durable run transcript before use.
   */
  artifactPath?: string;
};

export type ArtifactPreviewStartRequest = ArtifactOpenRequest & {
  requestId: string;
};

export type ArtifactOpenErrorCode =
  | "runtime_unavailable"
  | "artifact_not_recorded"
  | "run_not_terminal"
  | "artifact_not_deliverable"
  | "unsupported_type"
  | "file_not_found"
  | "not_a_file"
  | "no_default_application"
  | "open_failed";

export type ArtifactOpenResult =
  { ok: true } | { ok: false; code: ArtifactOpenErrorCode; message: string };

export type ArtifactMediaSourceResult =
  | { ok: true; src: string; mimeType: string; byteLength: number }
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

export type ComputerHostStatus = {
  available: boolean;
  healthy: boolean;
  version?: string;
  protocol_version?: number;
  os?: string;
  tcc_screen_recording: boolean;
  tcc_accessibility: boolean;
  node_path?: string;
  host_script_path?: string;
  message?: string;
};

export type ComputerSettings = {
  enabled: boolean;
  allow_background_window_actions: boolean;
};

export type ComputerSettingsDocument = {
  revision: number;
  value: ComputerSettings;
};

export type ComputerSettingsPatch = {
  enabled?: boolean;
  allow_background_window_actions?: boolean;
};

export type ComputerSettingsSaveRequest = {
  expected_revision: number;
} & ComputerSettingsPatch;

export type ComputerRuntimeSetupResult = {
  node_ready: boolean;
  cua_host_ready: boolean;
  message?: string;
};

export type ComputerPermission = "screen_recording" | "accessibility";

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
// Scheduled tasks (contract [HTTP、IPC 与事件](docs/12_定时任务/定时执行与提醒/实现方案.md#httpipc-与事件)). Wire shapes mirror the Go
// json tags of src/scheduledtask; the camelCase projections are the only
// shapes Renderer code may touch.
// ---------------------------------------------------------------------------

export type ScheduledTaskState = "enabled" | "paused" | "running" | "completed" | "invalid";

export type ScheduledTaskActionKind = "reminder" | "agent";

export type ScheduledTaskActionWire = {
  kind: ScheduledTaskActionKind;
  content: string;
  composer_tokens?: ComposerToken[];
};

export type ScheduledTaskAction = {
  kind: ScheduledTaskActionKind;
  content: string;
  composerTokens?: ComposerToken[];
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
  "queued" | "running" | "success" | "error" | "skipped" | "cancelled" | "interrupted";

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
  "execution_started" | "execution_finished" | "notice" | "task_changed";

export type ScheduledTaskEventWire = {
  type: ScheduledTaskEventType;
  task_id: string;
  project_id?: string;
  execution_id?: string;
  run_id?: string;
  thread_id?: string;
  task_name?: string;
  status?: string;
  content?: string;
};

export type ScheduledTaskEvent = {
  type: ScheduledTaskEventType;
  taskId: string;
  projectId?: string;
  executionId?: string;
  runId?: string;
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
  saveSearchSettings: (request: SearchSettingsSaveRequest) => Promise<SearchSettingsSaveResult>;
  testSearchProvider: (request: SearchProviderTestRequest) => Promise<SearchProviderTestResult>;
  getACPRunners: () => Promise<DesktopRequestResult<ACPRunnersSettings>>;
  createACPRunner: (request: ACPRunnerCreateRequest) => Promise<DesktopRequestResult<ACPRunnerView>>;
  updateACPRunner: (runnerId: string, request: ACPRunnerUpdateRequest) => Promise<DesktopRequestResult<ACPRunnerView>>;
  deleteACPRunner: (runnerId: string) => Promise<DesktopRequestResult<void>>;
  probeACPRunner: (runnerId: string) => Promise<DesktopRequestResult<ACPRunnerProbeResult>>;
  getRun: (runId: string) => Promise<DesktopRequestResult<RuntimeRun>>;
  getThreadTranscript: (threadId: string) => Promise<DesktopRequestResult<RuntimeThreadTranscript>>;
  rewindThread: (request: RewindThreadRequest) => Promise<DesktopRequestResult<RewindThreadResult>>;
  listThreads: (view?: RuntimeThreadListView) => Promise<DesktopRequestResult<RuntimeThread[]>>;
  renameThread: (threadId: string, label: string) => Promise<DesktopRequestResult<RuntimeThread>>;
  setThreadArchived: (threadId: string, archived: boolean) => Promise<DesktopRequestResult<SetThreadArchivedResult>>;
  updateThreadModelPreference: (
    threadId: string,
    request: ThreadModelPreferenceRequest,
  ) => Promise<DesktopRequestResult<RuntimeThread>>;
  deleteThread: (threadId: string) => Promise<DesktopRequestResult<ThreadDeleteResult>>;
  deleteThreads: (threadIds: string[]) => Promise<DesktopRequestResult<ThreadBatchDeleteResult>>;
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
  ) => Promise<DesktopRequestResult<ProjectMutationResult>>;
  trashProjectFolder: (
    projectId: string,
    path: string,
    idempotencyKey: string,
  ) => Promise<ProjectMutationResult>;
  getProjectOperation: (operationId: string) => Promise<RuntimeProjectOperation>;
  openArtifact: (request: ArtifactOpenRequest) => Promise<ArtifactOpenResult>;
  getArtifactMediaSource: (
    request: ArtifactOpenRequest,
  ) => Promise<ArtifactMediaSourceResult>;
  revealPath: (target: string) => Promise<{ ok: boolean; message?: string }>;
  revealArtifact: (request: ArtifactOpenRequest) => Promise<ArtifactOpenResult>;
  startRun: (request: StartRunRequest) => Promise<DesktopOperationResult<RuntimeRun>>;
  cancelRun: (runId: string) => Promise<DesktopRequestResult<RuntimeRun>>;
  cancelSubagent: (runId: string, taskId: string, reason?: string) => Promise<DesktopRequestResult<void>>;
  answerUserQuestion: (runId: string, questionId: string, answer: string) => Promise<DesktopRequestResult<void>>;
  cancelUserQuestion: (runId: string, questionId: string) => Promise<DesktopRequestResult<void>>;
  resolveExternalInteraction: (
    runId: string,
    interactionId: string,
    resolution: "completed" | "cancelled" | "expired" | "failed",
    content?: Record<string, unknown>,
  ) => Promise<DesktopRequestResult<void>>;
  resolveACPPermission: (
    runId: string,
    interactionId: string,
    optionId: string,
    cancelled: boolean,
  ) => Promise<DesktopRequestResult<void>>;
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
  prepareComputerRuntime: () => Promise<ComputerRuntimeSetupResult>;
  requestComputerPermissions: () => Promise<ComputerHostStatus>;
  openComputerPermissionSettings: (
    permission: ComputerPermission,
  ) => Promise<void>;
  subscribeRun: (runId: string, handlers: RunStreamHandlers, afterSeq?: number) => Promise<DesktopRequestResult<string>>;
  subscribeThreadActivity?: (threadId: string, handlers: RunStreamHandlers, afterCursor?: number) => Promise<DesktopRequestResult<string>>;
  stopThreadAgents?: (threadId: string) => Promise<DesktopRequestResult<void>>;
  unsubscribeRun: (subscriptionId: string) => void;
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
  getRuntimeInfo: () => Promise<RuntimeInfo>;
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
  listPlugins: () => Promise<DesktopRequestResult<RuntimePlugin[]>>;
  getPlugin: (name: string) => Promise<DesktopRequestResult<RuntimePlugin>>;
  validatePlugin: (source: RuntimePluginSource) => Promise<DesktopRequestResult<RuntimePluginValidation>>;
  installPlugin: (request: InstallPluginRequest) => Promise<DesktopRequestResult<InstallPluginResponse>>;
  commitPlugin: (name: string, request: CommitPluginRequest) => Promise<DesktopRequestResult<InstallPluginResponse>>;
  enablePlugin: (name: string) => Promise<DesktopRequestResult<RuntimePlugin>>;
  disablePlugin: (name: string) => Promise<DesktopRequestResult<RuntimePlugin>>;
  uninstallPlugin: (name: string) => Promise<DesktopRequestResult<{ trashedPath?: string; message: string }>>;
  enablePluginServer: (name: string, shortName: string) => Promise<DesktopRequestResult<RuntimePlugin>>;
  disablePluginServer: (name: string, shortName: string) => Promise<DesktopRequestResult<RuntimePlugin>>;
  listPluginComponents: (name: string) => Promise<DesktopRequestResult<RuntimePluginComponentState[]>>;
  getPluginComponent: (
    name: string,
    kind: RuntimePluginComponentKind,
    logicalIdEncoded: string,
  ) => Promise<DesktopRequestResult<RuntimePluginComponentDetail>>;
  updatePluginComponentSource: (
    name: string,
    kind: RuntimePluginComponentKind,
    logicalIdEncoded: string,
    request: UpdatePluginComponentSourceRequest,
  ) => Promise<DesktopRequestResult<RuntimePluginComponentSourceResponse>>;
  setPluginComponentState: (
    name: string,
    kind: RuntimePluginComponentKind,
    logicalIdEncoded: string,
    enabled: boolean,
  ) => Promise<DesktopRequestResult<RuntimePluginComponentStateResponse>>;
  setPluginMcpToolState: (
    name: string,
    logicalIdEncoded: string,
    toolName: string,
    enabled: boolean,
  ) => Promise<DesktopRequestResult<RuntimePluginComponentStateResponse>>;
  setPluginRuleActivation: (
    name: string,
    logicalIdEncoded: string,
    activation: "always" | "file_globs" | "manual" | "model_decides",
  ) => Promise<DesktopRequestResult<{ plugin: RuntimePlugin; revision: number }>>;
  listPluginHookReceipts: (
    name: string,
    logicalIdEncoded: string,
    revisionId?: string,
  ) => Promise<DesktopRequestResult<RuntimePluginHookReceiptResponse>>;
  retryPluginHookReceipt: (
    name: string,
    logicalIdEncoded: string,
    receiptId: string,
  ) => Promise<DesktopRequestResult<{ receipt: RuntimePluginHookReceipt; diagnosticOnly: true }>>;
  getPluginConfiguration: (name: string) => Promise<DesktopRequestResult<RuntimePluginConfigurationResponse>>;
  updatePluginConfiguration: (
    name: string,
    request: UpdatePluginConfigurationRequest,
  ) => Promise<DesktopRequestResult<RuntimePluginConfigurationResponse>>;
  getPluginSetup: (name: string) => Promise<DesktopRequestResult<RuntimePluginSetupResponse>>;
  startPluginSetup: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<DesktopRequestResult<RuntimePluginAuthSession>>;
  getActivePluginAuthSession: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<DesktopRequestResult<RuntimePluginAuthSession | null>>;
  getPluginAuthSession: (name: string, sessionId: string) => Promise<DesktopRequestResult<RuntimePluginAuthSession>>;
  cancelPluginAuthSession: (name: string, sessionId: string) => Promise<DesktopRequestResult<RuntimePluginAuthSession>>;
  markPluginAdminApprovalPending: (name: string, sessionId: string) => Promise<DesktopRequestResult<RuntimePluginAuthSession>>;
  probePluginSetup: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<DesktopRequestResult<RuntimePluginSetupResponse>>;
  disconnectPluginSetup: (
    name: string,
    setupId: string,
    request: RuntimePluginSetupMutationRequest,
  ) => Promise<DesktopRequestResult<RuntimePluginSetupResponse>>;
  listModelProviders: () => Promise<ModelProvider[]>;
  listModelProfiles: () => Promise<RuntimeModelProfileSummary[]>;
  getModelPurposeRouting: () => Promise<ModelPurposeRouting>;
  saveModelPurposeRouting: (routing: ModelPurposeRouting) => Promise<ModelPurposeRouting>;
  getAgentPlanAccount: () => Promise<AgentPlanAccount>;
  saveAgentPlanAccount: (
    request: AgentPlanAccountSaveRequest,
  ) => Promise<DesktopOperationResult<AgentPlanAccount>>;
  refreshAgentPlanModels: () => Promise<DesktopOperationResult<AgentPlanAccount>>;
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
  ) => Promise<DesktopOperationResult<RuntimeModelProfileSummary>>;
  deleteModelProfile: (profileId: string) => Promise<void>;
  setDefaultModelProfile: (profileId: string) => Promise<RuntimeModelProfileSummary>;
  testModelProfile: (
    request: ModelProfileSaveRequest,
  ) => Promise<DesktopOperationResult<ModelProfileTestResult>>;
  testAgentPlanReasoningCapability: (
    request: ModelProfileSaveRequest,
  ) => Promise<DesktopOperationResult<ModelReasoningCapabilityTestResult>>;
  syncModelCapabilities: () => Promise<ModelProvider[]>;
  refreshModelCatalog: (
    request: ModelProfileSaveRequest,
  ) => Promise<DesktopOperationResult<ModelCatalogRefreshResult>>;
  getModelProfileBalances: () => Promise<ModelProfileBalancesResult>;
  pickAttachments: (defaultPath?: string) => Promise<AttachmentPickResult>;
  pickDirectory: (defaultPath?: string) => Promise<WorkspacePickResult>;
  pickPluginSource: (defaultPath?: string) => Promise<DesktopRequestResult<WorkspacePickResult>>;
  resolveDroppedPluginSource: (file: File) => Promise<DesktopRequestResult<WorkspacePickResult>>;
  pickProjectFolders: (defaultPath?: string) => Promise<ProjectFoldersPickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
  notifyRendererReady: () => void;
  listScheduledTasks: () => Promise<DesktopRequestResult<ScheduledTask[]>>;
  getScheduledTask: (taskId: string) => Promise<DesktopRequestResult<ScheduledTask>>;
  createScheduledTask: (input: ScheduledTaskCreateInput) => Promise<DesktopRequestResult<ScheduledTask>>;
  patchScheduledTask: (taskId: string, input: ScheduledTaskPatchInput) => Promise<DesktopRequestResult<ScheduledTask>>;
  deleteScheduledTask: (
    taskId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ) => Promise<DesktopRequestResult<void>>;
  pauseScheduledTask: (taskId: string, expectedVersion: number) => Promise<DesktopRequestResult<ScheduledTask>>;
  resumeScheduledTask: (taskId: string, expectedVersion: number) => Promise<DesktopRequestResult<ScheduledTask>>;
  runScheduledTaskNow: (taskId: string, idempotencyKey: string) => Promise<DesktopRequestResult<ScheduledTaskExecution>>;
  cancelScheduledTaskExecution: (taskId: string) => Promise<DesktopRequestResult<void>>;
  listScheduledTaskExecutions: (taskId: string) => Promise<DesktopRequestResult<ScheduledTaskExecution[]>>;
  getSystemTimezone: () => Promise<DesktopRequestResult<string>>;
  subscribeScheduledTaskEvents: (handler: (event: ScheduledTaskEvent) => void) => () => void;
  onScheduledTaskThreadActivation: (handler: (threadId: string) => void) => () => void;
  getDesktopUpdateState: () => Promise<DesktopUpdateSnapshot>;
  checkDesktopUpdate: () => Promise<DesktopUpdateSnapshot>;
  downloadDesktopUpdate: () => Promise<DesktopUpdateSnapshot>;
  restartAndInstallDesktopUpdate: () => Promise<DesktopUpdateSnapshot>;
  onDesktopUpdateStateChanged: (handler: (snapshot: DesktopUpdateSnapshot) => void) => () => void;
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
    fileComments: run.file_comments?.map(normalizeFileLineComment),
    status: run.status,
    result: run.result,
    error: run.error,
    cancelRequested: Boolean(run.cancel_requested),
    createdAt: run.created_at,
    updatedAt: run.updated_at,
    finishedAt: run.finished_at ?? undefined,
    modelProfileId: run.model_profile_id || undefined,
    reasoningEffort: run.reasoning_effort || undefined,
    hookSnapshotId: run.hook_snapshot_id || undefined,
		instructionSnapshot: run.instruction_snapshot
			? normalizeInstructionSnapshot(run.instruction_snapshot)
			: undefined,
  };
}

export function normalizeInstructionSnapshot(snapshot: InstructionSnapshotSummaryWire): InstructionSnapshotSummary {
	return {
		snapshotId: snapshot.snapshot_id,
		capturedAt: snapshot.captured_at,
		partial: Boolean(snapshot.partial),
		sources: (snapshot.sources ?? []).map((source) => ({
			scope: source.scope,
			displayPath: source.display_path,
			scopeRoot: source.scope_root,
			digest: source.digest,
			originalBytes: source.original_bytes,
			appliedBytes: source.applied_bytes,
			truncated: Boolean(source.truncated),
			status: source.status,
		})),
		diagnostics: (snapshot.diagnostics ?? []).map((diagnostic) => ({
			code: diagnostic.code,
			message: diagnostic.message,
			scope: diagnostic.scope,
			displayPath: diagnostic.display_path,
		})),
	};
}

export function normalizeGlobalAgentsDocument(document: GlobalAgentsDocumentWire): GlobalAgentsDocument {
	return {
		exists: Boolean(document.exists),
		revision: document.revision,
		digest: document.digest,
		content: document.content,
		byteCount: document.byte_count,
		updatedAt: document.updated_at,
		maxBytes: document.max_bytes,
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
    selectedModelProfileId: thread.selected_model_profile_id || undefined,
    reasoningEffortByProfile: thread.reasoning_effort_by_profile ?? {},
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
    archived: Boolean(thread.archived),
    archivedAt:
      thread.archived_at && thread.archived_at !== "0001-01-01T00:00:00Z"
        ? thread.archived_at
        : undefined,
    source: thread.source || undefined,
    acpSessionId: thread.acp_session_id || undefined,
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
    folders.push({
      path: project.workspace_root,
      role: "user",
      isPrimary: true,
    });
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
    activityCursor: transcript.activity_cursor ?? 0,
    thread: normalizeRuntimeThread(transcript.thread),
    runs: transcript.runs.map((entry) => ({
      run: normalizeRuntimeRun(entry.run),
      events: entry.events,
      streamCursor: entry.stream_cursor ?? 0,
    })),
  };
}

export function normalizeRewindThreadResult(
  result: RewindThreadResultWire,
): RewindThreadResult {
  return {
    thread: normalizeRuntimeThread(result.thread),
    targetRun: normalizeRuntimeRun(result.target_run),
    removedRunIds: [...result.removed_run_ids],
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
    file_comments: request.fileComments?.map(toFileLineCommentWire),
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
    model_profile_id: request.modelProfileId,
    reasoning_effort: request.reasoningEffort,
    model_profile: request.modelProfile
      ? normalizeModelProfileRunConfig(request.modelProfile)
      : undefined,
  };
}

export function normalizeModelProvider(provider: ModelProviderWire): ModelProvider {
  return {
    id: provider.id,
    displayName: provider.display_name,
    baseURL: provider.base_url,
    protocol: provider.protocol,
    supportedProtocols: provider.supported_protocols?.length
      ? [...provider.supported_protocols]
      : [provider.protocol],
    officialProtocols: provider.official_protocols
      ? [...provider.official_protocols]
      : [provider.protocol],
    protocolBaseURLs: { ...(provider.protocol_base_urls ?? {}) },
    protocolAuthSchemes: { ...(provider.protocol_auth_schemes ?? {}) },
    catalogStrategy: provider.catalog_strategy ?? "openai",
    catalogSource: provider.catalog_source ?? "official_manifest",
    catalogObservedAt: provider.catalog_observed_at,
    authScheme: provider.auth_scheme,
    keyEnvName: provider.key_env_name,
    defaultModel: provider.default_model,
    models: provider.models,
    modelDisplayNames: { ...(provider.model_display_names ?? {}) },
    docsURL: provider.docs_url,
    apiDocsURL: provider.api_docs_url,
    accessType: provider.access_type ?? (provider.coding_preset ? "coding_plan" : "api"),
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
      normalizeModelCapability(capability),
    ]),
  );
}

export function normalizeModelCapability(
  capability: ModelCapabilityWire,
): ModelCapability {
  return {
    supportedProtocols: capability.supported_protocols?.length
      ? [...capability.supported_protocols]
      : undefined,
    maxInputTokens: capability.max_input_tokens,
    contextWindow: capability.context_window,
    maxOutputTokens: capability.max_output_tokens,
    defaultMaxTokens: capability.default_max_tokens,
    defaultTemperature: capability.default_temperature,
    supportsVision: capability.supports_vision,
    supportsNativePDF: capability.supports_native_pdf,
    supportsVideo: capability.supports_video,
    supportsTools: capability.supports_tools,
    supportsPromptCache: capability.supports_prompt_cache,
    reasoningEffort: capability.reasoning_effort
      ? {
          levels: [...capability.reasoning_effort.levels],
          defaultLevel: capability.reasoning_effort.default_level,
          displayLabels: capability.reasoning_effort.display_labels
            ? { ...capability.reasoning_effort.display_labels }
            : undefined,
        }
      : undefined,
    fieldSources: capability.field_sources
      ? Object.fromEntries(
          Object.entries(capability.field_sources).map(([field, value]) => [
            field,
            value
              ? { source: value.source, observedAt: value.observed_at }
              : undefined,
          ]),
        )
      : undefined,
  };
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
    inferenceEndpoint: advanced.inference_endpoint,
    maxInputTokens: advanced.max_input_tokens,
    contextWindow: advanced.context_window,
    supportsVision: advanced.supports_vision,
    supportsNativePDF: advanced.supports_native_pdf,
    supportsVideo: advanced.supports_video,
    supportsTools: advanced.supports_tools,
    supportsPromptCache: advanced.supports_prompt_cache,
    defaultReasoningEffort: advanced.default_reasoning_effort,
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
    inference_endpoint: advanced.inferenceEndpoint,
    max_input_tokens: advanced.maxInputTokens,
    context_window: advanced.contextWindow,
    supports_vision: advanced.supportsVision,
    supports_native_pdf: advanced.supportsNativePDF,
    supports_video: advanced.supportsVideo,
    supports_tools: advanced.supportsTools,
    supports_prompt_cache: advanced.supportsPromptCache,
    default_reasoning_effort: advanced.defaultReasoningEffort,
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
    accessType: profile.access_type ?? "api",
    credentialReference: profile.credential_reference
      ? profile.credential_reference.kind === "provider_account"
        ? {
            kind: "provider_account",
            providerId: profile.credential_reference.provider_id,
            accountId: profile.credential_reference.account_id,
          }
        : {
            kind: "profile_secret",
            profileId: profile.credential_reference.profile_id,
          }
      : undefined,
    modelId: profile.model_id,
    modelDisplayName: profile.model_display_name ?? profile.model_id,
    baseURL: profile.base_url,
    protocol: profile.protocol,
    authScheme: profile.auth_scheme,
    hasKey: profile.has_key,
    maskedKey: profile.masked_key,
    hasManagementKey: profile.has_management_key,
    maskedManagementKey: profile.masked_management_key,
    enabled: profile.enabled,
    isDefault: profile.is_default,
    profileStatus: profile.profile_status,
    advanced: normalizeModelProfileAdvanced(profile.advanced),
    effectiveCapability: profile.effective_capability
      ? normalizeModelCapability(profile.effective_capability)
      : undefined,
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
    capability_model_id: profile.capabilityModelId,
    endpoint_selector: profile.endpointSelector,
    model_selector_kind: profile.modelSelectorKind,
    base_url: profile.baseURL,
    inference_endpoint: profile.inferenceEndpoint,
    protocol: profile.protocol,
    auth_scheme: profile.authScheme,
    api_key: profile.apiKey,
    temperature: profile.temperature,
    max_tokens: profile.maxTokens,
    context_window: profile.contextWindow,
    max_input_tokens: profile.maxInputTokens,
    prompt_cache_key: profile.promptCacheKey,
    credential_revision: profile.credentialRevision,
    supports_vision: profile.supportsVision,
    supports_native_pdf: profile.supportsNativePDF,
    supports_video: profile.supportsVideo,
    supports_tools: profile.supportsTools,
    supports_prompt_cache: profile.supportsPromptCache,
    reasoning_effort: profile.reasoningEffort,
    purpose_profile_sources: profile.purposeProfileSources,
    config_snapshot_revision: profile.configSnapshotRevision,
    purpose_profiles: profile.purposeProfiles
      ? Object.fromEntries(
          Object.entries(profile.purposeProfiles).map(([purpose, value]) => [
            purpose,
            value ? normalizeModelProfileRunConfig(value) : undefined,
          ]),
        )
      : undefined,
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

export function toRunInputAttachmentWire(attachment: RunInputAttachment): RunInputAttachmentWire {
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

export function normalizeFileLineComment(comment: FileLineCommentWire): FileLineComment {
  return {
    id: comment.id,
    kind: comment.kind ?? "comment",
    projectId: comment.project_id,
    folderPath: comment.folder_path,
    relativePath: comment.relative_path,
    displayName: comment.display_name,
    contentDigest: comment.content_digest,
    startLine: comment.start_line,
    endLine: comment.end_line,
    selectedText: comment.selected_text,
    comment: comment.comment,
    createdAt: comment.created_at,
    validationState: "ready",
  };
}

export function toFileLineCommentWire(comment: FileLineComment): FileLineCommentWire {
  return {
    id: comment.id,
    kind: comment.kind ?? "comment",
    project_id: comment.projectId,
    folder_path: comment.folderPath,
    relative_path: comment.relativePath,
    display_name: comment.displayName,
    content_digest: comment.contentDigest,
    start_line: comment.startLine,
    end_line: comment.endLine,
    selected_text: comment.selectedText,
    comment: comment.comment,
    created_at: comment.createdAt,
  };
}

export function normalizeModelBalanceStatus(status: ModelBalanceStatusWire): ModelBalanceStatus {
  return status;
}

export function normalizeModelBalanceAmount(amount: ModelBalanceAmountWire): ModelBalanceAmount {
  return {
    currency: amount.currency,
    total: amount.total,
    granted: amount.granted,
    toppedUp: amount.topped_up,
  };
}

export function normalizeModelBalanceQuota(quota: ModelBalanceQuotaWire): ModelBalanceQuota {
  return {
    name: quota.name,
    limit: quota.limit,
    used: quota.used,
    remaining: quota.remaining,
    unit: quota.unit,
    unlimited: quota.unlimited,
    resetAt: quota.reset_at,
  };
}

export function normalizeModelBalanceDetail(detail: ModelBalanceDetailWire): ModelBalanceDetail {
  if (detail.kind === "amount") {
    return {
      kind: "amount",
      amounts: detail.amounts.map(normalizeModelBalanceAmount),
    };
  }
  if (detail.kind === "usage") {
    return {
      kind: "usage",
      metrics: detail.metrics.map((metric) => ({ ...metric })),
      consoleURL: detail.console_url,
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
    setupURL: balance.setup_url,
    setupLabel: balance.setup_label,
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
      repeat_end: schedule.repeatEnd ? toScheduledTaskRepeatEndWire(schedule.repeatEnd) : undefined,
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
    action: {
      kind: task.action.kind,
      content: task.action.content,
      ...(task.action.composer_tokens?.length
        ? {
            composerTokens: task.action.composer_tokens.map((token) => ({
              ...token,
              payload: token.payload ? { ...token.payload } : undefined,
            })),
          }
        : {}),
    },
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

export function normalizeScheduledTaskEvent(event: ScheduledTaskEventWire): ScheduledTaskEvent {
  return {
    type: event.type,
    taskId: event.task_id,
    projectId: event.project_id || undefined,
    executionId: event.execution_id || undefined,
    runId: event.run_id || undefined,
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
    action: {
      kind: input.action.kind,
      content: input.action.content,
      ...(input.action.composerTokens?.length
        ? {
            composer_tokens: input.action.composerTokens.map((token) => ({
              ...token,
              payload: token.payload ? { ...token.payload } : undefined,
            })),
          }
        : {}),
    },
    execution: toScheduledTaskExecutionOptionsWire(input.execution),
    template_origin: input.templateOrigin
      ? toScheduledTaskTemplateOriginWire(input.templateOrigin)
      : undefined,
  };
}

export function toScheduledTaskPatchWire(input: ScheduledTaskPatchInput): ScheduledTaskPatchWire {
  return {
    expected_version: input.expectedVersion,
    name: input.name,
    enabled: input.enabled,
    project_id: input.projectId,
    schedule: input.schedule ? toScheduledTaskScheduleWire(input.schedule) : undefined,
    action: input.action
      ? {
          kind: input.action.kind,
          content: input.action.content,
          ...(input.action.composerTokens?.length
            ? {
                composer_tokens: input.action.composerTokens.map((token) => ({
                  ...token,
                  payload: token.payload ? { ...token.payload } : undefined,
                })),
              }
            : {}),
        }
      : undefined,
    execution: input.execution ? toScheduledTaskExecutionOptionsWire(input.execution) : undefined,
    template_origin: input.templateOrigin
      ? toScheduledTaskTemplateOriginWire(input.templateOrigin)
      : undefined,
  };
}
