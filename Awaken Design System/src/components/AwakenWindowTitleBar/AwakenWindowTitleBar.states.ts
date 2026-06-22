import type { ComponentProps } from "react";
import type { AwakenWindowTitleBar } from "./AwakenWindowTitleBar";

export type AwakenWindowTitleBarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWindowTitleBar>>;
};

const noop = () => {};

export const AwakenWindowTitleBarStates: AwakenWindowTitleBarState[] = [
  {
    id: "macos",
    label: "macOS 控制区",
    props: {
      platform: "macos",
    } as unknown as Partial<ComponentProps<typeof AwakenWindowTitleBar>>,
  },
  {
    id: "windows",
    label: "Windows 控制区",
    props: {
      platform: "windows",
      onMinimize: noop,
      onMaximize: noop,
      onClose: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenWindowTitleBar>>,
  },
  {
    id: "windows-disabled",
    label: "Windows 禁用按钮",
    props: {
      platform: "windows",
      onMinimize: undefined,
      onMaximize: undefined,
      onClose: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenWindowTitleBar>>,
  },
  {
    id: "non-desktop",
    label: "非桌面平台",
    props: {
      platform: "web",
    } as unknown as Partial<ComponentProps<typeof AwakenWindowTitleBar>>,
  },
];
