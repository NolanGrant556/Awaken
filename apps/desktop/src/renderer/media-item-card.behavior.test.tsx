// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AwakenMediaGenerationCard, AwakenMediaItemCard } from "@awaken/design-system";

describe("media item card behavior", () => {
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

  it("replaces the requested ratio with the image's real dimensions and enables preview", () => {
    const onPreview = vi.fn();
    act(() => {
      root.render(
        <AwakenMediaItemCard
          kind="image"
          status="ready"
          title="竖版图片"
          src="awaken-artifact://media/image/portrait.png"
          aspectRatio={4 / 3}
          onPreview={onPreview}
        />,
      );
    });

    const image = host.querySelector<HTMLImageElement>("img");
    expect(image).not.toBeNull();
    Object.defineProperty(image, "naturalWidth", { configurable: true, value: 900 });
    Object.defineProperty(image, "naturalHeight", { configurable: true, value: 1600 });
    act(() => image?.dispatchEvent(new Event("load")));

    const card = host.querySelector<HTMLElement>(".aw-media-item-card");
    const previewButton = host.querySelector<HTMLButtonElement>(".aw-media-item-card-surface");
    expect(card?.style.getPropertyValue("--aw-media-item-aspect")).toBe("0.5625");
    expect(previewButton?.disabled).toBe(false);
    act(() => previewButton?.click());
    expect(onPreview).toHaveBeenCalledTimes(1);
  });

  it("uses video metadata as the compact card ratio", () => {
    act(() => {
      root.render(
        <AwakenMediaItemCard
          kind="video"
          status="ready"
          title="竖版视频"
          src="awaken-artifact://media/video/portrait.mp4"
        />,
      );
    });

    const video = host.querySelector<HTMLVideoElement>("video");
    expect(video).not.toBeNull();
    Object.defineProperty(video, "videoWidth", { configurable: true, value: 1080 });
    Object.defineProperty(video, "videoHeight", { configurable: true, value: 1920 });
    act(() => video?.dispatchEvent(new Event("loadedmetadata")));

    const card = host.querySelector<HTMLElement>(".aw-media-item-card");
    expect(card?.style.getPropertyValue("--aw-media-item-aspect")).toBe("0.5625");
  });

  it("renders partial group results as independent ready and failed cards", () => {
    act(() => {
      root.render(
        <AwakenMediaGenerationCard
          kind="image"
          status="partial"
          requestedCount={3}
          sources={[
            { id: "one", index: 1, status: "ready", src: "awaken-artifact://one.png" },
            { id: "two", index: 2, status: "error" },
            { id: "three", index: 3, status: "ready", src: "awaken-artifact://three.png" },
          ]}
        />,
      );
    });

    const cards = [...host.querySelectorAll<HTMLElement>(".aw-media-item-card")];
    expect(cards).toHaveLength(3);
    expect(cards.map((card) => card.dataset.status)).toEqual(["ready", "error", "ready"]);
  });
});
