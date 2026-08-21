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
  icon?: React.ReactNode;   // Awaken 图标库图形
  shortcut?: string;        // 如 "N", "Shift+P"
  badge?: string | number;  // 右上角徽章
  disabled?: boolean;
};

// === 对话 ===

export type ConversationItem = {
  id: string;
  label: string;       // 任务标题
  timeLabel: string;   // 显示文本，如 "now", "1h", "2h"
  statusLabel?: string;
  statusTone?: ComponentTone;
  disabled?: boolean;
};

// === 项目树 ===

export type ProjectItem = {
  id: string;
  label: string;
  kind?: "project" | "thread";
  path?: string;
  timeLabel?: string;           // 如 "当前", "3"
  children?: ProjectItem[];     // 子项目/会话
  action?: React.ReactNode;      // 行内操作，如在项目下创建新会话
  menuItems?: ProjectMenuItem[];  // 行内更多菜单项，由业务层提供行为
  isLoading?: boolean;          // 展开时显示 loading ring
  disabled?: boolean;
  hoverCard?: ProjectHoverCard;  // 悬停时在行右侧展示的文件夹组成卡片
  selection?: ProjectItemSelection; // 受控批量选择态；存在时整行切换选择而不是导航
  // 注意：展开状态由 ProjectTree 组件通过 expandedIds / defaultExpandedIds 管理，
  // 不在节点数据中保存，避免 controlled/uncontrolled 状态重复。
};

export type ProjectItemSelection = {
  checked: boolean;
  disabled?: boolean;
  disabledReason?: string;
  onChange?: (checked: boolean) => void;
};

export type ProjectHoverCardFolder = {
  path: string;
  label?: string;               // 缺省时展示完整 path
};

export type ProjectHoverCard = {
  title: string;
  summary?: string;             // 如 "3 个对话串"
  folders: ProjectHoverCardFolder[];
  emptyFoldersLabel?: string;   // 零文件夹项目的占位文案
  revealHint?: string;          // 文件夹行 title 前缀，如 "在访达中显示："
  actionLabel?: string;         // 底部动作，如 "编辑项目"
  onRevealFolder?: (path: string) => void;
  onAction?: () => void;
};

export type ProjectMenuItem = {
  id: string;
  label: string;
  tone?: "neutral" | "danger";
  disabled?: boolean;
  onSelect?: () => void;
};

// === 项目文件夹编辑 ===

export type ProjectFolderRowAction = {
  id: string;
  label: string;
  tone?: "neutral" | "danger";
  disabled?: boolean;
  onSelect?: (path: string) => void;
};

export type ProjectFolderRow = {
  path: string;
  label?: string;               // 缺省时由组件从 path 取末段
  isPrimary?: boolean;          // 主文件夹徽标
  onMakePrimary?: (path: string) => void;  // 非主文件夹行悬停/聚焦时露出
  onRemove?: (path: string) => void;       // 常显 ×，把文件夹移出这一组
  actions?: ProjectFolderRowAction[];      // 仅承载切主与移出之外的额外操作
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

export type ComposerReadyState = boolean; // 由业务层根据 value.length > 0 计算后传入
