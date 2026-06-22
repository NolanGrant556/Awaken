import type { ComponentProps } from "react";
import type { AwakenExploringGroup } from "./AwakenExploringGroup";

export type AwakenExploringGroupState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenExploringGroup>>;
};

const noop = () => {};

export const AwakenExploringGroupStates: AwakenExploringGroupState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenExploringGroup",
      status: "done",
      actions: [{ id: "action", title: "Read file", detail: "src/Awaken.tsx", status: "done" }],
      onSelectAction: noop,
      children: "AwakenExploringGroup",
    } as unknown as Partial<ComponentProps<typeof AwakenExploringGroup>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenExploringGroup with a deliberately long value that should remain readable inside its container",
      children: "AwakenExploringGroup with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenExploringGroup>>,
  },
];
