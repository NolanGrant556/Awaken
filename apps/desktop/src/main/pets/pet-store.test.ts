import { describe, expect, it, beforeEach, afterEach } from "vitest";
import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { PetStore, sha256String } from "./pet-store.js";

async function createTempDir(): Promise<string> {
  return fs.mkdtemp(path.join(os.tmpdir(), "awaken-pet-store-"));
}

async function createSpritesheet(dir: string, name = "spritesheet.webp"): Promise<string> {
  const file = path.join(dir, name);
  // A minimal valid WebP image (1x1 pixel, lossy).
  const webp = Buffer.from(
    "524946461a0000005745425056503820 0d0000009d012a0000fa00ba017d01",
    "hex",
  );
  await fs.writeFile(file, webp);
  return file;
}

describe("PetStore", () => {
  let tempDir: string;
  let configDir: string;
  let dataDir: string;
  let store: PetStore;

  beforeEach(async () => {
    tempDir = await createTempDir();
    configDir = path.join(tempDir, "config");
    dataDir = path.join(tempDir, "userData");
    store = new PetStore(configDir, dataDir);
  });

  afterEach(async () => {
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  it("initializes with default settings", async () => {
    await store.initialize();
    const settings = store.getSettings();
    expect(settings.selectedPetId).toBeNull();
    expect(settings.overlayOpen).toBe(false);
    expect(settings.mascotWidthPx).toBe(112);
  });

  it("does not open without a selected pet", async () => {
    await store.initialize();
    await expect(store.setOverlayOpen(true)).rejects.toThrow(
      "Select an available pet",
    );
  });

  it("imports a pet and lists it", async () => {
    await store.initialize();
    const spritesheet = await createSpritesheet(tempDir);
    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });

    const pets = store.listPets();
    expect(pets).toHaveLength(1);
    expect(pets[0]?.id).toBe("my-pet");
    expect(pets[0]?.available).toBe(true);
  });

  it("persists Codex v2 sprite version", async () => {
    await store.initialize();
    const spritesheet = await createSpritesheet(tempDir);
    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spriteVersionNumber: 2,
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });

    const store2 = new PetStore(configDir, dataDir);
    await store2.initialize();
    expect(store2.listPets()[0]?.spriteVersionNumber).toBe(2);
  });

  it("selecting a pet does not change on unrelated operations", async () => {
    await store.initialize();
    const spritesheet = await createSpritesheet(tempDir);
    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });

    await store.selectPet("my-pet");
    expect(store.getSettings().selectedPetId).toBe("my-pet");

    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "other-pet",
      displayName: "Other Pet",
      description: "Another pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/other-pet",
    });

    expect(store.getSettings().selectedPetId).toBe("my-pet");
  });

  it("skips re-import when fingerprint is unchanged", async () => {
    await store.initialize();
    const spritesheet = await createSpritesheet(tempDir);
    const first = await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });
    const second = await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });
    expect(first.updated).toBe(false);
    expect(second.updated).toBe(false);
  });

  it("updates metadata even when the spritesheet is unchanged", async () => {
    await store.initialize();
    const spritesheet = await createSpritesheet(tempDir);
    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "Old name",
      description: "Old description.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });
    const result = await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "New name",
      description: "New description.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });

    expect(result.updated).toBe(true);
    expect(store.listPets()[0]).toMatchObject({
      displayName: "New name",
      description: "New description.",
    });
  });

  it("persists settings across re-initialization", async () => {
    await store.initialize();
    const spritesheet = await createSpritesheet(tempDir);
    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });
    await store.selectPet("my-pet");
    await store.setOverlayOpen(true);
    await store.setImportPreferences({ importCodexPets: false });

    const store2 = new PetStore(configDir, dataDir);
    await store2.initialize();
    expect(store2.getSettings().overlayOpen).toBe(true);
    expect(store2.getSettings().importCodexPets).toBe(false);
  });

  it("removes catalog entry and clears selection", async () => {
    await store.initialize();
    const spritesheet = await createSpritesheet(tempDir);
    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });
    await store.selectPet("my-pet");
    await store.removeCatalogEntry("my-pet");

    expect(store.listPets()).toHaveLength(0);
    expect(store.getSettings().selectedPetId).toBeNull();
  });

  it("records welcomed pet ids", async () => {
    await store.initialize();
    await store.recordWelcomedPet("awaken:default");
    expect(store.getSettings().welcomedPetIds).toContain("awaken:default");

    const store2 = new PetStore(configDir, dataDir);
    await store2.initialize();
    expect(store2.getSettings().welcomedPetIds).toContain("awaken:default");
  });

  it("records dismissed notification keys", async () => {
    await store.initialize();
    await store.recordDismissedNotificationKeys(["run-1:turn-a", "run-2"]);
    expect(store.isNotificationDismissed("run-1:turn-a")).toBe(true);
    expect(store.isNotificationDismissed("run-2")).toBe(true);
    expect(store.isNotificationDismissed("run-3")).toBe(false);

    const store2 = new PetStore(configDir, dataDir);
    await store2.initialize();
    expect(store2.isNotificationDismissed("run-1:turn-a")).toBe(true);
  });

  it("creates a local pet template", async () => {
    await store.initialize();
    const result = await store.createLocalPet({
      sourceId: "my-local-pet",
      displayName: "My Local Pet",
      description: "A template pet.",
    });
    expect(result.petId).toBe("awaken-local:my-local-pet");

    const pets = store.listPets();
    const pet = pets.find((p) => p.id === result.petId);
    expect(pet).toBeDefined();
    expect(pet?.available).toBe(true);
  });
});

describe("sha256String", () => {
  it("returns consistent hashes", async () => {
    const a = await sha256String("hello");
    const b = await sha256String("hello");
    expect(a).toBe(b);
    expect(a).not.toBe(await sha256String("world"));
  });
});
