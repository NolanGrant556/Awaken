import type { AwakenPluginCardItem } from "../AwakenPluginCard";

export const awakenPluginsDemoItems: AwakenPluginCardItem[] = [
  {
    name: "acme.tools",
    version: "1.2.0",
    description: "把团队代码检查和发布工具接入 Awaken。",
    skillCount: 2,
    serverCount: 1,
    enabled: true,
  },
  {
    name: "research-kit",
    version: "0.9.0",
    description: "用于资料整理与研究工作流的插件包。",
    skillCount: 4,
    serverCount: 2,
    enabled: false,
  },
];
