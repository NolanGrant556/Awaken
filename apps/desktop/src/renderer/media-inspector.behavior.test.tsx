// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AwakenMediaInspector } from "@awaken/design-system";

describe("AwakenMediaInspector", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  it("shows exact media metadata and keeps video playback controls", () => {
    act(() => {
      root.render(
        <AwakenMediaInspector
          item={{
            id: "video-1",
            kind: "video",
            title: "成品视频",
            src: "awaken-artifact://media/video/final.mp4",
            metadata: { width: 1080, height: 1920, durationSeconds: 5.085 },
            formatLabel: "mp4",
          }}
          onClose={vi.fn()}
        />,
      );
    });

    expect(host.textContent).toContain("1080×1920 · 5.1 秒 · MP4");
    expect(host.querySelector("video")?.controls).toBe(true);
  });

  it("offers a bounded-retry hook when the host marks the source unavailable", () => {
    const onMediaError = vi.fn();
    act(() => {
      root.render(
        <AwakenMediaInspector
          item={{ id: "image-1", kind: "image", title: "图片" }}
          sourceUnavailable
          onClose={vi.fn()}
          onMediaError={onMediaError}
        />,
      );
    });

    const reload = [...host.querySelectorAll<HTMLButtonElement>("button")].find(
      (button) => button.textContent?.includes("重新加载"),
    );
    act(() => reload?.click());
    expect(onMediaError).toHaveBeenCalledTimes(1);
  });
});
