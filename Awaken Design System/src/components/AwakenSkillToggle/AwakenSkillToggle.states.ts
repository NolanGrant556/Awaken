import type { AwakenSkillToggleProps } from "./AwakenSkillToggle";

export type AwakenSkillToggleState = {
  id: string;
  label: string;
  props: AwakenSkillToggleProps;
};

const noop = () => {};

export const AwakenSkillToggleStates: AwakenSkillToggleState[] = [
  { id: "enabled", label: "已启用", props: { checked: true, label: "经营分析", onCheckedChange: noop } },
  { id: "disabled", label: "已停用", props: { checked: false, label: "经营分析", onCheckedChange: noop } },
  { id: "busy", label: "处理中", props: { checked: true, label: "经营分析", busy: true, onCheckedChange: noop } },
  { id: "locked", label: "不可操作", props: { checked: false, label: "经营分析", disabled: true, onCheckedChange: noop } },
];
