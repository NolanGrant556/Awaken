import type { ComponentProps } from "react";
import type { AwakenAssistantMessage } from "./AwakenAssistantMessage";

export type AwakenAssistantMessageState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenAssistantMessage>>;
};

const noop = () => {};

export const AwakenAssistantMessageStates: AwakenAssistantMessageState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      markdown: "AwakenAssistantMessage **markdown**",
      status: "done",
      children: "AwakenAssistantMessage",
    } as unknown as Partial<ComponentProps<typeof AwakenAssistantMessage>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      markdown: "AwakenAssistantMessage with a deliberately long value that should remain readable inside its container **markdown**",
      children: "AwakenAssistantMessage with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenAssistantMessage>>,
  },
];
