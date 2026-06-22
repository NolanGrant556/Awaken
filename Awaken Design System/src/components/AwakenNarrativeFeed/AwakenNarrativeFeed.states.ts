import type { ComponentProps } from "react";
import type { AwakenNarrativeFeed } from "./AwakenNarrativeFeed";

export type AwakenNarrativeFeedState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenNarrativeFeed>>;
};

const noop = () => {};

export const AwakenNarrativeFeedStates: AwakenNarrativeFeedState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenNarrativeFeed",
      label: "AwakenNarrativeFeed",
      emptyTitle: "AwakenNarrativeFeed",
      emptyDetail: "AwakenNarrativeFeed",
    } as unknown as Partial<ComponentProps<typeof AwakenNarrativeFeed>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenNarrativeFeed with a deliberately long value that should remain readable inside its container",
      label: "AwakenNarrativeFeed with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenNarrativeFeed>>,
  },
];
