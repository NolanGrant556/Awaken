import fs from "node:fs/promises";
import path from "node:path";
import type { PetImportReport, PetManifest } from "../../shared/pets.js";
import { PetStore } from "./pet-store.js";
import { validatePetPackage } from "./pet-validator.js";

export function resolveBuiltinPetsRoot(input: {
  packaged: boolean;
  resourcesPath: string;
  appPath: string;
}): string {
  return input.packaged
    ? path.join(input.resourcesPath, "pets")
    : path.join(input.appPath, "resources", "pets");
}

export async function importBuiltinPets(
  petStore: PetStore,
  petsRoot: string,
): Promise<PetImportReport> {
  const imported: PetManifest[] = [];
  const updated: PetManifest[] = [];
  const failed: PetImportReport["failed"] = [];
  const conflicts: PetImportReport["conflicts"] = [];
  const keptSourcePaths = new Map<string, string>();

  let entries: string[];
  try {
    entries = await fs.readdir(petsRoot);
  } catch (error) {
    return {
      imported,
      updated,
      sourceMissing: [],
      failed: [{
        sourcePath: petsRoot,
        error: error instanceof Error ? error.message : "Built-in pets directory is unavailable",
      }],
      conflicts,
      selectedPetIdUnchanged: petStore.getSettings().selectedPetId,
    };
  }

  for (const folderName of entries.sort((a, b) => a.localeCompare(b))) {
    if (folderName.startsWith(".")) continue;
    const petDir = path.join(petsRoot, folderName);
    try {
      if (!(await fs.stat(petDir)).isDirectory()) continue;
    } catch {
      continue;
    }

    const validation = await validatePetPackage(petDir, folderName);
    if (!validation.valid) {
      failed.push({
        sourcePath: petDir,
        sourceId: folderName,
        error: validation.error,
      });
      continue;
    }

    const sourceId = validation.value.sourceId;
    const keptSourcePath = keptSourcePaths.get(sourceId);
    if (keptSourcePath) {
      conflicts.push({ sourcePath: petDir, sourceId, keptSourcePath });
      continue;
    }
    keptSourcePaths.set(sourceId, petDir);

    try {
      const result = await petStore.importOrUpdatePet({
        source: "awaken-builtin",
        sourceId,
        displayName: validation.value.displayName,
        description: validation.value.description,
        spriteVersionNumber: validation.value.spriteVersionNumber,
        spritesheetSourcePath: validation.value.spritesheetPath,
        sourcePath: petDir,
      });
      (result.updated ? updated : imported).push(result.manifest);
    } catch (error) {
      failed.push({
        sourcePath: petDir,
        sourceId,
        error: error instanceof Error ? error.message : "Built-in pet import failed",
      });
    }
  }

  return {
    imported,
    updated,
    sourceMissing: [],
    failed,
    conflicts,
    selectedPetIdUnchanged: petStore.getSettings().selectedPetId,
  };
}
