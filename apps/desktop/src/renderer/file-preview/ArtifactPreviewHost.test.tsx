// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ArtifactPreviewHost from "./ArtifactPreviewHost";

describe("ArtifactPreviewHost media tabs", () => {
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

  it("renders a video from the existing media source without starting awaken-preview", () => {
    const startArtifactPreview = vi.fn();
    const onMediaMetadata = vi.fn();
    const runtimeClient = {
      startArtifactPreview,
      closeProjectFilePreview: vi.fn(async () => undefined),
      openArtifact: vi.fn(async () => ({ ok: true as const })),
      revealArtifact: vi.fn(async () => ({ ok: true as const })),
    };

    act(() => {
      root.render(
        <ArtifactPreviewHost
          runtimeClient={runtimeClient}
          selection={{
            threadId: "thread-1",
            runId: "run-1",
            artifactId: "video-1",
            title: "成片.mp4",
            mediaKind: "video",
          }}
          mediaSource="awaken-artifact://media/video-1/clip.mp4"
          mediaMetadata={{ width: 1080, height: 1920, durationSeconds: 5.085 }}
          revealLabel="在文件资源管理器中显示"
          onMediaMetadata={onMediaMetadata}
        />,
      );
    });

    const video = host.querySelector<HTMLVideoElement>("video");
    expect(startArtifactPreview).not.toHaveBeenCalled();
    expect(video?.controls).toBe(true);
    expect(video?.playsInline).toBe(true);
    expect(video?.preload).toBe("metadata");
    expect(host.textContent).toContain("1080×1920 · 5.1 秒 · MP4");
    expect(
      host.querySelector<HTMLButtonElement>('[aria-label="在文件资源管理器中显示"]'),
    ).not.toBeNull();

    Object.defineProperty(video, "videoWidth", { configurable: true, value: 1920 });
    Object.defineProperty(video, "videoHeight", { configurable: true, value: 1080 });
    Object.defineProperty(video, "duration", { configurable: true, value: 12.4 });
    act(() => video?.dispatchEvent(new Event("loadedmetadata")));
    expect(onMediaMetadata).toHaveBeenCalledWith({
      width: 1920,
      height: 1080,
      durationSeconds: 12.4,
    });
  });

  it("uses the host-owned bounded refresh callback for an unavailable media source", () => {
    const onMediaError = vi.fn();
    act(() => {
      root.render(
        <ArtifactPreviewHost
          runtimeClient={{
            startArtifactPreview: vi.fn(),
            closeProjectFilePreview: vi.fn(async () => undefined),
            openArtifact: vi.fn(async () => ({ ok: true as const })),
            revealArtifact: vi.fn(async () => ({ ok: true as const })),
          }}
          selection={{
            threadId: "thread-1",
            runId: "run-1",
            artifactId: "image-1",
            title: "主视觉.png",
            mediaKind: "image",
          }}
          mediaSourceUnavailable
          revealLabel="在文件资源管理器中显示"
          onMediaError={onMediaError}
        />,
      );
    });

    const retry = [...host.querySelectorAll<HTMLButtonElement>("button")].find(
      (button) => button.textContent?.includes("重试"),
    );
    act(() => retry?.click());
    expect(onMediaError).toHaveBeenCalledOnce();
    expect(host.textContent).toContain("在文件资源管理器中显示");
  });
});
