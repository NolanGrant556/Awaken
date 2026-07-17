import fs from "node:fs";
import fsp from "node:fs/promises";
import path from "node:path";
import { pipeline } from "node:stream/promises";
import { x as extractTar } from "tar";
import yauzl from "yauzl";

export type SkillBundleLayout = {
  archiveRoot: string;
  destinationRoot?: string;
  skillNames: string[];
};

const maxSkillFiles = 2_000;
const maxSkillBytes = 50 * 1024 * 1024;

export function safeArchivePath(value: string): string {
  const normalized = value.replaceAll("\\", "/");
  if (!normalized || normalized.startsWith("/") || normalized.includes("\0")) {
    throw new Error("CLI_ARCHIVE_UNSAFE_PATH");
  }
  const trimmed = normalized.replace(/\/+$/, "");
  if (!trimmed) throw new Error("CLI_ARCHIVE_UNSAFE_PATH");
  const parts = trimmed.split("/");
  if (parts.some((part) => part === "" || part === "." || part === "..")) {
    throw new Error("CLI_ARCHIVE_UNSAFE_PATH");
  }
  return trimmed;
}

export async function extractSingleExecutable(
  archivePath: string,
  archiveType: "tar.gz" | "zip",
  expectedArchivePath: string,
  destinationPath: string,
): Promise<void> {
  const expected = safeArchivePath(expectedArchivePath);
  await fsp.mkdir(path.dirname(destinationPath), { recursive: true, mode: 0o700 });
  if (archiveType === "tar.gz") {
    let found = 0;
    let unsafeEntry = false;
    await extractTar({
      file: archivePath,
      cwd: path.dirname(destinationPath),
      gzip: true,
      strict: true,
      filter: (entryPath, entry) => {
        const safe = safeArchivePath(entryPath);
        if ("type" in entry && (entry.type === "SymbolicLink" || entry.type === "Link")) {
          unsafeEntry = true;
          return false;
        }
        if (safe !== expected) return false;
        if (!("type" in entry) || (entry.type !== "File" && entry.type !== "OldFile" && entry.type !== "ContiguousFile")) {
          throw new Error("CLI_ARCHIVE_UNSAFE_ENTRY");
        }
        found += 1;
        return true;
      },
    });
    if (unsafeEntry) throw new Error("CLI_ARCHIVE_UNSAFE_ENTRY");
    if (found !== 1) throw new Error("CLI_ARCHIVE_EXECUTABLE_MISSING");
    const extractedPath = path.join(path.dirname(destinationPath), ...expected.split("/"));
    if (extractedPath !== destinationPath) {
      await fsp.rename(extractedPath, destinationPath);
      const topLevelPath = path.join(path.dirname(destinationPath), expected.split("/")[0]);
      if (topLevelPath !== destinationPath) await fsp.rm(topLevelPath, { recursive: true, force: true });
    }
    return;
  }
  await extractZipExecutable(archivePath, expected, destinationPath);
}

export async function extractSkillBundle(
  archivePath: string,
  archiveType: "tar.gz" | "zip",
  layout: SkillBundleLayout,
  destinationRoot: string,
): Promise<void> {
  const archiveRoot = layout.archiveRoot ? safeArchivePath(layout.archiveRoot) : "";
  const destinationName = layout.destinationRoot
    ? validateSkillName(layout.destinationRoot)
    : undefined;
  const skillNames = [...new Set(layout.skillNames.map(validateSkillName))];
  if (skillNames.length === 0 || (destinationName && (skillNames.length !== 1 || skillNames[0] !== destinationName))) {
    throw new Error("CLI_SKILL_BUNDLE_INVALID_LAYOUT");
  }

  await fsp.mkdir(destinationRoot, { recursive: true, mode: 0o700 });
  const extractionRoot = destinationName ? path.join(destinationRoot, destinationName) : destinationRoot;
  await fsp.mkdir(extractionRoot, { recursive: true, mode: 0o700 });
  const selected = (entryPath: string): string | undefined => {
    const safe = safeArchivePath(entryPath);
    const relative = archiveRoot
      ? safe === archiveRoot
        ? ""
        : safe.startsWith(`${archiveRoot}/`)
          ? safe.slice(archiveRoot.length + 1)
          : undefined
      : safe;
    if (!relative) return undefined;
    if (!destinationName && !skillNames.includes(relative.split("/")[0])) return undefined;
    return relative;
  };

  if (archiveType === "tar.gz") {
    let fileCount = 0;
    let totalBytes = 0;
    await extractTar({
      file: archivePath,
      cwd: extractionRoot,
      gzip: true,
      strict: true,
      strip: archiveRoot ? archiveRoot.split("/").length : 0,
      filter: (entryPath, entry) => {
        const relative = selected(entryPath);
        if (!relative) return false;
        const entryType = "type" in entry ? entry.type : undefined;
        if (entryType === "SymbolicLink" || entryType === "Link") {
          throw new Error("CLI_ARCHIVE_UNSAFE_ENTRY");
        }
        if (entryType === "Directory") return true;
        if (entryType !== "File" && entryType !== "OldFile" && entryType !== "ContiguousFile") {
          throw new Error("CLI_ARCHIVE_UNSAFE_ENTRY");
        }
        fileCount += 1;
        totalBytes += entry.size;
        if (fileCount > maxSkillFiles || totalBytes > maxSkillBytes) {
          throw new Error("CLI_SKILL_BUNDLE_TOO_LARGE");
        }
        return true;
      },
    });
  } else {
    await extractZipSkillBundle(archivePath, extractionRoot, selected);
  }

  for (const skillName of skillNames) {
    const skillFile = path.join(destinationRoot, skillName, "SKILL.md");
    const stat = await fsp.lstat(skillFile).catch(() => undefined);
    if (!stat?.isFile() || stat.isSymbolicLink()) throw new Error("CLI_SKILL_BUNDLE_MISSING_SKILL");
  }
}

async function extractZipExecutable(
  archivePath: string,
  expected: string,
  destinationPath: string,
): Promise<void> {
  const zip = await new Promise<yauzl.ZipFile>((resolve, reject) => {
    yauzl.open(archivePath, { lazyEntries: true, autoClose: false }, (error, value) => {
      if (error || !value) reject(error ?? new Error("CLI_ARCHIVE_OPEN_FAILED"));
      else resolve(value);
    });
  });
  try {
    let found = 0;
    await new Promise<void>((resolve, reject) => {
      zip.on("error", reject);
      zip.on("end", () => found === 1 ? resolve() : reject(new Error("CLI_ARCHIVE_EXECUTABLE_MISSING")));
      zip.on("entry", (entry) => {
        let safe: string;
        try {
          safe = safeArchivePath(entry.fileName);
        } catch (error) {
          reject(error);
          return;
        }
        const unixMode = (entry.externalFileAttributes >>> 16) & 0xffff;
        const fileType = unixMode & 0o170000;
        if (fileType === 0o120000) {
          reject(new Error("CLI_ARCHIVE_UNSAFE_ENTRY"));
          return;
        }
        if (safe !== expected) {
          zip.readEntry();
          return;
        }
        found += 1;
        if (found > 1) {
          reject(new Error("CLI_ARCHIVE_DUPLICATE_EXECUTABLE"));
          return;
        }
        zip.openReadStream(entry, (error, stream) => {
          if (error || !stream) {
            reject(error ?? new Error("CLI_ARCHIVE_READ_FAILED"));
            return;
          }
          void pipeline(stream, fs.createWriteStream(destinationPath, { mode: 0o700 }))
            .then(() => zip.readEntry(), reject);
        });
      });
      zip.readEntry();
    });
  } finally {
    zip.close();
  }
}

async function extractZipSkillBundle(
  archivePath: string,
  destinationRoot: string,
  selected: (entryPath: string) => string | undefined,
): Promise<void> {
  const zip = await new Promise<yauzl.ZipFile>((resolve, reject) => {
    yauzl.open(archivePath, { lazyEntries: true, autoClose: false }, (error, value) => {
      if (error || !value) reject(error ?? new Error("CLI_ARCHIVE_OPEN_FAILED"));
      else resolve(value);
    });
  });
  let fileCount = 0;
  let totalBytes = 0;
  try {
    await new Promise<void>((resolve, reject) => {
      zip.on("error", reject);
      zip.on("end", resolve);
      zip.on("entry", (entry) => {
        let relative: string | undefined;
        try {
          relative = selected(entry.fileName);
        } catch (error) {
          reject(error);
          return;
        }
        if (!relative) {
          zip.readEntry();
          return;
        }
        const unixMode = (entry.externalFileAttributes >>> 16) & 0xffff;
        const fileType = unixMode & 0o170000;
        if (fileType === 0o120000) {
          reject(new Error("CLI_ARCHIVE_UNSAFE_ENTRY"));
          return;
        }
        const destination = path.join(destinationRoot, ...relative.split("/"));
        if (entry.fileName.endsWith("/")) {
          void fsp.mkdir(destination, { recursive: true, mode: 0o700 }).then(() => zip.readEntry(), reject);
          return;
        }
        if (fileType !== 0 && fileType !== 0o100000) {
          reject(new Error("CLI_ARCHIVE_UNSAFE_ENTRY"));
          return;
        }
        fileCount += 1;
        totalBytes += entry.uncompressedSize;
        if (fileCount > maxSkillFiles || totalBytes > maxSkillBytes) {
          reject(new Error("CLI_SKILL_BUNDLE_TOO_LARGE"));
          return;
        }
        void fsp.mkdir(path.dirname(destination), { recursive: true, mode: 0o700 }).then(() => {
          zip.openReadStream(entry, (error, stream) => {
            if (error || !stream) {
              reject(error ?? new Error("CLI_ARCHIVE_READ_FAILED"));
              return;
            }
            const mode = unixMode & 0o111 ? 0o700 : 0o600;
            void pipeline(stream, fs.createWriteStream(destination, { mode }))
              .then(() => zip.readEntry(), reject);
          });
        }, reject);
      });
      zip.readEntry();
    });
  } finally {
    zip.close();
  }
}

function validateSkillName(value: string): string {
  if (!/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(value)) throw new Error("CLI_SKILL_BUNDLE_INVALID_NAME");
  return value;
}
