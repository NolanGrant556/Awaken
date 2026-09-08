import {
  ipcMain,
  shell
} from "electron";
import {
  normalizeGlobalAgentsDocument,
  type CliActivation,
  type CliProviderId,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type DeleteSkillRequest,
  type DeleteSkillResponse,
  type GetSkillPreviewRequest,
  type GlobalAgentsDeleteRequest,
  type GlobalAgentsDocument,
  type GlobalAgentsDocumentWire,
  type GlobalAgentsSaveRequest,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RuntimeConnectorAuthSession,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeSkill,
  type RuntimeToolInfo,
  type SkillPreviewResponse,
  type SkillProjectRoots,
  type UpdateMcpServerConfigRequest
} from "../shared/runtime.js";
import { CliConnectorManager } from "./cli-connectors/manager.js";
import {
  validatePluginAuthBrowserUrl
} from "./plugin-auth-browser.js";
import type { RuntimeRequester } from "./runtime-http.js";
import { RuntimeDomainError } from "./runtime-http.js";

type Dependencies = {
  readonly requestRuntimeOK: RuntimeRequester["requestVoid"];
  readonly requestRuntime: RuntimeRequester["request"];
  readonly cliConnectorManager: CliConnectorManager;
  readonly prepareNetworkRoute: (surface: string) => Promise<void>;
};

export function registerExtensionsIpc(dependencies: Dependencies): void {
  ipcMain.handle("awaken:get-hook-status", async (_event, projectId?: string) => {
    const suffix = projectId ? `?project_id=${encodeURIComponent(projectId)}` : "";
    return dependencies.requestRuntime(`/v1/hooks${suffix}`);
  });

  ipcMain.handle("awaken:list-connectors", async () => {
    const connectors = await dependencies.requestRuntime<RuntimeConnector[]>("/v1/connectors", {
      method: "GET",
    });
    const cliById = new Map(
      (dependencies.cliConnectorManager.list() ?? []).map((entry) => [entry.providerId, entry]),
    );
    return connectors.map((connector) => {
      const cli = cliById.get(connector.id as CliProviderId);
      return cli ? { ...connector, kind: "cli" as const, cliReadiness: cli.readiness } : connector;
    });
  });

  ipcMain.handle("awaken:connect-connector", async (_event, connectorId: string) => {
    await dependencies.prepareNetworkRoute("connector-connect");
    const result = await dependencies.requestRuntime<RuntimeConnectorActionResponse & { authorizationUrl?: string; }>(`/v1/connectors/${connectorId}/connect`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({}),
    });
    if (result.authorizationUrl) {
      await shell.openExternal(validatePluginAuthBrowserUrl(result.authorizationUrl));
    }
    const { authorizationUrl: _authorizationUrl, ...sanitized } = result;
    return sanitized;
  });

  ipcMain.handle("awaken:disconnect-connector", async (_event, connectorId: string) => {
    return dependencies.requestRuntime<RuntimeConnectorActionResponse>(
      `/v1/connectors/${connectorId}/disconnect`,
      {
        method: "POST",
      },
    );
  });

  ipcMain.handle("awaken:get-connector-auth-session", async (_event, sessionId: string) => {
    return dependencies.requestRuntime<RuntimeConnectorAuthSession>(
      `/v1/connectors/auth/sessions/${encodeURIComponent(String(sessionId ?? "").trim())}`,
      { method: "GET" },
    );
  });

  ipcMain.handle("awaken:cancel-connector-auth-session", async (_event, sessionId: string) => {
    return dependencies.requestRuntime<RuntimeConnectorAuthSession>(
      `/v1/connectors/auth/sessions/${encodeURIComponent(String(sessionId ?? "").trim())}/cancel`,
      { method: "POST" },
    );
  });

  ipcMain.handle("awaken:mark-connector-admin-approval", async (_event, sessionId: string) => {
    return dependencies.requestRuntime<RuntimeConnectorAuthSession>(
      `/v1/connectors/auth/sessions/${encodeURIComponent(String(sessionId ?? "").trim())}/admin-approval`,
      { method: "POST" },
    );
  });

  ipcMain.handle("awaken:list-cli-connectors", async () => dependencies.cliConnectorManager.list() ?? []);

  ipcMain.handle("awaken:refresh-cli-connector", async (_event, providerId: CliProviderId) => {

    return dependencies.cliConnectorManager.refreshProvider(providerId);
  });

  ipcMain.handle("awaken:install-cli-connector", async (_event, providerId: CliProviderId) => {

    return dependencies.cliConnectorManager.install(providerId);
  });

  ipcMain.handle("awaken:cancel-cli-install", async (_event, providerId: CliProviderId) => {

    return dependencies.cliConnectorManager.cancelInstall(providerId);
  });

  ipcMain.handle("awaken:start-cli-auth", async (_event, providerId: CliProviderId) => {

    return dependencies.cliConnectorManager.startAuth(providerId);
  });

  ipcMain.handle("awaken:cancel-cli-auth", async (_event, providerId: CliProviderId) => {

    return dependencies.cliConnectorManager.cancelAuth(providerId);
  });

  ipcMain.handle("awaken:retry-cli-auth", async (_event, providerId: CliProviderId) => {

    return dependencies.cliConnectorManager.retryAuth(providerId);
  });

  ipcMain.handle(
    "awaken:set-cli-activation",
    async (_event, providerId: CliProviderId, activation: CliActivation) => {

      return dependencies.cliConnectorManager.setActivation(providerId, activation);
    },
  );

  ipcMain.handle("awaken:logout-cli-connector", async (_event, providerId: CliProviderId) => {

    return dependencies.cliConnectorManager.logout(providerId);
  });

  ipcMain.handle("awaken:list-mcp-servers", async () => {
    return dependencies.requestRuntime<RuntimeMcpServer[]>("/v1/mcp/servers", {
      method: "GET",
    });
  });

  ipcMain.handle(
    "awaken:configure-mcp-servers",
    async (_event, request: ConfigureMcpServersRequest) => {
      await dependencies.prepareNetworkRoute("mcp-configure");
      return dependencies.requestRuntime<ConfigureMcpServersResponse>("/v1/mcp/servers", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(request),
      });
    },
  );

  ipcMain.handle("awaken:get-mcp-server-config", async (_event, name: string) => {
    return dependencies.requestRuntime<RuntimeMcpServerConfigResponse>(
      `/v1/mcp/servers/${encodeURIComponent(name)}/config`,
      {
        method: "GET",
      },
    );
  });

  ipcMain.handle(
    "awaken:update-mcp-server-config",
    async (_event, name: string, request: UpdateMcpServerConfigRequest) => {
      await dependencies.prepareNetworkRoute("mcp-update");
      return dependencies.requestRuntime<ConfigureMcpServersResponse>(
        `/v1/mcp/servers/${encodeURIComponent(name)}/config`,
        {
          method: "PATCH",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify(request),
        },
      );
    },
  );

  ipcMain.handle("awaken:list-mcp-tools", async () => {
    return dependencies.requestRuntime<RuntimeToolInfo[]>("/v1/mcp/tools", {
      method: "GET",
    });
  });

  ipcMain.handle("awaken:disable-mcp-server", async (_event, name: string) => {
    return dependencies.requestRuntimeOK(`/v1/mcp/servers/${encodeURIComponent(name)}/disable`, {
      method: "POST",
    });
  });

  ipcMain.handle("awaken:enable-mcp-server", async (_event, name: string) => {
    await dependencies.prepareNetworkRoute("mcp-enable");
    return dependencies.requestRuntimeOK(`/v1/mcp/servers/${encodeURIComponent(name)}/enable`, {
      method: "POST",
    });
  });

  ipcMain.handle("awaken:delete-mcp-server", async (_event, name: string) => {
    return dependencies.requestRuntimeOK(`/v1/mcp/servers/${encodeURIComponent(name)}`, {
      method: "DELETE",
    });
  });

  ipcMain.handle("awaken:list-skills", async (_event, projectRoots?: SkillProjectRoots) => {
    const roots = (Array.isArray(projectRoots) ? projectRoots : [projectRoots])
      .map((root) => root?.trim())
      .filter((root): root is string => Boolean(root));
    const query = new URLSearchParams();
    for (const root of roots) {
      query.append("projectRoot", root);
    }
    const requestPath = query.size > 0 ? `/v1/skills?${query.toString()}` : "/v1/skills";
    return dependencies.requestRuntime<RuntimeSkill[]>(requestPath, {
      method: "GET",
    });
  });

  ipcMain.handle("awaken:get-skill-preview", async (_event, request: GetSkillPreviewRequest) => {
    return dependencies.requestRuntime<SkillPreviewResponse>("/v1/skills/preview", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });
  });

  ipcMain.handle("awaken:install-skill", async (_event, request: InstallSkillRequest) => {
    return dependencies.requestRuntime<InstallSkillResponse>("/v1/skills/install", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });
  });

  ipcMain.handle("awaken:delete-skill", async (_event, request: DeleteSkillRequest) => {
    const id = request.id.trim();
    if (!id) {
      throw new Error("Skill id is required");
    }
    return dependencies.requestRuntime<DeleteSkillResponse>(`/v1/skills/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        sourceOrigin: request.sourceOrigin,
        projectRoot: request.projectRoot,
      }),
    });
  });

  ipcMain.handle("awaken:disable-skill", async (_event, id: string) => {
    const trimmed = id.trim();
    if (!trimmed) {
      throw new Error("Skill id is required");
    }
    return dependencies.requestRuntime<{ id: string; state: string; }>(
      `/v1/skills/${encodeURIComponent(trimmed)}/disable`,
      {
        method: "POST",
      },
    );
  });

  ipcMain.handle("awaken:enable-skill", async (_event, id: string) => {
    const trimmed = id.trim();
    if (!trimmed) {
      throw new Error("Skill id is required");
    }
    return dependencies.requestRuntime<{ id: string; state: string; }>(
      `/v1/skills/${encodeURIComponent(trimmed)}/enable`,
      {
        method: "POST",
      },
    );
  });

  ipcMain.handle("awaken:get-global-agents-document", async (): Promise<GlobalAgentsDocument> => {
    const document = await dependencies.requestRuntime<GlobalAgentsDocumentWire>("/v1/instructions/global", { method: "GET" });
    return normalizeGlobalAgentsDocument(document);
  });

  ipcMain.handle(
    "awaken:save-global-agents-document",
    async (_event, request: GlobalAgentsSaveRequest): Promise<GlobalAgentsDocument> => {
      try {
        const document = await dependencies.requestRuntime<GlobalAgentsDocumentWire>("/v1/instructions/global", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ expected_revision: request.expectedRevision, content: request.content }),
        });
        return normalizeGlobalAgentsDocument(document);
      } catch (error) {
        if (error instanceof RuntimeDomainError) throw new Error(`${error.code}: ${error.message}`);
        throw error;
      }
    },
  );

  ipcMain.handle(
    "awaken:delete-global-agents-document",
    async (_event, request: GlobalAgentsDeleteRequest): Promise<GlobalAgentsDocument> => {
      try {
        const document = await dependencies.requestRuntime<GlobalAgentsDocumentWire>("/v1/instructions/global", {
          method: "DELETE",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ expected_revision: request.expectedRevision }),
        });
        return normalizeGlobalAgentsDocument(document);
      } catch (error) {
        if (error instanceof RuntimeDomainError) throw new Error(`${error.code}: ${error.message}`);
        throw error;
      }
    },
  );
}
