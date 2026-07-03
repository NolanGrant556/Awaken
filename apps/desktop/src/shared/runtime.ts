import type { AguiEvent } from "./agui.js";

export type DesktopPlatform = "macos" | "windows" | "linux";

export type RuntimeRunStatus =
  | "running"
  | "awaiting_approval"
  | "cancelling"
  | "finished"
  | "error"
  | "cancelled";

export type RuntimeRunWire = {
  run_id: string;
  thread_id: string;
  input?: string;
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
};

export type ModelProfileAdvanced = {
  temperature?: number;
  maxTokens?: number;
  endpointId?: string;
  promptCacheKey?: string;
  contextWindow?: number;
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

export type WorkspaceConfigWire = {
  workspace_root: string;
  writable_roots?: string[];
};

export type WorkspaceConfig = {
  workspaceRoot: string;
  writableRoots: string[];
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
  thread_id?: string;
  workspace: WorkspaceConfigWire;
  client: ClientInfoWire;
  model_profile?: ModelProfileRunConfigWire;
};

export type StartRunRequest = {
  input: string;
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
};

export type ApprovalDecision = "approved" | "rejected";

export type WorkspacePickResult =
  | { canceled: true; error?: string }
  | { canceled: false; path: string };

export type WorkspaceValidationResult =
  | { valid: true }
  | { valid: false; error: string };

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
  resolveApproval: (
    runId: string,
    approvalId: string,
    decision: ApprovalDecision,
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
  listSkills: () => Promise<RuntimeSkill[]>;
  installSkill: (request: InstallSkillRequest) => Promise<InstallSkillResponse>;
  listModelProviders: () => Promise<ModelProvider[]>;
  listModelProfiles: () => Promise<RuntimeModelProfileSummary[]>;
  saveModelProfile: (request: ModelProfileSaveRequest) => Promise<RuntimeModelProfileSummary>;
  deleteModelProfile: (profileId: string) => Promise<void>;
  setDefaultModelProfile: (profileId: string) => Promise<RuntimeModelProfileSummary>;
  testModelProfile: (request: ModelProfileSaveRequest) => Promise<ModelProfileTestResult>;
  pickWorkspace: (defaultPath?: string) => Promise<WorkspacePickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
  notifyRendererReady: () => void;
};

export function normalizeRuntimeRun(run: RuntimeRunWire): RuntimeRun {
  return {
    runId: run.run_id,
    threadId: run.thread_id,
    input: run.input?.trim() || undefined,
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
      writableRoots: thread.workspace.writable_roots ?? [],
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
    thread_id: request.threadId,
    workspace: {
      workspace_root: request.workspace.workspaceRoot,
      writable_roots: request.workspace.writableRoots,
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
  };
}
