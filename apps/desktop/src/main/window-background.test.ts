import { readFileSync } from "node:fs";
import { describe, expect, it, vi } from "vitest";
import {
  updateMainWindowBackground,
  windowBackgroundColor,
} from "./window-background.js";

describe("windowBackgroundColor", () => {
  it("resolves explicit and system theme colors", () => {
    expect(windowBackgroundColor("light", true)).toBe("#f8f8f8");
    expect(windowBackgroundColor("dark", false)).toBe("#1a1a1a");
    expect(windowBackgroundColor("system", true)).toBe("#1a1a1a");
    expect(windowBackgroundColor("system", false)).toBe("#f8f8f8");
  });
});

describe("updateMainWindowBackground", () => {
  it("updates only the explicitly owned main window", () => {
    const mainWindow = {
      isDestroyed: vi.fn(() => false),
      setBackgroundColor: vi.fn(),
    };
    const petWindow = {
      isDestroyed: vi.fn(() => false),
      setBackgroundColor: vi.fn(),
    };

    updateMainWindowBackground(mainWindow, "dark", false);

    expect(mainWindow.setBackgroundColor).toHaveBeenCalledWith("#1a1a1a");
    expect(petWindow.setBackgroundColor).not.toHaveBeenCalled();
  });

  it("does not update a missing or destroyed main window", () => {
    const destroyedWindow = {
      isDestroyed: vi.fn(() => true),
      setBackgroundColor: vi.fn(),
    };

    updateMainWindowBackground(null, "light", false);
    updateMainWindowBackground(destroyedWindow, "light", false);

    expect(destroyedWindow.setBackgroundColor).not.toHaveBeenCalled();
  });
});

// Keep Electron's loading surface aligned with the renderer and keep muted
// labels readable when either theme's canvas/navigation/raised colors change.
describe("desktop theme palette", () => {
  const css = readFileSync(new URL("../../../../Awaken Design System/src/tokens/tokens.css", import.meta.url), "utf8");
  const [light, dark] = css.split(':root[data-aw-theme="dark"]');
  function luminance(hex: string): number {
    const channels = hex.slice(1).match(/.{2}/g)!.map((channel) => {
      const value = parseInt(channel, 16) / 255;
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
    });
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  }
  for (const [theme, block] of [["light", light], ["dark", dark]] as const) {
    const value = (name: string) => {
      const match = block.match(new RegExp(`${name}:\\s*(#[0-9a-f]{6})`));
      expect(match, name).not.toBeNull();
      return match![1];
    };
    it(`${theme} matches the native window background`, () => {
      expect(windowBackgroundColor(theme, false)).toBe(value("--aw-bg-main"));
    });
    it(`${theme} keeps primary, secondary and tertiary labels at 4.5:1`, () => {
      for (const foreground of ["primary", "secondary", "tertiary"]) {
        for (const background of ["main", "nav", "white"]) {
          const a = luminance(value(`--aw-text-${foreground}`));
          const b = luminance(value(`--aw-bg-${background}`));
          expect((Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05), `${foreground} on ${background}`).toBeGreaterThanOrEqual(4.5);
        }
      }
    });
  }
});
