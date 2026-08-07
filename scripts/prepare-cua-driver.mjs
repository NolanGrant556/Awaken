#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { accessSync, constants, existsSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(import.meta.url);
const repoRoot = resolve(dirname(scriptPath), "..");
const hostDir = join(repoRoot, "apps", "desktop", "resources", "cua-driver");
const hostScript = join(hostDir, "host.mjs");

function hasNodeModules() {
  const marker = join(hostDir, "node_modules", ".package-lock.json");
  try {
    accessSync(marker, constants.F_OK);
    return true;
  } catch {
    return false;
  }
}

function sourceIsNewer() {
  if (!existsSync(hostScript)) {
    throw new Error(`CUA host script missing: ${hostScript}`);
  }
  if (!hasNodeModules()) {
    return true;
  }
  const packageJson = join(hostDir, "package.json");
  const lock = join(hostDir, "node_modules", ".package-lock.json");
  try {
    const pkgStat = statSync(packageJson);
    const lockStat = statSync(lock);
    if (pkgStat.mtimeMs > lockStat.mtimeMs) {
      return true;
    }
    const hostStat = statSync(hostScript);
    if (hostStat.mtimeMs > lockStat.mtimeMs) {
      return true;
    }
  } catch {
    return true;
  }
  return false;
}

function install() {
  console.log(`[prepare-cua-driver] installing dependencies in ${hostDir}`);
  execFileSync("npm", ["install", "--omit=dev"], {
    cwd: hostDir,
    stdio: "inherit",
    shell: process.platform === "win32",
  });
}

function main() {
  if (!existsSync(hostScript)) {
    console.error(`[prepare-cua-driver] host script not found: ${hostScript}`);
    process.exit(1);
  }
  if (sourceIsNewer()) {
    install();
  } else {
    console.log(`[prepare-cua-driver] node_modules is up to date`);
  }
}

main();
