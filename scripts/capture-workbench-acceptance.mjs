#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const defaultOutDir = join(tmpdir(), "awaken-workbench-acceptance");

function usage() {
  console.log(`Capture the Awaken Desktop workbench window for lightweight visual acceptance.

Usage:
  npm run capture:workbench -- --state center
  npm run capture:workbench -- --state dock --out /tmp/awaken-acceptance
  npm run capture:workbench -- --fresh --state center
  npm run capture:workbench -- --state marker --capture-mode region

Options:
  --state <name>          Label for the screenshot, e.g. center, dock, marker.
  --out <dir>             Output directory. Defaults to ${defaultOutDir}
  --capture-mode <mode>   Capture mode: window or region. Defaults to window, with region fallback.
  --settle-ms <ms>        Wait before capture. Defaults to 500.
  --fresh                 Close and reopen Awaken Desktop before capturing.
  --no-open               Do not run open:desktop before capturing.
  --help                  Show this help.
`);
}

function parseArgs(argv) {
  const args = {
    state: "workbench",
    outDir: defaultOutDir,
    fresh: false,
    open: true,
    help: false,
    captureMode: "window",
    settleMs: 500,
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
    if (arg === "--state") {
      args.state = argv[index + 1] ?? "";
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

  if (!args.state.trim()) {
    throw new Error("--state must not be empty.");
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

function swiftWindowScript() {
  return `
import Foundation
import CoreGraphics

let options = CGWindowListOption(arrayLiteral: .optionOnScreenOnly, .excludeDesktopElements)
let windows = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] ?? []
var rows: [[String: Any]] = []

for window in windows {
  let name = window[kCGWindowName as String] as? String ?? ""
  let ownerName = window[kCGWindowOwnerName as String] as? String ?? ""
  if name == "Awaken Desktop" && ownerName == "Electron" {
    let bounds = window[kCGWindowBounds as String] as? [String: Any] ?? [:]
    rows.append([
      "pid": window[kCGWindowOwnerPID as String] as? Int ?? -1,
      "ownerName": ownerName,
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

function isUsableWindow(window) {
  return window.width >= 1000 && window.height >= 700;
}

function findAwakenWindow() {
  if (process.platform !== "darwin") {
    throw new Error("capture-workbench-acceptance currently supports macOS screencapture only.");
  }

  const result = runRequired("swift", ["-e", swiftWindowScript()]);
  const windows = JSON.parse(result.stdout || "[]");
  const window = windows.find(isUsableWindow) ?? windows[0];
  if (!window) {
    throw new Error("Could not find a visible Awaken Desktop window.");
  }
  return {
    ...window,
    x: Math.round(window.x),
    y: Math.round(window.y),
    width: Math.round(window.width),
    height: Math.round(window.height),
  };
}

function slug(value) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "") || "workbench";
}

function timestamp() {
  return new Date().toISOString().replaceAll(":", "-").replace(/\.\d{3}Z$/, "Z");
}

function movePointerAway(window) {
  const x = Math.max(window.x + 24, 0);
  const y = Math.max(window.y + 24, 0);
  run("swift", [
    "-e",
    `
import CoreGraphics
CGWarpMouseCursorPosition(CGPoint(x: ${x}, y: ${y}))
CGAssociateMouseAndMouseCursorPosition(boolean_t(1))
`,
  ]);
  run("sleep", ["0.2"]);
}

function waitForSettle(settleMs) {
  if (settleMs > 0) {
    run("sleep", [String(settleMs / 1000)]);
  }
}

function captureArgsForWindow({ window, captureMode, screenshotPath }) {
  const region = `${window.x},${window.y},${window.width},${window.height}`;
  if (captureMode === "region") {
    return {
      args: ["-x", "-R", region, screenshotPath],
      region,
    };
  }

  if (!Number.isFinite(window.windowNumber) || window.windowNumber <= 0) {
    throw new Error("Awaken Desktop window number is unavailable; retry with --capture-mode region.");
  }

  return {
    args: ["-x", "-l", String(window.windowNumber), screenshotPath],
    region,
  };
}

function captureScreenshot({ capture, requestedCaptureMode }) {
  const result = run("screencapture", capture.args);
  if (result.ok) {
    return {
      actualCaptureMode: requestedCaptureMode,
      fallbackReason: null,
      capture,
    };
  }

  if (requestedCaptureMode !== "window") {
    throw new Error(
      [
        `Command failed: screencapture ${capture.args.join(" ")}`,
        result.stderr,
        result.stdout,
        result.error?.message,
      ]
        .filter(Boolean)
        .join("\n"),
    );
  }

  const fallbackCapture = captureArgsForWindow({
    window: capture.window,
    captureMode: "region",
    screenshotPath: capture.screenshotPath,
  });
  const fallbackResult = run("screencapture", fallbackCapture.args);
  if (!fallbackResult.ok) {
    throw new Error(
      [
        `Command failed: screencapture ${capture.args.join(" ")}`,
        result.stderr,
        result.stdout,
        result.error?.message,
        `Fallback failed: screencapture ${fallbackCapture.args.join(" ")}`,
        fallbackResult.stderr,
        fallbackResult.stdout,
        fallbackResult.error?.message,
      ]
        .filter(Boolean)
        .join("\n"),
    );
  }

  return {
    actualCaptureMode: "region",
    fallbackReason: [result.stderr, result.stdout, result.error?.message].filter(Boolean).join("\n"),
    capture: fallbackCapture,
  };
}

function captureWindow({ state, outDir, captureMode, settleMs }) {
  waitForSettle(settleMs);
  const window = findAwakenWindow();
  movePointerAway(window);

  const captureDir = resolve(outDir, `${timestamp()}-${slug(state)}`);
  mkdirSync(captureDir, { recursive: true });

  const screenshotPath = join(captureDir, `${slug(state)}.png`);
  const requestedCapture = captureArgsForWindow({ window, captureMode, screenshotPath });
  const result = captureScreenshot({
    capture: { ...requestedCapture, window, screenshotPath },
    requestedCaptureMode: captureMode,
  });

  const manifest = {
    state,
    capturedAt: new Date().toISOString(),
    screenshotPath,
    window,
    region: result.capture.region,
    captureMode: result.actualCaptureMode,
    requestedCaptureMode: captureMode,
    settleMs,
    fallbackReason: result.fallbackReason,
  };
  const manifestPath = join(captureDir, "manifest.json");
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  console.log("Workbench acceptance capture complete");
  console.log(`- state: ${state}`);
  console.log(`- capture mode: ${result.actualCaptureMode}`);
  console.log(`- settle ms: ${settleMs}`);
  if (result.fallbackReason) {
    console.log(`- fallback: ${result.fallbackReason}`);
  }
  console.log(`- screenshot: ${screenshotPath}`);
  console.log(`- manifest: ${manifestPath}`);
  console.log(`- window: ${window.name} ${window.width}x${window.height} at ${window.x},${window.y}`);
}

try {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    usage();
    process.exit(0);
  }

  if (args.fresh) {
    runRequired("node", ["scripts/close-desktop.mjs"]);
  }
  if (args.open) {
    runRequired("node", ["scripts/open-desktop.mjs"]);
  }

  captureWindow({
    state: args.state,
    outDir: args.outDir,
    captureMode: args.captureMode,
    settleMs: args.settleMs,
  });
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
