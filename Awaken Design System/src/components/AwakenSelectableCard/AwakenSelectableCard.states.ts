import { createElement, type ComponentProps } from "react";
import { Cloud, Monitor } from "lucide-react";
import type { AwakenSelectableCard } from "./AwakenSelectableCard";

export type AwakenSelectableCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSelectableCard>>;
};

const noop = () => {};

export const AwakenSelectableCardStates: AwakenSelectableCardState[] = [
  {
    id: "local-selected",
    label: "本地已选",
    props: {
      selected: true,
      icon: createElement(Monitor, { size: 18, strokeWidth: 1.9 }),
      title: "本地",
      description: "从这台 Mac 选择一个已有文件夹。",
      onSelect: noop,
    } as Partial<ComponentProps<typeof AwakenSelectableCard>>,
  },
  {
    id: "remote-disabled",
    label: "未来态禁用",
    props: {
      disabled: true,
      icon: createElement(Cloud, { size: 18, strokeWidth: 1.9 }),
      title: "远程",
      description: "远程仓库和云工作区稍后开放。",
      onSelect: noop,
    } as Partial<ComponentProps<typeof AwakenSelectableCard>>,
  },
];
