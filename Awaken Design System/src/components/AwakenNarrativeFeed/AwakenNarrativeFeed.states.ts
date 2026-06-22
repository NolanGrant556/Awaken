import { createElement, type ComponentProps } from "react";
import type { AwakenNarrativeFeed } from "./AwakenNarrativeFeed";

export type AwakenNarrativeFeedState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenNarrativeFeed>>;
};

const feedItem = (title: string, detail: string) =>
  createElement(
    "div",
    {
      key: title,
      style: {
        display: "grid",
        gap: 4,
        padding: "10px 12px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-md)",
        background: "var(--aw-bg-white)",
      },
    },
    createElement("strong", { style: { color: "var(--aw-text-primary)" } }, title),
    createElement("span", { style: { color: "var(--aw-text-tertiary)" } }, detail),
  );

export const AwakenNarrativeFeedStates: AwakenNarrativeFeedState[] = [
  {
    id: "run-narrative",
    label: "运行叙事",
    props: {
      label: "任务运行叙事",
      children: [
        feedItem("用户提交任务", "请检查设计系统第 3 组组件用途说明。"),
        feedItem("正在整理上下文", "已读取组件 props、demo、states 和 README。"),
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenNarrativeFeed>>,
  },
  {
    id: "empty-feed",
    label: "无运行内容",
    props: {
      label: "空运行叙事",
      children: null,
      emptyTitle: "还没有运行记录",
      emptyDetail: "提交任务后，父级会把 prompt、步骤和产物摘要放入这里。",
    } as unknown as Partial<ComponentProps<typeof AwakenNarrativeFeed>>,
  },
  {
    id: "long-record",
    label: "长文本记录",
    props: {
      label: "长任务运行叙事",
      children: feedItem(
        "正在汇总跨目录组件说明",
        "用于验证父级传入较长叙事节点时，feed 仍只负责承载子内容，不替子组件截断、排序或生成空态。",
      ),
    } as unknown as Partial<ComponentProps<typeof AwakenNarrativeFeed>>,
  },
];
