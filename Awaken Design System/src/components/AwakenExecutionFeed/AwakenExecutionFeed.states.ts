import { createElement, type ComponentProps } from "react";
import type { AwakenExecutionFeed } from "./AwakenExecutionFeed";

export type AwakenExecutionFeedState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenExecutionFeed>>;
};

function feedItem(title: string, detail: string) {
  return createElement(
    "article",
    {
      key: title,
      style: {
        display: "grid",
        gap: "4px",
        padding: "10px 0",
        borderBottom: "1px solid var(--aw-border-light)",
      },
    },
    createElement("strong", { style: { color: "var(--aw-text-primary)" } }, title),
    createElement("span", { style: { color: "var(--aw-text-subtle)" } }, detail),
  );
}

export const AwakenExecutionFeedStates: AwakenExecutionFeedState[] = [
  {
    id: "running-events",
    label: "执行事件",
    props: {
      children: [
        feedItem("读取项目结构", "已确认 Design System 组件目录和允许修改的文档文件。"),
        feedItem("整理用途说明", "正在把组件实现映射到真实使用场景。"),
      ],
      label: "Agent 执行叙事",
    } as unknown as Partial<ComponentProps<typeof AwakenExecutionFeed>>,
  },
  {
    id: "empty-feed",
    label: "暂无事件",
    props: {
      children: null,
      label: "尚无执行事件",
    } as unknown as Partial<ComponentProps<typeof AwakenExecutionFeed>>,
  },
  {
    id: "long-event",
    label: "长事件",
    props: {
      children: feedItem(
        "复核一条较长的执行叙事标题，验证 feed 容器不会替子项截断业务信息",
        "子项负责自己的内容排版，ExecutionFeed 只负责承载、过滤空节点和提供 section 标签。",
      ),
      label: "长执行叙事",
    } as unknown as Partial<ComponentProps<typeof AwakenExecutionFeed>>,
  },
];
