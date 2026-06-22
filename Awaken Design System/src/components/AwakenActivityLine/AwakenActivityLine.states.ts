import type { ComponentProps } from "react";
import type { AwakenActivityLine } from "./AwakenActivityLine";

export type AwakenActivityLineState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenActivityLine>>;
};

const noop = () => {};

export const AwakenActivityLineStates: AwakenActivityLineState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenActivityLine",
      detail: "AwakenActivityLine",
      status: "done",
      children: "AwakenActivityLine",
    } as unknown as Partial<ComponentProps<typeof AwakenActivityLine>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenActivityLine with a deliberately long value that should remain readable inside its container",
      children: "AwakenActivityLine with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenActivityLine>>,
  },
];
