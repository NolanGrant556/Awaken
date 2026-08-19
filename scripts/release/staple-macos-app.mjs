#!/usr/bin/env node

// electron-builder `afterSign` hook for the macOS release build.
//
// electron-builder resolves hook paths starting with "." against process.cwd()
// (app-builder-lib out/util/resolve.js); all dist scripts run electron-builder
// with cwd = apps/desktop, so ../../scripts/release/staple-macos-app.mjs lands
// here, inside the npm workspace root.
//
// Ordering: MacPackager.signApp() runs doSign -> notarizeIfProvided(appPath)
// before platformPackager emits afterSign, so when this hook sees a
// Developer ID signed app, electron-builder has already notarized it via
// `mac.notarize: true`. This hook staples the notarization ticket onto the
// exact `.app` of this build and verifies Gatekeeper acceptance, so the ZIP
// packaged afterwards contains the stapled app.

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const LOG_PREFIX = "[staple-macos-app]";

export const APPLE_NOTARY_ENV = [
  "APPLE_API_KEY",
  "APPLE_API_KEY_ID",
  "APPLE_API_ISSUER",
];

function log(message) {
  console.log(`${LOG_PREFIX} ${message}`);
}

export function runCommand(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  const output = `${result.stdout ?? ""}${result.stderr ?? ""}`;
  if (result.error) {
    throw new Error(`${command} failed to start: ${result.error.message}`);
  }
  if (result.status !== 0) {
    throw new Error(
      `${command} ${args.join(" ")} exited with ${result.status}: ${output.trim()}`,
    );
  }
  return output;
}

// Resolve the single `.app` of THIS build from the hook context. Never glob
// recursively: stale apps from earlier builds must not be stapled or shipped.
export function resolveAppPath(context) {
  const productFilename = context?.packager?.appInfo?.productFilename;
  if (!context?.appOutDir || !productFilename) {
    throw new Error("afterSign context is missing appOutDir or productFilename");
  }
  const appPath = join(context.appOutDir, `${productFilename}.app`);
  if (!existsSync(appPath)) {
    throw new Error(`expected signed app does not exist: ${appPath}`);
  }
  return appPath;
}

// `codesign -dv --verbose=4` writes its report to stderr; runCommand merges it.
export function inspectCodesign(appPath, run = runCommand) {
  return run("codesign", ["-dv", "--verbose=4", appPath]);
}

export function isAdhocSignature(codesignOutput) {
  // A Developer ID authority marks a release signature; anything else
  // (Signature=adhoc, or no authority chain at all) is a local/ad-hoc build.
  return !/^Authority=Developer ID Application:/m.test(codesignOutput);
}

export function requireEnv(names, env = process.env) {
  const missing = names.filter((name) => !env[name]);
  if (missing.length > 0) {
    throw new Error(
      `missing required release environment: ${missing.join(", ")}`,
    );
  }
}

export async function stapleMacosApp(context, options = {}) {
  if (context?.electronPlatformName !== "darwin") {
    return;
  }
  const run = options.runCommand ?? runCommand;
  const env = options.env ?? process.env;
  const appPath = resolveAppPath(context);

  // Local/ad-hoc builds (`dist:adhoc`, `-c.mac.identity=-`) never carry a
  // Developer ID signature; stapling is a release-only step, so skip them.
  const signature = inspectCodesign(appPath, run);
  if (isAdhocSignature(signature)) {
    log(`ad-hoc signature detected, skipping staple for ${appPath}`);
    return;
  }

  // A Developer ID signed app means this is a release build: electron-builder
  // already notarized it (notarize: true), so the notary credentials must be
  // present. Fail closed otherwise.
  requireEnv(APPLE_NOTARY_ENV, env);

  log(`stapling notarization ticket: ${appPath}`);
  run("xcrun", ["stapler", "staple", appPath]);
  run("xcrun", ["stapler", "validate", appPath]);
  run("spctl", ["-a", "-vv", "-t", "exec", appPath]);
  log("staple + Gatekeeper validation passed");
}

export default stapleMacosApp;

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  console.error(
    `${LOG_PREFIX} this script is an electron-builder afterSign hook, not a CLI`,
  );
  process.exit(1);
}
