export type PetAnimationState =
  | "idle"
  | "running-right"
  | "running-left"
  | "waving"
  | "jumping"
  | "failed"
  | "waiting"
  | "running"
  | "review";

export type PetSource = "awaken-builtin" | "awaken-local" | "codex-custom";

export type PetManifest = {
  schemaVersion: 1;
  id: string;
  sourceId: string;
  displayName: string;
  description: string;
  spritesheetFile: string;
  source: PetSource;
  sourcePath?: string;
  sourceFingerprint?: string;
  importedAt?: string;
};

export type PetSummary = {
  id: string;
  sourceId: string;
  displayName: string;
  description: string;
  source: PetSource;
  spritesheetUrl?: string;
  available: boolean;
  warning?: string;
};

export type PetSettings = {
  schemaVersion: 1;
  selectedPetId: string | null;
  overlayOpen: boolean;
  mascotWidthPx: number;
  importCodexPets: boolean;
  welcomedPetIds: string[];
  dismissedNotificationKeys: string[];
  placementByDisplay: Record<string, PetOverlayPlacement>;
};

export type PetOverlayPlacement = {
  displayId: string;
  workArea: {
    x: number;
    y: number;
    width: number;
    height: number;
  };
  windowBounds: {
    x: number;
    y: number;
    width: number;
    height: number;
  };
  mascotWidthPx: number;
  updatedAt: string;
};

export type PetSessionStatus = "idle" | "running" | "waiting" | "failed" | "review";

export type PetQuestionOption = {
  id: string;
  label: string;
  description?: string;
};

export type PetWaitingRequest =
  | { type: "question"; requestId: string; prompt: string; options: PetQuestionOption[] }
  | { type: "command-approval"; requestId: string; summary: string }
  | { type: "file-approval"; requestId: string; files: string[] }
  | { type: "permission"; requestId: string; summary: string }
  | { type: "mcp-elicitation"; requestId: string; summary: string }
  | { type: "implement-plan"; turnId: string; summary: string };

export type PetSessionActivity = {
  key: string;
  status: PetSessionStatus;
  runId?: string;
  threadId?: string;
  turnKey?: string | null;
  title: string;
  subtitle?: string;
  updatedAtMs: number;
  waitingRequest?: PetWaitingRequest | null;
};

export type PetActivitySnapshot = {
  sessions: PetSessionActivity[];
  emittedAtMs: number;
};

export type PetImportReport = {
  imported: PetManifest[];
  updated: PetManifest[];
  sourceMissing: PetManifest[];
  failed: Array<{ sourcePath: string; sourceId?: string; error: string }>;
  conflicts: Array<{ sourcePath: string; sourceId: string; keptSourcePath: string }>;
  selectedPetIdUnchanged: string | null;
};

export type PetBridge = {
  listPets(): Promise<PetSummary[]>;
  getPetSettings(): Promise<PetSettings>;
  selectPet(petId: string): Promise<PetSettings>;
  refreshCodexPets(): Promise<PetImportReport>;
  createPet(): Promise<{ threadId: string }>;
  setPetOverlayOpen(open: boolean): Promise<PetSettings>;
  setPetImportPreferences(input: {
    importCodexPets?: boolean;
  }): Promise<PetSettings>;
  updatePetActivity(snapshot: PetActivitySnapshot): void;
  onPetCatalogChanged(callback: () => void): () => void;
  onPetThreadActivationRequested(callback: (threadId: string) => void): () => void;
  openAwakenPetsDirectory(): Promise<void>;
  openCodexPetsDirectory(): Promise<void>;
};

export type ScreenPoint = {
  x: number;
  y: number;
};

export type PetNotificationActionRequest =
  | { type: "answer-question"; runId: string; questionId: string; answer: string }
  | { type: "approve-command"; runId: string; approvalId: string }
  | { type: "deny-command"; runId: string; approvalId: string }
  | { type: "approve-file"; runId: string; approvalId: string }
  | { type: "deny-file"; runId: string; approvalId: string }
  | { type: "approve-permission"; runId: string; approvalId: string }
  | { type: "deny-permission"; runId: string; approvalId: string }
  | { type: "approve-mcp"; runId: string; approvalId: string }
  | { type: "deny-mcp"; runId: string; approvalId: string }
  | { type: "implement-plan"; runId: string; turnId: string }
  | { type: "stop-run"; runId: string }
  | { type: "follow-up"; threadId: string; prompt: string };

export type PetOverlayBootstrap = {
  petId: string | null;
  displayName: string | null;
  spritesheetUrl: string | null;
  mascotWidthPx: number;
  isOpen: boolean;
  welcomedPetIds: string[];
  dismissedNotificationKeys: string[];
};

export type PetOverlayBounds = {
  width: number;
  height: number;
};

export type PetOverlayBridge = {
  getBootstrap(): Promise<PetOverlayBootstrap>;
  onBootstrapChanged(callback: (value: PetOverlayBootstrap) => void): () => void;
  onActivityChanged(callback: (value: PetActivitySnapshot) => void): () => void;
  onMainWindowFocused(callback: () => void): () => void;
  beginDrag(point: ScreenPoint): void;
  moveDrag(point: ScreenPoint): void;
  endDrag(point: ScreenPoint): void;
  setPointerInteractive(interactive: boolean): void;
  resizeMascot(width: number): void;
  finishResize(width: number): void;
  reportBounds(bounds: PetOverlayBounds): void;
  activateNotification(notificationId: string): Promise<void>;
  performNotificationAction(input: PetNotificationActionRequest): Promise<void>;
  dismissNotification(input: { notificationId: string; turnKey: string | null }): Promise<void>;
  recordPetWelcomed(petId: string): Promise<void>;
  openMainWindow(): void;
  closeOverlay(): void;
};

export const PET_ROWS: Record<
  PetAnimationState,
  { row: number; frames: number; frameDurationsMs: number[] }
> = {
  idle: { row: 0, frames: 6, frameDurationsMs: [280, 110, 110, 140, 140, 320] },
  "running-right": {
    row: 1,
    frames: 8,
    frameDurationsMs: [120, 120, 120, 120, 120, 120, 120, 220],
  },
  "running-left": {
    row: 2,
    frames: 8,
    frameDurationsMs: [120, 120, 120, 120, 120, 120, 120, 220],
  },
  waving: { row: 3, frames: 4, frameDurationsMs: [140, 140, 140, 280] },
  jumping: { row: 4, frames: 5, frameDurationsMs: [140, 140, 140, 140, 280] },
  failed: { row: 5, frames: 8, frameDurationsMs: [140, 140, 140, 140, 140, 140, 140, 240] },
  waiting: { row: 6, frames: 6, frameDurationsMs: [150, 150, 150, 150, 150, 260] },
  running: { row: 7, frames: 6, frameDurationsMs: [120, 120, 120, 120, 120, 220] },
  review: { row: 8, frames: 6, frameDurationsMs: [150, 150, 150, 150, 150, 280] },
};

export const PET_SHEET_WIDTH = 1536;
export const PET_SHEET_HEIGHT = 1872;
export const PET_SHEET_COLUMNS = 8;
export const PET_SHEET_ROWS = 9;
export const PET_CELL_WIDTH = 192;
export const PET_CELL_HEIGHT = 208;

export const PET_MASCOT_MIN_WIDTH_PX = 80;
export const PET_MASCOT_DEFAULT_WIDTH_PX = 112;
export const PET_MASCOT_MAX_WIDTH_PX = 224;

export const PET_NOTIFICATION_TTL_MS: Record<Exclude<PetSessionStatus, "idle">, number> = {
  running: 3 * 60 * 1000,
  failed: 15 * 1000,
  waiting: 30 * 60 * 1000,
  review: 15 * 1000,
};

export const PET_FIRST_AWAKE_TTL_MS = 8_000;
export const PET_DRAG_THRESHOLD_PX = 4;

export const PET_SOURCE_NAMESPACE = {
  awakenBuiltin: "awaken",
  awakenLocal: "awaken-local",
  codexCustom: "codex-custom",
} as const;

export function petIdForSource(source: PetSource, sourceId: string): string {
  switch (source) {
    case "awaken-builtin":
      return `${PET_SOURCE_NAMESPACE.awakenBuiltin}:${sourceId}`;
    case "awaken-local":
      return `${PET_SOURCE_NAMESPACE.awakenLocal}:${sourceId}`;
    case "codex-custom":
      // Keep Codex IDs and folder names consistent with the source: no namespace prefix.
      return sourceId;
    default:
      return `${PET_SOURCE_NAMESPACE.awakenBuiltin}:${sourceId}`;
  }
}

export function petAssetUrl(petId: string, version?: string): string {
  const query = version ? `?v=${encodeURIComponent(version)}` : "";
  return `awaken-pet://asset/${encodeURIComponent(petId)}/spritesheet${query}`;
}

export const PET_IPC_CHANNELS = {
  listPets: "awaken:list-pets",
  getPetSettings: "awaken:get-pet-settings",
  selectPet: "awaken:select-pet",
  refreshCodexPets: "awaken:refresh-codex-pets",
  createPet: "awaken:create-pet",
  setPetOverlayOpen: "awaken:set-pet-overlay-open",
  setPetImportPreferences: "awaken:set-pet-import-preferences",
  updatePetActivity: "awaken:update-pet-activity",
  petCatalogChanged: "awaken:pet-catalog-changed",
  petThreadActivationRequested: "awaken:pet-thread-activation-requested",
  petActivityChanged: "awaken:pet-activity-changed",
  petMainWindowFocused: "awaken:pet-main-window-focused",
  petOverlayBootstrap: "awaken:pet-overlay-bootstrap",
  petPointerInteractive: "awaken:pet-pointer-interactive",
  petDragStart: "awaken:pet-drag-start",
  petDragMove: "awaken:pet-drag-move",
  petDragEnd: "awaken:pet-drag-end",
  petResizeMascot: "awaken:pet-resize-mascot",
  petFinishResize: "awaken:pet-finish-resize",
  petReportBounds: "awaken:pet-report-bounds",
  petActivateNotification: "awaken:pet-activate-notification",
  petPerformNotificationAction: "awaken:pet-perform-notification-action",
  petDismissNotification: "awaken:pet-dismiss-notification",
  petRecordPetWelcomed: "awaken:pet-record-pet-welcomed",
  petOpenMainWindow: "awaken:pet-open-main-window",
  petCloseOverlay: "awaken:pet-close-overlay",
  openAwakenPetsDirectory: "awaken:open-awaken-pets-directory",
  openCodexPetsDirectory: "awaken:open-codex-pets-directory",
} as const;
