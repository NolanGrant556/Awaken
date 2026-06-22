import type { ComponentProps } from "react";
import type { AwakenPromptCard } from "./AwakenPromptCard";

export type AwakenPromptCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPromptCard>>;
};

export const AwakenPromptCardStates: AwakenPromptCardState[] = [
  {
    id: "submitted-prompt",
    label: "已提交任务",
    props: {
      text: "检查 Awaken Design System 第 3 组组件的用途说明，并把 demo、README、states 改成真实中文场景。",
      eyebrow: "用户任务",
      status: "已接收",
    } as unknown as Partial<ComponentProps<typeof AwakenPromptCard>>,
  },
  {
    id: "running-prompt",
    label: "处理中",
    props: {
      text: "基于真实组件实现补齐用途说明，不要根据组件名猜测，也不要修改组件实现文件。",
      eyebrow: "执行要求",
      status: "整理中",
    } as unknown as Partial<ComponentProps<typeof AwakenPromptCard>>,
  },
  {
    id: "long-prompt",
    label: "长提示",
    props: {
      text: "请逐个阅读 AwakenNarrativeFeed、AwakenNewTaskButton、AwakenPill、AwakenPlatformProvider、AwakenPopover、AwakenPrimaryNav、AwakenProfileButton、AwakenProjectTree、AwakenPromptCard、AwakenQuickActionChip、AwakenQuickActionGrid、AwakenRunStage、AwakenRunStep、AwakenRunTimeline、AwakenSendButton、AwakenSidebar 的 props 和渲染逻辑，然后只在允许的 demo、states、README 文件中写清楚它们在 Awaken 桌面端运行体验中的真实职责。",
      eyebrow: "原始任务",
      status: "长文本",
    } as unknown as Partial<ComponentProps<typeof AwakenPromptCard>>,
  },
];
