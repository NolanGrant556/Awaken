import type { ComponentProps } from "react";
import type { AwakenDivider } from "./AwakenDivider";

export type AwakenDividerState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenDivider>>;
};

export const AwakenDividerStates: AwakenDividerState[] = [
  {
    id: "compact",
    label: "紧凑分隔",
    props: {
      spacing: "sm",
    } as unknown as Partial<ComponentProps<typeof AwakenDivider>>,
  },
  {
    id: "standard",
    label: "标准分隔",
    props: {
      spacing: "md",
    } as unknown as Partial<ComponentProps<typeof AwakenDivider>>,
  },
  {
    id: "section-break",
    label: "区块分隔",
    props: {
      spacing: "lg",
    } as unknown as Partial<ComponentProps<typeof AwakenDivider>>,
  },
];
