import { AwakenSettingsNavItems } from "../AwakenSettingsNav";
import { AwakenModelSettingsTabs } from "../AwakenSettingsTabs";
import type { AwakenSettingsProps } from "./AwakenSettings";

export const AwakenSettingsStates = [
  {
    id: "models",
    label: "模型设置",
    props: {
      navItems: AwakenSettingsNavItems,
      activeSectionId: "models",
      sectionTitle: "模型",
      tabs: AwakenModelSettingsTabs,
      activeTabId: "configure",
      children: "模型设置内容",
      onSelectSection: () => undefined,
    },
  },
  {
    id: "appearance",
    label: "外观设置",
    props: {
      navItems: AwakenSettingsNavItems,
      activeSectionId: "appearance",
      sectionTitle: "外观",
      children: "外观设置内容",
      onSelectSection: () => undefined,
    },
  },
] satisfies Array<{ id: string; label: string; props: AwakenSettingsProps }>;
