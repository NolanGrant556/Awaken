import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { RuntimeClient } from "../runtime/client";
import { MemorySettingsPanel, memoryDreamStatusLabel } from "./MemorySettingsPanel";

describe("MemorySettingsPanel", () => {
  it("offers four switches plus view and delete, without create or edit actions", () => {
    const html = renderToString(
      React.createElement(MemorySettingsPanel, {
        runtimeClient: {} as RuntimeClient,
      }),
    );

    expect(html).toContain("自动召回");
    expect(html).toContain("主动记忆");
    expect(html).toContain("Auto Dream");
    expect(html).toContain("搜索记忆");
    expect(html).toContain("立即整理");
    expect(html).not.toContain("<select");
    expect(html).not.toContain("创建记忆");
    expect(html).not.toContain("编辑记忆");
  });

  it("uses Chinese copy for every Dream task status", () => {
    expect(memoryDreamStatusLabel("queued")).toBe("等待整理");
    expect(memoryDreamStatusLabel("running")).toBe("正在整理");
    expect(memoryDreamStatusLabel("completed")).toBe("整理完成");
    expect(memoryDreamStatusLabel("failed")).toBe("整理失败");
    expect(memoryDreamStatusLabel("cancelled")).toBe("已取消");
  });
});
