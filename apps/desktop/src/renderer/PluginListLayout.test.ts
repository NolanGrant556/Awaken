import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const pluginListStyles = readFileSync(
  new URL(
    "../../../../Awaken Design System/src/components/AwakenPlugins/AwakenPlugins.css",
    import.meta.url,
  ),
  "utf8",
);

describe("Plugin list layout", () => {
  it("keeps page scrolling outside the centered grid and preserves a bottom tail", () => {
    expect(pluginListStyles).toMatch(
      /\.aw-plugins\s*\{[\s\S]*?overflow-y:\s*auto;/,
    );
    expect(pluginListStyles).toMatch(
      /\.aw-plugins__inner\s*\{[\s\S]*?align-self:\s*start;[\s\S]*?padding:\s*var\(--aw-space-13\)\s+0\s+var\(--aw-space-15\);[\s\S]*?overflow:\s*visible;/,
    );
    expect(pluginListStyles).toMatch(
      /\.aw-plugins__grid\s*\{[\s\S]*?grid-template-columns:\s*repeat\(2,\s*minmax\(280px,\s*1fr\)\);/,
    );
  });
});
