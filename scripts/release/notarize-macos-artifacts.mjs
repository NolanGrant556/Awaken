#!/usr/bin/env node

// electron-builder `afterAllArtifactBuild` hook: sign, notarize, staple and
// validate the DMG installers produced by THIS build.
//
// Hook path resolution is identical to staple-macos-app.mjs (cwd-relative,
// see that file). Only `context.artifactPaths` is used — no directory scans,
// so stale DMGs from earlier builds are never touched.
//
// Ordering guarantee (verified against app-builder-lib out/index.js and
// out/publish/updateInfoBuilder.js): `afterAllArtifactBuild` runs inside the
// `packager.build().then(...)` chain, while `PublishManager.awaitTasks()` —
// which calls `writeUpdateInfoFiles()` and writes `latest-mac.yml` with
// SHA-512 checksums — runs afterwards in `executeFinally`. DMGs are also not
// update-feed members (`isWriteUpdateInfo` is only set for ZIP/NSIS), so
// re-signing DMGs here cannot invalidate update metadata.
//
// Ad-hoc detection (single deterministic mechanism): each DMG is inspected
// with `codesign -dv`. Ad-hoc or unsigned DMGs come from local builds
// (`dist` / `dist:adhoc` pass `-c.mac.identity=-` / no identity) and are
// skipped with a log line; a Developer ID signed DMG marks a release build
// and then every notarization requirement fails closed.

import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  isAdhocSignature,
  requireEnv,
  runCommand,
} from "./staple-macos-app.mjs";

const LOG_PREFIX = "[notarize-macos-artifacts]";

export const NOTARY_SUBMIT_ENV = [
  "APPLE_API_KEY",
  "APPLE_API_KEY_ID",
  "APPLE_API_ISSUER",
];

function log(message) {
  console.log(`${LOG_PREFIX} ${message}`);
}

export function filterDmgArtifacts(artifactPaths) {
  return (artifactPaths ?? []).filter((path) => path.endsWith(".dmg"));
}

// Returns "release" (Developer ID signed), "adhoc" or "unsigned".
export function classifyDmgSignature(dmgPath, run = runCommand) {
  const result = spawnSync("codesign", ["-dv", "--verbose=4", dmgPath], {
    encoding: "utf8",
  });
  const output = `${result.stdout ?? ""}${result.stderr ?? ""}`;
  if (result.status !== 0) {
    if (/not signed at all|code object is not signed/.test(output)) {
      return "unsigned";
    }
    throw new Error(`codesign inspection failed for ${dmgPath}: ${output.trim()}`);
  }
  return isAdhocSignature(output) ? "adhoc" : "release";
}

export function submitToNotary(dmgPath, env, run = runCommand) {
  const output = run("xcrun", [
    "notarytool",
    "submit",
    dmgPath,
    "--key",
    env.APPLE_API_KEY,
    "--key-id",
    env.APPLE_API_KEY_ID,
    "--issuer",
    env.APPLE_API_ISSUER,
    "--wait",
  ]);
  const submissionId = output.match(/^\s*id:\s*([0-9a-f-]+)$/im)?.[1];
  if (!/^\s*status:\s*Accepted\s*$/im.test(output)) {
    if (submissionId) {
      const detail = run("xcrun", [
        "notarytool",
        "log",
        submissionId,
        "--key",
        env.APPLE_API_KEY,
        "--key-id",
        env.APPLE_API_KEY_ID,
        "--issuer",
        env.APPLE_API_ISSUER,
      ]);
      log(`notary log for rejected submission ${submissionId}:\n${detail}`);
    }
    throw new Error(`notarization was not Accepted for ${dmgPath}`);
  }
  return submissionId;
}

export function notarizeDmg(dmgPath, env, run = runCommand) {
  const identity = env.AWAKEN_CODESIGN_IDENTITY;
  log(`signing DMG with Developer ID: ${dmgPath}`);
  run("codesign", ["--force", "--sign", identity, "--timestamp", dmgPath]);
  run("codesign", ["--verify", "--strict", dmgPath]);
  const submissionId = submitToNotary(dmgPath, env, run);
  log(`notarization Accepted (submission ${submissionId ?? "unknown"})`);
  run("xcrun", ["stapler", "staple", dmgPath]);
  run("xcrun", ["stapler", "validate", dmgPath]);
  run("spctl", [
    "-a",
    "-vv",
    "-t",
    "open",
    "--context",
    "context:primary-signature",
    dmgPath,
  ]);
}

export async function notarizeMacosArtifacts(context, options = {}) {
  const dmgPaths = filterDmgArtifacts(context?.artifactPaths);
  if (dmgPaths.length === 0) {
    // Windows/Linux builds reach this hook too; nothing to do.
    return [];
  }
  const run = options.runCommand ?? runCommand;
  const env = options.env ?? process.env;

  const kinds = dmgPaths.map((dmgPath) => classifyDmgSignature(dmgPath, run));
  if (kinds.every((kind) => kind !== "release")) {
    log("ad-hoc/unsigned DMGs detected, skipping notarization (local build)");
    return [];
  }
  if (kinds.some((kind) => kind !== "release")) {
    throw new Error(
      "mixed signed and unsigned DMGs in one build; refusing to notarize a partial set",
    );
  }

  requireEnv(["AWAKEN_CODESIGN_IDENTITY", ...NOTARY_SUBMIT_ENV], env);
  for (const dmgPath of dmgPaths) {
    notarizeDmg(dmgPath, env, run);
  }
  log(`notarized and stapled ${dmgPaths.length} DMG(s)`);
  return [];
}

export default notarizeMacosArtifacts;

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  console.error(
    `${LOG_PREFIX} this script is an electron-builder afterAllArtifactBuild hook, not a CLI`,
  );
  process.exit(1);
}
