import { createElement, type ComponentProps } from "react";
import { Monitor } from "lucide-react";
import type { AwakenSelectableCard } from "./AwakenSelectableCard";

export type AwakenSelectableCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSelectableCard>>;
};

const noop = () => {};

export const AwakenSelectableCardStates: AwakenSelectableCardState[] = [
  {
    id: "local-selected",
    label: "本地已选",
    props: {
      selected: true,
      icon: createElement(Monitor, { size: 18, strokeWidth: 1.9 }),
      title: "Local",
      description: "Use a folder on this Mac as an Awaken project.",
      onSelect: noop,
    } as Partial<ComponentProps<typeof AwakenSelectableCard>>,
  },
  {
    id: "local-available",
    label: "本地可选",
    props: {
      selected: false,
      icon: createElement(Monitor, { size: 18, strokeWidth: 1.9 }),
      title: "Local",
      description: "Use a folder on this Mac as an Awaken project.",
      onSelect: noop,
    } as Partial<ComponentProps<typeof AwakenSelectableCard>>,
  },
];
