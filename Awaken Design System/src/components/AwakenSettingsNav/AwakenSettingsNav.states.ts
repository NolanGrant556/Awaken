import type { AwakenSettingsNavProps } from "./AwakenSettingsNav";
import { Brain, Cpu, Palette, PawPrint, Search } from "lucide-react";

export const AwakenSettingsNavItems: AwakenSettingsNavProps["items"] = [
  { id: "models", label: "模型", icon: Cpu },
  { id: "search", label: "搜索", icon: Search },
  { id: "memory", label: "记忆", icon: Brain },
  { id: "appearance", label: "外观", icon: Palette },
  { id: "pets", label: "宠物", icon: PawPrint },
];

export const AwakenSettingsNavStates = [
  { id: "models", label: "模型选中", props: { items: AwakenSettingsNavItems, activeId: "models" } },
  { id: "appearance", label: "外观选中", props: { items: AwakenSettingsNavItems, activeId: "appearance" } },
] satisfies Array<{ id: string; label: string; props: AwakenSettingsNavProps }>;
