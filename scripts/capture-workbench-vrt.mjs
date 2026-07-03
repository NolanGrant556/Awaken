#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { createInterface } from "node:readline/promises";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const defaultOutDir = join(tmpdir(), "awaken-workbench-vrt");
const defaultStates = ["center", "dock", "marker"];

function usage() {
  console.log(`Capture a lightweight Workbench VRT bundle for PR review.

This script reuses capture-workbench-acceptance.mjs for each requested state and
writes one aggregate manifest. In an interactive terminal it pauses before each
state so you can prepare center, dock, and marker views in the real Desktop app.

Usage:
  npm run capture:workbench:vrt
  npm run capture:workbench:vrt -- --fresh
  npm run capture:workbench:vrt -- --state center --state marker
  npm run capture:workbench:vrt -- --non-interactive --state center --no-open

Options:
  --state <name>          State to capture. Repeatable. Defaults to center,dock,marker.
  --states <csv>          Comma-separated states to capture.
  --out <dir>             Output directory. Defaults to ${defaultOutDir}
  --capture-mode <mode>   Capture mode passed to acceptance capture: window or region. Defaults to window.
  --settle-ms <ms>        Wait before each capture. Defaults to 1000.
  --fresh                 Close and reopen Awaken Desktop before the first capture.
  --no-open               Do not run open:desktop before capturing.
  --interactive           Pause before each state, even if auto-detection would not.
  --non-interactive       Capture states immediately without prompts.
  --help                  Show this help.
`);
}

function splitStates(value) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseArgs(argv) {
  const args = {
    states: [],
    outDir: defaultOutDir,
    captureMode: "window",
    settleMs: 1000,
    fresh: false,
    open: true,
    interactive: undefined,
    help: false,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--help" || arg === "-h") {
      args.help = true;
      continue;
    }
    if (arg === "--fresh") {
      args.fresh = true;
      continue;
    }
    if (arg === "--no-open") {
      args.open = false;
      continue;
    }
    if (arg === "--interactive") {
      args.interactive = true;
      continue;
    }
    if (arg === "--non-interactive") {
      args.interactive = false;
      continue;
    }
    if (arg === "--state") {
      args.states.push(...splitStates(argv[index + 1] ?? ""));
      index += 1;
      continue;
    }
    if (arg === "--states") {
      args.states.push(...splitStates(argv[index + 1] ?? ""));
      index += 1;
      continue;
    }
    if (arg === "--out") {
      args.outDir = argv[index + 1] ?? "";
      index += 1;
      continue;
    }
    if (arg === "--capture-mode") {
      args.captureMode = argv[index + 1] ?? "";
      index += 1;
      continue;
    }
    if (arg === "--settle-ms") {
      args.settleMs = Number(argv[index + 1] ?? Number.NaN);
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  if (args.states.length === 0) {
    args.states = [...defaultStates];
  }
  args.states = Array.from(new Set(args.states));

  if (args.states.length === 0) {
    throw new Error("At least one state is required.");
  }
  if (!args.outDir.trim()) {
    throw new Error("--out must not be empty.");
  }
  if (!["window", "region"].includes(args.captureMode)) {
    throw new Error("--capture-mode must be one of: window, region.");
  }
  if (!Number.isFinite(args.settleMs) || args.settleMs < 0) {
    throw new Error("--settle-ms must be a non-negative number.");
  }
  if (args.fresh && !args.open) {
    throw new Error("--fresh cannot be combined with --no-open.");
  }
  if (args.interactive === true && !process.stdin.isTTY) {
    throw new Error("--interactive requires a TTY. Use --non-interactive for automation.");
  }

  return args;
}

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

function runRequired(command, args, options = {}) {
  const result = run(command, args, options);
  if (!result.ok) {
    throw new Error(
      [
        `Command failed: ${command} ${args.join(" ")}`,
        result.stderr,
        result.stdout,
        result.error?.message,
      ]
        .filter(Boolean)
        .join("\n"),
    );
  }
  return result;
}

function timestamp() {
  return new Date().toISOString().replaceAll(":", "-").replace(/\.\d{3}Z$/, "Z");
}

function captureManifestPath(output) {
  const line = output
    .split("\n")
    .map((item) => item.trim())
    .find((item) => item.startsWith("- manifest:"));

  if (!line) {
    throw new Error(`Could not find capture manifest path in output:\n${output}`);
  }

  const manifestPath = line.replace("- manifest:", "").trim();
  if (!manifestPath || !existsSync(manifestPath)) {
    throw new Error(`Capture manifest was not written: ${manifestPath}`);
  }
  return manifestPath;
}

function gitSnapshot() {
  const branch = run("git", ["rev-parse", "--abbrev-ref", "HEAD"]);
  const commit = run("git", ["rev-parse", "HEAD"]);
  const status = run("git", ["status", "--short"]);
  const statusShort = status.ok ? status.stdout.split("\n").filter(Boolean) : [];

  return {
    branch: branch.ok ? branch.stdout : null,
    commit: commit.ok ? commit.stdout : null,
    dirty: statusShort.length > 0,
    statusShort,
  };
}

function stateInstruction(state) {
  const instructions = {
    center: "Prepare the empty Workbench state with the center composer visible.",
    dock: "Prepare a run state with the dock composer and ExecutionFeed visible.",
    marker: "Prepare a multi-turn run where the turn marker rail is visible.",
  };
  return instructions[state] ?? `Prepare the Workbench state named "${state}".`;
}

async function waitForState({ rl, state }) {
  console.log("");
  console.log(`State: ${state}`);
  console.log(stateInstruction(state));
  await rl.question("Press Enter to capture this state...");
}

function captureState({ state, runDir, captureMode, settleMs }) {
  const result = runRequired("node", [
    "scripts/capture-workbench-acceptance.mjs",
    "--state",
    state,
    "--out",
    runDir,
    "--no-open",
    "--capture-mode",
    captureMode,
    "--settle-ms",
    String(settleMs),
  ]);
  const manifestPath = captureManifestPath(result.stdout);
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));

  return {
    state,
    manifestPath,
    screenshotPath: manifest.screenshotPath,
    capturedAt: manifest.capturedAt,
    captureMode: manifest.captureMode,
    requestedCaptureMode: manifest.requestedCaptureMode,
    settleMs: manifest.settleMs,
    fallbackReason: manifest.fallbackReason,
    window: manifest.window,
    region: manifest.region,
  };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    usage();
    return;
  }

  const interactive = args.interactive ?? Boolean(process.stdin.isTTY && process.stdout.isTTY);
  const runDir = resolve(args.outDir, `${timestamp()}-workbench-vrt`);
  mkdirSync(runDir, { recursive: true });

  if (args.fresh) {
    runRequired("node", ["scripts/close-desktop.mjs"]);
  }
  if (args.open) {
    runRequired("node", ["scripts/open-desktop.mjs"]);
  }

  if (!interactive) {
    console.log("Non-interactive mode captures the current Desktop window as-is.");
    console.log("Inspect screenshots before using them as PR evidence.");
    if (args.states.length > 1) {
      console.log("For distinct center/dock/marker states, run from a TTY or pass --interactive.");
    }
  }

  const rl = interactive
    ? createInterface({ input: process.stdin, output: process.stdout })
    : null;
  const captures = [];

  try {
    for (const state of args.states) {
      if (rl) {
        await waitForState({ rl, state });
      }
      captures.push(
        captureState({
          state,
          runDir,
          captureMode: args.captureMode,
          settleMs: args.settleMs,
        }),
      );
    }
  } finally {
    rl?.close();
  }

  const manifest = {
    kind: "awaken-workbench-vrt",
    generatedAt: new Date().toISOString(),
    rootDir,
    runDir,
    interactive,
    openedDesktop: args.open,
    freshDesktop: args.fresh,
    captureMode: args.captureMode,
    settleMs: args.settleMs,
    states: args.states,
    git: gitSnapshot(),
    captures,
  };
  const manifestPath = join(runDir, "workbench-vrt-manifest.json");
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  console.log("");
  console.log("Workbench VRT bundle complete");
  console.log(`- manifest: ${manifestPath}`);
  for (const capture of captures) {
    console.log(`- ${capture.state}: ${capture.screenshotPath}`);
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
