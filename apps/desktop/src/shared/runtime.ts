import type { AguiEvent } from "./agui.js";

export type DesktopPlatform = "macos" | "windows" | "linux";

export type RuntimeRunStatus =
  | "queued"
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
  delegation?: DelegationMetadataWire;
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
  delegation?: DelegationMetadata;
  status: RuntimeRunStatus;
  result?: string;
  error?: string;
  cancelRequested: boolean;
  createdAt: string;
  updatedAt: string;
  finishedAt?: string;
};

export type RuntimeThreadState = "idle" | "running" | "requires_action";

export type RuntimePendingActionWire = {
  ToolName?: string;
  ActionDescription?: string;
  ToolUseID?: string;
  RequestID?: string;
  Input?: Record<string, unknown>;
};

export type RuntimePendingAction = {
  toolName?: string;
  actionDescription?: string;
  toolUseId?: string;
  requestId?: string;
  input?: Record<string, unknown>;
};

export type RuntimeThreadBranchMetadataWire = {
  original_thread_id?: string;
  original_session_id?: string;
  forked_from_message_uuid?: string;
  title?: string;
};

export type RuntimeThreadBranchMetadata = {
  originalThreadId?: string;
  originalSessionId?: string;
  forkedFromMessageUuid?: string;
  title?: string;
};

export type RuntimeThreadMetadataWire = {
  session_id?: string;
  custom_title?: string;
  ai_title?: string;
  display_title?: string;
  tag?: string;
  last_prompt?: string;
  task_summary?: string;
  task_summary_timestamp?: string;
  mode?: string;
  branch?: RuntimeThreadBranchMetadataWire;
};

export type RuntimeThreadMetadata = {
  sessionId?: string;
  customTitle?: string;
  aiTitle?: string;
  displayTitle?: string;
  tag?: string;
  lastPrompt?: string;
  taskSummary?: string;
  taskSummaryTimestamp?: string;
  mode?: string;
  branch?: RuntimeThreadBranchMetadata;
};

export type RuntimeThreadWire = {
  thread_id: string;
  label: string;
  state?: RuntimeThreadState;
  pending_action?: RuntimePendingActionWire;
  metadata?: RuntimeThreadMetadataWire;
  run_id?: string;
  active_run_id?: string;
  delegation?: DelegationMetadataWire;
  latest_run?: RuntimeThreadRunSummaryWire;
  workspace: WorkspaceConfigWire;
  created_at: string;
  updated_at: string;
};

export type RuntimeThreadRunSummaryWire = {
  run_id: string;
  status: RuntimeRunStatus;
  error?: string;
  updated_at: string;
  finished_at?: string | null;
};

export type RuntimeThreadRunSummary = {
  runId: string;
  status: RuntimeRunStatus;
  error?: string;
  updatedAt: string;
  finishedAt?: string;
};

export type RuntimeThread = {
  threadId: string;
  label: string;
  state?: RuntimeThreadState;
  pendingAction?: RuntimePendingAction;
  metadata?: RuntimeThreadMetadata;
  runId?: string;
  activeRunId?: string;
  delegation?: DelegationMetadata;
  latestRun?: RuntimeThreadRunSummary;
  workspace: WorkspaceConfig;
  createdAt: string;
  updatedAt: string;
};

export type RuntimeThreadLiteWire = {
  thread_id: string;
  label: string;
  state: RuntimeThreadState;
  head: string;
  tail: string;
  size: number;
};

export type RuntimeThreadLite = {
  threadId: string;
  label: string;
  state: RuntimeThreadState;
  head: string;
  tail: string;
  size: number;
};

export type RuntimeContextFileWire = {
  path: string;
  source?: string;
  tokens?: number;
  last_read_time?: string;
};

export type RuntimeThreadContextWire = {
  thread_id: string;
  files: {
    markdown: string;
    files: RuntimeContextFileWire[];
  };
  usage: {
    markdown: string;
    model: string;
    model_display_name?: string;
    total_tokens: number;
    max_tokens: number;
    percentage: number;
    message_count: number;
  };
};

export type RuntimeThreadContext = {
  threadId: string;
  files: {
    markdown: string;
    files: RuntimeContextFileWire[];
  };
  usage: {
    markdown: string;
    model: string;
    modelDisplayName?: string;
    totalTokens: number;
    maxTokens: number;
    percentage: number;
    messageCount: number;
  };
};

export type BranchThreadRequestWire = {
  fork_session_id?: string;
  custom_title?: string;
};

export type BranchThreadRequest = {
  forkSessionId?: string;
  customTitle?: string;
};

export type BranchThreadResultWire = {
  thread: RuntimeThreadWire;
  fork: unknown;
};

export type BranchThreadResult = {
  thread: RuntimeThread;
  fork: unknown;
};

export type RuntimeSessionInsightsReport = Record<string, unknown>;

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

export type RuntimeThreadSearchResultWire = {
  thread: RuntimeThreadWire;
  score: number;
  match: string;
  message_index?: number;
  snippet?: string;
};

export type RuntimeThreadSearchResult = {
  thread: RuntimeThread;
  score: number;
  match: string;
  messageIndex?: number;
  snippet?: string;
};

export type RuntimeThreadExportFormat = "plain_text" | "jsonl";

export type RuntimeThreadExportResultWire = {
  thread: RuntimeThreadWire;
  format: RuntimeThreadExportFormat;
  filename: string;
  content_type: string;
  content: string;
};

export type RuntimeThreadExportResult = {
  thread: RuntimeThread;
  format: RuntimeThreadExportFormat;
  filename: string;
  contentType: string;
  content: string;
};

export type RuntimeThreadShareVisibility = "private" | "org" | "public";
export type RuntimeThreadShareAction = "copy" | "export" | "share";

export type RuntimeThreadShareBundleRequestWire = {
  visibility?: RuntimeThreadShareVisibility;
  action?: RuntimeThreadShareAction;
  include_tool_outputs?: boolean;
  redact_secrets?: boolean;
  redact_paths?: boolean;
  max_tool_output_chars?: number;
  summary_chars?: number;
};

export type RuntimeThreadShareBundleRequest = {
  visibility?: RuntimeThreadShareVisibility;
  action?: RuntimeThreadShareAction;
  includeToolOutputs?: boolean;
  redactSecrets?: boolean;
  redactPaths?: boolean;
  maxToolOutputChars?: number;
  summaryChars?: number;
};

export type RuntimeThreadShareBundle = Record<string, unknown>;

export type RuntimeThreadShareBundleResultWire = {
  thread: RuntimeThreadWire;
  bundle: RuntimeThreadShareBundle;
};

export type RuntimeThreadShareBundleResult = {
  thread: RuntimeThread;
  bundle: RuntimeThreadShareBundle;
};

export type RuntimeThreadClearPlan = Record<string, unknown>;

export type RuntimeThreadClearPlanResultWire = {
  thread: RuntimeThreadWire;
  plan: RuntimeThreadClearPlan;
};

export type RuntimeThreadClearPlanResult = {
  thread: RuntimeThread;
  plan: RuntimeThreadClearPlan;
};

export type RuntimeInfoWire = {
  runtime_version: string;
  api_version: string;
  configured_model: string;
  configured_model_display_name?: string;
  protocol: string;
  auth_scheme: string;
  has_api_key: boolean;
  default_workspace: string;
  base_url_host: string;
  active_profile: string;
  profiles: RuntimeProfileWire[];
  slash_commands?: SlashCommandInfoWire[];
};

export type RuntimeInfo = {
  runtimeVersion: string;
  apiVersion: string;
  configuredModel: string;
  configuredModelDisplayName?: string;
  protocol: string;
  authScheme: string;
  hasApiKey: boolean;
  defaultWorkspace: string;
  baseUrlHost: string;
  activeProfile: string;
  profiles: RuntimeProfile[];
  slashCommands: SlashCommandInfo[];
};

export type RuntimeProfileWire = {
  name: string;
  model: string;
  model_display_name?: string;
  protocol: string;
  auth_scheme: string;
  has_api_key: boolean;
  base_url_host: string;
  is_active: boolean;
};

export type RuntimeProfile = {
  name: string;
  model: string;
  modelDisplayName?: string;
  protocol: string;
  authScheme: string;
  hasApiKey: boolean;
  baseUrlHost: string;
  isActive: boolean;
};

export type SlashCommandInfoWire = {
  name: string;
  description: string;
  argument_hint?: string;
};

export type SlashCommandInfo = {
  name: string;
  description: string;
  argumentHint?: string;
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

export type RunProfileWire = {
  id?: string;
  name?: string;
  description?: string;
  system_prompt?: string;
  model?: string;
  max_turns?: number;
  tools?: string[];
  disallowed_tools?: string[];
  permission_mode?: string;
};

export type RunProfile = {
  id?: string;
  name?: string;
  description?: string;
  systemPrompt?: string;
  model?: string;
  maxTurns?: number;
  tools?: string[];
  disallowedTools?: string[];
  permissionMode?: string;
};

export type DelegationMetadataWire = {
  parent_run_id?: string;
  parent_thread_id?: string;
  delegate_label?: string;
  delegate_role?: string;
  delegation_prompt?: string;
};

export type DelegationMetadata = {
  parentRunId?: string;
  parentThreadId?: string;
  delegateLabel?: string;
  delegateRole?: string;
  delegationPrompt?: string;
};

export type BusyInputMode = "queue" | "steer" | "interrupt";
export type BackgroundNotificationMode = "all" | "result" | "error" | "off";
export type ImageInputMode = "auto" | "native" | "text";

export type RunPolicyOptionsWire = {
  busy_input_mode?: BusyInputMode;
  background_notification_mode?: BackgroundNotificationMode;
  image_input_mode?: ImageInputMode;
};

export type RunPolicyOptions = {
  busyInputMode?: BusyInputMode;
  backgroundNotificationMode?: BackgroundNotificationMode;
  imageInputMode?: ImageInputMode;
  restartDrainTimeoutMs?: number;
};

export type RunAttachmentWire = {
  path: string;
  type?: "image";
};

export type RunAttachment = {
  path: string;
  type?: "image";
};

export type StartRunRequestWire = {
  input: string;
  thread_id?: string;
  delegation?: DelegationMetadataWire;
  workspace: WorkspaceConfigWire;
  client: ClientInfoWire;
  profile?: RunProfileWire;
  policy_options?: RunPolicyOptionsWire;
  attachments?: RunAttachmentWire[];
};

export type StartRunRequest = {
  input: string;
  threadId?: string;
  delegation?: DelegationMetadata;
  workspace: WorkspaceConfig;
  client: ClientInfo;
  profile?: RunProfile;
  policyOptions?: RunPolicyOptions;
  attachments?: RunAttachment[];
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

export type SupervisorStatus = "stopped" | "starting" | "running" | "crashed" | "stopping" | "draining";

export type SupervisorStatusInfo = {
  status: SupervisorStatus;
  errorMessage?: string;
  drain?: {
    activeRuns: number;
    queuedRuns: number;
    timedOut: boolean;
    timeoutMs: number;
  };
};

export type AwakenBridge = {
  platform: DesktopPlatform;
  getRun: (runId: string) => Promise<RuntimeRun>;
  getThreadLite: (threadId: string) => Promise<RuntimeThreadLite>;
  getThreadContext: (threadId: string) => Promise<RuntimeThreadContext>;
  getThreadTranscript: (threadId: string) => Promise<RuntimeThreadTranscript>;
  searchThreads: (query: string, limit?: number) => Promise<RuntimeThreadSearchResult[]>;
  exportThread: (
    threadId: string,
    format?: RuntimeThreadExportFormat,
  ) => Promise<RuntimeThreadExportResult>;
  buildThreadShareBundle: (
    threadId: string,
    request?: RuntimeThreadShareBundleRequest,
  ) => Promise<RuntimeThreadShareBundleResult>;
  buildThreadClearPlan: (threadId: string) => Promise<RuntimeThreadClearPlanResult>;
  getSessionInsights: () => Promise<RuntimeSessionInsightsReport>;
  listThreads: () => Promise<RuntimeThread[]>;
  branchThread: (threadId: string, request?: BranchThreadRequest) => Promise<BranchThreadResult>;
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
  pickWorkspace: (defaultPath?: string) => Promise<WorkspacePickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
  setPolicySettings: (settings: RunPolicyOptions) => Promise<void>;
  notifyRendererReady: () => void;
};

export function normalizeRuntimeRun(run: RuntimeRunWire): RuntimeRun {
  return {
    runId: run.run_id,
    threadId: run.thread_id,
    input: run.input?.trim() || undefined,
    delegation: normalizeDelegationMetadata(run.delegation),
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
    state: thread.state ?? "idle",
    pendingAction: normalizePendingAction(thread.pending_action),
    metadata: normalizeThreadMetadata(thread.metadata),
    runId: thread.run_id || undefined,
    activeRunId: thread.active_run_id || undefined,
    delegation: normalizeDelegationMetadata(thread.delegation),
    latestRun: thread.latest_run
      ? {
          runId: thread.latest_run.run_id,
          status: thread.latest_run.status,
          error: thread.latest_run.error,
          updatedAt: thread.latest_run.updated_at,
          finishedAt: thread.latest_run.finished_at ?? undefined,
        }
      : undefined,
    workspace: {
      workspaceRoot: thread.workspace.workspace_root,
      writableRoots: thread.workspace.writable_roots ?? [],
    },
    createdAt: thread.created_at,
    updatedAt: thread.updated_at,
  };
}

export function normalizeThreadMetadata(
  metadata?: RuntimeThreadMetadataWire,
): RuntimeThreadMetadata {
  if (!metadata) {
    return {};
  }
  return {
    sessionId: metadata.session_id || undefined,
    customTitle: metadata.custom_title || undefined,
    aiTitle: metadata.ai_title || undefined,
    displayTitle: metadata.display_title || undefined,
    tag: metadata.tag || undefined,
    lastPrompt: metadata.last_prompt || undefined,
    taskSummary: metadata.task_summary || undefined,
    taskSummaryTimestamp: metadata.task_summary_timestamp || undefined,
    mode: metadata.mode || undefined,
    branch: metadata.branch
      ? {
          originalThreadId: metadata.branch.original_thread_id || undefined,
          originalSessionId: metadata.branch.original_session_id || undefined,
          forkedFromMessageUuid: metadata.branch.forked_from_message_uuid || undefined,
          title: metadata.branch.title || undefined,
        }
      : undefined,
  };
}

export function normalizePendingAction(
  action?: RuntimePendingActionWire,
): RuntimePendingAction | undefined {
  if (!action) {
    return undefined;
  }
  return {
    toolName: action.ToolName || undefined,
    actionDescription: action.ActionDescription || undefined,
    toolUseId: action.ToolUseID || undefined,
    requestId: action.RequestID || undefined,
    input: action.Input,
  };
}

export function normalizeRuntimeThreadLite(lite: RuntimeThreadLiteWire): RuntimeThreadLite {
  return {
    threadId: lite.thread_id,
    label: lite.label,
    state: lite.state,
    head: lite.head,
    tail: lite.tail,
    size: lite.size,
  };
}

export function normalizeRuntimeThreadContext(
  context: RuntimeThreadContextWire,
): RuntimeThreadContext {
  return {
    threadId: context.thread_id,
    files: {
      markdown: context.files.markdown,
      files: context.files.files ?? [],
    },
    usage: {
      markdown: context.usage.markdown,
      model: context.usage.model,
      modelDisplayName: context.usage.model_display_name || undefined,
      totalTokens: context.usage.total_tokens,
      maxTokens: context.usage.max_tokens,
      percentage: context.usage.percentage,
      messageCount: context.usage.message_count,
    },
  };
}

export function normalizeBranchThreadResult(
  result: BranchThreadResultWire,
): BranchThreadResult {
  return {
    thread: normalizeRuntimeThread(result.thread),
    fork: result.fork,
  };
}

export function normalizeBranchThreadRequest(
  request?: BranchThreadRequest,
): BranchThreadRequestWire {
  return {
    fork_session_id: request?.forkSessionId,
    custom_title: request?.customTitle,
  };
}

export function normalizeDelegationMetadata(
  delegation?: DelegationMetadataWire,
): DelegationMetadata | undefined {
  if (!delegation) {
    return undefined;
  }

  return {
    parentRunId: delegation.parent_run_id || undefined,
    parentThreadId: delegation.parent_thread_id || undefined,
    delegateLabel: delegation.delegate_label || undefined,
    delegateRole: delegation.delegate_role || undefined,
    delegationPrompt: delegation.delegation_prompt || undefined,
  };
}

function toDelegationMetadataWire(
  delegation?: DelegationMetadata,
): DelegationMetadataWire | undefined {
  if (!delegation) {
    return undefined;
  }

  return {
    parent_run_id: delegation.parentRunId,
    parent_thread_id: delegation.parentThreadId,
    delegate_label: delegation.delegateLabel,
    delegate_role: delegation.delegateRole,
    delegation_prompt: delegation.delegationPrompt,
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

export function normalizeRuntimeThreadSearchResult(
  result: RuntimeThreadSearchResultWire,
): RuntimeThreadSearchResult {
  return {
    thread: normalizeRuntimeThread(result.thread),
    score: result.score,
    match: result.match,
    messageIndex: result.message_index,
    snippet: result.snippet,
  };
}

export function normalizeRuntimeThreadExportResult(
  result: RuntimeThreadExportResultWire,
): RuntimeThreadExportResult {
  return {
    thread: normalizeRuntimeThread(result.thread),
    format: result.format,
    filename: result.filename,
    contentType: result.content_type,
    content: result.content,
  };
}

export function normalizeRuntimeThreadShareBundleRequest(
  request?: RuntimeThreadShareBundleRequest,
): RuntimeThreadShareBundleRequestWire {
  return {
    visibility: request?.visibility,
    action: request?.action,
    include_tool_outputs: request?.includeToolOutputs,
    redact_secrets: request?.redactSecrets,
    redact_paths: request?.redactPaths,
    max_tool_output_chars: request?.maxToolOutputChars,
    summary_chars: request?.summaryChars,
  };
}

export function normalizeRuntimeThreadShareBundleResult(
  result: RuntimeThreadShareBundleResultWire,
): RuntimeThreadShareBundleResult {
  return {
    thread: normalizeRuntimeThread(result.thread),
    bundle: result.bundle,
  };
}

export function normalizeRuntimeThreadClearPlanResult(
  result: RuntimeThreadClearPlanResultWire,
): RuntimeThreadClearPlanResult {
  return {
    thread: normalizeRuntimeThread(result.thread),
    plan: result.plan,
  };
}

export function normalizeRuntimeInfo(info: RuntimeInfoWire): RuntimeInfo {
  return {
    runtimeVersion: info.runtime_version,
    apiVersion: info.api_version,
    configuredModel: info.configured_model,
    configuredModelDisplayName: info.configured_model_display_name || undefined,
    protocol: info.protocol,
    authScheme: info.auth_scheme,
    hasApiKey: info.has_api_key,
    defaultWorkspace: info.default_workspace,
    baseUrlHost: info.base_url_host,
    activeProfile: info.active_profile,
    profiles: (info.profiles ?? []).map((profile) => ({
      name: profile.name,
      model: profile.model,
      modelDisplayName: profile.model_display_name || undefined,
      protocol: profile.protocol,
      authScheme: profile.auth_scheme,
      hasApiKey: profile.has_api_key,
      baseUrlHost: profile.base_url_host,
      isActive: profile.is_active,
    })),
    slashCommands: (info.slash_commands ?? []).map((command) => ({
      name: command.name,
      description: command.description,
      argumentHint: command.argument_hint,
    })),
  };
}

export function normalizeRunProfile(profile?: RunProfile): RunProfileWire | undefined {
  if (!profile) {
    return undefined;
  }

  const wire: RunProfileWire = {
    id: profile.id,
    name: profile.name,
    description: profile.description,
    system_prompt: profile.systemPrompt,
    model: profile.model,
    max_turns: profile.maxTurns,
    tools: profile.tools,
    disallowed_tools: profile.disallowedTools,
    permission_mode: profile.permissionMode,
  };

  return Object.values(wire).some((value) => value !== undefined) ? wire : undefined;
}

export function normalizeStartRunRequest(request: StartRunRequest): StartRunRequestWire {
  return {
    input: request.input,
    thread_id: request.threadId,
    delegation: toDelegationMetadataWire(request.delegation),
    workspace: {
      workspace_root: request.workspace.workspaceRoot,
      writable_roots: request.workspace.writableRoots,
    },
    client: {
      session_id: request.client.sessionId,
      request_id: request.client.requestId,
      entrypoint: request.client.entrypoint,
    },
    profile: normalizeRunProfile(request.profile),
    policy_options: normalizeRunPolicyOptions(request.policyOptions),
    attachments: request.attachments?.map((attachment) => ({
      path: attachment.path,
      type: attachment.type,
    })),
  };
}

export function normalizeRunPolicyOptions(
  options?: RunPolicyOptions,
): RunPolicyOptionsWire | undefined {
  if (!options) {
    return undefined;
  }
  const wire: RunPolicyOptionsWire = {
    busy_input_mode: options.busyInputMode,
    background_notification_mode: options.backgroundNotificationMode,
    image_input_mode: options.imageInputMode,
  };
  return Object.values(wire).some((value) => value !== undefined) ? wire : undefined;
}
