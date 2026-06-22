import type { ComponentProps } from "react";
import type { AwakenArtifactCard } from "./AwakenArtifactCard";

export type AwakenArtifactCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenArtifactCard>>;
};

const noop = () => {};

export const AwakenArtifactCardStates: AwakenArtifactCardState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenArtifactCard",
      description: "AwakenArtifactCard",
      onClick: noop,
      changeType: "AwakenArtifactCard",
      path: "AwakenArtifactCard",
      children: "AwakenArtifactCard",
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactCard>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenArtifactCard with a deliberately long value that should remain readable inside its container",
      description: "AwakenArtifactCard with a deliberately long value that should remain readable inside its container",
      children: "AwakenArtifactCard with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactCard>>,
  },
];
