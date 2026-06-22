import type { ComponentProps } from "react";
import type { AwakenArtifactGrid } from "./AwakenArtifactGrid";

export type AwakenArtifactGridState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenArtifactGrid>>;
};

const noop = () => {};

export const AwakenArtifactGridStates: AwakenArtifactGridState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      items: [{ id: "artifact", title: "Component file", description: "Generated state", changeType: "modified", path: "Awaken.tsx" }],
      children: "AwakenArtifactGrid",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactGrid>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenArtifactGrid with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactGrid>>,
  },
];
