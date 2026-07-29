export const AwakenSettingsProviderCardStates = [
  { id: "default", label: "默认", props: { name: "DeepSeek", brand: "deepseek" } },
  { id: "selected", label: "已选择", props: { name: "Kimi Coding Plan", brand: "kimi", selected: true } },
  { id: "disabled", label: "不可用", props: { name: "火山 Coding Plan", brand: "volcengine", disabled: true } },
] as const;
