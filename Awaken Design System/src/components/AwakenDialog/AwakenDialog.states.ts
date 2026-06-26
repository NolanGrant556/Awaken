import { createElement, type ComponentProps } from "react";
import { AwakenButton } from "../AwakenButton";
import type { AwakenDialog } from "./AwakenDialog";

export type AwakenDialogState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenDialog>>;
};

const noop = () => {};

export const AwakenDialogStates: AwakenDialogState[] = [
  {
    id: "create-project",
    label: "创建项目",
    props: {
      open: true,
      title: "创建项目",
      onOpenChange: noop,
      children: "选择项目来源后，可以继续使用已有本地文件夹。",
      footer: createElement(
        "div",
        { style: { display: "inline-flex", gap: 8 } },
        createElement(AwakenButton, { variant: "default", size: "md", children: "使用现有文件夹" }),
        createElement(AwakenButton, { variant: "primary", size: "md", children: "下一步" }),
      ),
    } as Partial<ComponentProps<typeof AwakenDialog>>,
  },
  {
    id: "closed",
    label: "关闭状态",
    props: {
      open: false,
      title: "创建项目",
      onOpenChange: noop,
      children: "关闭状态不会渲染弹窗。",
    } as Partial<ComponentProps<typeof AwakenDialog>>,
  },
];
