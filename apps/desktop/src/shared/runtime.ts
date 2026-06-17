import type { AguiEvent } from "./agui.js";

export type DesktopPlatform = "macos" | "windows" | "linux";

export type RuntimeRunStatus =
  | "running"
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

export type ApprovalDecision = "approved" | "rejected";

export type RunStreamHandlers = {
  onEvent: (event: AguiEvent) => void;
  onError?: (message: string) => void;
  onClose?: () => void;
};

export type AwakenBridge = {
  platform: DesktopPlatform;
  runtimeBaseUrl: string;
  createRun: (input: string) => Promise<RuntimeRun>;
  cancelRun: (runId: string) => Promise<RuntimeRun>;
  resolveApproval: (
    runId: string,
    approvalId: string,
    decision: ApprovalDecision,
  ) => Promise<void>;
  subscribeRun: (runId: string, handlers: RunStreamHandlers) => Promise<string>;
  unsubscribeRun: (subscriptionId: string) => void;
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
