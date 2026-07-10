import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import {
  PET_MASCOT_DEFAULT_WIDTH_PX,
  PET_MASCOT_MAX_WIDTH_PX,
  PET_MASCOT_MIN_WIDTH_PX,
  type PetImportReport,
  type PetManifest,
  type PetOverlayPlacement,
  type PetSettings,
  type PetSpriteVersion,
  type PetSource,
  type PetSummary,
  petAssetUrl,
  petIdForSource,
} from "../../shared/pets.js";
import { createTransparentSpritesheet } from "./png-utils.js";

const PETS_CONFIG_FILE = "pets.json";
const PETS_INDEX_FILE = "index.json";
const PETS_DATA_DIR = path.join("data", "pets");
const MAX_DISMISSED_NOTIFICATION_KEYS = 500;

const DEFAULT_PET_SETTINGS: PetSettings = {
  schemaVersion: 1,
  selectedPetId: null,
  overlayOpen: false,
  mascotWidthPx: PET_MASCOT_DEFAULT_WIDTH_PX,
  importCodexPets: true,
  welcomedPetIds: [],
  dismissedNotificationKeys: [],
  placementByDisplay: {},
};

function normalizeSettings(value: unknown): PetSettings {
  if (!value || typeof value !== "object") {
    return { ...DEFAULT_PET_SETTINGS };
  }

  const candidate = value as Record<string, unknown>;
  return {
    schemaVersion: 1,
    selectedPetId:
      typeof candidate.selectedPetId === "string" ? candidate.selectedPetId : null,
    overlayOpen: candidate.overlayOpen === true,
    mascotWidthPx: clampMascotWidth(candidate.mascotWidthPx),
    importCodexPets: candidate.importCodexPets !== false,
    welcomedPetIds: Array.isArray(candidate.welcomedPetIds)
      ? Array.from(
          new Set(
            candidate.welcomedPetIds.filter(
              (item): item is string => typeof item === "string",
            ),
          ),
        )
      : [],
    dismissedNotificationKeys: Array.isArray(candidate.dismissedNotificationKeys)
      ? candidate.dismissedNotificationKeys
          .filter((item): item is string => typeof item === "string")
          .slice(-MAX_DISMISSED_NOTIFICATION_KEYS)
      : [],
    placementByDisplay:
      candidate.placementByDisplay && typeof candidate.placementByDisplay === "object" && !Array.isArray(candidate.placementByDisplay)
        ? (candidate.placementByDisplay as Record<string, PetOverlayPlacement>)
        : {},
  };
}

function normalizeManifest(value: unknown): PetManifest | null {
  if (!value || typeof value !== "object") return null;
  const candidate = value as Record<string, unknown>;
  if (
    typeof candidate.id !== "string" ||
    typeof candidate.sourceId !== "string" ||
    typeof candidate.displayName !== "string" ||
    typeof candidate.description !== "string" ||
    typeof candidate.spritesheetFile !== "string" ||
    typeof candidate.source !== "string"
  ) {
    return null;
  }
  const source = candidate.source;
  if (
    source !== "awaken-builtin" &&
    source !== "awaken-local" &&
    source !== "codex-custom"
  ) {
    return null;
  }
  const expectedId = petIdForSource(source, candidate.sourceId);
  if (candidate.id !== expectedId) {
    return null;
  }
  const spritesheetFile = candidate.spritesheetFile;
  if (
    spritesheetFile !== "spritesheet.png" &&
    spritesheetFile !== "spritesheet.webp"
  ) {
    return null;
  }
  const spriteVersionNumber =
    candidate.spriteVersionNumber === 2 ? 2 : 1;
  return {
    schemaVersion: 1,
    id: candidate.id,
    sourceId: candidate.sourceId,
    displayName: candidate.displayName,
    description: candidate.description,
    spriteVersionNumber,
    spritesheetFile,
    source,
    sourcePath: typeof candidate.sourcePath === "string" ? candidate.sourcePath : undefined,
    sourceFingerprint: typeof candidate.sourceFingerprint === "string" ? candidate.sourceFingerprint : undefined,
    importedAt: typeof candidate.importedAt === "string" ? candidate.importedAt : undefined,
  };
}

export async function sha256File(filePath: string): Promise<string> {
  const hash = createHash("sha256");
  const data = await fs.readFile(filePath);
  hash.update(data);
  return hash.digest("hex");
}

export async function sha256String(value: string): Promise<string> {
  const hash = createHash("sha256");
  hash.update(value, "utf8");
  return hash.digest("hex");
}

export class PetStore {
  private settings: PetSettings = { ...DEFAULT_PET_SETTINGS };
  private catalog: Map<string, PetManifest> = new Map();
  private readonly configFilePath: string;
  private readonly dataDir: string;
  private readonly indexFilePath: string;

  constructor(configDir: string, userDataDir: string) {
    this.configFilePath = path.join(configDir, PETS_CONFIG_FILE);
    this.dataDir = path.join(userDataDir, PETS_DATA_DIR);
    this.indexFilePath = path.join(this.dataDir, PETS_INDEX_FILE);
  }

  async initialize(): Promise<void> {
    await fs.mkdir(this.dataDir, { recursive: true });
    this.settings = await this.readSettings();
    this.catalog = await this.readCatalog();
    if (
      this.settings.selectedPetId &&
      !this.catalog.has(this.settings.selectedPetId)
    ) {
      this.settings.selectedPetId = null;
      this.settings.overlayOpen = false;
      await this.writeSettings();
    }
  }

  snapshot(): PetSettings {
    return structuredClone(this.settings);
  }

  listPets(): PetSummary[] {
    const sourceOrder: Record<PetSource, number> = {
      "awaken-builtin": 0,
      "awaken-local": 1,
      "codex-custom": 2,
    };
    return Array.from(this.catalog.values())
      .map((manifest) => this.toSummary(manifest))
      .sort((a, b) => {
        const aSelected = a.id === this.settings.selectedPetId ? 0 : 1;
        const bSelected = b.id === this.settings.selectedPetId ? 0 : 1;
        if (aSelected !== bSelected) return aSelected - bSelected;
        if (sourceOrder[a.source] !== sourceOrder[b.source]) {
          return sourceOrder[a.source] - sourceOrder[b.source];
        }
        return a.displayName.localeCompare(b.displayName);
      });
  }

  getAllManifests(): PetManifest[] {
    return Array.from(this.catalog.values());
  }

  getSettings(): PetSettings {
    return this.snapshot();
  }

  async selectPet(petId: string | null): Promise<PetSettings> {
    if (petId !== null && !this.catalog.has(petId)) {
      throw new Error("Pet not found");
    }
    this.settings.selectedPetId = petId;
    await this.writeSettings();
    return this.snapshot();
  }

  async setOverlayOpen(open: boolean): Promise<PetSettings> {
    if (open && !this.getSelectedPet()) {
      throw new Error("Select an available pet before opening the overlay");
    }
    this.settings.overlayOpen = open;
    await this.writeSettings();
    return this.snapshot();
  }

  async setImportPreferences(input: { importCodexPets?: boolean }): Promise<PetSettings> {
    if (typeof input.importCodexPets === "boolean") {
      this.settings.importCodexPets = input.importCodexPets;
    }
    await this.writeSettings();
    return this.snapshot();
  }

  async recordWelcomedPet(petId: string): Promise<void> {
    if (!this.settings.welcomedPetIds.includes(petId)) {
      this.settings.welcomedPetIds.push(petId);
      await this.writeSettings();
    }
  }

  async clearWelcomedPet(petId: string): Promise<void> {
    this.settings.welcomedPetIds = this.settings.welcomedPetIds.filter((id) => id !== petId);
    await this.writeSettings();
  }

  async recordDismissedNotificationKeys(keys: string[]): Promise<void> {
    let changed = false;
    for (const key of keys) {
      if (!this.settings.dismissedNotificationKeys.includes(key)) {
        this.settings.dismissedNotificationKeys.push(key);
        changed = true;
      }
    }
    if (changed) {
      this.settings.dismissedNotificationKeys =
        this.settings.dismissedNotificationKeys.slice(
          -MAX_DISMISSED_NOTIFICATION_KEYS,
        );
      await this.writeSettings();
    }
  }

  isNotificationDismissed(key: string): boolean {
    return this.settings.dismissedNotificationKeys.includes(key);
  }

  async clearDismissedNotificationKeys(keys?: string[]): Promise<void> {
    if (!keys || keys.length === 0) {
      this.settings.dismissedNotificationKeys = [];
    } else {
      const toRemove = new Set(keys);
      this.settings.dismissedNotificationKeys = this.settings.dismissedNotificationKeys.filter(
        (key) => !toRemove.has(key),
      );
    }
    await this.writeSettings();
  }

  async setPlacement(placement: PetOverlayPlacement): Promise<void> {
    this.settings.placementByDisplay[placement.displayId] = placement;
    this.settings.mascotWidthPx = placement.mascotWidthPx;
    await this.writeSettings();
  }

  async setMascotWidthPx(width: number): Promise<void> {
    this.settings.mascotWidthPx = clampMascotWidth(width);
    await this.writeSettings();
  }

  async createLocalPet(input: {
    sourceId: string;
    displayName: string;
    description: string;
  }): Promise<{ petId: string; threadId: string }> {
    const sourceId = sanitizePetId(input.sourceId);
    if (!sourceId) {
      throw new Error("宠物 id 不合法");
    }
    const stagingDir = path.join(this.dataDir, `.create-${randomUUID()}`);
    const spritesheetPath = path.join(stagingDir, "spritesheet.png");
    const manifestPath = path.join(stagingDir, "pet.json");

    const manifest = {
      id: sourceId,
      displayName: input.displayName,
      description: input.description,
      spritesheetPath: "spritesheet.png",
    };

    await fs.mkdir(stagingDir, { recursive: true });
    await fs.writeFile(spritesheetPath, createTransparentSpritesheet(), "binary");
    await fs.writeFile(
      manifestPath,
      `${JSON.stringify(manifest, null, 2)}\n`,
      "utf8",
    );

    const result = await this.importOrUpdatePet({
      source: "awaken-local",
      sourceId,
      displayName: input.displayName,
      description: input.description,
      spritesheetSourcePath: spritesheetPath,
      sourcePath: stagingDir,
    });

    await fs.rm(stagingDir, { recursive: true, force: true });

    return { petId: result.manifest.id, threadId: randomUUID() };
  }

  async removePlacement(displayId: string): Promise<void> {
    delete this.settings.placementByDisplay[displayId];
    await this.writeSettings();
  }

  getSelectedPet(): PetManifest | null {
    if (!this.settings.selectedPetId) return null;
    return this.catalog.get(this.settings.selectedPetId) ?? null;
  }

  getPetAssetPath(petId: string): string | null {
    const manifest = this.catalog.get(petId);
    if (!manifest) return null;
    const petDir = path.resolve(this.petDir(petId));
    const assetPath = path.resolve(petDir, manifest.spritesheetFile);
    if (!assetPath.startsWith(`${petDir}${path.sep}`)) {
      return null;
    }
    return assetPath;
  }

  async importOrUpdatePet(
    input: {
      source: PetSource;
      sourceId: string;
      displayName: string;
      description: string;
      spriteVersionNumber?: PetSpriteVersion;
      spritesheetSourcePath: string;
      sourcePath: string;
    },
  ): Promise<{ manifest: PetManifest; updated: boolean }> {
    const petId = petIdForSource(input.source, input.sourceId);
    const imageFingerprint = await sha256File(input.spritesheetSourcePath);
    const newFingerprint = await sha256String(
      JSON.stringify({
        imageFingerprint,
        source: input.source,
        sourceId: input.sourceId,
        displayName: input.displayName,
        description: input.description,
        spriteVersionNumber: input.spriteVersionNumber ?? 1,
        extension: path.extname(input.spritesheetSourcePath).toLowerCase(),
      }),
    );

    const existing = this.catalog.get(petId);
    if (existing && existing.sourceFingerprint === newFingerprint) {
      if (
        existing.sourcePath !== input.sourcePath ||
        existing.displayName !== input.displayName ||
        existing.description !== input.description
      ) {
        const restored: PetManifest = {
          ...existing,
          displayName: input.displayName,
          description: input.description,
          spriteVersionNumber: existing.spriteVersionNumber,
          sourcePath: input.sourcePath,
          importedAt: new Date().toISOString(),
        };
        this.catalog.set(petId, restored);
        await this.writeCatalog();
        return { manifest: restored, updated: true };
      }
      return { manifest: existing, updated: false };
    }

    const ext = path.extname(input.spritesheetSourcePath);
    const manifest: PetManifest = {
      schemaVersion: 1,
      id: petId,
      sourceId: input.sourceId,
      displayName: input.displayName,
      description: input.description,
      spriteVersionNumber: input.spriteVersionNumber ?? 1,
      spritesheetFile: `spritesheet${ext}`,
      source: input.source,
      sourcePath: input.sourcePath,
      sourceFingerprint: newFingerprint,
      importedAt: new Date().toISOString(),
    };

    const targetDir = this.petDir(petId);
    const stagingDir = path.join(this.dataDir, `.import-${randomUUID()}`);

    await fs.mkdir(stagingDir, { recursive: true });
    await fs.copyFile(input.spritesheetSourcePath, path.join(stagingDir, manifest.spritesheetFile));
    await fs.writeFile(
      path.join(stagingDir, "pet.json"),
      `${JSON.stringify(manifest, null, 2)}\n`,
      "utf8",
    );

    const backupDir = `${targetDir}.backup-${randomUUID()}`;
    try {
      if (existing && (await pathExists(targetDir))) {
        await fs.rename(targetDir, backupDir);
      }
      await fs.rename(stagingDir, targetDir);
      await fs.rm(backupDir, { recursive: true, force: true });
    } catch (error) {
      if (await pathExists(backupDir)) {
        await fs.rm(targetDir, { recursive: true, force: true });
        await fs.rename(backupDir, targetDir);
      }
      await fs.rm(stagingDir, { recursive: true, force: true });
      throw new Error(
        `Failed to replace pet directory: ${
          error instanceof Error ? error.message : "unknown error"
        }`,
      );
    }

    this.catalog.set(petId, manifest);
    await this.writeCatalog();

    return { manifest, updated: Boolean(existing) };
  }

  async markSourceMissing(petId: string): Promise<void> {
    const existing = this.catalog.get(petId);
    if (!existing) return;
    // Keep the local copy but update sourcePath to indicate source is gone.
    const updated: PetManifest = {
      ...existing,
      sourcePath: `${existing.sourcePath}#source-missing`,
      importedAt: new Date().toISOString(),
    };
    this.catalog.set(petId, updated);
    await this.writeCatalog();
  }

  async removeCatalogEntry(petId: string): Promise<void> {
    this.catalog.delete(petId);
    await this.writeCatalog();
    const targetDir = this.petDir(petId);
    await fs.rm(targetDir, { recursive: true, force: true });
    if (this.settings.selectedPetId === petId) {
      this.settings.selectedPetId = null;
      await this.writeSettings();
    }
  }

  buildImportReport(): PetImportReport {
    const imported: PetManifest[] = [];
    const updated: PetManifest[] = [];
    const sourceMissing: PetManifest[] = [];

    for (const manifest of this.catalog.values()) {
      if (manifest.sourcePath?.endsWith("#source-missing")) {
        sourceMissing.push(manifest);
      } else if (manifest.importedAt && new Date(manifest.importedAt).getTime() > Date.now() - 60_000) {
        imported.push(manifest);
      } else {
        updated.push(manifest);
      }
    }

    return {
      imported,
      updated,
      sourceMissing,
      failed: [],
      conflicts: [],
      selectedPetIdUnchanged: this.settings.selectedPetId,
    };
  }

  private petDir(petId: string): string {
    return path.join(this.dataDir, petId.replace(/:/g, "_"));
  }

  private toSummary(manifest: PetManifest): PetSummary {
    const sourceMissing = manifest.sourcePath?.endsWith("#source-missing") ?? false;
    return {
      id: manifest.id,
      sourceId: manifest.sourceId,
      displayName: manifest.displayName,
      description: manifest.description,
      source: manifest.source,
      spriteVersionNumber: manifest.spriteVersionNumber,
      spritesheetUrl: petAssetUrl(manifest.id, manifest.sourceFingerprint),
      available: true,
      warning: sourceMissing ? "Codex 源包已缺失，正在使用 Awaken 副本" : undefined,
    };
  }

  private async readSettings(): Promise<PetSettings> {
    try {
      const text = await fs.readFile(this.configFilePath, "utf8");
      return normalizeSettings(JSON.parse(text));
    } catch {
      return { ...DEFAULT_PET_SETTINGS };
    }
  }

  private async writeSettings(): Promise<void> {
    await fs.mkdir(path.dirname(this.configFilePath), { recursive: true });
    await fs.writeFile(
      this.configFilePath,
      `${JSON.stringify(this.settings, null, 2)}\n`,
      "utf8",
    );
  }

  private async readCatalog(): Promise<Map<string, PetManifest>> {
    const catalog = new Map<string, PetManifest>();
    let entries: string[] = [];
    try {
      const text = await fs.readFile(this.indexFilePath, "utf8");
      const parsed = JSON.parse(text) as unknown;
      if (Array.isArray(parsed)) {
        for (const item of parsed) {
          const manifest = normalizeManifest(item);
          if (manifest) {
            catalog.set(manifest.id, manifest);
          }
        }
      }
    } catch {
      // If index is missing, fall back to scanning directories.
    }

    if (catalog.size === 0) {
      try {
        entries = await fs.readdir(this.dataDir);
      } catch {
        entries = [];
      }
      for (const entry of entries) {
        if (entry.startsWith(".")) continue;
        const petJsonPath = path.join(this.dataDir, entry, "pet.json");
        try {
          const text = await fs.readFile(petJsonPath, "utf8");
          const manifest = normalizeManifest(JSON.parse(text));
          if (manifest) {
            catalog.set(manifest.id, manifest);
          }
        } catch {
          // Ignore unreadable directories.
        }
      }
    }

    for (const [petId, manifest] of catalog) {
      const petDir = path.resolve(this.petDir(petId));
      const assetPath = path.resolve(petDir, manifest.spritesheetFile);
      if (
        !assetPath.startsWith(`${petDir}${path.sep}`) ||
        !(await pathExists(assetPath))
      ) {
        catalog.delete(petId);
      }
    }

    return catalog;
  }

  private async writeCatalog(): Promise<void> {
    const list = Array.from(this.catalog.values()).sort((a, b) =>
      a.id.localeCompare(b.id),
    );
    await fs.mkdir(path.dirname(this.indexFilePath), { recursive: true });
    await fs.writeFile(
      this.indexFilePath,
      `${JSON.stringify(list, null, 2)}\n`,
      "utf8",
    );
  }
}

function clampMascotWidth(value: unknown): number {
  const width =
    typeof value === "number" && Number.isFinite(value)
      ? value
      : PET_MASCOT_DEFAULT_WIDTH_PX;
  return Math.round(
    Math.max(PET_MASCOT_MIN_WIDTH_PX, Math.min(PET_MASCOT_MAX_WIDTH_PX, width)),
  );
}

async function pathExists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

function sanitizePetId(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}
