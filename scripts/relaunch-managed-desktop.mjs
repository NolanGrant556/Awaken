#!/usr/bin/env node
import { spawn } from "node:child_process";
import { dirname, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const previousElectronPid = Number(process.argv[2]);
const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
const deadline = Date.now() + 20_000;

function processExists(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return error?.code === "EPERM";
  }
}

while (processExists(previousElectronPid) && Date.now() < deadline) {
  await delay(200);
}

// open:desktop is the only supported macOS lifecycle owner. Detaching this
// helper lets the old managed screen finish before a fresh one is created.
const child = spawn(npmCommand, ["run", "open:desktop"], {
  cwd: rootDir,
  detached: true,
  stdio: "ignore",
});
child.unref();
