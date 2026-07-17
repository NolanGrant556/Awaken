#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const screenName = "awaken-dev-desktop";
const isWindows = process.platform === "win32";
const electronExecutable = isWindows
  ? join(rootDir, "node_modules/electron/dist/electron.exe")
  : join(rootDir, "node_modules/electron/dist/Electron.app/Contents/MacOS/Electron");
const gracefulTimeoutMs = 2_000;

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: rootDir,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });

  return {
    ok: result.status === 0,
    stdout: result.stdout?.trim() ?? "",
    stderr: result.stderr?.trim() ?? "",
    error: result.error,
  };
}

function screenInfo() {
  const result = run("screen", ["-ls"]);
  const output = [result.stdout, result.stderr].filter(Boolean).join("\n");
  return {
    available: !result.error,
    running: output.includes(`.${screenName}`) || output.includes(`\t${screenName}`),
  };
}

function normalizeCommandLine(value) {
  return (value ?? "").replace(/\\/g, "/").toLowerCase();
}

function matchesProcess(commandLine) {
  const cmd = normalizeCommandLine(commandLine);
  return (
    (cmd.includes("/awaken-core") && cmd.includes("resources/bin")) ||
    (cmd.includes("vite") &&
      cmd.includes("--host 127.0.0.1") &&
      cmd.includes("--port 5174")) ||
    (cmd.includes("tsc") &&
      cmd.includes("tsconfig.node.json") &&
      cmd.includes("--watch")) ||
    (cmd.includes("electron") && cmd.includes(". --dev"))
  );
}

function processInfoUnix() {
  const result = run("ps", ["-axo", "pid=,args="]);
  if (!result.ok) {
    return [];
  }

  return result.stdout
    .split("\n")
    .map((line) => line.trim().match(/^(\d+)\s+(.*)$/))
    .filter(Boolean)
    .map((match) => ({ pid: Number(match[1]), args: match[2] }))
    .filter(
      (row) =>
        matchesProcess(row.args) ||
        (row.args.includes(`${rootDir}/node_modules/.bin/vite`) &&
          row.args.includes("--host 127.0.0.1") &&
          row.args.includes("--port 5174")) ||
        (row.args.includes(electronExecutable) && row.args.includes(". --dev")) ||
        (row.args.includes("/awaken-core") &&
          row.args.includes(`${rootDir}/apps/desktop/resources/bin/`)),
    );
}

function processInfoWindows() {
  const command =
    'Get-CimInstance Win32_Process | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress';
  const result = run("powershell.exe", ["-NoProfile", "-Command", command]);
  if (!result.ok) {
    return [];
  }

  let parsed;
  try {
    parsed = JSON.parse(result.stdout);
  } catch {
    return [];
  }
  const rows = Array.isArray(parsed) ? parsed : [parsed];
  return rows
    .map((row) => ({ pid: Number(row.ProcessId), args: row.CommandLine ?? "" }))
    .filter((row) => matchesProcess(row.args));
}

function processInfo() {
  return isWindows ? processInfoWindows() : processInfoUnix();
}

function terminate(pid, signal) {
  if (isWindows) {
    const force = signal === "SIGKILL";
    const result = run("taskkill", ["/PID", String(pid), "/T", force ? "/F" : ""].filter(Boolean));
    return result.ok || result.stdout.includes("not found") || result.stderr.includes("not found");
  }

  try {
    process.kill(pid, signal);
    return true;
  } catch (error) {
    return error?.code === "ESRCH";
  }
}

async function stopProcesses() {
  const initial = processInfo();
  for (const processRow of initial) {
    terminate(processRow.pid, "SIGTERM");
  }

  const deadline = Date.now() + gracefulTimeoutMs;
  while (Date.now() < deadline) {
    if (processInfo().length === 0) {
      return;
    }
    await delay(200);
  }

  for (const processRow of processInfo()) {
    terminate(processRow.pid, "SIGKILL");
  }
}

if (!isWindows) {
  const screen = screenInfo();
  if (screen.available && screen.running) {
    run("screen", ["-S", screenName, "-X", "quit"]);
  }
}

await stopProcesses();

const remaining = processInfo();
console.log("Awaken Desktop closed");
if (!isWindows) {
  const screen = screenInfo();
  console.log(`- screen: ${screen.running ? `${screenName} stopped` : "not running"}`);
}
console.log(
  `- processes: ${
    remaining.length === 0 ? "stopped" : remaining.map((processRow) => processRow.pid).join(", ")
  }`,
);

process.exit(remaining.length === 0 ? 0 : 1);
