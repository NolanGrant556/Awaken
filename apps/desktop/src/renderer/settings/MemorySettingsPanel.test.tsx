import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AwakenMemorySettings } from "@awaken/design-system";
import type { MemoryDreamTask, RuntimeMemory, RuntimeProject } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { buildMemoryLibraryPresentation, loadRecentActivitySnapshot, MemorySettingsPanel, memoryDreamStatusLabel, memoryRelativeTime } from "./MemorySettingsPanel";

describe("MemorySettingsPanel", () => {
  it("stacks recent activity above the library without create or edit actions", () => {
    const html = renderToString(
      React.createElement(MemorySettingsPanel, {
        runtimeClient: {} as RuntimeClient,
      }),
    );

    expect(html).toContain("自动召回");
    expect(html).toContain("主动记忆");
    expect(html).toContain("Auto Dream");
    expect(html).not.toContain("搜索记忆");
    expect(html).toContain("更新记忆");
    expect(html).not.toContain("立即整理");
    expect(html).toContain("记忆库");
    expect(html).toContain("memory-library-overview");
    expect(html).toContain("用户资料、协作偏好与个人参考入口");
    expect(html).toContain("按工作区保存的项目背景、决策与参考入口");
    expect(html).toContain("最近动态");
    expect(html).not.toContain("查看记忆更新与整理结果。");
    expect(html).not.toContain("显示当前记录，不代表完整版本历史");
    expect(html).not.toContain("最近一次整理");
    expect(html).not.toContain("memory-dream-summary");
    expect(html.indexOf("最近动态")).toBeLessThan(html.indexOf("记忆库"));
    expect(html.indexOf("memory-activity-panel")).toBeLessThan(html.indexOf("更新记忆"));
    expect(html).toContain('class="memory-manager" aria-label="记忆管理"');
    expect(html).not.toContain(">记忆管理</h2>");
    expect(html).not.toContain("查看记忆变化，并管理跨任务保存的主题记忆。");
    expect(html).not.toContain("记忆管理视图");
    expect(html).not.toContain('role="tablist"');
    expect(html).not.toContain("选择一条记忆查看正文");
    expect(html).toContain("aw-memory-light-flow-layout");
    expect(html).toContain("aw-memory-light-flow-layout__title");
    expect(html).toContain("aw-memory-light-flow-layout__title-icon");
    expect(html).toContain("aw-memory-light-flow-layout__content");
    expect(html).toContain("aw-memory-light-flow-layout__scroll-thumb");
    expect(html).toContain("<span>记忆</span>");
    expect(html).toContain("aw-memory-light-flow--light");
    expect(html).toContain("<canvas");
    expect(html).not.toContain("<select");
    expect(html).not.toContain("创建记忆");
    expect(html).not.toContain("编辑记忆");
  });

  it("keeps recent activity rows on one line and truncates long visible titles", () => {
    const longTitle = "123456789012345678901234567890123456";
    const html = renderToString(
      React.createElement(AwakenMemorySettings, {
        switches: [],
        memories: [],
        activities: [
          {
            id: "activity-long",
            kind: "dream",
            label: "记忆整理",
            title: longTitle,
            description: "更新 0 条 · 已替代 0 条 · 冲突 0 条 · 跳过 0 条",
            timeLabel: "刚刚",
          },
        ],
      }),
    );

    expect(html).not.toContain("更新 0 条");
    expect(html).toContain("12345678901234567890123456789012.....");
    expect(html).toContain(`aria-label="记忆整理 ${longTitle} 刚刚"`);
  });

  it("loads recent activity independently from the memory library scope", async () => {
    const userMemory: RuntimeMemory = {
      memoryId: "user-memory",
      name: "用户称呼",
      description: "称呼用户为 Nolan",
      type: "user",
      status: "active",
      revision: 1,
      updatedAt: "2026-07-23T08:00:00Z",
      sourceThreadId: "thread-user",
      sourceRecordId: "record-user",
      scope: "user",
    };
    const projectMemory: RuntimeMemory = {
      ...userMemory,
      memoryId: "project-memory",
      name: "项目约束",
      type: "project",
      sourceThreadId: "thread-project",
      sourceRecordId: "record-project",
      scope: "project",
    };
    const userDream: MemoryDreamTask = {
      taskId: "dream-user",
      status: "completed",
      scopes: ["user"],
      createdAt: "2026-07-23T08:00:00Z",
    };
    const projectDream: MemoryDreamTask = {
      taskId: "dream-project",
      status: "completed",
      projectId: "project-1",
      scopes: ["user", "project"],
      createdAt: "2026-07-23T09:00:00Z",
    };
    const project: RuntimeProject = {
      projectId: "project-1",
      workspaceRoot: "/tmp/project-1",
      displayName: "项目一",
      state: "active",
      createdAt: "2026-07-23T08:00:00Z",
      updatedAt: "2026-07-23T08:00:00Z",
      version: 1,
    };
    const memoryRequests: Array<{ scope?: string; projectId?: string }> = [];
    const dreamRequests: Array<string | undefined> = [];
    const runtimeClient = {
      listMemories: async (request: { scope?: string; projectId?: string }) => {
        memoryRequests.push(request);
        return request.projectId === "project-1" ? [projectMemory] : [userMemory];
      },
      listMemoryDreams: async (projectId?: string) => {
        dreamRequests.push(projectId);
        return projectId === "project-1" ? [userDream, projectDream] : [userDream];
      },
    } as Pick<RuntimeClient, "listMemories" | "listMemoryDreams">;

    const snapshot = await loadRecentActivitySnapshot(runtimeClient, [project]);

    expect(memoryRequests).toEqual([
      expect.objectContaining({ scope: "user", projectId: undefined }),
      expect.objectContaining({ scope: "project", projectId: "project-1" }),
    ]);
    expect(dreamRequests).toEqual([undefined, "project-1"]);
    expect(snapshot.memories.map((entry) => [entry.memory.memoryId, entry.projectId])).toEqual([
      ["user-memory", undefined],
      ["project-memory", "project-1"],
    ]);
    expect(snapshot.dreamTasks.map((task) => task.taskId)).toEqual(["dream-user", "dream-project"]);
    expect(snapshot.errors).toEqual([]);
  });

  it("keeps successful recent activity when one project cannot be read", async () => {
    const runtimeClient = {
      listMemories: async (request: { projectId?: string }) => {
        if (request.projectId) throw new Error("project unavailable");
        return [];
      },
      listMemoryDreams: async () => [],
    } as Pick<RuntimeClient, "listMemories" | "listMemoryDreams">;
    const project = {
      projectId: "project-1",
      workspaceRoot: "/tmp/project-1",
      displayName: "项目一",
      state: "active",
      createdAt: "2026-07-23T08:00:00Z",
      updatedAt: "2026-07-23T08:00:00Z",
      version: 1,
    } satisfies RuntimeProject;

    const snapshot = await loadRecentActivitySnapshot(runtimeClient, [project]);

    expect(snapshot.memories).toEqual([]);
    expect(snapshot.dreamTasks).toEqual([]);
    expect(snapshot.errors).toEqual(["项目一读取失败：project unavailable"]);
  });

  it("builds a file-oriented library summary from user and project topic metadata", () => {
    const projectOne = {
      projectId: "project-1",
      workspaceRoot: "/tmp/project-1",
      displayName: "项目一",
      state: "active",
      createdAt: "2026-07-23T08:00:00Z",
      updatedAt: "2026-07-23T08:00:00Z",
      version: 1,
    } satisfies RuntimeProject;
    const projectTwo = { ...projectOne, projectId: "project-2", displayName: "空项目" } satisfies RuntimeProject;
    const baseMemory: RuntimeMemory = {
      memoryId: "user-1",
      name: "用户称呼",
      description: "称呼用户为 Nolan",
      type: "user",
      status: "active",
      revision: 1,
      updatedAt: "2026-07-23T08:00:00Z",
      sourceThreadId: "thread-user",
      sourceRecordId: "record-user",
      scope: "user",
    };

    const presentation = buildMemoryLibraryPresentation([
      { memory: baseMemory },
      { memory: { ...baseMemory, memoryId: "project-memory", scope: "project", type: "project", updatedAt: "2026-07-23T09:00:00Z" }, projectId: "project-1", projectName: "项目一" },
    ], [projectOne, projectTwo], new Date("2026-07-23T10:00:00Z").getTime());

    expect(presentation.summary).toEqual({
      userTopicCount: 1,
      userTopicCountCapped: false,
      userUpdatedLabel: "2 小时前",
      projectCount: 1,
      projectTopicCount: 1,
      projectTopicCountCapped: false,
      projectUpdatedLabel: "1 小时前",
    });
    expect(presentation.projects).toEqual([{
      id: "project-1",
      name: "项目一",
      topicCount: 1,
      topicCountCapped: false,
      updatedLabel: "1 小时前",
    }]);
  });

  it("uses Chinese copy for every Dream task status", () => {
    expect(memoryDreamStatusLabel("queued")).toBe("等待整理");
    expect(memoryDreamStatusLabel("running")).toBe("正在整理");
    expect(memoryDreamStatusLabel("completed")).toBe("整理完成");
    expect(memoryDreamStatusLabel("failed")).toBe("整理失败");
    expect(memoryDreamStatusLabel("cancelled")).toBe("已取消");
    expect(memoryDreamStatusLabel("interrupted")).toBe("整理中断");
  });

  it("formats recent memory timestamps without changing stored data", () => {
    const now = new Date("2026-07-23T10:00:00+08:00").getTime();
    expect(memoryRelativeTime("2026-07-23T09:40:00+08:00", now)).toBe("20 分钟前");
    expect(memoryRelativeTime("2026-07-20T10:00:00+08:00", now)).toBe("3 天前");
    expect(memoryRelativeTime("not-a-date", now)).toBe("时间未知");
  });

  it("forwards the effective Desktop theme to the decorative light flow", () => {
    const html = renderToString(
      React.createElement(MemorySettingsPanel, {
        runtimeClient: {} as RuntimeClient,
        theme: "dark",
      }),
    );

    expect(html).toContain("aw-memory-light-flow--dark");
    expect(html).not.toContain("aw-memory-light-flow--light");
  });
});
