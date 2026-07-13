#!/usr/bin/env node

import {
  readFileSync,
  rmSync,
} from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export const desktopPackageOutputPaths = [
  "apps/desktop/dist",
  "apps/desktop/dist-electron",
  "apps/desktop/resources/bin",
];

function packageName(path) {
  return JSON.parse(readFileSync(path, "utf8"))?.name;
}

export function assertAwakenRepository(root) {
  if (
    packageName(join(root, "package.json")) !== "awaken" ||
    packageName(join(root, "apps", "desktop", "package.json")) !== "@awaken/desktop"
  ) {
    throw new Error(`refusing to clean outside the Awaken repository: ${root}`);
  }
}

export function cleanDesktopPackageOutputs(root = scriptRoot) {
  const resolvedRoot = resolve(root);
  assertAwakenRepository(resolvedRoot);

  const removed = [];
  for (const relativePath of desktopPackageOutputPaths) {
    const target = resolve(resolvedRoot, relativePath);
    if (relative(resolvedRoot, target).startsWith("..")) {
      throw new Error(`refusing to clean outside the Awaken repository: ${target}`);
    }
    rmSync(target, { recursive: true, force: true });
    removed.push(relativePath);
  }
  return removed;
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  try {
    const removed = cleanDesktopPackageOutputs();
    console.log("Desktop package staging cleaned");
    for (const path of removed) {
      console.log(`- ${path}`);
    }
  } catch (error) {
    console.error(
      `clean-desktop-package: ${error instanceof Error ? error.message : String(error)}`,
    );
    process.exit(1);
  }
}
