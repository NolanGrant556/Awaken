import { createElement, type ComponentProps } from "react";
import type { AwakenQuickActionGrid } from "./AwakenQuickActionGrid";
import { AwakenQuickActionChip } from "../AwakenQuickActionChip";

export type AwakenQuickActionGridState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenQuickActionGrid>>;
};

const noop = () => {};

const actionChip = (key: string, label: string, prompt: string, disabled = false) =>
  createElement(AwakenQuickActionChip, {
    key,
    label,
    prompt,
    disabled,
    onClick: noop,
  });

export const AwakenQuickActionGridStates: AwakenQuickActionGridState[] = [
  {
    id: "welcome-actions",
    label: "欢迎态建议",
    props: {
      children: [
        actionChip("review", "审查当前 diff", "检查当前工作树中的潜在问题。"),
        actionChip("tests", "生成测试计划", "根据当前任务生成最小测试计划。"),
        actionChip("docs", "整理文档说明", "把组件用途写成中文 README 段落。"),
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionGrid>>,
  },
  {
    id: "single-action",
    label: "单个建议",
    props: {
      children: actionChip("resume", "继续上次运行", "恢复最近一次 Awaken 运行上下文。"),
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionGrid>>,
  },
  {
    id: "many-actions",
    label: "多项换行",
    props: {
      children: [
        actionChip("plan", "只规划", "先输出计划，不改文件。"),
        actionChip("fix", "修复失败测试", "读取失败输出并修复最小范围代码。"),
        actionChip("summarize", "总结运行", "总结当前运行的关键动作和阻塞项。"),
        actionChip("permissions", "检查权限", "列出本轮会触及的目录边界。"),
        actionChip("disabled", "远程部署", "部署当前桌面应用。", true),
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionGrid>>,
  },
];
