// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WebviewPreview from "./WebviewPreview";

describe("WebviewPreview", () => {
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

  it("does not call guest methods while callbacks rerender or the webview detaches", async () => {
    const preview = {
      previewId: "preview-office",
      requestId: "request-office",
      kind: "office" as const,
      transport: "url" as const,
      displayName: "report.docx",
      relativePath: "report.docx",
      byteLength: 128,
      src: "awaken-preview://preview-office/report.html",
      partition: "awaken-preview-preview-office",
    };
    const frameProps = {
      displayName: preview.displayName,
      onReload: vi.fn(),
      onOpenExternal: vi.fn(),
      onReveal: vi.fn(),
    };

    await act(async () => {
      root.render(
        <WebviewPreview
          preview={preview}
          frameProps={frameProps}
          onLoadError={vi.fn()}
        />,
      );
    });

    const webview = container.querySelector("webview") as HTMLElement & {
      stop?: () => void;
    };
    const stop = vi.fn(() => {
      throw new Error("detached guest method");
    });
    webview.stop = stop;

    await act(async () => {
      root.render(
        <WebviewPreview
          preview={preview}
          frameProps={frameProps}
          onLoadingChange={vi.fn()}
          onLoadError={vi.fn()}
        />,
      );
    });

    expect(container.querySelector("webview")).toBe(webview);
    expect(webview.hasAttribute("plugins")).toBe(false);
    expect(stop).not.toHaveBeenCalled();
  });
});
