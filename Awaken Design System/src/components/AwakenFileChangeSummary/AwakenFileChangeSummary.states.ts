import type { ComponentProps } from "react";
import type { AwakenFileChangeSummary } from "./AwakenFileChangeSummary";

export type AwakenFileChangeSummaryState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenFileChangeSummary>>;
};

const noop = () => {};

export const AwakenFileChangeSummaryStates: AwakenFileChangeSummaryState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      files: [{ id: "file", title: "Awaken.tsx", path: "src/Awaken.tsx", status: "done" }],
      status: "done",
      title: "AwakenFileChangeSummary",
      children: "AwakenFileChangeSummary",
    } as unknown as Partial<ComponentProps<typeof AwakenFileChangeSummary>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenFileChangeSummary with a deliberately long value that should remain readable inside its container",
      children: "AwakenFileChangeSummary with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenFileChangeSummary>>,
  },
];
