import type { ComponentProps } from "react";
import type { AwakenComposerInput } from "./AwakenComposerInput";

export type AwakenComposerInputState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenComposerInput>>;
};

const noop = () => {};

export const AwakenComposerInputStates: AwakenComposerInputState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      value: "",
      onChange: noop,
      onSubmit: noop,
      placeholder: "AwakenComposerInput",
      minRows: 1,
      maxRows: 4,
      disabled: false,
      children: "AwakenComposerInput",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerInput>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      value: "AwakenComposerInput with a deliberately long value that should remain readable inside its container",
      placeholder: "AwakenComposerInput with a deliberately long value that should remain readable inside its container",
      children: "AwakenComposerInput with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenComposerInput>>,
  },
];
