import path from "node:path";
import type { ProjectFilePreviewKind } from "../shared/runtime.js";

export const MAX_TEXT_PREVIEW_BYTES = 2 * 1024 * 1024;
export const MAX_HTML_TEXT_RESOURCE_BYTES = 10 * 1024 * 1024;
export const MAX_OFFICE_SOURCE_BYTES = 100 * 1024 * 1024;
export const OFFICE_RENDER_TIMEOUT_MS = 120_000;
export const MAX_OFFICE_HTML_BYTES = 250 * 1024 * 1024;
export const MAX_PDF_BYTES = 500 * 1024 * 1024;
export const MAX_IMAGE_BYTES = 100 * 1024 * 1024;
export const MAX_IMAGE_PIXELS = 64_000_000;
export const MAX_PREVIEW_REQUESTS = 512;
export const MAX_PREVIEW_INFLIGHT_REQUESTS = 24;
export const MAX_PREVIEW_RESPONSE_BYTES = 512 * 1024 * 1024;
export const MAX_OFFICE_STDOUT_BYTES = 1024 * 1024;
export const MAX_OFFICE_STDERR_BYTES = 1024 * 1024;
export const PREVIEW_CACHE_MAX_BYTES = 1024 * 1024 * 1024;

const officeExtensions = new Set(["docx", "xlsx", "pptx"]);
const markdownExtensions = new Set(["md", "markdown"]);
const pdfExtensions = new Set(["pdf"]);
const imageExtensions = new Set([
  "bmp",
  "gif",
  "jpeg",
  "jpg",
  "png",
  "svg",
  "webp",
]);
const htmlExtensions = new Set(["html", "htm"]);
const codeExtensions = new Set([
  "astro",
  "bash",
  "c",
  "cc",
  "cfg",
  "conf",
  "cpp",
  "cs",
  "css",
  "csv",
  "cxx",
  "env.example",
  "fish",
  "go",
  "graphql",
  "h",
  "hpp",
  "ini",
  "java",
  "js",
  "json",
  "json5",
  "jsonc",
  "jsx",
  "kt",
  "kts",
  "less",
  "log",
  "lua",
  "mjs",
  "ndjson",
  "php",
  "properties",
  "py",
  "rb",
  "rs",
  "sass",
  "scss",
  "sh",
  "sql",
  "svelte",
  "swift",
  "text",
  "toml",
  "ts",
  "tsx",
  "tsv",
  "txt",
  "vue",
  "xml",
  "yaml",
  "yml",
  "zsh",
]);
const extensionlessCodeFiles = new Set([
  "dockerfile",
  "gemfile",
  "justfile",
  "makefile",
  "procfile",
  "rakefile",
]);

export type PreviewFileClassification = {
  kind: ProjectFilePreviewKind;
  extension: string;
  language?: string;
};

export function normalizedFileExtension(fileName: string): string {
  const normalizedName = fileName.toLocaleLowerCase();
  if (normalizedName.endsWith(".env.example")) return "env.example";
  const extension = path.extname(normalizedName).slice(1);
  return extension;
}

export function classifyPreviewFile(
  fileName: string,
): PreviewFileClassification | undefined {
  const extension = normalizedFileExtension(fileName);
  const normalizedName = fileName.toLocaleLowerCase();
  if (officeExtensions.has(extension)) return { kind: "office", extension };
  if (markdownExtensions.has(extension)) {
    return { kind: "markdown", extension, language: "markdown" };
  }
  if (pdfExtensions.has(extension)) return { kind: "pdf", extension };
  if (imageExtensions.has(extension)) return { kind: "image", extension };
  if (htmlExtensions.has(extension)) return { kind: "html", extension };
  if (codeExtensions.has(extension) || extensionlessCodeFiles.has(normalizedName)) {
    return {
      kind: "code",
      extension,
      language: languageNameForFile(fileName),
    };
  }
  return undefined;
}

export function previewByteLimit(kind: ProjectFilePreviewKind): number {
  switch (kind) {
    case "markdown":
    case "code":
      return MAX_TEXT_PREVIEW_BYTES;
    case "office":
      return MAX_OFFICE_SOURCE_BYTES;
    case "pdf":
      return MAX_PDF_BYTES;
    case "image":
      return MAX_IMAGE_BYTES;
    case "html":
      return MAX_HTML_TEXT_RESOURCE_BYTES;
  }
}

export function languageNameForFile(fileName: string): string {
  const extension = normalizedFileExtension(fileName);
  const normalizedName = fileName.toLocaleLowerCase();
  if (extensionlessCodeFiles.has(normalizedName)) return normalizedName;
  switch (extension) {
    case "mjs":
    case "cjs":
    case "jsx":
      return "javascript";
    case "ts":
    case "tsx":
    case "mts":
    case "cts":
      return "typescript";
    case "yml":
      return "yaml";
    case "sh":
    case "bash":
    case "zsh":
    case "fish":
      return "shell";
    case "txt":
    case "text":
    case "log":
      return "text";
    default:
      return extension || "text";
  }
}

const mimeTypes: Record<string, string> = {
  bmp: "image/bmp",
  css: "text/css; charset=utf-8",
  gif: "image/gif",
  htm: "text/html; charset=utf-8",
  html: "text/html; charset=utf-8",
  ico: "image/x-icon",
  jpeg: "image/jpeg",
  jpg: "image/jpeg",
  js: "text/javascript; charset=utf-8",
  json: "application/json; charset=utf-8",
  mjs: "text/javascript; charset=utf-8",
  pdf: "application/pdf",
  png: "image/png",
  svg: "image/svg+xml",
  txt: "text/plain; charset=utf-8",
  webp: "image/webp",
  xml: "application/xml; charset=utf-8",
};

export function mimeTypeForFile(fileName: string): string {
  return mimeTypes[normalizedFileExtension(fileName)] ?? "application/octet-stream";
}

export type ParsedByteRange = { start: number; end: number };

export function parseSingleByteRange(
  value: string | null,
  fileSize: number,
): ParsedByteRange | "unsatisfiable" | undefined {
  if (!value) return undefined;
  const match = /^bytes=(\d*)-(\d*)$/.exec(value.trim());
  if (!match || fileSize <= 0) return "unsatisfiable";
  const [, rawStart, rawEnd] = match;
  if (!rawStart && !rawEnd) return "unsatisfiable";

  if (!rawStart) {
    const suffixLength = Number(rawEnd);
    if (!Number.isSafeInteger(suffixLength) || suffixLength <= 0) {
      return "unsatisfiable";
    }
    return {
      start: Math.max(0, fileSize - suffixLength),
      end: fileSize - 1,
    };
  }

  const start = Number(rawStart);
  const requestedEnd = rawEnd ? Number(rawEnd) : fileSize - 1;
  if (
    !Number.isSafeInteger(start) ||
    !Number.isSafeInteger(requestedEnd) ||
    start < 0 ||
    requestedEnd < start ||
    start >= fileSize
  ) {
    return "unsatisfiable";
  }
  return { start, end: Math.min(requestedEnd, fileSize - 1) };
}

export function safePreviewPathname(pathname: string): string[] | undefined {
  if (/%2f|%5c/i.test(pathname)) return undefined;
  let decoded: string;
  try {
    decoded = decodeURIComponent(pathname);
  } catch {
    return undefined;
  }
  if (decoded.includes("\0") || decoded.includes("\\")) return undefined;
  const parts = decoded.split("/").filter(Boolean);
  if (parts.some((part) => part === "." || part === "..")) return undefined;
  return parts;
}

export type RasterDimensions = { width: number; height: number };

function uint24LittleEndian(buffer: Buffer, offset: number): number {
  return buffer[offset] | (buffer[offset + 1] << 8) | (buffer[offset + 2] << 16);
}

export function rasterDimensions(
  extension: string,
  buffer: Buffer,
): RasterDimensions | undefined {
  if (extension === "png" && buffer.length >= 24) {
    return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) };
  }
  if (extension === "gif" && buffer.length >= 10) {
    return { width: buffer.readUInt16LE(6), height: buffer.readUInt16LE(8) };
  }
  if (extension === "bmp" && buffer.length >= 26) {
    return {
      width: Math.abs(buffer.readInt32LE(18)),
      height: Math.abs(buffer.readInt32LE(22)),
    };
  }
  if ((extension === "jpg" || extension === "jpeg") && buffer.length >= 4) {
    let offset = 2;
    while (offset + 9 < buffer.length) {
      if (buffer[offset] !== 0xff) {
        offset += 1;
        continue;
      }
      const marker = buffer[offset + 1];
      if (marker === 0xd8 || marker === 0xd9) {
        offset += 2;
        continue;
      }
      const length = buffer.readUInt16BE(offset + 2);
      if (length < 2 || offset + length + 2 > buffer.length) break;
      if (
        (marker >= 0xc0 && marker <= 0xc3) ||
        (marker >= 0xc5 && marker <= 0xc7) ||
        (marker >= 0xc9 && marker <= 0xcb) ||
        (marker >= 0xcd && marker <= 0xcf)
      ) {
        return {
          width: buffer.readUInt16BE(offset + 7),
          height: buffer.readUInt16BE(offset + 5),
        };
      }
      offset += length + 2;
    }
  }
  if (extension === "webp" && buffer.length >= 30) {
    const chunk = buffer.toString("ascii", 12, 16);
    if (chunk === "VP8X") {
      return {
        width: uint24LittleEndian(buffer, 24) + 1,
        height: uint24LittleEndian(buffer, 27) + 1,
      };
    }
    if (chunk === "VP8 " && buffer.length >= 30) {
      return {
        width: buffer.readUInt16LE(26) & 0x3fff,
        height: buffer.readUInt16LE(28) & 0x3fff,
      };
    }
    if (chunk === "VP8L" && buffer.length >= 25) {
      const bits = buffer.readUInt32LE(21);
      return {
        width: (bits & 0x3fff) + 1,
        height: ((bits >> 14) & 0x3fff) + 1,
      };
    }
  }
  return undefined;
}
