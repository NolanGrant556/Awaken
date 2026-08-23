import { createContext, type ReactNode, useContext } from "react";
import type { AwakenIconConcept } from "../../icons";

export type AwakenSettingsSectionContextValue = {
  title: string;
  concept?: AwakenIconConcept;
};

const AwakenSettingsSectionContext =
  createContext<AwakenSettingsSectionContextValue | null>(null);

export function AwakenSettingsSectionProvider({
  value,
  children,
}: {
  value: AwakenSettingsSectionContextValue;
  children: ReactNode;
}) {
  return (
    <AwakenSettingsSectionContext.Provider value={value}>
      {children}
    </AwakenSettingsSectionContext.Provider>
  );
}

export function useAwakenSettingsSection(): AwakenSettingsSectionContextValue | null {
  return useContext(AwakenSettingsSectionContext);
}
