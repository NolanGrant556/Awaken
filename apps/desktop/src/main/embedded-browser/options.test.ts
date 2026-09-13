import { describe, expect, it } from "vitest";
import type { WebPreferences } from "electron";
import {
  EMBEDDED_BROWSER_PARTITION,
  localHtmlPartition,
} from "../../shared/embedded-browser.js";
import { authorizeEmbeddedBrowserWebview } from "./options.js";

function prefs(): WebPreferences {
  return {
    sandbox: false,
    contextIsolation: false,
    nodeIntegration: true,
    webSecurity: false,
    plugins: true,
    preload: "/tmp/evil.js",
  } as WebPreferences;
}

describe("authorizeEmbeddedBrowserWebview", () => {
  it("allows persist http pages and hardens preferences", () => {
    const webPreferences = prefs();
    expect(
      authorizeEmbeddedBrowserWebview(webPreferences, {
        src: "https://example.com/",
        partition: EMBEDDED_BROWSER_PARTITION,
      }),
    ).toBe(true);
    expect(webPreferences.preload).toBeUndefined();
    expect(webPreferences.nodeIntegration).toBe(false);
    expect(webPreferences.sandbox).toBe(true);
    expect(webPreferences.webSecurity).toBe(true);
  });

  it("allows local HTML only on the isolated file partition", () => {
    const webPreferences = prefs();
    expect(
      authorizeEmbeddedBrowserWebview(webPreferences, {
        src: "file:///tmp/index.html",
        partition: localHtmlPartition("slot:1"),
      }),
    ).toBe(true);
    expect(
      (webPreferences as { allowFileAccessFromFileUrls?: boolean })
        .allowFileAccessFromFileUrls,
    ).toBe(true);
    expect(webPreferences.webSecurity).toBe(false);

    expect(
      authorizeEmbeddedBrowserWebview(prefs(), {
        src: "file:///tmp/index.html",
        partition: EMBEDDED_BROWSER_PARTITION,
      }),
    ).toBe(false);
    expect(
      authorizeEmbeddedBrowserWebview(prefs(), {
        src: "https://example.com/",
        partition: localHtmlPartition("slot:1"),
      }),
    ).toBe(false);
  });

  it("rejects office preview partitions and javascript", () => {
    expect(
      authorizeEmbeddedBrowserWebview(prefs(), {
        src: "awaken-preview://id/index.html",
        partition: "awaken-preview-id",
      }),
    ).toBe(false);
    expect(
      authorizeEmbeddedBrowserWebview(prefs(), {
        src: "javascript:alert(1)",
        partition: EMBEDDED_BROWSER_PARTITION,
      }),
    ).toBe(false);
  });
});
