/// <reference types="node" />

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const computerSettingsStyles = readFileSync(
  new URL(
    "../../../../../Awaken Design System/src/components/AwakenComputerSettings/AwakenComputerSettings.css",
    import.meta.url,
  ),
  "utf8",
);

describe("Computer settings theme contract", () => {
  it("uses the current semantic tokens for dark and light themes", () => {
    expect(computerSettingsStyles).toContain(
      '@import "../../tokens/components.css";',
    );
    expect(computerSettingsStyles).toMatch(
      /\.computer-settings\s*\{[\s\S]*?color:\s*var\(--aw-text-primary\);/,
    );
    expect(computerSettingsStyles).toMatch(
      /\.computer-settings-card\s*\{[\s\S]*?border:\s*1px solid var\(--aw-border-main\);[\s\S]*?background:\s*var\(--aw-surface-raised\);/,
    );
    expect(computerSettingsStyles).toContain("var(--aw-text-secondary)");
    expect(computerSettingsStyles).toContain("var(--aw-status-success)");
    expect(computerSettingsStyles).toContain("var(--aw-status-danger)");
    expect(computerSettingsStyles).not.toContain("--awaken-");
    expect(computerSettingsStyles).not.toContain("hsl(");
  });
});
