import { createHash } from "node:crypto";
import { deflateSync } from "node:zlib";
import {
  PET_SHEET_HEIGHT,
  PET_SHEET_WIDTH,
  petSheetHeightForVersion,
  type PetSpriteVersion,
} from "../../shared/pets.js";

export function createTransparentSpritesheet(spriteVersionNumber: PetSpriteVersion = 1): Buffer {
  const sheetHeight = spriteVersionNumber === 1
    ? PET_SHEET_HEIGHT
    : petSheetHeightForVersion(spriteVersionNumber);
  const rowSize = 1 + PET_SHEET_WIDTH * 4;
  const raw = Buffer.alloc(sheetHeight * rowSize);
  for (let y = 0; y < sheetHeight; y++) {
    raw[y * rowSize] = 0; // filter byte: None
  }
  const compressed = deflateSync(raw);

  const chunks: Buffer[] = [];
  chunks.push(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]));
  chunks.push(pngChunk("IHDR", ihdrData(PET_SHEET_WIDTH, sheetHeight)));
  chunks.push(pngChunk("IDAT", compressed));
  chunks.push(pngChunk("IEND", Buffer.alloc(0)));
  return Buffer.concat(chunks);
}

function ihdrData(width: number, height: number): Buffer {
  const data = Buffer.alloc(13);
  data.writeUInt32BE(width, 0);
  data.writeUInt32BE(height, 4);
  data[8] = 8; // bit depth
  data[9] = 6; // color type: RGBA
  data[10] = 0; // compression
  data[11] = 0; // filter
  data[12] = 0; // interlace
  return data;
}

function pngChunk(type: string, data: Buffer): Buffer {
  const typeBuffer = Buffer.from(type, "ascii");
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length, 0);
  const crcValue = Buffer.alloc(4);
  crcValue.writeUInt32BE(crc32(Buffer.concat([typeBuffer, data])), 0);
  return Buffer.concat([length, typeBuffer, data, crcValue]);
}

function crc32(data: Buffer): number {
  const table = getCrc32Table();
  let crc = 0xffffffff;
  for (let i = 0; i < data.length; i++) {
    crc = table[(crc ^ data[i]) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

let crc32Table: Uint32Array | null = null;

function getCrc32Table(): Uint32Array {
  if (crc32Table) return crc32Table;
  crc32Table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) {
      c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    crc32Table[n] = c >>> 0;
  }
  return crc32Table;
}

export function createHashFromBuffer(data: Buffer): string {
  const hash = createHash("sha256");
  hash.update(data);
  return hash.digest("hex");
}
