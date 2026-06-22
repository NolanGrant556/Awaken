import type { ComponentProps } from "react";
import type { AwakenPlatformProvider } from "./AwakenPlatformProvider";

export type AwakenPlatformProviderState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPlatformProvider>>;
};

const noop = () => {};

export const AwakenPlatformProviderStates: AwakenPlatformProviderState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      platform: "macos",
      children: "AwakenPlatformProvider",
    } as unknown as Partial<ComponentProps<typeof AwakenPlatformProvider>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenPlatformProvider with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenPlatformProvider>>,
  },
];
