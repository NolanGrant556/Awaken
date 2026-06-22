import "./AwakenPlatformProvider.css";
import { createContext, useMemo, type ReactNode } from "react";
import type { PlatformKind } from "../../types";

export type PlatformContextValue = {
  platform: PlatformKind;
  isMac: boolean;
  isWindows: boolean;
  modifierKey: "cmd" | "ctrl";
};

export const AwakenPlatformContext = createContext<PlatformContextValue | null>(null);

export type AwakenPlatformProviderProps = {
  platform: PlatformKind;
  children: ReactNode;
};

export function AwakenPlatformProvider({ platform, children }: AwakenPlatformProviderProps) {
  const value = useMemo<PlatformContextValue>(() => {
    return {
      platform,
      isMac: platform === "macos",
      isWindows: platform === "windows",
      modifierKey: platform === "macos" ? "cmd" : "ctrl",
    };
  }, [platform]);

  return (
    <AwakenPlatformContext.Provider value={value}>{children}</AwakenPlatformContext.Provider>
  );
}
