import { describe, expect, it } from "vitest";
import {
  browserActionAllowed,
  deriveBrowserState,
  EMBEDDED_BROWSER_PARTITION,
  normalizeBrowserAddressInput,
  originOf,
  parseNavigable,
  safeExternalUrl,
  viewportBounds,
} from "./embedded-browser";

describe("normalizeBrowserAddressInput", () => {
  it("treats a bare host as https", () => {
    expect(normalizeBrowserAddressInput("example.com")).toEqual({
      ok: true,
      url: "https://example.com/",
    });
  });

  it("accepts explicit http and https", () => {
    expect(normalizeBrowserAddressInput("http://localhost:3000/path")).toEqual({
      ok: true,
      url: "http://localhost:3000/path",
    });
    expect(normalizeBrowserAddressInput("https://awaken.dev")).toEqual({
      ok: true,
      url: "https://awaken.dev/",
    });
  });

  it("rejects empty, file, javascript and custom schemes", () => {
    expect(normalizeBrowserAddressInput("  ")).toEqual({
      ok: false,
      reason: "empty",
    });
    expect(normalizeBrowserAddressInput("file:///tmp/index.html")).toEqual({
      ok: false,
      reason: "unsupported_scheme",
    });
    expect(normalizeBrowserAddressInput("javascript:alert(1)")).toEqual({
      ok: false,
      reason: "unsupported_scheme",
    });
    expect(normalizeBrowserAddressInput("awaken-preview://id/index.html")).toEqual({
      ok: false,
      reason: "unsupported_scheme",
    });
  });

  it("rejects malformed URLs", () => {
    expect(normalizeBrowserAddressInput("https://exa mple.com")).toEqual({
      ok: false,
      reason: "invalid_url",
    });
  });
});

describe("parseNavigable and external schemes", () => {
  it("returns null for non-web input", () => {
    expect(parseNavigable("file:///etc/passwd")).toBeNull();
    expect(parseNavigable("")).toBeNull();
  });

  it("only escapes mailto and tel to the OS", () => {
    expect(safeExternalUrl("mailto:a@b.com")).toBe("mailto:a@b.com");
    expect(safeExternalUrl("tel:+15551212")).toBe("tel:+15551212");
    expect(safeExternalUrl("file:///tmp")).toBeNull();
    expect(safeExternalUrl("https://example.com")).toBeNull();
  });
});

describe("viewportBounds", () => {
  it("hides empty, negative or non-finite rects", () => {
    expect(viewportBounds(null)).toBeNull();
    expect(
      viewportBounds({ x: 10, y: 10, width: 0, height: 100 }),
    ).toBeNull();
    expect(
      viewportBounds({ x: Number.NaN, y: 0, width: 100, height: 100 }),
    ).toBeNull();
  });

  it("clamps and rounds a visible rect", () => {
    expect(
      viewportBounds({ x: -4.2, y: 8.6, width: 120.4, height: 80.2 }),
    ).toEqual({ x: 0, y: 9, width: 120, height: 80 });
  });
});

describe("browserActionAllowed", () => {
  it("forbids every action when the conversation view is hidden", () => {
    expect(
      browserActionAllowed("observe", { shown: false, hasViewport: true }),
    ).toBe(false);
    expect(
      browserActionAllowed("navigate", { shown: false, hasViewport: false }),
    ).toBe(false);
  });

  it("lets observe and navigate run without painted bounds", () => {
    expect(
      browserActionAllowed("observe", { shown: true, hasViewport: false }),
    ).toBe(true);
    expect(
      browserActionAllowed("navigate", { shown: true, hasViewport: false }),
    ).toBe(true);
    expect(
      browserActionAllowed("mutate", { shown: true, hasViewport: false }),
    ).toBe(false);
    expect(
      browserActionAllowed("mutate", { shown: true, hasViewport: true }),
    ).toBe(true);
  });
});

describe("deriveBrowserState", () => {
  it("hides the native page until a real URL is loaded", () => {
    expect(
      deriveBrowserState({
        url: "",
        title: "",
        canGoBack: false,
        canGoForward: false,
        loading: false,
      }).hasPage,
    ).toBe(false);
    expect(
      deriveBrowserState({
        url: "about:blank",
        title: "",
        canGoBack: false,
        canGoForward: false,
        loading: false,
      }).hasPage,
    ).toBe(false);
    const live = deriveBrowserState({
      url: "https://example.com/docs",
      title: "Docs",
      canGoBack: true,
      canGoForward: false,
      loading: true,
    });
    expect(live).toMatchObject({
      hasPage: true,
      secure: true,
      title: "Docs",
      canGoBack: true,
    });
  });
});

describe("originOf", () => {
  it("returns the origin for http(s) pages", () => {
    expect(originOf("https://example.com/a?b=1")).toBe("https://example.com");
    expect(originOf("not a url")).toBe("");
  });
});

describe("session isolation", () => {
  it("keeps the public web partition distinct from project HTML preview", () => {
    expect(EMBEDDED_BROWSER_PARTITION).toBe("persist:awaken-browser");
    expect(EMBEDDED_BROWSER_PARTITION.startsWith("awaken-preview")).toBe(false);
    expect(EMBEDDED_BROWSER_PARTITION.includes("preview")).toBe(false);
  });
});
