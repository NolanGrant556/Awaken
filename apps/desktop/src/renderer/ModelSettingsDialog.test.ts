import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { AppearanceSettings } from "./ModelSettingsDialog";

describe("AppearanceSettings", () => {
  it("renders light and dark theme choices with the selected state", () => {
    const html = renderToString(
      React.createElement(AppearanceSettings, {
        theme: "dark",
        themeSaving: false,
        onThemeChange: vi.fn(),
      }),
    );

    expect(html).toContain("浅色");
    expect(html).toContain("黑色");
    expect(html).toContain('role="radiogroup"');
    expect(html).toContain('aria-checked="false"');
    expect(html).toContain('aria-checked="true"');
    expect(html).toContain("theme-choice-active");
  });
});
