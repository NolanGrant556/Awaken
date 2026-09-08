import type { XlsxPictureGeometry } from "../shared/xlsx-picture-geometry.js";
import { positionPictures } from "../shared/xlsx-picture-layout.js";
import fs from "node:fs/promises";
import path from "node:path";
import yauzl from "yauzl";
import { SaxesParser } from "saxes";

// Bump when the adapter or its generated HTML changes. This participates in the
// Office cache key; old, incorrectly sized previews must never be reused.
export const XLSX_PICTURE_LAYOUT_VERSION = 1;
const MAX_XML_BYTES = 8 * 1024 * 1024;
const MAX_XML_TOTAL_BYTES = 32 * 1024 * 1024;
const MAX_ZIP_ENTRIES = 20_000;
const MAX_XML_NODES = 250_000;
const MAX_PICTURES = 2000;

type XmlNode = {
  name: string;
  attrs: Record<string, string>;
  text: string;
  children: XmlNode[];
};
type Marker = XlsxPictureGeometry["from"];

function parseXml(xml: string): XmlNode {
  const parser = new SaxesParser({ xmlns: true });
  const root: XmlNode = { name: "root", attrs: {}, text: "", children: [] };
  const stack = [root];
  let nodes = 0;
  parser.on("doctype", () => {
    throw new Error("Excel 预览不支持带 DTD 的 XML。");
  });
  parser.on("opentag", (tag) => {
    if (++nodes > MAX_XML_NODES || stack.length > 64)
      throw new Error("Excel 图片布局 XML 超过解析上限。");
    const node: XmlNode = {
      name: tag.local,
      attrs: {},
      text: "",
      children: [],
    };
    for (const attr of Object.values(tag.attributes))
      node.attrs[attr.local] = attr.value;
    stack.at(-1)!.children.push(node);
    stack.push(node);
  });
  parser.on("text", (value) => {
    stack.at(-1)!.text += value;
  });
  parser.on("closetag", () => {
    stack.pop();
  });
  parser.write(xml).close();
  return root;
}

function descendants(node: XmlNode, name: string): XmlNode[] {
  return node.children.flatMap((child) => [
    ...(child.name === name ? [child] : []),
    ...descendants(child, name),
  ]);
}
function child(node: XmlNode, name: string): XmlNode | undefined {
  return node.children.find((entry) => entry.name === name);
}
function number(value: string | undefined): number {
  const result = Number(value);
  if (value === undefined || !Number.isFinite(result))
    throw new Error("Excel 图片锚点尺寸无效。");
  return result;
}
function marker(node: XmlNode | undefined): Marker {
  if (!node) throw new Error("Excel 图片缺少单元格锚点。");
  const col = number(child(node, "col")?.text),
    row = number(child(node, "row")?.text);
  if (
    !Number.isInteger(col) ||
    !Number.isInteger(row) ||
    col < 0 ||
    col > 16383 ||
    row < 0 ||
    row > 1048575
  )
    throw new Error("Excel 图片锚点超出工作表范围。");
  return {
    col,
    row,
    dx: number(child(node, "colOff")?.text ?? "0") / 12700,
    dy: number(child(node, "rowOff")?.text ?? "0") / 12700,
  };
}
function relationshipPath(source: string): string {
  return path.posix.join(
    path.posix.dirname(source),
    "_rels",
    `${path.posix.basename(source)}.rels`,
  );
}
function relationships(
  files: Map<string, string>,
  source: string,
): Map<string, string> {
  const xml = files.get(relationshipPath(source));
  if (!xml) return new Map();
  return new Map(
    descendants(parseXml(xml), "Relationship").flatMap((rel) => {
      if (rel.attrs.TargetMode === "External") return [];
      const target = decodeURIComponent(rel.attrs.Target ?? "");
      const resolved = path.posix.normalize(
        target.startsWith("/")
          ? target.slice(1)
          : path.posix.join(path.posix.dirname(source), target),
      );
      if (!resolved.startsWith("xl/") || resolved.includes("\\"))
        throw new Error("Excel 图片关系超出工作簿范围。");
      return [[rel.attrs.Id, resolved] as const];
    }),
  );
}

export function xlsxPictureGeometry(
  files: Map<string, string>,
  entryNames: Set<string>,
): XlsxPictureGeometry[][] {
  const workbook = files.get("xl/workbook.xml");
  if (!workbook) throw new Error("Excel 缺少工作簿定义。");
  const sheetLinks = relationships(files, "xl/workbook.xml");
  let count = 0;
  return descendants(parseXml(workbook), "sheet").map((sheet) => {
    const sheetPath = sheetLinks.get(sheet.attrs.id),
      sheetXml = sheetPath && files.get(sheetPath);
    if (!sheetPath || !sheetXml) throw new Error("Excel 工作表关系无法解析。");
    const drawingId = descendants(parseXml(sheetXml), "drawing")[0]?.attrs.id;
    if (!drawingId) return [];
    const drawingPath = relationships(files, sheetPath).get(drawingId);
    const drawingXml = drawingPath && files.get(drawingPath);
    if (!drawingPath || !drawingXml)
      throw new Error("Excel 图片布局无法解析。");
    const imageLinks = relationships(files, drawingPath);
    const drawing = descendants(parseXml(drawingXml), "wsDr")[0];
    if (!drawing) throw new Error("Excel 图片布局缺少根元素。");
    return drawing.children.flatMap((anchor): XlsxPictureGeometry[] => {
      const picture = child(anchor, "pic");
      if (!picture || !["oneCellAnchor", "twoCellAnchor"].includes(anchor.name))
        return [];
      const embed = descendants(picture, "blip")[0]?.attrs.embed;
      const imagePath = embed && imageLinks.get(embed);
      // Match the bundled renderer's omission of external or missing images.
      if (!imagePath || !entryNames.has(imagePath)) return [];
      if (++count > MAX_PICTURES)
        throw new Error("Excel 图片数量超过预览上限。");
      const from = marker(child(anchor, "from"));
      if (anchor.name === "twoCellAnchor")
        return [{ from, to: marker(child(anchor, "to")) }];
      const extent = child(anchor, "ext");
      const width = number(extent?.attrs.cx) / 12700,
        height = number(extent?.attrs.cy) / 12700;
      if (width <= 0 || height <= 0)
        throw new Error("Excel 图片尺寸必须大于零。");
      return [{ from, width, height }];
    });
  });
}

async function readLayoutParts(
  sourcePath: string,
  signal?: AbortSignal,
): Promise<{ files: Map<string, string>; names: Set<string> }> {
  signal?.throwIfAborted();
  const zip = await new Promise<yauzl.ZipFile>((resolve, reject) => {
    yauzl.open(
      sourcePath,
      { lazyEntries: true, autoClose: false },
      (error, value) =>
        error || !value
          ? reject(error ?? new Error("无法打开 Excel 文件。"))
          : resolve(value),
    );
  });
  return new Promise((resolve, reject) => {
    const files = new Map<string, string>(),
      names = new Set<string>();
    let total = 0,
      settled = false;
    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      signal?.removeEventListener("abort", abort);
      zip.close();
      if (error) reject(error);
      else resolve({ files, names });
    };
    const abort = () => finish(new Error("已取消 Excel 图片预览。"));
    signal?.addEventListener("abort", abort, { once: true });
    if (signal?.aborted) {
      abort();
      return;
    }
    zip.on("error", finish);
    zip.on("end", () => finish());
    zip.on("entry", (entry: yauzl.Entry) => {
      if (settled) return;
      if (names.has(entry.fileName) || names.size >= MAX_ZIP_ENTRIES) {
        finish(new Error("Excel ZIP 重复条目或超过数量上限。"));
        return;
      }
      names.add(entry.fileName);
      if (
        !/^xl\/(?:workbook\.xml|_rels\/workbook\.xml\.rels|(?:worksheets|drawings)\/(?:_rels\/)?[^/]+\.(?:xml|rels))$/.test(
          entry.fileName,
        )
      ) {
        zip.readEntry();
        return;
      }
      if (
        entry.uncompressedSize > MAX_XML_BYTES ||
        total + entry.uncompressedSize > MAX_XML_TOTAL_BYTES
      ) {
        finish(new Error("Excel 图片布局 XML 超过大小上限。"));
        return;
      }
      zip.openReadStream(entry, (error, stream) => {
        if (error || !stream) {
          finish(error ?? new Error("无法读取 Excel 图片布局。"));
          return;
        }
        const chunks: Buffer[] = [];
        let size = 0;
        stream.on("error", finish);
        stream.on("data", (chunk: Buffer) => {
          size += chunk.length;
          total += chunk.length;
          if (settled || size > MAX_XML_BYTES || total > MAX_XML_TOTAL_BYTES) {
            stream.destroy();
            finish(new Error("Excel 图片布局 XML 超过大小上限。"));
            return;
          }
          chunks.push(chunk);
        });
        stream.on("end", () => {
          if (settled) return;
          files.set(entry.fileName, Buffer.concat(chunks).toString("utf8"));
          zip.readEntry();
        });
      });
    });
    zip.readEntry();
  });
}

export function normalizeXlsxPictureHtml(
  html: string,
  sheets: XlsxPictureGeometry[][],
): string {
  const parts = html.split(
    /(?=<div class="sheet-content(?: active)?" data-sheet="\d+">)/,
  );
  const seen = new Set<number>();
  for (let i = 1; i < parts.length; i++) {
    const index = Number(
      /^<div class="sheet-content(?: active)?" data-sheet="(\d+)">/.exec(
        parts[i],
      )?.[1],
    );
    const images = Array.from(parts[i].matchAll(/<img class="xlsx-picture" /g));
    if (images.length !== (sheets[index]?.length ?? 0))
      throw new Error("Excel 图片与预览布局数量不匹配。");
    seen.add(index);
    // Prevent the known oversized frame from appearing before grid measurement.
    parts[i] = parts[i].replace(
      /(<div style=")([^"]+)(" data-from-col="\d+" data-from-row="\d+">\s*<img class="xlsx-picture")/g,
      (_match, prefix, style, suffix) =>
        `${prefix}${style};visibility:hidden${suffix}`,
    );
  }
  if (sheets.some((pictures, index) => pictures.length > 0 && !seen.has(index)))
    throw new Error("Excel 图片所在工作表未生成预览。");
  if (!sheets.some((pictures) => pictures.length)) return html;
  if (!html.includes("</body>")) throw new Error("Excel 预览 HTML 不完整。");
  const script = `<script data-awaken-xlsx-picture-layout="${XLSX_PICTURE_LAYOUT_VERSION}">(${positionPictures.toString()})(${JSON.stringify(sheets)}, document, ResizeObserver);</script>`;
  return parts.join("").replace("</body>", `${script}\n</body>`);
}

export async function correctXlsxPictureLayout(
  sourcePath: string,
  htmlPath: string,
  signal?: AbortSignal,
): Promise<void> {
  if (path.extname(sourcePath).toLowerCase() !== ".xlsx") return;
  const { files, names } = await readLayoutParts(sourcePath, signal);
  const sheets = xlsxPictureGeometry(files, names);
  if (!sheets.some((pictures) => pictures.length)) return;
  const html = await fs.readFile(htmlPath, "utf8");
  signal?.throwIfAborted();
  await fs.writeFile(htmlPath, normalizeXlsxPictureHtml(html, sheets), "utf8");
}
