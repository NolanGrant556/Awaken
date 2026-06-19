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
  status: RuntimeRunStatus;
  result?: string;
  error?: string;
  cancelRequested: boolean;
  createdAt: string;
  updatedAt: string;
  finishedAt?: string;
};

export type RuntimeInfoWire = {
  runtime_version: string;
  api_version: string;
  configured_model: string;
  has_api_key: boolean;
  default_workspace: string;
  base_url_host: string;
};

export type RuntimeInfo = {
  runtimeVersion: string;
  apiVersion: string;
  configuredModel: string;
  hasApiKey: boolean;
  defaultWorkspace: string;
  baseUrlHost: string;
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
  workspace: WorkspaceConfigWire;
  client: ClientInfoWire;
};

export type StartRunRequest = {
  input: string;
  workspace: WorkspaceConfig;
  client: ClientInfo;
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

export type AwakenBridge = {
  platform: DesktopPlatform;
  runtimeBaseUrl: string;
  getRun: (runId: string) => Promise<RuntimeRun>;
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
};

export function normalizeRuntimeRun(run: RuntimeRunWire): RuntimeRun {
  return {
    runId: run.run_id,
    threadId: run.thread_id,
    status: run.status,
    result: run.result,
    error: run.error,
    cancelRequested: Boolean(run.cancel_requested),
    createdAt: run.created_at,
    updatedAt: run.updated_at,
    finishedAt: run.finished_at ?? undefined,
  };
}

export function normalizeRuntimeInfo(info: RuntimeInfoWire): RuntimeInfo {
  return {
    runtimeVersion: info.runtime_version,
    apiVersion: info.api_version,
    configuredModel: info.configured_model,
    hasApiKey: info.has_api_key,
    defaultWorkspace: info.default_workspace,
    baseUrlHost: info.base_url_host,
  };
}

export function normalizeStartRunRequest(request: StartRunRequest): StartRunRequestWire {
  return {
    input: request.input,
    workspace: {
      workspace_root: request.workspace.workspaceRoot,
      writable_roots: request.workspace.writableRoots,
    },
    client: {
      session_id: request.client.sessionId,
      request_id: request.client.requestId,
      entrypoint: request.client.entrypoint,
    },
  };
}
