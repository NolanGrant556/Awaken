// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import PdfPreview from "./PdfPreview";

const pdfMocks = vi.hoisted(() => {
  const renderTask = {
    promise: Promise.resolve(),
    cancel: vi.fn(),
  };
  const page = {
    getViewport: vi.fn(({ scale }: { scale: number }) => ({
      width: 600 * scale,
      height: 800 * scale,
    })),
    render: vi.fn(() => renderTask),
    cleanup: vi.fn(),
  };
  const documentProxy = {
    numPages: 3,
    getPage: vi.fn(async (_pageNumber: number) => page),
  };
  const loadingTask = {
    promise: Promise.resolve(documentProxy),
    destroy: vi.fn(async () => undefined),
  };
  return {
    documentProxy,
    getDocument: vi.fn(() => loadingTask),
    loadingTask,
    page,
    renderTask,
  };
});

vi.mock("pdfjs-dist", () => ({
  getDocument: pdfMocks.getDocument,
  GlobalWorkerOptions: { workerSrc: "" },
  RenderingCancelledException: class RenderingCancelledException extends Error {},
}));

class TestResizeObserver {
  constructor(private readonly callback: ResizeObserverCallback) {}

  observe(target: Element) {
    this.callback(
      [{ target, contentRect: { width: 720 } } as ResizeObserverEntry],
      this as unknown as ResizeObserver,
    );
  }

  disconnect() {}
  unobserve() {}
}

async function settleUntil(isReady: () => boolean): Promise<void> {
  await act(async () => {
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 10));
      if (isReady()) return;
    }
  });
}

describe("PdfPreview", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    vi.stubGlobal("ResizeObserver", TestResizeObserver);
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(720);
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    pdfMocks.documentProxy.getPage.mockClear();
    pdfMocks.page.getViewport.mockClear();
    pdfMocks.page.render.mockClear();
    pdfMocks.page.cleanup.mockClear();
    pdfMocks.renderTask.cancel.mockClear();
    pdfMocks.loadingTask.destroy.mockClear();
  });

  it("loads the opaque PDF URL, renders a page, and navigates pages", async () => {
    const onLoadingChange = vi.fn();
    await act(async () => {
      root.render(
        <PdfPreview
          preview={{
            previewId: "preview-pdf",
            requestId: "request-pdf",
            kind: "pdf",
            transport: "url",
            displayName: "manual.pdf",
            relativePath: "manual.pdf",
            byteLength: 128,
            src: "awaken-preview://preview-pdf/document.pdf",
          }}
          frameProps={{
            displayName: "manual.pdf",
            onReload: vi.fn(),
            onOpenExternal: vi.fn(),
            onReveal: vi.fn(),
          }}
          onLoadingChange={onLoadingChange}
          onLoadError={vi.fn()}
        />,
      );
    });
    await settleUntil(() => pdfMocks.page.render.mock.calls.length > 0);

    expect(pdfMocks.getDocument).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "awaken-preview://preview-pdf/document.pdf",
        rangeChunkSize: 64 * 1024,
      }),
    );
    expect(pdfMocks.documentProxy.getPage).toHaveBeenCalledWith(1);
    expect(
      container.querySelector('canvas[aria-label="PDF 第 1 页，共 3 页"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[aria-label="缩放比例"]')?.textContent,
    ).toBe("适宽");
    expect(
      container
        .querySelector('[aria-label="PDF 查看工具栏"]')
        ?.closest(".project-file-preview-heading"),
    ).not.toBeNull();
    expect(container.querySelector(".project-file-preview-toolbar")).toBeNull();
    expect(onLoadingChange).toHaveBeenLastCalledWith(false);

    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="下一页"]')
        ?.click();
    });
    await settleUntil(() =>
      pdfMocks.documentProxy.getPage.mock.calls.some(
        ([pageNumber]) => pageNumber === 2,
      ),
    );

    expect(
      container.querySelector('canvas[aria-label="PDF 第 2 页，共 3 页"]'),
    ).not.toBeNull();
  });
});
