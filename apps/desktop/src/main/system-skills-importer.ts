import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";

const MANIFEST_FILE_NAME = ".manifest.json";
const STAGING_PREFIX = ".system-skills.stage-";
const BACKUP_PREFIX = ".system-skills.backup-";

export type SystemSkillsManifest = {
  schemaVersion: 1;
  appVersion: string;
  files: Record<string, string>;
};

export type SystemSkillsSyncReport = {
  status: "installed" | "updated" | "unchanged";
  sourceRoot: string;
  targetRoot: string;
  fileCount: number;
};

export type SystemSkillsSyncErrorCode =
  | "SYSTEM_SKILLS_SOURCE_UNAVAILABLE"
  | "SYSTEM_SKILLS_SOURCE_INVALID"
  | "SYSTEM_SKILLS_SYNC_FAILED";

export class SystemSkillsSyncError extends Error {
  constructor(
    public readonly code: SystemSkillsSyncErrorCode,
    message: string,
    options?: ErrorOptions,
  ) {
    super(message, options);
    this.name = "SystemSkillsSyncError";
  }
}

type InventoryFile = {
  absolutePath: string;
  relativePath: string;
  sha256: string;
};

type SystemSkillsSyncDeps = {
  rename?: typeof fs.rename;
  uuid?: () => string;
};

function compareStable(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

export function resolveSystemSkillsSourceRoot(input: {
  packaged: boolean;
  resourcesPath: string;
  appPath: string;
}): string {
  return input.packaged
    ? path.join(input.resourcesPath, "system-skills")
    : path.join(input.appPath, "resources", "system-skills");
}

export function resolveSystemSkillsTargetRoot(homePath: string): string {
  return path.join(homePath, ".awaken", "skills", "system-skills");
}

function posixRelative(root: string, absolutePath: string): string {
  return path.relative(root, absolutePath).split(path.sep).join("/");
}

async function pathExists(candidate: string): Promise<boolean> {
  try {
    await fs.lstat(candidate);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return false;
    }
    throw error;
  }
}

async function inventoryRoot(
  root: string,
  options: { source: boolean },
): Promise<InventoryFile[]> {
  let rootStat;
  try {
    rootStat = await fs.lstat(root);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      throw new SystemSkillsSyncError(
        "SYSTEM_SKILLS_SOURCE_UNAVAILABLE",
        `system Skills source is unavailable: ${root}`,
        { cause: error },
      );
    }
    throw error;
  }
  if (rootStat.isSymbolicLink() || !rootStat.isDirectory()) {
    throw new SystemSkillsSyncError(
      options.source
        ? "SYSTEM_SKILLS_SOURCE_INVALID"
        : "SYSTEM_SKILLS_SYNC_FAILED",
      `system Skills root must be a real directory: ${root}`,
    );
  }

  const inventory: InventoryFile[] = [];
  const visit = async (directory: string): Promise<void> => {
    const entries = await fs.readdir(directory, { withFileTypes: true });
    entries.sort((left, right) => compareStable(left.name, right.name));
    for (const entry of entries) {
      const absolutePath = path.join(directory, entry.name);
      const relativePath = posixRelative(root, absolutePath);
      if (relativePath === MANIFEST_FILE_NAME && !options.source) {
        continue;
      }
      if (relativePath === MANIFEST_FILE_NAME && options.source) {
        throw new SystemSkillsSyncError(
          "SYSTEM_SKILLS_SOURCE_INVALID",
          `${MANIFEST_FILE_NAME} is reserved for Desktop sync metadata`,
        );
      }
      const stat = await fs.lstat(absolutePath);
      if (stat.isSymbolicLink()) {
        throw new SystemSkillsSyncError(
          options.source
            ? "SYSTEM_SKILLS_SOURCE_INVALID"
            : "SYSTEM_SKILLS_SYNC_FAILED",
          `system Skills directory cannot contain symlinks: ${relativePath}`,
        );
      }
      if (stat.isDirectory()) {
        await visit(absolutePath);
        continue;
      }
      if (!stat.isFile()) {
        throw new SystemSkillsSyncError(
          options.source
            ? "SYSTEM_SKILLS_SOURCE_INVALID"
            : "SYSTEM_SKILLS_SYNC_FAILED",
          `system Skills directory cannot contain special files: ${relativePath}`,
        );
      }
      const content = await fs.readFile(absolutePath);
      inventory.push({
        absolutePath,
        relativePath,
        sha256: createHash("sha256").update(content).digest("hex"),
      });
    }
  };
  await visit(root);
  inventory.sort((left, right) =>
    compareStable(left.relativePath, right.relativePath),
  );
  return inventory;
}

function manifestFromInventory(
  appVersion: string,
  inventory: InventoryFile[],
): SystemSkillsManifest {
  const files: Record<string, string> = {};
  for (const file of inventory) {
    files[file.relativePath] = file.sha256;
  }
  return { schemaVersion: 1, appVersion, files };
}

function manifestsEqual(
  left: SystemSkillsManifest,
  right: SystemSkillsManifest,
): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

async function readManifest(targetRoot: string): Promise<SystemSkillsManifest | null> {
  try {
    const parsed = JSON.parse(
      await fs.readFile(path.join(targetRoot, MANIFEST_FILE_NAME), "utf8"),
    ) as Partial<SystemSkillsManifest>;
    if (
      parsed.schemaVersion !== 1 ||
      typeof parsed.appVersion !== "string" ||
      !parsed.files ||
      Array.isArray(parsed.files) ||
      typeof parsed.files !== "object" ||
      Object.values(parsed.files).some((value) => typeof value !== "string")
    ) {
      return null;
    }
    const files: Record<string, string> = {};
    for (const key of Object.keys(parsed.files).sort(compareStable)) {
      files[key] = parsed.files[key] as string;
    }
    return {
      schemaVersion: 1,
      appVersion: parsed.appVersion,
      files,
    };
  } catch {
    return null;
  }
}

async function targetMatches(
  targetRoot: string,
  expected: SystemSkillsManifest,
): Promise<boolean> {
  if (!(await pathExists(targetRoot))) {
    return false;
  }
  const recorded = await readManifest(targetRoot);
  if (!recorded || !manifestsEqual(recorded, expected)) {
    return false;
  }
  try {
    const actual = manifestFromInventory(
      expected.appVersion,
      await inventoryRoot(targetRoot, { source: false }),
    );
    return manifestsEqual(actual, expected);
  } catch {
    return false;
  }
}

async function recoverInterruptedSwap(targetRoot: string): Promise<void> {
  const parent = path.dirname(targetRoot);
  await fs.mkdir(parent, { recursive: true });
  const entries = await fs.readdir(parent, { withFileTypes: true });
  const stages = entries
    .filter((entry) => entry.name.startsWith(STAGING_PREFIX))
    .map((entry) => path.join(parent, entry.name));
  const backups = entries
    .filter((entry) => entry.name.startsWith(BACKUP_PREFIX))
    .map((entry) => path.join(parent, entry.name));

  await Promise.all(stages.map((stage) => fs.rm(stage, { recursive: true, force: true })));
  if (await pathExists(targetRoot)) {
    await Promise.all(backups.map((backup) => fs.rm(backup, { recursive: true, force: true })));
    return;
  }
  if (backups.length === 0) {
    return;
  }
  const ranked = await Promise.all(
    backups.map(async (backup) => ({ backup, mtimeMs: (await fs.lstat(backup)).mtimeMs })),
  );
  ranked.sort(
    (left, right) =>
      right.mtimeMs - left.mtimeMs || compareStable(right.backup, left.backup),
  );
  await fs.rename(ranked[0].backup, targetRoot);
  await Promise.all(
    ranked.slice(1).map(({ backup }) => fs.rm(backup, { recursive: true, force: true })),
  );
}

async function createStagingDirectory(
  stagingRoot: string,
  inventory: InventoryFile[],
  manifest: SystemSkillsManifest,
): Promise<void> {
  await fs.mkdir(stagingRoot, { recursive: false });
  for (const file of inventory) {
    const destination = path.join(
      stagingRoot,
      ...file.relativePath.split("/"),
    );
    await fs.mkdir(path.dirname(destination), { recursive: true });
    const stat = await fs.lstat(file.absolutePath);
    if (stat.isSymbolicLink() || !stat.isFile()) {
      throw new SystemSkillsSyncError(
        "SYSTEM_SKILLS_SOURCE_INVALID",
        `system Skills source changed during synchronization: ${file.relativePath}`,
      );
    }
    const content = await fs.readFile(file.absolutePath);
    const sha256 = createHash("sha256").update(content).digest("hex");
    if (sha256 !== file.sha256) {
      throw new SystemSkillsSyncError(
        "SYSTEM_SKILLS_SOURCE_INVALID",
        `system Skills source changed during synchronization: ${file.relativePath}`,
      );
    }
    await fs.writeFile(destination, content);
  }
  await fs.writeFile(
    path.join(stagingRoot, MANIFEST_FILE_NAME),
    `${JSON.stringify(manifest, null, 2)}\n`,
    { encoding: "utf8", mode: 0o600 },
  );
}

export async function syncSystemSkills(input: {
  sourceRoot: string;
  targetRoot: string;
  appVersion: string;
  deps?: SystemSkillsSyncDeps;
}): Promise<SystemSkillsSyncReport> {
  const { sourceRoot, targetRoot, appVersion } = input;
  const rename = input.deps?.rename ?? fs.rename;
  const uuid = input.deps?.uuid ?? randomUUID;

  try {
    await recoverInterruptedSwap(targetRoot);
  } catch (error) {
    throw new SystemSkillsSyncError(
      "SYSTEM_SKILLS_SYNC_FAILED",
      `failed to recover interrupted system Skills synchronization in ${path.dirname(targetRoot)}`,
      { cause: error },
    );
  }

  let sourceInventory: InventoryFile[];
  try {
    sourceInventory = await inventoryRoot(sourceRoot, { source: true });
  } catch (error) {
    if (error instanceof SystemSkillsSyncError) {
      throw error;
    }
    throw new SystemSkillsSyncError(
      "SYSTEM_SKILLS_SOURCE_INVALID",
      `failed to inspect system Skills source: ${sourceRoot}`,
      { cause: error },
    );
  }
  const expected = manifestFromInventory(appVersion, sourceInventory);

  try {
    if (await targetMatches(targetRoot, expected)) {
      return {
        status: "unchanged",
        sourceRoot,
        targetRoot,
        fileCount: sourceInventory.length,
      };
    }

    const targetExisted = await pathExists(targetRoot);
    const parent = path.dirname(targetRoot);
    const stagingRoot = path.join(parent, `${STAGING_PREFIX}${uuid()}`);
    const backupRoot = path.join(parent, `${BACKUP_PREFIX}${uuid()}`);
    let movedExistingTarget = false;
    try {
      await createStagingDirectory(stagingRoot, sourceInventory, expected);
    } catch (error) {
      await fs.rm(stagingRoot, { recursive: true, force: true });
      throw error;
    }
    try {
      if (targetExisted) {
        await rename(targetRoot, backupRoot);
        movedExistingTarget = true;
      }
      await rename(stagingRoot, targetRoot);
      if (movedExistingTarget) {
        await fs.rm(backupRoot, { recursive: true, force: true });
      }
    } catch (error) {
      if (movedExistingTarget && (await pathExists(backupRoot))) {
        await fs.rm(targetRoot, { recursive: true, force: true });
        await fs.rename(backupRoot, targetRoot);
      }
      await fs.rm(stagingRoot, { recursive: true, force: true });
      throw error;
    }

    return {
      status: targetExisted ? "updated" : "installed",
      sourceRoot,
      targetRoot,
      fileCount: sourceInventory.length,
    };
  } catch (error) {
    if (error instanceof SystemSkillsSyncError) {
      throw error;
    }
    throw new SystemSkillsSyncError(
      "SYSTEM_SKILLS_SYNC_FAILED",
      `failed to synchronize system Skills into ${targetRoot}`,
      { cause: error },
    );
  }
}
