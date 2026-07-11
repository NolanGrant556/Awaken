import { afterEach, beforeEach, describe, expect, it } from "vitest";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { createTransparentSpritesheet } from "./png-utils.js";
import { PetStore } from "./pet-store.js";
import { importBuiltinPets, resolveBuiltinPetsRoot } from "./builtin-pet-importer.js";

async function writeBuiltinPet(
  root: string,
  id: string,
  description = `${id} pet.`,
): Promise<void> {
  const petDir = path.join(root, id);
  await fs.mkdir(petDir, { recursive: true });
  await fs.writeFile(path.join(petDir, "spritesheet.png"), createTransparentSpritesheet());
  await fs.writeFile(
    path.join(petDir, "pet.json"),
    JSON.stringify({
      id,
      displayName: id,
      description,
      spritesheetPath: "spritesheet.png",
    }),
    "utf8",
  );
}

describe("resolveBuiltinPetsRoot", () => {
  it("uses Electron resources in packaged builds", () => {
    expect(resolveBuiltinPetsRoot({
      packaged: true,
      resourcesPath: "/Applications/Awaken.app/Contents/Resources",
      appPath: "/unused",
    })).toBe("/Applications/Awaken.app/Contents/Resources/pets");
  });

  it("uses app resources in development", () => {
    expect(resolveBuiltinPetsRoot({
      packaged: false,
      resourcesPath: "/unused",
      appPath: "/repo/apps/desktop",
    })).toBe("/repo/apps/desktop/resources/pets");
  });
});

describe("importBuiltinPets", () => {
  let tempDir: string;
  let petsRoot: string;
  let store: PetStore;

  beforeEach(async () => {
    tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-builtin-pets-"));
    petsRoot = path.join(tempDir, "resources", "pets");
    store = new PetStore(path.join(tempDir, "config"), path.join(tempDir, "userData"));
    await store.initialize();
  });

  afterEach(async () => {
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  it("imports bundled pets into the Awaken catalog without selecting one", async () => {
    await writeBuiltinPet(petsRoot, "alpha");
    await writeBuiltinPet(petsRoot, "beta");

    const report = await importBuiltinPets(store, petsRoot);

    expect(report.failed).toEqual([]);
    expect(report.imported).toHaveLength(2);
    expect(report.selectedPetIdUnchanged).toBeNull();
    expect(store.listPets().map((pet) => pet.id)).toEqual(["awaken:alpha", "awaken:beta"]);
    expect(store.listPets().every((pet) => pet.source === "awaken-builtin")).toBe(true);
  });

  it("updates the Awaken copy when a bundled pet changes", async () => {
    await writeBuiltinPet(petsRoot, "alpha", "First version.");
    await importBuiltinPets(store, petsRoot);
    await store.selectPet("awaken:alpha");

    await writeBuiltinPet(petsRoot, "alpha", "Second version.");
    const report = await importBuiltinPets(store, petsRoot);

    expect(report.updated).toHaveLength(1);
    expect(report.selectedPetIdUnchanged).toBe("awaken:alpha");
    expect(store.listPets()[0]?.description).toBe("Second version.");
  });

  it("isolates invalid bundled packages", async () => {
    await writeBuiltinPet(petsRoot, "valid");
    const invalidDir = path.join(petsRoot, "invalid");
    await fs.mkdir(invalidDir, { recursive: true });
    await fs.writeFile(path.join(invalidDir, "pet.json"), "{}", "utf8");

    const report = await importBuiltinPets(store, petsRoot);

    expect(report.imported).toHaveLength(1);
    expect(report.failed).toHaveLength(1);
    expect(store.listPets()).toHaveLength(1);
  });
});
