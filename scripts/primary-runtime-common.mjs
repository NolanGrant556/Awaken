import { execFileSync, spawnSync } from "node:child_process";
import {
  accessSync,
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  statSync,
  writeFileSync,
  constants,
} from "node:fs";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));

export const rootDir = resolve(scriptDir, "..");
export const desktopResourcesDir = join(rootDir, "apps", "desktop", "resources");
export const runtimeDir = join(desktopResourcesDir, "runtime");
export const runtimeManifestPath = join(
  desktopResourcesDir,
  "runtime-manifest",
  "primary-runtime.json",
);

export function readRuntimeManifest() {
  return JSON.parse(readFileSync(runtimeManifestPath, "utf8"));
}

export function resolveRuntimePath(componentPath) {
  if (!componentPath) {
    return "";
  }
  if (isAbsolute(componentPath)) {
    return componentPath;
  }
  return resolve(dirname(runtimeManifestPath), componentPath);
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

export function which(command) {
  const output = commandOutput("which", [command]);
  return output.split("\n")[0]?.trim() || "";
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

export function fileSize(path) {
  try {
    return statSync(path).size;
  } catch {
    return 0;
  }
}

export function candidateSeedDirs() {
  const dirs = [];
  if (process.env.AWAKEN_PRIMARY_RUNTIME_SEED_DIR) {
    dirs.push(process.env.AWAKEN_PRIMARY_RUNTIME_SEED_DIR);
  }
  if (process.env.AWAKEN_PRIMARY_RUNTIME_CACHE_DIR) {
    dirs.push(process.env.AWAKEN_PRIMARY_RUNTIME_CACHE_DIR);
  }
  if (process.env.HOME) {
    dirs.push(
      join(
        process.env.HOME,
        ".cache",
        "codex-runtimes",
        "codex-primary-runtime",
        "dependencies",
      ),
    );
    dirs.push(join(process.env.HOME, ".cache", "awaken-runtimes", "primary-runtime", "dependencies"));
  }
  return dirs.filter(exists);
}

export function findSofficeExecutable(path) {
  return firstExecutable([
    path,
    join(path, "Contents", "MacOS", "soffice"),
    join(path, "program", "soffice"),
    join(path, "soffice"),
    join(path, "soffice.exe"),
  ]);
}

export function componentPath(manifest, name) {
  const component = manifest.components.find((item) => item.name === name);
  return component ? resolveRuntimePath(component.path) : "";
}

export function componentExists(path, kind) {
  if (!exists(path)) {
    return false;
  }
  const stat = statSync(path);
  switch (kind) {
    case "directory":
    case "application":
      return stat.isDirectory();
    default:
      return stat.isFile();
  }
}

export function checkPrimaryRuntime() {
  const manifest = readRuntimeManifest();
  const components = [];
  const missing = [];

  for (const component of manifest.components) {
    const path = resolveRuntimePath(component.path);
    const status = {
      name: component.name,
      kind: component.kind,
      required: Boolean(component.required),
      path,
      ok: componentExists(path, component.kind),
      detail: "",
    };

    if (status.ok) {
      switch (component.name) {
        case "python":
          status.ok = runOk(path, ["-c", "import pptx, PIL"]);
          status.detail = status.ok ? "python-pptx and Pillow available" : "missing python-pptx or Pillow";
          break;
        case "node":
          status.ok = runOk(path, ["--version"]);
          status.detail = status.ok ? commandOutput(path, ["--version"]) : "node did not execute";
          break;
        case "libreoffice": {
          const soffice = findSofficeExecutable(path);
          status.ok = Boolean(soffice) && runOk(soffice, ["--version"]);
          status.detail = status.ok ? commandOutput(soffice, ["--version"]) : "soffice not executable";
          break;
        }
        case "poppler": {
          const pdftoppm = join(path, "pdftoppm");
          const pdfinfo = join(path, "pdfinfo");
          status.ok = runOk(pdftoppm, ["-h"]) && runOk(pdfinfo, ["-v"]);
          status.detail = status.ok ? "pdftoppm and pdfinfo available" : "missing pdftoppm or pdfinfo";
          break;
        }
        case "ffmpeg":
          status.ok = runOk(path, ["-version"]);
          status.detail = status.ok ? "ffmpeg available" : "ffmpeg did not execute";
          break;
        case "helper-packages":
          status.detail = "helper package directory available";
          break;
        default:
          status.detail = "component path exists";
          break;
      }
    }

    if (component.required && !status.ok) {
      missing.push(component.name);
    }
    components.push(status);
  }

  return {
    runtimeName: manifest.name || "primary-runtime",
    version: manifest.version || "",
    manifestPath: runtimeManifestPath,
    runtimeDir,
    ready: missing.length === 0,
    missing,
    components,
  };
}
