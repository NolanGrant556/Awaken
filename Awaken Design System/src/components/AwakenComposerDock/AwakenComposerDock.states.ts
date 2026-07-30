import type { ComponentProps } from "react";
import type { AwakenComposerDock } from "./AwakenComposerDock";

export type AwakenComposerDockState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposerDock>>;
};

export const AwakenComposerDockStates: AwakenComposerDockState[] = [
  {
    id: "dock-slot",
    label: "底部停靠槽",
    props: {
      children: "这里承载停靠在工作台底部的输入栏。",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerDock>>,
  },
  {
    id: "long-dock-content",
    label: "长停靠内容",
    props: {
      children: "父级把 AwakenComposerBar 或自定义 composer 作为 children 传入；Dock 只提供固定的底部容器和边界样式。",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerDock>>,
  },
  {
    id: "overlay-inactive",
    label: "媒体预览时暂停交互",
    props: {
      children: "媒体预览层打开时保留布局，但输入区暂时不可交互。",
      interactionInactive: true,
    },
  },
];
