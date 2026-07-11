#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import {
  closeCompetingPackagedAwaken,
  listPackagedAwakenProcesses,
} from "./desktop-lifecycle-common.mjs";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const screenName = "awaken-dev-desktop";
const rendererUrl = "http://127.0.0.1:5174/";
const readyTimeoutMs = 60_000;
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
  const result = run("ps", ["-axo", "pid=,args="]);
  if (!result.ok) {
    return { vite: [], electron: [], core: [], packaged: [] };
  }

  const rows = result.stdout
    .split("\n")
    .map((line) => line.trim().match(/^(\d+)\s+(.*)$/))
    .filter(Boolean)
    .map((match) => ({ pid: Number(match[1]), args: match[2] }));

  return {
    vite: rows.filter(
      (row) =>
        row.args.includes(`${rootDir}/node_modules/.bin/vite`) &&
        row.args.includes("--host 127.0.0.1") &&
        row.args.includes("--port 5174"),
    ),
    electron: rows.filter(
      (row) =>
        row.args.includes(electronExecutable) &&
        row.args.includes(". --dev"),
    ),
    core: rows.filter(
      (row) =>
        row.args.includes("/awaken-core") &&
        row.args.includes(`${rootDir}/apps/desktop/resources/bin/`),
    ),
    packaged: listPackagedAwakenProcesses(),
  };
}

function startDesktopScreen() {
  const command = `cd ${shellQuote(rootDir)} && npm run dev:desktop`;
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

function stopDesktopScreen() {
  return run("screen", ["-S", screenName, "-X", "quit"]);
}

function activateDarwinApp(processes) {
  if (process.platform !== "darwin" || processes.electron.length === 0) {
    return;
  }

  const pid = processes.electron[0].pid;
  const script = [
    "import AppKit",
    `if let target = NSRunningApplication(processIdentifier: pid_t(${pid})) {`,
    "  _ = target.activate(options: [.activateAllWindows])",
    "}",
  ].join("\n");
  run("swift", ["-e", script]);
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
      processes.packaged.length === 0 &&
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
    `- competing packaged Awaken: ${
      status.processes.packaged.length > 0
        ? status.processes.packaged.map((row) => row.pid).join(", ")
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
      activateDarwinApp(lastStatus.processes);
      activated = true;
    }

    await delay(pollIntervalMs);
    lastStatus = await collectStatus();
  }

  return lastStatus;
}

if (!statusOnly) {
  try {
    const competing = await closeCompetingPackagedAwaken();
    if (competing.closed.length > 0) {
      console.log(
        `Closed competing packaged Awaken instance(s): ${competing.closed
          .map((row) => row.pid)
          .join(", ")}`,
      );
    }
  } catch (error) {
    console.error(
      `Awaken Desktop cannot establish the README-owned instance: ${
        error instanceof Error ? error.message : String(error)
      }`,
    );
    process.exit(1);
  }
}

const initialStatus = await collectStatus();

if (statusOnly) {
  printStatus(initialStatus);
  process.exit(initialStatus.ready ? 0 : 1);
}

if (initialStatus.ready) {
  activateDarwinApp(initialStatus.processes);
  printStatus(initialStatus);
  process.exit(0);
}

if (!initialStatus.screen.available) {
  console.error("screen is not available; install screen or run npm run dev:desktop manually.");
  process.exit(1);
}

const hasDesktopProcess =
  initialStatus.processes.vite.length > 0 ||
  initialStatus.processes.electron.length > 0 ||
  initialStatus.processes.core.length > 0;

if (!hasDesktopProcess) {
  if (initialStatus.screen.running) {
    console.log(`Resetting stale screen session ${screenName}...`);
    stopDesktopScreen();
  }
  console.log(`Starting Awaken Desktop in screen session ${screenName}...`);
  startDesktopScreen();
} else if (initialStatus.screen.running) {
  console.log(`Reusing screen session ${screenName}...`);
} else {
  console.log("Found existing Awaken Desktop dev processes outside the managed screen session...");
}

const finalStatus = await waitUntilReady();
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
