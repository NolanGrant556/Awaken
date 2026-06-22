import type { ComponentProps } from "react";
import type { AwakenRunStep } from "./AwakenRunStep";

export type AwakenRunStepState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenRunStep>>;
};

const noop = () => {};

export const AwakenRunStepStates: AwakenRunStepState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      index: "AwakenRunStep",
      title: "AwakenRunStep",
      description: "AwakenRunStep",
      tag: "AwakenRunStep",
      status: "done",
      children: "AwakenRunStep",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenRunStep>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenRunStep with a deliberately long value that should remain readable inside its container",
      description: "AwakenRunStep with a deliberately long value that should remain readable inside its container",
      children: "AwakenRunStep with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenRunStep>>,
  },
];
