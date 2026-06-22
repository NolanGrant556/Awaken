import { createElement, type ComponentProps } from "react";
import type { AwakenDropdown } from "./AwakenDropdown";

export type AwakenDropdownState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenDropdown>>;
};

const noop = () => {};

function trigger(label: string) {
  return createElement(
    "button",
    {
      type: "button",
      style: {
        padding: "6px 10px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-control)",
        background: "var(--aw-surface-raised)",
        color: "var(--aw-text-primary)",
      },
    },
    label,
  );
}

function menuItem(label: string, disabled = false) {
  return createElement(
    "button",
    {
      key: label,
      type: "button",
      disabled,
      style: {
        display: "block",
        width: "100%",
        padding: "6px 8px",
        border: 0,
        borderRadius: "var(--aw-radius-sm)",
        background: "transparent",
        color: "var(--aw-text-primary)",
        textAlign: "left",
        opacity: disabled ? 0.48 : 1,
      },
    },
    label,
  );
}

export const AwakenDropdownStates: AwakenDropdownState[] = [
  {
    id: "model-menu-open",
    label: "模型菜单打开",
    props: {
      trigger: trigger("选择模型"),
      children: [
        menuItem("GPT-5 Thinking"),
        menuItem("本地轻量模型（暂不可用）", true),
        menuItem("Claude Sonnet"),
      ],
      open: true,
      onOpenChange: noop,
      placement: "bottom-start",
      offset: 4,
    } as unknown as Partial<ComponentProps<typeof AwakenDropdown>>,
  },
  {
    id: "actions-bottom-end",
    label: "右对齐操作",
    props: {
      trigger: trigger("更多操作"),
      children: [menuItem("复制摘要"), menuItem("导出日志"), menuItem("清空当前视图")],
      open: true,
      placement: "bottom-end",
    } as unknown as Partial<ComponentProps<typeof AwakenDropdown>>,
  },
  {
    id: "no-content",
    label: "无菜单内容",
    props: {
      trigger: trigger("暂无可选项"),
      children: null,
      open: false,
    } as unknown as Partial<ComponentProps<typeof AwakenDropdown>>,
  },
  {
    id: "long-option",
    label: "长菜单项",
    props: {
      trigger: trigger("选择视图"),
      children: [
        menuItem("显示完整执行叙事、工具调用摘要和最近一次文件变更"),
        menuItem("只显示检查器中的文件改动"),
      ],
      open: true,
    } as unknown as Partial<ComponentProps<typeof AwakenDropdown>>,
  },
];
