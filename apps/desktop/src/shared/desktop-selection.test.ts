import { describe, expect, it } from "vitest";
import { isDesktopSelectAllShortcut } from "./desktop-selection.js";

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
