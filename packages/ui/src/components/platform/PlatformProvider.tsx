import { createContext, useMemo, type ReactNode } from "react";
import type { PlatformKind } from "../../types";

export type PlatformContextValue = {
  platform: PlatformKind;
  isMac: boolean;
  isWindows: boolean;
  modifierKey: "cmd" | "ctrl";
};

export const PlatformContext = createContext<PlatformContextValue | null>(null);

type PlatformProviderProps = {
  platform: PlatformKind;
  children: ReactNode;
};

export function PlatformProvider({ platform, children }: PlatformProviderProps) {
  const value = useMemo<PlatformContextValue>(() => {
    return {
      platform,
      isMac: platform === "macos",
      isWindows: platform === "windows",
      modifierKey: platform === "macos" ? "cmd" : "ctrl",
    };
  }, [platform]);

  return (
    <PlatformContext.Provider value={value}>{children}</PlatformContext.Provider>
  );
}
