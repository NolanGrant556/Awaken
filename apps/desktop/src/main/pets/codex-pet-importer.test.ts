import { describe, expect, it, beforeEach, afterEach } from "vitest";
import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { PetStore } from "./pet-store.js";
import { detectCodexHome, scanCodexCustomPets, importCodexPets } from "./codex-pet-importer.js";
import { createTransparentSpritesheet } from "./png-utils.js";

async function createTempDir(): Promise<string> {
  return fs.mkdtemp(path.join(os.tmpdir(), "awaken-codex-import-"));
}

async function createValidSpritesheet(
  dir: string,
  name = "spritesheet.png",
  spriteVersionNumber: 1 | 2 = 1,
): Promise<string> {
  const file = path.join(dir, name);
  await fs.writeFile(file, createTransparentSpritesheet(spriteVersionNumber));
  return file;
}

async function writePetJson(
  dir: string,
  manifest: Record<string, string | number>,
): Promise<void> {
  await fs.writeFile(path.join(dir, "pet.json"), JSON.stringify(manifest), "utf8");
}

describe("detectCodexHome", () => {
  it("uses CODEX_HOME when set", () => {
    const original = process.env.CODEX_HOME;
    process.env.CODEX_HOME = "/custom/codex";
    expect(detectCodexHome("/home/user")).toBe(path.normalize("/custom/codex"));
    if (original === undefined) {
      delete process.env.CODEX_HOME;
    } else {
      process.env.CODEX_HOME = original;
    }
  });

  it("falls back to home/.codex", () => {
    delete process.env.CODEX_HOME;
    expect(detectCodexHome("/home/user")).toBe(path.join("/home/user", ".codex"));
  });
});

describe("scanCodexCustomPets", () => {
  let codexHome: string;

  beforeEach(async () => {
    codexHome = await createTempDir();
    await fs.mkdir(path.join(codexHome, "pets"), { recursive: true });
  });

  afterEach(async () => {
    await fs.rm(codexHome, { recursive: true, force: true });
  });

  it("returns empty when pets dir is missing", async () => {
    const emptyHome = await createTempDir();
    const result = await scanCodexCustomPets(emptyHome);
    expect(result).toEqual([]);
    await fs.rm(emptyHome, { recursive: true, force: true });
  });

  it("ignores cache and avatars directories", async () => {
    await fs.mkdir(path.join(codexHome, "pets", "cache"), { recursive: true });
    await fs.mkdir(path.join(codexHome, "pets", "avatars"), { recursive: true });
    await fs.mkdir(path.join(codexHome, "pets", "my-pet"), { recursive: true });
    await writePetJson(path.join(codexHome, "pets", "my-pet"), {
      id: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetPath: "spritesheet.png",
    });
    const result = await scanCodexCustomPets(codexHome);
    expect(result).toHaveLength(1);
    expect(result[0]?.folderName).toBe("my-pet");
  });

  it("does not recursively scan", async () => {
    await fs.mkdir(path.join(codexHome, "pets", "nested", "inner"), { recursive: true });
    await writePetJson(path.join(codexHome, "pets", "nested", "inner"), {
      id: "inner",
      displayName: "Inner",
      description: "Inner.",
      spritesheetPath: "spritesheet.png",
    });
    const result = await scanCodexCustomPets(codexHome);
    expect(result).toHaveLength(0);
  });
});

describe("importCodexPets", () => {
  let tempDir: string;
  let codexHome: string;
  let store: PetStore;

  beforeEach(async () => {
    tempDir = await createTempDir();
    codexHome = path.join(tempDir, "codex");
    const configDir = path.join(tempDir, "config");
    const dataDir = path.join(tempDir, "userData");
    store = new PetStore(configDir, dataDir);
    await store.initialize();
  });

  afterEach(async () => {
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  it("imports multiple valid custom pets", async () => {
    for (const name of ["alpha", "beta"]) {
      const petDir = path.join(codexHome, "pets", name);
      await fs.mkdir(petDir, { recursive: true });
      await createValidSpritesheet(petDir);
      await writePetJson(petDir, {
        id: name,
        displayName: name,
        description: `${name} pet.`,
        spritesheetPath: "spritesheet.png",
      });
    }

    const report = await importCodexPets(store, codexHome);
    expect(report.imported).toHaveLength(2);
    expect(report.failed).toHaveLength(0);
    expect(store.listPets()).toHaveLength(2);
  });

  it("imports Codex v2 custom pets", async () => {
    const petDir = path.join(codexHome, "pets", "v2-pet");
    await fs.mkdir(petDir, { recursive: true });
    await createValidSpritesheet(petDir, "spritesheet.png", 2);
    await writePetJson(petDir, {
      id: "v2-pet",
      displayName: "V2 Pet",
      description: "A v2 pet.",
      spriteVersionNumber: 2,
      spritesheetPath: "spritesheet.png",
    });

    const report = await importCodexPets(store, codexHome);
    expect(report.failed).toHaveLength(0);
    expect(report.imported[0]?.spriteVersionNumber).toBe(2);
    expect(store.listPets()[0]?.spriteVersionNumber).toBe(2);
  });

  it("keeps the Awaken copy available when the Codex source is removed", async () => {
    const petDir = path.join(codexHome, "pets", "gone");
    await fs.mkdir(petDir, { recursive: true });
    await createValidSpritesheet(petDir);
    await writePetJson(petDir, {
      id: "gone",
      displayName: "Gone",
      description: "Will be removed.",
      spritesheetPath: "spritesheet.png",
    });

    await importCodexPets(store, codexHome);
    expect(store.listPets()[0]?.available).toBe(true);

    await fs.rm(petDir, { recursive: true, force: true });
    await importCodexPets(store, codexHome);

    const pets = store.listPets();
    expect(pets).toHaveLength(1);
    expect(pets[0]?.available).toBe(true);
    expect(pets[0]?.warning).toContain("Awaken 副本");
  });

  it("clears the missing-source warning when the Codex package returns", async () => {
    const petDir = path.join(codexHome, "pets", "returning");
    await fs.mkdir(petDir, { recursive: true });
    await createValidSpritesheet(petDir);
    await writePetJson(petDir, {
      id: "returning",
      displayName: "Returning",
      description: "Comes back.",
      spritesheetPath: "spritesheet.png",
    });

    await importCodexPets(store, codexHome);
    await fs.rm(petDir, { recursive: true, force: true });
    await importCodexPets(store, codexHome);
    expect(store.listPets()[0]?.warning).toBeDefined();

    await fs.mkdir(petDir, { recursive: true });
    await createValidSpritesheet(petDir);
    await writePetJson(petDir, {
      id: "returning",
      displayName: "Returning",
      description: "Comes back.",
      spritesheetPath: "spritesheet.png",
    });
    await importCodexPets(store, codexHome);

    expect(store.listPets()[0]?.warning).toBeUndefined();
  });

  it("does not change selectedPetId", async () => {
    const petDir = path.join(codexHome, "pets", "only");
    await fs.mkdir(petDir, { recursive: true });
    await createValidSpritesheet(petDir);
    await writePetJson(petDir, {
      id: "only",
      displayName: "Only",
      description: "Only pet.",
      spritesheetPath: "spritesheet.png",
    });

    await store.selectPet(null);
    const report = await importCodexPets(store, codexHome);
    expect(report.selectedPetIdUnchanged).toBeNull();
    expect(store.getSettings().selectedPetId).toBeNull();
  });

  it("reports duplicate source ids as conflicts", async () => {
    for (const folder of ["first", "second"]) {
      const petDir = path.join(codexHome, "pets", folder);
      await fs.mkdir(petDir, { recursive: true });
      await createValidSpritesheet(petDir);
      await writePetJson(petDir, {
        id: "same-id",
        displayName: folder,
        description: `${folder} pet.`,
        spritesheetPath: "spritesheet.png",
      });
    }

    const report = await importCodexPets(store, codexHome);
    expect(report.imported).toHaveLength(1);
    expect(report.conflicts).toHaveLength(1);
    expect(store.listPets()).toHaveLength(1);
  });

  it("reports invalid manifest as failed", async () => {
    const petDir = path.join(codexHome, "pets", "bad");
    await fs.mkdir(petDir, { recursive: true });
    await createValidSpritesheet(petDir);
    await writePetJson(petDir, {
      id: "bad",
      displayName: "Bad",
      description: "Missing spritesheetPath.",
      spritesheetPath: "",
    });

    const report = await importCodexPets(store, codexHome);
    expect(report.failed).toHaveLength(1);
    expect(store.listPets()).toHaveLength(0);
  });
});
