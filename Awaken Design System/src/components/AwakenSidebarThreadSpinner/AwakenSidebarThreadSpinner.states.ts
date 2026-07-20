import type { ComponentProps } from "react";
import type { AwakenSidebarThreadSpinner } from "./AwakenSidebarThreadSpinner";

export type AwakenSidebarThreadSpinnerState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSidebarThreadSpinner>>;
};

export const AwakenSidebarThreadSpinnerStates: AwakenSidebarThreadSpinnerState[] = [
  {
    id: "running-thread",
    label: "线程运行中",
    props: { label: "线程运行中" },
  },
  {
    id: "cancelling-thread",
    label: "正在取消",
    props: { label: "正在取消线程" },
  },
];
