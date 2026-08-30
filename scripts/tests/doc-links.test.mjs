import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
  checkDocLinks,
  extractMarkdownDestinations,
} from "../check-doc-links.mjs";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const temporaryDirectories = [];

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { recursive: true, force: true });
  }
});

describe("documentation link checker", () => {
  it("ignores example links inside fenced and inline code", () => {
    const markdown = [
      "[真实链接](docs/真实.md)",
      "`[行内示例](inline-missing.md)`",
      "```markdown",
      "[围栏示例](feedback-testing-a82c.md)",
      "```",
      "~~~text",
      "[另一围栏示例](other-missing.md)",
      "~~~",
    ].join("\n");

    assert.deepEqual(extractMarkdownDestinations(markdown), [
      { target: "docs/真实.md", line: 1 },
    ]);
  });

  it("reports a real missing target with its source line", () => {
    const rootDir = mkdtempSync(join(tmpdir(), "awaken-doc-links-"));
    temporaryDirectories.push(rootDir);
    mkdirSync(join(rootDir, "docs"));
    writeFileSync(
      join(rootDir, "README.md"),
      ["[存在](docs/存在.md)", "[缺失](docs/缺失.md)"].join("\n"),
    );
    writeFileSync(join(rootDir, "docs", "存在.md"), "# 存在\n");

    const result = checkDocLinks({ rootDir });

    assert.equal(result.checkedLinks, 2);
    assert.deepEqual(result.missing, [
      { file: "README.md", line: 2, target: "docs/缺失.md" },
    ]);
  });

  it("accepts every real local link in the current repository docs", () => {
    const result = checkDocLinks({ rootDir: repoRoot });

    assert.equal(result.missing.length, 0);
    assert.ok(result.checkedFiles > 1);
    assert.ok(result.checkedLinks > 1);
  });
});
