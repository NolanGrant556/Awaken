import type { AwakenMemoryLightFlowProps } from "./AwakenMemoryLightFlow";

export type AwakenMemoryLightFlowState = {
  id: "light" | "dark";
  label: string;
  props: AwakenMemoryLightFlowProps;
};

export const AwakenMemoryLightFlowStates: readonly AwakenMemoryLightFlowState[] = [
  {
    id: "light",
    label: "浅色",
    props: { theme: "light", ariaLabel: "抽象记忆光流装饰" },
  },
  {
    id: "dark",
    label: "深色",
    props: { theme: "dark", ariaLabel: "抽象记忆光流装饰" },
  },
];
