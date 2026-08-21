import { createElement, type ComponentProps } from "react";
import { AwakenIcon } from "../AwakenIcon";
import type { AwakenSidebarSectionAction } from "./AwakenSidebarSectionAction";

export type AwakenSidebarSectionActionState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSidebarSectionAction>>;
};

export const AwakenSidebarSectionActionStates: AwakenSidebarSectionActionState[] = [
  {
    id: "add-folder",
    label: "添加文件夹",
    props: {
      label: "添加文件夹",
      children: createElement(AwakenIcon, { concept: "project-add", size: 14 }),
    } as Partial<ComponentProps<typeof AwakenSidebarSectionAction>>,
  },
  {
    id: "filter",
    label: "筛选项目",
    props: {
      label: "筛选项目",
      children: createElement(AwakenIcon, { concept: "filter", size: 14 }),
    } as Partial<ComponentProps<typeof AwakenSidebarSectionAction>>,
  },
  {
    id: "disabled",
    label: "禁用",
    props: {
      label: "正在同步",
      disabled: true,
      children: createElement(AwakenIcon, { concept: "project-add", size: 14 }),
    } as Partial<ComponentProps<typeof AwakenSidebarSectionAction>>,
  },
];
