import { promises as fs } from "node:fs";
import { join, relative, resolve } from "node:path";

const migrationFileName = "primary-runtime-removed-v1.json";

export interface LegacyRuntimeCleanupResult {
  status: "absent" | "removed" | "already_migrated" | "refused";
  root: string;
  reason?: string;
}

function isMissing(error: unknown): boolean {
  return (error as NodeJS.ErrnoException | undefined)?.code === "ENOENT";
}

async function makeOwnedTreeRemovable(root: string): Promise<void> {
  const info = await fs.lstat(root);
  if (info.isSymbolicLink() || !info.isDirectory()) {
    return;
  }

  // Archived app bundles can legitimately contain read-only directories.
  // Add owner access only inside the already-validated legacy cache root;
  // never follow nested symlinks into another tree.
  await fs.chmod(root, info.mode | 0o700);
  const entries = await fs.readdir(root, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.isDirectory() && !entry.isSymbolicLink()) {
      await makeOwnedTreeRemovable(join(root, entry.name));
    }
  }
}

export async function cleanupLegacyPrimaryRuntime(
  userDataDir: string,
): Promise<LegacyRuntimeCleanupResult> {
  const userData = resolve(userDataDir);
  const migrationDir = join(userData, "data", "migrations");
  const marker = join(migrationDir, migrationFileName);
  const legacyRoot = join(userData, "runtimes", "primary-runtime");

  try {
    await fs.access(marker);
    return { status: "already_migrated", root: legacyRoot };
  } catch (error) {
    if (!isMissing(error)) {
      throw error;
    }
  }

  const relativeRoot = relative(userData, legacyRoot);
  if (
    relativeRoot !== join("runtimes", "primary-runtime") ||
    relativeRoot.startsWith("..")
  ) {
    return {
      status: "refused",
      root: legacyRoot,
      reason: "legacy runtime root is outside Awaken userData",
    };
  }

  let status: LegacyRuntimeCleanupResult["status"] = "absent";
  try {
    const info = await fs.lstat(legacyRoot);
    if (info.isSymbolicLink()) {
      return {
        status: "refused",
        root: legacyRoot,
        reason: "legacy runtime root is a symbolic link",
      };
    }
    if (!info.isDirectory()) {
      return {
        status: "refused",
        root: legacyRoot,
        reason: "legacy runtime root is not a directory",
      };
    }
    await makeOwnedTreeRemovable(legacyRoot);
    await fs.rm(legacyRoot, {
      recursive: true,
      force: true,
      maxRetries: 3,
      retryDelay: 100,
    });
    status = "removed";
  } catch (error) {
    if (!isMissing(error)) {
      throw error;
    }
  }

  await fs.mkdir(migrationDir, { recursive: true });
  await fs.writeFile(
    marker,
    `${JSON.stringify(
      {
        schemaVersion: 1,
        migration: "primary-runtime-removed",
        status,
        completedAt: new Date().toISOString(),
      },
      null,
      2,
    )}\n`,
    { encoding: "utf8", mode: 0o600 },
  );
  return { status, root: legacyRoot };
}
