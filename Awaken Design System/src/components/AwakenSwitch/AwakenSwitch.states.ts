import type { AwakenSwitchProps } from "./AwakenSwitch";

const noop = () => {};

export const AwakenSwitchStates = [
  { id: "on", label: "开启", props: { checked: true, label: "自动同步", onCheckedChange: noop } },
  { id: "off", label: "关闭", props: { checked: false, label: "自动同步", onCheckedChange: noop } },
  { id: "disabled", label: "不可用", props: { checked: false, label: "自动同步", disabled: true, onCheckedChange: noop } },
  { id: "busy", label: "处理中", props: { checked: true, label: "自动同步", busy: true, onCheckedChange: noop } },
  { id: "compact", label: "紧凑", props: { checked: true, label: "模型能力", size: "compact", onCheckedChange: noop } },
] satisfies Array<{ id: string; label: string; props: AwakenSwitchProps }>;
