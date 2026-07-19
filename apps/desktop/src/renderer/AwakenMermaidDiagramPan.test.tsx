// @vitest-environment jsdom

import { act, createRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AwakenMermaidDiagram } from "@awaken/design-system";

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async (id: string) => ({
      svg: [
        `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" id="${id}" width="400" height="300" aria-labelledby="${id}-title">`,
        `<style>#${id} .edge{marker-end:url(#${id}-arrow);}</style>`,
        `<title id="${id}-title">mermaid</title>`,
        `<defs><marker id="${id}-arrow"></marker><path id="${id}-shape" d="M0 0h1v1z" /></defs>`,
        `<path class="edge" marker-end="url(#${id}-arrow)" d="M0 0h10" />`,
        `<use href="#${id}-shape" />`,
        `<use xlink:href="#${id}-shape" />`,
        "</svg>",
      ].join(""),
    })),
  },
}));

function pointerEvent(
  type: string,
  init: { clientX?: number; clientY?: number; pointerId?: number } = {},
) {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    button: 0,
    clientX: init.clientX ?? 0,
    clientY: init.clientY ?? 0,
  });
  Object.defineProperty(event, "pointerId", { value: init.pointerId ?? 1 });
  return event;
}

async function flush() {
  await act(async () => {
    await Promise.resolve();
  });
}

describe("AwakenMermaidDiagram expanded pan", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(async () => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => {
      root.render(<AwakenMermaidDiagram chart={"graph TD; A-->B"} />);
    });
    await flush();
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
    vi.restoreAllMocks();
  });

  async function expand() {
    const expandButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Expand diagram"]',
    );
    expect(expandButton).toBeTruthy();
    await act(async () => {
      expandButton!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();
  }

  function expandedScroll(): HTMLDivElement {
    const el = document.querySelector<HTMLDivElement>(
      ".assistant-mermaid-expanded-scroll",
    );
    expect(el).toBeTruthy();
    return el!;
  }

  function zoomFrame(): HTMLDivElement {
    const el = document.querySelector<HTMLDivElement>(
      ".assistant-mermaid-expanded-zoom-frame",
    );
    expect(el).toBeTruthy();
    return el!;
  }

  function svgIds(svg: SVGSVGElement): string[] {
    return [svg.id, ...Array.from(svg.querySelectorAll<SVGElement>("[id]"), (element) => element.id)];
  }

  function expectInternalReferencesToResolve(svg: SVGSVGElement) {
    const ids = svgIds(svg);
    expect(ids.every((id) => !/[.:]/.test(id))).toBe(true);
    const labelledBy = svg.getAttribute("aria-labelledby");
    expect(labelledBy).toBeTruthy();
    expect(ids).toContain(labelledBy);

    const markerReference = svg
      .querySelector<SVGPathElement>("path.edge")
      ?.getAttribute("marker-end")
      ?.match(/^url\(#(.+)\)$/)?.[1];
    expect(markerReference).toBeTruthy();
    expect(ids).toContain(markerReference);

    const useReferences = Array.from(svg.querySelectorAll<SVGUseElement>("use"), (element) =>
      (element.getAttribute("href")
        ?? element.getAttributeNS("http://www.w3.org/1999/xlink", "href"))?.slice(1),
    );
    expect(useReferences).toHaveLength(2);
    useReferences.forEach((useReference) => {
      expect(useReference).toBeTruthy();
      expect(ids).toContain(useReference);
    });

    const styleText = svg.querySelector("style")?.textContent ?? "";
    expect(styleText).toContain(`#${svg.id} .edge`);
    expect(styleText.match(/#[^\s,{]+/g)?.every((selector) => !/[.:]/.test(selector))).toBe(true);
  }

  async function dragBy(dx: number, dy: number) {
    const scroll = expandedScroll();
    await act(async () => {
      scroll.dispatchEvent(pointerEvent("pointerdown", { clientX: 100, clientY: 100 }));
    });
    await act(async () => {
      window.dispatchEvent(
        pointerEvent("pointermove", { clientX: 100 + dx, clientY: 100 + dy }),
      );
    });
  }

  it("pans the zoom frame on drag, even when the diagram fits the viewport", async () => {
    const parseSpy = vi.spyOn(DOMParser.prototype, "parseFromString");
    await expand();
    await dragBy(40, -25);

    expect(zoomFrame().style.transform).toBe("translate(40px, -25px)");

    const renderedSvgs = Array.from(
      document.querySelectorAll<SVGSVGElement>(".assistant-mermaid-diagram-svg > svg"),
    );
    const renderedIds = renderedSvgs.flatMap(svgIds);
    expect(renderedSvgs).toHaveLength(2);
    expect(new Set(renderedIds).size).toBe(renderedIds.length);
    renderedSvgs.forEach(expectInternalReferencesToResolve);
    expect(parseSpy).toHaveBeenCalledTimes(1);

    const closeButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Close expanded diagram"]',
    );
    expect(closeButton).toBeTruthy();
    expect(closeButton?.getAttribute("title")).toBeNull();
    await act(async () => closeButton?.focus());
    expect(document.querySelector('[role="tooltip"]')).toBeNull();
    await act(async () => {
      closeButton!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(parseSpy).toHaveBeenCalledTimes(1);
    parseSpy.mockRestore();
  });

  it("keeps panning after zooming with the slider", async () => {
    await expand();

    const slider = document.querySelector<HTMLInputElement>(
      ".assistant-mermaid-zoom-range",
    );
    expect(slider).toBeTruthy();
    await act(async () => {
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        window.HTMLInputElement.prototype,
        "value",
      )!.set!;
      nativeInputValueSetter.call(slider, "2");
      slider!.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await flush();

    await dragBy(60, 30);

    expect(zoomFrame().style.transform).toBe("translate(60px, 30px)");
  });

  it("reuses the cached svg on remount without a loading flash", async () => {
    const { default: mermaid } = await import("mermaid");
    const renderMock = vi.mocked(mermaid.render);
    const callsBefore = renderMock.mock.calls.length;
    const originalSvg = container.querySelector<SVGSVGElement>(
      ".assistant-mermaid-diagram-svg > svg",
    );
    expect(originalSvg).toBeTruthy();

    const remountContainer = document.createElement("div");
    document.body.appendChild(remountContainer);
    const remountRoot = createRoot(remountContainer);
    await act(async () => {
      remountRoot.render(<AwakenMermaidDiagram chart={"graph TD; A-->B"} />);
    });

    expect(remountContainer.textContent).not.toContain("Rendering diagram");
    const remountedSvg = remountContainer.querySelector<SVGSVGElement>(
      ".assistant-mermaid-diagram-svg > svg",
    );
    expect(remountedSvg).toBeTruthy();
    expect(remountedSvg!.id).not.toBe(originalSvg!.id);
    expectInternalReferencesToResolve(remountedSvg!);
    expect(renderMock.mock.calls.length).toBe(callsBefore);

    act(() => {
      remountRoot.unmount();
    });
    remountContainer.remove();
  });

  it("scopes cached svg ids for two identical diagrams mounted together", async () => {
    const { default: mermaid } = await import("mermaid");
    const renderMock = vi.mocked(mermaid.render);
    const callsBefore = renderMock.mock.calls.length;
    const twinContainer = document.createElement("div");
    document.body.appendChild(twinContainer);
    const twinRoot = createRoot(twinContainer);

    await act(async () => {
      twinRoot.render(
        <>
          <AwakenMermaidDiagram chart={"graph TD; A-->B"} />
          <AwakenMermaidDiagram chart={"graph TD; A-->B"} />
        </>,
      );
    });

    expect(twinContainer.textContent).not.toContain("Rendering diagram");
    const diagrams = Array.from(
      twinContainer.querySelectorAll<SVGSVGElement>(".assistant-mermaid-diagram-svg > svg"),
    );
    expect(diagrams).toHaveLength(2);

    const allIds = diagrams.flatMap(svgIds);
    expect(new Set(allIds).size).toBe(allIds.length);
    diagrams.forEach(expectInternalReferencesToResolve);
    expect(renderMock.mock.calls.length).toBe(callsBefore);

    act(() => {
      twinRoot.unmount();
    });
    twinContainer.remove();
  });
});
