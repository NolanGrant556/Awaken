import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  AwakenDeliverableCard as DeliverableCard,
  AwakenDeliverableList as DeliverableList,
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
