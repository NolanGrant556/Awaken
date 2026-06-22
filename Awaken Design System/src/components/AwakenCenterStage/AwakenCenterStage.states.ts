import type { ComponentProps } from "react";
import type { AwakenCenterStage } from "./AwakenCenterStage";

export type AwakenCenterStageState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenCenterStage>>;
};

const noop = () => {};

export const AwakenCenterStageStates: AwakenCenterStageState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenCenterStage",
      width: "AwakenCenterStage",
      topMargin: "AwakenCenterStage",
    } as unknown as Partial<ComponentProps<typeof AwakenCenterStage>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenCenterStage with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenCenterStage>>,
  },
];
