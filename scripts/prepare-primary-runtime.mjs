#!/usr/bin/env node
import { dirname, join } from "node:path";
import { mkdirSync, writeFileSync } from "node:fs";
import {
  candidateSeedDirs,
  checkPrimaryRuntime,
  commandOutput,
  ensureDir,
  firstExisting,
  runtimeDir,
  run,
  runOk,
  which,
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

function pythonCanImport(path) {
  return Boolean(path) && runOk(path, ["-c", "import pptx, PIL"]);
}

function findPreparedPython() {
  const candidates = [
    process.env.AWAKEN_PYTHON,
    ...seedCandidates("python", "bin", "python3"),
    join(runtimeDir, "python", "bin", "python3"),
    which("python3"),
    which("python"),
  ];
  return candidates.find(pythonCanImport) || "";
}

function findBasePython() {
  const candidates = [
    process.env.AWAKEN_BOOTSTRAP_PYTHON,
    which("python3"),
    which("python"),
    ...seedCandidates("python", "bin", "python3"),
  ];
  return candidates.find((candidate) => candidate && runOk(candidate, ["--version"])) || "";
}

function ensurePython() {
  const target = join(runtimeDir, "python", "bin", "python3");
  const prepared = findPreparedPython();
  if (prepared && prepared !== target) {
    writeExecWrapper(target, prepared, "python");
    return { target, source: prepared, mode: "wrapper" };
  }
  if (prepared === target) {
    return { target, source: target, mode: "existing" };
  }

  const basePython = findBasePython();
  if (!basePython) {
    fail("primary-runtime: unable to find Python for bootstrapping");
  }

  const venvDir = join(runtimeDir, "python");
  log(`[prepare-primary-runtime] creating Python venv at ${venvDir}`);
  ensureDir(dirname(venvDir));
  const venv = run(basePython, ["-m", "venv", venvDir], { stdio: "inherit" });
  if (venv.status !== 0) {
    fail("primary-runtime: failed to create Python venv");
  }

  const python = join(venvDir, "bin", "python3");
  const pipInstall = run(
    python,
    ["-m", "pip", "install", "--upgrade", "pip", "python-pptx", "Pillow"],
    { stdio: "inherit" },
  );
  if (pipInstall.status !== 0 || !pythonCanImport(python)) {
    fail("primary-runtime: failed to install python-pptx and Pillow");
  }
  return { target: python, source: basePython, mode: "venv" };
}

function executableCandidates(envName, commandName, ...seedSuffixes) {
  return [
    process.env[envName],
    ...seedCandidates(...seedSuffixes),
    which(commandName),
  ].filter(Boolean);
}

function ensureNode() {
  const source = firstExisting(executableCandidates("AWAKEN_NODE", "node", "node", "bin", "node"));
  if (!source) {
    fail("primary-runtime: unable to find node");
  }
  const target = join(runtimeDir, "node", "bin", "node");
  writeExecWrapper(target, source, "node");
  return { target, source, mode: "wrapper" };
}

function ensureLibreOffice() {
  const candidates = [
    process.env.AWAKEN_SOFFICE,
    ...seedCandidates("bin", "soffice"),
    which("soffice"),
    which("libreoffice"),
    "/Applications/LibreOffice.app/Contents/MacOS/soffice",
    "/opt/homebrew/bin/soffice",
    "/usr/local/bin/soffice",
  ].filter(Boolean);
  const source = firstExisting(candidates);
  if (!source) {
    fail("primary-runtime: unable to find LibreOffice/soffice");
  }
  const target = join(runtimeDir, "libreoffice", "LibreOffice.app", "Contents", "MacOS", "soffice");
  writeExecWrapper(target, source, "soffice");
  return { target, source, mode: "wrapper" };
}

function findPopplerTool(name) {
  return firstExisting([
    process.env.AWAKEN_POPPLER_BIN ? join(process.env.AWAKEN_POPPLER_BIN, name) : "",
    ...seedCandidates("bin", name),
    which(name),
    join("/opt/homebrew/bin", name),
    join("/usr/local/bin", name),
  ]);
}

function ensurePoppler() {
  const tools = ["pdftoppm", "pdfinfo"].map((name) => {
    const source = findPopplerTool(name);
    if (!source) {
      fail(`primary-runtime: unable to find Poppler tool ${name}`);
    }
    const target = join(runtimeDir, "poppler", "bin", name);
    writeExecWrapper(target, source, name);
    return { name, target, source, mode: "wrapper" };
  });
  return tools;
}

function ensureFFmpeg() {
  const source = firstExisting(executableCandidates("AWAKEN_FFMPEG", "ffmpeg", "bin", "ffmpeg"));
  if (!source) {
    fail("primary-runtime: unable to find ffmpeg");
  }
  const target = join(runtimeDir, "ffmpeg", "bin", "ffmpeg");
  writeExecWrapper(target, source, "ffmpeg");
  return { target, source, mode: "wrapper" };
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
