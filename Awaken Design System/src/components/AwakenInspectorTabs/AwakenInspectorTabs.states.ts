import type { ComponentProps } from "react";
import type { AwakenInspectorTabs } from "./AwakenInspectorTabs";

export type AwakenInspectorTabsState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenInspectorTabs>>;
};

const noop = () => {};

export const AwakenInspectorTabsStates: AwakenInspectorTabsState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      tabs: [{ id: "changes", label: "Changes", badge: 1 }, { id: "terminal", label: "Terminal" }],
      activeTabId: "default",
      onTabChange: noop,
      children: "AwakenInspectorTabs",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorTabs>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenInspectorTabs with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorTabs>>,
  },
];
