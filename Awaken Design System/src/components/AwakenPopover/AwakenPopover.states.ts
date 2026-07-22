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
    id: "account-open",
    label: "账户额度面板打开",
    props: {
      trigger: "Nolan",
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
        createElement("strong", null, "模型额度"),
        createElement("span", null, "deepseek-v4-pro · 可用"),
        createElement("span", null, "kimi-k2.6 · 可用"),
      ),
      open: true,
      onOpenChange: noop,
      placement: "top-start",
      offset: 8,
      matchTriggerWidth: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
  {
    id: "closed",
    label: "关闭状态",
    props: {
      trigger: "Nolan",
      content: "模型额度与设置入口",
      open: false,
      onOpenChange: noop,
      placement: "top-start",
      offset: 8,
      matchTriggerWidth: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
  {
    id: "project-menu-open",
    label: "项目更多菜单打开",
    props: {
      trigger: "更多操作",
      content: "新建会话、在 Finder 中显示、从 Awaken 移除",
      open: true,
      onOpenChange: noop,
      placement: "bottom-end",
      offset: 6,
      matchTriggerWidth: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
];
