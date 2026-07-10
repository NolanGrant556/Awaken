import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { PetsSettingsPanel } from "./PetsSettingsPanel";
import type { RuntimeClient } from "../runtime/client";

function createMockRuntimeClient(): RuntimeClient {
  return {
    source: "electron",
    platform: "windows",
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
    connectConnector: vi.fn(async () => ({ connectorId: "", connectorState: "connected" })),
    disconnectConnector: vi.fn(async () => ({ connectorId: "", connectorState: "disconnected" })),
    listMcpServers: vi.fn(async () => []),
    configureMcpServers: vi.fn(async () => ({ serverNames: [], message: "" })),
    getMcpServerConfig: vi.fn(async () => ({ name: "", configText: "", scope: "" })),
    updateMcpServerConfig: vi.fn(async () => ({ serverNames: [], message: "" })),
    getRuntimeLogs: vi.fn(async () => []),
    disableMcpServer: vi.fn(async () => undefined),
    enableMcpServer: vi.fn(async () => undefined),
    deleteMcpServer: vi.fn(async () => undefined),
    listMcpTools: vi.fn(async () => []),
    listSkills: vi.fn(async () => []),
    installSkill: vi.fn(async () => ({ skill: {} as never, path: "", message: "" })),
    listModelProviders: vi.fn(async () => []),
    listModelProfiles: vi.fn(async () => []),
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
    syncModelCapabilities: vi.fn(async () => []),
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
    renameThread: vi.fn(async () => ({ threadId: "", label: "", workspace: {} as never, createdAt: "", updatedAt: "" })),
    deleteThread: vi.fn(async () => undefined),
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
    resolveMassMessageReview: vi.fn(async () => undefined),
    answerUserQuestion: vi.fn(async () => undefined),
    getDesktopPreferences: vi.fn(async () => ({ theme: "light" as const })),
    saveDesktopPreferences: vi.fn(async () => ({ theme: "light" as const })),
    subscribeRun: vi.fn(async () => () => {}),
    pickAttachments: vi.fn(async () => ({ canceled: true as const })),
    pickDirectory: vi.fn(async () => ({ canceled: true as const })),
    pickWorkspace: vi.fn(async () => ({ canceled: true as const })),
    validateWorkspace: vi.fn(async () => ({ valid: true as const })),
    readWorkspaceRegistry: vi.fn(async () => ({ registeredWorkspacePaths: [] })),
    persistWorkspaceRegistry: vi.fn(async () => undefined),
    getSupervisorStatus: vi.fn(async () => ({ status: "running" as const })),
    onSupervisorStatusChange: vi.fn(() => () => {}),
    notifyRendererReady: vi.fn(),
    listPets: vi.fn(async () => [
      {
        id: "codex-custom:test-pet",
        sourceId: "test-pet",
        displayName: "Test Pet",
        description: "A test pet.",
        source: "codex-custom" as const,
        available: true,
      },
    ]),
    getPetSettings: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "codex-custom:test-pet",
      overlayOpen: false,
      mascotWidthPx: 112,
      importCodexPets: true,
      welcomedPetIds: [],
      dismissedNotificationKeys: [],
      placementByDisplay: {},
    })),
    selectPet: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "codex-custom:test-pet",
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
      selectedPetIdUnchanged: "codex-custom:test-pet",
    })),
    createPet: vi.fn(async () => ({ threadId: "task-1" })),
    setPetOverlayOpen: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "codex-custom:test-pet",
      overlayOpen: true,
      mascotWidthPx: 112,
      importCodexPets: true,
      welcomedPetIds: [],
      dismissedNotificationKeys: [],
      placementByDisplay: {},
    })),
    setPetImportPreferences: vi.fn(async () => ({
      schemaVersion: 1 as const,
      selectedPetId: "codex-custom:test-pet",
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
  } satisfies RuntimeClient;
}

describe("PetsSettingsPanel", () => {
  it("renders the pets settings shell", async () => {
    const client = createMockRuntimeClient();
    const html = renderToString(
      React.createElement(PetsSettingsPanel, { runtimeClient: client }),
    );

    expect(html).toContain("宠物");
    expect(html).toContain("唤醒宠物");
    expect(html).toContain("收起宠物");
    expect(html).toContain("刷新 Codex 宠物");
    expect(html).toContain("创建自己的宠物");
  });
});
