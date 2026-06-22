import type { ComponentProps } from "react";
import type { AwakenRunStage } from "./AwakenRunStage";

export type AwakenRunStageState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenRunStage>>;
};

const noop = () => {};

export const AwakenRunStageStates: AwakenRunStageState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenRunStage",
      composer: "AwakenRunStage",
      scrollRef: "AwakenRunStage",
      onScroll: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenRunStage>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenRunStage with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenRunStage>>,
  },
];
