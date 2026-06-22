import type { ComponentProps } from "react";
import type { AwakenContextButton } from "./AwakenContextButton";

export type AwakenContextButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenContextButton>>;
};

const noop = () => {};

export const AwakenContextButtonStates: AwakenContextButtonState[] = [
  {
    id: "enabled",
    label: "可添加上下文",
    props: {
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenContextButton>>,
  },
  {
    id: "disabled",
    label: "暂不可添加",
    props: {
      onClick: noop,
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenContextButton>>,
  },
];
