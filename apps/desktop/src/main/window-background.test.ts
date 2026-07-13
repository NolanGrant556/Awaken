import { describe, expect, it, vi } from "vitest";
import {
  updateMainWindowBackground,
  windowBackgroundColor,
} from "./window-background.js";

describe("windowBackgroundColor", () => {
  it("resolves explicit and system theme colors", () => {
    expect(windowBackgroundColor("light", true)).toBe("#f8f8f7");
    expect(windowBackgroundColor("dark", false)).toBe("#0f0f0e");
    expect(windowBackgroundColor("system", true)).toBe("#0f0f0e");
    expect(windowBackgroundColor("system", false)).toBe("#f8f8f7");
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

    expect(mainWindow.setBackgroundColor).toHaveBeenCalledWith("#0f0f0e");
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
