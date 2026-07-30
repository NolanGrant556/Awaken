import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  AwakenDeliverableCard as DeliverableCard,
  AwakenDeliverableList as DeliverableList,
  AwakenMediaGenerationCard as MediaGenerationCard,
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
    expect(html).toContain("打开 交付版本.pdf");
    expect(html).toContain("在 Finder/资源管理器中显示");
  });

  it("renders image and video delivery cards with inline media surfaces", () => {
    const imageHtml = renderToString(
      React.createElement(DeliverableCard, {
        title: "主视觉.png",
        extension: "png",
        fileType: "image",
        previewSrc: "awaken-artifact://media/image/main.png",
        onOpen: () => undefined,
      }),
    );
    const videoHtml = renderToString(
      React.createElement(DeliverableCard, {
        title: "成片.mp4",
        extension: "mp4",
        fileType: "video",
        previewSrc: "awaken-artifact://media/video/main.mp4",
        onOpen: () => undefined,
      }),
    );

    expect(imageHtml).toContain("aw-deliverable-card-media-preview");
    expect(imageHtml).toContain("<img");
    expect(imageHtml).toContain("主视觉.png");
    expect(videoHtml).toContain("<video");
    expect(videoHtml).toContain("controls");
    expect(videoHtml).toContain("成片.mp4");
  });

  it("renders a 15-image generation batch inside one compact gallery", () => {
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

    expect(html).toContain('data-group="true"');
    expect(html).toContain("已生成 15 张图片");
    expect(html).toContain("--aw-media-gallery-columns:5");
    expect((html.match(/class="aw-media-generation-card-item"/g) ?? [])).toHaveLength(15);
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
    expect(html).toContain("在 Finder/资源管理器中显示");
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
