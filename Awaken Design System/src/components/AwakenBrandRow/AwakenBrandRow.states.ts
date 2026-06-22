import type { ComponentProps } from "react";
import type { AwakenBrandRow } from "./AwakenBrandRow";

export type AwakenBrandRowState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenBrandRow>>;
};

export const AwakenBrandRowStates: AwakenBrandRowState[] = [
  {
    id: "default-brand",
    label: "默认品牌",
    props: {
      title: "Awaken",
      meta: "Local-first desktop agent",
    } as unknown as Partial<ComponentProps<typeof AwakenBrandRow>>,
  },
  {
    id: "workspace-brand",
    label: "工作区标识",
    props: {
      title: "Awaken Design System",
      meta: "组件库 Gallery",
    } as unknown as Partial<ComponentProps<typeof AwakenBrandRow>>,
  },
  {
    id: "long-brand",
    label: "长品牌文本",
    props: {
      title: "Awaken Local Desktop Runtime",
      meta: "用于验证侧栏品牌行的标题和辅助信息截断",
    } as unknown as Partial<ComponentProps<typeof AwakenBrandRow>>,
  },
];
