#!/usr/bin/env node

import { createHash, randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  accessSync,
  chmodSync,
  constants,
  createReadStream,
  createWriteStream,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmSync,
  statSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { Readable } from "node:stream";
import { pipeline } from "node:stream/promises";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(scriptPath), "..");
export const manifestPath = join(
  repoRoot,
  "apps",
  "desktop",
  "resources",
  "node-runtime.json",
);

export function readNodeRuntimeManifest(path = manifestPath) {
  const manifest = JSON.parse(readFileSync(path, "utf8"));
  if (
    typeof manifest.version !== "string" ||
    !/^\d+\.\d+\.\d+$/.test(manifest.version) ||
    typeof manifest.license !== "string" ||
    typeof manifest.releaseBaseUrl !== "string" ||
    !manifest.releaseBaseUrl.startsWith("https://nodejs.org/dist/") ||
    !manifest.targets ||
    typeof manifest.targets !== "object"
  ) {
    throw new Error("invalid Node runtime release manifest");
  }
  const validFormats = new Set(["tar.gz", "tar.xz", "zip"]);
  for (const [target, entry] of Object.entries(manifest.targets)) {
    if (
      !["darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "win32-arm64", "win32-x64"].includes(target) ||
      typeof entry?.asset !== "string" ||
      !validFormats.has(entry?.format) ||
      typeof entry?.sha256 !== "string" ||
      !/^[a-f0-9]{64}$/.test(entry.sha256)
    ) {
      throw new Error(`invalid Node runtime target entry: ${target}`);
    }
  }
  return manifest;
}

export function nodeRuntimeTarget(platform = process.platform, arch = process.arch) {
  const target = `${platform}-${arch}`;
  if (!new Set(["darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "win32-arm64", "win32-x64"]).has(target)) {
    throw new Error(`Node runtime is not available for ${target}`);
  }
  return target;
}

export function nodeBinaryName(target) {
  return target.startsWith("win32-") ? "node.exe" : "node";
}

export function nodeBinaryPath(target) {
  const relativeBinaryDir = target.startsWith("win32-") ? [] : ["bin"];
  return join(
    repoRoot,
    "apps",
    "desktop",
    "resources",
    "bin",
    target,
    "node",
    ...relativeBinaryDir,
    nodeBinaryName(target),
  );
}

export function nodeRuntimeDirectory(target) {
  return join(repoRoot, "apps", "desktop", "resources", "bin", target, "node");
}

export function sha256File(path) {
  const hash = createHash("sha256");
  return new Promise((resolveHash, reject) => {
    const stream = createReadStream(path);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("error", reject);
    stream.on("end", () => resolveHash(hash.digest("hex")));
  });
}

function requestedTargets(manifest) {
  const argument = process.argv.find((value) => value.startsWith("--targets="));
  const configured = argument?.slice("--targets=".length) || process.env.AWAKEN_BINARY_TARGETS;
  const defaults =
    process.platform === "darwin"
      ? ["darwin-arm64", "darwin-x64"]
      : [nodeRuntimeTarget()];
  const targets = configured
    ? configured.split(",").map((value) => value.trim()).filter(Boolean)
    : defaults;
  for (const target of targets) {
    if (!manifest.targets[target]) {
      throw new Error(`Node runtime manifest has no target ${target}`);
    }
  }
  return [...new Set(targets)];
}

async function downloadAsset(url, outputPath) {
  const response = await fetch(url, {
    redirect: "follow",
    signal: AbortSignal.timeout(120_000),
  });
  if (!response.ok || !response.body) {
    throw new Error(`download failed (${response.status}) for ${url}`);
  }
  mkdirSync(dirname(outputPath), { recursive: true });
  await pipeline(Readable.fromWeb(response.body), createWriteStream(outputPath));
}

function canExecuteTarget(target) {
  return target === `${process.platform}-${process.arch}`;
}

function verifyExecutable(binaryPath, expectedVersion) {
  accessSync(binaryPath, constants.X_OK);
  const output = String(execFileSync(binaryPath, ["--version"], { encoding: "utf8" })).trim();
  if (!output.includes(expectedVersion)) {
    throw new Error(
      `${basename(binaryPath)} version mismatch: expected ${expectedVersion}, received ${output}`,
    );
  }
}

function extractArchive(archivePath, outputDir, format) {
  mkdirSync(outputDir, { recursive: true });
  if (format === "zip") {
    execFileSync("unzip", ["-q", "-o", archivePath, "-d", outputDir], { stdio: "inherit" });
  } else if (format === "tar.xz") {
    execFileSync("tar", ["-xJf", archivePath, "-C", outputDir], { stdio: "inherit" });
  } else {
    // tar.gz
    execFileSync("tar", ["-xzf", archivePath, "-C", outputDir], { stdio: "inherit" });
  }

  // Node archives contain a single top-level directory like
  // node-v22.15.0-darwin-arm64. Move its contents up one level so the
  // consumer can use a stable <target>/node path.
  const entries = readdirSync(outputDir, { withFileTypes: true });
  const topDirs = entries.filter((entry) => entry.isDirectory());
  if (topDirs.length === 1 && topDirs[0].name.startsWith("node-v")) {
    const nested = join(outputDir, topDirs[0].name);
    for (const entry of readdirSync(nested, { withFileTypes: true })) {
      renameSync(join(nested, entry.name), join(outputDir, entry.name));
    }
    rmSync(nested, { recursive: true, force: true });
  }
}

export async function prepareNodeRuntime({ targets, verify = true } = {}) {
  const manifest = readNodeRuntimeManifest();
  const selectedTargets = targets ?? requestedTargets(manifest);

  for (const target of selectedTargets) {
    const entry = manifest.targets[target];
    if (!entry) throw new Error(`Node runtime manifest has no target ${target}`);

    const outputDir = nodeRuntimeDirectory(target);
    const binaryPath = nodeBinaryPath(target);

    let needsInstall = true;
    if (existsSync(binaryPath)) {
      const binaryStat = statSync(binaryPath);
      if (binaryStat.size > 0) {
        needsInstall = false;
      }
    }

    if (needsInstall) {
      const url = `${manifest.releaseBaseUrl}/${entry.asset}`;
      const temporaryPath = join(tmpdir(), `awaken-node-runtime-${target}-${process.pid}-${randomUUID()}.${entry.format === "zip" ? "zip" : "tar.gz"}`);
      try {
        console.log(`[prepare-node-runtime] downloading ${target} ${entry.asset}`);
        await downloadAsset(url, temporaryPath);
        const checksum = await sha256File(temporaryPath);
        if (checksum !== entry.sha256) {
          throw new Error(
            `Node runtime checksum mismatch for ${target}: expected ${entry.sha256}, received ${checksum}`,
          );
        }
        rmSync(outputDir, { recursive: true, force: true });
        mkdirSync(dirname(outputDir), { recursive: true });
        console.log(`[prepare-node-runtime] extracting ${target}`);
        extractArchive(temporaryPath, outputDir, entry.format);
      } finally {
        rmSync(temporaryPath, { force: true });
      }
    }

    if (!target.startsWith("win32-")) chmodSync(binaryPath, 0o755);

    const verifyAll = process.env.AWAKEN_NODE_RUNTIME_VERIFY_ALL === "1";
    if (verify && (canExecuteTarget(target) || verifyAll)) {
      verifyExecutable(binaryPath, manifest.version);
    }
    console.log(`[prepare-node-runtime] ready ${target} -> ${binaryPath}`);
  }

  return { version: manifest.version, targets: selectedTargets };
}

if (resolve(process.argv[1] || "") === scriptPath) {
  prepareNodeRuntime().catch((error) => {
    console.error(
      `[prepare-node-runtime] ${error instanceof Error ? error.message : String(error)}`,
    );
    process.exitCode = 1;
  });
}
