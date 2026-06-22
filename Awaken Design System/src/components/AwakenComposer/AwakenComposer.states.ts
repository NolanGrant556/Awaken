import type { ComponentProps } from "react";
import type { AwakenComposer } from "./AwakenComposer";

export type AwakenComposerState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposer>>;
};

const noop = () => {};

export const AwakenComposerStates: AwakenComposerState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      value: "",
      onChange: noop,
      onSubmit: noop,
      onAddContext: noop,
      mode: "Plan",
      onModeChange: noop,
      model: { id: "default", label: "Default" },
      models: [{ id: "default", label: "Default" }],
      onModelChange: noop,
      placeholder: "AwakenComposer",
      disabled: false,
      isReady: true,
      submitMode: "send",
      children: "AwakenComposer",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenComposer>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      value: "AwakenComposer with a deliberately long value that should remain readable inside its container",
      placeholder: "AwakenComposer with a deliberately long value that should remain readable inside its container",
      children: "AwakenComposer with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenComposer>>,
  },
];
