// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  installDesktopKeyboardPolicy,
  isUnmodifiedTabNavigation,
} from "./desktop-keyboard-policy";

describe("desktop keyboard policy", () => {
  let cleanup: (() => void) | undefined;

  afterEach(() => {
    cleanup?.();
    cleanup = undefined;
  });

  it("blocks Tab and Shift+Tab before controls can handle them", () => {
    cleanup = installDesktopKeyboardPolicy(document);
    const downstream = vi.fn();
    document.addEventListener("keydown", downstream);

    for (const shiftKey of [false, true]) {
      const event = new KeyboardEvent("keydown", {
        bubbles: true,
        cancelable: true,
        key: "Tab",
        shiftKey,
      });
      document.body.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(true);
    }
    expect(downstream).not.toHaveBeenCalled();

    document.removeEventListener("keydown", downstream);
  });

  it("leaves modified Tab and other keyboard input alone", () => {
    expect(
      isUnmodifiedTabNavigation(
        new KeyboardEvent("keydown", { key: "Tab", metaKey: true }),
      ),
    ).toBe(false);
    expect(
      isUnmodifiedTabNavigation(
        new KeyboardEvent("keydown", { key: "Tab", ctrlKey: true }),
      ),
    ).toBe(false);
    expect(
      isUnmodifiedTabNavigation(
        new KeyboardEvent("keydown", { key: "Tab", altKey: true }),
      ),
    ).toBe(false);
    expect(
      isUnmodifiedTabNavigation(new KeyboardEvent("keydown", { key: "Enter" })),
    ).toBe(false);
  });
});
