import type { AwakenSkillPreviewResource } from "./AwakenSkillPreviewDialog";

export type AwakenSkillPreviewDialogState = {
  frontmatterRaw: string;
  markdown: string;
  resources: AwakenSkillPreviewResource[];
};

export const AwakenSkillPreviewDialogState: AwakenSkillPreviewDialogState = {
  frontmatterRaw: "---\nname: management-weekly\ndescription: 将跨部门进展整理为管理层周报。\nsource: 09_经营管理部\n---",
  markdown: "# 管理层周报\n\n将跨部门进展整理为管理层周报。\n\n## 使用方式\n\n1. 核对用户目标与当前工作区。\n2. 读取完成任务所需的最少上下文。\n3. 执行操作并保留可复核证据。\n\n> 这是组件库中的只读预览状态。",
  resources: [
    { path: "SKILL.md", kind: "file", sizeBytes: 1184 },
    { path: "references", kind: "directory", sizeBytes: 0 },
    { path: "references/report-contract.md", kind: "file", sizeBytes: 2840 },
    { path: "scripts", kind: "directory", sizeBytes: 0 },
    { path: "scripts/validate_report.py", kind: "file", sizeBytes: 3921 },
  ],
};
