import {
  dialog,
  shell
} from "electron";
import {
  type CommitPluginRequest,
  type InstallPluginRequest,
  type InstallPluginResponse,
  type RuntimePlugin,
  type RuntimePluginAuthSession,
  type RuntimePluginComponentDetail,
  type RuntimePluginComponentKind,
  type RuntimePluginComponentSourceResponse,
  type RuntimePluginComponentState,
  type RuntimePluginComponentStateResponse,
  type RuntimePluginConfigurationResponse,
  type RuntimePluginHookReceipt,
  type RuntimePluginHookReceiptResponse,
  type RuntimePluginSetupMutationRequest,
  type RuntimePluginSetupResponse,
  type RuntimePluginSource,
  type RuntimePluginValidation,
  type UpdatePluginComponentSourceRequest,
  type UpdatePluginConfigurationRequest
} from "../shared/runtime.js";
import {
  sanitizedPluginAuthSession,
  validatePluginAuthBrowserUrl,
  type CorePluginAuthStartResult,
} from "./plugin-auth-browser.js";
import {
  pickPluginSource,
  resolveDroppedPluginSourcePath,
} from "./plugin-source-picker.js";
import type { RuntimeRequester } from "./runtime-http.js";
import { registerRuntimeRequest } from "./runtime-request.js";

type Dependencies = {
  readonly requestRuntime: RuntimeRequester["request"];

};

export function registerPluginsIpc(dependencies: Dependencies): void {
  registerRuntimeRequest("awaken:list-plugins", async () => {
    return dependencies.requestRuntime<RuntimePlugin[]>("/v1/plugins", { method: "GET" });
  });

  registerRuntimeRequest("awaken:get-plugin", async (_event, name: string) => {
    const trimmed = String(name ?? "").trim();
    if (!trimmed) {
      throw new Error("Plugin name is required");
    }
    return dependencies.requestRuntime<RuntimePlugin>(`/v1/plugins/${encodeURIComponent(trimmed)}`, {
      method: "GET",
    });
  });

  registerRuntimeRequest("awaken:validate-plugin", async (_event, source: RuntimePluginSource) => {
    return dependencies.requestRuntime<RuntimePluginValidation>("/v1/plugins/validate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ source }),
    });
  });

  registerRuntimeRequest("awaken:install-plugin", async (_event, request: InstallPluginRequest) => {
    return dependencies.requestRuntime<InstallPluginResponse>("/v1/plugins/install", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    });
  });

  registerRuntimeRequest(
    "awaken:commit-plugin",
    async (_event, name: string, request: CommitPluginRequest) => {
      const trimmed = String(name ?? "").trim();
      if (!trimmed || !request?.expectedDigest?.trim()) {
        throw new Error("Plugin name and expected digest are required");
      }
      return dependencies.requestRuntime<InstallPluginResponse>(
        `/v1/plugins/${encodeURIComponent(trimmed)}/commit`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(request),
        },
      );
    },
  );

  registerRuntimeRequest("awaken:enable-plugin", async (_event, name: string) => {
    return dependencies.requestRuntime<RuntimePlugin>(`/v1/plugins/${encodeURIComponent(name)}/enable`, {
      method: "POST",
    });
  });

  registerRuntimeRequest("awaken:disable-plugin", async (_event, name: string) => {
    return dependencies.requestRuntime<RuntimePlugin>(`/v1/plugins/${encodeURIComponent(name)}/disable`, {
      method: "POST",
    });
  });

  registerRuntimeRequest("awaken:uninstall-plugin", async (_event, name: string) => {
    return dependencies.requestRuntime<{ trashedPath?: string; message: string; }>(
      `/v1/plugins/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    );
  });

  registerRuntimeRequest(
    "awaken:enable-plugin-server",
    async (_event, name: string, shortName: string) => {
      return dependencies.requestRuntime<RuntimePlugin>(
        `/v1/plugins/${encodeURIComponent(name)}/servers/${encodeURIComponent(shortName)}/enable`,
        { method: "POST" },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:disable-plugin-server",
    async (_event, name: string, shortName: string) => {
      return dependencies.requestRuntime<RuntimePlugin>(
        `/v1/plugins/${encodeURIComponent(name)}/servers/${encodeURIComponent(shortName)}/disable`,
        { method: "POST" },
      );
    },
  );

  registerRuntimeRequest("awaken:list-plugin-components", async (_event, name: string) => {
    const trimmed = String(name ?? "").trim();
    if (!trimmed) {
      throw new Error("Plugin name is required");
    }
    return dependencies.requestRuntime<RuntimePluginComponentState[]>(
      `/v1/plugins/${encodeURIComponent(trimmed)}/components`,
      { method: "GET" },
    );
  });

  registerRuntimeRequest(
    "awaken:get-plugin-component",
    async (_event, name: string, kind: RuntimePluginComponentKind, logicalIdEncoded: string) => {
      return dependencies.requestRuntime<RuntimePluginComponentDetail>(
        `/v1/plugins/${encodeURIComponent(name)}/components/${encodeURIComponent(kind)}/${encodeURIComponent(logicalIdEncoded)}`,
        { method: "GET" },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:update-plugin-component-source",
    async (
      _event,
      name: string,
      kind: RuntimePluginComponentKind,
      logicalIdEncoded: string,
      request: UpdatePluginComponentSourceRequest,
    ) => {
      const trimmedName = String(name ?? "").trim();
      const trimmedLogicalId = String(logicalIdEncoded ?? "").trim();
      if (!trimmedName || !trimmedLogicalId || !request?.expectedDigest?.trim()) {
        throw new Error("Plugin name, component identity, and expected digest are required");
      }
      return dependencies.requestRuntime<RuntimePluginComponentSourceResponse>(
        `/v1/plugins/${encodeURIComponent(trimmedName)}/components/${encodeURIComponent(kind)}/${encodeURIComponent(trimmedLogicalId)}/source`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(request),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:set-plugin-component-state",
    async (
      _event,
      name: string,
      kind: RuntimePluginComponentKind,
      logicalIdEncoded: string,
      enabled: boolean,
    ) => {
      return dependencies.requestRuntime<RuntimePluginComponentStateResponse>(
        `/v1/plugins/${encodeURIComponent(name)}/components/${encodeURIComponent(kind)}/${encodeURIComponent(logicalIdEncoded)}/state`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled }),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:set-plugin-mcp-tool-state",
    async (
      _event,
      name: string,
      logicalIdEncoded: string,
      toolName: string,
      enabled: boolean,
    ) => {
      return dependencies.requestRuntime<RuntimePluginComponentStateResponse>(
        `/v1/plugins/${encodeURIComponent(name)}/mcp/${encodeURIComponent(logicalIdEncoded)}/tools/${encodeURIComponent(toolName)}/state`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled }),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:set-plugin-rule-activation",
    async (
      _event,
      name: string,
      logicalIdEncoded: string,
      activation: "always" | "file_globs" | "manual" | "model_decides",
    ) => {
      return dependencies.requestRuntime<{ plugin: RuntimePlugin; revision: number; }>(
        `/v1/plugins/${encodeURIComponent(name)}/components/rule/${encodeURIComponent(logicalIdEncoded)}/activation`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ activation }),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:list-plugin-hook-receipts",
    async (_event, name: string, logicalIdEncoded: string, revisionId?: string) => {
      const query = revisionId ? `?revisionId=${encodeURIComponent(revisionId)}` : "";
      return dependencies.requestRuntime<RuntimePluginHookReceiptResponse>(
        `/v1/plugins/${encodeURIComponent(name)}/hooks/${encodeURIComponent(logicalIdEncoded)}/receipts${query}`,
        { method: "GET" },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:retry-plugin-hook-receipt",
    async (_event, name: string, logicalIdEncoded: string, receiptId: string) => {
      return dependencies.requestRuntime<{ receipt: RuntimePluginHookReceipt; diagnosticOnly: true; }>(
        `/v1/plugins/${encodeURIComponent(name)}/hooks/${encodeURIComponent(logicalIdEncoded)}/receipts/${encodeURIComponent(receiptId)}/retry`,
        { method: "POST" },
      );
    },
  );

  registerRuntimeRequest("awaken:get-plugin-configuration", async (_event, name: string) => {
    return dependencies.requestRuntime<RuntimePluginConfigurationResponse>(
      `/v1/plugins/${encodeURIComponent(name)}/configuration`,
      { method: "GET" },
    );
  });

  registerRuntimeRequest(
    "awaken:update-plugin-configuration",
    async (_event, name: string, request: UpdatePluginConfigurationRequest) => {
      return dependencies.requestRuntime<RuntimePluginConfigurationResponse>(
        `/v1/plugins/${encodeURIComponent(name)}/configuration`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(request),
        },
      );
    },
  );

  registerRuntimeRequest("awaken:get-plugin-setup", async (_event, name: string) => {
    return dependencies.requestRuntime<RuntimePluginSetupResponse>(
      `/v1/plugins/${encodeURIComponent(String(name ?? "").trim())}/setup`,
      { method: "GET" },
    );
  });

  registerRuntimeRequest(
    "awaken:start-plugin-setup",
    async (
      _event,
      name: string,
      setupId: string,
      request: RuntimePluginSetupMutationRequest,
    ): Promise<RuntimePluginAuthSession> => {
      const pluginName = String(name ?? "").trim();
      const requirementId = String(setupId ?? "").trim();
      if (!pluginName || !requirementId || !request?.pluginDigest?.trim() || !request?.requirementFingerprint?.trim()) {
        throw new Error("插件、设置项和版本指纹不能为空");
      }
      const result = await dependencies.requestRuntime<CorePluginAuthStartResult>(
        `/v1/plugins/${encodeURIComponent(pluginName)}/setup/${encodeURIComponent(requirementId)}/sessions`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(request),
        },
      );
      const browserUrl = result.authorizationUrl || result.verificationUrl;
      if (browserUrl) {
        await shell.openExternal(validatePluginAuthBrowserUrl(browserUrl));
      }
      if (result.displayUserCode && result.userCode) {
        await dialog.showMessageBox({
          type: "info",
          title: "连接插件账号",
          message: "在浏览器中输入此临时代码",
          detail: result.userCode,
          buttons: ["我知道了"],
          noLink: true,
        });
      }
      return sanitizedPluginAuthSession(pluginName, requirementId, result);
    },
  );

  registerRuntimeRequest(
    "awaken:get-active-plugin-auth-session",
    async (
      _event,
      name: string,
      setupId: string,
      request: RuntimePluginSetupMutationRequest,
    ): Promise<RuntimePluginAuthSession | null> => {
      const pluginName = String(name ?? "").trim();
      const requirementId = String(setupId ?? "").trim();
      const pluginDigest = String(request?.pluginDigest ?? "").trim();
      const requirementFingerprint = String(request?.requirementFingerprint ?? "").trim();
      if (!pluginName || !requirementId || !pluginDigest || !requirementFingerprint) {
        throw new Error("插件、设置项和版本指纹不能为空");
      }
      const query = new URLSearchParams({ pluginDigest, requirementFingerprint });
      const result = await dependencies.requestRuntime<{ session?: RuntimePluginAuthSession | null; }>(
        `/v1/plugins/${encodeURIComponent(pluginName)}/active-setup-sessions/${encodeURIComponent(requirementId)}?${query.toString()}`,
        { method: "GET" },
      );
      return result.session ?? null;
    },
  );

  registerRuntimeRequest("awaken:get-plugin-auth-session", async (_event, name: string, sessionId: string) => {
    return dependencies.requestRuntime<RuntimePluginAuthSession>(
      `/v1/plugins/${encodeURIComponent(String(name ?? "").trim())}/setup/sessions/${encodeURIComponent(String(sessionId ?? "").trim())}`,
      { method: "GET" },
    );
  });

  registerRuntimeRequest("awaken:cancel-plugin-auth-session", async (_event, name: string, sessionId: string) => {
    return dependencies.requestRuntime<RuntimePluginAuthSession>(
      `/v1/plugins/${encodeURIComponent(String(name ?? "").trim())}/setup/sessions/${encodeURIComponent(String(sessionId ?? "").trim())}/cancel`,
      { method: "POST" },
    );
  });

  registerRuntimeRequest("awaken:mark-plugin-admin-approval", async (_event, name: string, sessionId: string) => {
    return dependencies.requestRuntime<RuntimePluginAuthSession>(
      `/v1/plugins/${encodeURIComponent(String(name ?? "").trim())}/setup/sessions/${encodeURIComponent(String(sessionId ?? "").trim())}/admin-approval`,
      { method: "POST" },
    );
  });

  registerRuntimeRequest(
    "awaken:probe-plugin-setup",
    async (_event, name: string, setupId: string, request: RuntimePluginSetupMutationRequest) => {
      return dependencies.requestRuntime<RuntimePluginSetupResponse>(
        `/v1/plugins/${encodeURIComponent(String(name ?? "").trim())}/setup/${encodeURIComponent(String(setupId ?? "").trim())}/probe`,
        { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(request) },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:disconnect-plugin-setup",
    async (_event, name: string, setupId: string, request: RuntimePluginSetupMutationRequest) => {
      return dependencies.requestRuntime<RuntimePluginSetupResponse>(
        `/v1/plugins/${encodeURIComponent(String(name ?? "").trim())}/setup/${encodeURIComponent(String(setupId ?? "").trim())}/credential`,
        { method: "DELETE", headers: { "Content-Type": "application/json" }, body: JSON.stringify(request) },
      );
    },
  );

  registerRuntimeRequest("awaken:pick-plugin-source", async (_event, defaultPath?: string) => {
    return pickPluginSource(dialog, { defaultPath });
  });

  registerRuntimeRequest("awaken:resolve-dropped-plugin-source", async (_event, droppedPath?: string) => {
    return resolveDroppedPluginSourcePath(droppedPath);
  });
}
