import { createElement, type ComponentProps } from "react";
import type { AwakenRunStage } from "./AwakenRunStage";

export type AwakenRunStageState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenRunStage>>;
};

const noop = () => {};

const stageBlock = (title: string, detail: string) =>
  createElement(
    "div",
    {
      key: title,
      style: {
        display: "grid",
        gap: 4,
        padding: 12,
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-md)",
        background: "var(--aw-bg-white)",
      },
    },
    createElement("strong", null, title),
    createElement("span", null, detail),
  );

export const AwakenRunStageStates: AwakenRunStageState[] = [
  {
    id: "with-composer",
    label: "运行舞台",
    props: {
      children: [
        stageBlock("用户任务", "补齐第 3 组组件用途说明。"),
        stageBlock("执行进度", "已读取组件实现，正在更新 states 和 README。"),
      ],
      composer: createElement(
        "div",
        {
          style: {
            padding: 12,
            borderTop: "1px solid var(--aw-border-main)",
            background: "var(--aw-bg-white)",
          },
        },
        "继续输入任务...",
      ),
      onScroll: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenRunStage>>,
  },
  {
    id: "content-only",
    label: "仅滚动内容",
    props: {
      children: stageBlock("只读分析", "父级可以只传运行内容，不挂载 composer。"),
      composer: null,
      onScroll: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenRunStage>>,
  },
  {
    id: "inactive-content-with-composer",
    label: "叙事区暂时隐藏",
    props: {
      children: stageBlock("当前会话", "会话叙事保持挂载，但由其他工作表面覆盖。"),
      composer: createElement(
        "div",
        {
          style: {
            padding: 12,
            borderTop: "1px solid var(--aw-border-main)",
            background: "var(--aw-bg-white)",
          },
        },
        "输入区仍然可见",
      ),
      contentInactive: true,
      onScroll: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenRunStage>>,
  },
  {
    id: "long-run",
    label: "长运行内容",
    props: {
      children: [
        stageBlock("步骤 1", "读取组件 props 与实现。"),
        stageBlock("步骤 2", "把 demo header 改成具体用途句。"),
        stageBlock("步骤 3", "为 README 补充用途、场景和父子责任边界。"),
        stageBlock("步骤 4", "为每个组件补齐真实中文状态。"),
      ],
      composer: createElement(
        "div",
        {
          style: {
            padding: 12,
            borderTop: "1px solid var(--aw-border-main)",
            background: "var(--aw-bg-white)",
          },
        },
        "等待下一步指令",
      ),
      onScroll: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenRunStage>>,
  },
];
