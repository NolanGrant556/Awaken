#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const screenName = "awaken-dev-desktop";
const rendererUrl = "http://127.0.0.1:5174/";
const readyTimeoutMs = 60_000;
const statusReadyTimeoutMs = 30_000;
const statusActivateIntervalMs = 2_000;
const pollIntervalMs = 500;
const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
const electronAppPath = join(rootDir, "node_modules/electron/dist/Electron.app");
const electronExecutable = join(electronAppPath, "Contents/MacOS/Electron");
const statusOnly = process.argv.includes("--status");

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: rootDir,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  });

  return {
    ok: result.status === 0,
    status: result.status,
    stdout: result.stdout?.trim() ?? "",
    stderr: result.stderr?.trim() ?? "",
    error: result.error,
  };
}

function shellQuote(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

async function canReachRenderer() {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 700);
  timeout.unref();

  try {
    const response = await fetch(rendererUrl, {
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

function screenInfo() {
  const result = run("screen", ["-ls"]);
  const output = [result.stdout, result.stderr].filter(Boolean).join("\n");
  return {
    available: !result.error,
    running: output.includes(`.${screenName}`) || output.includes(`\t${screenName}`),
    output,
  };
}

function processInfo() {
  const result = run("ps", ["-axo", "pid=,ppid=,args="]);
  if (!result.ok) {
    return { vite: [], electron: [], core: [], managed: [] };
  }

  const rows = result.stdout
    .split("\n")
    .map((line) => line.trim().match(/^(\d+)\s+(\d+)\s+(.*)$/))
    .filter(Boolean)
    .map((match) => ({ pid: Number(match[1]), ppid: Number(match[2]), args: match[3] }));

  const vite = rows.filter(
    (row) =>
      row.args.includes(`${rootDir}/node_modules/.bin/vite`) &&
      row.args.includes("--host 127.0.0.1") &&
      row.args.includes("--port 5174"),
  );
  const electron = rows.filter(
    (row) =>
      row.args.includes(electronExecutable) &&
      row.args.includes(". --dev"),
  );
  const core = rows.filter(
    (row) =>
      row.args.includes("/awaken-core") &&
      row.args.includes(`${rootDir}/apps/desktop/resources/bin/`),
  );
  const explicitlyManaged = rows.filter(
    (row) =>
      vite.some((process) => process.pid === row.pid) ||
      electron.some((process) => process.pid === row.pid) ||
      core.some((process) => process.pid === row.pid) ||
      row.args.includes(`cd ${shellQuote(rootDir)} && npm run dev:desktop`) ||
      row.args.includes(`${rootDir}/node_modules/.bin/concurrently`) ||
      (row.args.includes(`${rootDir}/node_modules/.bin/tsc`) &&
        row.args.includes("tsconfig.node.json")) ||
      row.args.includes(`${rootDir}/node_modules/.bin/electron`),
  );
  const managedPids = new Set(explicitlyManaged.map((row) => row.pid));
  let changed = true;
  while (changed) {
    changed = false;
    for (const row of rows) {
      if (!managedPids.has(row.pid) && managedPids.has(row.ppid)) {
        managedPids.add(row.pid);
        changed = true;
      }
    }
  }

  return {
    vite,
    electron,
    core,
    managed: rows.filter((row) => managedPids.has(row.pid)),
  };
}

function startDesktopScreen() {
  const command = [
    "unset ELECTRON_FORCE_IS_PACKAGED",
    'export NO_PROXY="${NO_PROXY:+$NO_PROXY,}localhost,127.0.0.1,::1"',
    'export no_proxy="${no_proxy:+$no_proxy,}localhost,127.0.0.1,::1"',
    `cd ${shellQuote(rootDir)} && npm run dev:desktop`,
  ].join("; ");
  const result = run("screen", ["-dmS", screenName, "bash", "-lc", command]);
  if (!result.ok) {
    throw new Error(
      [
        `Failed to start screen session ${screenName}.`,
        result.stderr || result.stdout || result.error?.message,
      ]
        .filter(Boolean)
        .join("\n"),
    );
  }
}

function hasDesktopProcess(status) {
  return (
    status.processes.vite.length > 0 ||
    status.processes.electron.length > 0 ||
    status.processes.core.length > 0
  );
}

function hasReadyProcesses(status) {
  return (
    status.rendererReachable &&
    status.processes.electron.length > 0 &&
    status.processes.core.length > 0
  );
}

function shouldCleanStart(status) {
  if (!hasDesktopProcess(status)) {
    return false;
  }

  const missingCore =
    (status.processes.vite.length > 0 || status.processes.electron.length > 0) &&
    status.processes.core.length === 0;
  return !status.screen.running || missingCore;
}

async function stopManagedDesktopProcesses(reason) {
  console.log(reason);
  run("screen", ["-S", screenName, "-X", "quit"]);
  await delay(1000);

  let processes = processInfo();
  let pids = processes.managed.map((row) => row.pid);
  if (pids.length > 0) {
    run("kill", pids.map(String));
    await delay(1500);
  }

  processes = processInfo();
  pids = processes.managed.map((row) => row.pid);
  if (pids.length > 0) {
    run("kill", ["-9", ...pids.map(String)]);
    await delay(500);
  }
}

function activateDarwinApp() {
  if (process.platform !== "darwin") {
    return;
  }

  run("open", ["-a", electronAppPath]);
}

function swiftWindowScript(pids) {
  const pidArray = `[${pids.join(",")}]`;
  return `
import Foundation
import CoreGraphics

let targetPids = Set<Int>(${pidArray})
let options = CGWindowListOption(arrayLiteral: .optionOnScreenOnly, .excludeDesktopElements)
let windows = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] ?? []
var rows: [[String: Any]] = []

for window in windows {
  let pid = window[kCGWindowOwnerPID as String] as? Int ?? -1
  let name = window[kCGWindowName as String] as? String ?? ""
  if targetPids.contains(pid) && name == "Awaken Desktop" {
    let bounds = window[kCGWindowBounds as String] as? [String: Any] ?? [:]
    rows.append([
      "pid": pid,
      "name": name,
      "windowNumber": window[kCGWindowNumber as String] as? Int ?? -1,
      "x": bounds["X"] as? Double ?? 0,
      "y": bounds["Y"] as? Double ?? 0,
      "width": bounds["Width"] as? Double ?? 0,
      "height": bounds["Height"] as? Double ?? 0
    ])
  }
}

let data = try! JSONSerialization.data(withJSONObject: rows, options: [])
print(String(data: data, encoding: .utf8)!)
`;
}

function windowInfo(processes) {
  if (process.platform !== "darwin" || processes.electron.length === 0) {
    return { checked: false, windows: [] };
  }

  const result = run("swift", ["-e", swiftWindowScript(processes.electron.map((row) => row.pid))]);
  if (!result.ok || !result.stdout) {
    return { checked: true, windows: [], error: result.stderr || result.error?.message };
  }

  try {
    return { checked: true, windows: JSON.parse(result.stdout) };
  } catch (error) {
    return { checked: true, windows: [], error: error.message };
  }
}

function isUsableWindow(window) {
  return (
    window &&
    window.width >= 1000 &&
    window.height >= 700 &&
    window.x + window.width > 200 &&
    window.y + window.height > 200
  );
}

async function collectStatus() {
  const processes = processInfo();
  const windows = windowInfo(processes);
  const rendererReachable = await canReachRenderer();
  const screen = screenInfo();
  const usableWindow = windows.windows.find(isUsableWindow);

  return {
    screen,
    processes,
    windows,
    rendererReachable,
    usableWindow,
    ready:
      rendererReachable &&
      processes.electron.length > 0 &&
      processes.core.length > 0 &&
      (!windows.checked || Boolean(usableWindow)),
  };
}

function printStatus(status) {
  const window = status.usableWindow ?? status.windows.windows[0];
  console.log("Awaken Desktop status");
  console.log(`- screen: ${status.screen.running ? screenName : "not running"}`);
  console.log(`- renderer: ${status.rendererReachable ? rendererUrl : "not reachable"}`);
  console.log(
    `- electron: ${
      status.processes.electron.length > 0
        ? status.processes.electron.map((row) => row.pid).join(", ")
        : "not running"
    }`,
  );
  console.log(
    `- awaken-core: ${
      status.processes.core.length > 0
        ? status.processes.core.map((row) => row.pid).join(", ")
        : "not running"
    }`,
  );
  if (status.windows.checked) {
    console.log(
      `- window: ${
        window
          ? `${window.name} ${Math.round(window.width)}x${Math.round(window.height)} at ${Math.round(window.x)},${Math.round(window.y)}`
          : "not visible"
      }`,
    );
  }
}

async function waitUntilReady() {
  const deadline = Date.now() + readyTimeoutMs;
  let lastStatus = await collectStatus();
  let activated = false;

  while (Date.now() < deadline) {
    if (lastStatus.ready) {
      return lastStatus;
    }

    if (lastStatus.processes.electron.length > 0 && !activated) {
      activateDarwinApp();
      activated = true;
    }

    await delay(pollIntervalMs);
    lastStatus = await collectStatus();
  }

  return lastStatus;
}

async function settleStatus(initialStatus) {
  if (!hasReadyProcesses(initialStatus) || !initialStatus.windows.checked || initialStatus.usableWindow) {
    return initialStatus;
  }

  activateDarwinApp();

  const deadline = Date.now() + statusReadyTimeoutMs;
  let lastStatus = initialStatus;
  let lastActivateAt = Date.now();
  while (Date.now() < deadline) {
    await delay(pollIntervalMs);
    lastStatus = await collectStatus();
    if (lastStatus.ready || !hasReadyProcesses(lastStatus)) {
      return lastStatus;
    }
    if (Date.now() - lastActivateAt >= statusActivateIntervalMs) {
      activateDarwinApp();
      lastActivateAt = Date.now();
    }
  }
  return lastStatus;
}

const initialStatus = await collectStatus();

if (statusOnly) {
  const status = await settleStatus(initialStatus);
  printStatus(status);
  process.exit(status.ready ? 0 : 1);
}

if (initialStatus.ready) {
  activateDarwinApp();
  printStatus(initialStatus);
  process.exit(0);
}

if (!initialStatus.screen.available) {
  console.error("screen is not available; install screen or run npm run dev:desktop manually.");
  process.exit(1);
}

let startupStatus = initialStatus;

if (shouldCleanStart(startupStatus)) {
  await stopManagedDesktopProcesses(
    "Found stale Awaken Desktop dev processes; cleaning them before restart...",
  );
  startupStatus = await collectStatus();
}

if (!startupStatus.screen.running && !hasDesktopProcess(startupStatus)) {
  console.log(`Starting Awaken Desktop in screen session ${screenName}...`);
  startDesktopScreen();
} else if (startupStatus.screen.running) {
  console.log(`Reusing screen session ${screenName}...`);
} else {
  console.log("Found existing Awaken Desktop dev processes outside the managed screen session...");
}

let finalStatus = await waitUntilReady();

if (!finalStatus.ready && hasDesktopProcess(finalStatus)) {
  await stopManagedDesktopProcesses(
    "Awaken Desktop did not become ready; retrying once from a clean dev process set...",
  );
  console.log(`Starting Awaken Desktop in screen session ${screenName}...`);
  startDesktopScreen();
  finalStatus = await waitUntilReady();
}
printStatus(finalStatus);

if (!finalStatus.ready) {
  console.error(
    [
      "Awaken Desktop did not become fully ready.",
      `Inspect the dev session with: screen -r ${screenName}`,
      "If a stale session is blocking startup, stop it first and rerun npm run open:desktop.",
    ].join("\n"),
  );
  process.exit(1);
}
