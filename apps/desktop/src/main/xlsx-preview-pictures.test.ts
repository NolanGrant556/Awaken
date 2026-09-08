import { describe, expect, it } from "vitest";
import {
  normalizeXlsxPictureHtml,
  xlsxPictureGeometry,
} from "./xlsx-preview-pictures.js";

const from = `<from><col>11</col><colOff>95250</colOff><row>3</row><rowOff>47625</rowOff></from>`;
function parts(anchor: string) {
  return new Map([
    [
      "xl/workbook.xml",
      '<workbook xmlns:r="urn:r"><sheets><sheet name="基础资料" r:id="s1"/></sheets></workbook>',
    ],
    [
      "xl/_rels/workbook.xml.rels",
      '<Relationships><Relationship Id="s1" Target="worksheets/sheet1.xml"/></Relationships>',
    ],
    [
      "xl/worksheets/sheet1.xml",
      '<worksheet xmlns:r="urn:r"><drawing r:id="d1"/></worksheet>',
    ],
    [
      "xl/worksheets/_rels/sheet1.xml.rels",
      '<Relationships><Relationship Id="d1" Target="../drawings/drawing1.xml"/></Relationships>',
    ],
    ["xl/drawings/drawing1.xml", `<wsDr xmlns:r="urn:r">${anchor}</wsDr>`],
    [
      "xl/drawings/_rels/drawing1.xml.rels",
      '<Relationships><Relationship Id="image1" Target="../media/image1.png"/></Relationships>',
    ],
  ]);
}
const pic = '<pic><blipFill><blip r:embed="image1"/></blipFill></pic>';
const imageNames = new Set(["xl/media/image1.png"]);
const oneCell = () =>
  xlsxPictureGeometry(
    parts(
      `<oneCellAnchor>${from}<ext cx="838200" cy="1066800"/>${pic}</oneCellAnchor>`,
    ),
    imageNames,
  );

describe("Excel preview picture geometry", () => {
  it("preserves exact one-cell EMU extents and offsets regardless of row height", () => {
    expect(oneCell()).toEqual([
      [{ from: { col: 11, row: 3, dx: 7.5, dy: 3.75 }, width: 66, height: 84 }],
    ]);
  });

  it("retains both markers including within-cell offsets for two-cell pictures", () => {
    const result = xlsxPictureGeometry(
      parts(
        `<twoCellAnchor>${from}<to><col>12</col><row>4</row><colOff>12700</colOff><rowOff>25400</rowOff></to>${pic}</twoCellAnchor>`,
      ),
      imageNames,
    );
    expect(result[0][0]).toEqual({
      from: { col: 11, row: 3, dx: 7.5, dy: 3.75 },
      to: { col: 12, row: 4, dx: 1, dy: 2 },
    });
  });

  it("rejects DTDs, invalid dimensions and relationships outside the workbook", () => {
    const files = parts(
      `<oneCellAnchor>${from}<ext cx="0" cy="1066800"/>${pic}</oneCellAnchor>`,
    );
    expect(() => xlsxPictureGeometry(files, imageNames)).toThrow(
      "尺寸必须大于零",
    );
    files.set("xl/workbook.xml", "<!DOCTYPE workbook><workbook/>");
    expect(() => xlsxPictureGeometry(files, imageNames)).toThrow("DTD");
    files.set(
      "xl/_rels/workbook.xml.rels",
      '<Relationships><Relationship Id="s1" Target="../../secrets"/></Relationships>',
    );
    expect(() => xlsxPictureGeometry(files, imageNames)).toThrow("工作簿范围");
  });

  it("refuses an unmatched preview instead of applying another picture's layout", () => {
    expect(() =>
      normalizeXlsxPictureHtml(
        '<body><div class="sheet-content active" data-sheet="0"></div></body>',
        oneCell(),
      ),
    ).toThrow("数量不匹配");
    expect(normalizeXlsxPictureHtml("<body>no pictures</body>", [[]])).toBe(
      "<body>no pictures</body>",
    );
  });
});
