import type { ComponentProps } from "react";
import type { AwakenBrandRow } from "./AwakenBrandRow";

export type AwakenBrandRowState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenBrandRow>>;
};

const noop = () => {};

export const AwakenBrandRowStates: AwakenBrandRowState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      logoSrc: "AwakenBrandRow",
      title: "AwakenBrandRow",
      meta: "AwakenBrandRow",
      children: "AwakenBrandRow",
    } as unknown as Partial<ComponentProps<typeof AwakenBrandRow>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenBrandRow with a deliberately long value that should remain readable inside its container",
      children: "AwakenBrandRow with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenBrandRow>>,
  },
];
