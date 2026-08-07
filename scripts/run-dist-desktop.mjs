#!/usr/bin/env node
// Wrapper for `npm run dist:desktop` that keeps everything on stdout.
//
// When invoked from Windows PowerShell, `npm.cmd run dist:desktop` fails with
// `NativeCommandError` if any child process (vite reporter, electron-builder
// warnings, etc.) writes to stderr — even though electron-builder itself
// completes successfully. PowerShell treats those stderr bytes as a fatal
// external command error and rejects the whole pipeline.
//
// This script runs the same steps as the previous dist:desktop chain but
// funnels the children's stderr into its own stdout, so the outer npm.cmd has
// nothing on stderr for PowerShell to trip over.

import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const npmCmd = process.platform === "win32" ? "npm.cmd" : "npm";

const steps = [
  { label: "build:ui", args: ["run", "build:ui"] },
  { label: "workspace @awaken/desktop dist", args: ["--workspace", "@awaken/desktop", "run", "dist"] },
];

function runStep({ label, args }) {
  return new Promise((resolvePromise, rejectPromise) => {
    console.log(`\n[run-dist-desktop] > ${label}`);
    // shell: true is required on Windows to spawn npm.cmd (Node 22+ rejects
    // direct .cmd/.bat spawns without a shell for security reasons).
    const child = spawn(npmCmd, args, {
      cwd: repoRoot,
      env: process.env,
      windowsHide: true,
      shell: true,
      stdio: ["inherit", "pipe", "pipe"],
    });
    child.stdout.on("data", (chunk) => process.stdout.write(chunk));
    // Merge child stderr into our stdout so PowerShell 5 does not treat
    // non-fatal warnings from vite / electron-builder as NativeCommandError.
    child.stderr.on("data", (chunk) => process.stdout.write(chunk));
    child.on("error", rejectPromise);
    child.on("close", (code) => {
      if (code === 0) {
        resolvePromise();
      } else {
        rejectPromise(new Error(`step "${label}" exited with code ${code}`));
      }
    });
  });
}

async function main() {
  for (const step of steps) {
    await runStep(step);
  }
  console.log("\n[run-dist-desktop] all steps completed");
}

main().catch((err) => {
  console.log(`\n[run-dist-desktop] failed: ${err.message}`);
  process.exit(1);
});
