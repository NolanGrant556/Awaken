import { AwakenModelSettingsTabs } from "../AwakenSettingsTabs";
import type { AwakenModelSettingsProps } from "./AwakenModelSettings";

export const AwakenModelSettingsStates = [
  {
    id: "configure",
    label: "模型配置",
    props: {
      tabs: AwakenModelSettingsTabs,
      activeTabId: "configure",
      children: "模型供应商配置内容",
    },
  },
  {
    id: "configured",
    label: "已配置模型",
    props: {
      tabs: AwakenModelSettingsTabs,
      activeTabId: "configured",
      children: "已配置模型内容",
    },
  },
] satisfies Array<{ id: string; label: string; props: AwakenModelSettingsProps }>;
