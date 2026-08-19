import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  AwakenDeliverableCard as DeliverableCard,
  AwakenDeliverableList as DeliverableList,
  AwakenMediaGenerationCard as MediaGenerationCard,
  AwakenMediaPreviewOverlay as MediaPreviewOverlay,
  type AwakenDeliverableListItem,
} from "@awaken/design-system";

function items(count: number): AwakenDeliverableListItem[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `artifact-${index + 1}`,
    title: `交付文件-${index + 1}.xlsx`,
    extension: "xlsx",
    fileType: "excel" as const,
    onOpen: () => undefined,
    onReveal: () => undefined,
  }));
}

describe("final deliverable list", () => {
  it("renders an explicit PDF card with its dedicated icon and open semantics", () => {
    const html = renderToString(
      React.createElement(DeliverableCard, {
        title: "交付版本.pdf",
        extension: ".pdf",
        fileType: "pdf",
        onOpen: () => undefined,
        onReveal: () => undefined,
      }),
    );

    expect(html).toContain("aw-deliverable-card-pdf");
    expect(html).toContain('data-file-type="pdf"');
    expect(html).toContain("lucide-file-type2");
    expect(html).toContain(">PDF</span>");
    expect(html).toContain("PDF 文件");
    expect(html).toContain("在右侧栏预览 交付版本.pdf");
    expect(html).toContain("更多 交付版本.pdf 操作");
    expect(html).not.toContain("在 Finder/资源管理器中显示");
  });

  it("renders image and video delivery cards as independent compact media items", () => {
    const imageHtml = renderToString(
      React.createElement(DeliverableCard, {
        title: "主视觉.png",
        extension: "png",
        fileType: "image",
        previewSrc: "awaken-artifact://media/image/main.png",
        previewAspectRatio: 16 / 9,
        onPreview: () => undefined,
        onOpen: () => undefined,
      }),
    );
    const videoHtml = renderToString(
      React.createElement(DeliverableCard, {
        title: "成片.mp4",
        extension: "mp4",
        fileType: "video",
        previewSrc: "awaken-artifact://media/video/main.mp4",
        onPreview: () => undefined,
        onOpen: () => undefined,
      }),
    );

    expect(imageHtml).toContain("aw-media-item-card-surface");
    expect(imageHtml).toContain("--aw-media-item-aspect:1.7777777777777777");
    expect(imageHtml).toContain("<img");
    expect(imageHtml).toContain("主视觉.png");
    expect(videoHtml).toContain("<video");
    expect(videoHtml).not.toContain("controls");
    expect(videoHtml).toContain("成片.mp4");
  });

  it("surfaces unavailable media deliverables instead of an infinite ready skeleton", () => {
    const html = renderToString(
      React.createElement(DeliverableCard, {
        title: "失效图.png",
        extension: "png",
        fileType: "image",
        previewUnavailable: true,
        onOpen: () => undefined,
        onReveal: () => undefined,
      }),
    );

    expect(html).toContain('data-status="error"');
    expect(html).toContain("预览不可用");
    expect(html).not.toContain("<img");
  });

  it("renders a 15-image generation batch as 15 independent compact cards", () => {
    const html = renderToString(
      React.createElement(MediaGenerationCard, {
        kind: "image",
        status: "ready",
        title: "系列图",
        sources: Array.from({ length: 15 }, (_, index) => ({
          id: `image-${index + 1}`,
          src: `awaken-artifact://media/image/${index + 1}.png`,
        })),
      }),
    );

    expect(html).toContain('data-count="15"');
    expect(html).toContain("已生成 15 张图片");
    expect((html.match(/class="aw-media-item-card aw-media-item-card-image"/g) ?? [])).toHaveLength(15);
    expect(html).not.toContain("aw-media-generation-card-viewport");
  });

  it("renders 15 independent placeholder cards while a group request is running", () => {
    const html = renderToString(
      React.createElement(MediaGenerationCard, {
        kind: "image",
        status: "generating",
        requestedCount: 15,
      }),
    );

    expect(html).toContain('data-count="15"');
    expect(html).toContain("最多 15 张");
    expect((html.match(/class="aw-media-item-card aw-media-item-card-image"/g) ?? [])).toHaveLength(15);
    expect((html.match(/aw-media-item-card-loading-grid/g) ?? [])).toHaveLength(15);
  });

  it("keeps 15 independent terminal cards after a group request fails", () => {
    const html = renderToString(
      React.createElement(MediaGenerationCard, {
        kind: "image",
        status: "error",
        requestedCount: 15,
        detail: "图片视频生成服务请求失败。",
      }),
    );

    expect(html).toContain('data-count="15"');
    expect((html.match(/class="aw-media-item-card aw-media-item-card-image"/g) ?? [])).toHaveLength(15);
    expect((html.match(/aw-media-item-card-terminal/g) ?? [])).toHaveLength(15);
  });

  it("lays out media deliverables as compact grid items while documents stay full width", () => {
    const mediaItems: AwakenDeliverableListItem[] = Array.from({ length: 15 }, (_, index) => ({
      id: `image-${index + 1}`,
      title: `图片-${index + 1}.png`,
      extension: "png",
      fileType: "image",
      previewSrc: `awaken-artifact://media/image/${index + 1}.png`,
    }));
    const html = renderToString(
      React.createElement(DeliverableList, {
        items: [...mediaItems, ...items(1)],
        initiallyVisibleCount: 16,
      }),
    );

    expect((html.match(/aw-deliverable-list-item-image/g) ?? [])).toHaveLength(15);
    expect(html).toContain("aw-deliverable-list-item-excel");
  });

  it("keeps every media deliverable visible while document expansion remains capped", () => {
    const mediaItems: AwakenDeliverableListItem[] = Array.from({ length: 15 }, (_, index) => ({
      id: `image-${index + 1}`,
      title: `图片-${index + 1}.png`,
      extension: "png",
      fileType: "image",
      previewSrc: `awaken-artifact://media/image/${index + 1}.png`,
      onPreview: () => undefined,
    }));
    const html = renderToString(
      React.createElement(DeliverableList, {
        items: [...mediaItems, ...items(4)],
      }),
    );

    expect((html.match(/aw-deliverable-list-item-image/g) ?? [])).toHaveLength(15);
    expect((html.match(/aw-deliverable-list-item-excel/g) ?? [])).toHaveLength(3);
    expect(html).toContain("展开其余 1 个交付文件");
  });

  it("renders the in-stage media preview with platform actions and navigation", () => {
    const html = renderToString(
      React.createElement(MediaPreviewOverlay, {
        item: {
          id: "image-2",
          kind: "image",
          title: "图片-2.png",
          src: "awaken-artifact://media/image/2.png",
        },
        position: 1,
        count: 15,
        onClose: () => undefined,
        onPrevious: () => undefined,
        onNext: () => undefined,
        onOpenExternal: () => undefined,
        onReveal: () => undefined,
        revealLabel: "在文件资源管理器中显示",
      }),
    );

    expect(html).toContain('role="dialog"');
    expect(html).toContain(">2<!-- --> / <!-- -->15</span>");
    expect(html).toContain("适应窗口");
    expect(html).toContain("原始尺寸");
    expect(html).toContain("使用本地应用打开");
    expect(html).toContain("在文件资源管理器中显示");
  });

  it.each([0, 1, 3])("renders %i files without an expand control", (count) => {
    const html = renderToString(
      React.createElement(DeliverableList, { items: items(count) }),
    );
    expect((html.match(/aw-deliverable-card aw-/g) ?? [])).toHaveLength(count);
    expect(html).not.toContain("展开其余");
  });

  it.each([
    { count: 4, hidden: 1 },
    { count: 20, hidden: 17 },
  ])("shows three of $count files and exposes the hidden count", ({ count, hidden }) => {
    const html = renderToString(
      React.createElement(DeliverableList, { items: items(count) }),
    );
    expect((html.match(/aw-deliverable-card aw-/g) ?? [])).toHaveLength(3);
    expect(html).toContain(`展开其余 ${hidden} 个交付文件`);
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain("更多 交付文件-1.xlsx 操作");
    expect(html).not.toContain("在 Finder/资源管理器中显示");
  });

  it.each([
    ["error", "任务最终失败，但以下文件已成功生成"],
    ["cancelled", "任务已停止，但以下文件已成功生成"],
  ] as const)("explains retained files for %s runs", (terminalStatus, message) => {
    const html = renderToString(
      React.createElement(DeliverableList, {
        items: items(1),
        terminalStatus,
      }),
    );
    expect(html).toContain(message);
  });
});
