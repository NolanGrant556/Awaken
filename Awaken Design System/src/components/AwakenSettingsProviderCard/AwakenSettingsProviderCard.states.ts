export const AwakenSettingsProviderCardStates = [
  { id: "expanded", label: "已展开", props: { name: "DeepSeek", iconLabel: "D", defaultExpanded: true } },
  { id: "collapsed", label: "已收起", props: { name: "Kimi Coding Plan", iconLabel: "K" } },
  { id: "disabled", label: "不可用", props: { name: "火山 Coding Plan", iconLabel: "火", disabled: true } },
] as const;
