import type { ComponentProps } from "react";
import type { AwakenArtifactCard } from "./AwakenArtifactCard";

export type AwakenArtifactCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenArtifactCard>>;
};

const noop = () => {};

export const AwakenArtifactCardStates: AwakenArtifactCardState[] = [
  {
    id: "modified-file",
    label: "修改文件",
    props: {
      title: "App.tsx",
      description: "更新运行工作台的主视图结构。",
      onClick: noop,
      changeType: "modified",
      path: "apps/desktop/src/renderer/App.tsx",
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactCard>>,
  },
  {
    id: "added-file",
    label: "新增文件",
    props: {
      title: "RunSummary.tsx",
      description: "新增运行摘要卡片，用于展示任务完成后的关键结果。",
      changeType: "added",
      path: "apps/desktop/src/renderer/workbench/RunSummary.tsx",
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactCard>>,
  },
  {
    id: "deleted-file",
    label: "删除文件",
    props: {
      title: "legacy-empty-state.tsx",
      description: "移除旧版空状态示例。",
      changeType: "deleted",
      path: "apps/desktop/src/renderer/legacy-empty-state.tsx",
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactCard>>,
  },
  {
    id: "long-path",
    label: "长路径",
    props: {
      title: "AwakenConversationList.states.ts",
      description: "验证两行说明和较长路径在卡片内截断，不挤压变更图标。",
      changeType: "modified",
      path: "Awaken Design System/src/components/AwakenConversationList/AwakenConversationList.states.ts",
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactCard>>,
  },
];
