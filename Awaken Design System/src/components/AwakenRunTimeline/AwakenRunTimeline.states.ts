import type { ComponentProps } from "react";
import type { AwakenRunTimeline } from "./AwakenRunTimeline";

export type AwakenRunTimelineState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenRunTimeline>>;
};

const noop = () => {};

export const AwakenRunTimelineStates: AwakenRunTimelineState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenRunTimeline",
      status: "done",
      steps: [{ id: "step", title: "Read file", description: "Inspect component", status: "done" }],
      children: "AwakenRunTimeline",
    } as unknown as Partial<ComponentProps<typeof AwakenRunTimeline>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenRunTimeline with a deliberately long value that should remain readable inside its container",
      children: "AwakenRunTimeline with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenRunTimeline>>,
  },
];
