import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { repairGfmTableDelimiters } from "./repairGfmTableDelimiters";

const minimaxTable = ["| 类别 | 工具 |", "|------|", "| 文件 | Read / Write / Edit |"].join("\n");

describe("repairGfmTableDelimiters", () => {
  it("pads a MiniMax one-column delimiter to the header width", () => {
    const repaired = repairGfmTableDelimiters(minimaxTable);
    assert.equal(repaired.split("\n")[1], "|------|------|");
    assert.equal(repaired.split("\n")[0], "| 类别 | 工具 |");
    assert.equal(repaired.split("\n")[2], "| 文件 | Read / Write / Edit |");
  });

  it("trims extra delimiter cells to the header width", () => {
    const source = ["| 类别 | 工具 |", "|---|---|---|", "| A | B |"].join("\n");
    assert.equal(repairGfmTableDelimiters(source).split("\n")[1], "|---|---|");
  });

  it("leaves a well-formed table unchanged", () => {
    const source = ["| 项目 | 结果 |", "| --- | --- |", "| Markdown 渲染 | 正常 |"].join("\n");
    assert.equal(repairGfmTableDelimiters(source), source);
  });

  it("does not rewrite pipes inside a fenced code block", () => {
    const source = ["```md", "| 类别 | 工具 |", "|------|", "| A | B |", "```"].join("\n");
    assert.equal(repairGfmTableDelimiters(source), source);
  });

  it("repairs a quoted table without touching the quote prefix", () => {
    const source = ["> | 类别 | 工具 |", "> |------|", "> | A | B |"].join("\n");
    assert.equal(repairGfmTableDelimiters(source).split("\n")[1], "> |------|------|");
  });

  it("preserves alignment markers when padding", () => {
    const source = ["| left | right |", "|:------|", "| a | b |"].join("\n");
    assert.equal(repairGfmTableDelimiters(source).split("\n")[1], "|:------|:------|");
  });

  it("does not treat a setext underline as a table delimiter", () => {
    const source = ["类别 | 工具", "------", "A | B"].join("\n");
    assert.equal(repairGfmTableDelimiters(source), source);
  });
});
