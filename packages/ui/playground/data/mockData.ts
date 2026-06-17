import type { ConversationItem, ProjectItem } from "../../src";

export const conversations: ConversationItem[] = [
  { id: "conv-status", label: "检查 Claude Code 任务状态", timeLabel: "1 天" },
  { id: "conv-model", label: "查看 Claude Code 模型配置", timeLabel: "2 天" },
  { id: "conv-proxy", label: "排查代理节点 ERROR", timeLabel: "3 天" },
  { id: "conv-local", label: "查看本地代理环境配置", timeLabel: "3 天" },
];

export const projects: ProjectItem[] = [
  {
    id: "project-awaken",
    label: "Awaken",
    children: [
      { id: "design-components", label: "设计前端设计组件", timeLabel: "1 分", isLoading: true },
      { id: "branch-base", label: "基础01-分支", timeLabel: "2 小时" },
      { id: "file-purpose", label: "查看 awaken 文件用途", timeLabel: "7 小时" },
      { id: "open-command", label: "获取 Claude Code 打开命令", timeLabel: "21 小时" },
      { id: "base-task", label: "基础01", timeLabel: "3 天" },
    ],
  },
  { id: "project-source", label: "Codex 源代码" },
  { id: "project-claude", label: "claudecode源代码" },
  { id: "project-openai", label: "OpenAI_Agents_SDK_TS" },
  { id: "project-high", label: "High Agent" },
  { id: "project-go", label: "Go_Agent" },
  { id: "project-knowledge", label: "我的知识库" },
  { id: "project-skill", label: "skill编写" },
  { id: "project-trae", label: "TRAE社区skill" },
  { id: "project-agent-kb", label: "Agent-Knowledge-Base" },
];
