import type { ComponentProps } from "react";
import type { AwakenConversationList } from "./AwakenConversationList";

export type AwakenConversationListState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenConversationList>>;
};

const noop = () => {};

export const AwakenConversationListStates: AwakenConversationListState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      items: [{ id: "thread", title: "Component review", timeLabel: "now" }],
      activeId: "default",
      onSelect: noop,
      onContextMenu: noop,
      children: "AwakenConversationList",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenConversationList>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenConversationList with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenConversationList>>,
  },
];
