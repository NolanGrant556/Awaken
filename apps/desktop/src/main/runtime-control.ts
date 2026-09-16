import type { OfficeEnvironmentStatus, OfficeEnvironmentOperation } from "../shared/office-environment.js";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import type { NetworkRouteSnapshot } from "./network-route-manager.js";

export interface RuntimeManifest {
  schema_version: number;
  runtime_identity: string;
  instance_id: string;
  pid: number;
  owner_mode: "desktop" | "headless";
  base_url: string;
  control_token: string;
  started_at: string;
}

export interface RuntimeLease {
  lease_id: string;
  owner_kind: string;
  owner_id: string;
  expires_at: string;
  ttl_ms: number;
  heartbeat_ms: number;
}

export class RuntimeControlClient {
  private baseUrl: string;
  private token: string;
  private leaseId?: string;
  private heartbeatTimer?: ReturnType<typeof setInterval>;

  constructor(manifest: RuntimeManifest) {
    this.baseUrl = manifest.base_url;
    this.token = manifest.control_token;
  }

  static async readManifest(userDataDir: string): Promise<RuntimeManifest | undefined> {
    try {
      const data = await readFile(join(userDataDir, "runtime", "core.json"), "utf8");
      return JSON.parse(data) as RuntimeManifest;
    } catch {
      return undefined;
    }
  }

  async verifyIdentity(expectedRuntimeIdentity: string): Promise<void> {
    const resp = await fetch(`${this.baseUrl}/v1/control/runtime/identity`, {
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!resp.ok) {
      throw new Error(`identity verification failed: ${resp.status}`);
    }
    const body = (await resp.json()) as {
      runtime_identity: string;
      instance_id: string;
      owner_mode: string;
      base_url: string;
    };
    if (body.runtime_identity !== expectedRuntimeIdentity) {
      throw new Error(
        `runtime identity mismatch: ${body.runtime_identity} != ${expectedRuntimeIdentity}`,
      );
    }
    if (body.base_url !== this.baseUrl) {
      throw new Error(`base_url mismatch: ${body.base_url} != ${this.baseUrl}`);
    }
  }

  async inspectCLIInstallation(component: string): Promise<{ executablePath: string; version: string; healthy: boolean } | null> {
    const response = await fetch(`${this.baseUrl}/v1/control/environments/components/${encodeURIComponent(component)}`, { headers: { Authorization: `Bearer ${this.token}` } });
    if (!response.ok) throw new Error("运行组件登记无法读取，请稍后重试");
    return (await response.json() as { installation: { executablePath: string; version: string; healthy: boolean } | null }).installation;
  }

  async reserveEnvironmentComponentRemoval(component: string): Promise<string> {
    const response = await fetch(`${this.baseUrl}/v1/control/environments/components/${encodeURIComponent(component)}/removal`, { method: "POST", headers: { Authorization: `Bearer ${this.token}`, "Content-Type": "application/json" }, body: JSON.stringify({ pid: process.pid }) });
    if (!response.ok) throw new Error("运行组件仍被任务或服务使用，请稍后卸载");
    return (await response.json() as { removal_token: string }).removal_token;
  }

  async releaseEnvironmentComponentRemoval(component: string, token: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/v1/control/environments/components/${encodeURIComponent(component)}/removal`, { method: "DELETE", headers: { Authorization: `Bearer ${this.token}`, "X-Awaken-Removal-Token": token } });
    if (!response.ok) throw new Error("共享组件卸载占用释放失败，请重试");
  }

  async holdEnvironmentExecutable(executable: string, removalToken?: string): Promise<string> {
    const response = await fetch(`${this.baseUrl}/v1/control/environments/host-leases`, { method: "POST", headers: { Authorization: `Bearer ${this.token}`, "Content-Type": "application/json" }, body: JSON.stringify({ executable, pid: process.pid, removal_token: removalToken }) });
    if (!response.ok) throw new Error("运行组件当前不可用或正在卸载");
    const { reference } = await response.json() as { reference: string };
    return reference;
  }

  async releaseEnvironmentExecutable(reference: string): Promise<void> {
    if (!reference) return;
    const released = await fetch(`${this.baseUrl}/v1/control/environments/host-leases/${encodeURIComponent(reference)}`, { method: "DELETE", headers: { Authorization: `Bearer ${this.token}` } });
    if (!released.ok) throw new Error("共享组件使用记录释放失败");
  }

  async removeEnvironmentComponent(component: string, removalToken?: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/v1/control/environments/components/${encodeURIComponent(component)}`, { method: "DELETE", headers: { Authorization: `Bearer ${this.token}`, ...(removalToken ? { "X-Awaken-Removal-Token": removalToken } : {}) } });
    if (!response.ok) throw new Error(`共享运行组件卸载失败 (${response.status})`);
  }

  async prepareEnvironment(capability: string, signal: AbortSignal, progress?: (message: string) => void): Promise<{ environment_ref: string; instance_id: string; executables: Record<string, { executable: string; args?: string[] }> }> {
    return this.streamEnvironment<{ environment_ref: string; instance_id: string; executables: Record<string, { executable: string; args?: string[] }> }>("prepare", { capability }, signal, (operation) => progress?.(operation.message));
  }

  async getOfficeEnvironment(): Promise<OfficeEnvironmentStatus> {
    const response = await fetch(`${this.baseUrl}/v1/control/environments/office`, {
      headers: { Authorization: `Bearer ${this.token}` }, signal: AbortSignal.timeout(30000),
    });
    if (!response.ok) throw new Error(`无法读取办公环境状态 (${response.status})`);
    return response.json() as Promise<OfficeEnvironmentStatus>;
  }

  async prepareOfficeEnvironment(action: "prepare" | "repair", expectedInstanceId: string | undefined, signal: AbortSignal, progress: (operation: OfficeEnvironmentOperation) => void): Promise<void> {
    await this.streamEnvironment<void>(`office/${action}`, action === "repair" ? { expected_instance_id: expectedInstanceId } : {}, signal, progress);
  }

  private async streamEnvironment<T>(path: string, body: object, signal: AbortSignal, progress?: (operation: OfficeEnvironmentOperation) => void): Promise<T> {
    const response = await fetch(`${this.baseUrl}/v1/control/environments/${path}`, {
      method: "POST", signal,
      headers: { Authorization: `Bearer ${this.token}`, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!response.ok || !response.body) throw new Error(`环境准备服务不可用 (${response.status})`);
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let pending = "";
    try {
      while (true) {
        const { value, done } = await reader.read();
        pending += decoder.decode(value, { stream: !done });
        if (pending.length > 1024 * 1024) throw new Error("环境准备响应过大");
        let newline: number;
        while ((newline = pending.indexOf("\n")) >= 0) {
          const line = pending.slice(0, newline); pending = pending.slice(newline + 1);
          if (!line.trim()) continue;
          const item = JSON.parse(line);
          if (item.type === "error") throw new Error(`${item.code}: ${item.message}`);
          if (item.type === "progress") progress?.(item.operation);
          if (item.type === "ready") return item.binding;
        }
        if (done) break;
      }
      throw new Error("环境准备连接中断，请重试");
    } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
  }

  async updateNetworkRoute(snapshot: NetworkRouteSnapshot): Promise<NetworkRouteSnapshot> {
    const resp = await fetch(`${this.baseUrl}/v1/control/network-route`, {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(snapshot),
    });
    if (!resp.ok) {
      throw new Error(`network route update failed: ${resp.status}`);
    }
    return (await resp.json()) as NetworkRouteSnapshot;
  }

  async reloadAgentPlanMediaTool(capability: string): Promise<void> {
    const resp = await fetch(
      `${this.baseUrl}/v1/control/agent-plan/media-tools/reload`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${this.token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ capability }),
      },
    );
    if (!resp.ok) {
      throw new Error(`Agent Plan media tool reload failed: ${resp.status}`);
    }
  }

  async createLease(ownerID: string, ownerKind: "desktop" | "bridge" = "desktop"): Promise<RuntimeLease> {
    const resp = await fetch(`${this.baseUrl}/v1/control/leases`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ owner_kind: ownerKind, owner_id: ownerID }),
    });
    if (!resp.ok) {
      throw new Error(`create lease failed: ${resp.status}`);
    }
    const lease = (await resp.json()) as RuntimeLease;
    this.leaseId = lease.lease_id;
    this.startHeartbeat(lease.heartbeat_ms);
    return lease;
  }

  async releaseLease(): Promise<void> {
    this.stopHeartbeat();
    if (!this.leaseId) {
      return;
    }
    const resp = await fetch(`${this.baseUrl}/v1/control/leases/${this.leaseId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${this.token}` },
    });
    this.leaseId = undefined;
    if (!resp.ok) {
      throw new Error(`release lease failed: ${resp.status}`);
    }
  }

  async resolveComputerReview(
    runId: string,
    reviewId: string,
    decision: "approved_once" | "denied",
  ): Promise<void> {
    const resp = await fetch(
      `${this.baseUrl}/v1/control/computer/runs/${encodeURIComponent(runId)}/reviews/${encodeURIComponent(reviewId)}/resolve`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${this.token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ decision, actor: "desktop-user" }),
      },
    );
    if (!resp.ok) {
      throw new Error(`resolve computer review failed: ${resp.status}`);
    }
  }

  async computerSessionControl(
    runId: string,
    action: "takeover" | "return" | "pause" | "terminate",
    sessionId?: string,
  ): Promise<void> {
    const resp = await fetch(
      `${this.baseUrl}/v1/control/computer/sessions/${encodeURIComponent(runId)}/${action}`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${this.token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ session_id: sessionId }),
      },
    );
    if (!resp.ok) {
      throw new Error(`computer session control ${action} failed: ${resp.status}`);
    }
  }

  async getComputerHostStatus(): Promise<{
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
  }> {
    const resp = await fetch(`${this.baseUrl}/v1/control/computer/status`, {
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!resp.ok) {
      throw new Error(`get computer host status failed: ${resp.status}`);
    }
    return (await resp.json()) as {
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
  }

  async requestComputerPermissions(): Promise<{
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
  }> {
    const resp = await fetch(
      `${this.baseUrl}/v1/control/computer/permissions/request`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${this.token}` },
      },
    );
    if (!resp.ok) {
      throw new Error(`request computer permissions failed: ${resp.status}`);
    }
    return (await resp.json()) as {
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
  }

  async getComputerSettings(): Promise<{
    revision: number;
    value: {
      enabled: boolean;
      allow_background_window_actions: boolean;
    };
  }> {
    const resp = await fetch(`${this.baseUrl}/v1/computer/settings`, {
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!resp.ok) {
      throw new Error(`get computer settings failed: ${resp.status}`);
    }
    return (await resp.json()) as {
      revision: number;
      value: {
        enabled: boolean;
        allow_background_window_actions: boolean;
      };
    };
  }

  async saveComputerSettings(request: {
    expected_revision: number;
    enabled?: boolean;
    allow_background_window_actions?: boolean;
  }): Promise<{
    revision: number;
    value: {
      enabled: boolean;
      allow_background_window_actions: boolean;
    };
  }> {
    const resp = await fetch(`${this.baseUrl}/v1/computer/settings`, {
      method: "PATCH",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });
    if (!resp.ok) {
      throw new Error(`save computer settings failed: ${resp.status}`);
    }
    return (await resp.json()) as {
      revision: number;
      value: {
        enabled: boolean;
        allow_background_window_actions: boolean;
      };
    };
  }

  private startHeartbeat(intervalMs: number): void {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(async () => {
      if (!this.leaseId) {
        return;
      }
      try {
        const resp = await fetch(
          `${this.baseUrl}/v1/control/leases/${this.leaseId}/heartbeat`,
          {
            method: "POST",
            headers: { Authorization: `Bearer ${this.token}` },
          },
        );
        if (!resp.ok) {
          this.stopHeartbeat();
        }
      } catch {
        this.stopHeartbeat();
      }
    }, intervalMs);
  }

  async registerBrowserHost(registration: {
    base_url: string;
    token: string;
  } | null): Promise<void> {
    const resp = await fetch(`${this.baseUrl}/v1/control/browser-host`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(registration ?? {}),
    });
    if (!resp.ok) {
      throw new Error(`register browser host failed: ${resp.status}`);
    }
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = undefined;
    }
  }
}
