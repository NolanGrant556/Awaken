#!/usr/bin/env node

// Release gate (design contract §8.2): audit the FINAL installer artifacts,
// not source directories. Unpacks macOS ZIP/DMG and Windows NSIS artifacts,
// extracts app.asar, builds a path/size/SHA-256 inventory, enforces the
// denied-content rules, scans text files for known secret patterns and
// compares the same-architecture DMG/ZIP `Awaken.app` inventories.
//
//   node scripts/release/audit-package-contents.mjs --dir <artifact dir> [--receipt <out.json>]
//
// Violations fail closed (exit 1). Scan reports contain only the file path
// and the rule id — never the matched secret text.
//
// Platform support: .zip via `unzip` (or `7z`), .dmg via `hdiutil` on macOS
// only (skipped with a clear message elsewhere), .exe via `7z` (available on
// CI Windows runners; skipped with a clear message when missing locally).

import { extractAll } from "@electron/asar";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const LOG_PREFIX = "[audit-package-contents]";

// Content scan is bounded so multi-hundred-MB binaries are skipped by the
// text check (they contain NUL bytes and fail isTextContent anyway).
const MAX_TEXT_SCAN_BYTES = 8 * 1024 * 1024;

function log(message) {
  console.log(`${LOG_PREFIX} ${message}`);
}

function sha256Hex(filePath) {
  return createHash("sha256").update(readFileSync(filePath)).digest("hex");
}

// --- inventory -------------------------------------------------------------

// Sorted [{path, size, sha256}] for every regular file under rootDir.
// Paths are relative, POSIX-separated, so inventories are comparable across
// unpack locations.
export function buildInventory(rootDir) {
  const entries = [];
  const walk = (current) => {
    for (const name of readdirSync(current).sort()) {
      const fullPath = join(current, name);
      const stat = lstatSync(fullPath);
      if (stat.isDirectory()) {
        walk(fullPath);
      } else if (stat.isFile()) {
        entries.push({
          path: relative(rootDir, fullPath).split(sep).join("/"),
          size: stat.size,
          sha256: sha256Hex(fullPath),
        });
      }
    }
  };
  walk(rootDir);
  entries.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  return entries;
}

// Path-level diff between two inventories: proves (or disproves) that the
// same-arch DMG and ZIP carry the same Awaken.app.
export function compareInventories(a, b) {
  const byPathA = new Map(a.map((entry) => [entry.path, entry]));
  const byPathB = new Map(b.map((entry) => [entry.path, entry]));
  const onlyInA = [];
  const onlyInB = [];
  const changed = [];
  for (const [path, entryA] of byPathA) {
    const entryB = byPathB.get(path);
    if (!entryB) {
      onlyInA.push(path);
    } else if (entryA.size !== entryB.size || entryA.sha256 !== entryB.sha256) {
      changed.push(path);
    }
  }
  for (const path of byPathB.keys()) {
    if (!byPathA.has(path)) {
      onlyInB.push(path);
    }
  }
  return { onlyInA, onlyInB, changed };
}

export function inventoriesEqual(diff) {
  return (
    diff.onlyInA.length === 0 &&
    diff.onlyInB.length === 0 &&
    diff.changed.length === 0
  );
}

// --- denied-content rules --------------------------------------------------

// Each rule: { id, test(relativePath) }. Matched rules report only
// { path, rule } — never file content.
export const PATH_RULES = [
  { id: "source-map", test: (p) => p.endsWith(".map") },
  { id: "test-file", test: (p) => /\.(test|spec)\.[^/]+$/.test(p) },
  { id: "test-dir", test: (p) => p.split("/").includes("__tests__") },
  { id: "fixture-dir", test: (p) => p.split("/").includes("fixtures") },
  {
    id: "env-file",
    test: (p) => {
      const name = p.split("/").pop();
      return name === ".env" || name.startsWith(".env.");
    },
  },
  { id: "git-metadata", test: (p) => p.split("/").includes(".git") },
  {
    id: "signing-material",
    test: (p) => /\.(p8|p12|pfx|pem|key|mobileprovision|keychain)$/i.test(p),
  },
];

// Known secret formats. Applied to text file contents; the matched text is
// never reported.
export const SECRET_RULES = [
  { id: "github-token", pattern: /ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}/ },
  // `task-…` class names and GFM anchors contain the letters `sk-`. A key is a
  // separate token, so the `sk-` must not continue an identifier.
  { id: "openai-api-key", pattern: /(?<![A-Za-z0-9])sk-(?!ant-)(?:proj-)?[A-Za-z0-9_-]{20,}/ },
  { id: "anthropic-api-key", pattern: /sk-ant-[A-Za-z0-9_-]{20,}/ },
  { id: "aws-access-key", pattern: /AKIA[0-9A-Z]{16}/ },
  // A quoted header or a short placeholder is not key material. Require a
  // base64 body; the checked fixture body is 18 characters.
  { id: "private-key", pattern: /-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----(?:\\n|\s)*[A-Za-z0-9+/=]{16,}/ },
];

export function isTextContent(buffer) {
  const sample = buffer.subarray(0, Math.min(buffer.length, 8192));
  return !sample.includes(0);
}

// Audits one inventory against the denied rules. `readEntry(entry)` must
// return a Buffer for content rules; entries larger than the scan bound or
// containing NUL bytes skip content rules (path rules always apply).
export function auditInventory(entries, readEntry) {
  const violations = [];
  for (const entry of entries) {
    for (const rule of PATH_RULES) {
      if (rule.test(entry.path)) {
        violations.push({ path: entry.path, rule: rule.id });
      }
    }
    if (!readEntry || entry.size > MAX_TEXT_SCAN_BYTES) {
      continue;
    }
    const content = readEntry(entry);
    if (!isTextContent(content)) {
      continue;
    }
    const text = content.toString("utf8");
    for (const rule of SECRET_RULES) {
      if (rule.pattern.test(text)) {
        violations.push({ path: entry.path, rule: rule.id });
      }
    }
  }
  return violations;
}

// --- asar ------------------------------------------------------------------

export function extractAsar(asarPath, outDir) {
  extractAll(asarPath, outDir);
  return outDir;
}

// --- artifact unpacking (CLI mode) -----------------------------------------

function commandExists(command) {
  const probe = spawnSync(
    process.platform === "win32" ? "where" : "which",
    [command],
    { encoding: "utf8" },
  );
  return probe.status === 0;
}

function run(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.error || result.status !== 0) {
    throw new Error(
      `${command} ${args.join(" ")} failed: ${(result.stderr || result.error?.message || "").trim()}`,
    );
  }
  return `${result.stdout ?? ""}${result.stderr ?? ""}`;
}

function findFiles(rootDir, predicate) {
  const matches = [];
  const walk = (current) => {
    for (const name of readdirSync(current)) {
      const fullPath = join(current, name);
      const stat = lstatSync(fullPath);
      if (stat.isDirectory()) {
        walk(fullPath);
      } else if (stat.isFile() && predicate(name, fullPath)) {
        matches.push(fullPath);
      }
    }
  };
  walk(rootDir);
  return matches.sort();
}

// Unpacks one artifact into outDir. Returns { unpacked, skipped, appRoot }.
export function unpackArtifact(artifactPath, outDir, platform = process.platform) {
  if (artifactPath.endsWith(".zip")) {
    if (commandExists("unzip")) {
      run("unzip", ["-q", artifactPath, "-d", outDir]);
    } else if (commandExists("7z")) {
      run("7z", ["x", artifactPath, `-o${outDir}`, "-y"]);
    } else {
      return { unpacked: false, skipped: "neither unzip nor 7z available" };
    }
    return { unpacked: true, skipped: null };
  }
  if (artifactPath.endsWith(".dmg")) {
    if (platform !== "darwin") {
      return { unpacked: false, skipped: "DMG audit requires macOS (hdiutil)" };
    }
    const mountPoint = mkdtempSync(join(tmpdir(), "awaken-dmg-mount-"));
    try {
      run("hdiutil", [
        "attach",
        artifactPath,
        "-nobrowse",
        "-readonly",
        "-mountpoint",
        mountPoint,
      ]);
      cpSync(mountPoint, outDir, { recursive: true });
    } finally {
      spawnSync("hdiutil", ["detach", mountPoint, "-quiet"], { encoding: "utf8" });
      rmSync(mountPoint, { recursive: true, force: true });
    }
    return { unpacked: true, skipped: null };
  }
  if (artifactPath.endsWith(".exe")) {
    if (!commandExists("7z")) {
      return { unpacked: false, skipped: "NSIS audit requires 7z (present on CI runners)" };
    }
    run("7z", ["x", artifactPath, `-o${outDir}`, "-y"]);
    // electron-builder NSIS embeds the installed application as app-*.7z.
    // Expanding only the outer EXE would scan the compressed blob but miss
    // app.asar and the actual extraResources, so recurse into that payload.
    const appArchives = findFiles(outDir, (name) => /^app-[^.]+\.7z$/i.test(name));
    if (appArchives.length !== 1) {
      throw new Error(
        `expected exactly one electron-builder NSIS app-*.7z payload, found ${appArchives.length}`,
      );
    }
    run("7z", ["x", appArchives[0], `-o${join(outDir, "nsis-app")}`, "-y"]);
    return { unpacked: true, skipped: null };
  }
  return { unpacked: false, skipped: `unsupported artifact type: ${basename(artifactPath)}` };
}

export function findAppBundle(rootDir) {
  // Returns the Awaken.app directory inside an unpacked mac artifact.
  const direct = join(rootDir, "Awaken.app");
  if (existsSync(direct)) {
    return direct;
  }
  for (const name of readdirSync(rootDir)) {
    if (name.endsWith(".app") && lstatSync(join(rootDir, name)).isDirectory()) {
      return join(rootDir, name);
    }
  }
  return null;
}

function findAsar(rootDir) {
  const candidates = findFiles(rootDir, (name) => name === "app.asar");
  if (candidates.length > 1) {
    throw new Error(`expected at most one app.asar, found ${candidates.length}`);
  }
  return candidates[0] ?? null;
}

// Audits one unpacked artifact tree: outer tree + extracted app.asar.
export function auditUnpackedTree(rootDir, workDir) {
  const violations = [];
  const outerInventory = buildInventory(rootDir);
  violations.push(
    ...auditInventory(outerInventory, (entry) =>
      readFileSync(join(rootDir, ...entry.path.split("/"))),
    ),
  );

  let asarInventory = null;
  const asarPath = findAsar(rootDir);
  if (asarPath) {
    const asarOut = join(workDir, "app.asar.extracted");
    extractAsar(asarPath, asarOut);
    asarInventory = buildInventory(asarOut);
    violations.push(
      ...auditInventory(asarInventory, (entry) =>
        readFileSync(join(asarOut, ...entry.path.split("/"))),
      ).map((violation) => ({
        ...violation,
        path: `app.asar/${violation.path}`,
      })),
    );
  }
  return { violations, outerInventory, asarInventory };
}

function artifactArch(fileName) {
  const match = fileName.match(/-(arm64|x64)\.(dmg|zip|exe)$/);
  return match ? match[1] : null;
}

// CLI: audit every installer artifact in a directory.
export function auditArtifactDir(dir) {
  const resolvedDir = resolve(dir);
  const artifacts = readdirSync(resolvedDir)
    .filter((name) => /\.(dmg|zip|exe)$/.test(name))
    .sort();
  if (artifacts.length === 0) {
    throw new Error(`no .dmg/.zip/.exe artifacts in ${resolvedDir}`);
  }

  const report = { dir: resolvedDir, artifacts: [], violations: [], comparisons: [], skipped: [] };
  const appInventories = new Map(); // `${arch}` -> Map(kind -> {path, inventory})

  for (const name of artifacts) {
    const artifactPath = join(resolvedDir, name);
    const workDir = mkdtempSync(join(tmpdir(), "awaken-audit-"));
    try {
      const unpackRoot = join(workDir, "unpacked");
      const { unpacked, skipped } = unpackArtifact(artifactPath, unpackRoot);
      if (!unpacked) {
        log(`SKIP ${name}: ${skipped}`);
        report.skipped.push({ artifact: name, reason: skipped });
        continue;
      }
      const { violations } = auditUnpackedTree(unpackRoot, workDir);
      for (const violation of violations) {
        report.violations.push({ artifact: name, ...violation });
      }

      const arch = artifactArch(name);
      const kind = name.endsWith(".dmg") ? "dmg" : name.endsWith(".zip") ? "zip" : "exe";
      const appBundle = findAppBundle(unpackRoot);
      if (arch && appBundle && (kind === "dmg" || kind === "zip")) {
        if (!appInventories.has(arch)) {
          appInventories.set(arch, new Map());
        }
        appInventories.get(arch).set(kind, buildInventory(appBundle));
      }
      report.artifacts.push(name);
    } finally {
      rmSync(workDir, { recursive: true, force: true });
    }
  }

  // Same-arch DMG/ZIP must carry the identical Awaken.app.
  for (const [arch, byKind] of appInventories) {
    if (byKind.has("dmg") && byKind.has("zip")) {
      const diff = compareInventories(byKind.get("dmg"), byKind.get("zip"));
      report.comparisons.push({ arch, equal: inventoriesEqual(diff), diff });
      if (!inventoriesEqual(diff)) {
        report.violations.push({
          artifact: `Awaken.app (${arch})`,
          path: "Awaken.app",
          rule: "dmg-zip-content-drift",
        });
      }
    }
  }
  return report;
}

// CI passes a receipt under smoke-evidence/, which does not exist until the
// first evidence file is written. Create the parent so a finished audit is
// not reported as ENOENT.
export function writeReceipt(receiptPath, report) {
  const target = resolve(receiptPath);
  mkdirSync(dirname(target), { recursive: true });
  writeFileSync(target, `${JSON.stringify(report, null, 2)}\n`);
}

export function parseCliArgs(argv) {
  const args = { dir: undefined, receipt: undefined };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    if (flag === "--dir" || flag === "--receipt") {
      const value = argv[index + 1];
      if (!value || value.startsWith("--")) {
        throw new Error(`flag ${flag} requires a value`);
      }
      args[flag.slice(2)] = value;
      index += 1;
    } else {
      throw new Error(`unknown argument: ${flag}`);
    }
  }
  if (!args.dir) {
    throw new Error("usage: audit-package-contents.mjs --dir <artifact dir> [--receipt <out.json>]");
  }
  return args;
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  let report;
  try {
    const args = parseCliArgs(process.argv.slice(2));
    report = auditArtifactDir(args.dir);
    if (args.receipt) {
      writeReceipt(args.receipt, report);
      log(`receipt written: ${args.receipt}`);
    }
  } catch (error) {
    console.error(
      `${LOG_PREFIX} ${error instanceof Error ? error.message : String(error)}`,
    );
    process.exit(1);
  }
  for (const violation of report.violations) {
    console.error(
      `${LOG_PREFIX} VIOLATION ${violation.rule}: ${violation.artifact} :: ${violation.path}`,
    );
  }
  if (report.violations.length > 0) {
    console.error(`${LOG_PREFIX} ${report.violations.length} violation(s); failing closed`);
    process.exit(1);
  }
  if (report.skipped.length > 0) {
    console.error(`${LOG_PREFIX} ${report.skipped.length} artifact(s) were not audited; failing closed`);
    process.exit(1);
  }
  log(
    `audit passed: ${report.artifacts.length} artifact(s), ${report.skipped.length} skipped, ${report.comparisons.length} DMG/ZIP comparison(s)`,
  );
}
