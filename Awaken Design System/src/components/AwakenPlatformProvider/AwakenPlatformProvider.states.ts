import type { ComponentProps } from "react";
import type { AwakenPlatformProvider } from "./AwakenPlatformProvider";

export type AwakenPlatformProviderState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPlatformProvider>>;
};

export const AwakenPlatformProviderStates: AwakenPlatformProviderState[] = [
  {
    id: "macos",
    label: "macOS 上下文",
    props: {
      platform: "macos",
      children: "子组件会把主修饰键解释为 Cmd。",
    } as unknown as Partial<ComponentProps<typeof AwakenPlatformProvider>>,
  },
  {
    id: "windows",
    label: "Windows 上下文",
    props: {
      platform: "windows",
      children: "子组件会把主修饰键解释为 Ctrl。",
    } as unknown as Partial<ComponentProps<typeof AwakenPlatformProvider>>,
  },
  {
    id: "web",
    label: "Web 预览",
    props: {
      platform: "web",
      children: "Gallery 或 Web 预览可以显式传入平台，避免组件自行探测系统。",
    } as unknown as Partial<ComponentProps<typeof AwakenPlatformProvider>>,
  },
];
