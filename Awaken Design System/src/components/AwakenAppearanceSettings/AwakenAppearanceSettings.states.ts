import type { AwakenAppearanceSettingsProps } from "./AwakenAppearanceSettings";

export const AwakenAppearanceSettingsStates = [
  { id: "system", label: "跟随系统", props: { value: "system" } },
  { id: "light", label: "浅色", props: { value: "light" } },
  { id: "dark", label: "深色", props: { value: "dark" } },
] satisfies Array<{ id: string; label: string; props: AwakenAppearanceSettingsProps }>;
