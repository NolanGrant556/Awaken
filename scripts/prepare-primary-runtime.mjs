#!/usr/bin/env node
import { basename, dirname, join } from "node:path";
import { mkdirSync, writeFileSync } from "node:fs";
import {
  candidateSeedDirs,
  checkPrimaryRuntime,
  commandOutput,
  ensureDir,
  firstExisting,
  isWindows,
  linkDirectory,
  runtimeDir,
  run,
  stageExecutable,
  whichAll,
  writeExecWrapper,
} from "./primary-runtime-common.mjs";

const args = new Set(process.argv.slice(2));
const json = args.has("--json");

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

function ensureLibreOffice() {
  const candidates = [
    process.env.AWAKEN_SOFFICE,
    ...seedCandidates("bin", executableName("soffice")),
    ...whichAll("soffice"),
    ...whichAll("libreoffice"),
    ...(isWindows
      ? [
          join(process.env.ProgramFiles || "", "LibreOffice", "program", "soffice.exe"),
          join(process.env["ProgramFiles(x86)"] || "", "LibreOffice", "program", "soffice.exe"),
        ]
      : []),
    "/Applications/LibreOffice.app/Contents/MacOS/soffice",
    "/opt/homebrew/bin/soffice",
    "/usr/local/bin/soffice",
  ].filter(Boolean);
  const source = firstExisting(candidates);
  if (!source) {
    fail("primary-runtime: unable to find LibreOffice/soffice");
  }
  if (isWindows) {
    const parent = dirname(source);
    const root = basename(parent).toLowerCase() === "program" ? dirname(parent) : parent;
    const target = join(runtimeDir, "libreoffice");
    try {
      linkDirectory(target, root, "libreoffice");
    } catch (error) {
      fail(`primary-runtime: failed to link LibreOffice runtime: ${error.message}`);
    }
    return { target, source: root, mode: "junction" };
  }
  const target = join(runtimeDir, "libreoffice", "LibreOffice.app", "Contents", "MacOS", "soffice");
  writeExecWrapper(target, source, "soffice");
  return { target, source, mode: "wrapper" };
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

function ensurePoppler() {
  const sources = ["pdftoppm", "pdfinfo"].map((name) => {
    const source = findPopplerTool(name);
    if (!source) {
      fail(`primary-runtime: unable to find Poppler tool ${name}`);
    }
    return { name, source };
  });
  if (isWindows) {
    const sourceDirs = [...new Set(sources.map(({ source }) => dirname(source)))];
    if (sourceDirs.length === 1) {
      const target = join(runtimeDir, "poppler", "bin");
      try {
        linkDirectory(target, sourceDirs[0], "poppler");
      } catch (error) {
        fail(`primary-runtime: failed to link Poppler runtime: ${error.message}`);
      }
      return sources.map(({ name, source }) => ({
        name,
        target: join(target, executableName(name)),
        source,
        mode: "junction",
      }));
    }
  }
  return sources.map(({ name, source }) => {
    const target = join(runtimeDir, "poppler", "bin", executableName(name));
    stageExecutable(target, source, name);
    return { name, target, source, mode: isWindows ? "copy" : "wrapper" };
  });
}

function ensureFFmpeg() {
  const source = firstExisting(executableCandidates("AWAKEN_FFMPEG", "ffmpeg", "bin", executableName("ffmpeg")));
  if (!source) {
    fail("primary-runtime: unable to find ffmpeg");
  }
  const target = join(runtimeDir, "ffmpeg", "bin", executableName("ffmpeg"));
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
