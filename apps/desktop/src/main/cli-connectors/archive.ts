import fs from "node:fs";
import fsp from "node:fs/promises";
import path from "node:path";
import { pipeline } from "node:stream/promises";
import { x as extractTar } from "tar";
import yauzl from "yauzl";

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
