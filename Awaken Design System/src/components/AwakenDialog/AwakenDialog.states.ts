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
      children: "可以先不选文件夹。项目会直接创建，之后随时能加。",
      footer: createElement(
        "div",
        { style: { display: "inline-flex", gap: 8 } },
        createElement(AwakenButton, { variant: "default", size: "md", children: "取消" }),
        createElement(AwakenButton, { variant: "primary", size: "md", children: "创建项目" }),
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
