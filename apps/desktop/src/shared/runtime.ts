import type { AguiEvent } from "./agui.js";

export type DesktopPlatform = "macos" | "windows" | "linux";

export type RuntimeRunStatus =
  | "running"
  | "cancelling"
  | "finished"
  | "error"
  | "cancelled";

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

export type RuntimeRunWire = {
  run_id: string;
  thread_id: string;
  input?: string;
  input_attachments?: RunInputAttachmentWire[];
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
  label: string;
  run_id?: string;
  active_run_id?: string;
  workspace: WorkspaceConfigWire;
  created_at: string;
  updated_at: string;
};

export type RuntimeThread = {
  threadId: string;
  label: string;
  runId?: string;
  activeRunId?: string;
  workspace: WorkspaceConfig;
  createdAt: string;
  updatedAt: string;
};

export type RenameThreadRequest = {
  label: string;
};

export type RuntimeThreadTranscriptRunWire = {
  run: RuntimeRunWire;
  events: AguiEvent[];
};

export type RuntimeThreadTranscriptWire = {
  thread: RuntimeThreadWire;
  runs: RuntimeThreadTranscriptRunWire[];
};

export type RuntimeThreadTranscriptRun = {
  run: RuntimeRun;
  events: AguiEvent[];
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

export type RuntimeConnector = {
  id: string;
  displayName: string;
  category: string;
  provider?: string;
  state: RuntimeConnectorState;
  sourceOrigin: string;
  authType: string;
  lastError?: string;
  serverNames?: string[];
  shadowedBy?: string;
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

export type RuntimeSkill = {
  id: string;
  name: string;
  displayName?: string;
  description: string;
  whenToUse?: string;
  state: "available" | "unavailable" | "shadowed" | string;
  unavailableReason?: string;
  sourceOrigin: string;
  context: "inline" | "fork" | string;
  shadowedBy?: string;
  requiredMcpServers?: string[];
};

export type InstallSkillRequest = {
  fileName: string;
  content?: string;
  contentBase64?: string;
};

export type InstallSkillResponse = {
  skill: RuntimeSkill;
  path: string;
  message: string;
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
  supports_video?: boolean;
};

export type ModelProfileAdvanced = {
  temperature?: number;
  maxTokens?: number;
  endpointId?: string;
  promptCacheKey?: string;
  contextWindow?: number;
  supportsVision?: boolean;
  supportsVideo?: boolean;
};

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
  profile_status: "ready" | "missing_key" | "disabled";
  advanced?: ModelProfileAdvancedWire;
};

export type RuntimeModelProfileSummary = {
  id: string;
  providerId: string;
  displayName: string;
  modelId: string;
  baseURL: string;
  protocol: string;
  authScheme: string;
  hasKey: boolean;
  maskedKey?: string;
  enabled: boolean;
  isDefault: boolean;
  profileStatus: "ready" | "missing_key" | "disabled";
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
  workspace_root: string;
  readable_roots?: string[];
  writable_roots?: string[];
  output_roots?: string[];
  attached_files?: TaskScopeFileWire[];
};

export type WorkspaceConfig = {
  workspaceRoot: string;
  readableRoots?: string[];
  writableRoots: string[];
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
  thread_id?: string;
  workspace: WorkspaceConfigWire;
  client: ClientInfoWire;
  model_profile?: ModelProfileRunConfigWire;
};

export type StartRunRequest = {
  input: string;
  attachments?: RunInputAttachment[];
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
  supports_vision?: boolean;
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
  supportsVision?: boolean;
  supportsVideo?: boolean;
};

export type WorkspacePickResult =
  | { canceled: true; error?: string }
  | { canceled: false; path: string };

export type WorkspaceValidationResult =
  | { valid: true }
  | { valid: false; error: string };

export type MassMessageReviewDecision = "approved" | "rejected";

export type WorkspaceRegistryPersistRequest = {
  defaultWorkspacePath: string;
  registeredWorkspacePaths: string[];
};

export type WorkspaceRegistrySnapshot = {
  lastWorkspacePath?: string;
  registeredWorkspacePaths: string[];
};

export type RunStreamHandlers = {
  onEvent: (event: AguiEvent) => void;
  onError?: (message: string) => void;
  onClose?: () => void;
};

export type SupervisorStatus = "stopped" | "starting" | "running" | "crashed" | "stopping";

export type SupervisorStatusInfo = {
  status: SupervisorStatus;
};

export type AwakenBridge = {
  platform: DesktopPlatform;
  getRun: (runId: string) => Promise<RuntimeRun>;
  getThreadTranscript: (threadId: string) => Promise<RuntimeThreadTranscript>;
  listThreads: () => Promise<RuntimeThread[]>;
  renameThread: (threadId: string, label: string) => Promise<RuntimeThread>;
  deleteThread: (threadId: string) => Promise<void>;
  startRun: (request: StartRunRequest) => Promise<RuntimeRun>;
  cancelRun: (runId: string) => Promise<RuntimeRun>;
  resolveMassMessageReview: (
    runId: string,
    reviewId: string,
    decision: MassMessageReviewDecision,
  ) => Promise<void>;
  subscribeRun: (
    runId: string,
    handlers: RunStreamHandlers,
    afterSeq?: number,
  ) => Promise<string>;
  unsubscribeRun: (subscriptionId: string) => void;
  getRuntimeInfo: () => Promise<RuntimeInfo>;
  listConnectors: () => Promise<RuntimeConnector[]>;
  connectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  disconnectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  listMcpServers: () => Promise<RuntimeMcpServer[]>;
  configureMcpServers: (request: ConfigureMcpServersRequest) => Promise<ConfigureMcpServersResponse>;
  getMcpServerConfig: (name: string) => Promise<RuntimeMcpServerConfigResponse>;
  updateMcpServerConfig: (name: string, request: UpdateMcpServerConfigRequest) => Promise<ConfigureMcpServersResponse>;
  disableMcpServer: (name: string) => Promise<void>;
  enableMcpServer: (name: string) => Promise<void>;
  deleteMcpServer: (name: string) => Promise<void>;
  getRuntimeLogs: () => Promise<RuntimeLogEntry[]>;
  listMcpTools: () => Promise<RuntimeToolInfo[]>;
  listSkills: () => Promise<RuntimeSkill[]>;
  installSkill: (request: InstallSkillRequest) => Promise<InstallSkillResponse>;
  listModelProviders: () => Promise<ModelProvider[]>;
  listModelProfiles: () => Promise<RuntimeModelProfileSummary[]>;
  saveModelProfile: (request: ModelProfileSaveRequest) => Promise<RuntimeModelProfileSummary>;
  deleteModelProfile: (profileId: string) => Promise<void>;
  setDefaultModelProfile: (profileId: string) => Promise<RuntimeModelProfileSummary>;
  testModelProfile: (request: ModelProfileSaveRequest) => Promise<ModelProfileTestResult>;
  syncModelCapabilities: () => Promise<ModelProvider[]>;
  getModelProfileBalances: () => Promise<ModelProfileBalancesResult>;
  pickWorkspace: (defaultPath?: string) => Promise<WorkspacePickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  readWorkspaceRegistry: () => Promise<WorkspaceRegistrySnapshot>;
  persistWorkspaceRegistry: (request: WorkspaceRegistryPersistRequest) => Promise<void>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
  notifyRendererReady: () => void;
};

export function normalizeRuntimeRun(run: RuntimeRunWire): RuntimeRun {
  return {
    runId: run.run_id,
    threadId: run.thread_id,
    input: run.input?.trim() || undefined,
    inputAttachments: run.input_attachments?.map(normalizeRunInputAttachment),
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
    label: thread.label,
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

export function normalizeRuntimeThreadTranscript(
  transcript: RuntimeThreadTranscriptWire,
): RuntimeThreadTranscript {
  return {
    thread: normalizeRuntimeThread(transcript.thread),
    runs: transcript.runs.map((entry) => ({
      run: normalizeRuntimeRun(entry.run),
      events: entry.events,
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
    thread_id: request.threadId,
    workspace: {
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
    supports_vision: profile.supportsVision,
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
  };
}

export function normalizeModelProfileBalancesResult(
  result: ModelProfileBalancesResultWire,
): ModelProfileBalancesResult {
  return {
    balances: result.balances.map(normalizeModelProfileBalance),
  };
}
