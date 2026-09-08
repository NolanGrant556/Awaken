import {
  ipcMain
} from "electron";
import {
  normalizeAgentPlanAccount,
  normalizeAgentPlanMediaTools,
  normalizeModelProfileBalancesResult,
  normalizeModelProfileSummary,
  normalizeModelProvider,
  type AgentPlanAccountSaveRequest,
  type AgentPlanMediaCapability,
  type AgentPlanMediaToolSaveRequest,
  type AgentPlanMediaToolsSaveResult,
  type AgentPlanMediaToolsWire,
  type ModelProfileBalancesResultWire,
  type ModelProfileSaveRequest,
  type ModelProviderWire,
  type ModelPurposeRouting,
  type RuntimeModelProfileSummaryWire,
  type RuntimeThreadWire,
  type SearchProviderTestRequest,
  type SearchProviderTestResult,
  type SearchSettings,
  type SearchSettingsSaveRequest,
  type SearchSettingsSaveResult
} from "../shared/runtime.js";
import { runDesktopOperation } from "./desktop-operation.js";
import { ModelProfileStore } from "./model-profile-store.js";
import type { RuntimeRequester } from "./runtime-http.js";
import { RuntimeSupervisor } from "./runtime-supervisor.js";
import { SearchSettingsStore } from "./search-settings-store.js";

type Dependencies = {
  readonly requestRuntime: RuntimeRequester["request"];
  readonly modelProfileStore: ModelProfileStore;
  readonly hasAgentPlanMediaToolReferences: () => Promise<boolean>;
  readonly ensureNoActiveRunsForSettingsChange: () => Promise<void>;
  readonly restartCoreForAgentPlanTools: () => Promise<void>;
  readonly mutateAgentPlanMediaTools: (capability: AgentPlanMediaCapability, operation: () => Promise<AgentPlanMediaToolsWire>) => Promise<AgentPlanMediaToolsSaveResult>;
  readonly prepareNetworkRoute: (surface: string) => Promise<void>;
  readonly searchSettingsStore: SearchSettingsStore;
  readonly supervisor: RuntimeSupervisor;
};

export function registerModelsIpc(dependencies: Dependencies): void {
  ipcMain.handle("awaken:list-model-providers", async () => {

    const providers = dependencies.modelProfileStore.listProviders() satisfies ModelProviderWire[];
    return providers.map(normalizeModelProvider);
  });

  ipcMain.handle("awaken:list-model-profiles", async () => {

    const profiles =
      (await dependencies.modelProfileStore.listProfiles()) satisfies RuntimeModelProfileSummaryWire[];
    return profiles.map(normalizeModelProfileSummary);
  });

  ipcMain.handle("awaken:get-model-purpose-routing", async () => {

    return dependencies.modelProfileStore.getPurposeRouting();
  });

  ipcMain.handle(
    "awaken:save-model-purpose-routing",
    async (_event, routing: ModelPurposeRouting) => {

      return dependencies.modelProfileStore.savePurposeRouting(routing);
    },
  );

  ipcMain.handle("awaken:get-agent-plan-account", async () => {

    return normalizeAgentPlanAccount(await dependencies.modelProfileStore.getAgentPlanAccount());
  });

  ipcMain.handle(
    "awaken:save-agent-plan-account",
    async (_event, request: AgentPlanAccountSaveRequest) =>
      runDesktopOperation(async () => {

        const restartTools = await dependencies.hasAgentPlanMediaToolReferences();
        if (restartTools) await dependencies.ensureNoActiveRunsForSettingsChange();
        const account = normalizeAgentPlanAccount(
          await dependencies.modelProfileStore.saveAgentPlanAccount(request),
        );
        if (restartTools) await dependencies.restartCoreForAgentPlanTools();
        return account;
      }),
  );

  ipcMain.handle("awaken:refresh-agent-plan-models", async () =>
    runDesktopOperation(async () => {

      const restartTools = await dependencies.hasAgentPlanMediaToolReferences();
      if (restartTools) await dependencies.ensureNoActiveRunsForSettingsChange();
      const account = normalizeAgentPlanAccount(
        await dependencies.modelProfileStore.refreshAgentPlanModels(),
      );
      if (restartTools) await dependencies.restartCoreForAgentPlanTools();
      return account;
    }),
  );

  ipcMain.handle("awaken:delete-agent-plan-account", async () => {

    await dependencies.modelProfileStore.deleteAgentPlanAccount();
  });

  ipcMain.handle("awaken:get-agent-plan-media-tools", async () => {

    return normalizeAgentPlanMediaTools(
      await dependencies.modelProfileStore.getAgentPlanMediaTools(),
    );
  });

  ipcMain.handle(
    "awaken:save-agent-plan-media-tool",
    async (_event, request: AgentPlanMediaToolSaveRequest) => {

      return dependencies.mutateAgentPlanMediaTools(request.capability, () =>
        dependencies.modelProfileStore!.saveAgentPlanMediaTool(request),
      );
    },
  );

  ipcMain.handle(
    "awaken:delete-agent-plan-media-tool",
    async (_event, capability: AgentPlanMediaCapability) => {

      return dependencies.mutateAgentPlanMediaTools(capability, () =>
        dependencies.modelProfileStore!.deleteAgentPlanMediaTool(capability),
      );
    },
  );

  ipcMain.handle(
    "awaken:save-model-profile",
    async (_event, request: ModelProfileSaveRequest) => {
      return runDesktopOperation(async () => {

        const restartTools =
          request.providerId === "volcengine-agent-plan" &&
          Boolean(request.apiKey?.trim()) &&
          (await dependencies.hasAgentPlanMediaToolReferences());
        if (restartTools) await dependencies.ensureNoActiveRunsForSettingsChange();
        const profile = normalizeModelProfileSummary(
          await dependencies.modelProfileStore.saveProfile(request),
        );
        if (restartTools) await dependencies.restartCoreForAgentPlanTools();
        return profile;
      });
    },
  );

  ipcMain.handle("awaken:delete-model-profile", async (_event, profileId: string) => {

    await dependencies.modelProfileStore.deleteProfile(profileId);
  });

  ipcMain.handle("awaken:set-default-model-profile", async (_event, profileId: string) => {

    return normalizeModelProfileSummary(await dependencies.modelProfileStore.setDefaultProfile(profileId));
  });

  ipcMain.handle("awaken:test-model-profile", async (_event, request: ModelProfileSaveRequest) => {
    return runDesktopOperation(async () => {

      await dependencies.prepareNetworkRoute("model-profile-test");
      return dependencies.modelProfileStore.testProfile(request);
    });
  });

  ipcMain.handle(
    "awaken:test-agent-plan-reasoning-capability",
    async (_event, request: ModelProfileSaveRequest) =>
      runDesktopOperation(async () => {

        await dependencies.prepareNetworkRoute("model-reasoning-test");
        return dependencies.modelProfileStore.testAgentPlanReasoningCapability(request);
      }),
  );

  ipcMain.handle("awaken:sync-model-capabilities", async () => {

    await dependencies.prepareNetworkRoute("model-capability-sync");
    await dependencies.modelProfileStore.syncCapabilities();
    const providers = dependencies.modelProfileStore.listProviders() satisfies ModelProviderWire[];
    return providers.map(normalizeModelProvider);
  });

  ipcMain.handle(
    "awaken:refresh-model-catalog",
    async (_event, request: ModelProfileSaveRequest) =>
      runDesktopOperation(async () => {

        await dependencies.prepareNetworkRoute("model-catalog-refresh");
        const result = await dependencies.modelProfileStore.refreshProviderCatalog(request);
        return {
          provider: normalizeModelProvider(result.provider),
          message: result.message,
          discoveredModelIds: result.discoveredModelIds,
        };
      }),
  );

  ipcMain.handle("awaken:get-model-profile-balances", async () => {

    await dependencies.prepareNetworkRoute("model-balance");
    const balances = await dependencies.modelProfileStore.fetchBalances();
    const result: ModelProfileBalancesResultWire = { balances };
    return normalizeModelProfileBalancesResult(result);
  });

  ipcMain.handle("awaken:get-search-settings", async (): Promise<SearchSettings> => {

    return dependencies.searchSettingsStore.snapshot();
  });

  ipcMain.handle(
    "awaken:test-search-provider",
    async (_event, request: SearchProviderTestRequest): Promise<SearchProviderTestResult> => {

      await dependencies.prepareNetworkRoute("search-provider-test");
      return dependencies.searchSettingsStore.testProvider(request.provider);
    },
  );

  ipcMain.handle(
    "awaken:save-search-settings",
    async (_event, request: SearchSettingsSaveRequest): Promise<SearchSettingsSaveResult> => {

      const threads = await dependencies.requestRuntime<RuntimeThreadWire[]>("/v1/threads");
      if (threads.some((thread) => Boolean(thread.active_run_id))) {
        throw new Error("当前有任务正在运行，请结束任务后重试。");
      }
      const settings = await dependencies.searchSettingsStore.save(request);
      try {
        const status = await dependencies.supervisor.restart();
        if (status.status !== "running" || !status.runtimeBaseUrl) {
          return {
            settings,
            restartStatus: "restart_failed",
            message: "搜索设置已保存，但 Core 重启失败。请稍后重试重启。",
          };
        }
        return { settings, restartStatus: "restarted" };
      } catch {
        return {
          settings,
          restartStatus: "restart_failed",
          message: "搜索设置已保存，但 Core 重启失败。请稍后重试重启。",
        };
      }
    },
  );
}
