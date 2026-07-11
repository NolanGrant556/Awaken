import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import fs from "node:fs/promises";
import { basename, isAbsolute, join } from "node:path";

type SpawnInstaller = (
  command: string,
  args: string[],
  options: {
    env: NodeJS.ProcessEnv;
    stdio: ["ignore", "pipe", "pipe"];
  },
) => ChildProcess;

interface ReleaseManifest {
  schemaVersion?: number;
  runtimeName?: string;
  active?: string;
  platform?: string;
  arch?: string;
  archive?: string;
  rootDirectory?: string;
  integrity?: {
    algorithm?: string;
    scope?: string;
    trustBoundary?: string;
  };
  signing?: {
    mode?: string;
    teamIdentifier?: string;
    bundleIdentifier?: string;
  };
}

export interface RuntimeArchiveInstallOptions {
  packaged: boolean;
  resourcesPath: string;
  userDataDir: string;
  electronExecutable: string;
  platform?: NodeJS.Platform;
  arch?: string;
  hostTeamIdentifier?: string;
  spawnInstaller?: SpawnInstaller;
}

export interface RuntimeArchiveInstallResult {
  status: "not_packaged" | "not_bundled" | "already_active" | "installed";
  active?: string;
}

function runtimePlatform(platform: NodeJS.Platform): string {
  return platform === "win32" ? "windows" : platform;
}

function signedHostTeamIdentifier(executable: string): string {
  const result = spawnSync("codesign", ["-d", "--verbose=4", executable], {
    encoding: "utf8",
  });
  if (result.status !== 0) {
    return "";
  }
  return (
    `${result.stdout ?? ""}\n${result.stderr ?? ""}`
      .match(/^TeamIdentifier=(.+)$/m)?.[1]
      ?.trim() ?? ""
  );
}

async function readJSON(path: string): Promise<unknown> {
  return JSON.parse(await fs.readFile(path, "utf8")) as unknown;
}

async function currentActive(userDataDir: string): Promise<string> {
  try {
    const current = (await readJSON(
      join(userDataDir, "runtimes", "primary-runtime", "current.json"),
    )) as { active?: unknown };
    return typeof current.active === "string" ? current.active : "";
  } catch {
    return "";
  }
}

function runInstaller(
  options: RuntimeArchiveInstallOptions,
  scriptPath: string,
  archivePath: string,
  manifestPath: string,
): Promise<void> {
  const spawnProcess = options.spawnInstaller ?? spawn;
  const expectedPlatform = runtimePlatform(options.platform ?? process.platform);
  const installerArgs = [
    scriptPath,
    "--archive",
    archivePath,
    "--manifest",
    manifestPath,
  ];
  if (expectedPlatform === "darwin") {
    installerArgs.push(
      "--expected-team-id",
      options.hostTeamIdentifier ?? "",
    );
  }
  return new Promise((resolve, reject) => {
    const child = spawnProcess(
      options.electronExecutable,
      installerArgs,
      {
        env: {
          ...process.env,
          ELECTRON_RUN_AS_NODE: "1",
          AWAKEN_USER_DATA_DIR: options.userDataDir,
        },
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
    let stdout = "";
    let stderr = "";
    child.stdout?.setEncoding("utf8");
    child.stderr?.setEncoding("utf8");
    child.stdout?.on("data", (chunk) => {
      stdout += String(chunk);
    });
    child.stderr?.on("data", (chunk) => {
      stderr += String(chunk);
    });
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(
        new Error(
          `bundled primary runtime install failed (${signal ?? code ?? "unknown"}): ${(
            stderr || stdout
          ).trim()}`,
        ),
      );
    });
  });
}

export async function installBundledPrimaryRuntimeArchive(
  options: RuntimeArchiveInstallOptions,
): Promise<RuntimeArchiveInstallResult> {
  if (!options.packaged) {
    return { status: "not_packaged" };
  }
  const archiveDirectory = join(options.resourcesPath, "runtime-archive");
  let entries: string[];
  try {
    entries = await fs.readdir(archiveDirectory);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return { status: "not_bundled" };
    }
    throw error;
  }
  const expectedPlatform = runtimePlatform(options.platform ?? process.platform);
  const expectedArch = options.arch ?? process.arch;
  if (expectedPlatform === "darwin" && !options.hostTeamIdentifier) {
    options.hostTeamIdentifier = signedHostTeamIdentifier(options.electronExecutable);
  }
  const candidates: Array<{ manifest: ReleaseManifest; manifestPath: string }> = [];
  for (const entry of entries.filter((name) => name.endsWith(".manifest.json"))) {
    const manifestPath = join(archiveDirectory, entry);
    const manifest = (await readJSON(manifestPath)) as ReleaseManifest;
    if (
      manifest.schemaVersion === 1 &&
      manifest.runtimeName === "primary-runtime" &&
      manifest.platform === expectedPlatform &&
      manifest.arch === expectedArch
    ) {
      candidates.push({ manifest, manifestPath });
    }
  }
  if (candidates.length === 0) {
    return { status: "not_bundled" };
  }
  if (candidates.length !== 1) {
    throw new Error(
      `expected exactly one bundled primary runtime for ${expectedPlatform}-${expectedArch}, found ${candidates.length}`,
    );
  }
  const { manifest, manifestPath } = candidates[0];
  const active = String(manifest.active ?? "").trim();
  const archiveName = String(manifest.archive ?? "").trim();
  const rootDirectory = String(manifest.rootDirectory ?? "").trim();
  if (
    !active ||
    isAbsolute(active) ||
    active.split(/[\\/]+/).includes("..") ||
    !archiveName ||
    basename(archiveName) !== archiveName ||
    !rootDirectory ||
    rootDirectory === "." ||
    basename(rootDirectory) !== rootDirectory
  ) {
    throw new Error("bundled primary runtime manifest contains an unsafe path");
  }
  if (
    manifest.integrity?.algorithm !== "sha256" ||
    manifest.integrity?.scope !== "archive" ||
    manifest.integrity?.trustBoundary !== "packaged-host"
  ) {
    throw new Error("bundled primary runtime manifest has no trusted archive digest contract");
  }
  if (
    expectedPlatform === "darwin" &&
    (!options.hostTeamIdentifier ||
      manifest.signing?.mode !== "developer-id" ||
      manifest.signing?.teamIdentifier !== options.hostTeamIdentifier ||
      manifest.signing?.bundleIdentifier !==
        "com.awaken.runtime.libreoffice-renderer")
  ) {
    throw new Error(
      "bundled primary runtime is not signed by the same trusted team as Awaken",
    );
  }
  if ((await currentActive(options.userDataDir)) === active) {
    return { status: "already_active", active };
  }
  const archivePath = join(archiveDirectory, archiveName);
  await fs.access(archivePath);
  const scriptPath = join(
    options.resourcesPath,
    "runtime-installer",
    "install-primary-runtime-archive.mjs",
  );
  await fs.access(scriptPath);
  await runInstaller(options, scriptPath, archivePath, manifestPath);
  if ((await currentActive(options.userDataDir)) !== active) {
    throw new Error("bundled primary runtime installer did not activate the expected revision");
  }
  return { status: "installed", active };
}
