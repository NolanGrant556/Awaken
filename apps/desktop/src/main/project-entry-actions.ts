import { spawn } from "node:child_process";
import fs from "node:fs/promises";
import path from "node:path";
import type {
  ProjectEntryActionResult,
  ProjectEntryKind,
  ProjectEntryOpenApplication,
} from "../shared/runtime.js";

const APPLICATION_LABELS: Record<ProjectEntryOpenApplication, string> = {
  wps: "WPS Office",
  office: "Microsoft Office",
  vscode: "VS Code",
  cursor: "Cursor",
  trae: "TRAE",
  xcode: "Xcode",
};

const APPLICATION_BUNDLE_IDS: Record<
  Exclude<ProjectEntryOpenApplication, "office">,
  readonly string[]
> = {
  wps: ["com.kingsoft.wpsoffice.mac"],
  vscode: ["com.microsoft.VSCode"],
  cursor: ["com.todesktop.230313mzl4w4u92"],
  trae: ["cn.trae.app", "cn.trae.solo.app"],
  xcode: ["com.apple.dt.Xcode"],
};

const WORD_EXTENSIONS = new Set([
  ".doc",
  ".docm",
  ".docx",
  ".dot",
  ".dotm",
  ".dotx",
  ".odt",
  ".rtf",
]);
const EXCEL_EXTENSIONS = new Set([".csv", ".ods", ".tsv", ".xls", ".xlsb", ".xlsm", ".xlsx"]);
const POWERPOINT_EXTENSIONS = new Set([".odp", ".ppt", ".pptm", ".pptx"]);
const PDF_EXTENSIONS = new Set([".pdf"]);

const WINDOWS_DEVELOPMENT_EXECUTABLES: Record<
  "vscode" | "cursor" | "trae",
  readonly string[]
> = {
  vscode: ["Code.exe"],
  cursor: ["Cursor.exe"],
  trae: ["TRAE.exe", "Trae.exe"],
};

const WINDOWS_SEARCH_DIRECTORY_LIMIT = 500;
const WINDOWS_SEARCH_DEPTH_LIMIT = 5;

type OpenProjectEntryDependencies = {
  openMacApplication: (bundleId: string, absolutePath: string) => Promise<boolean>;
  resolveWindowsExecutable: (
    application: ProjectEntryOpenApplication,
    executableNames: readonly string[],
  ) => Promise<string | undefined>;
  launchWindowsApplication: (executablePath: string, absolutePath: string) => Promise<boolean>;
};

function entryExtension(absolutePath: string): string {
  return path.win32.extname(absolutePath).toLowerCase();
}

export function projectEntryOpenBundleIds(
  application: ProjectEntryOpenApplication,
  absolutePath: string,
  kind: ProjectEntryKind,
): readonly string[] {
  if (kind === "directory" && (application === "wps" || application === "office")) {
    return [];
  }
  if (application !== "office") {
    return APPLICATION_BUNDLE_IDS[application];
  }
  const extension = entryExtension(absolutePath);
  if (WORD_EXTENSIONS.has(extension)) return ["com.microsoft.Word"];
  if (EXCEL_EXTENSIONS.has(extension)) return ["com.microsoft.Excel"];
  if (POWERPOINT_EXTENSIONS.has(extension)) return ["com.microsoft.Powerpoint"];
  return [];
}

export function projectEntryOpenWindowsExecutableNames(
  application: ProjectEntryOpenApplication,
  absolutePath: string,
  kind: ProjectEntryKind,
): readonly string[] {
  if (kind === "directory" && (application === "wps" || application === "office")) {
    return [];
  }
  if (application === "xcode") return [];
  if (application === "office") {
    const extension = entryExtension(absolutePath);
    if (WORD_EXTENSIONS.has(extension)) return ["WINWORD.EXE"];
    if (EXCEL_EXTENSIONS.has(extension)) return ["EXCEL.EXE"];
    if (POWERPOINT_EXTENSIONS.has(extension)) return ["POWERPNT.EXE"];
    return [];
  }
  if (application === "wps") {
    const extension = entryExtension(absolutePath);
    if (EXCEL_EXTENSIONS.has(extension)) return ["et.exe"];
    if (POWERPOINT_EXTENSIONS.has(extension)) return ["wpp.exe"];
    if (PDF_EXTENSIONS.has(extension)) return ["wpspdf.exe", "wps.exe"];
    return ["wps.exe"];
  }
  return WINDOWS_DEVELOPMENT_EXECUTABLES[application];
}

function openWithBundleId(bundleId: string, absolutePath: string): Promise<boolean> {
  return new Promise((resolve, reject) => {
    const child = spawn("/usr/bin/open", ["-b", bundleId, absolutePath], {
      stdio: "ignore",
    });
    child.once("error", reject);
    child.once("close", (code) => resolve(code === 0));
  });
}

function launchWindowsApplication(
  executablePath: string,
  absolutePath: string,
): Promise<boolean> {
  return new Promise((resolve) => {
    const child = spawn(executablePath, [absolutePath], {
      detached: true,
      stdio: "ignore",
      windowsHide: true,
    });
    let settled = false;
    const finish = (opened: boolean): void => {
      if (settled) return;
      settled = true;
      resolve(opened);
    };
    child.once("error", () => finish(false));
    child.once("spawn", () => {
      child.unref();
      finish(true);
    });
  });
}

function uniqueNonEmpty(values: Array<string | undefined>): string[] {
  return [...new Set(values.filter((value): value is string => Boolean(value)))];
}

function windowsApplicationSearchRoots(
  application: ProjectEntryOpenApplication,
  environment: NodeJS.ProcessEnv,
): string[] {
  const localAppData = environment.LOCALAPPDATA;
  const programFiles = environment.ProgramW6432 ?? environment.ProgramFiles;
  const programFilesX86 = environment["ProgramFiles(x86)"];
  switch (application) {
    case "wps":
      return uniqueNonEmpty([
        localAppData && path.win32.join(localAppData, "Kingsoft", "WPS Office"),
        programFiles && path.win32.join(programFiles, "Kingsoft", "WPS Office"),
        programFilesX86 && path.win32.join(programFilesX86, "Kingsoft", "WPS Office"),
      ]);
    case "office":
      return uniqueNonEmpty([
        programFiles && path.win32.join(programFiles, "Microsoft Office"),
        programFilesX86 && path.win32.join(programFilesX86, "Microsoft Office"),
      ]);
    case "vscode":
      return uniqueNonEmpty([
        localAppData && path.win32.join(localAppData, "Programs", "Microsoft VS Code"),
        programFiles && path.win32.join(programFiles, "Microsoft VS Code"),
        programFilesX86 && path.win32.join(programFilesX86, "Microsoft VS Code"),
      ]);
    case "cursor":
      return uniqueNonEmpty([
        localAppData && path.win32.join(localAppData, "Programs", "cursor"),
        localAppData && path.win32.join(localAppData, "Programs", "Cursor"),
        programFiles && path.win32.join(programFiles, "Cursor"),
        programFilesX86 && path.win32.join(programFilesX86, "Cursor"),
      ]);
    case "trae":
      return uniqueNonEmpty([
        localAppData && path.win32.join(localAppData, "Programs", "TRAE"),
        localAppData && path.win32.join(localAppData, "Programs", "Trae"),
        programFiles && path.win32.join(programFiles, "TRAE"),
        programFilesX86 && path.win32.join(programFilesX86, "TRAE"),
      ]);
    case "xcode":
      return [];
  }
}

function windowsDirectExecutableCandidates(
  application: ProjectEntryOpenApplication,
  executableNames: readonly string[],
  environment: NodeJS.ProcessEnv,
): string[] {
  const roots = windowsApplicationSearchRoots(application, environment);
  const candidates: string[] = [];
  for (const root of roots) {
    for (const executableName of executableNames) {
      candidates.push(path.win32.join(root, executableName));
      if (application === "wps") {
        candidates.push(path.win32.join(root, "office6", executableName));
      } else if (application === "office") {
        candidates.push(path.win32.join(root, "root", "Office16", executableName));
        candidates.push(path.win32.join(root, "Office16", executableName));
        candidates.push(path.win32.join(root, "Office15", executableName));
      }
    }
  }
  const pathValue = environment.Path ?? environment.PATH ?? "";
  for (const directory of pathValue.split(path.win32.delimiter)) {
    const normalizedDirectory = directory.trim().replace(/^"|"$/g, "");
    if (!normalizedDirectory) continue;
    for (const executableName of executableNames) {
      candidates.push(path.win32.join(normalizedDirectory, executableName));
    }
  }
  return uniqueNonEmpty(candidates);
}

async function isFile(candidate: string): Promise<boolean> {
  try {
    return (await fs.stat(candidate)).isFile();
  } catch {
    return false;
  }
}

function hasExpectedExecutableName(
  candidate: string,
  executableNames: readonly string[],
): boolean {
  const candidateName = path.win32.basename(candidate).toLowerCase();
  return executableNames.some((name) => name.toLowerCase() === candidateName);
}

function expandWindowsEnvironmentVariables(
  value: string,
  environment: NodeJS.ProcessEnv,
): string {
  const normalizedEnvironment = new Map(
    Object.entries(environment).map(([key, entry]) => [key.toUpperCase(), entry]),
  );
  return value.replace(/%([^%]+)%/g, (match, key: string) => {
    return normalizedEnvironment.get(key.toUpperCase()) ?? match;
  });
}

export function windowsRegistryExecutablePath(
  output: string,
  environment: NodeJS.ProcessEnv = process.env,
): string | undefined {
  const match = output.match(/REG_(?:EXPAND_)?SZ\s+([^\r\n]+)/i);
  if (!match?.[1]) return undefined;
  const expanded = expandWindowsEnvironmentVariables(match[1].trim(), environment);
  if (expanded.startsWith('"')) {
    const closingQuote = expanded.indexOf('"', 1);
    return closingQuote > 1 ? expanded.slice(1, closingQuote) : undefined;
  }
  const executableMatch = expanded.match(/^(.+?\.exe)(?:\s|$)/i);
  return executableMatch?.[1]?.trim();
}

function queryWindowsRegistry(
  executable: string,
  args: readonly string[],
): Promise<string | undefined> {
  return new Promise((resolve) => {
    const child = spawn(executable, [...args], {
      stdio: ["ignore", "pipe", "ignore"],
      windowsHide: true,
    });
    let output = "";
    child.stdout?.setEncoding("utf8");
    child.stdout?.on("data", (chunk: string) => {
      output += chunk;
    });
    child.once("error", () => resolve(undefined));
    child.once("close", (code) => resolve(code === 0 ? output : undefined));
  });
}

async function registeredWindowsExecutable(
  executableNames: readonly string[],
  environment: NodeJS.ProcessEnv,
): Promise<string | undefined> {
  const systemRoot = environment.SystemRoot ?? environment.WINDIR;
  const registryExecutable = systemRoot
    ? path.win32.join(systemRoot, "System32", "reg.exe")
    : "reg.exe";
  for (const executableName of executableNames) {
    const registryKeySuffix = `Software\\Microsoft\\Windows\\CurrentVersion\\App Paths\\${executableName}`;
    const queries: ReadonlyArray<readonly string[]> = [
      ["query", `HKCU\\${registryKeySuffix}`, "/ve"],
      ["query", `HKLM\\${registryKeySuffix}`, "/ve", "/reg:64"],
      ["query", `HKLM\\${registryKeySuffix}`, "/ve", "/reg:32"],
    ];
    for (const args of queries) {
      const output = await queryWindowsRegistry(registryExecutable, args);
      const candidate = output
        ? windowsRegistryExecutablePath(output, environment)
        : undefined;
      if (
        candidate &&
        hasExpectedExecutableName(candidate, executableNames) &&
        (await isFile(candidate))
      ) {
        return candidate;
      }
    }
  }
  return undefined;
}

async function findExecutableUnderWindowsRoots(
  executableNames: readonly string[],
  roots: readonly string[],
): Promise<string | undefined> {
  const targetNames = new Set(executableNames.map((name) => name.toLowerCase()));
  for (const root of roots) {
    const queue: Array<{ directory: string; depth: number }> = [{ directory: root, depth: 0 }];
    let inspectedDirectories = 0;
    while (queue.length > 0 && inspectedDirectories < WINDOWS_SEARCH_DIRECTORY_LIMIT) {
      const current = queue.shift();
      if (!current) break;
      inspectedDirectories += 1;
      let entries;
      try {
        entries = await fs.readdir(current.directory, { withFileTypes: true });
      } catch {
        continue;
      }
      entries.sort((left, right) =>
        right.name.localeCompare(left.name, undefined, { numeric: true }),
      );
      for (const entry of entries) {
        const candidate = path.win32.join(current.directory, entry.name);
        if (entry.isFile() && targetNames.has(entry.name.toLowerCase())) return candidate;
        if (
          entry.isDirectory() &&
          !entry.isSymbolicLink() &&
          current.depth < WINDOWS_SEARCH_DEPTH_LIMIT
        ) {
          queue.push({ directory: candidate, depth: current.depth + 1 });
        }
      }
    }
  }
  return undefined;
}

async function resolveWindowsExecutable(
  application: ProjectEntryOpenApplication,
  executableNames: readonly string[],
  environment: NodeJS.ProcessEnv = process.env,
): Promise<string | undefined> {
  const registered = await registeredWindowsExecutable(executableNames, environment);
  if (registered) return registered;
  for (const candidate of windowsDirectExecutableCandidates(
    application,
    executableNames,
    environment,
  )) {
    if (await isFile(candidate)) return candidate;
  }
  return findExecutableUnderWindowsRoots(
    executableNames,
    windowsApplicationSearchRoots(application, environment),
  );
}

function unsupportedApplicationMessage(
  application: ProjectEntryOpenApplication,
  platform: NodeJS.Platform,
): string {
  if (platform === "win32" && application === "xcode") {
    return "Xcode 不支持 Windows。";
  }
  if (application === "office") {
    return "这个条目没有匹配的 Microsoft Office 打开方式。";
  }
  return "WPS 和 Microsoft Office 只用于支持的文件。";
}

export async function openProjectEntryWithApplication(
  application: ProjectEntryOpenApplication,
  absolutePath: string,
  kind: ProjectEntryKind,
  platform: NodeJS.Platform = process.platform,
  dependencies: Partial<OpenProjectEntryDependencies> = {},
): Promise<ProjectEntryActionResult> {
  const label = APPLICATION_LABELS[application];
  const openMacApplication = dependencies.openMacApplication ?? openWithBundleId;
  const resolveExecutable = dependencies.resolveWindowsExecutable ?? resolveWindowsExecutable;
  const launchExecutable = dependencies.launchWindowsApplication ?? launchWindowsApplication;

  if (platform === "darwin") {
    const bundleIds = projectEntryOpenBundleIds(application, absolutePath, kind);
    if (bundleIds.length === 0) {
      return {
        ok: false,
        code: "UNSUPPORTED_APPLICATION",
        message: unsupportedApplicationMessage(application, platform),
      };
    }
    try {
      for (const bundleId of bundleIds) {
        if (await openMacApplication(bundleId, absolutePath)) return { ok: true };
      }
      return {
        ok: false,
        code: "APPLICATION_UNAVAILABLE",
        message: `未找到 ${label}，请先安装后重试。`,
      };
    } catch {
      return {
        ok: false,
        code: "APPLICATION_OPEN_FAILED",
        message: `暂时无法用 ${label} 打开这个条目。`,
      };
    }
  }

  if (platform === "win32") {
    const executableNames = projectEntryOpenWindowsExecutableNames(
      application,
      absolutePath,
      kind,
    );
    if (executableNames.length === 0) {
      return {
        ok: false,
        code: "UNSUPPORTED_APPLICATION",
        message: unsupportedApplicationMessage(application, platform),
      };
    }
    try {
      const executablePath = await resolveExecutable(application, executableNames);
      if (!executablePath) {
        return {
          ok: false,
          code: "APPLICATION_UNAVAILABLE",
          message: `未找到 ${label}，请先安装后重试。`,
        };
      }
      if (await launchExecutable(executablePath, absolutePath)) return { ok: true };
      return {
        ok: false,
        code: "APPLICATION_OPEN_FAILED",
        message: `暂时无法用 ${label} 打开这个条目。`,
      };
    } catch {
      return {
        ok: false,
        code: "APPLICATION_OPEN_FAILED",
        message: `暂时无法用 ${label} 打开这个条目。`,
      };
    }
  }

  return {
    ok: false,
    code: "UNSUPPORTED_PLATFORM",
    message: "指定应用打开目前支持 macOS 和 Windows。",
  };
}
