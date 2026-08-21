import type { AwakenSettingsNavProps } from "./AwakenSettingsNav";

export const AwakenSettingsNavItems: AwakenSettingsNavProps["items"] = [
  { id: "models", label: "模型", concept: "settings-model" },
  { id: "search", label: "搜索", concept: "search" },
  { id: "memory", label: "记忆", concept: "settings-memory" },
  { id: "instructions", label: "全局规则", concept: "context-file" },
  { id: "appearance", label: "外观", concept: "settings-appearance" },
  { id: "pets", label: "宠物", concept: "settings-pets" },
  { id: "computer", label: "计算机操作", concept: "computer-use" },
  { id: "hooks", label: "钩子", concept: "hook" },
  // ACP 配置的是外部 runner 进程，用机器人头像与本机能力型分区区分开。
  { id: "acp", label: "ACP", concept: "settings-acp" },
];

export const AwakenSettingsNavStates = [
  { id: "models", label: "模型选中", props: { items: AwakenSettingsNavItems, activeId: "models" } },
  { id: "appearance", label: "外观选中", props: { items: AwakenSettingsNavItems, activeId: "appearance" } },
] satisfies Array<{ id: string; label: string; props: AwakenSettingsNavProps }>;
