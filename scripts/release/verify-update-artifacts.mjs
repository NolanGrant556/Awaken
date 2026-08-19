#!/usr/bin/env node

// Release gate: validate electron-builder update feeds (`latest-mac.yml`,
// `latest.yml`) and the on-disk artifact set for one release version.
//
//   node scripts/release/verify-update-artifacts.mjs \
//     --version X.Y.Z --dir <artifact dir> [--receipt <out.json>]
//
// Fails closed on: version mismatch, 0.0.0 / non-SemVer versions, missing or
// size/SHA-512-mismatched files, duplicate names inside a feed, unexpected
// extra installer assets in the directory, or a feed that does not cover the
// required platform/arch set. On success it emits a machine-readable JSON
// receipt (per-asset size/SHA-512/SHA-256) for CI retention.

import { createHash } from "node:crypto";
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml } from "yaml";

const LOG_PREFIX = "[verify-update-artifacts]";

export const SEMVER_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;

const MAC_FEED = "latest-mac.yml";
const WIN_FEED = "latest.yml";
const LOCAL_BUILD_DIAGNOSTICS = new Set(["builder-debug.yml", "builder-effective-config.yaml"]);

function log(message) {
  console.log(`${LOG_PREFIX} ${message}`);
}

export function isValidReleaseVersion(version) {
  return SEMVER_PATTERN.test(version) && version !== "0.0.0";
}

export function assertReleaseVersion(version) {
  if (!SEMVER_PATTERN.test(version)) {
    throw new Error(`version "${version}" is not strict SemVer X.Y.Z`);
  }
  if (version === "0.0.0") {
    throw new Error("version 0.0.0 is the pre-update placeholder, not a release");
  }
}

export function computeSha512Base64(filePath) {
  return createHash("sha512").update(readFileSync(filePath)).digest("base64");
}

export function computeSha256Hex(filePath) {
  return createHash("sha256").update(readFileSync(filePath)).digest("hex");
}

export function parseFeed(feedPath) {
  let document;
  try {
    document = parseYaml(readFileSync(feedPath, "utf8"));
  } catch (error) {
    throw new Error(`cannot parse ${basename(feedPath)}: ${error.message}`);
  }
  if (!document || typeof document !== "object" || Array.isArray(document)) {
    throw new Error(`${basename(feedPath)} is not a YAML mapping`);
  }
  if (typeof document.version !== "string" || document.version.length === 0) {
    throw new Error(`${basename(feedPath)} has no top-level version`);
  }
  if (!Array.isArray(document.files) || document.files.length === 0) {
    throw new Error(`${basename(feedPath)} has no files[] entries`);
  }
  for (const entry of document.files) {
    if (!entry || typeof entry.url !== "string" || typeof entry.sha512 !== "string") {
      throw new Error(`${basename(feedPath)} files[] entries must carry url and sha512`);
    }
  }
  const names = document.files.map((entry) => entry.url);
  if (new Set(names).size !== names.length) {
    throw new Error(`${basename(feedPath)} files[] contains duplicate names`);
  }
  return document;
}

export function expectedMacZipNames(version) {
  return [`Awaken-${version}-arm64.zip`, `Awaken-${version}-x64.zip`];
}

export function expectedMacDmgNames(version) {
  return [`Awaken-${version}-arm64.dmg`, `Awaken-${version}-x64.dmg`];
}

export function expectedWinExeNames(version) {
  return [`Awaken-Setup-${version}-x64.exe`];
}

function verifyFeedEntries(feed, feedName, dir) {
  if (feed.version === undefined) {
    throw new Error(`${feedName} has no version`);
  }
  const assets = [];
  for (const entry of feed.files) {
    const filePath = join(dir, entry.url);
    if (!existsSync(filePath)) {
      throw new Error(`${feedName} references missing file ${entry.url}`);
    }
    const size = statSync(filePath).size;
    if (typeof entry.size === "number" && entry.size !== size) {
      throw new Error(
        `${entry.url} size mismatch: feed says ${entry.size}, disk has ${size}`,
      );
    }
    const sha512 = computeSha512Base64(filePath);
    if (sha512 !== entry.sha512) {
      throw new Error(`${entry.url} SHA-512 mismatch against ${feedName}`);
    }
    assets.push({
      name: entry.url,
      size,
      sha512,
      sha256: computeSha256Hex(filePath),
    });
  }
  return assets;
}

function assertExactNameSet(actual, expected, feedName) {
  const actualSorted = [...actual].sort();
  const expectedSorted = [...expected].sort();
  if (actualSorted.join("\n") !== expectedSorted.join("\n")) {
    throw new Error(
      `${feedName} must reference exactly ${expectedSorted.join(", ")}; got ${actualSorted.join(", ")}`,
    );
  }
}

// Verifies the artifact directory and returns a receipt object. `feeds` may
// limit which feed files are required; by default every feed present on disk
// is verified, and at least one feed must exist.
export function verifyUpdateArtifacts({ dir, version }) {
  assertReleaseVersion(version);
  const resolvedDir = resolve(dir);
  if (!existsSync(resolvedDir)) {
    throw new Error(`artifact directory does not exist: ${resolvedDir}`);
  }

  const hasMacFeed = existsSync(join(resolvedDir, MAC_FEED));
  const hasWinFeed = existsSync(join(resolvedDir, WIN_FEED));
  if (!hasMacFeed && !hasWinFeed) {
    throw new Error(`no update feed (${MAC_FEED} / ${WIN_FEED}) in ${resolvedDir}`);
  }

  const assets = [];
  const allowed = new Set();

  if (hasMacFeed) {
    const feed = parseFeed(join(resolvedDir, MAC_FEED));
    if (feed.version !== version) {
      throw new Error(
        `${MAC_FEED} version ${feed.version} does not match expected ${version}`,
      );
    }
    assertExactNameSet(
      feed.files.map((entry) => entry.url),
      expectedMacZipNames(version),
      MAC_FEED,
    );
    assets.push(...verifyFeedEntries(feed, MAC_FEED, resolvedDir));
    allowed.add(MAC_FEED);
    for (const name of [...expectedMacZipNames(version), ...expectedMacDmgNames(version)]) {
      allowed.add(name);
      allowed.add(`${name}.blockmap`);
    }
  }

  if (hasWinFeed) {
    const feed = parseFeed(join(resolvedDir, WIN_FEED));
    if (feed.version !== version) {
      throw new Error(
        `${WIN_FEED} version ${feed.version} does not match expected ${version}`,
      );
    }
    assertExactNameSet(
      feed.files.map((entry) => entry.url),
      expectedWinExeNames(version),
      WIN_FEED,
    );
    assets.push(...verifyFeedEntries(feed, WIN_FEED, resolvedDir));
    allowed.add(WIN_FEED);
    for (const name of expectedWinExeNames(version)) {
      allowed.add(name);
      allowed.add(`${name}.blockmap`);
    }
  }

  for (const fileName of readdirSync(resolvedDir)) {
    if (!statSync(join(resolvedDir, fileName)).isFile()) {
      continue;
    }
    if (LOCAL_BUILD_DIAGNOSTICS.has(fileName)) {
      continue;
    }
    if (!allowed.has(fileName)) {
      throw new Error(`unexpected installer asset in directory: ${fileName}`);
    }
  }

  return {
    version,
    dir: resolvedDir,
    feeds: [hasMacFeed ? MAC_FEED : null, hasWinFeed ? WIN_FEED : null].filter(Boolean),
    assets,
    checkedAt: new Date().toISOString(),
  };
}

export function parseCliArgs(argv) {
  const args = { version: undefined, dir: undefined, receipt: undefined };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    if (flag === "--version" || flag === "--dir" || flag === "--receipt") {
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
  if (!args.version || !args.dir) {
    throw new Error("usage: verify-update-artifacts.mjs --version X.Y.Z --dir <dir> [--receipt <out.json>]");
  }
  return args;
}

if (resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  try {
    const args = parseCliArgs(process.argv.slice(2));
    const receipt = verifyUpdateArtifacts({ dir: args.dir, version: args.version });
    if (args.receipt) {
      writeFileSync(args.receipt, `${JSON.stringify(receipt, null, 2)}\n`);
      log(`receipt written: ${args.receipt}`);
    }
    log(
      `verified ${receipt.assets.length} asset(s) for v${receipt.version} via ${receipt.feeds.join(" + ")}`,
    );
  } catch (error) {
    console.error(
      `${LOG_PREFIX} ${error instanceof Error ? error.message : String(error)}`,
    );
    process.exit(1);
  }
}
