import type { ComponentProps } from "react";
import type { AwakenCopyButton } from "./AwakenCopyButton";

export type AwakenCopyButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenCopyButton>>;
};

const noop = () => {};

export const AwakenCopyButtonStates: AwakenCopyButtonState[] = [
  {
    id: "answer-copy",
    label: "复制回答",
    props: {
      text: "可以直接把这段结论放进发布说明。",
      label: "复制回答",
      copiedLabel: "已复制",
      errorLabel: "复制失败",
      onCopied: noop,
      onCopyError: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenCopyButton>>,
  },
  {
    id: "command-copy",
    label: "复制命令",
    props: {
      text: "pnpm --filter @awaken/design-system check",
      label: "复制命令",
      copiedLabel: "命令已复制",
      errorLabel: "命令复制失败",
    } as unknown as Partial<ComponentProps<typeof AwakenCopyButton>>,
  },
  {
    id: "empty-text",
    label: "无内容",
    props: {
      text: "",
      label: "无内容可复制",
      copiedLabel: "已复制",
      errorLabel: "复制失败",
    } as unknown as Partial<ComponentProps<typeof AwakenCopyButton>>,
  },
  {
    id: "disabled",
    label: "禁用复制",
    props: {
      text: "受限环境中的敏感片段",
      label: "复制被禁用",
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenCopyButton>>,
  },
  {
    id: "long-label",
    label: "长路径标签",
    props: {
      text: "/Users/nolangrant/Awaken/Awaken Design System/src/components/AwakenCopyButton/README.md",
      label: "复制当前 Design System 组件 README 的完整绝对路径",
      copiedLabel: "完整路径已复制",
      errorLabel: "路径复制失败",
    } as unknown as Partial<ComponentProps<typeof AwakenCopyButton>>,
  },
];
