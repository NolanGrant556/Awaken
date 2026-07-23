// @vitest-environment jsdom

import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import { AwakenMemorySettings, type AwakenMemoryActivityItem, type AwakenMemoryDetail } from "@awaken/design-system";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

describe("AwakenMemorySettings recent activity disclosure", () => {
  let container: HTMLDivElement | null = null;
  let root: Root | null = null;

  afterEach(() => {
    if (root) act(() => root?.unmount());
    container?.remove();
    root = null;
    container = null;
  });

  it("shows 6 activities initially, expands by 6 to 18, then collapses", () => {
    const activities: AwakenMemoryActivityItem[] = Array.from({ length: 20 }, (_, index) => ({
      id: `activity-${index + 1}`,
      kind: "memory",
      label: "用户记忆",
      title: `动态 ${index + 1}`,
      description: `动态 ${index + 1} 的完整内容`,
      timeLabel: `${index + 1} 分钟前`,
    }));
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);

    act(() => {
      root?.render(<AwakenMemorySettings switches={[]} filters={[]} query="" memories={[]} activities={activities} />);
    });

    const rowCount = () => container?.querySelectorAll(".memory-activity-row").length;
    const toggle = () => container?.querySelector<HTMLButtonElement>(".memory-activity-list-toggle") ?? null;

    expect(rowCount()).toBe(6);
    expect(container.textContent).toContain("动态 6");
    expect(container.textContent).not.toContain("动态 7");
    expect(toggle()?.textContent).toBe("显示更多 · 6 条");

    act(() => toggle()?.click());
    expect(rowCount()).toBe(12);
    expect(container.textContent).toContain("动态 12");
    expect(container.textContent).not.toContain("动态 13");
    expect(toggle()?.textContent).toBe("显示更多 · 6 条");

    act(() => toggle()?.click());
    expect(rowCount()).toBe(18);
    expect(container.textContent).toContain("动态 18");
    expect(container.textContent).not.toContain("动态 19");
    expect(toggle()?.textContent).toBe("收起");

    act(() => toggle()?.click());
    expect(rowCount()).toBe(6);
    expect(container.textContent).not.toContain("动态 7");
    expect(toggle()?.textContent).toBe("显示更多 · 6 条");
  });

  it("renders memory detail metadata as a compact separated row", () => {
    const detail: AwakenMemoryDetail = {
      id: "memory-1",
      name: "用户称呼：Nolan",
      description: "用户名叫 Nolan",
      scopeLabel: "用户记忆",
      typeLabel: "用户资料",
      statusLabel: "有效",
      status: "active",
      updatedLabel: "4 天前",
      kind: "user",
      content: "# 用户称呼\n\n用户的名字是 Nolan。",
      sourceThreadId: "thread-1",
      revision: 1,
    };
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);

    act(() => {
      root?.render(<AwakenMemorySettings switches={[]} filters={[]} query="" memories={[]} detail={detail} selectedMemoryId={detail.id} />);
    });

    const metadata = document.querySelector<HTMLElement>(".memory-detail-tags");
    expect(metadata?.getAttribute("aria-label")).toBe("用户记忆，用户资料，有效，4 天前");
    expect(Array.from(metadata?.querySelectorAll(":scope > span") ?? []).map((item) => item.textContent)).toEqual(["用户记忆", "用户资料", "有效", "4 天前"]);
    expect(metadata?.querySelectorAll(":scope > i")).toHaveLength(3);
    expect(metadata?.querySelector(".memory-detail-tag-active")?.textContent).toBe("有效");
  });
});
