import type { ComponentProps } from "react";
import type { AwakenComposerBar } from "./AwakenComposerBar";

export type AwakenComposerBarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposerBar>>;
};

const noop = () => {};

export const AwakenComposerBarStates: AwakenComposerBarState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      value: "",
      onChange: noop,
      onSubmit: noop,
      onAddContext: noop,
      placeholder: "AwakenComposerBar",
      disabled: false,
      isReady: true,
      submitMode: "send",
      variant: "dock",
      metaItems: [{ id: "repo", label: "awaken" }, { id: "model", label: "Default" }],
      children: "AwakenComposerBar",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      value: "AwakenComposerBar with a deliberately long value that should remain readable inside its container",
      placeholder: "AwakenComposerBar with a deliberately long value that should remain readable inside its container",
      children: "AwakenComposerBar with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerBar>>,
  },
];
