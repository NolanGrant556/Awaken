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
      title: "Create project",
      onOpenChange: noop,
      children: "Use a folder on this Mac as an Awaken project.",
      footer: createElement(
        "div",
        { style: { display: "inline-flex", gap: 8 } },
        createElement(AwakenButton, { variant: "default", size: "md", children: "Use existing folder" }),
        createElement(AwakenButton, { variant: "primary", size: "md", children: "Next" }),
      ),
    } as Partial<ComponentProps<typeof AwakenDialog>>,
  },
  {
    id: "closed",
    label: "关闭状态",
    props: {
      open: false,
      title: "Create project",
      onOpenChange: noop,
      children: "关闭状态不会渲染弹窗。",
    } as Partial<ComponentProps<typeof AwakenDialog>>,
  },
];
