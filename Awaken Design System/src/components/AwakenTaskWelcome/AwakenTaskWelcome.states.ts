import type { ComponentProps } from "react";
import type { AwakenTaskWelcome } from "./AwakenTaskWelcome";

export type AwakenTaskWelcomeState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenTaskWelcome>>;
};

export const AwakenTaskWelcomeStates: AwakenTaskWelcomeState[] = [
  {
    id: "desktop-current",
    label: "Desktop 当前欢迎区",
    props: {},
  },
];
