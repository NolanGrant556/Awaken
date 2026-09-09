import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AwakenContextUsagePanel, type AwakenContextUsageModelUsage } from "@awaken/design-system";
import { contextUsagePanelData } from "./contextUsagePresentation";

function renderUsage(usage: AwakenContextUsageModelUsage) {
  return renderToStaticMarkup(React.createElement(AwakenContextUsagePanel, {
    currentInputTokens: 1000,
    effectiveInputWindowTokens: 10000,
    modelWindowTokens: 10000,
    outputReserveTokens: 1000,
    autoCompactThresholdTokens: 9000,
    latestModelUsage: usage,
    onClose: () => {},
  }));
}

describe("context cache rate", () => {
  it.each([
    [750, 1000, "75.0%"], [0, 1000, "0.0%"], [1000, 1000, "100.0%"],
    [1, 3, "33.3%"], [undefined, 1000, "未知"], [0, 0, "未知"],
    [1001, 1000, "未知"], [-1, 1000, "未知"], [NaN, 1000, "未知"],
    [10, Infinity, "未知"],
  ])("renders cached %s / input %s as %s", (cached, input, label) => {
    expect(renderUsage({cacheMode: "automatic_prefix", cachedInputTokens: cached, inputTokens: input}))
      .toContain(`缓存率</dt><dd>${label}</dd>`);
  });

  it("does not infer a rate for an undocumented cache contract", () => {
    expect(renderUsage({cacheMode: "undocumented", cachedInputTokens: 750, inputTokens: 1000}))
      .toContain("缓存率</dt><dd>未确认</dd>");
  });

  it("keeps rate and cost visible while token rows start collapsed", () => {
    const html = renderUsage({cacheMode: "automatic_prefix", cachedInputTokens: 750,
      uncachedInputTokens: 250, inputTokens: 1000, outputTokens: 20, calculatedCostUsd: 0.01});
    const details = html.match(/<details\b([^>]*)>(.*?)<\/details>/)?.[0];
    expect(details).toBeDefined();
    expect(details).not.toMatch(/<details[^>]*\bopen\b/);
    for (const label of ["Token 明细", "输入 Token", "命中 Token", "未命中 Token", "输出 Token"]) {
      expect(details).toContain(label);
    }
    expect(details).not.toContain("缓存率");
    expect(details).not.toContain("成本");
  });

  it("uses inclusive reconstructed input including cache writes", () => {
    const data = contextUsagePanelData({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT", modelWindow: 10000, effectiveWindow: 10000,
      estimatedTokens: 500,
      latestModelUsage: {
        schemaVersion: 1, requestId: "cache-rate-test", purpose: "agent_turn",
        status: "succeeded", usageComplete: true, cacheMode: "automatic_prefix",
        cachedInputTokens: 600, uncachedInputTokens: 200, cacheWriteTokens: 200,
        latencyMs: 1, completedAt: "2026-09-09T00:00:00Z",
      },
    });
    expect(data?.currentInputTokens).toBe(1000);
    expect(data?.latestModelUsage?.inputTokens).toBe(1000);
    expect(renderUsage(data!.latestModelUsage!)).toContain("缓存率</dt><dd>60.0%</dd>");
  });
});
