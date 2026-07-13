#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

if (process.platform !== "win32") {
  console.log(
    "[package-primary-runtime-for-desktop] local non-Windows package does not embed an unsigned runtime archive",
  );
  process.exit(0);
}

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const script = join(root, "scripts", "package-primary-runtime-archive.mjs");
const result = spawnSync(process.execPath, [script, ...process.argv.slice(2)], {
  cwd: process.cwd(),
  env: process.env,
  stdio: "inherit",
});
if (result.error) {
  console.error(
    `[package-primary-runtime-for-desktop] ${result.error.message}`,
  );
  process.exit(1);
}
process.exit(result.status ?? 1);
