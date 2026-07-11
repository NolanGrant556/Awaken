#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  createReadStream,
  lstatSync,
  mkdirSync,
  readFileSync,
  readlinkSync,
  realpathSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
} from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import {
  createStagingDir,
  readCurrentJson,
  readResolvedState,
  resolvePrimaryRuntimeRoot,
  versionDir,
  writeCurrentJson,
} from "./primary-runtime-common.mjs";
import {
  extractRuntimeArchiveInvocation,
  hasRuntimeArchiveIntegrity,
  runtimeArchivePlatform,
  runtimeOfficeExecutableRelativePath,
} from "./runtime-archive-platform.mjs";

function fail(message) {
  console.error(`install-primary-runtime-archive: ${message}`);
  process.exit(1);
}

function argument(name) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? String(process.argv[index + 1] || "") : "";
}

function required(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(`${command} failed: ${(result.stderr || result.stdout || "").trim()}`);
  }
  return `${result.stdout || ""}\n${result.stderr || ""}`.trim();
}

async function sha256(path) {
  const hash = createHash("sha256");
  await new Promise((resolvePromise, reject) => {
    const input = createReadStream(path);
    input.on("data", (chunk) => hash.update(chunk));
    input.on("error", reject);
    input.on("end", resolvePromise);
  });
  return hash.digest("hex");
}

function inside(root, target) {
  const rel = relative(root, target);
  return rel === "" || (rel !== ".." && !isAbsolute(rel) && !rel.startsWith(`..${sep}`));
}

function validateExtractedTree(root) {
  const queue = [root];
  while (queue.length > 0) {
    const current = queue.pop();
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const path = join(current, entry.name);
      const info = lstatSync(path);
      if (info.isSymbolicLink()) {
        const link = readlinkSync(path);
        if (isAbsolute(link)) {
          throw new Error(`absolute symlink is forbidden: ${path}`);
        }
        const target = realpathSync(path);
        if (!inside(root, target)) {
          throw new Error(`symlink escapes runtime revision: ${path}`);
        }
      } else if (info.isDirectory()) {
        queue.push(path);
      }
    }
  }
}

const archivePath = resolve(argument("--archive"));
const manifestPath = resolve(argument("--manifest"));
const expectedTeamIdentifier = argument("--expected-team-id");
const expectedPlatform = runtimeArchivePlatform();
if (
  !argument("--archive") ||
  !argument("--manifest") ||
  (expectedPlatform === "darwin" && !expectedTeamIdentifier)
) {
  fail(
    "usage: --archive <zip> --manifest <manifest.json> [--expected-team-id <TeamIdentifier on macOS>]",
  );
}
const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
const manifestActive = String(manifest.active || "");
const manifestRootDirectory = String(manifest.rootDirectory || "");
const manifestArchive = String(manifest.archive || "");
if (
  manifest.schemaVersion !== 1 ||
  manifest.runtimeName !== "primary-runtime" ||
  manifest.platform !== expectedPlatform ||
  manifest.arch !== process.arch ||
  !manifestActive ||
  isAbsolute(manifestActive) ||
  manifestActive.split(/[\\/]+/).includes("..") ||
  !manifestRootDirectory ||
  manifestRootDirectory === "." ||
  basename(manifestRootDirectory) !== manifestRootDirectory ||
  !manifestArchive ||
  basename(manifestArchive) !== manifestArchive ||
  !hasRuntimeArchiveIntegrity(manifest) ||
  (expectedPlatform === "darwin" &&
    (manifest.signing?.mode !== "developer-id" ||
      manifest.signing?.teamIdentifier !== expectedTeamIdentifier ||
      manifest.signing?.bundleIdentifier !== "com.awaken.runtime.libreoffice-renderer" ||
      manifest.signing?.notarized !== true ||
      manifest.signing?.stapled !== true)) ||
  (expectedPlatform !== "darwin" && expectedPlatform !== "windows")
) {
  fail("release manifest is incompatible with this machine");
}
const archiveStat = statSync(archivePath);
if (archiveStat.size !== manifest.byteSize || (await sha256(archivePath)) !== manifest.sha256) {
  fail("runtime archive size or SHA-256 does not match its release manifest");
}

const root = resolvePrimaryRuntimeRoot();
mkdirSync(root, { recursive: true });
const staging = createStagingDir(root, "archive-install");
let installError = "";
try {
  const extraction = extractRuntimeArchiveInvocation(
    process.platform,
    archivePath,
    staging,
  );
  required(extraction.command, extraction.args);
  const extracted = join(staging, manifestRootDirectory);
  validateExtractedTree(extracted);
  const state = readResolvedState(extracted);
  if (
    !state ||
    state.runtimeName !== "primary-runtime" ||
    state.bundleVersion !== manifest.bundleVersion ||
    state.platform !== manifest.platform ||
    state.arch !== manifest.arch
  ) {
    throw new Error("extracted runtime state does not match the release manifest");
  }
  for (const component of state.components || []) {
    if (!component.required) {
      continue;
    }
    const relativePath = String(component.relativePath || "");
    const componentPath = resolve(extracted, relativePath);
    if (!relativePath || !inside(extracted, componentPath) || !statSafe(componentPath)) {
      throw new Error(`required runtime component is missing or unsafe: ${component.name || "unknown"}`);
    }
  }
  const libreOffice = state.components?.find((component) => component.name === "libreoffice");
  if (!libreOffice?.relativePath) {
    throw new Error("extracted runtime does not contain LibreOffice");
  }
  const officeExecutable = statSafe(
    join(
      extracted,
      libreOffice.relativePath,
      runtimeOfficeExecutableRelativePath(),
    ),
  );
  if (!officeExecutable?.isFile()) {
    throw new Error(`extracted ${expectedPlatform} runtime does not contain its native soffice executable`);
  }
  if (expectedPlatform === "darwin") {
    if (
      state.security?.macOSRuntimeSigning?.notarized !== true ||
      state.security?.macOSRuntimeSigning?.stapled !== true
    ) {
      throw new Error("extracted macOS runtime is not notarized and stapled");
    }
    const appRoot = join(extracted, libreOffice.relativePath);
    required("codesign", ["--verify", "--deep", "--strict", "--verbose=4", appRoot]);
    const signatureDetails = required("codesign", ["-d", "--verbose=4", appRoot]);
    const actualTeamIdentifier = signatureDetails.match(/^TeamIdentifier=(.+)$/m)?.[1]?.trim();
    const actualBundleIdentifier = signatureDetails.match(/^Identifier=(.+)$/m)?.[1]?.trim();
    if (
      actualTeamIdentifier !== expectedTeamIdentifier ||
      actualBundleIdentifier !== "com.awaken.runtime.libreoffice-renderer"
    ) {
      throw new Error("runtime signature identity does not match the signed Awaken host");
    }
    const requirement = required("codesign", ["-d", "-r-", appRoot]);
    if (!requirement.includes(manifest.signing.designatedRequirement)) {
      throw new Error("runtime designated requirement does not match the release manifest");
    }
    required("xcrun", ["stapler", "validate", appRoot]);
    required("spctl", ["-a", "-vv", "--type", "execute", appRoot]);
  }

  const target = versionDir(root, manifestActive);
  mkdirSync(dirname(target), { recursive: true });
  if (statSafe(target)) {
    throw new Error(`immutable runtime revision already exists: ${manifestActive}`);
  }
  renameSync(extracted, target);
  const previous = readCurrentJson(root)?.active || "";
  writeCurrentJson(root, { active: manifestActive, previous });
  console.log(JSON.stringify({ ok: true, active: manifestActive, root: target }, null, 2));
} catch (error) {
  installError = error instanceof Error ? error.message : String(error);
} finally {
  rmSync(staging, { recursive: true, force: true });
}
if (installError) {
  fail(installError);
}

function statSafe(path) {
  try {
    return statSync(path);
  } catch {
    return null;
  }
}
