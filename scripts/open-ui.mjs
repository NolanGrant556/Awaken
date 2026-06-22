#!/usr/bin/env node
import { spawn } from "node:child_process";
import { dirname, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const galleryUrl = "http://127.0.0.1:5173/";
const readyTimeoutMs = 15_000;

async function canReachGallery() {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 700);
  timeout.unref();

  try {
    const response = await fetch(galleryUrl, {
      method: "GET",
      signal: controller.signal,
    });
    return response.ok;
  } catch {
    return false;
  } finally {
    clearTimeout(timeout);
  }
}

function openGallery() {
  const opener =
    process.platform === "darwin"
      ? { command: "open", args: [galleryUrl] }
      : process.platform === "win32"
        ? { command: "cmd", args: ["/c", "start", "", galleryUrl] }
        : { command: "xdg-open", args: [galleryUrl] };

  const child = spawn(opener.command, opener.args, {
    detached: true,
    stdio: "ignore",
  });
  child.unref();
}

async function waitForGallery(serverProcess) {
  const deadline = Date.now() + readyTimeoutMs;

  while (Date.now() < deadline) {
    if (await canReachGallery()) {
      return true;
    }

    if (serverProcess.exitCode !== null) {
      return false;
    }

    await delay(250);
  }

  return false;
}

if (await canReachGallery()) {
  openGallery();
  console.log(`Opened ${galleryUrl}`);
  process.exit(0);
}

console.log("Starting Awaken Design System gallery...");

const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
const serverProcess = spawn(npmCommand, ["run", "dev:ui"], {
  cwd: rootDir,
  stdio: "inherit",
});

let opened = false;

waitForGallery(serverProcess).then((ready) => {
  if (!ready) {
    console.error(`Gallery did not become reachable at ${galleryUrl}`);
    return;
  }

  opened = true;
  openGallery();
  console.log(`Opened ${galleryUrl}`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    serverProcess.kill(signal);
  });
}

serverProcess.on("exit", (code, signal) => {
  if (!opened && code !== 0) {
    process.exitCode = code ?? 1;
    return;
  }

  process.exitCode = signal ? 1 : (code ?? 0);
});
