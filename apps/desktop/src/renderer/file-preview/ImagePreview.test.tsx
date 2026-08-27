// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ImagePreview, {
  imageFitScale,
  imagePointAtViewportCenter,
  nextImageZoomScale,
  scrollOffsetForCenteredImagePoint,
} from "./ImagePreview";

let canvasWidth = 1048;
let canvasHeight = 548;
let resizeCallback: ResizeObserverCallback | undefined;

function rect(
  left: number,
  top: number,
  width: number,
  height: number,
): DOMRect {
  return {
    x: left,
    y: top,
    left,
    top,
    right: left + width,
    bottom: top + height,
    width,
    height,
    toJSON: () => ({}),
  } as DOMRect;
}

class TestResizeObserver {
  constructor(callback: ResizeObserverCallback) {
    resizeCallback = callback;
  }

  observe(target: Element): void {
    resizeCallback?.(
      [
        {
          target,
          contentRect: rect(0, 0, canvasWidth, canvasHeight),
        } as unknown as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver,
    );
  }

  disconnect(): void {}
  unobserve(): void {}
}

function preview(previewId = "image-1") {
  return {
    previewId,
    requestId: previewId,
    kind: "image" as const,
    transport: "url" as const,
    displayName: `${previewId}.png`,
    relativePath: `${previewId}.png`,
    byteLength: 128,
    src: `awaken-preview://${previewId}/image.png`,
  };
}

describe("ImagePreview zoom geometry", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    canvasWidth = 1048;
    canvasHeight = 548;
    resizeCallback = undefined;
    vi.stubGlobal("ResizeObserver", TestResizeObserver);
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(
      function (this: HTMLElement) {
        return this.classList.contains("project-file-image-preview__canvas")
          ? canvasWidth
          : 0;
      },
    );
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockImplementation(
      function (this: HTMLElement) {
        return this.classList.contains("project-file-image-preview__canvas")
          ? canvasHeight
          : 0;
      },
    );
    vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
      function (this: Element) {
        if (this.classList.contains("project-file-image-preview__canvas")) {
          return rect(0, 0, canvasWidth, canvasHeight);
        }
        if (this instanceof HTMLImageElement) {
          const canvas = this.closest<HTMLDivElement>(
            ".project-file-image-preview__canvas",
          );
          const width = Number.parseFloat(this.style.width) || 0;
          const height = Number.parseFloat(this.style.height) || 0;
          const contentWidth = canvasWidth - 48;
          const contentHeight = canvasHeight - 48;
          const left =
            24 +
            Math.max(0, (contentWidth - width) / 2) -
            (canvas?.scrollLeft ?? 0);
          const top =
            24 +
            Math.max(0, (contentHeight - height) / 2) -
            (canvas?.scrollTop ?? 0);
          return rect(left, top, width, height);
        }
        return rect(0, 0, 0, 0);
      },
    );
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  async function renderLoadedImage(
    previewId = "image-1",
    naturalWidth = 2000,
    naturalHeight = 1000,
  ): Promise<HTMLImageElement> {
    await act(async () => {
      root.render(
        <ImagePreview
          preview={preview(previewId)}
          onLoadError={vi.fn()}
          frameProps={{
            displayName: `${previewId}.png`,
            onReload: vi.fn(),
            onOpenExternal: vi.fn(),
            onReveal: vi.fn(),
          }}
        />,
      );
    });
    const image = container.querySelector<HTMLImageElement>("img");
    expect(image).not.toBeNull();
    Object.defineProperty(image, "naturalWidth", {
      configurable: true,
      value: naturalWidth,
    });
    Object.defineProperty(image, "naturalHeight", {
      configurable: true,
      value: naturalHeight,
    });
    await act(async () => image?.dispatchEvent(new Event("load")));
    return image!;
  }

  it("reports the real fit ratio and zooms around the viewport center in small steps", async () => {
    const image = await renderLoadedImage();
    const canvas = container.querySelector<HTMLDivElement>(
      ".project-file-image-preview__canvas",
    );
    const scale = container.querySelector('[aria-label="图片缩放比例"]');
    const zoomIn = container.querySelector<HTMLButtonElement>(
      '[aria-label="放大图片"]',
    );
    const zoomOut = container.querySelector<HTMLButtonElement>(
      '[aria-label="缩小图片"]',
    );
    const fitAction = (): HTMLButtonElement | null =>
      container.querySelector<HTMLButtonElement>('[aria-label="适应窗口"]');

    expect(scale?.textContent).toBe("50%");
    expect(image.style.width).toBe("1000px");
    expect(image.style.height).toBe("500px");
    expect(image.style.transform).toBe("");
    expect(fitAction()?.title).toBe("适应窗口");
    expect(fitAction()?.getAttribute("aria-pressed")).toBe("true");
    expect(
      fitAction()?.querySelector("svg.aw-icon--corners-out"),
    ).not.toBeNull();

    await act(async () => zoomIn?.click());
    expect(scale?.textContent).toBe("55%");
    expect(image.style.width).toBe("1100px");
    expect(canvas?.scrollLeft).toBeCloseTo(50);
    expect(canvas?.scrollTop).toBeCloseTo(25);
    expect(fitAction()?.title).toBe("恢复适应窗口");
    expect(fitAction()?.getAttribute("aria-pressed")).toBe("false");
    expect(
      fitAction()?.querySelector("svg.aw-icon--corners-in"),
    ).not.toBeNull();

    await act(async () => zoomOut?.click());
    expect(scale?.textContent).toBe("50%");
    expect(canvas?.scrollLeft).toBeCloseTo(0);
    expect(canvas?.scrollTop).toBeCloseTo(0);
    expect(fitAction()?.title).toBe("适应窗口");
    expect(fitAction()?.getAttribute("aria-pressed")).toBe("false");
    expect(
      fitAction()?.querySelector("svg.aw-icon--corners-out"),
    ).not.toBeNull();

    await act(async () => zoomOut?.click());
    expect(scale?.textContent).toBe("45%");
    expect(fitAction()?.title).toBe("适应窗口");
    expect(
      fitAction()?.querySelector("svg.aw-icon--corners-out"),
    ).not.toBeNull();

    await act(async () => fitAction()?.click());
    expect(scale?.textContent).toBe("50%");
    expect(fitAction()?.getAttribute("aria-pressed")).toBe("true");

    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('[aria-label="原始尺寸"]')
        ?.click(),
    );
    expect(scale?.textContent).toBe("100%");
    expect(canvas?.scrollLeft).toBeCloseTo(500);
    expect(canvas?.scrollTop).toBeCloseTo(250);
    expect(fitAction()?.title).toBe("恢复适应窗口");
    expect(fitAction()?.getAttribute("aria-pressed")).toBe("false");
    expect(
      fitAction()?.querySelector("svg.aw-icon--corners-in"),
    ).not.toBeNull();

    await act(async () => fitAction()?.click());
    expect(scale?.textContent).toBe("50%");
    expect(canvas?.scrollLeft).toBe(0);
    expect(canvas?.scrollTop).toBe(0);
    expect(fitAction()?.getAttribute("aria-pressed")).toBe("true");
  });

  it("recomputes fit zoom on resize and resets zoom for another preview", async () => {
    await renderLoadedImage();
    const scale = container.querySelector('[aria-label="图片缩放比例"]');
    const canvas = container.querySelector<HTMLDivElement>(
      ".project-file-image-preview__canvas",
    );
    const fitAction = container.querySelector<HTMLButtonElement>(
      '[aria-label="适应窗口"]',
    );
    expect(scale?.textContent).toBe("50%");

    canvasWidth = 848;
    await act(async () => {
      resizeCallback?.(
        [
          {
            target: canvas!,
            contentRect: rect(0, 0, canvasWidth, canvasHeight),
          } as unknown as ResizeObserverEntry,
        ],
        {} as ResizeObserver,
      );
    });
    expect(scale?.textContent).toBe("40%");

    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('[aria-label="放大图片"]')
        ?.click(),
    );
    expect(scale?.textContent).toBe("44%");
    expect(fitAction?.title).toBe("恢复适应窗口");
    expect(
      fitAction?.querySelector("svg.aw-icon--corners-in"),
    ).not.toBeNull();

    canvasWidth = 1048;
    await act(async () => {
      resizeCallback?.(
        [
          {
            target: canvas!,
            contentRect: rect(0, 0, canvasWidth, canvasHeight),
          } as unknown as ResizeObserverEntry,
        ],
        {} as ResizeObserver,
      );
    });
    expect(scale?.textContent).toBe("44%");
    expect(fitAction?.title).toBe("适应窗口");
    expect(
      fitAction?.querySelector("svg.aw-icon--corners-out"),
    ).not.toBeNull();

    await act(async () => {
      root.render(
        <ImagePreview
          preview={preview("image-2")}
          onLoadError={vi.fn()}
          frameProps={{
            displayName: "image-2.png",
            onReload: vi.fn(),
            onOpenExternal: vi.fn(),
            onReveal: vi.fn(),
          }}
        />,
      );
    });
    expect(scale?.textContent).toBe("—");
    expect(canvas?.scrollLeft).toBe(0);
    expect(canvas?.scrollTop).toBe(0);
  });

  it("keeps an explicit 1:1 choice when a small image later loses space", async () => {
    const image = await renderLoadedImage("small-image", 400, 200);
    const scale = container.querySelector('[aria-label="图片缩放比例"]');
    const fit = container.querySelector<HTMLButtonElement>(
      '[aria-label="适应窗口"]',
    );
    expect(scale?.textContent).toBe("100%");
    expect(fit?.getAttribute("aria-pressed")).toBe("true");

    await act(async () =>
      container
        .querySelector<HTMLButtonElement>('[aria-label="原始尺寸"]')
        ?.click(),
    );
    expect(fit?.getAttribute("aria-pressed")).toBe("false");

    canvasWidth = 248;
    await act(async () => {
      resizeCallback?.(
        [
          {
            target: image.closest(".project-file-image-preview__canvas")!,
            contentRect: rect(0, 0, canvasWidth, canvasHeight),
          } as unknown as ResizeObserverEntry,
        ],
        {} as ResizeObserver,
      );
    });
    expect(scale?.textContent).toBe("100%");
    expect(image.style.width).toBe("400px");
  });
});

describe("ImagePreview zoom helpers", () => {
  it("uses intrinsic pixels for fit, relative steps, and centered scroll math", () => {
    expect(
      imageFitScale({
        imageWidth: 2000,
        imageHeight: 1000,
        viewportWidth: 1000,
        viewportHeight: 500,
      }),
    ).toBe(0.5);
    expect(nextImageZoomScale(0.5, "in")).toBeCloseTo(0.55);
    expect(nextImageZoomScale(0.55, "out")).toBeCloseTo(0.5);
    expect(
      imagePointAtViewportCenter({
        scrollOffset: 50,
        viewportSize: 1048,
        imageOffset: 24,
        scale: 0.55,
      }),
    ).toBeCloseTo(1000);
    expect(
      scrollOffsetForCenteredImagePoint({
        imagePoint: 1000,
        viewportSize: 1048,
        imageOffset: 24,
        scale: 0.55,
      }),
    ).toBeCloseTo(50);
  });
});
