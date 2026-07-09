#!/usr/bin/env node
import { basename, dirname, join } from "node:path";
import { mkdirSync, readdirSync, writeFileSync } from "node:fs";
import {
  candidateSeedDirs,
  checkPrimaryRuntime,
  commandOutput,
  ensureDir,
  findSofficeExecutable,
  firstExisting,
  isWindows,
  linkDirectory,
  runtimeDir,
  run,
  runOk,
  stageExecutable,
  whichAll,
  writeExecWrapper,
} from "./primary-runtime-common.mjs";

const args = new Set(process.argv.slice(2));
const json = args.has("--json");
const shouldAutoInstall = args.has("--install") || args.has("-i");

function log(message) {
  if (!json) {
    console.log(message);
  }
}

function fail(message) {
  if (json) {
    console.log(JSON.stringify({ ok: false, error: message }, null, 2));
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
    log(`[prepare-primary-runtime] ${label} is already installed via winget`);
    return;
  }
  log(`[prepare-primary-runtime] installing ${label} via winget (${id})...`);
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
  log(`[prepare-primary-runtime] ${label} installed`);
}

function wingetPackagesDir() {
  return join(
    process.env.LOCALAPPDATA || "",
    "Microsoft",
    "WinGet",
    "Packages",
  );
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

function addToProcessPath(dir) {
  if (!dir) {
    return;
  }
  const sep = isWindows ? ";" : ":";
  const current = process.env.PATH || "";
  const parts = current.split(sep).filter(Boolean);
  if (!parts.includes(dir)) {
    process.env.PATH = [dir, ...parts].join(sep);
  }
}

function persistUserPathEntry(dir) {
  if (!isWindows || !dir) {
    return;
  }
  try {
    const result = run(
      "powershell",
      [
        "-ExecutionPolicy",
        "Bypass",
        "-Command",
        `$current = [Environment]::GetEnvironmentVariable('PATH', 'User'); $parts = $current -split ';' | Where-Object { $_ -ne '' }; if ($parts -notcontains '${dir.replace(/'/g, "''")}') { [Environment]::SetEnvironmentVariable('PATH', (($parts + '${dir.replace(/'/g, "''")}') -join ';'), 'User') }`,
      ],
      { stdio: "ignore" },
    );
    if (result.status !== 0) {
      // Non-fatal; process PATH is already updated for this run.
    }
  } catch {
    // ignore
  }
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

function pythonTargetPath() {
  return isWindows
    ? join(runtimeDir, "python", "Scripts", "python.exe")
    : join(runtimeDir, "python", "bin", "python3");
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
      ? [
          ...seedCandidates("python", "Scripts", "python.exe"),
          join(runtimeDir, "python", "Scripts", "python.exe"),
        ]
      : []),
    ...seedCandidates("python", "bin", "python3"),
    join(runtimeDir, "python", "bin", "python3"),
    ...whichAll("python3"),
    ...whichAll("python"),
  ].filter(Boolean);
  return candidates.find((candidate) => pythonCanImport(pythonSpec(candidate))) || "";
}

function findBasePython() {
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

function ensurePython() {
  const target = pythonTargetPath();
  const prepared = findPreparedPython();
  if (prepared === target) {
    return { target, source: target, mode: "existing" };
  }
  if (prepared && prepared !== target) {
    if (!isWindows) {
      writeExecWrapper(target, prepared, "python");
      return { target, source: prepared, mode: "wrapper" };
    }
  }

  const basePython = prepared ? pythonSpec(prepared) : findBasePython();
  if (!basePython) {
    fail("primary-runtime: unable to find Python for bootstrapping");
  }

  const venvDir = join(runtimeDir, "python");
  log(`[prepare-primary-runtime] creating Python venv at ${venvDir}`);
  ensureDir(dirname(venvDir));
  const venv = runPython(basePython, ["-m", "venv", venvDir], { stdio: "inherit" });
  if (venv.status !== 0) {
    fail("primary-runtime: failed to create Python venv");
  }

  const python = pythonTargetPath();
  const pipInstall = run(
    python,
    ["-m", "pip", "install", "--upgrade", "pip", "python-pptx", "Pillow"],
    { stdio: "inherit" },
  );
  if (pipInstall.status !== 0 || !pythonCanImport(python)) {
    fail("primary-runtime: failed to install python-pptx and Pillow");
  }
  return { target: python, source: basePython.label, mode: "venv" };
}

function executableCandidates(envName, commandName, ...seedSuffixes) {
  return [
    process.env[envName],
    ...seedCandidates(...seedSuffixes),
    ...whichAll(commandName),
  ].filter(Boolean);
}

function ensureNode() {
  const source = firstExisting(
    executableCandidates("AWAKEN_NODE", "node", "node", isWindows ? "" : "bin", executableName("node")),
  );
  if (!source) {
    fail("primary-runtime: unable to find node");
  }
  const target = isWindows
    ? join(runtimeDir, "node", "node.exe")
    : join(runtimeDir, "node", "bin", "node");
  stageExecutable(target, source, "node");
  return { target, source, mode: isWindows ? "copy" : "wrapper" };
}

function libreOfficeCandidates() {
  return [
    process.env.AWAKEN_SOFFICE,
    join(runtimeDir, "libreoffice", "program", "soffice.bin.exe"),
    join(runtimeDir, "libreoffice", "program", "soffice.exe"),
    join(runtimeDir, "libreoffice", "Contents", "MacOS", "soffice"),
    join(runtimeDir, "libreoffice", "soffice.bin.exe"),
    join(runtimeDir, "libreoffice", "soffice.exe"),
    ...seedCandidates("bin", executableName("soffice")),
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

function ensureLibreOffice() {
  const target = join(runtimeDir, "libreoffice");
  const existing = findSofficeExecutable(target);
  if (existing) {
    return { target, source: existing, mode: "existing" };
  }

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
    try {
      linkDirectory(target, root, "libreoffice");
    } catch (error) {
      fail(`primary-runtime: failed to link LibreOffice runtime: ${error.message}`);
    }
    return { target, source: root, mode: "junction" };
  }
  const wrapperTarget = join(target, "LibreOffice.app", "Contents", "MacOS", "soffice");
  writeExecWrapper(wrapperTarget, source, "soffice");
  return { target: wrapperTarget, source, mode: "wrapper" };
}

function findPopplerTool(name) {
  const binaryName = executableName(name);
  return firstExisting([
    process.env.AWAKEN_POPPLER_BIN ? join(process.env.AWAKEN_POPPLER_BIN, binaryName) : "",
    join(runtimeDir, "poppler", "bin", binaryName),
    ...seedCandidates("poppler", "bin", binaryName),
    ...seedCandidates("bin", binaryName),
    ...whichAll(binaryName),
    ...whichAll(name),
    join("/opt/homebrew/bin", name),
    join("/usr/local/bin", name),
  ]);
}

function ensurePoppler() {
  const targetDir = join(runtimeDir, "poppler", "bin");
  const existingTargets = ["pdftoppm", "pdfinfo"].map((name) => ({
    name,
    target: join(targetDir, executableName(name)),
    source: findPopplerTool(name),
  }));
  if (existingTargets.every(({ source }) => source)) {
    return existingTargets.map(({ name, target, source }) => ({
      name,
      target,
      source,
      mode: "existing",
    }));
  }

  let sources = ["pdftoppm", "pdfinfo"].map((name) => {
    const source = findPopplerTool(name);
    return { name, source };
  });
  if (sources.some(({ source }) => !source) && autoInstallComponent("poppler")) {
    const binDir = discoverWingetBinaryDir(wingetPackages.poppler.id, executableName("pdftoppm"));
    if (binDir) {
      addToProcessPath(binDir);
      persistUserPathEntry(binDir);
    }
    sources = ["pdftoppm", "pdfinfo"].map((name) => {
      const source = findPopplerTool(name);
      return { name, source };
    });
  }
  for (const { name, source } of sources) {
    if (!source) {
      fail(`primary-runtime: unable to find Poppler tool ${name}`);
    }
  }
  if (isWindows) {
    const sourceDirs = [...new Set(sources.map(({ source }) => dirname(source)))];
    if (sourceDirs.length === 1) {
      try {
        linkDirectory(targetDir, sourceDirs[0], "poppler");
      } catch (error) {
        fail(`primary-runtime: failed to link Poppler runtime: ${error.message}`);
      }
      return sources.map(({ name, source }) => ({
        name,
        target: join(targetDir, executableName(name)),
        source,
        mode: "junction",
      }));
    }
  }
  return sources.map(({ name, source }) => {
    const target = join(targetDir, executableName(name));
    stageExecutable(target, source, name);
    return { name, target, source, mode: isWindows ? "copy" : "wrapper" };
  });
}

function ensureFFmpeg() {
  const target = join(runtimeDir, "ffmpeg", "bin", executableName("ffmpeg"));
  if (firstExisting([target])) {
    return { target, source: target, mode: "existing" };
  }

  let source = firstExisting(executableCandidates("AWAKEN_FFMPEG", "ffmpeg", "bin", executableName("ffmpeg")));
  if (!source && autoInstallComponent("ffmpeg")) {
    const binDir = discoverWingetBinaryDir(wingetPackages.ffmpeg.id, executableName("ffmpeg"));
    if (binDir) {
      addToProcessPath(binDir);
      persistUserPathEntry(binDir);
    }
    source = firstExisting(executableCandidates("AWAKEN_FFMPEG", "ffmpeg", "bin", executableName("ffmpeg")));
  }
  if (!source) {
    fail("primary-runtime: unable to find ffmpeg");
  }
  stageExecutable(target, source, "ffmpeg");
  return { target, source, mode: isWindows ? "copy" : "wrapper" };
}

function ensureHelperPackages() {
  const packagesDir = join(runtimeDir, "packages");
  mkdirSync(packagesDir, { recursive: true });
  writeFileSync(
    join(packagesDir, "runtime.json"),
    JSON.stringify(
      {
        name: "awaken-primary-runtime-packages",
        version: "0.1.0",
        generatedAt: new Date().toISOString(),
      },
      null,
      2,
    ) + "\n",
  );
  return { target: packagesDir, source: "generated", mode: "directory" };
}

ensureDir(runtimeDir);

const prepared = {
  python: ensurePython(),
  node: ensureNode(),
  libreoffice: ensureLibreOffice(),
  poppler: ensurePoppler(),
  ffmpeg: ensureFFmpeg(),
  helperPackages: ensureHelperPackages(),
};

const report = checkPrimaryRuntime();
if (!report.ready) {
  if (json) {
    console.log(JSON.stringify({ ok: false, prepared, report }, null, 2));
  } else {
    console.error("[prepare-primary-runtime] runtime is still missing:", report.missing.join(", "));
    for (const component of report.components) {
      console.error(`- ${component.name}: ${component.ok ? "ok" : "missing"} ${component.path}`);
    }
  }
  process.exit(1);
}

if (json) {
  console.log(JSON.stringify({ ok: true, prepared, report }, null, 2));
} else {
  log("[prepare-primary-runtime] primary-runtime ready");
  for (const component of report.components) {
    log(`- ${component.name}: ${component.detail || "ok"}`);
  }
  const version = commandOutput(prepared.python.target, ["--version"]);
  if (version) {
    log(`- python: ${version}`);
  }
}
