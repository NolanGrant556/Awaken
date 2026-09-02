#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import {
  existsSync,
  lstatSync,
  readdirSync,
  readFileSync,
} from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(import.meta.url);
const defaultRepoRoot = resolve(dirname(scriptPath), "..");
const builtinPluginsRelativeRoot = "apps/desktop/resources/builtin-plugins";

function posixRelative(root, path) {
  return relative(root, path).split(sep).join("/");
}

function regularFiles(root) {
  const files = [];
  const walk = (current) => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const path = join(current, entry.name);
      if (entry.isDirectory()) {
        walk(path);
      } else if (entry.isFile()) {
        files.push(path);
      }
    }
  };
  walk(root);
  return files;
}

function parseJson(path, label, errors) {
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    errors.push(`${label} is not valid JSON: ${error.message}`);
    return null;
  }
}

function trackedFiles(repoRoot, errors) {
  const result = spawnSync(
    "git",
    ["ls-files", "-z", "--", builtinPluginsRelativeRoot],
    { cwd: repoRoot, encoding: "utf8" },
  );
  if (result.error || result.status !== 0) {
    errors.push(
      `cannot inspect tracked built-in plugin resources: ${
        (result.stderr || result.error?.message || "git ls-files failed").trim()
      }`,
    );
    return new Set();
  }
  return new Set(result.stdout.split("\0").filter(Boolean));
}

export function validateBuiltinPlugins({
  repoRoot = defaultRepoRoot,
  requireTracked = false,
} = {}) {
  const errors = [];
  const builtinRoot = join(repoRoot, builtinPluginsRelativeRoot);
  if (!existsSync(builtinRoot) || !lstatSync(builtinRoot).isDirectory()) {
    return [`${builtinPluginsRelativeRoot} is missing or not a directory`];
  }

  const pluginDirectories = readdirSync(builtinRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && !entry.name.startsWith("."))
    .sort((a, b) => a.name.localeCompare(b.name));
  if (pluginDirectories.length === 0) {
    errors.push(`${builtinPluginsRelativeRoot} does not contain a built-in plugin`);
  }

  const tracked = requireTracked ? trackedFiles(repoRoot, errors) : null;
  for (const entry of pluginDirectories) {
    const pluginRoot = join(builtinRoot, entry.name);
    const manifestPath = join(pluginRoot, "plugin.json");
    const builtinMetadataPath = join(pluginRoot, ".awaken", "builtin.json");
    const requiredPaths = [manifestPath, builtinMetadataPath];
    if (entry.name === "awaken-outlook") {
      requiredPaths.push(join(pluginRoot, ".awaken", "setup.json"));
    }

    for (const path of requiredPaths) {
      const label = posixRelative(repoRoot, path);
      if (!existsSync(path) || !lstatSync(path).isFile()) {
        errors.push(`${label} is missing or not a regular file`);
      }
    }
    if (requiredPaths.some((path) => !existsSync(path) || !lstatSync(path).isFile())) {
      continue;
    }

    const manifest = parseJson(
      manifestPath,
      posixRelative(repoRoot, manifestPath),
      errors,
    );
    const builtinMetadata = parseJson(
      builtinMetadataPath,
      posixRelative(repoRoot, builtinMetadataPath),
      errors,
    );
    if (manifest && manifest.name !== entry.name) {
      errors.push(
        `${posixRelative(repoRoot, manifestPath)} name must match directory ${entry.name}`,
      );
    }
    if (
      builtinMetadata &&
      (builtinMetadata.version !== 1 ||
        typeof builtinMetadata.displayName !== "string" ||
        builtinMetadata.displayName.trim() === "" ||
        !Number.isInteger(builtinMetadata.order))
    ) {
      errors.push(
        `${posixRelative(repoRoot, builtinMetadataPath)} must declare version 1, displayName, and integer order`,
      );
    }
    if (entry.name === "awaken-outlook") {
      const setupPath = join(pluginRoot, ".awaken", "setup.json");
      parseJson(setupPath, posixRelative(repoRoot, setupPath), errors);
    }

    if (tracked) {
      for (const path of regularFiles(pluginRoot)) {
        const label = posixRelative(repoRoot, path);
        if (!tracked.has(label)) {
          errors.push(`${label} exists locally but is not tracked by Git`);
        }
      }
    }
  }

  return errors;
}

const isMain = process.argv[1] && resolve(process.argv[1]) === scriptPath;
if (isMain) {
  const requireTracked = process.argv.includes("--require-tracked");
  const errors = validateBuiltinPlugins({ requireTracked });
  if (errors.length > 0) {
    for (const error of errors) {
      console.error(`[validate-builtin-plugins] ${error}`);
    }
    process.exit(1);
  }
  console.log(
    `[validate-builtin-plugins] built-in plugin resources are complete${
      requireTracked ? " and tracked" : ""
    }`,
  );
}
