import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AwakenContextUsagePanel } from "./AwakenContextUsagePanel";

describe("AwakenContextUsagePanel", () => {
  it("renders all seven typed categories without compact or invalidation diagnostics", () => {
    const html = renderToStaticMarkup(
      React.createElement(AwakenContextUsagePanel, {
        currentInputTokens: 92_100,
        effectiveInputWindowTokens: 240_000,
        modelWindowTokens: 256_000,
        outputReserveTokens: 16_000,
        autoCompactThresholdTokens: 216_000,
        contributions: [
          { id: "system_prompt", label: "系统提示词", estimatedTokens: 1_300 },
          { id: "builtin_tools", label: "内置工具", estimatedTokens: 8_200 },
          { id: "mcp", label: "MCP", estimatedTokens: 4_800 },
          { id: "skills", label: "技能", estimatedTokens: 2_400 },
          { id: "memory", label: "记忆", estimatedTokens: 1_800 },
          { id: "task_checkpoint", label: "任务检查点", estimatedTokens: 2_000 },
          { id: "conversation", label: "对话", estimatedTokens: 68_000 },
          { id: "other", label: "其他/协议开销", estimatedTokens: 3_600 },
        ],
        latestModelUsage: {
          status: "succeeded",
          cacheMode: "automatic_prefix",
          inputTokens: 92_100,
          cachedInputTokens: 78_400,
          uncachedInputTokens: 13_700,
          outputTokens: 1_240,
          calculatedCostUsd: 0.0234,
          currency: "USD",
        },
        onClose: () => {},
      }),
    );

    expect(html).toMatch(/系统提示词/);
    expect(html).toMatch(/内置工具/);
    expect(html).toMatch(/MCP/);
    expect(html).toMatch(/技能/);
    expect(html).toMatch(/记忆/);
    expect(html).toMatch(/任务检查点/);
    expect(html).toMatch(/对话/);
    expect(html).not.toMatch(/最近一次压缩/);
    expect(html).not.toMatch(/失效原因/);
  });

  it("keeps cache and cost details after the breakdown", () => {
    const html = renderToStaticMarkup(
      React.createElement(AwakenContextUsagePanel, {
        currentInputTokens: 64_200,
        effectiveInputWindowTokens: 112_000,
        modelWindowTokens: 128_000,
        outputReserveTokens: 16_000,
        autoCompactThresholdTokens: 100_800,
        contributions: [
          { id: "system_prompt", label: "系统提示词", estimatedTokens: 1_300 },
          { id: "builtin_tools", label: "内置工具", estimatedTokens: 8_200 },
          { id: "mcp", label: "MCP", estimatedTokens: 4_800 },
          { id: "skills", label: "技能", estimatedTokens: 2_400 },
          { id: "memory", label: "记忆", estimatedTokens: 1_800 },
          { id: "task_checkpoint", label: "任务检查点", estimatedTokens: 1_000 },
          { id: "conversation", label: "对话", estimatedTokens: 41_100 },
          { id: "other", label: "其他/协议开销", estimatedTokens: 3_600 },
        ],
        latestModelUsage: {
          status: "succeeded",
          cacheMode: "undocumented",
          inputTokens: 64_200,
          outputTokens: 980,
          calculatedCostUsd: null,
          currency: "USD",
        },
        onClose: () => {},
      }),
    );

    expect(html).toMatch(/缓存/);
    expect(html).toMatch(/未确认/);
    expect(html).toMatch(/成本/);
    expect(html).toMatch(/未知/);
  });
});
