import { createElement, type ComponentProps } from "react";
import type { AwakenIcon } from "./AwakenIcon";

export type AwakenIconState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenIcon>>;
};

export const AwakenIconStates: AwakenIconState[] = [
  {
    id: "search-icon",
    label: "默认图标",
    props: {
      size: 18,
      strokeWidth: 1.85,
      ariaHidden: true,
      children: [
        createElement("circle", { key: "lens", cx: 11, cy: 11, r: 6 }),
        createElement("path", { key: "handle", d: "m16 16 4 4" }),
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenIcon>>,
  },
  {
    id: "small-check",
    label: "小尺寸状态",
    props: {
      size: 14,
      strokeWidth: 2.2,
      ariaHidden: true,
      children: createElement("path", { d: "m5 12 4 4 10-10" }),
    } as unknown as Partial<ComponentProps<typeof AwakenIcon>>,
  },
  {
    id: "exposed-icon",
    label: "非装饰图标",
    props: {
      size: 20,
      strokeWidth: 2,
      ariaHidden: false,
      children: [
        createElement("path", { key: "box", d: "M4 7h16v11H4z" }),
        createElement("path", { key: "lid", d: "M8 7V5h8v2" }),
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenIcon>>,
  },
];
