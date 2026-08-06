// @vitest-environment jsdom

import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { AwakenMediaPreviewOverlay } from "@awaken/design-system";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const items = [
  { id: "image-1", kind: "image" as const, title: "图片 1", src: "preview://1" },
  { id: "image-2", kind: "image" as const, title: "图片 2", src: "preview://2" },
];

function PreviewHarness() {
  const [index, setIndex] = useState(0);
  return (
    <AwakenMediaPreviewOverlay
      item={items[index]}
      position={index}
      count={items.length}
      onClose={vi.fn()}
      onPrevious={() => setIndex((current) => Math.max(0, current - 1))}
      onNext={() => setIndex((current) => Math.min(items.length - 1, current + 1))}
    />
  );
}

describe("AwakenMediaPreviewOverlay focus ownership", () => {
  let host: HTMLDivElement;
  let root: Root;
  let animationFrames: FrameRequestCallback[];

  beforeEach(() => {
    (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    animationFrames = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      animationFrames.push(callback);
      return animationFrames.length;
    });
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
    delete (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT?: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT;
  });

  function flushAnimationFrames(): void {
    const pending = animationFrames.splice(0);
    pending.forEach((callback) => callback(0));
  }

  it("keeps programmatic focus on the dialog when ArrowRight changes the image", () => {
    act(() => root.render(<PreviewHarness />));
    act(() => flushAnimationFrames());

    const dialog = host.querySelector<HTMLElement>(".aw-media-preview-dialog")!;
    const close = host.querySelector<HTMLButtonElement>(
      '[aria-label="关闭媒体预览"]',
    )!;
    expect(document.activeElement).toBe(dialog);
    expect(document.activeElement).not.toBe(close);

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
      );
    });
    act(() => flushAnimationFrames());

    expect(host.textContent).toContain("图片 2");
    expect(document.activeElement).toBe(dialog);
    expect(document.activeElement).not.toBe(close);
  });
});
