import type { ComponentProps } from "react";
import type { AwakenToolTraceGroup } from "./AwakenToolTraceGroup";

export type AwakenToolTraceGroupState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolTraceGroup>>;
};

const noop = () => {};

export const AwakenToolTraceGroupStates: AwakenToolTraceGroupState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      actions: [{ id: "action", title: "Read file", detail: "src/Awaken.tsx", status: "done" }],
      label: "AwakenToolTraceGroup",
      onSelectAction: noop,
      children: "AwakenToolTraceGroup",
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      label: "AwakenToolTraceGroup with a deliberately long value that should remain readable inside its container",
      children: "AwakenToolTraceGroup with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
];
