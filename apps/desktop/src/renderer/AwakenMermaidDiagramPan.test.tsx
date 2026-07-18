// @vitest-environment jsdom

import { act, createRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AwakenMermaidDiagram } from "@awaken/design-system";

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async () => ({
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="400" height="300"><text>mermaid</text></svg>',
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
    await expand();
    await dragBy(40, -25);

    expect(zoomFrame().style.transform).toBe("translate(40px, -25px)");
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

    const remountContainer = document.createElement("div");
    document.body.appendChild(remountContainer);
    const remountRoot = createRoot(remountContainer);
    await act(async () => {
      remountRoot.render(<AwakenMermaidDiagram chart={"graph TD; A-->B"} />);
    });

    expect(remountContainer.textContent).not.toContain("Rendering diagram");
    expect(
      remountContainer.querySelector(".assistant-mermaid-diagram-svg svg"),
    ).toBeTruthy();
    expect(renderMock.mock.calls.length).toBe(callsBefore);

    act(() => {
      remountRoot.unmount();
    });
    remountContainer.remove();
  });
});
