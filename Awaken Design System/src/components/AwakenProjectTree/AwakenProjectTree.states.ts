import type { ComponentProps } from "react";
import type { AwakenProjectTree } from "./AwakenProjectTree";

export type AwakenProjectTreeState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenProjectTree>>;
};

const noop = () => {};

export const AwakenProjectTreeStates: AwakenProjectTreeState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      items: [{ id: "awaken", label: "awaken" }],
      activeId: "default",
      expandedIds: ["awaken"],
      defaultExpandedIds: ["awaken"],
      onExpandedChange: noop,
      onSelect: noop,
      children: "AwakenProjectTree",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenProjectTree with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
];
