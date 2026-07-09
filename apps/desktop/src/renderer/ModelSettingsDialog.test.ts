import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { AppearanceSettings } from "./ModelSettingsDialog";

describe("AppearanceSettings", () => {
  it("renders system, light, and dark theme choices with the selected state", () => {
    const html = renderToString(
      React.createElement(AppearanceSettings, {
        themePreference: "system",
        themeSaving: false,
        onThemePreferenceChange: vi.fn(),
      }),
    );

    expect(html).toContain("跟随系统");
    expect(html).toContain("浅色");
    expect(html).toContain("黑色");
    expect(html).toContain('role="radiogroup"');
    expect(html.match(/aria-checked="false"/g)).toHaveLength(2);
    expect(html).toContain('aria-checked="true"');
    expect(html).toContain("theme-choice-active");
  });
});
