#!/usr/bin/env node
// Launch the host-native packaged Desktop and require it to create the main
// window. This is the CI startup smoke for ad-hoc macOS and unsigned Windows
// packages. It isolates HOME/APPDATA under a temp directory, then stops the
// process tree. It does not install, sign, notarize, or exercise product flows.

import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

export const READY_MARKER = "desktop_main_window_created";
const CRASH_PATTERN = /\[awaken-desktop\] (?:uncaughtException|unhandledRejection)/;

export function packagedExecutableCandidates(platform, arch) {
  if (platform === "darwin" && arch === "arm64") {
    return ["mac-arm64/Awaken.app/Contents/MacOS/Awaken"];
  }
  if (platform === "darwin" && arch === "x64") {
    return [
      "mac/Awaken.app/Contents/MacOS/Awaken",
      "mac-x64/Awaken.app/Contents/MacOS/Awaken",
    ];
  }
  if (platform === "win32" && (arch === "x64" || arch === "arm64")) {
    return ["win-unpacked/Awaken.exe"];
  }
  throw new Error(`packaged desktop smoke does not support ${platform}-${arch}`);
}

export function resolvePackagedExecutable(dir, platform, arch, exists) {
  const root = resolve(dir);
  for (const relativePath of packagedExecutableCandidates(platform, arch)) {
    const candidate = join(root, relativePath);
    if (exists(candidate)) return candidate;
  }
  throw new Error(
    `packaged Awaken executable for ${platform}-${arch} was not found under ${root}`,
  );
}

export function isolatedLaunchEnvironment(root, baseEnv = process.env) {
  const home = join(root, "home");
  const env = { ...baseEnv };
  delete env.AWAKEN_DESKTOP_ACCEPTANCE_ROOT;
  delete env.ELECTRON_RUN_AS_NODE;
  return {
    ...env,
    HOME: home,
    USERPROFILE: home,
    APPDATA: join(home, "AppData", "Roaming"),
    LOCALAPPDATA: join(home, "AppData", "Local"),
    XDG_CONFIG_HOME: join(home, ".config"),
    XDG_DATA_HOME: join(home, ".local", "share"),
    XDG_CACHE_HOME: join(home, ".cache"),
    ELECTRON_ENABLE_LOGGING: "1",
  };
}

export function assessSmokeTranscript(output) {
  const text = String(output ?? "");
  if (!text.includes(READY_MARKER)) {
    return {
      ok: false,
      reason: "packaged desktop did not create the main window",
    };
  }
  if (CRASH_PATTERN.test(text)) {
    return {
      ok: false,
      reason: "packaged desktop reported an uncaught exception",
    };
  }
  return { ok: true, reason: "" };
}

function prepareIsolationRoot(root) {
  const home = join(root, "home");
  for (const path of [
    home,
    join(home, "Library", "Application Support"),
    join(home, "AppData", "Roaming"),
    join(home, "AppData", "Local"),
    join(home, ".config"),
    join(home, ".local", "share"),
    join(home, ".cache"),
    join(home, ".awaken"),
  ]) {
    mkdirSync(path, { recursive: true });
  }
}

function signalProcessTree(child, signal) {
  if (!child.pid || child.exitCode != null || child.signalCode != null) return;
  if (process.platform === "win32") {
    spawnSync("taskkill", ["/PID", String(child.pid), "/T", "/F"], {
      windowsHide: true,
      stdio: "ignore",
    });
    return;
  }
  try {
    process.kill(-child.pid, signal);
  } catch {
    try {
      child.kill(signal);
    } catch {
      // The process already exited.
    }
  }
}

function parseArgs(argv) {
  const args = {
    dir: join("apps", "desktop", "dist-electron"),
    log: "",
    timeoutMs: 120_000,
    platform: process.platform,
    arch: process.arch,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    const value = argv[index + 1];
    if (flag === "--dir") {
      args.dir = value;
      index += 1;
    } else if (flag === "--log") {
      args.log = value;
      index += 1;
    } else if (flag === "--timeout-ms") {
      args.timeoutMs = Number(value);
      index += 1;
    } else if (flag === "--help") {
      args.help = true;
    } else {
      throw new Error(`unknown argument: ${flag}`);
    }
  }
  if (!Number.isFinite(args.timeoutMs) || args.timeoutMs < 1_000) {
    throw new Error("--timeout-ms must be at least 1000");
  }
  return args;
}

export async function smokePackagedDesktop({
  dir,
  platform = process.platform,
  arch = process.arch,
  timeoutMs = 120_000,
  log = "",
  exists,
  spawnProcess = spawn,
  isolationRoot = join(tmpdir(), `awaken-packaged-smoke-${process.pid}`),
} = {}) {
  const executable = resolvePackagedExecutable(dir, platform, arch, exists ?? existsSync);
  prepareIsolationRoot(isolationRoot);
  const child = spawnProcess(executable, ["--disable-gpu"], {
    cwd: dirname(executable),
    env: isolatedLaunchEnvironment(isolationRoot),
    detached: process.platform !== "win32",
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });
  let output = "";
  const append = (chunk) => {
    output += chunk.toString();
  };
  child.stdout?.on("data", append);
  child.stderr?.on("data", append);
  child.once("error", (error) => {
    output += `${error instanceof Error ? error.message : String(error)}\n`;
  });

  let settled = false;
  let timer;
  const ready = new Promise((resolveReady) => {
    timer = setInterval(() => {
      if (output.includes(READY_MARKER)) {
        clearInterval(timer);
        settled = true;
        resolveReady("ready");
      }
    }, 200);
    child.once("exit", (code) => {
      clearInterval(timer);
      if (!settled) {
        settled = true;
        resolveReady(`exit:${code ?? "null"}`);
      }
    });
  });
  const outcome = await Promise.race([
    ready,
    delay(timeoutMs).then(() => "timeout"),
  ]);
  clearInterval(timer);
  if (outcome === "ready") {
    await delay(2_000);
  }
  signalProcessTree(child, "SIGTERM");
  await Promise.race([once(child, "exit"), delay(5_000)]).catch(() => {});
  signalProcessTree(child, "SIGKILL");
  if (log) {
    mkdirSync(dirname(resolve(log)), { recursive: true });
    writeFileSync(resolve(log), output);
  }
  const assessment = assessSmokeTranscript(output);
  if (!assessment.ok) {
    const detail = outcome === "timeout" ? "timed out" : String(outcome);
    throw new Error(`${assessment.reason} (${detail})\n${output.slice(-4_000)}`);
  }
  return { executable, output };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    console.log(`Smoke the host-native packaged Awaken desktop.

Usage:
  node scripts/smoke-packaged-desktop.mjs [--dir apps/desktop/dist-electron] [--log smoke.log] [--timeout-ms 120000]
`);
    return;
  }
  const result = await smokePackagedDesktop(args);
  console.log(`[smoke-packaged-desktop] main window created: ${result.executable}`);
}

const scriptPath = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] || "") === scriptPath) {
  main().catch((error) => {
    console.error(`[smoke-packaged-desktop] ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  });
}
