import type { ComponentProps } from "react";
import type { AwakenFileChangeSummary } from "./AwakenFileChangeSummary";

export type AwakenFileChangeSummaryState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenFileChangeSummary>>;
};

export const AwakenFileChangeSummaryStates: AwakenFileChangeSummaryState[] = [
  {
    id: "changes-done",
    label: "改动完成",
    props: {
      files: [
        {
          id: "copy-readme",
          title: "补充复制按钮用途",
          path: "AwakenCopyButton/README.md",
          description: "说明复制内容来源由父级传入，组件只负责剪贴板反馈。",
          status: "done",
        },
        {
          id: "copy-states",
          title: "替换复制按钮状态示例",
          path: "AwakenCopyButton/AwakenCopyButton.states.ts",
          description: "用回答、命令和空内容覆盖真实复制场景。",
          status: "done",
        },
      ],
      status: "done",
      title: "本次文件改动",
    } as unknown as Partial<ComponentProps<typeof AwakenFileChangeSummary>>,
  },
  {
    id: "changes-active",
    label: "仍在更新",
    props: {
      title: "正在写入组件文档",
      status: "active",
      files: [
        {
          id: "markdown-readme",
          title: "更新 MarkdownAnswer 用途说明",
          path: "AwakenMarkdownAnswer/README.md",
          description: "明确仅支持段落、列表和行内 code。",
          status: "active",
        },
        {
          id: "inspector-demo",
          title: "检查器 demo 文案已完成",
          path: "AwakenInspectorRail/AwakenInspectorRail.demo.tsx",
          status: "done",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenFileChangeSummary>>,
  },
  {
    id: "file-error",
    label: "文件出错",
    props: {
      title: "文件更新遇到问题",
      status: "active",
      files: [
        {
          id: "state-error",
          title: "状态示例未通过类型检查",
          path: "AwakenInspectorTabs/AwakenInspectorTabs.states.ts",
          description: "activeTabId 必须使用 changes、browser、terminal 或 files。",
          status: "error",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenFileChangeSummary>>,
  },
  {
    id: "empty-files",
    label: "暂无改动",
    props: {
      title: "暂无文件改动",
      status: "done",
      files: [],
    } as unknown as Partial<ComponentProps<typeof AwakenFileChangeSummary>>,
  },
  {
    id: "long-path",
    label: "长路径",
    props: {
      title: "包含长路径的改动摘要",
      files: [
        {
          id: "long-readme",
          title: "补充一个较长目录名下的组件用途说明",
          path: "/Users/nolangrant/Awaken/Awaken Design System/src/components/AwakenFileChangeSummary/README.md",
          description: "验证文件路径和说明在摘要卡片内正确换行，不由父级额外截断。",
          status: "done",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenFileChangeSummary>>,
  },
];
