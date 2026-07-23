// @vitest-environment jsdom

import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AwakenMemorySettings, type AwakenMemoryActivityItem, type AwakenMemoryDetail, type AwakenMemoryListItem } from "@awaken/design-system";

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
      root?.render(<AwakenMemorySettings switches={[]} memories={[]} activities={activities} />);
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

  it("only disables the setting being saved and reserves Dream busy for update memory", () => {
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    const switches = [
      { id: "enabled", title: "记忆", description: "记忆总开关", checked: true },
      { id: "automaticRecall", title: "自动召回", description: "自动召回记忆", checked: true },
      { id: "proactiveMemory", title: "主动记忆", description: "主动保存记忆", checked: true },
      { id: "autoDream", title: "Auto Dream", description: "自动整理记忆", checked: false },
    ];

    act(() => {
      root?.render(
        <AwakenMemorySettings
          switches={switches}
          memories={[]}
          busy={false}
          busySwitchId="autoDream"
          dreamBusy={false}
        />,
      );
    });

    const memorySwitch = container.querySelector<HTMLButtonElement>('button[aria-label="关闭记忆"]');
    const recallSwitch = container.querySelector<HTMLButtonElement>('button[aria-label="关闭自动召回"]');
    const proactiveSwitch = container.querySelector<HTMLButtonElement>('button[aria-label="关闭主动记忆"]');
    const autoDreamSwitch = container.querySelector<HTMLButtonElement>('button[aria-label="开启Auto Dream"]');
    const updateMemoryButton = container.querySelector<HTMLButtonElement>(".memory-dream-button");
    expect(memorySwitch?.disabled).toBe(false);
    expect(recallSwitch?.disabled).toBe(false);
    expect(proactiveSwitch?.disabled).toBe(false);
    expect(autoDreamSwitch?.disabled).toBe(true);
    expect(autoDreamSwitch?.getAttribute("aria-busy")).toBe("true");
    expect(updateMemoryButton?.disabled).toBe(false);

    act(() => {
      root?.render(
        <AwakenMemorySettings
          switches={switches}
          memories={[]}
          busy={false}
          busySwitchId={null}
          dreamBusy
        />,
      );
    });

    expect(autoDreamSwitch?.disabled).toBe(false);
    expect(autoDreamSwitch?.hasAttribute("aria-busy")).toBe(false);
    expect(updateMemoryButton?.disabled).toBe(true);
  });

  it("keeps the master switch available while dependent switches and manual Dream are disabled", () => {
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);

    act(() => {
      root?.render(
        <AwakenMemorySettings
          switches={[
            { id: "enabled", title: "记忆", description: "记忆总开关", checked: false },
            { id: "automaticRecall", title: "自动召回", description: "自动召回记忆", checked: false, disabled: true },
            { id: "proactiveMemory", title: "主动记忆", description: "主动保存记忆", checked: false, disabled: true },
            { id: "autoDream", title: "Auto Dream", description: "自动整理记忆", checked: false, disabled: true },
          ]}
          memories={[]}
          canStartDream={false}
        />,
      );
    });

    expect(container.querySelector<HTMLButtonElement>('button[aria-label="开启记忆"]')?.disabled).toBe(false);
    expect(container.querySelector<HTMLButtonElement>('button[aria-label="开启自动召回"]')?.disabled).toBe(true);
    expect(container.querySelector<HTMLButtonElement>('button[aria-label="开启主动记忆"]')?.disabled).toBe(true);
    expect(container.querySelector<HTMLButtonElement>('button[aria-label="开启Auto Dream"]')?.disabled).toBe(true);
    expect(container.querySelector<HTMLButtonElement>(".memory-dream-button")?.disabled).toBe(true);
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
      root?.render(<AwakenMemorySettings switches={[]} memories={[]} detail={detail} selectedMemoryId={detail.id} />);
    });

    const metadata = document.querySelector<HTMLElement>(".memory-detail-tags");
    expect(metadata?.getAttribute("aria-label")).toBe("用户记忆，用户资料，有效，4 天前");
    expect(Array.from(metadata?.querySelectorAll(":scope > span") ?? []).map((item) => item.textContent)).toEqual(["用户记忆", "用户资料", "有效", "4 天前"]);
    expect(metadata?.querySelectorAll(":scope > i")).toHaveLength(3);
    expect(metadata?.querySelector(".memory-detail-tag-active")?.textContent).toBe("有效");
  });

  it("opens user topics, drills into project Markdown, and returns to project folders on close", () => {
    const memory: AwakenMemoryListItem = {
      id: "project-memory",
      name: "项目边界",
      description: "项目记忆正文摘要",
      scopeLabel: "项目记忆",
      typeLabel: "项目背景",
      statusLabel: "有效",
      status: "active",
      updatedLabel: "刚刚",
      kind: "project",
      projectId: "project-1",
    };
    const onOpenLibraryScope = vi.fn();
    const onOpenMemory = vi.fn();
    const onCloseMemory = vi.fn();
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);

    act(() => {
      root?.render(
        <AwakenMemorySettings
          switches={[]}
          memories={[memory]}
          librarySummary={{ userTopicCount: 2, projectCount: 1, projectTopicCount: 1 }}
          memoryProjects={[{ id: "project-1", name: "这是一个名称很长的项目记忆文件夹", topicCount: 1, updatedLabel: "刚刚" }]}
          onOpenLibraryScope={onOpenLibraryScope}
          onOpenMemory={onOpenMemory}
          onCloseMemory={onCloseMemory}
        />,
      );
    });

    const entries = container.querySelectorAll<HTMLButtonElement>(".memory-library-entry");
    expect(entries).toHaveLength(2);

    act(() => entries[0]?.click());
    expect(onOpenLibraryScope).toHaveBeenLastCalledWith("user", undefined);
    expect(document.querySelector<HTMLInputElement>('input[aria-label="搜索记忆"]')).toBeNull();
    expect(document.querySelector<HTMLButtonElement>(".memory-filter-panel-trigger")).toBeNull();
    expect(document.querySelector(".memory-topic-row")?.textContent).toContain("项目边界");

    act(() => document.querySelector<HTMLButtonElement>('.memory-library-dialog button[aria-label="关闭"]')?.click());
    act(() => entries[1]?.click());
    const persistentLibraryDialog = document.querySelector<HTMLElement>(".memory-library-dialog");
    const persistentBackdrop = persistentLibraryDialog?.closest(".aw-dialog-backdrop");
    const projectFolder = document.querySelector<HTMLButtonElement>(".aw-project-skill-folder");
    expect(projectFolder?.textContent).toContain("这是一个名称很长的项目记忆文件夹");
    expect(projectFolder?.textContent).toContain("1 个主题");
    expect(projectFolder?.dataset.size).toBe("compact");
    expect(projectFolder?.title).toBe("这是一个名称很长的项目记忆文件夹");
    expect(projectFolder?.getAttribute("aria-label")).toBe("查看这是一个名称很长的项目记忆文件夹的项目记忆");
    expect(document.querySelector(".memory-project-row")).toBeNull();
    expect(document.querySelector<HTMLInputElement>('input[aria-label="搜索记忆"]')).toBeNull();

    act(() => projectFolder?.click());
    expect(onOpenLibraryScope).toHaveBeenLastCalledWith("project", "project-1");
    expect(document.querySelector<HTMLInputElement>('input[aria-label="搜索记忆"]')).toBeNull();

    act(() => document.querySelector<HTMLButtonElement>(".memory-topic-row > button")?.click());
    expect(onOpenMemory).toHaveBeenCalledWith("project-memory", "project-1");
    expect(document.querySelector(".memory-library-dialog")).toBe(persistentLibraryDialog);
    expect(document.querySelector(".memory-library-dialog")?.closest(".aw-dialog-backdrop")).toBe(persistentBackdrop);
    expect(document.querySelector(".memory-topic-row")?.textContent).toContain("项目边界");

    const detail: AwakenMemoryDetail = {
      ...memory,
      content: "# 项目边界\n\nDesign System 负责纯 UI。",
      sourceThreadId: "thread-project-1",
      revision: 1,
    };
    act(() => {
      root?.render(
        <AwakenMemorySettings
          switches={[]}
          memories={[memory]}
          librarySummary={{ userTopicCount: 2, projectCount: 1, projectTopicCount: 1 }}
          memoryProjects={[{ id: "project-1", name: "这是一个名称很长的项目记忆文件夹", topicCount: 1, updatedLabel: "刚刚" }]}
          selectedMemoryId={detail.id}
          detail={detail}
          onOpenLibraryScope={onOpenLibraryScope}
          onOpenMemory={onOpenMemory}
          onCloseMemory={onCloseMemory}
        />,
      );
    });

    expect(document.querySelector(".memory-library-dialog")).toBe(persistentLibraryDialog);
    expect(document.querySelector(".memory-library-dialog")?.closest(".aw-dialog-backdrop")).toBe(persistentBackdrop);
    expect(document.querySelector(".memory-library-dialog")?.textContent).toContain("Design System 负责纯 UI。");
    expect(document.querySelector(".memory-detail-dialog")).toBeNull();
    act(() => document.querySelector<HTMLButtonElement>('.memory-library-dialog button[aria-label="关闭"]')?.click());
    expect(onCloseMemory).toHaveBeenCalledOnce();
    expect(document.querySelector(".memory-detail-dialog")).toBeNull();
    expect(document.querySelector(".memory-library-dialog")).toBe(persistentLibraryDialog);
    expect(document.querySelector(".memory-library-dialog")?.closest(".aw-dialog-backdrop")).toBe(persistentBackdrop);
    expect(document.querySelector(".memory-library-dialog")?.textContent).toContain("项目记忆");
    expect(document.querySelector(".aw-project-skill-folder")?.textContent).toContain("这是一个名称很长的项目记忆文件夹");
    expect(document.querySelector(".memory-topic-row")).toBeNull();
  });
});
