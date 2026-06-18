import type React from "react";

// === 基础枚举 ===

export type PlatformKind = "macos" | "windows" | "linux" | "web";

export type ComponentSize = "sm" | "md" | "lg";

export type ComponentTone =
  | "neutral"   // 默认灰
  | "local"     // 绿色-本地模式
  | "permission" // 橙色-权限
  | "success"   // 绿色-成功
  | "warning"   // 橙色-警告
  | "error";    // 红色-错误

export type RunStepStatus = "pending" | "active" | "done" | "error";

export type ModelSelectorPlacement = "bottom-start" | "bottom-end";

// === 导航 ===

export type NavItem = {
  id: string;
  label: string;
  icon?: React.ReactNode;   // lucide-react 图标组件
  shortcut?: string;        // 如 "N", "Shift+P"
  badge?: string | number;  // 右上角徽章
  disabled?: boolean;
};

// === 对话 ===

export type ConversationItem = {
  id: string;
  label: string;       // 任务标题
  timeLabel: string;   // 显示文本，如 "now", "1h", "2h"
  disabled?: boolean;
};

// === 项目树 ===

export type ProjectItem = {
  id: string;
  label: string;
  timeLabel?: string;           // 如 "当前", "3"
  children?: ProjectItem[];     // 子项目/会话
  isLoading?: boolean;          // 展开时显示 loading ring
  disabled?: boolean;
  // 注意：展开状态由 ProjectTree 组件通过 expandedIds / defaultExpandedIds 管理，
  // 不在节点数据中保存，避免 controlled/uncontrolled 状态重复。
};

// === 快捷任务 ===

export type QuickActionItem = {
  id: string;
  label: string;       // chip 显示文本
  prompt: string;      // 点击后填入 composer 的完整文本
  icon?: React.ReactNode;
  disabled?: boolean;
};

// === 模型选项 ===

export type ModelOption = {
  id: string;
  label: string;
  disabled?: boolean;
};

// === 运行步骤 ===

export type RunStepItem = {
  id: string;
  index: number;        // 显示序号（从 1 开始）
  title: string;
  description?: string;
  tag?: string;         // 右侧 code 标签，如 "scope"
  status: RunStepStatus;
};

// === 产物 ===

export type ArtifactItem = {
  id: string;
  title: string;
  description?: string;
  onClick?: () => void;
};

// === Toast ===

export type ToastItem = {
  id: string;
  message: string;
  duration?: number;    // ms，默认 1500
  tone?: ComponentTone;
};

// === Approval / Permission ===

export type ApprovalRiskTone = "safe" | "warning" | "danger";

export type ApprovalItem = {
  id: string;
  title: string;
  description?: string;
  riskTone: ApprovalRiskTone;
  approveLabel?: string;
  rejectLabel?: string;
  affectedPaths?: string[];
  patchSummary?: string;
};

// === Composer 状态 ===

export type ComposerMode = "Plan" | "Act";

export type ComposerReadyState = boolean; // 由业务层根据 value.length > 0 计算后传入
