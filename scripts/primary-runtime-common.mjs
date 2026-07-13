import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  accessSync,
  chmodSync,
  closeSync,
  copyFileSync,
  existsSync,
  fsyncSync,
  lstatSync,
  mkdtempSync,
  mkdirSync,
  openSync,
  readFileSync,
  readSync,
  readlinkSync,
  readdirSync,
  realpathSync,
  renameSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
  writeSync,
  constants,
} from "node:fs";
import { homedir, tmpdir } from "node:os";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
export const isWindows = process.platform === "win32";

export const rootDir = resolve(scriptDir, "..");
export const desktopResourcesDir = join(rootDir, "apps", "desktop", "resources");
export const runtimeManifestPath = join(
  desktopResourcesDir,
  "runtime-manifest",
  "primary-runtime.json",
);
export const bundledRuntimeSeedDir = join(desktopResourcesDir, "runtime");

const MIGRATION_MARKER_NAME = "primary-runtime-v2.json";
const CURRENT_JSON = "current.json";
const STATE_JSON = "state.json";

export function resolveAwakenUserDataDir(env = process.env, platform = process.platform) {
  const explicit = String(env.AWAKEN_USER_DATA_DIR || "").trim();
  if (explicit) {
    return resolve(explicit);
  }
  const home = String(env.HOME || env.USERPROFILE || homedir() || "").trim();
  if (!home) {
    throw new Error("unable to resolve Awaken userData: HOME/USERPROFILE missing");
  }
  if (platform === "darwin") {
    return join(home, "Library", "Application Support", "@awaken", "desktop");
  }
  if (platform === "win32") {
    const appData = String(env.APPDATA || "").trim();
    if (appData) {
      return resolve(appData, "@awaken", "desktop");
    }
    return join(home, "AppData", "Roaming", "@awaken", "desktop");
  }
  const xdg = String(env.XDG_CONFIG_HOME || "").trim();
  if (xdg) {
    return resolve(xdg, "@awaken", "desktop");
  }
  return join(home, ".config", "@awaken", "desktop");
}

export function resolvePrimaryRuntimeRoot(env = process.env, platform = process.platform) {
  const override = String(env.AWAKEN_PRIMARY_RUNTIME_ROOT || "").trim();
  if (override) {
    return resolve(override);
  }
  return join(resolveAwakenUserDataDir(env, platform), "runtimes", "primary-runtime");
}

export function getRuntimeDir(env = process.env, platform = process.platform) {
  return resolvePrimaryRuntimeRoot(env, platform);
}

export const runtimeDir = resolvePrimaryRuntimeRoot();

export function platformArchKey(platform = process.platform, arch = process.arch) {
  const os = platform === "win32" ? "windows" : platform;
  return `${os}-${arch}`;
}

function safeKeySegment(value, fallback) {
  const normalized = String(value || "")
    .trim()
    .replace(/[^a-zA-Z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return normalized || fallback;
}

export function createBuildRevision(now = Date.now(), entropy = `${process.pid}`) {
  const suffix = createHash("sha256")
    .update(`${now}:${entropy}`)
    .digest("hex")
    .slice(0, 10);
  return `build-${Number(now).toString(36)}-${suffix}`;
}

export function versionKey(
  bundleVersion,
  platform = process.platform,
  arch = process.arch,
  revision = "",
) {
  const base = `${safeKeySegment(bundleVersion, "unversioned")}/${platformArchKey(platform, arch)}`;
  return revision ? `${base}/${safeKeySegment(revision, "build")}` : base;
}

export function versionDir(root, key) {
  return join(root, "versions", ...String(key).split("/"));
}

export function stagingRoot(root) {
  return join(root, "staging");
}

export function migrationMarkerPath(env = process.env, platform = process.platform) {
  return join(resolveAwakenUserDataDir(env, platform), "data", "migrations", MIGRATION_MARKER_NAME);
}

export function readRuntimeManifest(path = runtimeManifestPath) {
  return JSON.parse(readFileSync(path, "utf8"));
}

export function macOSRuntimeSigningExpectation(
  env = process.env,
  platform = process.platform,
) {
  if (platform !== "darwin") {
    return null;
  }
  const release = envFlag(env.AWAKEN_RUNTIME_RELEASE_SIGNING);
  const identity = String(env.AWAKEN_RUNTIME_CODESIGN_IDENTITY || "-").trim() || "-";
  return {
    release,
    mode: release ? "developer-id" : "ad-hoc",
    identity,
    strictVerified: true,
    gatekeeperVerified: release,
    notarized: release,
    stapled: release,
  };
}

export function runtimeRequiresPreparation(report, manifest, repair = false, options = {}) {
  if (repair || !report?.ready) {
    return true;
  }
  const expectedVersion = String(manifest?.bundleVersion || manifest?.version || "").trim();
  const activeVersion = String(report?.bundleVersion || "").trim();
  if (expectedVersion === "" || activeVersion !== expectedVersion) {
    return true;
  }
  const expectedSigning = options.macOSRuntimeSigning;
  if (expectedSigning) {
    const activeSigning = report?.security?.macOSRuntimeSigning;
    if (!activeSigning || activeSigning.mode !== expectedSigning.mode ||
        activeSigning.strictVerified !== true ||
        activeSigning.gatekeeperVerified !== expectedSigning.gatekeeperVerified ||
        activeSigning.notarized !== expectedSigning.notarized ||
        activeSigning.stapled !== expectedSigning.stapled) {
      return true;
    }
    if (expectedSigning.release && activeSigning.identity !== expectedSigning.identity) {
      return true;
    }
  }
  return false;
}

function platformPathKeys(platform = process.platform) {
  return platform === "win32" || platform === "windows"
    ? ["win32", "windows", "default"]
    : [platform, "default"];
}

export function componentRelativePath(component, platform = process.platform) {
  const paths = component?.relativePathByPlatform && typeof component.relativePathByPlatform === "object"
    ? component.relativePathByPlatform
    : component?.pathByPlatform && typeof component.pathByPlatform === "object"
      ? component.pathByPlatform
      : {};
  for (const key of platformPathKeys(platform)) {
    const platformPath = String(paths[key] ?? "").trim();
    if (platformPath) {
      return platformPath.replace(/^\.\.\/runtime\//, "");
    }
  }
  const relativePath = String(component?.relativePath ?? component?.path ?? "").trim();
  return relativePath.replace(/^\.\.\/runtime\//, "");
}

export function componentRuntimePath(component) {
  return componentRelativePath(component);
}

export function resolveRuntimePath(componentPath, baseDir = dirname(runtimeManifestPath)) {
  if (!componentPath) {
    return "";
  }
  if (isAbsolute(componentPath)) {
    return componentPath;
  }
  return resolve(baseDir, componentPath);
}

export function ensureDir(path) {
  mkdirSync(path, { recursive: true });
}

export function exists(path) {
  return Boolean(path) && existsSync(path);
}

export function isExecutable(path) {
  try {
    accessSync(path, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

export function run(command, args, options = {}) {
  return spawnSync(command, args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  });
}

export function runOk(command, args, options = {}) {
  const result = run(command, args, options);
  return result.status === 0;
}

export function commandOutput(command, args, options = {}) {
  try {
    return execFileSync(command, args, {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      ...options,
    }).trim();
  } catch {
    return "";
  }
}

export function whichAll(command) {
  const output = isWindows
    ? commandOutput("where.exe", [command]) || commandOutput("where", [command])
    : commandOutput("which", ["-a", command]) || commandOutput("which", [command]);
  return output
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

export function which(command) {
  return whichAll(command)[0] || "";
}

export function firstExisting(candidates) {
  const seen = new Set();
  for (const candidate of candidates) {
    const path = String(candidate || "").trim();
    if (!path || seen.has(path)) {
      continue;
    }
    seen.add(path);
    if (exists(path)) {
      return path;
    }
  }
  return "";
}

export function firstExecutable(candidates) {
  const seen = new Set();
  for (const candidate of candidates) {
    const path = String(candidate || "").trim();
    if (!path || seen.has(path)) {
      continue;
    }
    seen.add(path);
    if (exists(path) && statSync(path).isFile() && isExecutable(path)) {
      return path;
    }
  }
  return "";
}

export function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}

export function writeExecutable(path, content) {
  ensureDir(dirname(path));
  writeFileSync(path, content, "utf8");
  chmodSync(path, 0o755);
}

export function writeExecWrapper(path, target, label) {
  if (!target) {
    throw new Error(`missing target for ${label}`);
  }
  if (isWindows) {
    throw new Error(`bash wrapper is not supported for ${label} on Windows`);
  }
  writeExecutable(
    path,
    [
      "#!/usr/bin/env bash",
      "set -euo pipefail",
      `exec ${shellQuote(target)} "$@"`,
      "",
    ].join("\n"),
  );
}

export function copyExecutable(path, source, label) {
  if (!source) {
    throw new Error(`missing source for ${label}`);
  }
  ensureDir(dirname(path));
  copyFileSync(source, path);
  if (!isWindows) {
    chmodSync(path, 0o755);
  }
}

export function stageExecutable(path, source, label) {
  copyExecutable(path, source, label);
}

export function copyDirectoryContents(source, target) {
  const sourceRoot = realpathSync(source);
  if (pathInsideRoot(resolve(target), sourceRoot)) {
    throw new Error(`managed runtime copy target must not be inside its source root: ${target}`);
  }
  const copyEntry = (from, to) => {
    const stat = lstatSync(from);
    if (stat.isSymbolicLink()) {
      const linkTarget = readlinkSync(from);
      if (isAbsolute(linkTarget)) {
        throw new Error(`absolute symlink is forbidden while copying managed runtime: ${from} -> ${linkTarget}`);
      }
      let resolvedTarget;
      try {
        resolvedTarget = realpathSync(resolve(dirname(from), linkTarget));
      } catch {
        throw new Error(`broken or cyclic symlink while copying managed runtime: ${from} -> ${linkTarget}`);
      }
      if (!pathInsideRoot(resolvedTarget, sourceRoot)) {
        throw new Error(`external symlink is forbidden while copying managed runtime: ${from} -> ${resolvedTarget}`);
      }
      ensureDir(dirname(to));
      const targetStat = statSync(resolvedTarget);
      symlinkSync(linkTarget, to, targetStat.isDirectory() ? "dir" : "file");
      return;
    }
    if (stat.isDirectory()) {
      ensureDir(to);
      for (const entry of readdirSync(from)) {
        copyEntry(join(from, entry), join(to, entry));
      }
      if (!isWindows) {
        chmodSync(to, stat.mode & 0o777);
      }
      return;
    }
    if (!stat.isFile()) {
      throw new Error(`unsupported runtime source entry: ${from}`);
    }
    ensureDir(dirname(to));
    copyFileSync(from, to);
    if (!isWindows) {
      chmodSync(to, stat.mode & 0o777);
    }
  };
  copyEntry(sourceRoot, target);
}

export function linkDirectory(path, source, label) {
  throw new Error(`directory symlink/junction is forbidden for managed runtime (${label}): ${source}`);
}

export function fileSize(path) {
  try {
    return statSync(path).size;
  } catch {
    return 0;
  }
}

export function candidateSeedDirs(env = process.env) {
  const dirs = [];
  const seed = String(env.AWAKEN_PRIMARY_RUNTIME_SEED_DIR || "").trim();
  if (seed) {
    dirs.push(resolve(seed));
  }
  if (String(env.AWAKEN_PRIMARY_RUNTIME_CACHE_DIR || "").trim()) {
    console.warn(
      "[primary-runtime] AWAKEN_PRIMARY_RUNTIME_CACHE_DIR is deprecated; use AWAKEN_PRIMARY_RUNTIME_SEED_DIR for prepare seeds, or rely on one-time migration only",
    );
  }
  return dirs.filter(exists);
}


export function oldAwakenCacheCandidates(env = process.env) {
  const dirs = [];
  const deprecated = String(env.AWAKEN_PRIMARY_RUNTIME_CACHE_DIR || "").trim();
  if (deprecated) {
    dirs.push(resolve(deprecated));
  }
  const home = String(env.HOME || env.USERPROFILE || homedir() || "").trim();
  if (home) {
    dirs.push(join(home, ".cache", "awaken-runtimes", "primary-runtime"));
    dirs.push(join(home, ".cache", "awaken-runtimes", "primary-runtime", "dependencies"));
  }
  return [...new Set(dirs.map((d) => resolve(d)))];
}

export function isCodexCachePath(path) {
  const normalized = resolve(String(path || "")).replaceAll("\\", "/").toLowerCase();
  return normalized.includes("/.cache/codex-runtimes/") || normalized.includes("/codex-runtimes/");
}

export function isForbiddenExternalPath(path) {
  const normalized = String(path || "").replaceAll("\\", "/").toLowerCase();
  return normalized.startsWith("/opt/homebrew/") ||
    normalized.startsWith("/usr/local/") ||
    normalized.includes("/.cache/codex-runtimes/") ||
    normalized.includes("/codex-runtimes/");
}

export function isAllowedSystemDependency(path, platform = process.platform) {
  const normalized = String(path || "").replaceAll("\\", "/");
  if (platform === "darwin") {
    return normalized.startsWith("/System/Library/") || normalized.startsWith("/usr/lib/");
  }
  if (platform === "linux") {
    return normalized.startsWith("/lib/") ||
      normalized.startsWith("/lib64/") ||
      normalized.startsWith("/usr/lib/") ||
      normalized.startsWith("/usr/lib64/");
  }
  return false;
}

export function pathInsideRoot(path, root) {
  if (!path || !root) {
    return false;
  }
  let resolvedPath;
  let resolvedRoot;
  const canonicalize = (value) => {
    let cursor = resolve(value);
    const missing = [];
    while (!exists(cursor)) {
      const parent = dirname(cursor);
      if (parent === cursor) {
        return resolve(value);
      }
      missing.unshift(basenameSafe(cursor));
      cursor = parent;
    }
    let canonical;
    try {
      canonical = realpathSync(cursor);
    } catch {
      canonical = cursor;
    }
    return join(canonical, ...missing);
  };
  resolvedRoot = canonicalize(root);
  resolvedPath = canonicalize(path);
  const rel = relative(resolvedRoot, resolvedPath);
  return rel === "" || (!rel.startsWith(`..${sep}`) && rel !== ".." && !isAbsolute(rel));
}

function walkFiles(root, visit) {
  if (!exists(root)) {
    return;
  }
  const pending = [root];
  while (pending.length > 0) {
    const current = pending.pop();
    let stat;
    try {
      stat = lstatSync(current);
    } catch {
      continue;
    }
    if (stat.isSymbolicLink()) {
      visit(current, stat);
      continue;
    }
    if (stat.isDirectory()) {
      for (const entry of readdirSync(current)) {
        pending.push(join(current, entry));
      }
      continue;
    }
    if (stat.isFile()) {
      visit(current, stat);
    }
  }
}

function fileMagic(path) {
  let fd;
  try {
    fd = openSync(path, "r");
    const buffer = Buffer.alloc(4);
    if (readSync(fd, buffer, 0, 4, 0) !== 4) {
      return "";
    }
    return buffer.toString("hex");
  } catch {
    return "";
  } finally {
    if (fd !== undefined) {
      try {
        closeSync(fd);
      } catch {
        // best effort
      }
    }
  }
}

export function isMachOFile(path) {
  return new Set([
    "feedface",
    "feedfacf",
    "cefaedfe",
    "cffaedfe",
    "cafebabe",
    "cafebabf",
    "bebafeca",
    "bfbafeca",
  ]).has(fileMagic(path));
}

export function machODependencies(path) {
  if (!isMachOFile(path)) {
    return [];
  }
  const output = commandOutput("otool", ["-L", path]);
  if (!output) {
    return [];
  }
  return output
    .split(/\r?\n/)
    .slice(1)
    .map((line) => line.trim().split(/\s+\(/)[0])
    .filter((line) => line && !line.endsWith(":") && (line.startsWith("/") || line.startsWith("@")));
}

export function machOInstallName(path) {
  if (!isMachOFile(path)) {
    return "";
  }
  const output = commandOutput("otool", ["-D", path]);
  return output
    .split(/\r?\n/)
    .slice(1)
    .map((line) => line.trim())
    .find((line) => line && !line.endsWith(":") && (line.startsWith("/") || line.startsWith("@"))) || "";
}

export function machORpaths(path) {
  if (!isMachOFile(path)) {
    return [];
  }
  const output = commandOutput("otool", ["-l", path]);
  const lines = output.split(/\r?\n/);
  const paths = [];
  for (let index = 0; index < lines.length; index += 1) {
    if (lines[index].trim() !== "cmd LC_RPATH") {
      continue;
    }
    for (let cursor = index + 1; cursor < Math.min(lines.length, index + 8); cursor += 1) {
      const match = lines[cursor].trim().match(/^path\s+(.+?)\s+\(offset\s+\d+\)$/);
      if (match) {
        paths.push(match[1]);
        break;
      }
    }
  }
  return paths;
}

function expandLoaderToken(value, loaderPath, executablePath) {
  if (value.startsWith("@loader_path/")) {
    return resolve(dirname(loaderPath), value.slice("@loader_path/".length));
  }
  if (value.startsWith("@executable_path/")) {
    return resolve(dirname(executablePath || loaderPath), value.slice("@executable_path/".length));
  }
  return value;
}

export function resolveMachODependency(reference, loaderPath, executablePath = loaderPath) {
  if (isAbsolute(reference)) {
    return exists(reference) ? realpathSync(reference) : "";
  }
  if (reference.startsWith("@loader_path/") || reference.startsWith("@executable_path/")) {
    const candidate = expandLoaderToken(reference, loaderPath, executablePath);
    return exists(candidate) ? realpathSync(candidate) : "";
  }
  if (reference.startsWith("@rpath/")) {
    const suffix = reference.slice("@rpath/".length);
    for (const rpath of machORpaths(loaderPath)) {
      const expanded = expandLoaderToken(rpath, loaderPath, executablePath);
      const candidate = resolve(expanded, suffix);
      if (exists(candidate)) {
        return realpathSync(candidate);
      }
    }
    for (const candidate of [
      join(dirname(loaderPath), suffix),
      join(dirname(executablePath || loaderPath), suffix),
    ]) {
      if (exists(candidate)) {
        return realpathSync(candidate);
      }
    }
  }
  return "";
}

function runRequired(command, args, label) {
  const result = run(command, args);
  if (result.status !== 0) {
    const detail = String(result.stderr || result.stdout || "").trim();
    throw new Error(`${label} failed${detail ? `: ${detail}` : ""}`);
  }
}

/**
 * Copies one or more macOS executables and their complete non-system Mach-O
 * dependency closure. Every copied load command is rewritten to a relative
 * @loader_path reference so the resulting tree is independent of Homebrew or
 * any build-machine prefix.
 */
export function bundleMacOSDependencyClosure(entries, dependencyDir) {
  if (process.platform !== "darwin") {
    throw new Error("bundleMacOSDependencyClosure is only available on macOS");
  }
  if (!runOk("xcrun", ["--find", "install_name_tool"]) && !which("install_name_tool")) {
    throw new Error("install_name_tool is required to bundle macOS runtime dependencies");
  }
  ensureDir(dependencyDir);
  const sourceToTarget = new Map();
  const queue = [];
  const touched = new Set();
  let dependencyIndex = 0;

  for (const entry of entries) {
    const source = realpathSync(entry.source);
    copyExecutable(entry.target, source, entry.label || basenameSafe(source));
    sourceToTarget.set(source, entry.target);
    queue.push({
      source,
      target: entry.target,
      executableSource: realpathSync(entry.executableSource || entry.source),
    });
  }

  while (queue.length > 0) {
    const current = queue.shift();
    const installName = machOInstallName(current.source);
    for (const reference of machODependencies(current.source)) {
      if (reference === installName) {
        continue;
      }
      let dependency = resolveMachODependency(reference, current.source, current.executableSource);
      if (!dependency) {
        if (isAllowedSystemDependency(reference, "darwin")) {
          continue;
        }
        throw new Error(`unable to resolve Mach-O dependency ${reference} from ${current.source}`);
      }
      dependency = realpathSync(dependency);
      if (dependency === current.source) {
        continue;
      }
      if (isAllowedSystemDependency(dependency, "darwin")) {
        if (reference !== dependency) {
          runRequired(
            "install_name_tool",
            ["-change", reference, dependency, current.target],
            `rewrite system dependency in ${current.target}`,
          );
          touched.add(current.target);
        }
        continue;
      }

      let dependencyTarget = sourceToTarget.get(dependency);
      if (!dependencyTarget) {
        dependencyIndex += 1;
        dependencyTarget = join(dependencyDir, `d${String(dependencyIndex).padStart(4, "0")}`);
        copyExecutable(dependencyTarget, dependency, basenameSafe(dependency));
        sourceToTarget.set(dependency, dependencyTarget);
        queue.push({
          source: dependency,
          target: dependencyTarget,
          executableSource: current.executableSource,
        });
      }
      const loaderReference = `@loader_path/${relative(dirname(current.target), dependencyTarget).replaceAll("\\", "/")}`;
      runRequired(
        "install_name_tool",
        ["-change", reference, loaderReference, current.target],
        `rewrite dependency in ${current.target}`,
      );
      touched.add(current.target);
    }
  }

  for (const path of touched) {
    if (which("codesign")) {
      run("codesign", ["--force", "--sign", "-", path]);
    }
  }
  return {
    entries: entries.map((entry) => entry.target),
    dependencyDir,
    dependencyCount: dependencyIndex,
  };
}

export function normalizeMacOSRuntimeTree(root) {
  if (process.platform !== "darwin") {
    return { changed: 0 };
  }
  const files = [];
  walkFiles(root, (path, stat) => {
    if (stat.isFile() && isMachOFile(path)) {
      files.push(path);
    }
  });
  const installNameTargets = new Map();
  for (const path of files) {
    const installName = machOInstallName(path);
    // Only absolute build-machine IDs need cross-file remapping. Relative
    // @loader_path IDs are component-local and may legitimately repeat across
    // Node, Poppler and ffmpeg dependency directories.
    if (installName && isAbsolute(installName)) {
      installNameTargets.set(installName, path);
    }
  }
  const touched = new Set();
  for (const path of files) {
    const selfInstallName = machOInstallName(path);
    for (const dependency of machODependencies(path)) {
      if (dependency === selfInstallName) {
        continue;
      }
      const target = installNameTargets.get(dependency);
      if (!target || target === path) {
        continue;
      }
      const loaderReference = `@loader_path/${relative(dirname(path), target).replaceAll("\\", "/")}`;
      runRequired(
        "install_name_tool",
        ["-change", dependency, loaderReference, path],
        `normalize internal dependency in ${path}`,
      );
      touched.add(path);
    }
    if (selfInstallName && isAbsolute(selfInstallName) && !isAllowedSystemDependency(selfInstallName, "darwin")) {
      runRequired(
        "install_name_tool",
        ["-id", `@loader_path/${basenameSafe(path)}`, path],
        `normalize install name in ${path}`,
      );
      touched.add(path);
    }
  }
  if (which("codesign")) {
    for (const path of touched) {
      run("codesign", ["--force", "--sign", "-", path]);
    }
  }
  return { changed: touched.size };
}

function collectRuntimeFileIndex(root) {
  const names = new Set();
  walkFiles(root, (path, stat) => {
    if (stat.isFile()) {
      names.add(basenameSafe(path));
    }
  });
  return names;
}

export function materializeMacOSRpathAliases(root) {
  if (process.platform !== "darwin") {
    return [];
  }
  const files = [];
  walkFiles(root, (path, stat) => {
    if (stat.isFile()) {
      files.push(path);
    }
  });
  const created = [];
  for (const loaderPath of files.filter(isMachOFile)) {
    const installName = machOInstallName(loaderPath);
    for (const reference of machODependencies(loaderPath)) {
      if (reference === installName || !reference.startsWith("@rpath/")) {
        continue;
      }
      if (resolveMachODependency(reference, loaderPath, loaderPath)) {
        continue;
      }
      const suffix = reference.slice("@rpath/".length);
      const wanted = basenameSafe(suffix);
      const candidates = files.filter((candidate) => {
        const name = basenameSafe(candidate);
        return name === wanted || name.startsWith(`${wanted}.`);
      });
      if (candidates.length === 0) {
        continue;
      }
      if (candidates.length > 1) {
        throw new Error(
          `ambiguous @rpath alias ${reference} from ${loaderPath}: ${candidates.join(", ")}`,
        );
      }
      const source = candidates[0];
      const rpath = machORpaths(loaderPath)[0];
      if (!rpath) {
        continue;
      }
      const base = expandLoaderToken(rpath, loaderPath, loaderPath);
      const target = resolve(base, suffix);
      if (!pathInsideRoot(target, root)) {
        continue;
      }
      ensureDir(dirname(target));
      copyFileSync(source, target);
      if (!isWindows) {
        chmodSync(target, statSync(source).mode & 0o777);
      }
      files.push(target);
      created.push({ source, target, reference });
    }
  }
  return created;
}

function envFlag(value) {
  return ["1", "true", "yes", "on"].includes(String(value || "").trim().toLowerCase());
}

function assertStagedMacOSBundle(appRoot, ownershipRoot, sourceRoot = "") {
  if (!ownershipRoot || !pathInsideRoot(appRoot, ownershipRoot)) {
    throw new Error(`refusing to mutate macOS bundle outside the staging root: ${appRoot}`);
  }
  if (sourceRoot && exists(sourceRoot) && realpathSync(sourceRoot) === realpathSync(appRoot)) {
    throw new Error(`refusing to mutate the LibreOffice source application: ${sourceRoot}`);
  }
}

export const macOSOfficeRenderBundleIdentifier = "com.awaken.runtime.libreoffice-renderer";

const macOSOfficeLaunchServicesKeys = [
  "CFBundleDocumentTypes",
  "CFBundleURLTypes",
  "NSServices",
  "UTExportedTypeDeclarations",
  "UTImportedTypeDeclarations",
];

const macOSOfficeRegistrationBundles = [
  ["Contents", "Library", "Spotlight", "OOoSpotlightImporter.mdimporter"],
  ["Contents", "PlugIns", "QuickLookPreview.appex"],
  ["Contents", "PlugIns", "QuickLookThumbnail.appex"],
];

export function setMacOSBackgroundOnlyApplication(appRoot, options = {}) {
  const platform = options.platform || process.platform;
  if (platform !== "darwin") {
    return { changed: false, reason: "not macOS" };
  }
  assertStagedMacOSBundle(appRoot, options.ownershipRoot, options.sourceRoot);
  const infoPlist = join(appRoot, "Contents", "Info.plist");
  if (!exists(infoPlist)) {
    throw new Error(`macOS application Info.plist is missing: ${infoPlist}`);
  }
  if (!which("plutil")) {
    throw new Error("plutil is required to configure the macOS LibreOffice runtime");
  }

  // LSBackgroundOnly and LSUIElement are mutually exclusive activation
  // policies. The managed LibreOffice copy is a faceless render worker, so it
  // must never inherit an agent/UIElement setting from another build.
  run("plutil", ["-remove", "LSUIElement", infoPlist]);
  for (const key of macOSOfficeLaunchServicesKeys) {
    run("plutil", ["-remove", key, infoPlist]);
  }
  runRequired(
    "plutil",
    [
      "-replace",
      "CFBundleIdentifier",
      "-string",
      options.bundleIdentifier || macOSOfficeRenderBundleIdentifier,
      infoPlist,
    ],
    `set managed LibreOffice bundle identifier in ${infoPlist}`,
  );
  runRequired(
    "plutil",
    ["-replace", "CFBundleName", "-string", "Awaken Office Render Runtime", infoPlist],
    `set managed LibreOffice bundle name in ${infoPlist}`,
  );
  const replaced = run("plutil", ["-replace", "LSBackgroundOnly", "-bool", "YES", infoPlist]);
  if (replaced.status !== 0) {
    runRequired(
      "plutil",
      ["-insert", "LSBackgroundOnly", "-bool", "YES", infoPlist],
      `set LSBackgroundOnly in ${infoPlist}`,
    );
  }
  const verified = run("plutil", ["-extract", "LSBackgroundOnly", "raw", "-o", "-", infoPlist]);
  if (verified.status !== 0 || String(verified.stdout || "").trim() !== "true") {
    throw new Error(`failed to verify LSBackgroundOnly in ${infoPlist}`);
  }
  const removedRegistrationBundles = [];
  for (const parts of macOSOfficeRegistrationBundles) {
    const path = join(appRoot, ...parts);
    if (exists(path)) {
      rmSync(path, { recursive: true, force: true });
      removedRegistrationBundles.push(path);
    }
  }
  return {
    changed: true,
    appRoot,
    infoPlist,
    activationPolicy: "background-only",
    bundleIdentifier: options.bundleIdentifier || macOSOfficeRenderBundleIdentifier,
    removedLaunchServicesKeys: macOSOfficeLaunchServicesKeys,
    removedRegistrationBundles,
  };
}

export function sealMacOSRuntimeApplicationPermissions(appRoot, options = {}) {
  const platform = options.platform || process.platform;
  if (platform !== "darwin") {
    return { sealed: false, reason: "not macOS", files: 0, directories: 0 };
  }
  assertStagedMacOSBundle(appRoot, options.ownershipRoot, options.sourceRoot);
  const files = [];
  const directories = [];
  const pending = [appRoot];
  while (pending.length > 0) {
    const current = pending.pop();
    const stat = lstatSync(current);
    if (stat.isSymbolicLink()) {
      continue;
    }
    if (stat.isDirectory()) {
      directories.push({ path: current, mode: stat.mode & 0o777 });
      for (const entry of readdirSync(current)) {
        pending.push(join(current, entry));
      }
    } else if (stat.isFile()) {
      files.push({ path: current, mode: stat.mode & 0o777 });
    }
  }
  for (const file of files) {
    chmodSync(file.path, file.mode & ~0o222);
  }
  directories.sort((left, right) => right.path.length - left.path.length);
  for (const directory of directories) {
    chmodSync(directory.path, directory.mode & ~0o222);
  }
  return { sealed: true, files: files.length, directories: directories.length };
}

const macOSCodeBundleSuffixes = [
  ".app",
  ".appex",
  ".bundle",
  ".framework",
  ".mdimporter",
  ".plugin",
  ".xpc",
];

function isMacOSCodeBundle(path) {
  const lower = String(path || "").toLowerCase();
  return macOSCodeBundleSuffixes.some((suffix) => lower.endsWith(suffix));
}

function collectMacOSCode(appRoot) {
  const bundles = [appRoot];
  const machOFiles = [];
  const pending = [appRoot];
  while (pending.length > 0) {
    const current = pending.pop();
    const stat = lstatSync(current);
    if (stat.isSymbolicLink()) {
      continue;
    }
    if (stat.isDirectory()) {
      if (current !== appRoot && isMacOSCodeBundle(current)) {
        bundles.push(current);
      }
      for (const entry of readdirSync(current)) {
        pending.push(join(current, entry));
      }
      continue;
    }
    if (stat.isFile() && isMachOFile(current)) {
      machOFiles.push(current);
    }
  }
  const depth = (path) => relative(appRoot, path).split(sep).filter(Boolean).length;
  bundles.sort((left, right) => depth(right) - depth(left));
  machOFiles.sort((left, right) => depth(right) - depth(left));
  return { bundles, machOFiles };
}

function extractCodeEntitlements(codePath, entitlementsDir) {
  const name = createHash("sha256").update(codePath).digest("hex").slice(0, 20);
  const outputPath = join(entitlementsDir, `${name}.plist`);
  const result = run("codesign", ["-d", "--xml", "--entitlements", outputPath, codePath]);
  if (result.status !== 0 || !exists(outputPath) || fileSize(outputPath) === 0) {
    rmSync(outputPath, { force: true });
    return "";
  }
  if (!runOk("plutil", ["-lint", outputPath])) {
    rmSync(outputPath, { force: true });
    return "";
  }
  return outputPath;
}

export function captureMacOSRuntimeSigningMetadata(appRoot, options = {}) {
  const platform = options.platform || process.platform;
  if (platform !== "darwin") {
    return null;
  }
  if (!which("codesign") || !which("plutil")) {
    throw new Error("codesign and plutil are required to capture macOS runtime signing metadata");
  }
  const { bundles, machOFiles } = collectMacOSCode(appRoot);
  const metadataDir = mkdtempSync(join(tmpdir(), "awaken-runtime-entitlements-"));
  const entitlements = new Map();
  try {
    for (const path of [...machOFiles, ...bundles]) {
      entitlements.set(path, extractCodeEntitlements(path, metadataDir));
    }
    return { appRoot: resolve(appRoot), metadataDir, entitlements };
  } catch (error) {
    rmSync(metadataDir, { recursive: true, force: true });
    throw error;
  }
}

export function disposeMacOSRuntimeSigningMetadata(metadata) {
  if (metadata?.metadataDir) {
    rmSync(metadata.metadataDir, { recursive: true, force: true });
  }
}

function macOSBundleVersion(bundlePath) {
  const candidates = [
    join(bundlePath, "Contents", "Info.plist"),
    join(bundlePath, "Resources", "Info.plist"),
    join(bundlePath, "Versions", "Current", "Resources", "Info.plist"),
  ];
  for (const plist of candidates) {
    if (!exists(plist)) {
      continue;
    }
    const result = run("plutil", ["-extract", "CFBundleVersion", "raw", "-o", "-", plist]);
    const version = String(result.stdout || "").trim();
    if (result.status === 0 && /^[a-zA-Z0-9._-]+$/.test(version)) {
      return version;
    }
  }
  return "";
}

function signMacOSCode(codePath, entitlementsPath, identity, release) {
  const args = ["--force", "--sign", identity, "--options", "runtime"];
  if (release) {
    args.push("--timestamp");
  }
  if (entitlementsPath) {
    args.push("--entitlements", entitlementsPath);
  }
  if (String(codePath).toLowerCase().endsWith(".framework")) {
    const version = macOSBundleVersion(codePath);
    if (version) {
      args.push("--bundle-version", version);
    }
  }
  args.push(codePath);
  runRequired("codesign", args, `sign macOS runtime code ${codePath}`);
}

export function verifyMacOSRuntimeApplication(appRoot, options = {}) {
  const platform = options.platform || process.platform;
  if (platform !== "darwin") {
    return { verified: false, reason: "not macOS" };
  }
  if (!which("codesign") || !which("plutil")) {
    throw new Error("codesign and plutil are required to verify the macOS LibreOffice runtime");
  }
  const infoPlist = join(appRoot, "Contents", "Info.plist");
  const expectedIdentifier = options.bundleIdentifier || macOSOfficeRenderBundleIdentifier;
  const background = run("plutil", ["-extract", "LSBackgroundOnly", "raw", "-o", "-", infoPlist]);
  const identifier = run("plutil", ["-extract", "CFBundleIdentifier", "raw", "-o", "-", infoPlist]);
  if (background.status !== 0 || String(background.stdout || "").trim() !== "true") {
    throw new Error(`managed LibreOffice is not background-only: ${appRoot}`);
  }
  if (identifier.status !== 0 || String(identifier.stdout || "").trim() !== expectedIdentifier) {
    throw new Error(`managed LibreOffice has an unexpected bundle identifier: ${appRoot}`);
  }
  for (const key of macOSOfficeLaunchServicesKeys) {
    if (run("plutil", ["-extract", key, "raw", "-o", "-", infoPlist]).status === 0) {
      throw new Error(`managed LibreOffice still declares LaunchServices key ${key}: ${appRoot}`);
    }
  }
  for (const parts of macOSOfficeRegistrationBundles) {
    const path = join(appRoot, ...parts);
    if (exists(path)) {
      throw new Error(`managed LibreOffice still contains a registration bundle: ${path}`);
    }
  }
  runRequired(
    "codesign",
    ["--verify", "--deep", "--strict", "--verbose=4", appRoot],
    `strictly verify macOS runtime application ${appRoot}`,
  );
  if (options.release) {
    if (!which("spctl") || !which("xcrun")) {
      throw new Error("spctl and xcrun are required to validate a release macOS runtime");
    }
    runRequired(
      "xcrun",
      ["stapler", "validate", appRoot],
      `validate notarization ticket for macOS runtime application ${appRoot}`,
    );
    runRequired(
      "spctl",
      ["-a", "-vv", "--type", "execute", appRoot],
      `Gatekeeper assess macOS runtime application ${appRoot}`,
    );
  }
  return {
    verified: true,
    strictVerified: true,
    gatekeeperVerified: Boolean(options.release),
    notarized: Boolean(options.release),
    stapled: Boolean(options.release),
    activationPolicy: "background-only",
    bundleIdentifier: expectedIdentifier,
  };
}

function notarizeMacOSRuntimeApplication(appRoot, options = {}) {
  const profile = String(
    options.notaryProfile || process.env.AWAKEN_RUNTIME_NOTARY_PROFILE || "",
  ).trim();
  if (!profile) {
    throw new Error(
      "release runtime notarization requires AWAKEN_RUNTIME_NOTARY_PROFILE for notarytool",
    );
  }
  if (!which("xcrun") || !which("ditto")) {
    throw new Error("xcrun and ditto are required to notarize a release macOS runtime");
  }
  const temporary = mkdtempSync(join(tmpdir(), "awaken-runtime-notary-"));
  const archive = join(temporary, "LibreOffice-runtime.zip");
  try {
    runRequired(
      "ditto",
      ["-c", "-k", "--keepParent", appRoot, archive],
      `archive macOS runtime application for notarization ${appRoot}`,
    );
    runRequired(
      "xcrun",
      ["notarytool", "submit", archive, "--keychain-profile", profile, "--wait"],
      `notarize macOS runtime application ${appRoot}`,
    );
    runRequired(
      "xcrun",
      ["stapler", "staple", appRoot],
      `staple notarization ticket to macOS runtime application ${appRoot}`,
    );
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
  return { notarized: true, stapled: true };
}

export function verifyMacOSLibreOfficeReleaseSeed(appRoot, options = {}) {
  const platform = options.platform || process.platform;
  if (platform !== "darwin") {
    return { verified: false, reason: "not macOS" };
  }
  const infoPlist = join(appRoot, "Contents", "Info.plist");
  const identifier = run("plutil", ["-extract", "CFBundleIdentifier", "raw", "-o", "-", infoPlist]);
  if (identifier.status !== 0 || String(identifier.stdout || "").trim() !== "org.libreoffice.script") {
    throw new Error(`release LibreOffice seed has an unexpected bundle identifier: ${appRoot}`);
  }
  runRequired(
    "codesign",
    ["--verify", "--deep", "--strict", "--verbose=4", appRoot],
    `strictly verify pristine LibreOffice release seed ${appRoot}`,
  );
  const details = run("codesign", ["-d", "--verbose=4", appRoot]);
  const signingOutput = `${details.stdout || ""}\n${details.stderr || ""}`;
  const team = signingOutput.match(/^TeamIdentifier=(.+)$/m)?.[1]?.trim() || "";
  const expectedTeam = String(
    options.teamIdentifier || process.env.AWAKEN_RUNTIME_LIBREOFFICE_TEAM_ID || "7P5S3ZLCN7",
  ).trim();
  if (details.status !== 0 || team !== expectedTeam) {
    throw new Error(
      `release LibreOffice seed TeamIdentifier mismatch: expected ${expectedTeam}, got ${team || "missing"}`,
    );
  }
  return { verified: true, bundleIdentifier: "org.libreoffice.script", teamIdentifier: team };
}

/**
 * Re-signs a staged LibreOffice application inside-out. Development builds use
 * an ad-hoc identity by default. Release builds must opt in explicitly with a
 * Developer ID identity and pass both strict codesign and Gatekeeper checks.
 */
export function signMacOSRuntimeApplication(appRoot, options = {}) {
  const platform = options.platform || process.platform;
  if (platform !== "darwin") {
    return { signed: false, reason: "not macOS" };
  }
  assertStagedMacOSBundle(appRoot, options.ownershipRoot, options.sourceRoot);
  if (!which("codesign")) {
    throw new Error("codesign is required to prepare the macOS LibreOffice runtime");
  }

  const expectation = macOSRuntimeSigningExpectation(process.env, platform);
  const release = options.release ?? expectation.release;
  const identity = String(
    options.identity ?? expectation.identity,
  ).trim() || "-";
  if (release && identity === "-") {
    throw new Error(
      "release runtime signing requires AWAKEN_RUNTIME_CODESIGN_IDENTITY with a Developer ID Application identity",
    );
  }

  const signingMetadata = options.signingMetadata || captureMacOSRuntimeSigningMetadata(appRoot, { platform });
  if (!signingMetadata || signingMetadata.appRoot !== resolve(appRoot)) {
    disposeMacOSRuntimeSigningMetadata(signingMetadata);
    throw new Error(`macOS signing metadata does not belong to the staged application: ${appRoot}`);
  }
  const { bundles, machOFiles } = collectMacOSCode(appRoot);
  try {
    // Metadata may have been captured from the pristine copy before plist,
    // rpath, and Mach-O normalization. Only newly materialized code falls back
    // to its current signature.
    for (const path of [...machOFiles, ...bundles]) {
      if (!signingMetadata.entitlements.has(path)) {
        signingMetadata.entitlements.set(
          path,
          extractCodeEntitlements(path, signingMetadata.metadataDir),
        );
      }
    }
    for (const path of machOFiles) {
      signMacOSCode(path, signingMetadata.entitlements.get(path), identity, release);
    }
    for (const path of bundles) {
      signMacOSCode(path, signingMetadata.entitlements.get(path), identity, release);
    }
  } finally {
    disposeMacOSRuntimeSigningMetadata(signingMetadata);
  }

  const notarization = release
    ? notarizeMacOSRuntimeApplication(appRoot, { notaryProfile: options.notaryProfile })
    : { notarized: false, stapled: false };

  verifyMacOSRuntimeApplication(appRoot, {
    release,
    bundleIdentifier: options.bundleIdentifier,
  });
  return {
    signed: true,
    mode: release ? "developer-id" : "ad-hoc",
    identity,
    machOCount: machOFiles.length,
    bundleCount: bundles.length,
    strictVerified: true,
    gatekeeperVerified: release,
    notarized: notarization.notarized,
    stapled: notarization.stapled,
  };
}

export function inspectRuntimeProvenance(
  versionRootPath,
  ownershipRoot = versionRootPath,
  platform = process.platform,
) {
  const issues = [];
  const fileNames = platform === "darwin" ? collectRuntimeFileIndex(versionRootPath) : new Set();
  walkFiles(versionRootPath, (path, stat) => {
    if (stat.isSymbolicLink()) {
      let target = "";
      try {
        const raw = readlinkSync(path);
        target = isAbsolute(raw) ? resolve(raw) : resolve(dirname(path), raw);
      } catch {
        issues.push({ path, code: "broken_symlink", detail: "unable to resolve symlink" });
        return;
      }
      if (!exists(target)) {
        issues.push({ path, code: "broken_symlink", detail: target });
      } else if (!pathInsideRoot(target, ownershipRoot)) {
        issues.push({ path, code: "external_symlink", detail: target });
      }
      return;
    }
    if (!stat.isFile()) {
      return;
    }
    if (basenameSafe(path).toLowerCase() === "pyvenv.cfg") {
      issues.push({ path, code: "python_venv_not_relocatable", detail: "pyvenv.cfg is forbidden" });
    }
    if (platform !== "darwin" || !isMachOFile(path)) {
      return;
    }
    const installName = machOInstallName(path);
    if (installName && isAbsolute(installName) && !isAllowedSystemDependency(installName, platform)) {
      issues.push({
        path,
        code: isForbiddenExternalPath(installName) ? "forbidden_external_install_name" : "external_install_name",
        detail: installName,
      });
    }
    for (const dependency of machODependencies(path)) {
      // A dylib's LC_ID_DYLIB is reported by `otool -L` as the first row but
      // is metadata, not a load dependency. It may preserve a build-machine
      // install name without creating any runtime dependency.
      if (dependency === installName) {
        continue;
      }
      if (isAbsolute(dependency)) {
        if (isAllowedSystemDependency(dependency, platform)) {
          continue;
        }
        if (!exists(dependency)) {
          issues.push({ path, code: "missing_absolute_dependency", detail: dependency });
        } else if (!pathInsideRoot(dependency, ownershipRoot)) {
          issues.push({
            path,
            code: isForbiddenExternalPath(dependency) ? "forbidden_external_dependency" : "external_dependency",
            detail: dependency,
          });
        }
        continue;
      }
      if (dependency.startsWith("@loader_path/")) {
        const target = resolve(dirname(path), dependency.slice("@loader_path/".length));
        if (!exists(target)) {
          issues.push({ path, code: "missing_loader_dependency", detail: dependency });
        }
        continue;
      }
      if ((dependency.startsWith("@rpath/") || dependency.startsWith("@executable_path/")) &&
          !fileNames.has(basenameSafe(dependency))) {
        issues.push({ path, code: "missing_runtime_dependency", detail: dependency });
      }
    }
  });
  return { ok: issues.length === 0, issues };
}

export function readBashWrapperTarget(path) {
  if (!path || !exists(path) || isWindows) {
    return "";
  }
  let fd;
  let content = "";
  try {
    fd = openSync(path, "r");
    const buffer = Buffer.alloc(8192);
    const size = readSync(fd, buffer, 0, buffer.length, 0);
    content = buffer.subarray(0, size).toString("utf8");
  } catch {
    return "";
  } finally {
    if (fd !== undefined) {
      try {
        closeSync(fd);
      } catch {
        // best effort
      }
    }
  }
  const match = content.match(/^\s*exec\s+('(?:\\'|[^'])*'|"(?:\\"|[^"])*"|\S+)/m);
  if (!match) {
    return "";
  }
  let target = match[1];
  if ((target.startsWith("'") && target.endsWith("'")) || (target.startsWith('"') && target.endsWith('"'))) {
    target = target.slice(1, -1).replaceAll("\\'", "'").replaceAll('\\"', '"');
  }
  return target;
}

export function resolveManagedPath(path, root) {
  if (!path) {
    return { ok: false, reason: "empty path" };
  }
  let current = resolve(path);
  const seen = new Set();
  for (let i = 0; i < 8; i += 1) {
    if (seen.has(current)) {
      return { ok: false, reason: "symlink cycle" };
    }
    seen.add(current);
    if (!exists(current)) {
      return { ok: false, reason: "path missing", path: current };
    }
    let stat;
    try {
      stat = lstatSync(current);
    } catch {
      return { ok: false, reason: "stat failed", path: current };
    }
    if (stat.isSymbolicLink()) {
      let target;
      try {
        target = readlinkSync(current);
      } catch {
        return { ok: false, reason: "broken symlink", path: current };
      }
      current = isAbsolute(target) ? resolve(target) : resolve(dirname(current), target);
      if (!pathInsideRoot(current, root)) {
        return { ok: false, reason: "symlink escapes awaken root", path: current };
      }
      continue;
    }
    if (stat.isFile()) {
      const wrapperTarget = readBashWrapperTarget(current);
      if (wrapperTarget) {
        const resolvedTarget = isAbsolute(wrapperTarget)
          ? resolve(wrapperTarget)
          : resolve(dirname(current), wrapperTarget);
        if (!exists(resolvedTarget)) {
          return { ok: false, reason: "wrapper target missing", path: resolvedTarget };
        }
        if (!pathInsideRoot(resolvedTarget, root)) {
          return { ok: false, reason: "wrapper escapes awaken root", path: resolvedTarget };
        }
        current = resolvedTarget;
        continue;
      }
    }
    if (!pathInsideRoot(current, root)) {
      return { ok: false, reason: "path outside awaken root", path: current };
    }
    return { ok: true, path: current };
  }
  return { ok: false, reason: "resolve depth exceeded", path: current };
}

export function findSofficeExecutable(path) {
  return firstExecutable([
    path,
    join(path, "Contents", "MacOS", "soffice"),
    join(path, "program", "soffice.com"),
    join(path, "program", "soffice.bin"),
    join(path, "program", "soffice.bin.exe"),
    join(path, "program", "soffice"),
    join(path, "program", "soffice.exe"),
    join(path, "soffice.bin"),
    join(path, "soffice.bin.exe"),
    join(path, "soffice"),
    join(path, "soffice.exe"),
  ]);
}

export function resolveLibreOfficeInstallRoot(source, platform = process.platform) {
  if (!source || !exists(source)) {
    return "";
  }
  let resolvedSource;
  try {
    resolvedSource = realpathSync(source);
  } catch {
    resolvedSource = resolve(source);
  }
  if (platform === "darwin") {
    const marker = "LibreOffice.app";
    const markerIndex = resolvedSource.indexOf(marker);
    if (markerIndex >= 0) {
      return resolvedSource.slice(0, markerIndex + marker.length);
    }
    const nested = join(resolvedSource, marker);
    return exists(nested) ? nested : "";
  }

  const sourceStat = statSync(resolvedSource);
  const parent = sourceStat.isDirectory() ? resolvedSource : dirname(resolvedSource);
  if (platform === "win32" || platform === "windows") {
    const root = basenameSafe(parent).toLowerCase() === "program" ? dirname(parent) : parent;
    return exists(join(root, "program")) ? root : "";
  }

  const root = basenameSafe(parent).toLowerCase() === "program" ? dirname(parent) : parent;
  const normalized = resolve(root);
  if (["/", "/usr", "/usr/lib", "/lib"].includes(normalized) ||
      !basenameSafe(normalized).toLowerCase().includes("libreoffice") ||
      !exists(join(normalized, "program", "soffice"))) {
    return "";
  }
  return normalized;
}

export function componentPath(manifest, name, versionRoot) {
  const component = manifest.components.find((item) => item.name === name);
  if (!component) {
    return "";
  }
  const rel = componentRelativePath(component);
  return versionRoot ? join(versionRoot, rel) : resolveRuntimePath(rel);
}

export function componentExists(path, kind) {
  if (!exists(path)) {
    return false;
  }
  const stat = statSync(path);
  switch (kind) {
    case "directory":
    case "application":
      return stat.isDirectory() || (kind === "application" && stat.isFile());
    default:
      return stat.isFile();
  }
}

function basenameSafe(filePath) {
  const parts = String(filePath).split(/[/\\]/);
  return parts[parts.length - 1] || "file";
}

export function writeJsonAtomic(filePath, value) {
  ensureDir(dirname(filePath));
  const tempPath = join(dirname(filePath), `.${basenameSafe(filePath)}.${process.pid}.${Date.now()}.tmp`);
  const payload = `${JSON.stringify(value, null, 2)}\n`;
  const fd = openSync(tempPath, "w");
  try {
    writeSync(fd, payload, 0, "utf8");
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
  renameSync(tempPath, filePath);
}

export function readCurrentJson(root) {
  const path = join(root, CURRENT_JSON);
  if (!exists(path)) {
    return null;
  }
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch {
    return null;
  }
}

export function writeCurrentJson(root, current) {
  writeJsonAtomic(join(root, CURRENT_JSON), {
    schemaVersion: 2,
    runtimeName: "primary-runtime",
    active: current.active || "",
    previous: current.previous || "",
    updatedAt: current.updatedAt || new Date().toISOString(),
  });
}

export function readResolvedState(versionRootPath) {
  const path = join(versionRootPath, STATE_JSON);
  if (!exists(path)) {
    return null;
  }
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch {
    return null;
  }
}

export function writeResolvedState(versionRootPath, state) {
  writeJsonAtomic(join(versionRootPath, STATE_JSON), state);
}

export function buildResolvedState({
  manifest,
  versionRootPath,
  platform = process.platform,
  arch = process.arch,
  sources = {},
}) {
  const components = (manifest.components || []).map((component) => {
    const relativePath = componentRelativePath(component, platform);
    return {
      name: component.name,
      kind: component.kind,
      required: Boolean(component.required),
      version: String(component.version || "managed"),
      source: String(sources[component.name] || component.source || "awaken_managed"),
      relativePath,
      probe: component.probe || null,
    };
  });
  return {
    schemaVersion: 2,
    runtimeName: manifest.runtimeName || manifest.name || "primary-runtime",
    bundleVersion: manifest.bundleVersion || manifest.version || "0.2.0",
    platform: platform === "win32" ? "windows" : platform,
    arch,
    root: versionRootPath,
    components,
    officeRender: manifest.officeRenderWorker || undefined,
    generatedAt: new Date().toISOString(),
  };
}

function probeArgsAccepted(status, accepted = [0]) {
  return accepted.includes(status);
}


export function probeComponent(component, absolutePath, root, options = {}) {
  const managed = resolveManagedPath(absolutePath, root);
  if (!managed.ok) {
    return {
      name: component.name,
      kind: component.kind,
      required: Boolean(component.required),
      ok: false,
      path: absolutePath,
      detail: managed.reason,
      resolvedPath: managed.path || "",
    };
  }
  const path = managed.path;
  const status = {
    name: component.name,
    kind: component.kind,
    required: Boolean(component.required),
    path: absolutePath,
    resolvedPath: path,
    ok: false,
    detail: "",
  };

  const probe = component.probe || {};
  const accepted = Array.isArray(probe.acceptedExitCodes) ? probe.acceptedExitCodes : [0];

  switch (component.name) {
    case "python": {
      if (!exists(path) || !statSync(path).isFile()) {
        status.detail = "executable missing";
        return status;
      }
      const args = probe.args || ["-c", "import pptx, PIL"];
      const result = run(path, args, options);
      status.ok = probeArgsAccepted(result.status, accepted);
      status.detail = status.ok ? "python-pptx and Pillow available" : "python probe failed";
      if (status.ok && probe.requireManagedPrefix !== false) {
        const prefixProbe = run(path, [
          "-c",
          "import json,sys; print(json.dumps({'executable':sys.executable,'prefix':sys.prefix,'base_prefix':sys.base_prefix}))",
        ], options);
        let prefixes = null;
        try {
          prefixes = JSON.parse(String(prefixProbe.stdout || "").trim());
        } catch {
          prefixes = null;
        }
        const paths = prefixes ? [prefixes.executable, prefixes.prefix, prefixes.base_prefix] : [];
        if (prefixProbe.status !== 0 || paths.length !== 3 || paths.some((entry) => !pathInsideRoot(entry, root))) {
          status.ok = false;
          status.detail = "python prefix/base_prefix escapes managed runtime";
        }
      }
      break;
    }
    case "node": {
      if (!exists(path) || !statSync(path).isFile()) {
        status.detail = "executable missing";
        return status;
      }
      const args = probe.args || ["--version"];
      const result = run(path, args, options);
      status.ok = probeArgsAccepted(result.status, accepted);
      status.detail = status.ok ? (commandOutput(path, args) || "node ok") : "node did not execute";
      break;
    }
    case "libreoffice": {
      let soffice = findSofficeExecutable(path);
      if (!soffice && probe.executableRelativePath) {
        soffice = join(path, probe.executableRelativePath);
      }
      if (soffice) {
        const managedSoffice = resolveManagedPath(soffice, root);
        if (!managedSoffice.ok) {
          status.detail = managedSoffice.reason;
          break;
        }
        soffice = managedSoffice.path;
      }
      const args = probe.args || ["--version"];
      status.ok = Boolean(soffice) && probeArgsAccepted(run(soffice, args, options).status, accepted);
      status.detail = status.ok ? (commandOutput(soffice, args) || "soffice ok") : "soffice not executable";
      status.resolvedPath = soffice || path;
      break;
    }
    case "poppler": {
      if (!exists(path)) {
        status.detail = "poppler bin missing";
        return status;
      }
      const tools = probe.tools || ["pdftoppm", "pdfinfo"];
      const results = tools.map((name) => {
        const toolPath = join(path, isWindows ? `${name}.exe` : name);
        const managedTool = resolveManagedPath(toolPath, root);
        if (!managedTool.ok) {
          return false;
        }
        const args = name === "pdfinfo" ? ["-v"] : ["-h"];
        const result = run(managedTool.path, args, options);
        return result.status === 0 || result.status === 1 || String(result.stderr || "").includes("Usage") || String(result.stdout || "").includes("Usage");
      });
      status.ok = results.every(Boolean);
      status.detail = status.ok ? "pdftoppm and pdfinfo available" : "missing pdftoppm or pdfinfo";
      break;
    }
    case "ffmpeg": {
      if (!exists(path) || !statSync(path).isFile()) {
        status.detail = "executable missing";
        return status;
      }
      const args = probe.args || ["-version"];
      const result = run(path, args, options);
      status.ok = probeArgsAccepted(result.status, accepted);
      status.detail = status.ok ? "ffmpeg available" : "ffmpeg did not execute";
      break;
    }
    case "helper-packages": {
      const requiredFile = probe.requireFile || "runtime.json";
      status.ok = exists(join(path, requiredFile));
      status.detail = status.ok ? "helper package directory available" : `missing ${requiredFile}`;
      break;
    }
    default: {
      status.ok = exists(path);
      status.detail = status.ok ? "component path exists" : "component missing";
      break;
    }
  }
  return status;
}

export function checkPrimaryRuntime(options = {}) {
  const env = options.env || process.env;
  const platform = options.platform || process.platform;
  const arch = options.arch || process.arch;
  const root = options.root || resolvePrimaryRuntimeRoot(env, platform);
  const manifestPath = options.manifestPath || runtimeManifestPath;
  const template = options.manifest || readRuntimeManifest(manifestPath);
  const current = readCurrentJson(root);
  const components = [];
  const missing = [];

  if (!current?.active) {
    return {
      schemaVersion: 2,
      runtimeName: template.runtimeName || template.name || "primary-runtime",
      bundleVersion: template.bundleVersion || template.version || "",
      manifestPath,
      runtimeRoot: root,
      runtimeDir: root,
      ready: false,
      missing: ["current"],
      components: [],
      active: "",
      previous: "",
      detail: "current.json missing or inactive",
    };
  }

  const activeKey = current.active;
  const activeRoot = versionDir(root, activeKey);
  const state = readResolvedState(activeRoot) || buildResolvedState({
    manifest: template,
    versionRootPath: activeRoot,
    platform,
    arch,
  });

  if (!pathInsideRoot(activeRoot, root) && exists(activeRoot)) {
    // versionDir is always under root by construction; still guard foreign active keys
  }
  if (!exists(activeRoot) || !String(activeKey).includes("/") || activeKey.includes("..")) {
    return {
      schemaVersion: 2,
      runtimeName: state.runtimeName,
      bundleVersion: state.bundleVersion,
      manifestPath,
      runtimeRoot: root,
      runtimeDir: root,
      ready: false,
      missing: ["active_version"],
      components: [],
      active: activeKey,
      previous: current.previous || "",
      detail: "active version missing or invalid",
    };
  }

  const componentDefs = state.components?.length
    ? state.components.map((item) => {
        const templateComponent = (template.components || []).find((c) => c.name === item.name) || {};
        return { ...templateComponent, ...item };
      })
    : template.components || [];

  for (const component of componentDefs) {
    const absolutePath = join(activeRoot, component.relativePath || componentRelativePath(component, platform));
    const status = probeComponent(component, absolutePath, root, options.probeOptions || {});
    if (component.required && !status.ok) {
      missing.push(component.name);
    }
    components.push(status);
  }

  const provenance = inspectRuntimeProvenance(activeRoot, activeRoot, platform);
  if (!provenance.ok) {
    missing.push("provenance");
  }

  return {
    schemaVersion: 2,
    runtimeName: state.runtimeName || "primary-runtime",
    bundleVersion: state.bundleVersion || "",
    platform: state.platform || (platform === "win32" ? "windows" : platform),
    arch: state.arch || arch,
    security: state.security || {},
    manifestPath,
    runtimeRoot: root,
    runtimeDir: root,
    versionRoot: activeRoot,
    ready: missing.length === 0,
    missing,
    components,
    active: activeKey,
    previous: current.previous || "",
    provenance,
  };
}

export function readMigrationMarker(env = process.env, platform = process.platform) {
  const path = migrationMarkerPath(env, platform);
  if (!exists(path)) {
    return null;
  }
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch {
    return null;
  }
}

export function writeMigrationMarker(result, env = process.env, platform = process.platform) {
  const path = migrationMarkerPath(env, platform);
  writeJsonAtomic(path, {
    schemaVersion: 2,
    name: MIGRATION_MARKER_NAME,
    completedAt: new Date().toISOString(),
    ...result,
  });
}


export function inspectMigrationComponent(sourcePath, sourceRoot) {
  if (!exists(sourcePath)) {
    return { ok: false, reason: "missing" };
  }
  if (isCodexCachePath(sourcePath)) {
    return { ok: false, reason: "codex cache" };
  }
  try {
    const stat = lstatSync(sourcePath);
    if (stat.isSymbolicLink()) {
      const target = readlinkSync(sourcePath);
      const resolved = isAbsolute(target) ? resolve(target) : resolve(dirname(sourcePath), target);
      if (!exists(resolved)) {
        return { ok: false, reason: "broken symlink" };
      }
      if (isCodexCachePath(resolved)) {
        return { ok: false, reason: "symlink to codex cache" };
      }
      if (sourceRoot && !pathInsideRoot(resolved, sourceRoot)) {
        return { ok: false, reason: "symlink escapes migration source root" };
      }
    }
  } catch (error) {
    return { ok: false, reason: error.message };
  }
  const wrapperTarget = readBashWrapperTarget(sourcePath);
  if (wrapperTarget) {
    const resolvedTarget = isAbsolute(wrapperTarget)
      ? resolve(wrapperTarget)
      : resolve(dirname(sourcePath), wrapperTarget);
    if (!exists(resolvedTarget)) {
      return { ok: false, reason: "wrapper target missing" };
    }
    if (isCodexCachePath(resolvedTarget)) {
      return { ok: false, reason: "wrapper to codex cache" };
    }
    if (sourceRoot && !pathInsideRoot(resolvedTarget, sourceRoot)) {
      return { ok: false, reason: "wrapper escapes migration source root" };
    }
  }
  return { ok: true, path: sourcePath };
}

function migrationSourceForComponent(dependenciesRoot, source, component, platform) {
  const rel = componentRelativePath(component, platform);
  return firstExisting([
    join(dependenciesRoot, rel),
    join(source, rel),
    join(dependenciesRoot, component.name),
    join(source, "dependencies", rel),
  ]);
}

export function migrateOldAwakenCacheIfNeeded(options = {}) {
  const env = options.env || process.env;
  const platform = options.platform || process.platform;
  const arch = options.arch || process.arch;
  const root = options.root || resolvePrimaryRuntimeRoot(env, platform);
  const marker = readMigrationMarker(env, platform);
  if (marker) {
    return { migrated: false, skipped: true, reason: "migration marker present", marker };
  }
  const current = readCurrentJson(root);
  if (current?.active) {
    writeMigrationMarker({ status: "skipped", reason: "current.json already present", active: current.active }, env, platform);
    return { migrated: false, skipped: true, reason: "current already present" };
  }

  const candidates = oldAwakenCacheCandidates(env);
  let source = "";
  let rejectReason = "";
  for (const candidate of candidates) {
    if (isCodexCachePath(candidate)) {
      rejectReason = "codex cache rejected";
      continue;
    }
    if (exists(candidate)) {
      source = candidate;
      break;
    }
  }
  if (!source) {
    writeMigrationMarker({
      status: "skipped",
      reason: rejectReason || "no old awaken cache found",
    }, env, platform);
    return { migrated: false, skipped: true, reason: rejectReason || "no old awaken cache found" };
  }

  const dependenciesRoot = exists(join(source, "dependencies")) ? join(source, "dependencies") : source;
  const template = options.manifest || readRuntimeManifest();
  const stageBase = join(stagingRoot(root), `migrate-${Date.now()}`);
  ensureDir(stageBase);

  const rejected = [];
  const copied = [];
  try {
    for (const component of template.components || []) {
      const rel = componentRelativePath(component, platform);
      const from = migrationSourceForComponent(dependenciesRoot, source, component, platform);
      const to = join(stageBase, rel);
      if (!from) {
        rejected.push({ name: component.name, reason: "missing" });
        continue;
      }
      const inspection = inspectMigrationComponent(from, dependenciesRoot);
      if (!inspection.ok) {
        rejected.push({ name: component.name, reason: inspection.reason });
        continue;
      }
      ensureDir(dirname(to));
      const st = statSync(from);
      if (st.isDirectory()) {
        copyDirectoryContents(from, to);
      } else {
        copyExecutable(to, from, component.name);
      }
      const probe = probeComponent(component, to, stageBase);
      if (!probe.ok) {
        rejected.push({ name: component.name, reason: probe.detail || "probe failed" });
        rmSync(to, { recursive: true, force: true });
        continue;
      }
      copied.push(component.name);
    }

    const required = (template.components || []).filter((c) => c.required).map((c) => c.name);
    const missingRequired = required.filter((name) => !copied.includes(name));
    if (missingRequired.length > 0) {
      writeMigrationMarker({
        status: "failed",
        source,
        rejected,
        missingRequired,
      }, env, platform);
      rmSync(stageBase, { recursive: true, force: true });
      return { migrated: false, failed: true, rejected, missingRequired, source };
    }

    const provenance = inspectRuntimeProvenance(stageBase, stageBase, platform);
    if (!provenance.ok) {
      writeMigrationMarker({
        status: "failed",
        source,
        rejected,
        missingRequired: ["provenance"],
        provenance: provenance.issues.slice(0, 20),
      }, env, platform);
      rmSync(stageBase, { recursive: true, force: true });
      return {
        migrated: false,
        failed: true,
        rejected,
        missingRequired: ["provenance"],
        provenance,
        source,
      };
    }

    const bundleVersion = template.bundleVersion || template.version || "0.2.0";
    const key = versionKey(
      bundleVersion,
      platform,
      arch,
      `migration-${Date.now().toString(36)}`,
    );
    const target = publishStaging(root, stageBase, key);
    const state = buildResolvedState({
      manifest: template,
      versionRootPath: target,
      platform,
      arch,
      sources: Object.fromEntries(copied.map((name) => [name, "awaken_cache_migration"])),
    });
    writeResolvedState(target, state);
    writeCurrentJson(root, { active: key, previous: "" });
    writeMigrationMarker({
      status: "success",
      source,
      active: key,
      copied,
      rejected,
    }, env, platform);
    return { migrated: true, active: key, copied, rejected, source };
  } catch (error) {
    rmSync(stageBase, { recursive: true, force: true });
    writeMigrationMarker({
      status: "failed",
      source,
      error: error.message,
    }, env, platform);
    return { migrated: false, failed: true, error: error.message, source };
  }
}

export function activateVersion(root, key, options = {}) {
  const current = readCurrentJson(root);
  const previous = options.retainPrevious === false
    ? ""
    : (current?.active && current.active !== key ? current.active : (current?.previous || ""));
  writeCurrentJson(root, {
    active: key,
    previous,
    updatedAt: new Date().toISOString(),
  });
  return { active: key, previous };
}

export function rollbackToPrevious(root) {
  const current = readCurrentJson(root);
  if (!current?.previous) {
    return { ok: false, reason: "no previous known-good" };
  }
  const previous = current.previous;
  const previousRoot = versionDir(root, previous);
  if (!exists(previousRoot)) {
    return { ok: false, reason: "previous version missing" };
  }
  writeCurrentJson(root, {
    active: previous,
    previous: "",
    updatedAt: new Date().toISOString(),
  });
  return { ok: true, active: previous };
}

export function createStagingDir(root, label = "prepare") {
  const dir = join(stagingRoot(root), `${label}-${process.pid}-${Date.now()}`);
  ensureDir(dir);
  return dir;
}

export function publishStaging(root, stagingDir, key) {
  const target = versionDir(root, key);
  ensureDir(dirname(target));
  if (exists(target)) {
    throw new Error(`immutable runtime version already exists: ${key}`);
  }
  if (isWindows) {
    // Windows may hold transient locks on freshly copied/executed files (e.g.
    // antivirus indexing, pip install handles). Retry the directory rename with
    // a short backoff instead of failing immediately.
    const maxAttempts = 20;
    const initialDelayMs = 50;
    let lastError = null;
    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        renameSync(stagingDir, target);
        return target;
      } catch (error) {
        lastError = error;
        if (error.code !== "EPERM" && error.code !== "EACCES") {
          throw error;
        }
        const delay = initialDelayMs * 2 ** attempt;
        Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, Math.min(delay, 5000));
      }
    }
    throw lastError;
  }
  renameSync(stagingDir, target);
  return target;
}

export function cleanupStaging(stagingDir) {
  if (stagingDir && exists(stagingDir)) {
    const pending = [stagingDir];
    while (pending.length > 0) {
      const current = pending.pop();
      let stat;
      try {
        stat = lstatSync(current);
      } catch {
        continue;
      }
      if (stat.isSymbolicLink()) {
        continue;
      }
      if (stat.isDirectory()) {
        try {
          chmodSync(current, (stat.mode & 0o777) | 0o700);
        } catch {
          // rmSync below remains the source of truth for cleanup failure.
        }
        for (const entry of readdirSync(current)) {
          pending.push(join(current, entry));
        }
      } else if (stat.isFile()) {
        try {
          chmodSync(current, (stat.mode & 0o777) | 0o200);
        } catch {
          // Best effort before recursive removal.
        }
      }
    }
    rmSync(stagingDir, { recursive: true, force: true });
  }
}

export function listVersionKeys(root) {
  const versionsRoot = join(root, "versions");
  if (!exists(versionsRoot)) {
    return [];
  }
  const keys = [];
  for (const bundle of readdirSync(versionsRoot, { withFileTypes: true })) {
    if (!bundle.isDirectory()) continue;
    const bundleDir = join(versionsRoot, bundle.name);
    for (const plat of readdirSync(bundleDir, { withFileTypes: true })) {
      if (!plat.isDirectory()) continue;
      const platformDir = join(bundleDir, plat.name);
      const stateAtPlatform = exists(join(platformDir, STATE_JSON));
      if (stateAtPlatform) {
        keys.push(`${bundle.name}/${plat.name}`);
        continue;
      }
      for (const revision of readdirSync(platformDir, { withFileTypes: true })) {
        if (revision.isDirectory()) {
          keys.push(`${bundle.name}/${plat.name}/${revision.name}`);
        }
      }
    }
  }
  return keys;
}

export function defaultTempRoot() {
  return join(tmpdir(), `awaken-primary-runtime-${process.pid}`);
}
