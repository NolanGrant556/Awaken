import type { ComponentProps } from "react";
import type { AwakenComposerDock } from "./AwakenComposerDock";

export type AwakenComposerDockState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposerDock>>;
};

const noop = () => {};

export const AwakenComposerDockStates: AwakenComposerDockState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenComposerDock",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerDock>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenComposerDock with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerDock>>,
  },
];
