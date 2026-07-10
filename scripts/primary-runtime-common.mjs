import { execFileSync, spawnSync } from "node:child_process";
import {
  accessSync,
  chmodSync,
  closeSync,
  copyFileSync,
  cpSync,
  existsSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  readlinkSync,
  readdirSync,
  realpathSync,
  renameSync,
  rmSync,
  statSync,
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

export function versionKey(bundleVersion, platform = process.platform, arch = process.arch) {
  return `${bundleVersion}/${platformArchKey(platform, arch)}`;
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
  ensureDir(target);
  cpSync(source, target, { recursive: true, dereference: true });
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

export function pathInsideRoot(path, root) {
  if (!path || !root) {
    return false;
  }
  let resolvedPath;
  let resolvedRoot;
  try {
    resolvedRoot = realpathSync(resolve(root));
  } catch {
    resolvedRoot = resolve(root);
  }
  try {
    resolvedPath = realpathSync(resolve(path));
  } catch {
    resolvedPath = resolve(path);
  }
  const rel = relative(resolvedRoot, resolvedPath);
  return rel === "" || (!rel.startsWith(`..${sep}`) && rel !== ".." && !isAbsolute(rel));
}

export function readBashWrapperTarget(path) {
  if (!path || !exists(path) || isWindows) {
    return "";
  }
  let content = "";
  try {
    content = readFileSync(path, "utf8");
  } catch {
    return "";
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

  return {
    schemaVersion: 2,
    runtimeName: state.runtimeName || "primary-runtime",
    bundleVersion: state.bundleVersion || "",
    platform: state.platform || (platform === "win32" ? "windows" : platform),
    arch: state.arch || arch,
    manifestPath,
    runtimeRoot: root,
    runtimeDir: root,
    versionRoot: activeRoot,
    ready: missing.length === 0,
    missing,
    components,
    active: activeKey,
    previous: current.previous || "",
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

    const bundleVersion = template.bundleVersion || template.version || "0.2.0";
    const key = versionKey(bundleVersion, platform, arch);
    const target = versionDir(root, key);
    ensureDir(dirname(target));
    if (exists(target)) {
      rmSync(target, { recursive: true, force: true });
    }
    renameSync(stageBase, target);
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
    rmSync(target, { recursive: true, force: true });
  }
  renameSync(stagingDir, target);
  return target;
}

export function cleanupStaging(stagingDir) {
  if (stagingDir && exists(stagingDir)) {
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
      keys.push(`${bundle.name}/${plat.name}`);
    }
  }
  return keys;
}

export function defaultTempRoot() {
  return join(tmpdir(), `awaken-primary-runtime-${process.pid}`);
}
