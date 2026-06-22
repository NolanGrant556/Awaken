import type { ComponentProps } from "react";
import type { AwakenQuickActionChip } from "./AwakenQuickActionChip";

export type AwakenQuickActionChipState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenQuickActionChip>>;
};

const noop = () => {};

export const AwakenQuickActionChipStates: AwakenQuickActionChipState[] = [
  {
    id: "fill-composer",
    label: "填入任务",
    props: {
      icon: "写",
      label: "生成测试计划",
      prompt: "基于当前 diff 生成最小可执行测试计划。",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionChip>>,
  },
  {
    id: "disabled-suggestion",
    label: "建议不可用",
    props: {
      icon: "锁",
      label: "连接远程仓库",
      prompt: "打开远程仓库同步流程。",
      onClick: noop,
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionChip>>,
  },
  {
    id: "long-label",
    label: "长建议",
    props: {
      icon: "查",
      label: "检查当前工作区所有未提交设计系统文档改动",
      prompt: "只检查 Awaken Design System 文档与状态示例改动，不修改组件实现。",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionChip>>,
  },
];
