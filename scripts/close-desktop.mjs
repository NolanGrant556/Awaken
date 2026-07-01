#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const screenName = "awaken-dev-desktop";
const electronExecutable = join(
  rootDir,
  "node_modules/electron/dist/Electron.app/Contents/MacOS/Electron",
);
const gracefulTimeoutMs = 2_000;

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: rootDir,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
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

function processInfo() {
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
        (row.args.includes(`${rootDir}/node_modules/.bin/vite`) &&
          row.args.includes("--host 127.0.0.1") &&
          row.args.includes("--port 5174")) ||
        (row.args.includes(electronExecutable) && row.args.includes(". --dev")) ||
        (row.args.includes("/awaken-core") &&
          row.args.includes(`${rootDir}/apps/desktop/resources/bin/`)),
    );
}

function terminate(pid, signal) {
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

const screen = screenInfo();
if (screen.available && screen.running) {
  run("screen", ["-S", screenName, "-X", "quit"]);
}

await stopProcesses();

const remaining = processInfo();
console.log("Awaken Desktop closed");
console.log(`- screen: ${screen.running ? `${screenName} stopped` : "not running"}`);
console.log(
  `- processes: ${
    remaining.length === 0 ? "stopped" : remaining.map((processRow) => processRow.pid).join(", ")
  }`,
);

process.exit(remaining.length === 0 ? 0 : 1);
