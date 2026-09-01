import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { PetsSettingsPanel } from "./PetsSettingsPanel";
import type { RuntimeClient } from "../runtime/client";

function createMockRuntimeClient(): RuntimeClient {
  return {
    source: "electron",
    platform: "windows",
    relaunchDesktop: vi.fn(async () => undefined),
    getSearchSettings: vi.fn(async () => ({
      providers: {
        exa: { enabled: true },
        tavily: { enabled: false },
      },
    })),
    saveSearchSettings: vi.fn(),
    testSearchProvider: vi.fn(),
    getACPRunners: vi.fn(async () => ({ runners: [] })),
    createACPRunner: vi.fn(),
    updateACPRunner: vi.fn(),
    deleteACPRunner: vi.fn(),
    probeACPRunner: vi.fn(),
    getRuntimeInfo: vi.fn(async () => ({
      runtimeVersion: "1.0.0",
      apiVersion: "v1",
      configuredModel: "default",
      protocol: "aguiv1",
      hasApiKey: true,
      defaultWorkspace: "/workspace",
      baseUrlHost: "127.0.0.1:19001",
    })),
    listConnectors: vi.fn(async () => []),
    connectConnector: vi.fn(async () => ({
      connectorId: "",
      connectorState: "connected",
    })),
    disconnectConnector: vi.fn(async () => ({
      connectorId: "",
      connectorState: "disconnected",
    })),
    listCliConnectors: vi.fn(async () => []),
    installCliConnector: vi.fn(),
    refreshCliConnector: vi.fn(),
    cancelCliInstall: vi.fn(),
    startCliAuth: vi.fn(),
    cancelCliAuth: vi.fn(),
    retryCliAuth: vi.fn(),
    setCliConnectorActivation: vi.fn(),
    logoutCliConnector: vi.fn(),
    uninstallManagedCli: vi.fn(),
    subscribeCliConnectorState: vi.fn(() => () => {}),
    subscribeComposerCatalogEvents: vi.fn(() => () => {}),
    listMcpServers: vi.fn(async () => []),
    configureMcpServers: vi.fn(async () => ({ serverNames: [], message: "" })),
    getMcpServerConfig: vi.fn(async () => ({
      name: "",
      configText: "",
      scope: "",
    })),
    updateMcpServerConfig: vi.fn(async () => ({
      serverNames: [],
      message: "",
    })),
    getRuntimeLogs: vi.fn(async () => []),
    disableMcpServer: vi.fn(async () => undefined),
    enableMcpServer: vi.fn(async () => undefined),
    deleteMcpServer: vi.fn(async () => undefined),
    listMcpTools: vi.fn(async () => []),
    listSkills: vi.fn(async () => []),
    getSkillPreview: vi.fn(async () => ({
      frontmatterRaw: "",
      bodyMarkdown: "",
    })),
    installSkill: vi.fn(async () => ({
      skill: {} as never,
      path: "",
      message: "",
    })),
    deleteSkill: vi.fn(async () => ({
      id: "",
      sourceOrigin: "user",
      originalPath: "",
      message: "",
    })),
    disableSkill: vi.fn(async () => undefined),
    enableSkill: vi.fn(async () => undefined),
    listPlugins: vi.fn(async () => []),
    getPlugin: vi.fn(),
    validatePlugin: vi.fn(),
    installPlugin: vi.fn(),
    commitPlugin: vi.fn(),
    updatePluginComponentSource: vi.fn(),
    enablePlugin: vi.fn(),
    disablePlugin: vi.fn(),
    uninstallPlugin: vi.fn(),
    enablePluginServer: vi.fn(),
    disablePluginServer: vi.fn(),
    listPluginComponents: vi.fn(async () => []),
    getPluginComponent: vi.fn(),
    setPluginComponentState: vi.fn(),
    setPluginMcpToolState: vi.fn(),
    setPluginRuleActivation: vi.fn(),
    listPluginHookReceipts: vi.fn(),
    retryPluginHookReceipt: vi.fn(),
    getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 0 })),
    updatePluginConfiguration: vi.fn(),
    listModelProviders: vi.fn(async () => []),
    listModelProfiles: vi.fn(async () => []),
    getAgentPlanAccount: vi.fn(async () => ({
      accountId: "volcengine-agent-plan-default" as const,
      providerId: "volcengine-agent-plan" as const,
      hasKey: false,
      credentialRevision: 1,
      modelIds: ["ark-code-latest"],
      catalog: [{ id: "ark-code-latest", kind: "llm" as const }],
    })),
    saveAgentPlanAccount: vi.fn(async () => ({
      accountId: "volcengine-agent-plan-default" as const,
      providerId: "volcengine-agent-plan" as const,
      hasKey: true,
      credentialRevision: 1,
      modelIds: ["ark-code-latest"],
      catalog: [{ id: "ark-code-latest", kind: "llm" as const }],
    })),
    refreshAgentPlanModels: vi.fn(async () => ({
      accountId: "volcengine-agent-plan-default" as const,
      providerId: "volcengine-agent-plan" as const,
      hasKey: true,
      credentialRevision: 1,
      modelIds: ["ark-code-latest"],
      catalog: [{ id: "ark-code-latest", kind: "llm" as const }],
    })),
    deleteAgentPlanAccount: vi.fn(async () => undefined),
    getAgentPlanMediaTools: vi.fn(async () => ({ configurations: [] })),
    saveAgentPlanMediaTool: vi.fn(async () => ({
      settings: { configurations: [] },
      applyStatus: "applied" as const,
    })),
    deleteAgentPlanMediaTool: vi.fn(async () => ({
      settings: { configurations: [] },
      applyStatus: "applied" as const,
    })),
    saveModelProfile: vi.fn(async () => ({
      id: "",
      providerId: "",
      displayName: "",
      modelId: "",
      baseURL: "",
      protocol: "",
      authScheme: "",
      hasKey: false,
      enabled: true,
      isDefault: false,
      profileStatus: "ready" as const,
    })),
    deleteModelProfile: vi.fn(async () => undefined),
    setDefaultModelProfile: vi.fn(async () => ({
      id: "",
      providerId: "",
      displayName: "",
      modelId: "",
      baseURL: "",
      protocol: "",
      authScheme: "",
      hasKey: false,
      enabled: true,
      isDefault: false,
      profileStatus: "ready" as const,
    })),
    testModelProfile: vi.fn(async () => ({ ok: true, message: "" })),
    testAgentPlanReasoningCapability: vi.fn(async () => ({ ok: false, message: "" })),
    syncModelCapabilities: vi.fn(async () => []),
    refreshModelCatalog: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    getModelProfileBalances: vi.fn(async () => ({ balances: [] })),
    getRun: vi.fn(async () => ({
      runId: "",
      threadId: "",
      status: "running" as const,
      cancelRequested: false,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    })),
    getThreadTranscript: vi.fn(async () => ({ thread: {} as never, runs: [] })),
    listThreads: vi.fn(async () => []),
    renameThread: vi.fn(async () => ({
      archived: false,
      threadId: "",
      label: "",
      workspace: {} as never,
      createdAt: "",
      updatedAt: "",
    })),
    updateThreadModelPreference: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    deleteThread: vi.fn(async () => ({ ok: true as const })),
    setThreadArchived: vi.fn(async () => ({ ok: true as const, thread: {} as never })),
    deleteThreads: vi.fn(async (threadIds: string[]) => ({
      ok: true as const,
      deletedThreadIds: threadIds,
      failures: [],
    })),
    listProjects: vi.fn(async () => []),
    listProjectDirectory: vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [],
    })),
    watchProjectDirectory: vi.fn(async () => ({
      ok: true as const,
      subscriptionId: "project-files-watch-1",
    })),
    unwatchProjectDirectory: vi.fn(async () => undefined),
    onProjectDirectoryChanged: vi.fn(() => () => {}),
    resolveProjectEntryForTask: vi.fn(async () => ({
      ok: false as const,
      code: "ENTRY_NOT_FOUND" as const,
      message: "not found",
    })),
    copyProjectEntryPath: vi.fn(async () => ({ ok: true as const })),
    revealProjectEntry: vi.fn(async () => ({ ok: true as const })),
    openProjectEntryWith: vi.fn(async () => ({ ok: true as const })),
    startProjectFilePreview: vi.fn(async () => ({
      ok: false as const,
      code: "UNSUPPORTED_FILE_TYPE" as const,
      message: "unsupported",
    })),
    startArtifactPreview: vi.fn(async () => ({
      ok: false as const,
      code: "UNSUPPORTED_FILE_TYPE" as const,
      message: "unsupported",
    })),
    closeProjectFilePreview: vi.fn(async () => undefined),
    onProjectFilePreviewInteraction: vi.fn(() => () => {}),
    openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
    revealProjectFile: vi.fn(async () => ({ ok: true as const })),
    saveProjectMarkdownFile: vi.fn(async () => ({
      ok: true as const,
      byteLength: 0,
    })),
    upsertProject: vi.fn(async () => ({}) as never),
    renameProject: vi.fn(async () => ({}) as never),
    reorderProjects: vi.fn(async () => ({}) as never),
    detachProject: vi.fn(async () => ({}) as never),
    deleteProjectThreads: vi.fn(async () => ({}) as never),
    createProject: vi.fn(async () => ({}) as never),
    attachProjectFolder: vi.fn(async () => ({}) as never),
    detachProjectFolder: vi.fn(async () => ({}) as never),
    setPrimaryProjectFolder: vi.fn(async () => ({}) as never),
    trashProjectFolder: vi.fn(async () => ({}) as never),
    getProjectOperation: vi.fn(async () => ({}) as never),
    openArtifact: vi.fn(async () => ({ ok: true as const })),
    getArtifactMediaSource: vi.fn(async () => ({
      ok: true as const,
      src: "awaken-artifact://media/test/artifact.png",
      mimeType: "image/png",
      byteLength: 128,
    })),
    revealPath: vi.fn(async () => ({ ok: true as const })),
    revealArtifact: vi.fn(async () => ({ ok: true as const })),
    startRun: vi.fn(async () => ({
      runId: "",
      threadId: "",
      status: "running" as const,
      cancelRequested: false,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    })),
    cancelRun: vi.fn(async () => ({
      runId: "",
      threadId: "",
      status: "cancelled" as const,
      cancelRequested: true,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    })),
    answerUserQuestion: vi.fn(async () => undefined),
    cancelUserQuestion: vi.fn(async () => undefined),
    resolveExternalInteraction: vi.fn(async () => undefined),
    resolveACPPermission: vi.fn(async () => undefined),
    resolveComputerReview: vi.fn(async () => undefined),
    computerSessionControl: vi.fn(async () => undefined),
    getComputerHostStatus: vi.fn(async () => ({
      available: false,
      healthy: false,
      tcc_screen_recording: false,
      tcc_accessibility: false,
      message: "not configured",
    })),
    getComputerSettings: vi.fn(async () => ({
      revision: 0,
      value: {
        enabled: false,
        allow_background_window_actions: false,
      },
    })),
    saveComputerSettings: vi.fn(async () => ({
      revision: 1,
      value: {
        enabled: false,
        allow_background_window_actions: false,
      },
    })),
    prepareComputerRuntime: vi.fn(async () => ({
      node_ready: false,
      cua_host_ready: false,
      message: "not configured",
    })),
    requestComputerPermissions: vi.fn(async () => ({
      available: false,
      healthy: false,
      tcc_screen_recording: false,
      tcc_accessibility: false,
    })),
    openComputerPermissionSettings: vi.fn(async () => undefined),
    getDesktopPreferences: vi.fn(async () => ({ theme: "light" as const })),
    saveDesktopPreferences: vi.fn(async () => ({ theme: "light" as const })),
    subscribeRun: vi.fn(async () => () => {}),
    pickAttachments: vi.fn(async () => ({ canceled: true as const })),
    pickDirectory: vi.fn(async () => ({ canceled: true as const })),
    pickPluginSource: vi.fn(async () => ({ canceled: true as const })),
    resolveDroppedPluginSource: vi.fn(async () => ({ canceled: true as const })),
    pickProjectFolders: vi.fn(async () => ({ canceled: true as const })),
    validateWorkspace: vi.fn(async () => ({ valid: true as const })),
    getSupervisorStatus: vi.fn(async () => ({ status: "running" as const })),
    onSupervisorStatusChange: vi.fn(() => () => {}),
    notifyRendererReady: vi.fn(),
    listPets: vi.fn(async () => [
      {
        id: "test-pet",
        sourceId: "test-pet",
        displayName: "Test Pet",
        description: "A test pet.",
        source: "codex-custom" as const,
        spriteVersionNumber: 1 as const,
        available: true,
      },
    ]),
    getPetSettings: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "test-pet",
      overlayOpen: false,
      mascotWidthPx: 112,
      importCodexPets: true,
      welcomedPetIds: [],
      dismissedNotificationKeys: [],
      placementByDisplay: {},
    })),
    selectPet: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "test-pet",
      overlayOpen: false,
      mascotWidthPx: 112,
      importCodexPets: true,
      welcomedPetIds: [],
      dismissedNotificationKeys: [],
      placementByDisplay: {},
    })),
    refreshCodexPets: vi.fn(async () => ({
      imported: [],
      updated: [],
      sourceMissing: [],
      failed: [],
      conflicts: [],
      selectedPetIdUnchanged: "test-pet",
    })),
    createPet: vi.fn(async () => ({ threadId: "task-1" })),
    setPetOverlayOpen: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "test-pet",
      overlayOpen: true,
      mascotWidthPx: 112,
      importCodexPets: true,
      welcomedPetIds: [],
      dismissedNotificationKeys: [],
      placementByDisplay: {},
    })),
    setPetImportPreferences: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "test-pet",
      overlayOpen: false,
      mascotWidthPx: 112,
      importCodexPets: false,
      welcomedPetIds: [],
      dismissedNotificationKeys: [],
      placementByDisplay: {},
    })),
    updatePetActivity: vi.fn(),
    onPetCatalogChanged: vi.fn(() => () => {}),
    onPetThreadActivationRequested: vi.fn(() => () => {}),
    openAwakenPetsDirectory: vi.fn(async () => undefined),
    openCodexPetsDirectory: vi.fn(async () => undefined),
    listMemories: vi.fn(async () => []),
    getMemory: vi.fn(async () => {
      throw new Error("not found");
    }),
    deleteMemory: vi.fn(async () => undefined),
    getMemorySettings: vi.fn(async () => ({
      revision: 0,
      value: {
        enabled: true,
        automaticRecall: true,
        proactiveMemory: true,
        autoDream: false,
      },
    })),
    updateMemorySettings: vi.fn(async () => ({
      revision: 1,
      value: {
        enabled: true,
        automaticRecall: true,
        proactiveMemory: true,
        autoDream: false,
      },
    })),
    startMemoryDream: vi.fn(async () => ({
      taskId: "dream",
      status: "queued" as const,
      scopes: ["user" as const],
      createdAt: new Date(0).toISOString(),
    })),
    getMemoryDream: vi.fn(async () => ({
      taskId: "dream",
      status: "completed" as const,
      scopes: ["user" as const],
      createdAt: new Date(0).toISOString(),
    })),
    listMemoryDreams: vi.fn(async () => []),
    cancelMemoryDream: vi.fn(async () => undefined),
    onMemoryDreamReceipt: vi.fn(() => () => {}),
    listScheduledTasks: vi.fn(async () => []),
    getScheduledTask: vi.fn(async () => {
      throw new Error("not found");
    }),
    createScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    patchScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    deleteScheduledTask: vi.fn(async () => undefined),
    pauseScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    resumeScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    runScheduledTaskNow: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    cancelScheduledTaskExecution: vi.fn(async () => undefined),
    listScheduledTaskExecutions: vi.fn(async () => []),
    getSystemTimezone: vi.fn(async () => "UTC"),
    subscribeScheduledTaskEvents: vi.fn(() => () => {}),
    onScheduledTaskThreadActivation: vi.fn(() => () => {}),
    getDesktopUpdateState: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    checkDesktopUpdate: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    downloadDesktopUpdate: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    restartAndInstallDesktopUpdate: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    onDesktopUpdateStateChanged: vi.fn(() => () => {}),
  } satisfies RuntimeClient;
}

describe("PetsSettingsPanel", () => {
  it("renders the pets settings shell", async () => {
    const client = createMockRuntimeClient();
    const html = renderToString(React.createElement(PetsSettingsPanel, { runtimeClient: client }));

    expect(html).toContain("宠物");
    expect(html).toContain("唤醒宠物");
    expect(html).toContain("当前选择的宠物");
    expect(html).toContain("刷新 Codex 宠物");
    expect(html).not.toContain("创建自己的宠物");
  });
});
