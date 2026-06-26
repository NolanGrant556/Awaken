import type { AguiEvent } from "./agui";
import {
  type ApprovalDecision,
  type AwakenBridge,
  type DesktopPlatform,
  type RuntimeInfo,
  type RuntimeRun,
  type RuntimeThreadTranscript,
  type RuntimeThread,
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
  source: "electron";
  platform: DesktopPlatform;
  getRuntimeInfo: () => Promise<RuntimeInfo>;
  getRun: (runId: string) => Promise<RuntimeRun>;
  getThreadTranscript: (threadId: string) => Promise<RuntimeThreadTranscript>;
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
  notifyRendererReady: () => void;
};

const MAX_RECONNECT_RETRIES = 5;

function reconnectDelay(retryCount: number): number {
  return Math.min(1000 * Math.pow(2, retryCount), 10000);
}

function isTerminalEvent(event: AguiEvent): boolean {
  return event.type === "RUN_FINISHED" || event.type === "RUN_ERROR";
}

function createElectronClient(bridge: AwakenBridge): RuntimeClient {
  return {
    source: "electron",
    platform: bridge.platform,
    getRuntimeInfo: () => bridge.getRuntimeInfo(),
    getRun: (runId) => bridge.getRun(runId),
    getThreadTranscript: (threadId) => bridge.getThreadTranscript(threadId),
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
    notifyRendererReady: () => bridge.notifyRendererReady(),
  };
}

export function createRuntimeClient(): RuntimeClient {
  if (!window.awaken) {
    throw new Error("Awaken Desktop must run through the Electron window.awaken bridge");
  }
  return createElectronClient(window.awaken);
}
