import fs from "node:fs/promises";
import path from "node:path";
import {
  PET_CELL_HEIGHT,
  PET_CELL_WIDTH,
  PET_SHEET_COLUMNS,
  PET_SHEET_WIDTH,
  petSheetHeightForVersion,
  type PetSpriteVersion,
  type PetSource,
} from "../../shared/pets.js";

const MAX_MANIFEST_BYTES = 64 * 1024;
const MAX_SPRITESHEET_BYTES = 16 * 1024 * 1024;
const MAX_DISPLAY_NAME_LENGTH = 64;
const MAX_DESCRIPTION_LENGTH = 256;

export type PetManifestInput = {
  id: string;
  sourceId: string;
  displayName: string;
  description: string;
  spriteVersionNumber: PetSpriteVersion;
  spritesheetPath: string;
};

export type ValidationResult<T> =
  | { valid: true; value: T; warnings: string[] }
  | { valid: false; error: string; warnings?: string[] };

export function validatePetManifestJson(
  value: unknown,
  folderName: string,
): ValidationResult<PetManifestInput> {
  const warnings: string[] = [];

  if (!value || typeof value !== "object") {
    return { valid: false, error: "pet.json 不是有效 JSON 对象" };
  }

  const candidate = value as Record<string, unknown>;

  for (const key of ["id", "displayName", "description", "spritesheetPath"]) {
    if (typeof candidate[key] !== "string" || candidate[key] === "") {
      return { valid: false, error: `缺少必填字段 ${key}` };
    }
  }

  const id = (candidate.id as string).trim();
  const displayName = (candidate.displayName as string).trim();
  const description = (candidate.description as string).trim();
  const spritesheetPath = (candidate.spritesheetPath as string).trim();
  const spriteVersionNumber =
    candidate.spriteVersionNumber === undefined
      ? 1
      : candidate.spriteVersionNumber;

  if (!displayName || !description || !spritesheetPath) {
    return { valid: false, error: "pet.json 必填字段不能只包含空白字符" };
  }

  if (!/^[a-z0-9-]+$/.test(id)) {
    return { valid: false, error: "id 只能包含小写字母、数字和连字符" };
  }

  if (spriteVersionNumber !== 1 && spriteVersionNumber !== 2) {
    return { valid: false, error: "spriteVersionNumber only supports 1 or 2" };
  }

  if (displayName.length > MAX_DISPLAY_NAME_LENGTH) {
    warnings.push("displayName 过长");
  }
  if (description.length > MAX_DESCRIPTION_LENGTH) {
    warnings.push("description 过长");
  }

  if (folderName !== id) {
    warnings.push(`文件夹名 ${folderName} 与 id ${id} 不一致`);
  }

  if (path.isAbsolute(spritesheetPath)) {
    return { valid: false, error: "spritesheetPath 不能使用绝对路径" };
  }

  const normalized = path.normalize(spritesheetPath);
  if (normalized.startsWith("..") || normalized.includes(`..${path.sep}`)) {
    return { valid: false, error: "spritesheetPath 不能逃逸宠物目录" };
  }

  return {
    valid: true,
    value: {
      id,
      sourceId: id,
      displayName,
      description,
      spriteVersionNumber,
      spritesheetPath: normalized,
    },
    warnings,
  };
}

export async function resolveRealPath(filePath: string): Promise<string | null> {
  try {
    return await fs.realpath(filePath);
  } catch {
    return null;
  }
}

export async function validateSpritesheetPath(
  petDir: string,
  spritesheetPath: string,
): Promise<ValidationResult<string>> {
  const resolvedSpritesheet = path.resolve(petDir, spritesheetPath);
  const realPetDir = await resolveRealPath(petDir);
  const realSpritesheet = await resolveRealPath(resolvedSpritesheet);

  if (!realSpritesheet) {
    return { valid: false, error: "spritesheet 文件不存在或不可访问" };
  }

  if (realPetDir && !realSpritesheet.startsWith(realPetDir + path.sep)) {
    return { valid: false, error: "spritesheet 通过符号链接指向宠物目录外" };
  }

  return { valid: true, value: realSpritesheet, warnings: [] };
}

export async function validateSpritesheetImage(
  spritesheetPath: string,
  spriteVersionNumber: PetSpriteVersion = 1,
): Promise<ValidationResult<{ width: number; height: number }>> {
  const warnings: string[] = [];

  let stat;
  try {
    stat = await fs.stat(spritesheetPath);
  } catch {
    return { valid: false, error: "无法读取 spritesheet 文件" };
  }

  if (!stat.isFile()) {
    return { valid: false, error: "spritesheet 不是文件" };
  }

  if (stat.size > MAX_SPRITESHEET_BYTES) {
    return { valid: false, error: "spritesheet 超过 16 MiB 限制" };
  }

  const ext = path.extname(spritesheetPath).toLowerCase();
  if (ext !== ".png" && ext !== ".webp") {
    return { valid: false, error: "spritesheet 必须是 PNG 或 WebP" };
  }

  let dimensions: { width: number; height: number };
  try {
    dimensions = await readImageDimensions(spritesheetPath);
  } catch (error) {
    return {
      valid: false,
      error: error instanceof Error ? error.message : "无法解码 spritesheet",
    };
  }

  const expectedHeight = petSheetHeightForVersion(spriteVersionNumber);
  if (dimensions.width !== PET_SHEET_WIDTH || dimensions.height !== expectedHeight) {
    return {
      valid: false,
      error: `spritesheet 尺寸必须是 ${PET_SHEET_WIDTH}x${expectedHeight}，实际为 ${dimensions.width}x${dimensions.height}`,
    };
  }

  // Optional strict check for fully-transparent required cells would require sharp.
  warnings.push("未进行逐像素透明校验（需可选 sharp 依赖）");

  return {
    valid: true,
    value: dimensions,
    warnings,
  };
}

async function readImageDimensions(filePath: string): Promise<{ width: number; height: number }> {
  const handle = await fs.open(filePath, "r");
  try {
    const header = Buffer.alloc(30);
    const { bytesRead } = await handle.read(header, 0, 30, 0);
    if (bytesRead < 24) {
      throw new Error("文件过短，无法识别图片格式");
    }

    if (isPng(header)) {
      return readPngDimensions(header);
    }
    if (isWebp(header)) {
      return readWebpDimensions(header);
    }
    throw new Error("不支持的图片格式");
  } finally {
    await handle.close();
  }
}

function isPng(header: Buffer): boolean {
  return header[0] === 0x89 && header[1] === 0x50 && header[2] === 0x4e && header[3] === 0x47;
}

function readPngDimensions(header: Buffer): { width: number; height: number } {
  if (header[4] !== 0x0d || header[5] !== 0x0a || header[6] !== 0x1a || header[7] !== 0x0a) {
    throw new Error("PNG 文件签名无效");
  }
  return {
    width: header.readUInt32BE(16),
    height: header.readUInt32BE(20),
  };
}

function isWebp(header: Buffer): boolean {
  return (
    header[0] === 0x52 &&
    header[1] === 0x49 &&
    header[2] === 0x46 &&
    header[3] === 0x46 &&
    header[8] === 0x57 &&
    header[9] === 0x45 &&
    header[10] === 0x42 &&
    header[11] === 0x50
  );
}

function readWebpDimensions(header: Buffer): { width: number; height: number } {
  const chunk = header.toString("ascii", 12, 16);

  if (chunk === "VP8 ") {
    // Lossy WebP: dimensions start at offset 23 (after 0x9d 0x01 0x2a signature).
    if (header[23] !== 0x9d || header[24] !== 0x01 || header[25] !== 0x2a) {
      throw new Error("无法解析 VP8 lossy WebP 尺寸");
    }
    const raw = header.readUInt32LE(26);
    return {
      width: raw & 0x3fff,
      height: (raw >> 16) & 0x3fff,
    };
  }

  if (chunk === "VP8L") {
    // Lossless WebP: 0x2f signature at offset 20, dimensions at 21.
    if (header[20] !== 0x2f) {
      throw new Error("无法解析 VP8L lossless WebP 尺寸");
    }
    const raw = header.readUInt32LE(21);
    return {
      width: (raw & 0x3fff) + 1,
      height: ((raw >> 14) & 0x3fff) + 1,
    };
  }

  if (chunk === "VP8X") {
    // Extended WebP: canvas dimensions stored as width-1 / height-1.
    return {
      width: (header[24] | (header[25] << 8) | (header[26] << 16)) + 1,
      height: (header[27] | (header[28] << 8) | (header[29] << 16)) + 1,
    };
  }

  throw new Error("无法识别的 WebP 子格式");
}

export type ValidatedPetPackage = {
  sourceId: string;
  displayName: string;
  description: string;
  spriteVersionNumber: PetSpriteVersion;
  spritesheetPath: string;
};

export async function validatePetPackage(
  petDir: string,
  folderName: string,
): Promise<ValidationResult<ValidatedPetPackage>> {
  const manifestPath = path.join(petDir, "pet.json");

  let raw: string;
  try {
    raw = await fs.readFile(manifestPath, "utf8");
  } catch {
    return { valid: false, error: "无法读取 pet.json" };
  }

  if (Buffer.byteLength(raw, "utf8") > MAX_MANIFEST_BYTES) {
    return { valid: false, error: "pet.json 超过 64 KiB" };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { valid: false, error: "pet.json 不是有效 JSON" };
  }

  const manifestResult = validatePetManifestJson(parsed, folderName);
  if (!manifestResult.valid) {
    return { valid: false, error: manifestResult.error };
  }

  const manifest = manifestResult.value;
  const pathResult = await validateSpritesheetPath(petDir, manifest.spritesheetPath);
  if (!pathResult.valid) {
    return { valid: false, error: pathResult.error };
  }

  const imageResult = await validateSpritesheetImage(
    pathResult.value,
    manifest.spriteVersionNumber,
  );
  if (!imageResult.valid) {
    return { valid: false, error: imageResult.error };
  }

  return {
    valid: true,
    value: {
      sourceId: manifest.sourceId,
      displayName: manifest.displayName,
      description: manifest.description,
      spriteVersionNumber: manifest.spriteVersionNumber,
      spritesheetPath: pathResult.value,
    },
    warnings: [
      ...manifestResult.warnings,
      ...imageResult.warnings,
    ],
  };
}

export function sanitizePetId(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

export function makePetManifest(
  input: ValidatedPetPackage,
  source: PetSource,
  sourcePath: string,
  fingerprint: string,
): {
  schemaVersion: 1;
  id: string;
  sourceId: string;
  displayName: string;
  description: string;
  spriteVersionNumber: PetSpriteVersion;
  spritesheetFile: string;
  source: PetSource;
  sourcePath: string;
  sourceFingerprint: string;
  importedAt: string;
} {
  const now = new Date().toISOString();
  return {
    schemaVersion: 1,
    id: `${source}:${input.sourceId}`,
    sourceId: input.sourceId,
    displayName: input.displayName,
    description: input.description,
    spriteVersionNumber: input.spriteVersionNumber,
    spritesheetFile: "spritesheet" + path.extname(input.spritesheetPath),
    source,
    sourcePath,
    sourceFingerprint: fingerprint,
    importedAt: now,
  };
}
