/// <reference types="node" />

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

function cssRule(source: string, selector: string): string {
  const start = source.indexOf(`${selector} {`);
  if (start < 0) {
    throw new Error(`Missing CSS rule: ${selector}`);
  }
  const end = source.indexOf("}", start);
  if (end < 0) {
    throw new Error(`Unterminated CSS rule: ${selector}`);
  }
  return source.slice(start, end + 1);
}

describe("RunStage layout", () => {
  it("reserves symmetric scrollbar space so centered turns do not shift", () => {
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );
    const galleryStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/gallery/styles.css",
        import.meta.url,
      ),
      "utf8",
    );
    const runStageScroll = cssRule(designSystemStyles, ".aw-run-stage-scroll");
    const galleryRunStageScroll = cssRule(
      galleryStyles,
      ".ads-workbench-scene .aw-run-stage-scroll",
    );

    expect(runStageScroll).toContain("overflow: auto;");
    expect(runStageScroll).toContain("scrollbar-gutter: stable both-edges;");
    expect(galleryRunStageScroll).toContain(
      "scrollbar-gutter: stable both-edges;",
    );
  });
});
