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

describe("Project Skills folder layout", () => {
  it("keeps Desktop and Design System folders filling from the left", () => {
    const desktopStyles = readFileSync(new URL("./styles.css", import.meta.url), "utf8");
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenSkills/AwakenSkills.css",
        import.meta.url,
      ),
      "utf8",
    );

    const rules = [
      cssRule(desktopStyles, ".skill-project-folder-grid"),
      cssRule(designSystemStyles, ".aw-skills__folder-grid"),
    ];

    for (const rule of rules) {
      expect(rule).toContain("repeat(auto-fill, minmax(164px, 1fr))");
      expect(rule).toContain("justify-items: start");
      expect(rule).not.toContain("auto-fit");
    }
  });
});
