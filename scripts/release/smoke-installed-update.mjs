#!/usr/bin/env node
// Installed-app upgrade smoke for a stable Desktop build.
//
// The candidate bundle is installed, then that copy is rewritten to 0.0.1 and
// a 127.0.0.1 generic feed. The feed serves the unmodified candidate metadata
// and payload. The app must discover, download, defer, quitAndInstall, and
// boot the candidate version. A missing marker fails the process; nothing here
// prints a success line in place of those events.

import { spawn, spawnSync } from "node:child_process";
import { createReadStream, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, statSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { createPackage, extractAll, extractFile, uncache } from "@electron/asar";
import { stringify } from "yaml";
import { isolatedLaunchEnvironment, READY_MARKER } from "../smoke-packaged-desktop.mjs";
import { computeSha512Base64, isValidReleaseVersion, parseFeed } from "./verify-update-artifacts.mjs";

export const PREVIOUS_SMOKE_VERSION = "0.0.1";
export const UPDATE_SMOKE_FLAG = "--awaken-update-smoke";
export const UPDATE_SMOKE_INSTALL_FLAG = "--awaken-update-smoke-install";
const RELEASES_OWNER = "NolanGrant556";
const RELEASES_REPO = "Awaken";
const CREDENTIAL_PATTERN = /ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|Bearer\s+\S+/;

export function assertTargetVersion(version) {
  if (!isValidReleaseVersion(version)) {
    throw new Error(`version "${version}" is not a stable SemVer release`);
  }
  const [major, minor, patch] = version.split(".").map(Number);
  const [previousMajor, previousMinor, previousPatch] = PREVIOUS_SMOKE_VERSION.split(".").map(Number);
  const newer =
    major > previousMajor ||
    (major === previousMajor && minor > previousMinor) ||
    (major === previousMajor && minor === previousMinor && patch > previousPatch);
  if (!newer) {
    throw new Error(
      `version ${version} is not newer than the smoke previous version ${PREVIOUS_SMOKE_VERSION}`,
    );
  }
}

export function assertProductionGithubFeed(text) {
  if (CREDENTIAL_PATTERN.test(text)) {
    throw new Error("app-update.yml contains a credential");
  }
  const document = parseFeedYaml(text);
  if (
    document.provider !== "github" ||
    document.owner !== RELEASES_OWNER ||
    document.repo !== RELEASES_REPO
  ) {
    throw new Error("app-update.yml is not the Awaken GitHub feed");
  }
  if (typeof document.url === "string") {
    throw new Error("production app-update.yml must not pin a feed URL");
  }
  const rawPublisher = document.publisherName;
  const publisherName =
    rawPublisher == null ? [] : Array.isArray(rawPublisher) ? rawPublisher : [rawPublisher];
  if (publisherName.some((name) => typeof name !== "string" || name.length === 0)) {
    throw new Error("app-update.yml publisherName is invalid");
  }
  return { publisherName };
}

function parseFeedYaml(text) {
  const lines = String(text).split(/\r?\n/);
  const document = {};
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const match = /^([A-Za-z0-9_]+):\s*(.*)$/.exec(line);
    if (!match || line.startsWith(" ")) continue;
    const key = match[1];
    const value = match[2].trim();
    if (value === "" || value === "|" || value === ">") {
      const items = [];
      while (index + 1 < lines.length && /^\s+-\s+/.test(lines[index + 1])) {
        index += 1;
        items.push(lines[index].replace(/^\s+-\s+/, "").replace(/^["']|["']$/g, ""));
      }
      document[key] = items.length > 0 ? items : "";
    } else {
      document[key] = value.replace(/^["']|["']$/g, "");
    }
  }
  return document;
}

export function buildLoopbackAppUpdateYaml({ url, publisherName }) {
  if (!/^http:\/\/127\.0\.0\.1:\d+\/$/.test(url)) {
    throw new Error("loopback feed URL must be http://127.0.0.1:<port>/");
  }
  const document = {
    provider: "generic",
    url,
    updaterCacheDirName: "awaken-desktop-updater",
  };
  if (publisherName.length > 0) document.publisherName = publisherName;
  return stringify(document, { lineWidth: 0 });
}

export function rewriteFeedForLoopback(document) {
  const files = document.files.map((entry) => ({
    ...entry,
    url: basename(String(entry.url).split("?")[0]),
  }));
  const next = { ...document, files };
  if (typeof document.path === "string") {
    next.path = basename(document.path.split("?")[0]);
  }
  return next;
}

export function hostUpdatePayload(version, platform, arch) {
  if (platform === "darwin" && (arch === "arm64" || arch === "x64")) {
    return {
      feedName: "latest-mac.yml",
      payloadName: `Awaken-${version}-${arch}.zip`,
    };
  }
  if (platform === "win32" && arch === "x64") {
    return {
      feedName: "latest.yml",
      payloadName: `Awaken-Setup-${version}-x64.exe`,
    };
  }
  throw new Error(`upgrade smoke does not support ${platform}-${arch}`);
}

export function windowsSilentInstallArgs(dest) {
  return ["/S", `/D=${dest}`];
}

export function resourcesDirFromApp(appRoot) {
  return appRoot.endsWith(".app")
    ? join(appRoot, "Contents", "Resources")
    : join(appRoot, "resources");
}

export function replacePlistVersion(xml, version) {
  const next = xml
    .replace(
      /(<key>CFBundleShortVersionString<\/key>\s*<string>)[^<]*(<\/string>)/,
      `$1${version}$2`,
    )
    .replace(/(<key>CFBundleVersion<\/key>\s*<string>)[^<]*(<\/string>)/, `$1${version}$2`);
  for (const key of ["CFBundleShortVersionString", "CFBundleVersion"]) {
    const match = next.match(new RegExp(`<key>${key}</key>\\s*<string>([^<]*)</string>`));
    if (match?.[1] !== version) {
      throw new Error(`Info.plist ${key} was not set to ${version}`);
    }
  }
  return next;
}

export function markerIndex(output, marker) {
  const index = output.indexOf(marker);
  return index === -1 ? null : index;
}

export function assertUpgradeTranscript(output, fromVersion, toVersion) {
  if (output.includes("desktop_update_smoke_failed")) {
    throw new Error(`upgrade smoke failed inside the app\n${output.slice(-4000)}`);
  }
  const versionAt = markerIndex(output, `desktop_update_smoke_version ${fromVersion}`);
  const availableAt = markerIndex(output, `desktop_update_smoke_available ${toVersion}`);
  const deferredAt = markerIndex(output, `desktop_update_smoke_deferred ${toVersion}`);
  const installMarkers = [
    markerIndex(output, `desktop_update_smoke_installing ${toVersion}`),
    markerIndex(output, "desktop_update_install_launching"),
  ].filter((index) => index != null);
  const installingAt = installMarkers.length > 0 ? Math.min(...installMarkers) : null;
  if (
    versionAt == null ||
    availableAt == null ||
    deferredAt == null ||
    installingAt == null ||
    !(versionAt < availableAt && availableAt < deferredAt && deferredAt < installingAt)
  ) {
    throw new Error(
      "upgrade transcript is missing discover -> download -> defer -> install\n" +
        output.slice(-4000),
    );
  }
}

export function assertBootTranscript(output, version) {
  if (output.includes("desktop_update_smoke_installing") || output.includes("desktop_update_smoke_failed")) {
    throw new Error(`booted app tried to install again\n${output.slice(-2000)}`);
  }
  if (!output.includes(`desktop_update_smoke_version ${version}`)) {
    throw new Error(`booted app did not report ${version}\n${output.slice(-2000)}`);
  }
  if (!output.includes(READY_MARKER)) {
    throw new Error(`booted app did not create the main window\n${output.slice(-2000)}`);
  }
}

export function redact(text, secrets) {
  let next = String(text ?? "");
  for (const secret of secrets) {
    if (typeof secret === "string" && secret.length >= 4) {
      next = next.split(secret).join("***");
    }
  }
  return next;
}

export function materializeP12(link, dest, { exists = existsSync, writeFile = writeFileSync } = {}) {
  if (!link) throw new Error("CSC_LINK is required to re-sign the upgrade smoke app");
  if (link.startsWith("https://") || link.startsWith("http://")) {
    throw new Error("CSC_LINK must be a p12 file path or base64, not a URL");
  }
  if (exists(link)) return link;
  const body = link.replace(/\s+/g, "");
  if (!/^[A-Za-z0-9+/=]+$/.test(body) || body.length < 64) {
    throw new Error("CSC_LINK is neither a p12 file path nor p12 base64");
  }
  const decoded = Buffer.from(body, "base64");
  if (decoded.length < 32) throw new Error("CSC_LINK base64 did not decode to a certificate");
  writeFile(dest, decoded);
  return dest;
}

export function signDarwinApp(appPath, env, runCommand, paths) {
  const identity = env.AWAKEN_CODESIGN_IDENTITY?.trim() ?? "";
  if (!identity) {
    throw new Error("AWAKEN_CODESIGN_IDENTITY is required to sign the upgrade smoke app");
  }
  const secrets = [env.CSC_KEY_PASSWORD, env.CSC_LINK, paths.keychainPassword];
  const run = (command, args) => {
    try {
      return runCommand(command, args);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      throw new Error(redact(message, secrets));
    }
  };
  if (identity !== "-") {
    const password = env.CSC_KEY_PASSWORD ?? "";
    if (!password) throw new Error("CSC_KEY_PASSWORD is required");
    const p12 = materializeP12(env.CSC_LINK, paths.p12Path, paths);
    run("security", ["create-keychain", "-p", paths.keychainPassword, paths.keychainPath]);
    run("security", ["set-keychain-settings", "-lut", "21600", paths.keychainPath]);
    run("security", ["unlock-keychain", "-p", paths.keychainPassword, paths.keychainPath]);
    run("security", ["import", p12, "-k", paths.keychainPath, "-P", password, "-T", "/usr/bin/codesign"]);
    run("security", [
      "set-key-partition-list",
      "-S",
      "apple-tool:,apple:,codesign:",
      "-s",
      "-k",
      paths.keychainPassword,
      paths.keychainPath,
    ]);
    run("security", ["list-keychains", "-d", "user", "-s", paths.keychainPath]);
    run("security", ["default-keychain", "-s", paths.keychainPath]);
  }
  run("codesign", [
    "--force",
    "--sign",
    identity,
    "--preserve-metadata=entitlements,flags,runtime",
    appPath,
  ]);
  run("codesign", ["--verify", "--deep", "--strict", appPath]);
}

export async function rewriteInstalledApp({ appRoot, version, feedYaml }) {
  const resources = resourcesDirFromApp(appRoot);
  const asarPath = join(resources, "app.asar");
  if (!existsSync(asarPath)) throw new Error(`app.asar is missing under ${appRoot}`);
  const staging = mkdtempSync(join(tmpdir(), "awaken-update-smoke-"));
  try {
    extractAll(asarPath, staging);
    const packagePath = join(staging, "package.json");
    const pkg = JSON.parse(readFileSync(packagePath, "utf8"));
    pkg.version = version;
    writeFileSync(packagePath, `${JSON.stringify(pkg, null, 2)}\n`);
    const nextAsar = `${asarPath}.next`;
    await createPackage(staging, nextAsar);
    rmSync(asarPath, { force: true });
    renameSync(nextAsar, asarPath);
    uncache(asarPath);
  } finally {
    rmSync(staging, { recursive: true, force: true });
  }
  writeFileSync(join(resources, "app-update.yml"), feedYaml);
  const plistPath = join(appRoot, "Contents", "Info.plist");
  if (!existsSync(plistPath)) return;
  const original = readFileSync(plistPath);
  if (original.includes("<key>CFBundleShortVersionString</key>")) {
    writeFileSync(plistPath, replacePlistVersion(original.toString("utf8"), version));
    return;
  }
  const converted = spawnSync("plutil", ["-convert", "xml1", "-o", "-", plistPath], {
    encoding: "utf8",
  });
  if (converted.status !== 0) {
    throw new Error(`plutil failed to read Info.plist: ${converted.stderr || converted.stdout}`);
  }
  const xmlPath = `${plistPath}.xml`;
  writeFileSync(xmlPath, replacePlistVersion(converted.stdout, version));
  const restored = spawnSync("plutil", ["-convert", "binary1", "-o", plistPath, xmlPath], {
    encoding: "utf8",
  });
  rmSync(xmlPath, { force: true });
  if (restored.status !== 0) {
    throw new Error(`plutil failed to write Info.plist: ${restored.stderr || restored.stdout}`);
  }
}

export function startLoopbackFeed(files) {
  const server = createServer((request, response) => {
    let name = "";
    try {
      const url = new URL(request.url ?? "/", "http://127.0.0.1");
      name = basename(decodeURIComponent(url.pathname));
    } catch {
      response.writeHead(400);
      response.end();
      return;
    }
    const item = files.get(name);
    if (!item) {
      response.writeHead(404);
      response.end();
      return;
    }
    if (item.text != null) {
      const body = Buffer.from(item.text);
      response.writeHead(200, {
        "Content-Length": body.length,
        "Content-Type": "text/yaml; charset=utf-8",
      });
      response.end(body);
      return;
    }
    const size = statSync(item.path).size;
    const range = request.headers.range;
    if (range) {
      const match = /^bytes=(\d+)-(\d*)$/.exec(range);
      if (!match) {
        response.writeHead(416);
        response.end();
        return;
      }
      const start = Number(match[1]);
      const end = match[2] ? Number(match[2]) : size - 1;
      if (start < 0 || end < start || end >= size) {
        response.writeHead(416);
        response.end();
        return;
      }
      response.writeHead(206, {
        "Accept-Ranges": "bytes",
        "Content-Range": `bytes ${start}-${end}/${size}`,
        "Content-Length": end - start + 1,
      });
      createReadStream(item.path, { start, end }).pipe(response);
      return;
    }
    response.writeHead(200, {
      "Accept-Ranges": "bytes",
      "Content-Length": size,
    });
    createReadStream(item.path).pipe(response);
  });
  return new Promise((resolveFeed, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("loopback feed did not bind to a TCP port"));
        return;
      }
      resolveFeed({
        url: `http://127.0.0.1:${address.port}/`,
        close: () =>
          new Promise((done) => {
            server.close(() => done());
          }),
      });
    });
  });
}

export function prepareFeedFiles(sourceDir, feedName, version) {
  const feedPath = join(sourceDir, feedName);
  const document = parseFeed(feedPath);
  if (document.version !== version) {
    throw new Error(`${feedName} version is ${document.version}, expected ${version}`);
  }
  const rewritten = rewriteFeedForLoopback(document);
  const files = new Map();
  files.set(feedName, { text: stringify(rewritten, { lineWidth: 0 }) });
  for (const entry of rewritten.files) {
    const path = join(sourceDir, entry.url);
    if (!existsSync(path)) throw new Error(`feed file missing: ${entry.url}`);
    const actual = computeSha512Base64(path);
    if (actual !== entry.sha512) {
      throw new Error(`sha512 mismatch for ${entry.url}`);
    }
    files.set(entry.url, { path });
  }
  return files;
}

function smokeChildEnvironment(root, baseEnv) {
  const env = isolatedLaunchEnvironment(root, baseEnv);
  for (const key of [
    "GH_TOKEN",
    "GITHUB_TOKEN",
    "CSC_LINK",
    "CSC_KEY_PASSWORD",
    "WIN_CSC_LINK",
    "WIN_CSC_KEY_PASSWORD",
    "APPLE_API_KEY",
    "APPLE_API_KEY_P8_BASE64",
  ]) {
    delete env[key];
  }
  return env;
}

function signalProcessTree(child) {
  if (!child.pid || child.exitCode != null || child.signalCode != null) return;
  if (process.platform === "win32") {
    spawnSync("taskkill", ["/PID", String(child.pid), "/T", "/F"], {
      windowsHide: true,
      stdio: "ignore",
    });
    return;
  }
  try {
    process.kill(-child.pid, "SIGKILL");
  } catch {
    try {
      child.kill("SIGKILL");
    } catch {
      // The process already exited.
    }
  }
}

export function launchPackagedApp({ executable, args, env, timeoutMs, stopWhen }) {
  return new Promise((resolveLaunch, reject) => {
    const child = spawn(executable, args, {
      env,
      detached: process.platform !== "win32",
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: false,
    });
    let output = "";
    let settled = false;
    const finish = (value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolveLaunch(value);
    };
    const timer = setTimeout(() => {
      signalProcessTree(child);
      settled = true;
      reject(new Error(`packaged app timed out\n${output.slice(-4000)}`));
    }, timeoutMs);
    const onData = (chunk) => {
      output += chunk.toString("utf8");
      if (stopWhen?.(output)) {
        signalProcessTree(child);
        finish({ output, exitCode: null });
      }
    };
    child.stdout?.on("data", onData);
    child.stderr?.on("data", onData);
    child.once("error", (error) => {
      clearTimeout(timer);
      if (!settled) {
        settled = true;
        reject(error);
      }
    });
    child.once("exit", (code) => {
      finish({ output, exitCode: code });
    });
  });
}

async function waitForInstalledVersion(readInstalledVersion, expected, sleep, attempts) {
  let last = "";
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      last = readInstalledVersion();
    } catch (error) {
      last = error instanceof Error ? error.message : String(error);
    }
    if (last === expected) return;
    await sleep(1_000);
  }
  throw new Error(`installed version stayed ${last}, expected ${expected}`);
}

export async function executeInstalledUpdateSmoke({
  targetVersion,
  productionFeedText,
  files,
  patchApp,
  signApp,
  launch,
  readInstalledVersion,
  sleep = delay,
  versionAttempts = 90,
}) {
  assertTargetVersion(targetVersion);
  const production = assertProductionGithubFeed(productionFeedText);
  const feed = await startLoopbackFeed(files);
  try {
    const feedYaml = buildLoopbackAppUpdateYaml({
      url: feed.url,
      publisherName: production.publisherName,
    });
    await patchApp({ version: PREVIOUS_SMOKE_VERSION, feedYaml });
    if (signApp) await signApp();
    const first = await launch({
      args: [UPDATE_SMOKE_FLAG, UPDATE_SMOKE_INSTALL_FLAG],
    });
    assertUpgradeTranscript(first.output, PREVIOUS_SMOKE_VERSION, targetVersion);
    if (first.exitCode == null) {
      throw new Error("previous app did not exit after quitAndInstall");
    }
    await waitForInstalledVersion(readInstalledVersion, targetVersion, sleep, versionAttempts);
    const second = await launch({
      args: [UPDATE_SMOKE_FLAG],
      stopWhen: (output) =>
        output.includes(`desktop_update_smoke_version ${targetVersion}`) &&
        output.includes(READY_MARKER),
    });
    assertBootTranscript(second.output, targetVersion);
    return {
      schemaVersion: 1,
      ok: true,
      fromVersion: PREVIOUS_SMOKE_VERSION,
      toVersion: targetVersion,
      phases: ["available", "deferred", "installing", "booted"],
      feedHost: "127.0.0.1",
      productionFeed: {
        provider: "github",
        owner: RELEASES_OWNER,
        repo: RELEASES_REPO,
      },
    };
  } finally {
    await feed.close();
  }
}

function findNamed(dir, name, depth = 0) {
  const direct = join(dir, name);
  if (existsSync(direct)) return direct;
  if (depth >= 3) return "";
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const found = findNamed(join(dir, entry.name), name, depth + 1);
    if (found) return found;
  }
  return "";
}

function readPackagedVersion(appRoot) {
  const asarPath = join(resourcesDirFromApp(appRoot), "app.asar");
  uncache(asarPath);
  const raw = extractFile(asarPath, "package.json");
  const pkg = JSON.parse(raw.toString("utf8"));
  if (typeof pkg.version !== "string") throw new Error("packaged package.json has no version");
  return pkg.version;
}

function runCommand(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(
      `${command} failed (${result.status}): ${result.stderr || result.stdout || "no output"}`,
    );
  }
  return result.stdout ?? "";
}

export function findWindowsInstall(dest) {
  const executable = join(dest, "Awaken.exe");
  return existsSync(executable) ? executable : "";
}

async function runHostSmoke({ dir, version, receiptPath, platform, arch }) {
  assertTargetVersion(version);
  const payload = hostUpdatePayload(version, platform, arch);
  const work = mkdtempSync(join(tmpdir(), "awaken-installed-update-"));
  const home = join(work, "home");
  mkdirSync(home, { recursive: true });
  let appRoot = "";
  let executable = "";
  if (platform === "darwin") {
    const zipPath = join(dir, payload.payloadName);
    if (!existsSync(zipPath)) throw new Error(`missing ${payload.payloadName}`);
    const unpacked = join(work, "unpacked");
    mkdirSync(unpacked, { recursive: true });
    runCommand("ditto", ["-x", "-k", zipPath, unpacked]);
    appRoot = findNamed(unpacked, "Awaken.app");
    if (!appRoot) throw new Error(`Awaken.app was not found in ${payload.payloadName}`);
    executable = join(appRoot, "Contents", "MacOS", "Awaken");
    spawnSync("xattr", ["-dr", "com.apple.quarantine", appRoot], { encoding: "utf8" });
  } else {
    const exe = join(dir, payload.payloadName);
    if (!existsSync(exe)) throw new Error(`missing ${payload.payloadName}`);
    const dest = join(work, "install");
    mkdirSync(dest, { recursive: true });
    runCommand(exe, windowsSilentInstallArgs(dest));
    executable = findWindowsInstall(dest);
    if (!executable) throw new Error("NSIS install did not produce Awaken.exe");
    appRoot = dirname(executable);
  }
  if (!existsSync(executable)) throw new Error(`packaged executable missing: ${executable}`);
  const productionFeedText = readFileSync(join(resourcesDirFromApp(appRoot), "app-update.yml"), "utf8");
  const files = prepareFeedFiles(dir, payload.feedName, version);
  const env = smokeChildEnvironment(home, process.env);
  const keychainPassword = `awaken-${process.pid}`;
  const receipt = await executeInstalledUpdateSmoke({
    targetVersion: version,
    productionFeedText,
    files,
    patchApp: (patch) => rewriteInstalledApp({ appRoot, ...patch }),
    signApp:
      platform === "darwin"
        ? () =>
            signDarwinApp(appRoot, process.env, runCommand, {
              keychainPath: join(work, "signing.keychain-db"),
              keychainPassword,
              p12Path: join(work, "developer-id.p12"),
              exists: existsSync,
              writeFile: writeFileSync,
            })
        : undefined,
    launch: (spec) =>
      launchPackagedApp({
        executable,
        args: spec.args,
        env,
        timeoutMs: spec.stopWhen ? 180_000 : 8 * 60_000,
        stopWhen: spec.stopWhen,
      }),
    readInstalledVersion: () => readPackagedVersion(appRoot),
  });
  if (platform === "win32") {
    const uninstaller = join(appRoot, "Uninstall Awaken.exe");
    if (existsSync(uninstaller)) {
      spawnSync(uninstaller, ["/S"], { encoding: "utf8" });
    }
  }
  writeFileSync(receiptPath, `${JSON.stringify({ ...receipt, platform, arch }, null, 2)}\n`);
  return receipt;
}

function parseArgs(argv) {
  const args = {
    dir: "",
    version: "",
    receipt: "",
    platform: process.platform,
    arch: process.arch,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    const value = argv[index + 1];
    if (flag === "--dir" || flag === "--version" || flag === "--receipt") {
      if (!value || value.startsWith("--")) throw new Error(`${flag} requires a value`);
      args[flag.slice(2)] = value;
      index += 1;
    } else if (flag === "--platform" || flag === "--arch") {
      if (!value || value.startsWith("--")) throw new Error(`${flag} requires a value`);
      args[flag.slice(2)] = value;
      index += 1;
    } else {
      throw new Error(`unknown argument: ${flag}`);
    }
  }
  if (!args.dir || !args.version || !args.receipt) {
    throw new Error(
      "usage: smoke-installed-update.mjs --dir <artifacts> --version X.Y.Z --receipt <file>",
    );
  }
  return args;
}

const scriptPath = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] || "") === scriptPath) {
  const args = parseArgs(process.argv.slice(2));
  mkdirSync(dirname(resolve(args.receipt)), { recursive: true });
  runHostSmoke({
    dir: resolve(args.dir),
    version: args.version,
    receiptPath: resolve(args.receipt),
    platform: args.platform,
    arch: args.arch,
  }).catch((error) => {
    const message = error instanceof Error ? error.message : String(error);
    writeFileSync(
      resolve(args.receipt),
      `${JSON.stringify({ schemaVersion: 1, ok: false, error: message.slice(0, 2000) }, null, 2)}\n`,
    );
    console.error(`[smoke-installed-update] ${message}`);
    process.exitCode = 1;
  });
}
