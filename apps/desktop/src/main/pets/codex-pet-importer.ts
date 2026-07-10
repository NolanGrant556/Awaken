import fs from "node:fs/promises";
import path from "node:path";
import type { PetImportReport, PetManifest } from "../../shared/pets.js";
import { PetStore } from "./pet-store.js";
import { validatePetPackage } from "./pet-validator.js";

export function detectCodexHome(homeDir: string): string {
  const env = process.env.CODEX_HOME;
  if (env && typeof env === "string" && env.trim() !== "") {
    return path.normalize(env.trim());
  }
  return path.join(homeDir, ".codex");
}

export type CodexPetCandidate = {
  folderName: string;
  petDir: string;
};

export async function scanCodexCustomPets(codexHome: string): Promise<CodexPetCandidate[]> {
  const petsDir = path.join(codexHome, "pets");
  let entries: string[] = [];
  try {
    entries = await fs.readdir(petsDir);
  } catch {
    return [];
  }

  const candidates: CodexPetCandidate[] = [];
  for (const entry of entries) {
    if (entry.startsWith(".")) continue;
    const petDir = path.join(petsDir, entry);
    try {
      const stat = await fs.stat(petDir);
      if (!stat.isDirectory()) continue;
      const manifestPath = path.join(petDir, "pet.json");
      await fs.access(manifestPath);
      candidates.push({ folderName: entry, petDir });
    } catch {
      // Skip entries without a readable pet.json.
    }
  }

  return candidates.sort((a, b) => a.petDir.localeCompare(b.petDir));
}

export async function importCodexPets(
  petStore: PetStore,
  codexHome: string,
): Promise<PetImportReport> {
  const candidates = await scanCodexCustomPets(codexHome);
  const imported: PetManifest[] = [];
  const updated: PetManifest[] = [];
  const failed: PetImportReport["failed"] = [];
  const conflicts: PetImportReport["conflicts"] = [];

  const keptSourcePaths = new Map<string, string>();

  for (const candidate of candidates) {
    const validation = await validatePetPackage(candidate.petDir, candidate.folderName);
    if (!validation.valid) {
      failed.push({
        sourcePath: candidate.petDir,
        sourceId: candidate.folderName,
        error: validation.error,
      });
      continue;
    }

    const sourceId = validation.value.sourceId;
    const keptSourcePath = keptSourcePaths.get(sourceId);
    if (keptSourcePath) {
      conflicts.push({
        sourcePath: candidate.petDir,
        sourceId,
        keptSourcePath,
      });
      continue;
    }
    keptSourcePaths.set(sourceId, candidate.petDir);

    try {
      const result = await petStore.importOrUpdatePet({
        source: "codex-custom",
        sourceId,
        displayName: validation.value.displayName,
        description: validation.value.description,
        spritesheetSourcePath: validation.value.spritesheetPath,
        sourcePath: candidate.petDir,
      });
      if (result.updated) {
        updated.push(result.manifest);
      } else {
        imported.push(result.manifest);
      }
    } catch (error) {
      failed.push({
        sourcePath: candidate.petDir,
        sourceId,
        error: error instanceof Error ? error.message : "Import failed",
      });
    }
  }

  // Mark existing codex-custom pets whose source directory no longer exists.
  for (const manifest of petStore.getAllManifests()) {
    if (manifest.source !== "codex-custom") continue;
    if (!manifest.sourcePath || manifest.sourcePath.endsWith("#source-missing")) continue;
    try {
      await fs.access(manifest.sourcePath);
    } catch {
      await petStore.markSourceMissing(manifest.id);
    }
  }

  return {
    imported,
    updated,
    sourceMissing: petStore.getAllManifests().filter(
      (manifest) =>
        manifest.source === "codex-custom" &&
        manifest.sourcePath?.endsWith("#source-missing"),
    ),
    failed,
    conflicts,
    selectedPetIdUnchanged: petStore.getSettings().selectedPetId,
  };
}
