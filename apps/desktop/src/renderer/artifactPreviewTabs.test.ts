import { describe, expect, it } from "vitest";

import {
  activeArtifactPreviewTab,
  artifactPreviewTabId,
  artifactPreviewTabsReducer,
  createArtifactPreviewTabsState,
} from "./artifactPreviewTabs";
import type { ArtifactPreviewSelection } from "./file-preview/ArtifactPreviewHost";

function artifact(
  artifactId: string,
  title = `${artifactId}.docx`,
): ArtifactPreviewSelection {
  return {
    threadId: "thread-1",
    runId: "run-1",
    artifactId,
    title,
  };
}

describe("artifactPreviewTabsReducer", () => {
  it("keeps an Artifact tab available while a project file becomes active", () => {
    const selection = artifact("artifact-1", "交付报告.docx");
    let state = artifactPreviewTabsReducer(createArtifactPreviewTabsState(), {
      type: "open",
      selection,
    });

    state = artifactPreviewTabsReducer(state, { type: "deactivate" });

    expect(state.tabs).toHaveLength(1);
    expect(state.tabs[0]?.selection.title).toBe("交付报告.docx");
    expect(activeArtifactPreviewTab(state)).toBeUndefined();

    state = artifactPreviewTabsReducer(state, {
      type: "activate",
      tabId: artifactPreviewTabId(selection),
    });
    expect(activeArtifactPreviewTab(state)?.selection).toEqual(selection);
  });

  it("deduplicates the durable Artifact identity and preserves multiple tabs", () => {
    const first = artifact("artifact-1", "第一份报告.docx");
    const second = artifact("artifact-2", "第二份报告.pdf");
    let state = artifactPreviewTabsReducer(createArtifactPreviewTabsState(), {
      type: "open",
      selection: first,
    });
    state = artifactPreviewTabsReducer(state, {
      type: "open",
      selection: second,
    });
    state = artifactPreviewTabsReducer(state, {
      type: "open",
      selection: { ...first, title: "第一份报告（更新）.docx" },
    });

    expect(state.tabs).toHaveLength(2);
    expect(state.tabs[0]?.selection.title).toBe("第一份报告（更新）.docx");
    expect(activeArtifactPreviewTab(state)?.id).toBe(
      artifactPreviewTabId(first),
    );
  });

  it("keeps media routing with the durable identity without storing an opaque source", () => {
    const image = { ...artifact("image-1", "主视觉.png"), mediaKind: "image" as const };
    const video = { ...artifact("video-1", "成片.mp4"), mediaKind: "video" as const };
    let state = artifactPreviewTabsReducer(createArtifactPreviewTabsState(), {
      type: "open",
      selection: image,
    });
    state = artifactPreviewTabsReducer(state, {
      type: "open",
      selection: video,
    });

    expect(state.tabs.map((tab) => tab.selection.mediaKind)).toEqual(["image", "video"]);
    expect(state.tabs.every((tab) => !("src" in tab.selection))).toBe(true);
    expect(activeArtifactPreviewTab(state)?.selection).toEqual(video);
  });

  it("selects the right Artifact neighbor, then the left, when closing", () => {
    const first = artifact("artifact-1");
    const second = artifact("artifact-2");
    const third = artifact("artifact-3");
    let state = [first, second, third].reduce(
      (current, selection) =>
        artifactPreviewTabsReducer(current, { type: "open", selection }),
      createArtifactPreviewTabsState(),
    );
    state = artifactPreviewTabsReducer(state, {
      type: "activate",
      tabId: artifactPreviewTabId(second),
    });
    state = artifactPreviewTabsReducer(state, {
      type: "close",
      tabId: artifactPreviewTabId(second),
    });
    expect(activeArtifactPreviewTab(state)?.id).toBe(
      artifactPreviewTabId(third),
    );

    state = artifactPreviewTabsReducer(state, {
      type: "close",
      tabId: artifactPreviewTabId(third),
    });
    expect(activeArtifactPreviewTab(state)?.id).toBe(
      artifactPreviewTabId(first),
    );
  });
});
