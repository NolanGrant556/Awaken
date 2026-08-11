import { describe, expect, it } from "vitest";
import {
  isDesktopSelectAllShortcut,
  isDesktopTabNavigation,
  isDesktopZoomShortcut,
} from "./desktop-selection.js";

function keyboardInput(
  overrides: Partial<Parameters<typeof isDesktopSelectAllShortcut>[0]> = {},
) {
  return {
    type: "keyDown",
    key: "a",
    code: "KeyA",
    control: false,
    meta: false,
    alt: false,
    shift: false,
    ...overrides,
  };
}

describe("isDesktopSelectAllShortcut", () => {
  it("recognizes macOS Command+A", () => {
    expect(
      isDesktopSelectAllShortcut(keyboardInput({ meta: true })),
    ).toBe(true);
  });

  it("recognizes Windows and Linux Ctrl+A", () => {
    expect(
      isDesktopSelectAllShortcut(keyboardInput({ control: true })),
    ).toBe(true);
  });

  it("ignores modified, key-up, and unrelated input", () => {
    expect(
      isDesktopSelectAllShortcut(keyboardInput({ meta: true, shift: true })),
    ).toBe(false);
    expect(
      isDesktopSelectAllShortcut(keyboardInput({ control: true, alt: true })),
    ).toBe(false);
    expect(
      isDesktopSelectAllShortcut(
        keyboardInput({ type: "keyUp", control: true }),
      ),
    ).toBe(false);
    expect(
      isDesktopSelectAllShortcut(
        keyboardInput({ key: "b", code: "KeyB", control: true }),
      ),
    ).toBe(false);
    expect(
      isDesktopSelectAllShortcut(
        keyboardInput({ control: true, meta: true }),
      ),
    ).toBe(false);
  });
});

describe("isDesktopZoomShortcut", () => {
  it("recognizes macOS and Windows/Linux zoom shortcuts", () => {
    expect(isDesktopZoomShortcut(keyboardInput({ key: "+", code: "Equal", meta: true, shift: true }))).toBe(true);
    expect(isDesktopZoomShortcut(keyboardInput({ key: "-", code: "Minus", control: true }))).toBe(true);
    expect(isDesktopZoomShortcut(keyboardInput({ key: "0", code: "Digit0", meta: true }))).toBe(true);
  });

  it("does not consume unmodified or unrelated editing shortcuts", () => {
    expect(isDesktopZoomShortcut(keyboardInput({ key: "+", code: "Equal" }))).toBe(false);
    expect(isDesktopZoomShortcut(keyboardInput({ key: "a", code: "KeyA", meta: true }))).toBe(false);
    expect(isDesktopZoomShortcut(keyboardInput({ key: "+", code: "Equal", control: true, alt: true }))).toBe(false);
  });
});

describe("isDesktopTabNavigation", () => {
  it("recognizes unmodified Tab and Shift+Tab key-down input", () => {
    expect(
      isDesktopTabNavigation(keyboardInput({ key: "Tab", code: "Tab" })),
    ).toBe(true);
    expect(
      isDesktopTabNavigation(
        keyboardInput({ key: "Tab", code: "Tab", shift: true }),
      ),
    ).toBe(true);
  });

  it("does not consume modified, key-up, or unrelated input", () => {
    expect(
      isDesktopTabNavigation(
        keyboardInput({ key: "Tab", code: "Tab", control: true }),
      ),
    ).toBe(false);
    expect(
      isDesktopTabNavigation(
        keyboardInput({ key: "Tab", code: "Tab", meta: true }),
      ),
    ).toBe(false);
    expect(
      isDesktopTabNavigation(
        keyboardInput({ key: "Tab", code: "Tab", alt: true }),
      ),
    ).toBe(false);
    expect(
      isDesktopTabNavigation(
        keyboardInput({ type: "keyUp", key: "Tab", code: "Tab" }),
      ),
    ).toBe(false);
    expect(isDesktopTabNavigation(keyboardInput())).toBe(false);
  });
});
