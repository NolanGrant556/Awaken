import type { AwakenModelSettingsProps } from "./AwakenModelSettings";

const accessTabs = [
  { id: "coding_plan", label: "Coding Plan" },
  { id: "api", label: "API" },
  { id: "agent_plan", label: "Agent Plan" },
];

const views = [
  { id: "connections", label: "配置 API Key" },
  { id: "llm", label: "LLM 设置" },
];

export const AwakenModelSettingsStates = [
  {
    id: "connections",
    label: "连接配置",
    props: {
      tabs: accessTabs,
      activeTabId: "api",
      views,
      activeViewId: "connections",
      title: "API",
      children: "模型供应商配置内容",
    },
  },
  {
    id: "llm",
    label: "LLM 设置",
    props: {
      tabs: accessTabs,
      activeTabId: "api",
      views,
      activeViewId: "llm",
      title: "API",
      children: "已配置模型内容",
    },
  },
] satisfies Array<{ id: string; label: string; props: AwakenModelSettingsProps }>;
