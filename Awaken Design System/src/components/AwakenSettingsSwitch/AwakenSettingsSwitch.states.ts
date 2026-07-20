import type { AwakenSettingsSwitchProps } from "./AwakenSettingsSwitch";

const noop = () => {};

export const AwakenSettingsSwitchStates = [
  { id: "enabled", label: "已开启", props: { label: "视觉能力", description: "允许模型读取并理解图片内容。", checked: true, variant: "card", onCheckedChange: noop } },
  { id: "disabled", label: "已关闭", props: { label: "工具调用", description: "允许模型调用 Awaken 提供的工具。", checked: false, variant: "card", onCheckedChange: noop } },
  { id: "busy", label: "保存中", props: { label: "deepseek-v4-pro 启用状态", checked: true, variant: "control", busy: true, onCheckedChange: noop } },
] satisfies Array<{ id: string; label: string; props: AwakenSettingsSwitchProps }>;
