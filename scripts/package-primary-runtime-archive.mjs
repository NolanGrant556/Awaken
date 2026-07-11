#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  createReadStream,
  mkdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { basename, isAbsolute, join, resolve } from "node:path";
import {
  checkPrimaryRuntime,
  readCurrentJson,
  readResolvedState,
  resolvePrimaryRuntimeRoot,
  versionDir,
} from "./primary-runtime-common.mjs";
import {
  createRuntimeArchiveInvocation,
  runtimeArchiveIntegrityManifest,
  runtimeArchivePlatform,
} from "./runtime-archive-platform.mjs";

function fail(message) {
  console.error(`package-primary-runtime-archive: ${message}`);
  process.exit(1);
}

function argument(name, fallback = "") {
  const index = process.argv.indexOf(name);
  return index >= 0 ? String(process.argv[index + 1] || "") : fallback;
}

function required(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.status !== 0) {
    fail(`${command} ${args.join(" ")} failed: ${(result.stderr || result.stdout || "").trim()}`);
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

const platform = runtimeArchivePlatform();
if (platform !== "darwin" && platform !== "windows") {
  fail(`release runtime archives are unsupported on ${platform}`);
}

const root = resolvePrimaryRuntimeRoot();
const current = readCurrentJson(root);
if (
  !current?.active ||
  isAbsolute(String(current.active)) ||
  String(current.active).split(/[\\/]+/).includes("..")
) {
  fail("primary runtime has no safe active revision");
}
const activeRoot = versionDir(root, current.active);
const state = readResolvedState(activeRoot);
const report = checkPrimaryRuntime({ root });
if (!report.ready || report.active !== current.active) {
  fail("primary runtime is not ready for archive packaging");
}
if (
  !state ||
  state.platform !== platform ||
  state.arch !== process.arch ||
  state.runtimeName !== "primary-runtime"
) {
  fail(`active runtime does not match ${platform}-${process.arch}`);
}

const signing = state?.security?.macOSRuntimeSigning;
const libreOffice = state.components?.find((component) => component.name === "libreoffice");
if (!libreOffice?.relativePath) {
  fail("active runtime state does not contain LibreOffice");
}
let signingManifest;
if (platform === "darwin") {
  if (
    signing?.mode !== "developer-id" ||
    signing?.strictVerified !== true ||
    signing?.notarized !== true ||
    signing?.stapled !== true
  ) {
    fail("active runtime is not a Developer ID signed, notarized, and stapled release revision");
  }
  const appRoot = join(activeRoot, libreOffice.relativePath);
  required("codesign", ["--verify", "--deep", "--strict", "--verbose=4", appRoot]);
  required("xcrun", ["stapler", "validate", appRoot]);
  required("spctl", ["-a", "-vv", "--type", "execute", appRoot]);
  const designatedRequirement = required("codesign", ["-d", "-r-", appRoot])
    .split("\n")
    .find((line) => line.trim().startsWith("designated =>"))
    ?.trim();
  if (!designatedRequirement) {
    fail("unable to read the runtime designated requirement");
  }
  const signatureDetails = required("codesign", ["-d", "--verbose=4", appRoot]);
  const teamIdentifier = signatureDetails.match(/^TeamIdentifier=(.+)$/m)?.[1]?.trim();
  if (!teamIdentifier) {
    fail("unable to read the runtime TeamIdentifier");
  }
  signingManifest = {
    mode: "developer-id",
    identity: signing.identity,
    designatedRequirement,
    teamIdentifier,
    notarized: true,
    stapled: true,
    bundleIdentifier:
      state.officeRender?.macOSBundleIdentifier ||
      "com.awaken.runtime.libreoffice-renderer",
  };
}

const outputDirectory = resolve(argument("--output", join(root, "release")));
mkdirSync(outputDirectory, { recursive: true });
const archiveName = `primary-runtime-${state.bundleVersion}-${platform}-${state.arch}.zip`;
const archivePath = join(outputDirectory, archiveName);
const archiveInvocation = createRuntimeArchiveInvocation(
  process.platform,
  activeRoot,
  archivePath,
);
required(archiveInvocation.command, archiveInvocation.args);
const archiveStat = statSync(archivePath);
const digest = await sha256(archivePath);
const manifest = {
  schemaVersion: 1,
  runtimeName: "primary-runtime",
  active: current.active,
  rootDirectory: basename(activeRoot),
  bundleVersion: state.bundleVersion,
  platform: state.platform,
  arch: state.arch,
  archive: archiveName,
  byteSize: archiveStat.size,
  sha256: digest,
  integrity: runtimeArchiveIntegrityManifest(),
  ...(signingManifest ? { signing: signingManifest } : {}),
  createdAt: new Date().toISOString(),
};
const manifestPath = join(outputDirectory, `${archiveName}.manifest.json`);
writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
// Read back before reporting so truncated writes fail the release job.
JSON.parse(readFileSync(manifestPath, "utf8"));
console.log(JSON.stringify({ ok: true, archivePath, manifestPath, sha256: digest }, null, 2));
