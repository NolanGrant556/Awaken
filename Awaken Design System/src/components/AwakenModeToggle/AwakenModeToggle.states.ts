import type { ComponentProps } from "react";
import type { AwakenModeToggle } from "./AwakenModeToggle";

export type AwakenModeToggleState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenModeToggle>>;
};

const noop = () => {};

export const AwakenModeToggleStates: AwakenModeToggleState[] = [
  {
    id: "plan-mode",
    label: "计划模式",
    props: {
      mode: "Plan",
      onChange: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenModeToggle>>,
  },
  {
    id: "act-mode",
    label: "执行模式",
    props: {
      mode: "Act",
      onChange: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenModeToggle>>,
  },
  {
    id: "disabled-plan",
    label: "禁用切换",
    props: {
      mode: "Plan",
      onChange: noop,
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenModeToggle>>,
  },
];
