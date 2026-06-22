import type { ComponentProps } from "react";
import type { AwakenToolTraceLine } from "./AwakenToolTraceLine";

export type AwakenToolTraceLineState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolTraceLine>>;
};

const noop = () => {};

export const AwakenToolTraceLineStates: AwakenToolTraceLineState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenToolTraceLine",
      detail: "AwakenToolTraceLine",
      status: "done",
      icon: "AwakenToolTraceLine",
      onSelect: noop,
      children: "AwakenToolTraceLine",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenToolTraceLine with a deliberately long value that should remain readable inside its container",
      children: "AwakenToolTraceLine with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
];
