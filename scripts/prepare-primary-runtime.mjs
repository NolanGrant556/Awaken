#!/usr/bin/env node
import { basename, dirname, join } from "node:path";
import { mkdirSync, readdirSync, writeFileSync, cpSync, existsSync } from "node:fs";
import {
  activateVersion,
  buildResolvedState,
  candidateSeedDirs,
  checkPrimaryRuntime,
  cleanupStaging,
  commandOutput,
  componentRelativePath,
  copyDirectoryContents,
  createStagingDir,
  ensureDir,
  exists,
  firstExisting,
  isWindows,
  migrateOldAwakenCacheIfNeeded,
  probeComponent,
  publishStaging,
  readRuntimeManifest,
  resolvePrimaryRuntimeRoot,
  rollbackToPrevious,
  run,
  runOk,
  stageExecutable,
  versionKey,
  whichAll,
  writeResolvedState,
} from "./primary-runtime-common.mjs";

const args = new Set(process.argv.slice(2));
const json = args.has("--json");
const shouldAutoInstall = args.has("--install") || args.has("-i");
const repair = args.has("--repair");

function log(message) {
  if (!json) {
    console.log(message);
  }
}

function emitProgress(phase, message, extra = {}) {
  const payload = { type: "RuntimePrepareProgress", phase, message, ...extra };
  if (json) {
    console.error(JSON.stringify(payload));
  } else {
    log(`[prepare-primary-runtime] ${phase}: ${message}`);
  }
}

function fail(message, extra = {}) {
  if (json) {
    console.log(JSON.stringify({ ok: false, error: message, ...extra }, null, 2));
  } else {
    console.error(message);
  }
  process.exit(1);
}

function seedCandidates(...suffixes) {
  return candidateSeedDirs().map((dir) => join(dir, ...suffixes));
}

const wingetPackages = {
  libreoffice: {
    id: "TheDocumentFoundation.LibreOffice",
    label: "LibreOffice",
  },
  ffmpeg: {
    id: "Gyan.FFmpeg",
    label: "FFmpeg",
  },
  poppler: {
    id: "oschwartz10612.Poppler",
    label: "Poppler",
  },
};

function hasWinget() {
  return isWindows && runOk("winget", ["--version"]);
}

function wingetIsInstalled(id) {
  return runOk("winget", [
    "list",
    "--id",
    id,
    "--accept-source-agreements",
    "--disable-interactivity",
  ]);
}

function wingetInstall({ id, label }) {
  if (wingetIsInstalled(id)) {
    emitProgress("host_install", `${label} already installed via winget`);
    return;
  }
  emitProgress("host_install", `installing ${label} via winget (${id})`);
  const result = run(
    "winget",
    [
      "install",
      "--id",
      id,
      "--accept-package-agreements",
      "--accept-source-agreements",
      "--silent",
      "--disable-interactivity",
    ],
    { stdio: "inherit" },
  );
  if (result.status !== 0) {
    fail(`primary-runtime: failed to install ${label} via winget`);
  }
}

function wingetPackagesDir() {
  return join(process.env.LOCALAPPDATA || "", "Microsoft", "WinGet", "Packages");
}

function discoverWingetBinaryDir(packageId, executableName) {
  const base = wingetPackagesDir();
  if (!base) {
    return "";
  }
  const prefix = `${packageId}_Microsoft.Winget.Source_8wekyb3d8bbwe`;
  try {
    for (const entry of readdirSync(base, { withFileTypes: true })) {
      if (!entry.isDirectory() || !entry.name.startsWith(prefix)) {
        continue;
      }
      const root = join(base, entry.name);
      for (const sub of readdirSync(root, { withFileTypes: true })) {
        if (!sub.isDirectory()) {
          continue;
        }
        const candidates = [join(root, sub.name, executableName)];
        if (executableName.endsWith(".exe")) {
          candidates.push(join(root, sub.name, "bin", executableName));
          candidates.push(join(root, sub.name, "Library", "bin", executableName));
        }
        for (const candidate of candidates) {
          if (firstExisting([candidate])) {
            return dirname(candidate);
          }
        }
      }
    }
  } catch {
    // ignore
  }
  return "";
}

function autoInstallComponent(name) {
  if (!shouldAutoInstall) {
    return false;
  }
  const spec = wingetPackages[name];
  if (!spec) {
    return false;
  }
  if (!hasWinget()) {
    fail(`primary-runtime: ${name} is missing and winget is not available for auto-install`);
  }
  wingetInstall(spec);
  return true;
}

function executableName(name) {
  return isWindows ? `${name}.exe` : name;
}

function pythonSpec(command, args = []) {
  return { command, args, label: [command, ...args].join(" ") };
}

function runPython(spec, args, options = {}) {
  if (typeof spec === "string") {
    return run(spec, args, options);
  }
  return run(spec.command, [...spec.args, ...args], options);
}

function pythonCanImport(spec) {
  return Boolean(typeof spec === "string" ? spec : spec?.command) &&
    runPython(spec, ["-c", "import pptx, PIL"]).status === 0;
}

function findPreparedPython() {
  const candidates = [
    process.env.AWAKEN_PYTHON,
    ...(isWindows
      ? seedCandidates("python", "Scripts", "python.exe")
      : []),
    ...seedCandidates("python", "bin", "python3"),
  ].filter(Boolean);
  return candidates.find((candidate) => pythonCanImport(pythonSpec(candidate))) || "";
}

function findBasePython() {
  // System python is copy/bootstrap source only — never a RuntimeReady target.
  const candidates = [
    process.env.AWAKEN_BOOTSTRAP_PYTHON ? pythonSpec(process.env.AWAKEN_BOOTSTRAP_PYTHON) : null,
    ...whichAll("python3").map((path) => pythonSpec(path)),
    ...whichAll("python").map((path) => pythonSpec(path)),
    ...(isWindows ? [pythonSpec("py", ["-3"])] : []),
    ...seedCandidates("python", "bin", "python3").map((path) => pythonSpec(path)),
    ...(isWindows
      ? seedCandidates("python", "Scripts", "python.exe").map((path) => pythonSpec(path))
      : []),
  ].filter(Boolean);
  return candidates.find((candidate) => runPython(candidate, ["--version"]).status === 0) || null;
}

function ensurePython(stagingDir) {
  const rel = isWindows ? join("python", "Scripts", "python.exe") : join("python", "bin", "python3");
  const target = join(stagingDir, rel);
  const prepared = findPreparedPython();
  if (prepared) {
    // Prefer creating a managed venv from prepared interpreter when possible.
  }

  const basePython = prepared ? pythonSpec(prepared) : findBasePython();
  if (!basePython) {
    fail("primary-runtime: unable to find Python for bootstrapping");
  }

  const venvDir = join(stagingDir, "python");
  emitProgress("host_install", `creating managed Python venv at ${venvDir}`);
  ensureDir(dirname(venvDir));
  const venv = runPython(basePython, ["-m", "venv", venvDir], { stdio: json ? "ignore" : "inherit" });
  if (venv.status !== 0) {
    fail("primary-runtime: failed to create Python venv");
  }

  const python = target;
  emitProgress("host_install", "installing python-pptx and Pillow into managed runtime");
  const pipInstall = run(
    python,
    ["-m", "pip", "install", "--upgrade", "pip", "python-pptx", "Pillow"],
    { stdio: json ? "ignore" : "inherit" },
  );
  if (pipInstall.status !== 0 || !pythonCanImport(python)) {
    fail("primary-runtime: failed to install python-pptx and Pillow");
  }
  return { target: python, source: basePython.label || String(basePython.command), mode: "venv" };
}

function executableCandidates(envName, commandName, ...seedSuffixes) {
  return [
    process.env[envName],
    ...seedCandidates(...seedSuffixes),
    ...whichAll(commandName),
  ].filter(Boolean);
}

function ensureNode(stagingDir) {
  const source = firstExisting(
    executableCandidates("AWAKEN_NODE", "node", "node", isWindows ? "" : "bin", executableName("node")),
  );
  if (!source) {
    fail("primary-runtime: unable to find node");
  }
  const target = isWindows
    ? join(stagingDir, "node", "node.exe")
    : join(stagingDir, "node", "bin", "node");
  stageExecutable(target, source, "node");
  return { target, source, mode: "copy" };
}

function libreOfficeCandidates() {
  return [
    process.env.AWAKEN_SOFFICE,
    ...seedCandidates("bin", executableName("soffice")),
    ...seedCandidates("libreoffice"),
    ...whichAll("soffice"),
    ...whichAll("libreoffice"),
    ...(isWindows
      ? [
          join(process.env.ProgramFiles || "", "LibreOffice", "program", "soffice.bin.exe"),
          join(process.env["ProgramFiles(x86)"] || "", "LibreOffice", "program", "soffice.bin.exe"),
          join(process.env.ProgramFiles || "", "LibreOffice", "program", "soffice.exe"),
          join(process.env["ProgramFiles(x86)"] || "", "LibreOffice", "program", "soffice.exe"),
        ]
      : []),
    "/Applications/LibreOffice.app/Contents/MacOS/soffice",
    "/opt/homebrew/bin/soffice",
    "/usr/local/bin/soffice",
  ].filter(Boolean);
}

function ensureLibreOffice(stagingDir) {
  const target = join(stagingDir, "libreoffice");
  let source = firstExisting(libreOfficeCandidates());
  if (!source && autoInstallComponent("libreoffice")) {
    source = firstExisting(libreOfficeCandidates());
  }
  if (!source) {
    fail("primary-runtime: unable to find LibreOffice/soffice");
  }

  if (isWindows) {
    const parent = dirname(source);
    const root = basename(parent).toLowerCase() === "program" ? dirname(parent) : parent;
    copyDirectoryContents(root, target);
    return { target, source: root, mode: "copy" };
  }

  // macOS app bundle or linux install: copy app/dir when possible, else copy soffice binary tree
  if (source.includes("LibreOffice.app")) {
    const appRoot = source.slice(0, source.indexOf("LibreOffice.app") + "LibreOffice.app".length);
    const destApp = join(target, "LibreOffice.app");
    copyDirectoryContents(appRoot, destApp);
    return { target: destApp, source: appRoot, mode: "copy" };
  }
  if (existsSync(source) && source.endsWith(".app")) {
    const destApp = join(target, "LibreOffice.app");
    copyDirectoryContents(source, destApp);
    return { target: destApp, source, mode: "copy" };
  }

  // Fallback: copy executable into managed Contents/MacOS layout
  const wrapperTarget = join(target, "LibreOffice.app", "Contents", "MacOS", "soffice");
  stageExecutable(wrapperTarget, source, "soffice");
  return { target: join(target, "LibreOffice.app"), source, mode: "copy" };
}

function findPopplerTool(name) {
  const binaryName = executableName(name);
  return firstExisting([
    process.env.AWAKEN_POPPLER_BIN ? join(process.env.AWAKEN_POPPLER_BIN, binaryName) : "",
    ...seedCandidates("poppler", "bin", binaryName),
    ...seedCandidates("bin", binaryName),
    ...whichAll(binaryName),
    ...whichAll(name),
    join("/opt/homebrew/bin", name),
    join("/usr/local/bin", name),
  ]);
}

function ensurePoppler(stagingDir) {
  const targetDir = join(stagingDir, "poppler", "bin");
  let sources = ["pdftoppm", "pdfinfo"].map((name) => ({
    name,
    source: findPopplerTool(name),
  }));
  if (sources.some(({ source }) => !source) && autoInstallComponent("poppler")) {
    const binDir = discoverWingetBinaryDir(wingetPackages.poppler.id, executableName("pdftoppm"));
    if (binDir) {
      sources = ["pdftoppm", "pdfinfo"].map((name) => ({
        name,
        source: join(binDir, executableName(name)),
      }));
    } else {
      sources = ["pdftoppm", "pdfinfo"].map((name) => ({
        name,
        source: findPopplerTool(name),
      }));
    }
  }
  for (const { name, source } of sources) {
    if (!source) {
      fail(`primary-runtime: unable to find Poppler tool ${name}`);
    }
  }

  // Windows: copy full bin directory so DLLs travel with the exes.
  if (isWindows) {
    const sourceDirs = [...new Set(sources.map(({ source }) => dirname(source)))];
    if (sourceDirs.length === 1) {
      copyDirectoryContents(sourceDirs[0], targetDir);
      return sources.map(({ name, source }) => ({
        name,
        target: join(targetDir, executableName(name)),
        source,
        mode: "copy-dir",
      }));
    }
  }

  // Also copy sibling shared libs when present next to unix binaries.
  const sourceDir = dirname(sources[0].source);
  ensureDir(targetDir);
  try {
    for (const entry of readdirSync(sourceDir)) {
      const lower = entry.toLowerCase();
      if (
        lower.startsWith("pdf") ||
        lower.includes("poppler") ||
        lower.endsWith(".dylib") ||
        lower.endsWith(".so") ||
        lower.endsWith(".dll")
      ) {
        const from = join(sourceDir, entry);
        const to = join(targetDir, entry);
        if (existsSync(from)) {
          cpSync(from, to, { dereference: true });
        }
      }
    }
  } catch {
    // fall through to explicit exe copy
  }
  return sources.map(({ name, source }) => {
    const target = join(targetDir, executableName(name));
    if (!exists(target)) {
      stageExecutable(target, source, name);
    }
    return { name, target, source, mode: "copy" };
  });
}

function ensureFFmpeg(stagingDir) {
  const target = join(stagingDir, "ffmpeg", "bin", executableName("ffmpeg"));
  let source = firstExisting(executableCandidates("AWAKEN_FFMPEG", "ffmpeg", "bin", executableName("ffmpeg")));
  if (!source && autoInstallComponent("ffmpeg")) {
    const binDir = discoverWingetBinaryDir(wingetPackages.ffmpeg.id, executableName("ffmpeg"));
    if (binDir) {
      source = join(binDir, executableName("ffmpeg"));
    } else {
      source = firstExisting(executableCandidates("AWAKEN_FFMPEG", "ffmpeg", "bin", executableName("ffmpeg")));
    }
  }
  if (!source) {
    fail("primary-runtime: unable to find ffmpeg");
  }
  stageExecutable(target, source, "ffmpeg");
  return { target, source, mode: "copy" };
}

function ensureHelperPackages(stagingDir, bundleVersion) {
  const packagesDir = join(stagingDir, "packages");
  mkdirSync(packagesDir, { recursive: true });
  writeFileSync(
    join(packagesDir, "runtime.json"),
    JSON.stringify(
      {
        name: "awaken-primary-runtime-packages",
        version: bundleVersion,
        generatedAt: new Date().toISOString(),
      },
      null,
      2,
    ) + "\n",
  );
  return { target: packagesDir, source: "generated", mode: "directory" };
}

const root = resolvePrimaryRuntimeRoot();
ensureDir(root);

emitProgress("started", "RuntimePrepareStarted", { runtimeRoot: root });
console.error(JSON.stringify({ type: "RuntimePrepareStarted", runtimeRoot: root }));

const migration = migrateOldAwakenCacheIfNeeded({ root });
if (migration.migrated) {
  emitProgress("migrate", `migrated old awaken cache to ${migration.active}`);
}

const existing = checkPrimaryRuntime({ root });
if (existing.ready && !repair) {
  if (json) {
    console.log(JSON.stringify({ ok: true, reused: true, report: existing, migration }, null, 2));
  } else {
    log("[prepare-primary-runtime] primary-runtime already ready");
  }
  console.error(JSON.stringify({ type: "RuntimeReady", active: existing.active }));
  process.exit(0);
}

const manifest = readRuntimeManifest();
const bundleVersion = manifest.bundleVersion || manifest.version || "0.2.0";
const key = versionKey(bundleVersion);
let stagingDir = "";
const previousActive = existing.active || "";

try {
  stagingDir = createStagingDir(root, "prepare");
  emitProgress("stage", `staging at ${stagingDir}`);

  const prepared = {
    python: ensurePython(stagingDir),
    node: ensureNode(stagingDir),
    libreoffice: ensureLibreOffice(stagingDir),
    poppler: ensurePoppler(stagingDir),
    ffmpeg: ensureFFmpeg(stagingDir),
    helperPackages: ensureHelperPackages(stagingDir, bundleVersion),
  };

  const state = buildResolvedState({
    manifest,
    versionRootPath: stagingDir,
    sources: {
      python: prepared.python.mode,
      node: prepared.node.mode,
      libreoffice: prepared.libreoffice.mode,
      poppler: Array.isArray(prepared.poppler) ? prepared.poppler[0]?.mode : prepared.poppler.mode,
      ffmpeg: prepared.ffmpeg.mode,
      "helper-packages": prepared.helperPackages.mode,
    },
  });
  // Probe against staging as temporary ownership root before publish.
  writeResolvedState(stagingDir, { ...state, root: stagingDir });

  emitProgress("probe", "probing staged components");
  const missing = [];
  for (const component of manifest.components || []) {
    const absolutePath = join(stagingDir, componentRelativePath(component));
    const status = probeComponent(component, absolutePath, stagingDir);
    if (component.required && !status.ok) {
      missing.push(`${component.name}: ${status.detail}`);
    }
  }
  if (missing.length > 0) {
    throw new Error(`staged probe failed: ${missing.join("; ")}`);
  }

  emitProgress("publish", `publishing ${key}`);
  const versionRoot = publishStaging(root, stagingDir, key);
  stagingDir = ""; // published
  writeResolvedState(versionRoot, { ...state, root: versionRoot });
  activateVersion(root, key);

  const report = checkPrimaryRuntime({ root });
  if (!report.ready) {
    throw new Error(`published runtime not ready: ${report.missing.join(", ")}`);
  }

  if (json) {
    console.log(JSON.stringify({ ok: true, prepared, report, key, migration }, null, 2));
  } else {
    log("[prepare-primary-runtime] primary-runtime ready");
    for (const component of report.components) {
      log(`- ${component.name}: ${component.detail || "ok"}`);
    }
    const pythonPath = join(versionRoot, componentRelativePath(
      (manifest.components || []).find((c) => c.name === "python") || { relativePath: "python/bin/python3" },
    ));
    const version = commandOutput(pythonPath, ["--version"]);
    if (version) {
      log(`- python: ${version}`);
    }
  }
  console.error(JSON.stringify({ type: "RuntimeReady", active: key }));
} catch (error) {
  cleanupStaging(stagingDir);
  let rolledBack = null;
  if (repair || previousActive) {
    const stillBroken = !checkPrimaryRuntime({ root }).ready;
    if (stillBroken && (existing.previous || previousActive)) {
      // Restore previous known-good pointer when repair/publish fails.
      if (existing.previous) {
        rolledBack = rollbackToPrevious(root);
      } else if (previousActive) {
        activateVersion(root, previousActive, { retainPrevious: false });
        rolledBack = { ok: true, active: previousActive };
      }
    }
  }
  console.error(JSON.stringify({
    type: "RuntimeRepairFailed",
    error: error.message,
    rolledBack,
  }));
  fail(`primary-runtime prepare failed: ${error.message}`, { rolledBack, previousActive });
}
