import type { AwakenSettingsTabsProps } from "./AwakenSettingsTabs";

export const AwakenModelSettingsTabs: AwakenSettingsTabsProps["items"] = [
  { id: "configure", label: "模型配置" },
  { id: "configured", label: "已配置模型" },
];

export const AwakenSettingsTabsStates = [
  { id: "configure", label: "模型配置", props: { items: AwakenModelSettingsTabs, activeId: "configure" } },
  { id: "configured", label: "已配置模型", props: { items: AwakenModelSettingsTabs, activeId: "configured" } },
] satisfies Array<{ id: string; label: string; props: AwakenSettingsTabsProps }>;
