import type { ComponentProps } from "react";
import type { AwakenApprovalCard } from "./AwakenApprovalCard";

export type AwakenApprovalCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenApprovalCard>>;
};

const noop = () => {};

export const AwakenApprovalCardStates: AwakenApprovalCardState[] = [
  {
    id: "pending-file-change",
    label: "等待审批",
    props: {
      title: "允许修改工作台布局文件？",
      description: "代理准备更新运行工作台的空状态文案和间距。",
      affectedPaths: [
        "apps/desktop/src/renderer/App.tsx",
        "apps/desktop/src/renderer/workbench/Layout.tsx",
      ],
      patchSummary: "调整中心输入区的说明文案，并保持现有运行状态不变。",
      riskTone: "warning",
      approveLabel: "允许修改",
      rejectLabel: "拒绝",
      onApprove: noop,
      onReject: noop,
      open: true,
      inline: true,
      onOpenChange: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenApprovalCard>>,
  },
  {
    id: "dangerous-command",
    label: "高风险操作",
    props: {
      title: "确认运行清理命令？",
      description: "该操作会删除本地构建缓存，可能影响当前调试会话。",
      affectedPaths: ["dist-electron/", "apps/desktop/.vite/"],
      patchSummary: "执行前需要用户确认，组件只负责呈现风险和回调选择。",
      riskTone: "danger",
      approveLabel: "继续执行",
      rejectLabel: "取消",
      onApprove: noop,
      onReject: noop,
      open: true,
      inline: true,
      onOpenChange: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenApprovalCard>>,
  },
  {
    id: "resolving",
    label: "处理中",
    props: {
      title: "正在提交审批结果",
      description: "按钮在回调处理中保持不可再次点击。",
      riskTone: "safe",
      resolving: true,
      onApprove: noop,
      onReject: noop,
      open: true,
      inline: true,
      onOpenChange: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenApprovalCard>>,
  },
  {
    id: "approved",
    label: "已允许",
    props: {
      title: "已允许读取项目配置",
      description: "决策完成后按钮显示最终选择并进入不可交互状态。",
      riskTone: "safe",
      resolvedDecision: "approved",
      onApprove: noop,
      onReject: noop,
      open: true,
      inline: true,
    } as unknown as Partial<ComponentProps<typeof AwakenApprovalCard>>,
  },
  {
    id: "long-paths",
    label: "长路径摘要",
    props: {
      title: "允许更新一组较长路径的设计系统示例？",
      description: "用于确认标题、说明、路径列表和摘要在窄面板内保持可读。",
      affectedPaths: [
        "Awaken Design System/src/components/AwakenComposerBar/AwakenComposerBar.states.ts",
        "Awaken Design System/src/components/AwakenConversationList/AwakenConversationList.states.ts",
      ],
      patchSummary: "补齐真实语义状态，保留 Gallery 可观测状态结构。",
      riskTone: "warning",
      approveLabel: "允许",
      rejectLabel: "拒绝",
      onApprove: noop,
      onReject: noop,
      open: true,
      inline: true,
    } as unknown as Partial<ComponentProps<typeof AwakenApprovalCard>>,
  },
];
