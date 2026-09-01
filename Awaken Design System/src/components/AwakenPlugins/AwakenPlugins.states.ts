import type { AwakenPluginCardItem } from "../AwakenPluginCard";

export const awakenPluginsDemoItems: AwakenPluginCardItem[] = [
  {
    name: "awaken-outlook",
    displayName: "Outlook 邮箱",
    description: "读取、整理、起草、回复、发送和删除 Outlook 邮件。",
    dialect: "agent_plugins",
    builtin: true,
    brandIcon: "outlook",
    builtinOrder: 0,
    enabled: false,
  },
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
