#!/usr/bin/env node
import { dirname, isAbsolute, join, resolve } from "node:path";
import {
  chmodSync,
  cpSync,
  existsSync,
  mkdirSync,
  readdirSync,
  realpathSync,
  writeFileSync,
} from "node:fs";
import {
  activateVersion,
  bundleMacOSDependencyClosure,
  buildResolvedState,
  candidateSeedDirs,
  captureMacOSRuntimeSigningMetadata,
  checkPrimaryRuntime,
  cleanupStaging,
  commandOutput,
  componentRelativePath,
  copyDirectoryContents,
  createBuildRevision,
  createStagingDir,
  disposeMacOSRuntimeSigningMetadata,
  ensureDir,
  exists,
  firstExisting,
  inspectRuntimeProvenance,
  isCodexCachePath,
  isWindows,
  macOSRuntimeSigningExpectation,
  materializeMacOSRpathAliases,
  migrateOldAwakenCacheIfNeeded,
  normalizeMacOSRuntimeTree,
  pathInsideRoot,
  probeComponent,
  publishStaging,
  readBashWrapperTarget,
  readRuntimeManifest,
  resolveLibreOfficeInstallRoot,
  resolvePrimaryRuntimeRoot,
  rollbackToPrevious,
  run,
  runOk,
  runtimeRequiresPreparation,
  setMacOSBackgroundOnlyApplication,
  sealMacOSRuntimeApplicationPermissions,
  signMacOSRuntimeApplication,
  stageExecutable,
  versionKey,
  verifyMacOSRuntimeApplication,
  verifyMacOSLibreOfficeReleaseSeed,
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

function pythonCanImport(python) {
  return Boolean(python) && run(python, ["-c", "import pptx, PIL"]).status === 0;
}

function resolvedCopySource(candidate) {
  const value = String(candidate || "").trim();
  if (!value || !existsSync(value) || isCodexCachePath(value)) {
    return "";
  }
  let resolved = value;
  try {
    resolved = realpathSync(value);
  } catch {
    resolved = resolve(value);
  }
  if (isCodexCachePath(resolved)) {
    return "";
  }
  const wrapperTarget = readBashWrapperTarget(resolved);
  if (wrapperTarget) {
    const target = isAbsolute(wrapperTarget)
      ? wrapperTarget
      : resolve(dirname(resolved), wrapperTarget);
    if (!existsSync(target) || isCodexCachePath(target)) {
      return "";
    }
    try {
      resolved = realpathSync(target);
    } catch {
      resolved = target;
    }
  }
  return resolved;
}

function requireExplicitSource(envName) {
  const configured = String(process.env[envName] || "").trim();
  if (!configured) {
    return "";
  }
  const resolved = resolvedCopySource(configured);
  if (!resolved) {
    fail(`primary-runtime: ${envName} is not a valid copy source (${configured})`);
  }
  return resolved;
}

function firstCopySource(candidates) {
  for (const candidate of candidates) {
    const resolved = resolvedCopySource(candidate);
    if (resolved) {
      return resolved;
    }
  }
  return "";
}

function pythonMetadata(python) {
  const result = run(python, [
    "-c",
    "import json,sys; print(json.dumps({'executable':sys.executable,'prefix':sys.prefix,'base_prefix':sys.base_prefix}))",
  ]);
  if (result.status !== 0) {
    return null;
  }
  try {
    return JSON.parse(String(result.stdout || "").trim());
  } catch {
    return null;
  }
}

function pythonDistribution(python) {
  const resolvedPython = resolvedCopySource(python);
  if (!resolvedPython) {
    return null;
  }
  const metadata = pythonMetadata(resolvedPython);
  if (!metadata || metadata.prefix !== metadata.base_prefix) {
    return null;
  }
  let root = metadata.base_prefix;
  try {
    root = realpathSync(root);
  } catch {
    root = resolve(root);
  }
  const executable = resolve(metadata.executable);
  const rel = executable.slice(root.length);
  if (!pathInsideRoot(executable, root)) {
    return null;
  }
  if (existsSync(join(root, "pyvenv.cfg"))) {
    return null;
  }
  return { root, python: resolvedPython, executableRelativePath: rel.replace(/^\/+/, "") };
}

function findPythonDistribution() {
  const explicit = requireExplicitSource("AWAKEN_PYTHON") ||
    requireExplicitSource("AWAKEN_BOOTSTRAP_PYTHON");
  if (explicit) {
    const distribution = pythonDistribution(explicit);
    if (!distribution) {
      fail(`primary-runtime: configured Python is a venv or non-relocatable install (${explicit})`);
    }
    return distribution;
  }

  const seedPaths = isWindows
    ? [
        ...seedCandidates("python", "python.exe"),
        ...seedCandidates("python", "Scripts", "python.exe"),
      ]
    : [
        ...seedCandidates("python", "bin", "python3"),
        ...seedCandidates("python", "bin", "python3.13"),
        ...seedCandidates("python", "bin", "python3.12"),
        ...seedCandidates("python", "bin", "python3.11"),
      ];
  for (const candidate of seedPaths) {
    const distribution = pythonDistribution(candidate);
    if (distribution) {
      return distribution;
    }
  }

  const uv = firstCopySource(whichAll(isWindows ? "uv.exe" : "uv"));
  if (uv) {
    const managed = commandOutput(uv, ["python", "find", "--managed-python"]);
    const distribution = pythonDistribution(managed);
    if (distribution) {
      return distribution;
    }
  }
  return null;
}

function ensurePython(stagingDir) {
  const rel = isWindows ? join("python", "python.exe") : join("python", "bin", "python3");
  const target = join(stagingDir, rel);
  const distribution = findPythonDistribution();
  if (!distribution) {
    fail(
      "primary-runtime: unable to find a relocatable Python distribution; set AWAKEN_PRIMARY_RUNTIME_SEED_DIR/AWAKEN_PYTHON or install an uv-managed Python",
    );
  }

  const venvDir = join(stagingDir, "python");
  emitProgress("host_install", `copying relocatable Python distribution to ${venvDir}`);
  copyDirectoryContents(distribution.root, venvDir);
  if (!existsSync(target)) {
    const copiedOriginal = join(venvDir, distribution.executableRelativePath);
    if (!existsSync(copiedOriginal)) {
      fail(`primary-runtime: copied Python executable missing (${copiedOriginal})`);
    }
    cpSync(copiedOriginal, target, { dereference: true });
    if (!isWindows) {
      chmodSync(target, 0o755);
    }
  }

  const python = target;
  if (!pythonCanImport(python)) {
    emitProgress("host_install", "installing python-pptx and Pillow into managed runtime");
    const pipArgs = [
      "-m",
      "pip",
      "install",
      "--disable-pip-version-check",
      "python-pptx",
      "Pillow",
    ];
    let pipInstall = run(python, pipArgs, { stdio: json ? "ignore" : "inherit" });
    if (pipInstall.status !== 0) {
      pipInstall = run(
        python,
        [...pipArgs.slice(0, 4), "--break-system-packages", ...pipArgs.slice(4)],
        { stdio: json ? "ignore" : "inherit" },
      );
    }
    if (pipInstall.status !== 0 || !pythonCanImport(python)) {
      fail("primary-runtime: failed to install python-pptx and Pillow");
    }
  }
  const metadata = pythonMetadata(python);
  if (!metadata || [metadata.executable, metadata.prefix, metadata.base_prefix].some(
    (entry) => !pathInsideRoot(entry, venvDir),
  )) {
    fail("primary-runtime: copied Python still resolves outside the managed runtime");
  }
  return { target: python, source: distribution.root, mode: "relocatable-distribution" };
}

function executableCandidates(envName, commandName, ...seedSuffixes) {
  return [
    ...seedCandidates(...seedSuffixes),
    ...whichAll(commandName),
  ].filter(Boolean);
}

function executableSource(envName, commandName, ...seedSuffixes) {
  return requireExplicitSource(envName) ||
    firstCopySource(executableCandidates(envName, commandName, ...seedSuffixes));
}

function copyWindowsCompanions(source, targetDir) {
  if (!isWindows) {
    return;
  }
  ensureDir(targetDir);
  for (const entry of readdirSync(dirname(source))) {
    if (entry.toLowerCase().endsWith(".dll")) {
      cpSync(join(dirname(source), entry), join(targetDir, entry), { dereference: true });
    }
  }
}

function ensureNode(stagingDir) {
  const source = executableSource(
    "AWAKEN_NODE",
    "node",
    "node",
    isWindows ? "" : "bin",
    executableName("node"),
  );
  if (!source) {
    fail("primary-runtime: unable to find node");
  }
  const target = isWindows
    ? join(stagingDir, "node", "node.exe")
    : join(stagingDir, "node", "bin", "node");
  if (process.platform === "darwin") {
    const closure = bundleMacOSDependencyClosure(
      [{ source, target, executableSource: source, label: "node" }],
      join(stagingDir, "node", "lib"),
    );
    return { target, source, mode: "copy-with-macho-closure", closure };
  }
  stageExecutable(target, source, "node");
  copyWindowsCompanions(source, dirname(target));
  return { target, source, mode: isWindows ? "copy-with-dlls" : "copy" };
}

function libreOfficeCandidates() {
  return [
    ...seedCandidates("libreoffice"),
    ...seedCandidates("bin", executableName("soffice")),
    ...(process.platform === "darwin"
      ? [
          "/Applications/LibreOffice.app",
          "/Applications/LibreOffice.app/Contents/MacOS/soffice",
        ]
      : []),
    ...whichAll("soffice"),
    ...whichAll("libreoffice"),
    ...(isWindows
      ? [
          join(process.env.ProgramFiles || "", "LibreOffice", "program", "soffice.com"),
          join(process.env["ProgramFiles(x86)"] || "", "LibreOffice", "program", "soffice.com"),
          join(process.env.ProgramFiles || "", "LibreOffice", "program", "soffice.bin.exe"),
          join(process.env["ProgramFiles(x86)"] || "", "LibreOffice", "program", "soffice.bin.exe"),
          join(process.env.ProgramFiles || "", "LibreOffice", "program", "soffice.exe"),
          join(process.env["ProgramFiles(x86)"] || "", "LibreOffice", "program", "soffice.exe"),
        ]
      : []),
    ...(process.platform === "darwin"
      ? ["/opt/homebrew/bin/soffice", "/usr/local/bin/soffice"]
      : []),
  ].filter(Boolean);
}

function ensureLibreOffice(stagingDir, options = {}) {
  const target = join(stagingDir, "libreoffice");
  const explicitSource = requireExplicitSource("AWAKEN_SOFFICE");
  if (options.release && !explicitSource) {
    fail("primary-runtime: release signing requires an explicit pristine AWAKEN_SOFFICE source");
  }
  let source = explicitSource || firstCopySource(libreOfficeCandidates());
  if (!source && autoInstallComponent("libreoffice")) {
    source = firstCopySource(libreOfficeCandidates());
  }
  if (!source) {
    fail("primary-runtime: unable to find LibreOffice/soffice");
  }

  if (isWindows) {
    const root = resolveLibreOfficeInstallRoot(source, process.platform);
    if (!root) {
      fail(`primary-runtime: unable to resolve a bounded Windows LibreOffice install root from ${source}`);
    }
    copyDirectoryContents(root, target);
    return { target, source: root, mode: "copy" };
  }

  if (process.platform === "darwin") {
    const appRoot = resolveLibreOfficeInstallRoot(source, process.platform);
    if (appRoot) {
      const releaseSeed = options.release
        ? verifyMacOSLibreOfficeReleaseSeed(appRoot)
        : null;
      const destApp = join(target, "LibreOffice.app");
      copyDirectoryContents(appRoot, destApp);
      return { target: destApp, source: appRoot, mode: "copy-app-bundle", releaseSeed };
    }
    fail(
      `primary-runtime: LibreOffice source is not a copyable app bundle (${source}); install LibreOffice.app or set AWAKEN_SOFFICE`,
    );
  }

  // Linux packages normally expose /usr/bin/libreoffice as a wrapper whose
  // real executable lives at <install>/program/soffice. Copy that precise
  // product root; never infer or copy the broad /usr/lib directory.
  const linuxRoot = resolveLibreOfficeInstallRoot(source, process.platform);
  if (!linuxRoot) {
    fail(`primary-runtime: unable to resolve a bounded Linux LibreOffice install root from ${source}`);
  }
  copyDirectoryContents(linuxRoot, target);
  return { target, source: linuxRoot, mode: "copy-linux-install-root" };
}

function findPopplerTool(name) {
  const binaryName = executableName(name);
  const configuredDir = String(process.env.AWAKEN_POPPLER_BIN || "").trim();
  if (configuredDir) {
    const configured = resolvedCopySource(join(configuredDir, binaryName));
    if (!configured) {
      fail(`primary-runtime: AWAKEN_POPPLER_BIN does not contain a copyable ${binaryName}`);
    }
    return configured;
  }
  return firstCopySource([
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
    fail("primary-runtime: Poppler executables must come from one Windows bin directory");
  }

  ensureDir(targetDir);
  if (process.platform === "darwin") {
    const entries = sources.map(({ name, source }) => ({
      name,
      source,
      target: join(targetDir, executableName(name)),
      executableSource: source,
      label: name,
    }));
    const closure = bundleMacOSDependencyClosure(entries, join(stagingDir, "poppler", "lib"));
    return entries.map(({ name, source, target }) => ({
      name,
      source,
      target,
      mode: "copy-with-macho-closure",
      closure,
    }));
  }

  // Linux keeps distribution-owned system libraries in /lib or /usr/lib. We
  // copy only the two required tools and deliberately never copy /usr/lib.
  return sources.map(({ name, source }) => {
    const target = join(targetDir, executableName(name));
    stageExecutable(target, source, name);
    return { name, target, source, mode: "copy-linux-executable" };
  });
}

function ensureFFmpeg(stagingDir) {
  const target = join(stagingDir, "ffmpeg", "bin", executableName("ffmpeg"));
  const resolveFFmpegSource = () => requireExplicitSource("AWAKEN_FFMPEG") || firstCopySource([
    ...seedCandidates("ffmpeg", "bin", executableName("ffmpeg")),
    ...seedCandidates("bin", executableName("ffmpeg")),
    ...whichAll(executableName("ffmpeg")),
    ...whichAll("ffmpeg"),
  ]);
  let source = resolveFFmpegSource();
  if (!source && autoInstallComponent("ffmpeg")) {
    const binDir = discoverWingetBinaryDir(wingetPackages.ffmpeg.id, executableName("ffmpeg"));
    if (binDir) {
      source = join(binDir, executableName("ffmpeg"));
    } else {
      source = resolveFFmpegSource();
    }
  }
  if (!source) {
    fail("primary-runtime: unable to find ffmpeg");
  }
  if (process.platform === "darwin") {
    const closure = bundleMacOSDependencyClosure(
      [{ source, target, executableSource: source, label: "ffmpeg" }],
      join(stagingDir, "ffmpeg", "lib"),
    );
    return { target, source, mode: "copy-with-macho-closure", closure };
  }
  stageExecutable(target, source, "ffmpeg");
  copyWindowsCompanions(source, dirname(target));
  return { target, source, mode: isWindows ? "copy-with-dlls" : "copy-linux-executable" };
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

const migration = process.env.AWAKEN_PRIMARY_RUNTIME_ROOT
  ? { migrated: false, skipped: true, reason: "isolated runtime root override" }
  : migrateOldAwakenCacheIfNeeded({ root });
if (migration.migrated) {
  emitProgress("migrate", `migrated old awaken cache to ${migration.active}`);
}

const manifest = readRuntimeManifest();
const bundleVersion = manifest.bundleVersion || manifest.version || "0.2.0";
const macOSSigningExpectation = macOSRuntimeSigningExpectation();
if (macOSSigningExpectation?.release && macOSSigningExpectation.identity === "-") {
  fail(
    "primary-runtime: release signing requires AWAKEN_RUNTIME_CODESIGN_IDENTITY with a Developer ID Application identity",
  );
}
if (macOSSigningExpectation?.release && !String(process.env.AWAKEN_RUNTIME_NOTARY_PROFILE || "").trim()) {
  fail(
    "primary-runtime: release signing requires AWAKEN_RUNTIME_NOTARY_PROFILE for notarytool and stapler",
  );
}
const existing = checkPrimaryRuntime({ root, manifest });
let requiresPreparation = runtimeRequiresPreparation(existing, manifest, repair || migration.migrated, {
  macOSRuntimeSigning: macOSSigningExpectation,
});
if (!requiresPreparation && process.platform === "darwin") {
  const existingLibreOffice = existing.components?.find((component) => component.name === "libreoffice");
  try {
    verifyMacOSRuntimeApplication(existingLibreOffice?.path || "", {
      release: macOSSigningExpectation.release,
      bundleIdentifier: manifest.officeRenderWorker?.macOSBundleIdentifier,
    });
  } catch (error) {
    requiresPreparation = true;
    emitProgress("repair", `active LibreOffice runtime verification failed: ${error.message}`);
  }
}
if (!requiresPreparation) {
  if (json) {
    console.log(JSON.stringify({ ok: true, reused: true, report: existing, migration }, null, 2));
  } else {
    log("[prepare-primary-runtime] primary-runtime already ready");
  }
  console.error(JSON.stringify({ type: "RuntimeReady", active: existing.active }));
  process.exit(0);
}
if (existing.ready && existing.bundleVersion !== bundleVersion) {
  emitProgress(
    "upgrade",
    `active runtime ${existing.bundleVersion || "unknown"} differs from manifest ${bundleVersion}; preparing a new revision`,
  );
}

const revision = createBuildRevision(Date.now(), `${process.pid}:${bundleVersion}`);
const key = versionKey(bundleVersion, process.platform, process.arch, revision);
let stagingDir = "";
let libreOfficeSigningMetadata = null;
const previousActive = existing.active || "";

try {
  stagingDir = createStagingDir(root, "prepare");
  emitProgress("stage", `staging at ${stagingDir}`);

  const prepared = {
    python: ensurePython(stagingDir),
    node: ensureNode(stagingDir),
    libreoffice: ensureLibreOffice(stagingDir, { release: Boolean(macOSSigningExpectation?.release) }),
    poppler: ensurePoppler(stagingDir),
    ffmpeg: ensureFFmpeg(stagingDir),
    helperPackages: ensureHelperPackages(stagingDir, bundleVersion),
  };
  if (process.platform === "darwin") {
    libreOfficeSigningMetadata = captureMacOSRuntimeSigningMetadata(prepared.libreoffice.target);
    prepared.libreoffice.background = setMacOSBackgroundOnlyApplication(prepared.libreoffice.target, {
      ownershipRoot: stagingDir,
      sourceRoot: prepared.libreoffice.source,
      bundleIdentifier: manifest.officeRenderWorker?.macOSBundleIdentifier,
    });
    prepared.libreoffice.aliases = materializeMacOSRpathAliases(prepared.libreoffice.target);
  }
  const normalizedMacOS = normalizeMacOSRuntimeTree(stagingDir);
  const signedMacOSLibreOffice = process.platform === "darwin"
    ? signMacOSRuntimeApplication(prepared.libreoffice.target, {
        ownershipRoot: stagingDir,
        sourceRoot: prepared.libreoffice.source,
        bundleIdentifier: manifest.officeRenderWorker?.macOSBundleIdentifier,
        signingMetadata: libreOfficeSigningMetadata,
      })
    : { signed: false, reason: "not macOS" };

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
  if (process.platform === "darwin") {
    state.security = { macOSRuntimeSigning: signedMacOSLibreOffice };
  }
  state.officeRender = manifest.officeRenderWorker || {};
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
  const provenance = inspectRuntimeProvenance(stagingDir, stagingDir, process.platform);
  if (!provenance.ok) {
    const summary = provenance.issues
      .slice(0, 20)
      .map((issue) => `${issue.code}: ${issue.detail} (${issue.path})`)
      .join("; ");
    throw new Error(`staged provenance failed: ${summary}`);
  }
  const immutableMacOSLibreOffice = process.platform === "darwin"
    ? sealMacOSRuntimeApplicationPermissions(prepared.libreoffice.target, {
        ownershipRoot: stagingDir,
        sourceRoot: prepared.libreoffice.source,
      })
    : { sealed: false, reason: "not macOS" };
  if (process.platform === "darwin") {
    signedMacOSLibreOffice.immutablePermissions = immutableMacOSLibreOffice.sealed;
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
    console.log(JSON.stringify({
      ok: true,
      prepared,
      normalizedMacOS,
      signedMacOSLibreOffice,
      immutableMacOSLibreOffice,
      report,
      key,
      migration,
    }, null, 2));
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
  disposeMacOSRuntimeSigningMetadata(libreOfficeSigningMetadata);
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
