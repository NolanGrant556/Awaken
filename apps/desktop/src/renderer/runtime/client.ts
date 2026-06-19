import type { AguiEvent } from "./agui";
import {
  normalizeRuntimeRun,
  normalizeRuntimeInfo,
  normalizeRuntimeThread,
  normalizeStartRunRequest,
  type ApprovalDecision,
  type AwakenBridge,
  type DesktopPlatform,
  type RuntimeInfo,
  type RuntimeInfoWire,
  type RuntimeRun,
  type RuntimeRunWire,
  type RuntimeThread,
  type RuntimeThreadWire,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type WorkspacePickResult,
  type WorkspaceValidationResult,
} from "../../shared/runtime";

export type SubscribeHandlers = {
  onEvent: (event: AguiEvent) => void;
  onError?: (message: string) => void;
  onReconnect?: () => void;
  onClose?: () => void;
};

export type RuntimeClient = {
  source: "electron" | "browser";
  platform: DesktopPlatform | "web";
  getRuntimeInfo: () => Promise<RuntimeInfo>;
  getRun: (runId: string) => Promise<RuntimeRun>;
  listThreads: () => Promise<RuntimeThread[]>;
  startRun: (request: StartRunRequest) => Promise<RuntimeRun>;
  cancelRun: (runId: string) => Promise<RuntimeRun>;
  resolveApproval: (
    runId: string,
    approvalId: string,
    decision: ApprovalDecision,
  ) => Promise<void>;
  subscribeRun: (
    runId: string,
    handlers: SubscribeHandlers,
    afterSeq?: number,
  ) => Promise<() => void>;
  pickWorkspace: (defaultPath?: string) => Promise<WorkspacePickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
};

const MAX_RECONNECT_RETRIES = 5;

function reconnectDelay(retryCount: number): number {
  return Math.min(1000 * Math.pow(2, retryCount), 10000);
}

function isTerminalEvent(event: AguiEvent): boolean {
  return event.type === "RUN_FINISHED" || event.type === "RUN_ERROR";
}

function joinRuntimeUrl(baseUrl: string, pathname: string): string {
  if (!baseUrl) {
    return pathname;
  }
  return `${baseUrl.replace(/\/$/, "")}${pathname}`;
}

async function readErrorMessage(response: Response): Promise<string> {
  const body = (await response.text()).trim();
  if (!body) return `Runtime request failed (${response.status})`;
  try {
    const parsed = JSON.parse(body);
    const code = parsed.code ?? parsed.error;
    if (code && parsed.message) {
      return `${code}: ${parsed.message}`;
    }
  } catch {
    // fall through
  }
  return body;
}

async function requestRuntime(baseUrl: string, pathname: string, init?: RequestInit): Promise<RuntimeRun> {
  const response = await fetch(joinRuntimeUrl(baseUrl, pathname), init);
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  return normalizeRuntimeRun((await response.json()) as RuntimeRunWire);
}

async function requestThreads(baseUrl: string): Promise<RuntimeThread[]> {
  const response = await fetch(joinRuntimeUrl(baseUrl, "/v1/threads"));
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
  return ((await response.json()) as RuntimeThreadWire[]).map(normalizeRuntimeThread);
}

async function requestRuntimeOK(baseUrl: string, pathname: string, init?: RequestInit): Promise<void> {
  const response = await fetch(joinRuntimeUrl(baseUrl, pathname), init);
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }
}

function createElectronClient(bridge: AwakenBridge): RuntimeClient {
  return {
    source: "electron",
    platform: bridge.platform,
    getRuntimeInfo: () => bridge.getRuntimeInfo(),
    getRun: (runId) => bridge.getRun(runId),
    listThreads: () => bridge.listThreads(),
    startRun: (request) => bridge.startRun(request),
    cancelRun: bridge.cancelRun,
    resolveApproval: bridge.resolveApproval,
    subscribeRun: async (runId, handlers, afterSeq) => {
      let lastSeq = afterSeq ?? 0;
      let terminalSeen = false;
      let retryCount = 0;
      let currentSubscriptionId: string | null = null;
      let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
      let disposed = false;

      const wrappedHandlers = {
        onEvent: (event: AguiEvent) => {
          if (event.seq !== undefined && event.seq > lastSeq) {
            lastSeq = event.seq;
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
        if (retryCount >= MAX_RECONNECT_RETRIES) {
          handlers.onError?.(
            `Runtime stream failed after ${MAX_RECONNECT_RETRIES} reconnect attempts: ${reason}`,
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
          currentSubscriptionId = await bridge.subscribeRun(
            runId,
            wrappedHandlers,
            lastSeq,
          );
        } catch (error) {
          if (disposed || terminalSeen) return;
          attemptReconnect(
            error instanceof Error ? error.message : "Reconnection failed",
          );
        }
      }

      try {
        currentSubscriptionId = await bridge.subscribeRun(
          runId,
          wrappedHandlers,
          lastSeq,
        );
      } catch (error) {
        if (disposed) throw error;
        attemptReconnect(
          error instanceof Error ? error.message : "Initial subscription failed",
        );
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
    pickWorkspace: (defaultPath) => bridge.pickWorkspace(defaultPath),
    validateWorkspace: (path) => bridge.validateWorkspace(path),
    getSupervisorStatus: () => bridge.getSupervisorStatus(),
    onSupervisorStatusChange: (callback) => bridge.onSupervisorStatusChange(callback),
  };
}

function createBrowserClient(): RuntimeClient {
  const runtimeBaseUrl = import.meta.env.VITE_AWAKEN_RUNTIME_URL?.trim() || "";

  return {
    source: "browser",
    platform: "web",
    getRuntimeInfo: async () => {
      const response = await fetch(joinRuntimeUrl(runtimeBaseUrl, "/v1/runtime/info"));
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      return normalizeRuntimeInfo((await response.json()) as RuntimeInfoWire);
    },
    getRun: (runId) => requestRuntime(runtimeBaseUrl, `/v1/runs/${runId}`),
    listThreads: () => requestThreads(runtimeBaseUrl),
    startRun: (request) =>
      requestRuntime(runtimeBaseUrl, "/v1/runs", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(normalizeStartRunRequest(request)),
      }),
    cancelRun: (runId) =>
      requestRuntime(runtimeBaseUrl, `/v1/runs/${runId}/cancel`, {
        method: "POST",
      }),
    resolveApproval: (runId, approvalId, decision) =>
      requestRuntimeOK(
        runtimeBaseUrl,
        `/v1/runs/${runId}/approvals/${approvalId}/resolve`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ decision }),
        },
      ),
    subscribeRun: async (runId, handlers, afterSeq) => {
      let lastSeq = afterSeq ?? 0;
      let terminalSeen = false;
      let retryCount = 0;
      let disposed = false;
      let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
      let source: EventSource | null = null;

      function buildEventsUrl(): string {
        const base = joinRuntimeUrl(runtimeBaseUrl, `/v1/runs/${runId}/events`);
        return lastSeq > 0 ? `${base}?after_seq=${lastSeq}` : base;
      }

      function closeSource(): void {
        if (source) {
          source.onmessage = null;
          source.onerror = null;
          source.close();
          source = null;
        }
      }

      function attemptReconnect(reason: string): void {
        if (disposed || terminalSeen) return;
        // Reentrancy guard: if a reconnect is already pending, ignore
        // additional onError/onClose triggers that fire in the same window.
        if (reconnectTimer) return;
        closeSource();

        if (retryCount >= MAX_RECONNECT_RETRIES) {
          handlers.onError?.(
            `Runtime stream failed after ${MAX_RECONNECT_RETRIES} reconnect attempts: ${reason}`,
          );
          return;
        }

        handlers.onReconnect?.();
        const delay = reconnectDelay(retryCount);
        retryCount += 1;

        reconnectTimer = setTimeout(() => {
          reconnectTimer = null;
          if (disposed || terminalSeen) return;
          connect();
        }, delay);
      }

      function connect(): void {
        if (disposed || terminalSeen) return;

        source = new EventSource(buildEventsUrl());

        source.onmessage = (event) => {
          const parsed = JSON.parse(event.data) as AguiEvent;
          if (parsed.seq !== undefined && parsed.seq > lastSeq) {
            lastSeq = parsed.seq;
          }
          if (isTerminalEvent(parsed)) {
            terminalSeen = true;
          }
          handlers.onEvent(parsed);
          if (terminalSeen) {
            closeSource();
            handlers.onClose?.();
          }
        };

        source.onerror = () => {
          if (disposed || terminalSeen) return;
          attemptReconnect("Runtime stream connection failed");
        };
      }

      connect();

      return () => {
        disposed = true;
        if (reconnectTimer) {
          clearTimeout(reconnectTimer);
          reconnectTimer = null;
        }
        closeSource();
      };
    },
    pickWorkspace: async () => ({
      canceled: true as const,
      error: "Browser fallback does not support directory picker",
    }),
    validateWorkspace: async (path) => {
      const trimmed = path.trim();
      if (trimmed === "") {
        return { valid: false as const, error: "Workspace path is required" };
      }
      if (!/^([A-Za-z]:[\\/]|\/)/.test(trimmed)) {
        return { valid: false as const, error: "Workspace path must be absolute" };
      }
      return { valid: true };
    },
    getSupervisorStatus: async () => ({ status: "running" }),
    onSupervisorStatusChange: () => () => {},
  };
}

export function createRuntimeClient(): RuntimeClient {
  if (window.awaken) {
    return createElectronClient(window.awaken);
  }
  return createBrowserClient();
}
