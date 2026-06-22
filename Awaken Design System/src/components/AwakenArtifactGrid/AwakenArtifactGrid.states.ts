import type { ComponentProps } from "react";
import type { AwakenArtifactGrid } from "./AwakenArtifactGrid";

export type AwakenArtifactGridState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenArtifactGrid>>;
};

const noop = () => {};

export const AwakenArtifactGridStates: AwakenArtifactGridState[] = [
  {
    id: "changed-files",
    label: "多项产物",
    props: {
      items: [
        {
          id: "app",
          title: "App.tsx",
          description: "主工作台布局更新。",
          changeType: "modified",
          path: "apps/desktop/src/renderer/App.tsx",
          onClick: noop,
        },
        {
          id: "summary",
          title: "RunSummary.tsx",
          description: "新增运行摘要入口。",
          changeType: "added",
          path: "apps/desktop/src/renderer/workbench/RunSummary.tsx",
        },
        {
          id: "legacy",
          title: "legacy-empty-state.tsx",
          description: "删除旧版空状态示例。",
          changeType: "deleted",
          path: "apps/desktop/src/renderer/legacy-empty-state.tsx",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactGrid>>,
  },
  {
    id: "empty",
    label: "暂无产物",
    props: {
      items: [],
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactGrid>>,
  },
  {
    id: "long-file-names",
    label: "长文件名",
    props: {
      items: [
        {
          id: "long-component",
          title: "AwakenConversationList.states.ts",
          description: "用于验证网格中的卡片在长标题和长路径下仍保持等宽排列。",
          changeType: "modified",
          path: "Awaken Design System/src/components/AwakenConversationList/AwakenConversationList.states.ts",
        },
        {
          id: "long-readme",
          title: "README.md",
          description: "补充用途、场景和父子责任边界。",
          changeType: "modified",
          path: "Awaken Design System/src/components/AwakenApprovalCard/README.md",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenArtifactGrid>>,
  },
];
