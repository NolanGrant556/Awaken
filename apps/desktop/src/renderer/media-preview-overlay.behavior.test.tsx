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

  it("attempts video playback, reports metadata, and asks the host to renew a failed source", async () => {
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
    const onMediaError = vi.fn();
    const onMetadata = vi.fn();
    act(() => {
      root.render(
        <AwakenMediaPreviewOverlay
          item={{ id: "video-1", kind: "video", title: "视频 1", src: "preview://video-1" }}
          position={0}
          count={1}
          onClose={vi.fn()}
          onMediaError={onMediaError}
          onMetadata={onMetadata}
        />,
      );
    });

    const video = host.querySelector<HTMLVideoElement>("video")!;
    Object.defineProperty(video, "videoWidth", { configurable: true, value: 1080 });
    Object.defineProperty(video, "videoHeight", { configurable: true, value: 1920 });
    Object.defineProperty(video, "duration", { configurable: true, value: 5.085 });
    await act(async () => video.dispatchEvent(new Event("loadedmetadata")));

    expect(play).toHaveBeenCalledTimes(1);
    expect(onMetadata).toHaveBeenCalledWith({
      width: 1080,
      height: 1920,
      durationSeconds: 5.085,
    });

    act(() => video.dispatchEvent(new Event("error")));
    expect(onMediaError).toHaveBeenCalledTimes(1);
    expect(host.textContent).toContain("正在重新载入视频预览");
  });

  it("shows an explicit play action when autoplay is rejected", async () => {
    vi.spyOn(HTMLMediaElement.prototype, "play").mockRejectedValue(new Error("blocked"));
    act(() => {
      root.render(
        <AwakenMediaPreviewOverlay
          item={{ id: "video-2", kind: "video", title: "视频 2", src: "preview://video-2" }}
          position={0}
          count={1}
          onClose={vi.fn()}
        />,
      );
    });

    await act(async () => {
      host.querySelector<HTMLVideoElement>("video")?.dispatchEvent(new Event("loadedmetadata"));
      await Promise.resolve();
    });
    expect(host.querySelector<HTMLButtonElement>(".aw-media-preview-play-fallback")?.textContent).toBe(
      "播放视频",
    );
  });
});
