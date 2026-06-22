import type { ComponentProps } from "react";
import type { AwakenExecutionFeed } from "./AwakenExecutionFeed";

export type AwakenExecutionFeedState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenExecutionFeed>>;
};

const noop = () => {};

export const AwakenExecutionFeedStates: AwakenExecutionFeedState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenExecutionFeed",
      label: "AwakenExecutionFeed",
    } as unknown as Partial<ComponentProps<typeof AwakenExecutionFeed>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenExecutionFeed with a deliberately long value that should remain readable inside its container",
      label: "AwakenExecutionFeed with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenExecutionFeed>>,
  },
];
