import type { AwakenPluginCardItem } from "../AwakenPluginCard";

export const awakenPluginsDemoItems: AwakenPluginCardItem[] = [
  {
    name: "acme.tools",
    description: "把团队代码检查和发布工具接入 Awaken。",
    dialect: "codex",
    enabled: true,
  },
  {
    name: "research-kit",
    description: "用于资料整理与研究工作流的插件包。",
    dialect: "claude_code",
    enabled: false,
  },
];
