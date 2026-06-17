import type { AguiEvent } from "./agui";
import {
  normalizeRuntimeRun,
  type ApprovalDecision,
  type AwakenBridge,
  type DesktopPlatform,
  type RuntimeRun,
  type RuntimeRunWire,
} from "../../shared/runtime";

export type RuntimeClient = {
  source: "electron" | "browser";
  platform: DesktopPlatform | "web";
  runtimeBaseUrl: string;
  createRun: (input: string) => Promise<RuntimeRun>;
  cancelRun: (runId: string) => Promise<RuntimeRun>;
  resolveApproval: (
    runId: string,
    approvalId: string,
    decision: ApprovalDecision,
  ) => Promise<void>;
  subscribeRun: (
    runId: string,
    handlers: {
      onEvent: (event: AguiEvent) => void;
      onError?: (message: string) => void;
      onClose?: () => void;
    },
  ) => Promise<() => void>;
};

function joinRuntimeUrl(baseUrl: string, pathname: string): string {
  if (!baseUrl) {
    return pathname;
  }
  return `${baseUrl.replace(/\/$/, "")}${pathname}`;
}

async function readErrorMessage(response: Response): Promise<string> {
  const body = (await response.text()).trim();
  return body || `Runtime request failed (${response.status})`;
}

async function requestRuntime(baseUrl: string, pathname: string, init?: RequestInit): Promise<RuntimeRun> {
  const response = await fetch(joinRuntimeUrl(baseUrl, pathname), init);
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  return normalizeRuntimeRun((await response.json()) as RuntimeRunWire);
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
    runtimeBaseUrl: bridge.runtimeBaseUrl,
    createRun: bridge.createRun,
    cancelRun: bridge.cancelRun,
    resolveApproval: bridge.resolveApproval,
    subscribeRun: async (runId, handlers) => {
      const subscriptionId = await bridge.subscribeRun(runId, handlers);
      return () => bridge.unsubscribeRun(subscriptionId);
    },
  };
}

function createBrowserClient(): RuntimeClient {
  const runtimeBaseUrl = import.meta.env.VITE_AWAKEN_RUNTIME_URL?.trim() || "";

  return {
    source: "browser",
    platform: "web",
    runtimeBaseUrl: runtimeBaseUrl || window.location.origin,
    createRun: (input) =>
      requestRuntime(runtimeBaseUrl, "/v1/runs", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ input }),
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
    subscribeRun: async (runId, handlers) => {
      const source = new EventSource(joinRuntimeUrl(runtimeBaseUrl, `/v1/runs/${runId}/events`));
      let terminalSeen = false;

      source.onmessage = (event) => {
        const parsed = JSON.parse(event.data) as AguiEvent;
        if (parsed.type === "RUN_FINISHED" || parsed.type === "RUN_ERROR") {
          terminalSeen = true;
        }
        handlers.onEvent(parsed);
        if (terminalSeen) {
          source.close();
          handlers.onClose?.();
        }
      };
      source.onerror = () => {
        source.close();
        if (!terminalSeen) {
          handlers.onError?.("Runtime stream connection failed");
        }
        handlers.onClose?.();
      };

      return () => {
        source.close();
      };
    },
  };
}

export function createRuntimeClient(): RuntimeClient {
  if (window.awaken) {
    return createElectronClient(window.awaken);
  }
  return createBrowserClient();
}
