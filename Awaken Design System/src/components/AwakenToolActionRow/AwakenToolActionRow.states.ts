import type { ComponentProps } from "react";
import type { AwakenToolActionRow } from "./AwakenToolActionRow";

export type AwakenToolActionRowState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolActionRow>>;
};

const noop = () => {};

export const AwakenToolActionRowStates: AwakenToolActionRowState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenToolActionRow",
      detail: "AwakenToolActionRow",
      status: "done",
      onSelect: noop,
      children: "AwakenToolActionRow",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolActionRow>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenToolActionRow with a deliberately long value that should remain readable inside its container",
      children: "AwakenToolActionRow with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenToolActionRow>>,
  },
];
