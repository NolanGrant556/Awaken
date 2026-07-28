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
  mkdtempSync,
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
  "officecli-release.json",
);

export function readOfficeCliManifest(path = manifestPath) {
  const manifest = JSON.parse(readFileSync(path, "utf8"));
  if (
    typeof manifest.version !== "string" ||
    !/^\d+\.\d+\.\d+$/.test(manifest.version) ||
    manifest.license !== "Apache-2.0" ||
    typeof manifest.releaseBaseUrl !== "string" ||
    !manifest.releaseBaseUrl.startsWith("https://github.com/iOfficeAI/OfficeCLI/releases/") ||
    !manifest.targets ||
    typeof manifest.targets !== "object"
  ) {
    throw new Error("invalid OfficeCLI release manifest");
  }
  for (const [target, entry] of Object.entries(manifest.targets)) {
    if (
      !["darwin-arm64", "darwin-x64", "win32-arm64", "win32-x64"].includes(target) ||
      typeof entry?.asset !== "string" ||
      typeof entry?.sha256 !== "string" ||
      !/^[a-f0-9]{64}$/.test(entry.sha256)
    ) {
      throw new Error(`invalid OfficeCLI target entry: ${target}`);
    }
  }
  return manifest;
}

export function officeCliTarget(platform = process.platform, arch = process.arch) {
  const target = `${platform}-${arch}`;
  if (!new Set(["darwin-arm64", "darwin-x64", "win32-arm64", "win32-x64"]).has(target)) {
    throw new Error(`OfficeCLI is not available for ${target}`);
  }
  return target;
}

export function officeCliBinaryName(target) {
  return target.startsWith("win32-") ? "officecli.exe" : "officecli";
}

export function officeCliBinaryPath(target) {
  return join(
    repoRoot,
    "apps",
    "desktop",
    "resources",
    "bin",
    target,
    officeCliBinaryName(target),
  );
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
      : [officeCliTarget()];
  const targets = configured
    ? configured.split(",").map((value) => value.trim()).filter(Boolean)
    : defaults;
  for (const target of targets) {
    if (!manifest.targets[target]) {
      throw new Error(`OfficeCLI release manifest has no target ${target}`);
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
  await pipeline(Readable.fromWeb(response.body), createWriteStream(outputPath, { mode: 0o755 }));
}

function assertLicenseFiles(version) {
  const licenseRoot = join(
    repoRoot,
    "apps",
    "desktop",
    "resources",
    "licenses",
    "officecli",
    version,
  );
  for (const name of ["LICENSE", "NOTICE", "THIRD-PARTY-NOTICES.txt"]) {
    const path = join(licenseRoot, name);
    if (!existsSync(path) || statSync(path).size === 0) {
      throw new Error(`missing OfficeCLI license file ${path}`);
    }
  }
}

function canExecuteTarget(target) {
  return target === `${process.platform}-${process.arch}`;
}

function officeCliEnvironment() {
  return {
    ...process.env,
    OFFICECLI_SKIP_UPDATE: "1",
    OFFICECLI_NO_AUTO_INSTALL: "1",
    OFFICECLI_NO_AUTO_RESIDENT: "1",
  };
}

function runOfficeCli(binaryPath, args, options = {}) {
  return execFileSync(binaryPath, args, {
    env: officeCliEnvironment(),
    encoding: "utf8",
    stdio: options.stdio ?? ["ignore", "pipe", "pipe"],
    timeout: options.timeout ?? 120_000,
  });
}

function verifyExecutable(binaryPath, version) {
  accessSync(binaryPath, constants.X_OK);
  const output = String(runOfficeCli(binaryPath, ["--version"]));
  if (!output.includes(version)) {
    throw new Error(
      `${basename(binaryPath)} version mismatch: expected ${version}, received ${output.trim()}`,
    );
  }
}

function smokeOfficeRendering(binaryPath) {
  const root = mkdtempSync(join(tmpdir(), "awaken-officecli-smoke-"));
  try {
    for (const extension of ["docx", "xlsx", "pptx"]) {
      const source = join(root, `smoke.${extension}`);
      const output = join(root, `smoke-${extension}.html`);
      runOfficeCli(binaryPath, ["create", source]);
      runOfficeCli(binaryPath, ["view", source, "html", "-o", output]);
      if (!existsSync(output) || statSync(output).size === 0) {
        throw new Error(`OfficeCLI did not produce ${output}`);
      }
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

export async function prepareOfficeCli({ targets, smoke = true } = {}) {
  const manifest = readOfficeCliManifest();
  assertLicenseFiles(manifest.version);
  const selectedTargets = targets ?? requestedTargets(manifest);

  for (const target of selectedTargets) {
    const entry = manifest.targets[target];
    if (!entry) throw new Error(`OfficeCLI release manifest has no target ${target}`);
    const outputPath = officeCliBinaryPath(target);
    mkdirSync(dirname(outputPath), { recursive: true });

    let checksum = existsSync(outputPath) ? await sha256File(outputPath) : "";
    if (checksum !== entry.sha256) {
      const temporaryPath = `${outputPath}.download-${process.pid}-${randomUUID()}`;
      try {
        console.log(`[prepare-officecli] downloading ${target} ${entry.asset}`);
        await downloadAsset(`${manifest.releaseBaseUrl}/${entry.asset}`, temporaryPath);
        checksum = await sha256File(temporaryPath);
        if (checksum !== entry.sha256) {
          throw new Error(
            `OfficeCLI checksum mismatch for ${target}: expected ${entry.sha256}, received ${checksum}`,
          );
        }
        if (!target.startsWith("win32-")) chmodSync(temporaryPath, 0o755);
        rmSync(outputPath, { force: true });
        renameSync(temporaryPath, outputPath);
      } finally {
        rmSync(temporaryPath, { force: true });
      }
    }
    if (!target.startsWith("win32-")) chmodSync(outputPath, 0o755);

    const verifyAll = process.env.AWAKEN_OFFICECLI_VERIFY_ALL === "1";
    if (canExecuteTarget(target) || verifyAll) {
      verifyExecutable(outputPath, manifest.version);
      if (smoke && process.env.AWAKEN_OFFICECLI_SKIP_SMOKE !== "1") {
        smokeOfficeRendering(outputPath);
      }
    }
    console.log(`[prepare-officecli] ready ${target} -> ${outputPath}`);
  }
  return { version: manifest.version, targets: selectedTargets };
}

if (resolve(process.argv[1] || "") === scriptPath) {
  prepareOfficeCli().catch((error) => {
    console.error(
      `[prepare-officecli] ${error instanceof Error ? error.message : String(error)}`,
    );
    process.exitCode = 1;
  });
}
