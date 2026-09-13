// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import IsolatedHtmlPreview from "./IsolatedHtmlPreview";
import type { EmbeddedHtmlNavigation } from "./EmbeddedBrowserChrome";

describe("IsolatedHtmlPreview", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("loads file URLs in a non-persistent local-html partition", async () => {
    await act(async () => {
      root.render(
        <IsolatedHtmlPreview
          src="file:///tmp/index.html"
          partition="awaken-local-html-slot-1"
          onHttpNavigate={vi.fn()}
          onNavigationChange={vi.fn()}
        />,
      );
    });

    const webview = container.querySelector("webview");
    expect(webview?.getAttribute("src")).toBe("file:///tmp/index.html");
    expect(webview?.getAttribute("partition")).toBe("awaken-local-html-slot-1");
    expect(webview?.getAttribute("partition")?.startsWith("persist:")).toBe(
      false,
    );
    expect(webview?.getAttribute("webpreferences")).toContain(
      "allowFileAccessFromFileUrls=yes",
    );
  });

  it("hands http(s) navigations to the public web layer and keeps file links", async () => {
    const onHttpNavigate = vi.fn();
    let navigation: EmbeddedHtmlNavigation | undefined;
    await act(async () => {
      root.render(
        <IsolatedHtmlPreview
          src="file:///tmp/index.html"
          partition="awaken-local-html-slot-1"
          onHttpNavigate={onHttpNavigate}
          onNavigationChange={(next) => {
            navigation = next;
          }}
        />,
      );
    });

    const webview = container.querySelector("webview") as HTMLElement;
    const httpEvent = new Event("will-navigate", { cancelable: true });
    Object.defineProperty(httpEvent, "url", { value: "https://example.com/docs" });
    await act(async () => {
      webview.dispatchEvent(httpEvent);
    });
    expect(onHttpNavigate).toHaveBeenCalledWith("https://example.com/docs");
    expect(httpEvent.defaultPrevented).toBe(true);

    const fileEvent = new Event("will-navigate", { cancelable: true });
    Object.defineProperty(fileEvent, "url", {
      value: "file:///tmp/about.html",
    });
    await act(async () => {
      webview.dispatchEvent(fileEvent);
    });
    expect(onHttpNavigate).toHaveBeenCalledTimes(1);
    expect(fileEvent.defaultPrevented).toBe(false);

    const scriptEvent = new Event("will-navigate", { cancelable: true });
    Object.defineProperty(scriptEvent, "url", { value: "javascript:alert(1)" });
    await act(async () => {
      webview.dispatchEvent(scriptEvent);
    });
    expect(scriptEvent.defaultPrevented).toBe(true);
    expect(navigation?.address).toBe("file:///tmp/index.html");
  });
});
