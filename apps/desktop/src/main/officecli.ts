import { createHash, randomUUID } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import fs from "node:fs/promises";
import path from "node:path";
import { correctXlsxPictureLayout, XLSX_PICTURE_LAYOUT_VERSION } from "./xlsx-preview-pictures.js";
import {
  MAX_OFFICE_HTML_BYTES,
  MAX_OFFICE_STDERR_BYTES,
  MAX_OFFICE_STDOUT_BYTES,
  OFFICE_RENDER_TIMEOUT_MS,
  PREVIEW_CACHE_MAX_BYTES,
} from "./project-file-preview-policy.js";

export type OfficeCliManifestTarget = {
  asset: string;
  sha256: string;
};

export type OfficeCliManifest = {
  version: string;
  license: "Apache-2.0";
  releaseBaseUrl: string;
  targets: Record<string, OfficeCliManifestTarget>;
};

export type OfficeCliRuntimePaths = {
  packaged: boolean;
  resourcesPath: string;
  appPath: string;
  cachePath: string;
  platform?: NodeJS.Platform;
  arch?: NodeJS.Architecture;
};

export type OfficeCliErrorCode =
  | "OFFICECLI_UNAVAILABLE"
  | "OFFICECLI_VERSION_MISMATCH"
  | "OFFICE_RENDER_FAILED"
  | "OFFICE_RENDER_TIMEOUT"
  | "PREVIEW_CANCELLED"
  | "PREVIEW_RESOURCE_LIMIT";

export class OfficeCliError extends Error {
  constructor(
    readonly code: OfficeCliErrorCode,
    message: string,
  ) {
    super(message);
    this.name = "OfficeCliError";
  }
}

export function officeCliTargetName(
  platform: NodeJS.Platform = process.platform,
  arch = process.arch,
): string {
  return `${platform}-${arch}`;
}

export function resolveOfficeCliManifestPath(paths: OfficeCliRuntimePaths): string {
  return paths.packaged
    ? path.join(paths.resourcesPath, "officecli-release.json")
    : path.join(paths.appPath, "resources", "officecli-release.json");
}

export function resolveOfficeCliBinaryPath(paths: OfficeCliRuntimePaths): string {
  const platform = paths.platform ?? process.platform;
  const arch = paths.arch ?? process.arch;
  const target = officeCliTargetName(platform, arch);
  const name = platform === "win32" ? "officecli.exe" : "officecli";
  return paths.packaged
    ? path.join(paths.resourcesPath, "bin", target, name)
    : path.join(paths.appPath, "resources", "bin", target, name);
}

async function readManifest(paths: OfficeCliRuntimePaths): Promise<OfficeCliManifest> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(
      await fs.readFile(resolveOfficeCliManifestPath(paths), "utf8"),
    );
  } catch {
    throw new OfficeCliError(
      "OFFICECLI_UNAVAILABLE",
      "Awaken 内置的 Office 预览清单不可用。",
    );
  }
  if (
    !parsed ||
    typeof parsed !== "object" ||
    !("version" in parsed) ||
    typeof parsed.version !== "string" ||
    !("targets" in parsed) ||
    !parsed.targets ||
    typeof parsed.targets !== "object"
  ) {
    throw new OfficeCliError(
      "OFFICECLI_UNAVAILABLE",
      "Awaken 内置的 Office 预览清单无效。",
    );
  }
  return parsed as OfficeCliManifest;
}

function officeCliEnvironment(): NodeJS.ProcessEnv {
  return {
    ...process.env,
    OFFICECLI_SKIP_UPDATE: "1",
    OFFICECLI_NO_AUTO_INSTALL: "1",
    OFFICECLI_NO_AUTO_RESIDENT: "1",
  };
}

function killProcessTree(child: Pick<ChildProcess, "pid" | "kill">): void {
  if (!child.pid) return;
  if (process.platform === "win32") {
    const killer = spawn(
      "taskkill",
      ["/pid", String(child.pid), "/T", "/F"],
      { stdio: "ignore", windowsHide: true },
    );
    killer.unref();
    return;
  }
  try {
    process.kill(-child.pid, "SIGKILL");
  } catch {
    child.kill("SIGKILL");
  }
}

type ExecuteOptions = {
  signal?: AbortSignal;
  timeoutMs?: number;
};

async function executeOfficeCli(
  binaryPath: string,
  args: string[],
  options: ExecuteOptions = {},
): Promise<{ stdout: string; stderr: string }> {
  if (options.signal?.aborted) {
    throw new OfficeCliError("PREVIEW_CANCELLED", "已取消文件预览。");
  }
  return new Promise((resolve, reject) => {
    const child = spawn(binaryPath, args, {
      env: officeCliEnvironment(),
      shell: false,
      detached: process.platform !== "win32",
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = Buffer.alloc(0);
    let stderr = Buffer.alloc(0);
    let settled = false;
    let limitError: OfficeCliError | undefined;

    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      options.signal?.removeEventListener("abort", abort);
      if (error) reject(error);
      else resolve({ stdout: stdout.toString("utf8"), stderr: stderr.toString("utf8") });
    };
    const abort = () => {
      killProcessTree(child);
      finish(new OfficeCliError("PREVIEW_CANCELLED", "已取消文件预览。"));
    };
    const timer = setTimeout(() => {
      killProcessTree(child);
      finish(
        new OfficeCliError(
          "OFFICE_RENDER_TIMEOUT",
          "Office 文件预览生成超时，请重试或使用系统默认应用打开。",
        ),
      );
    }, options.timeoutMs ?? OFFICE_RENDER_TIMEOUT_MS);
    timer.unref?.();

    options.signal?.addEventListener("abort", abort, { once: true });
    child.stdout.on("data", (chunk: Buffer) => {
      if (stdout.length + chunk.length > MAX_OFFICE_STDOUT_BYTES) {
        limitError = new OfficeCliError(
          "PREVIEW_RESOURCE_LIMIT",
          "Office 预览引擎输出超过安全上限。",
        );
        killProcessTree(child);
        return;
      }
      stdout = Buffer.concat([stdout, chunk]);
    });
    child.stderr.on("data", (chunk: Buffer) => {
      if (stderr.length + chunk.length > MAX_OFFICE_STDERR_BYTES) {
        limitError = new OfficeCliError(
          "PREVIEW_RESOURCE_LIMIT",
          "Office 预览引擎错误输出超过安全上限。",
        );
        killProcessTree(child);
        return;
      }
      stderr = Buffer.concat([stderr, chunk]);
    });
    child.once("error", (error) => {
      finish(
        new OfficeCliError(
          "OFFICECLI_UNAVAILABLE",
          `Awaken 内置的 Office 预览引擎无法启动：${error.message}`,
        ),
      );
    });
    child.once("close", (code, signal) => {
      if (limitError) {
        finish(limitError);
        return;
      }
      if (code !== 0) {
        finish(
          new OfficeCliError(
            "OFFICE_RENDER_FAILED",
            `Office 文件预览生成失败（${signal ?? code ?? "unknown"}）。`,
          ),
        );
        return;
      }
      finish();
    });
  });
}

type CacheEntry = { path: string; size: number; mtimeMs: number };

async function collectCacheEntries(root: string): Promise<CacheEntry[]> {
  const entries: CacheEntry[] = [];
  async function visit(candidate: string): Promise<void> {
    let stat;
    try {
      stat = await fs.stat(candidate);
    } catch {
      return;
    }
    if (stat.isDirectory()) {
      for (const entry of await fs.readdir(candidate, { withFileTypes: true })) {
        await visit(path.join(candidate, entry.name));
      }
      return;
    }
    if (stat.isFile()) entries.push({ path: candidate, size: stat.size, mtimeMs: stat.mtimeMs });
  }
  await visit(root);
  return entries;
}

async function pruneCache(root: string): Promise<void> {
  const entries = await collectCacheEntries(root);
  let total = entries.reduce((sum, entry) => sum + entry.size, 0);
  if (total <= PREVIEW_CACHE_MAX_BYTES) return;
  entries.sort((left, right) => left.mtimeMs - right.mtimeMs);
  for (const entry of entries) {
    if (total <= PREVIEW_CACHE_MAX_BYTES) break;
    await fs.rm(entry.path, { force: true });
    total -= entry.size;
  }
}

export class OfficeCliRenderer {
  private readonly binaryPath: string;
  private manifestPromise?: Promise<OfficeCliManifest>;
  private readonly cacheRoot: string;
  private queue: Promise<void> = Promise.resolve();
  private versionVerified = false;

  constructor(private readonly paths: OfficeCliRuntimePaths) {
    this.binaryPath = resolveOfficeCliBinaryPath(paths);
    this.cacheRoot = path.join(paths.cachePath, "file-previews", "office");
  }

  private async verifyVersion(signal?: AbortSignal): Promise<OfficeCliManifest> {
    this.manifestPromise ??= readManifest(this.paths);
    const manifest = await this.manifestPromise;
    if (this.versionVerified) return manifest;
    const target = officeCliTargetName(
      this.paths.platform ?? process.platform,
      this.paths.arch ?? process.arch,
    );
    if (!manifest.targets[target]) {
      throw new OfficeCliError(
        "OFFICECLI_UNAVAILABLE",
        `当前平台没有可用的 Office 预览引擎（${target}）。`,
      );
    }
    const result = await executeOfficeCli(this.binaryPath, ["--version"], {
      signal,
      timeoutMs: 15_000,
    });
    if (!`${result.stdout}\n${result.stderr}`.includes(manifest.version)) {
      throw new OfficeCliError(
        "OFFICECLI_VERSION_MISMATCH",
        `Office 预览引擎版本不匹配，应为 ${manifest.version}。`,
      );
    }
    this.versionVerified = true;
    return manifest;
  }

  async render(sourcePath: string, signal?: AbortSignal): Promise<string> {
    const task = async () => this.renderNow(sourcePath, signal);
    const result = this.queue.then(task, task);
    this.queue = result.then(() => undefined, () => undefined);
    return result;
  }

  private async renderNow(sourcePath: string, signal?: AbortSignal): Promise<string> {
    if (signal?.aborted) {
      throw new OfficeCliError("PREVIEW_CANCELLED", "已取消文件预览。");
    }
    const [manifest, originalStat] = await Promise.all([
      this.verifyVersion(signal),
      fs.stat(sourcePath),
    ]);
    const key = createHash("sha256")
      .update(
        JSON.stringify({
          sourcePath,
          size: originalStat.size,
          mtimeMs: originalStat.mtimeMs,
          version: manifest.version,
          pictureLayoutVersion: XLSX_PICTURE_LAYOUT_VERSION,
        }),
      )
      .digest("hex");
    const cacheDir = path.join(this.cacheRoot, key);
    const outputPath = path.join(cacheDir, "index.html");
    try {
      const cached = await fs.stat(outputPath);
      if (cached.isFile() && cached.size > 0 && cached.size <= MAX_OFFICE_HTML_BYTES) {
        const now = new Date();
        await fs.utimes(outputPath, now, now);
        return outputPath;
      }
    } catch {
      // Cache misses are rendered below.
    }

    await fs.mkdir(cacheDir, { recursive: true });
    const temporaryPath = path.join(cacheDir, `index-${randomUUID()}.tmp.html`);
    try {
      await executeOfficeCli(
        this.binaryPath,
        ["view", sourcePath, "html", "-o", temporaryPath],
        { signal },
      );
      const rawOutputStat = await fs.stat(temporaryPath);
      if (rawOutputStat.size <= 0 || rawOutputStat.size > MAX_OFFICE_HTML_BYTES) {
        throw new OfficeCliError("PREVIEW_RESOURCE_LIMIT", "Office 预览结果超过安全上限。");
      }
      await correctXlsxPictureLayout(sourcePath, temporaryPath, signal);
      const [outputStat, afterStat] = await Promise.all([
        fs.stat(temporaryPath),
        fs.stat(sourcePath),
      ]);
      if (outputStat.size <= 0 || outputStat.size > MAX_OFFICE_HTML_BYTES) {
        throw new OfficeCliError(
          "PREVIEW_RESOURCE_LIMIT",
          "Office 预览结果超过安全上限。",
        );
      }
      if (
        afterStat.size !== originalStat.size ||
        afterStat.mtimeMs !== originalStat.mtimeMs
      ) {
        throw new OfficeCliError(
          "OFFICE_RENDER_FAILED",
          "Office 预览过程中检测到原文件发生变化，已停止加载。",
        );
      }
      await fs.rename(temporaryPath, outputPath);
      await pruneCache(this.cacheRoot);
      return outputPath;
    } finally {
      await fs.rm(temporaryPath, { force: true });
    }
  }
}
