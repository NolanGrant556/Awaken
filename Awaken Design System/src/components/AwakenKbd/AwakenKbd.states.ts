import type { ComponentProps } from "react";
import type { AwakenKbd } from "./AwakenKbd";

export type AwakenKbdState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenKbd>>;
};

export const AwakenKbdStates: AwakenKbdState[] = [
  {
    id: "macos-command",
    label: "macOS 快捷键",
    props: {
      children: "K",
      platform: "macos",
    } as unknown as Partial<ComponentProps<typeof AwakenKbd>>,
  },
  {
    id: "macos-explicit",
    label: "已有前缀",
    props: {
      children: "⌘Enter",
      platform: "macos",
    } as unknown as Partial<ComponentProps<typeof AwakenKbd>>,
  },
  {
    id: "windows-control",
    label: "Windows 快捷键",
    props: {
      children: "Enter",
      platform: "windows",
    } as unknown as Partial<ComponentProps<typeof AwakenKbd>>,
  },
  {
    id: "long-combo",
    label: "组合快捷键",
    props: {
      children: "Ctrl+Shift+P",
      platform: "windows",
    } as unknown as Partial<ComponentProps<typeof AwakenKbd>>,
  },
];
