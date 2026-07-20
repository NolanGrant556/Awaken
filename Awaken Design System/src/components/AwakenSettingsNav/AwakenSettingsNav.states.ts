import type { AwakenSettingsNavProps } from "./AwakenSettingsNav";

export const AwakenSettingsNavItems: AwakenSettingsNavProps["items"] = [
  { id: "models", label: "模型" },
  { id: "search", label: "搜索" },
  { id: "memory", label: "记忆" },
  { id: "appearance", label: "外观" },
  { id: "pets", label: "宠物" },
];

export const AwakenSettingsNavStates = [
  { id: "models", label: "模型选中", props: { items: AwakenSettingsNavItems, activeId: "models" } },
  { id: "appearance", label: "外观选中", props: { items: AwakenSettingsNavItems, activeId: "appearance" } },
] satisfies Array<{ id: string; label: string; props: AwakenSettingsNavProps }>;
