import { describe, expect, it } from "vitest";
import {
  MAX_TEXT_PREVIEW_BYTES,
  classifyPreviewFile,
  decodeTextPreviewBuffer,
  parseSingleByteRange,
  previewByteLimit,
  rasterDimensions,
  safePreviewPathname,
} from "./project-file-preview-policy.js";

describe("project file preview policy", () => {
  it("dispatches every supported first-release format", () => {
    expect(classifyPreviewFile("report.docx")?.kind).toBe("office");
    expect(classifyPreviewFile("report.xlsx")?.kind).toBe("office");
    expect(classifyPreviewFile("report.pptx")?.kind).toBe("office");
    expect(classifyPreviewFile("README.markdown")?.kind).toBe("markdown");
    expect(classifyPreviewFile("main.tsx")).toMatchObject({
      kind: "code",
      language: "typescript",
    });
    expect(classifyPreviewFile("go.mod")).toMatchObject({
      kind: "code",
      language: "mod",
    });
    expect(classifyPreviewFile("go.sum")?.kind).toBe("code");
    expect(classifyPreviewFile(".gitignore")?.kind).toBe("code");
    expect(classifyPreviewFile("manual.pdf")?.kind).toBe("pdf");
    expect(classifyPreviewFile("photo.webp")?.kind).toBe("image");
    expect(classifyPreviewFile("index.html")?.kind).toBe("html");
    expect(classifyPreviewFile("archive.zip")).toBeUndefined();
    expect(previewByteLimit("code")).toBe(MAX_TEXT_PREVIEW_BYTES);
  });

  it("decodes safe source text and rejects binary or invalid UTF-8 content", () => {
    expect(decodeTextPreviewBuffer(Buffer.from("module example.com/app\n"))).toBe(
      "module example.com/app\n",
    );
    expect(decodeTextPreviewBuffer(Buffer.from([0x00, 0x01, 0x02]))).toBeUndefined();
    expect(decodeTextPreviewBuffer(Buffer.from([0xc3, 0x28]))).toBeUndefined();
    expect(
      decodeTextPreviewBuffer(Buffer.from([0xff, 0xfe, 0x61, 0x00, 0x62, 0x00])),
    ).toBe("ab");
  });

  it("parses bounded Chromium byte range requests", () => {
    expect(parseSingleByteRange(null, 100)).toBeUndefined();
    expect(parseSingleByteRange("bytes=10-19", 100)).toEqual({ start: 10, end: 19 });
    expect(parseSingleByteRange("bytes=90-", 100)).toEqual({ start: 90, end: 99 });
    expect(parseSingleByteRange("bytes=-10", 100)).toEqual({ start: 90, end: 99 });
    expect(parseSingleByteRange("bytes=100-101", 100)).toBe("unsatisfiable");
    expect(parseSingleByteRange("bytes=1-2,4-5", 100)).toBe("unsatisfiable");
  });

  it("rejects encoded separators, traversal, malformed escapes, and NUL", () => {
    expect(safePreviewPathname("/assets/app.js")).toEqual(["assets", "app.js"]);
    expect(safePreviewPathname("/../secret")).toBeUndefined();
    expect(safePreviewPathname("/assets%2Fsecret")).toBeUndefined();
    expect(safePreviewPathname("/assets%5Csecret")).toBeUndefined();
    expect(safePreviewPathname("/%E0%A4%A")).toBeUndefined();
    expect(safePreviewPathname("/bad%00name")).toBeUndefined();
  });

  it("reads common raster dimensions without decoding the whole image", () => {
    const png = Buffer.alloc(24);
    png.writeUInt32BE(640, 16);
    png.writeUInt32BE(480, 20);
    expect(rasterDimensions("png", png)).toEqual({ width: 640, height: 480 });

    const gif = Buffer.alloc(10);
    gif.writeUInt16LE(320, 6);
    gif.writeUInt16LE(200, 8);
    expect(rasterDimensions("gif", gif)).toEqual({ width: 320, height: 200 });
  });
});
