// @vitest-environment jsdom

import { afterEach, describe, expect, it } from "vitest";
import {
  AWAKEN_FOCUS_MODALITY_ATTRIBUTE,
  installAwakenFocusModality,
} from "../../../../Awaken Design System/src/focusModality";

describe("Awaken focus modality", () => {
  let cleanup: (() => void) | undefined;

  afterEach(() => {
    cleanup?.();
    cleanup = undefined;
  });

  it("keeps pointer focus quiet and enables keyboard focus for navigation keys", () => {
    cleanup = installAwakenFocusModality(document.documentElement);

    expect(
      document.documentElement.getAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe("pointer");

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "a" }));
    expect(
      document.documentElement.getAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe("pointer");

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab" }));
    expect(
      document.documentElement.getAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe("keyboard");

    document.dispatchEvent(new Event("pointerdown"));
    expect(
      document.documentElement.getAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe("pointer");
  });

  it("does not treat modified shortcuts as keyboard focus navigation", () => {
    cleanup = installAwakenFocusModality(document.documentElement);

    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Tab", metaKey: true }),
    );

    expect(
      document.documentElement.getAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe("pointer");
  });

  it("does not treat ordinary text editing as focus navigation", () => {
    cleanup = installAwakenFocusModality(document.documentElement);
    const textarea = document.createElement("textarea");
    document.body.append(textarea);

    textarea.dispatchEvent(
      new KeyboardEvent("keydown", { key: " ", bubbles: true }),
    );
    expect(
      document.documentElement.getAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe("pointer");

    textarea.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Tab", bubbles: true }),
    );
    expect(
      document.documentElement.getAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe("keyboard");

    textarea.remove();
  });

  it("removes listeners and the root marker on cleanup", () => {
    cleanup = installAwakenFocusModality(document.documentElement);
    cleanup();
    cleanup = undefined;

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab" }));

    expect(
      document.documentElement.hasAttribute(AWAKEN_FOCUS_MODALITY_ATTRIBUTE),
    ).toBe(false);
  });
});
