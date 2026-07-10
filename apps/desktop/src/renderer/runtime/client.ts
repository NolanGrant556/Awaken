import type { AguiEvent } from "./agui";
import {
  type AttachmentPickResult,
  type AwakenBridge,
  type ConfigureMcpServersRequest,
  type ConfigureMcpServersResponse,
  type DesktopPreferences,
  type DesktopPreferencesSaveRequest,
  type DesktopPlatform,
  type InstallSkillRequest,
  type InstallSkillResponse,
  type MassMessageReviewDecision,
  type ModelProfileBalancesResult,
  type UserQuestionAnswer,
  type ModelProfileSaveRequest,
  type ModelProfileTestResult,
  type ModelProvider,
  type RuntimeConnector,
  type RuntimeConnectorActionResponse,
  type RuntimeInfo,
  type RuntimeLogEntry,
  type RuntimeMcpServer,
  type RuntimeMcpServerConfigResponse,
  type RuntimeModelProfileSummary,
  type RuntimeRun,
  type RuntimeSkill,
  type RuntimeThreadTranscript,
  type RuntimeThread,
  type RuntimeToolInfo,
  type StartRunRequest,
  type SupervisorStatusInfo,
  type UpdateMcpServerConfigRequest,
  type WorkspacePickResult,
  type WorkspaceRegistrySnapshot,
  type WorkspaceRegistryPersistRequest,
  type WorkspaceValidationResult,
} from "../../shared/runtime";
import type {
  PetActivitySnapshot,
  PetImportReport,
  PetSettings,
  PetSummary,
} from "../../shared/pets";

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
  listConnectors: () => Promise<RuntimeConnector[]>;
  connectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  disconnectConnector: (connectorId: string) => Promise<RuntimeConnectorActionResponse>;
  listMcpServers: () => Promise<RuntimeMcpServer[]>;
  configureMcpServers: (request: ConfigureMcpServersRequest) => Promise<ConfigureMcpServersResponse>;
  getMcpServerConfig: (name: string) => Promise<RuntimeMcpServerConfigResponse>;
  updateMcpServerConfig: (name: string, request: UpdateMcpServerConfigRequest) => Promise<ConfigureMcpServersResponse>;
  getRuntimeLogs: () => Promise<RuntimeLogEntry[]>;
  disableMcpServer: (name: string) => Promise<void>;
  enableMcpServer: (name: string) => Promise<void>;
  deleteMcpServer: (name: string) => Promise<void>;
  listMcpTools: () => Promise<RuntimeToolInfo[]>;
  listSkills: (projectRoot?: string) => Promise<RuntimeSkill[]>;
  installSkill: (request: InstallSkillRequest) => Promise<InstallSkillResponse>;
  listModelProviders: () => Promise<ModelProvider[]>;
  listModelProfiles: () => Promise<RuntimeModelProfileSummary[]>;
  saveModelProfile: (request: ModelProfileSaveRequest) => Promise<RuntimeModelProfileSummary>;
  deleteModelProfile: (profileId: string) => Promise<void>;
  setDefaultModelProfile: (profileId: string) => Promise<RuntimeModelProfileSummary>;
  testModelProfile: (request: ModelProfileSaveRequest) => Promise<ModelProfileTestResult>;
  syncModelCapabilities: () => Promise<ModelProvider[]>;
  getModelProfileBalances: () => Promise<ModelProfileBalancesResult>;
  getRun: (runId: string) => Promise<RuntimeRun>;
  getThreadTranscript: (threadId: string) => Promise<RuntimeThreadTranscript>;
  listThreads: () => Promise<RuntimeThread[]>;
  renameThread: (threadId: string, label: string) => Promise<RuntimeThread>;
  deleteThread: (threadId: string) => Promise<void>;
  startRun: (request: StartRunRequest) => Promise<RuntimeRun>;
  cancelRun: (runId: string) => Promise<RuntimeRun>;
  resolveMassMessageReview: (
    runId: string,
    reviewId: string,
    decision: MassMessageReviewDecision,
  ) => Promise<void>;
  answerUserQuestion: (
    runId: string,
    questionId: string,
    answer: string,
  ) => Promise<void>;
  getDesktopPreferences: () => Promise<DesktopPreferences>;
  saveDesktopPreferences: (request: DesktopPreferencesSaveRequest) => Promise<DesktopPreferences>;
  subscribeRun: (
    runId: string,
    handlers: SubscribeHandlers,
    afterSeq?: number,
  ) => Promise<() => void>;
  pickAttachments: (defaultPath?: string) => Promise<AttachmentPickResult>;
  pickDirectory: (defaultPath?: string) => Promise<WorkspacePickResult>;
  pickWorkspace: (defaultPath?: string) => Promise<WorkspacePickResult>;
  validateWorkspace: (path: string) => Promise<WorkspaceValidationResult>;
  readWorkspaceRegistry: () => Promise<WorkspaceRegistrySnapshot>;
  persistWorkspaceRegistry: (request: WorkspaceRegistryPersistRequest) => Promise<void>;
  getSupervisorStatus: () => Promise<SupervisorStatusInfo>;
  onSupervisorStatusChange: (callback: (info: SupervisorStatusInfo) => void) => () => void;
  notifyRendererReady: () => void;
  listPets: () => Promise<PetSummary[]>;
  getPetSettings: () => Promise<PetSettings>;
  selectPet: (petId: string) => Promise<PetSettings>;
  refreshCodexPets: () => Promise<PetImportReport>;
  createPet: () => Promise<{ threadId: string }>;
  setPetOverlayOpen: (open: boolean) => Promise<PetSettings>;
  setPetImportPreferences: (input: { importCodexPets?: boolean }) => Promise<PetSettings>;
  updatePetActivity: (snapshot: PetActivitySnapshot) => void;
  onPetCatalogChanged: (callback: () => void) => () => void;
  onPetThreadActivationRequested: (callback: (threadId: string) => void) => () => void;
  openAwakenPetsDirectory: () => Promise<void>;
  openCodexPetsDirectory: () => Promise<void>;
};

const MAX_RECONNECT_RETRIES = 5;

function reconnectDelay(retryCount: number): number {
  return Math.min(1000 * Math.pow(2, retryCount), 10000);
}

function isTerminalEvent(event: AguiEvent): boolean {
  return event.type === "RUN_FINISHED" || event.type === "RUN_ERROR";
}

function isRunNotFoundError(message: string): boolean {
  const normalized = message.toLowerCase();
  return (
    normalized.includes("run_not_found") ||
    normalized.includes("404 page not found") ||
    normalized.includes("404 not found")
  );
}

function createElectronClient(bridge: AwakenBridge): RuntimeClient {
  return {
    source: "electron",
    platform: bridge.platform,
    getRuntimeInfo: () => bridge.getRuntimeInfo(),
    listConnectors: () => bridge.listConnectors(),
    connectConnector: (connectorId) => bridge.connectConnector(connectorId),
    disconnectConnector: (connectorId) => bridge.disconnectConnector(connectorId),
    listMcpServers: () => bridge.listMcpServers(),
    configureMcpServers: (request) => bridge.configureMcpServers(request),
    getMcpServerConfig: (name) => bridge.getMcpServerConfig(name),
    updateMcpServerConfig: (name, request) => bridge.updateMcpServerConfig(name, request),
    getRuntimeLogs: () => bridge.getRuntimeLogs(),
    disableMcpServer: (name) => bridge.disableMcpServer(name),
    enableMcpServer: (name) => bridge.enableMcpServer(name),
    deleteMcpServer: (name) => bridge.deleteMcpServer(name),
    listMcpTools: () => bridge.listMcpTools(),
    listSkills: (projectRoot) => bridge.listSkills(projectRoot),
    installSkill: (request) => bridge.installSkill(request),
    listModelProviders: () => bridge.listModelProviders(),
    listModelProfiles: () => bridge.listModelProfiles(),
    saveModelProfile: (request) => bridge.saveModelProfile(request),
    deleteModelProfile: (profileId) => bridge.deleteModelProfile(profileId),
    setDefaultModelProfile: (profileId) => bridge.setDefaultModelProfile(profileId),
    testModelProfile: (request) => bridge.testModelProfile(request),
    syncModelCapabilities: () => bridge.syncModelCapabilities(),
    getModelProfileBalances: () => bridge.getModelProfileBalances(),
    getRun: (runId) => bridge.getRun(runId),
    getThreadTranscript: (threadId) => bridge.getThreadTranscript(threadId),
    listThreads: () => bridge.listThreads(),
    renameThread: (threadId, label) => bridge.renameThread(threadId, label),
    deleteThread: (threadId) => bridge.deleteThread(threadId),
    startRun: (request) => bridge.startRun(request),
    cancelRun: bridge.cancelRun,
    resolveMassMessageReview: bridge.resolveMassMessageReview,
    answerUserQuestion: bridge.answerUserQuestion,
    getDesktopPreferences: () => bridge.getDesktopPreferences(),
    saveDesktopPreferences: (request) => bridge.saveDesktopPreferences(request),
    subscribeRun: async (runId, handlers, afterCursor) => {
      let lastCursor = afterCursor ?? 0;
      let terminalSeen = false;
      let retryCount = 0;
      let currentSubscriptionId: string | null = null;
      let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
      let disposed = false;

      const wrappedHandlers = {
        onEvent: (event: AguiEvent) => {
          const cursor = event.streamCursor ?? event.seq;
          if (cursor !== undefined && cursor > lastCursor) {
            lastCursor = cursor;
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
        if (isRunNotFoundError(reason)) {
          handlers.onError?.(
            `Run session was not found on the server (it may have been lost after restart): ${reason}`,
          );
          return;
        }
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
            lastCursor,
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
          lastCursor,
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
    pickAttachments: (defaultPath) => bridge.pickAttachments(defaultPath),
    pickDirectory: (defaultPath) => bridge.pickDirectory(defaultPath),
    pickWorkspace: (defaultPath) => bridge.pickWorkspace(defaultPath),
    validateWorkspace: (path) => bridge.validateWorkspace(path),
    readWorkspaceRegistry: () => bridge.readWorkspaceRegistry(),
    persistWorkspaceRegistry: (request) => bridge.persistWorkspaceRegistry(request),
    getSupervisorStatus: () => bridge.getSupervisorStatus(),
    onSupervisorStatusChange: (callback) => bridge.onSupervisorStatusChange(callback),
    notifyRendererReady: () => bridge.notifyRendererReady(),
    listPets: () => bridge.listPets(),
    getPetSettings: () => bridge.getPetSettings(),
    selectPet: (petId) => bridge.selectPet(petId),
    refreshCodexPets: () => bridge.refreshCodexPets(),
    createPet: () => bridge.createPet(),
    setPetOverlayOpen: (open) => bridge.setPetOverlayOpen(open),
    setPetImportPreferences: (input) => bridge.setPetImportPreferences(input),
    updatePetActivity: (snapshot) => bridge.updatePetActivity(snapshot),
    onPetCatalogChanged: (callback) => bridge.onPetCatalogChanged(callback),
    onPetThreadActivationRequested: (callback) =>
      bridge.onPetThreadActivationRequested(callback),
    openAwakenPetsDirectory: () => bridge.openAwakenPetsDirectory(),
    openCodexPetsDirectory: () => bridge.openCodexPetsDirectory(),
  };
}

export function createRuntimeClient(): RuntimeClient {
  if (!window.awaken) {
    throw new Error("Awaken Desktop must run through the Electron window.awaken bridge");
  }
  return createElectronClient(window.awaken);
}
