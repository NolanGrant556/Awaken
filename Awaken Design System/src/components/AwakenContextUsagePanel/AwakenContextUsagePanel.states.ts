import type { ComponentProps } from "react";
import type { AwakenContextUsagePanel } from "./AwakenContextUsagePanel";

export type AwakenContextUsagePanelState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenContextUsagePanel>>;
};

const noop = () => {};

export const AwakenContextUsagePanelStates: AwakenContextUsagePanelState[] = [
  {
    id: "breakdown",
    label: "分类估算",
    props: {
      estimatedInputTokens: 92_100,
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
        { id: "conversation", label: "对话", estimatedTokens: 73_600 },
      ],
      onClose: noop,
    },
  },
  {
    id: "legacy",
    label: "旧记录暂无分类明细",
    props: {
      estimatedInputTokens: 12_000,
      effectiveInputWindowTokens: 112_000,
      modelWindowTokens: 128_000,
      outputReserveTokens: 16_000,
      autoCompactThresholdTokens: 100_800,
      contributions: [],
      onClose: noop,
    },
  },
  {
    id: "zero-middle-category",
    label: "中间分类为零时保持语义配色",
    props: {
      estimatedInputTokens: 110_724,
      effectiveInputWindowTokens: 613_000,
      modelWindowTokens: 629_000,
      outputReserveTokens: 16_000,
      autoCompactThresholdTokens: 551_700,
      contributions: [
        { id: "system_prompt", label: "系统提示词", estimatedTokens: 1_461 },
        { id: "builtin_tools", label: "内置工具", estimatedTokens: 11_060 },
        { id: "mcp", label: "MCP", estimatedTokens: 0 },
        { id: "skills", label: "技能", estimatedTokens: 1_736 },
        { id: "memory", label: "记忆", estimatedTokens: 747 },
        { id: "conversation", label: "对话", estimatedTokens: 95_720 },
      ],
      onClose: noop,
    },
  },
  {
    id: "cache-hit",
    label: "缓存命中与成本",
    props: {
      estimatedInputTokens: 92_100,
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
        { id: "conversation", label: "对话", estimatedTokens: 73_600 },
      ],
      latestModelUsage: {
        status: "succeeded",
        cacheMode: "automatic_prefix",
        cacheKeyApplied: false,
        inputTokens: 92_100,
        cachedInputTokens: 78_400,
        uncachedInputTokens: 13_700,
        outputTokens: 1_240,
        calculatedCostUsd: 0.0234,
        currency: "USD",
        purpose: "agent_turn",
      },
      onClose: noop,
    },
  },
  {
    id: "cache-undocumented",
    label: "火山未确认",
    props: {
      estimatedInputTokens: 64_200,
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
        { id: "conversation", label: "对话", estimatedTokens: 45_700 },
      ],
      latestModelUsage: {
        status: "succeeded",
        cacheMode: "undocumented",
        cacheKeyApplied: false,
        inputTokens: 64_200,
        outputTokens: 980,
        calculatedCostUsd: null,
        currency: "USD",
        purpose: "agent_turn",
      },
      onClose: noop,
    },
  },
];
