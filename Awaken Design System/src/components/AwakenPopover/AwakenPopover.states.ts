import { createElement, type ComponentProps } from "react";
import type { AwakenPopover } from "./AwakenPopover";

export type AwakenPopoverState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPopover>>;
};

const noop = () => {};

export const AwakenPopoverStates: AwakenPopoverState[] = [
  {
    id: "model-menu-open",
    label: "模型菜单打开",
    props: {
      trigger: "选择模型",
      content: createElement(
        "div",
        {
          style: {
            display: "grid",
            gap: 6,
            minWidth: 180,
            padding: 10,
            border: "1px solid var(--aw-border-main)",
            borderRadius: "var(--aw-radius-md)",
            background: "var(--aw-bg-white)",
          },
        },
        createElement("strong", null, "本次运行模型"),
        createElement("span", null, "Claude Sonnet"),
        createElement("span", null, "GPT-5 Thinking"),
      ),
      open: true,
      onOpenChange: noop,
      placement: "bottom-start",
      offset: 6,
      matchTriggerWidth: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
  {
    id: "closed",
    label: "关闭状态",
    props: {
      trigger: "更多操作",
      content: "复制运行链接、归档任务或打开审计记录。",
      open: false,
      onOpenChange: noop,
      placement: "bottom-start",
      offset: 6,
      matchTriggerWidth: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
  {
    id: "match-width",
    label: "跟随触发器宽度",
    props: {
      trigger: "运行模式：Act",
      content: "Plan / Act 模式切换面板会与触发器保持同宽。",
      open: true,
      onOpenChange: noop,
      placement: "bottom-start",
      offset: 6,
      matchTriggerWidth: true,
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
  {
    id: "long-content",
    label: "长浮层内容",
    props: {
      trigger: "权限说明",
      content: "此操作会读取当前工作区内的组件文档和状态示例，但不会修改组件实现、Gallery 框架或其他 worker 负责的目录。",
      open: true,
      onOpenChange: noop,
      placement: "bottom-end",
      offset: 8,
      matchTriggerWidth: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
];
