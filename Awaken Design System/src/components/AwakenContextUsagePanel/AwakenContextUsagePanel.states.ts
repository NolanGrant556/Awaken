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
];
