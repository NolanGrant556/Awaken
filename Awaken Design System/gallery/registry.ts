import type { ComponentType } from "react";
import { AwakenActivityLineDemo } from "../src/components/AwakenActivityLine";
import { AwakenApprovalCardDemo } from "../src/components/AwakenApprovalCard";
import { AwakenAppShellDemo } from "../src/components/AwakenAppShell";
import { AwakenArtifactCardDemo } from "../src/components/AwakenArtifactCard";
import { AwakenArtifactGridDemo } from "../src/components/AwakenArtifactGrid";
import { AwakenAssistantMessageDemo } from "../src/components/AwakenAssistantMessage";
import { AwakenAvatarDemo } from "../src/components/AwakenAvatar";
import { AwakenBrandRowDemo } from "../src/components/AwakenBrandRow";
import { AwakenButtonDemo } from "../src/components/AwakenButton";
import { AwakenCenterStageDemo } from "../src/components/AwakenCenterStage";
import { AwakenComposerDemo } from "../src/components/AwakenComposer";
import { AwakenComposerBarDemo } from "../src/components/AwakenComposerBar";
import { AwakenComposerDockDemo } from "../src/components/AwakenComposerDock";
import { AwakenComposerInputDemo } from "../src/components/AwakenComposerInput";
import { AwakenContextButtonDemo } from "../src/components/AwakenContextButton";
import { AwakenConversationListDemo } from "../src/components/AwakenConversationList";
import { AwakenCopyButtonDemo } from "../src/components/AwakenCopyButton";
import { AwakenDisclosureDemo } from "../src/components/AwakenDisclosure";
import { AwakenDividerDemo } from "../src/components/AwakenDivider";
import { AwakenDialogDemo } from "../src/components/AwakenDialog";
import { AwakenDropdownDemo } from "../src/components/AwakenDropdown";
import { AwakenExecutionFeedDemo } from "../src/components/AwakenExecutionFeed";
import { AwakenExploringGroupDemo } from "../src/components/AwakenExploringGroup";
import { AwakenFileChangeSummaryDemo } from "../src/components/AwakenFileChangeSummary";
import { AwakenIconDemo } from "../src/components/AwakenIcon";
import { AwakenIconButtonDemo } from "../src/components/AwakenIconButton";
import { AwakenInspectorRailDemo } from "../src/components/AwakenInspectorRail";
import { AwakenInspectorTabsDemo } from "../src/components/AwakenInspectorTabs";
import { AwakenKbdDemo } from "../src/components/AwakenKbd";
import { AwakenLightInspectorRailDemo } from "../src/components/AwakenLightInspectorRail";
import { AwakenMarkdownAnswerDemo } from "../src/components/AwakenMarkdownAnswer";
import { AwakenModelSelectorDemo } from "../src/components/AwakenModelSelector";
import { AwakenModeToggleDemo } from "../src/components/AwakenModeToggle";
import { AwakenNarrativeFeedDemo } from "../src/components/AwakenNarrativeFeed";
import { AwakenNewTaskButtonDemo } from "../src/components/AwakenNewTaskButton";
import { AwakenOriginalThinkingDemo } from "../src/components/AwakenOriginalThinking";
import { AwakenPillDemo } from "../src/components/AwakenPill";
import { AwakenPlatformProviderDemo } from "../src/components/AwakenPlatformProvider";
import { AwakenPopoverDemo } from "../src/components/AwakenPopover";
import { AwakenPrimaryNavDemo } from "../src/components/AwakenPrimaryNav";
import { AwakenProfileButtonDemo } from "../src/components/AwakenProfileButton";
import { AwakenProjectTreeDemo } from "../src/components/AwakenProjectTree";
import { AwakenPromptCardDemo } from "../src/components/AwakenPromptCard";
import { AwakenQuickActionChipDemo } from "../src/components/AwakenQuickActionChip";
import { AwakenQuickActionGridDemo } from "../src/components/AwakenQuickActionGrid";
import { AwakenRunStageDemo } from "../src/components/AwakenRunStage";
import { AwakenRunStepDemo } from "../src/components/AwakenRunStep";
import { AwakenRunTimelineDemo } from "../src/components/AwakenRunTimeline";
import { AwakenSendButtonDemo } from "../src/components/AwakenSendButton";
import { AwakenSelectableCardDemo } from "../src/components/AwakenSelectableCard";
import { AwakenSidebarDemo } from "../src/components/AwakenSidebar";
import { AwakenSidebarSectionActionDemo } from "../src/components/AwakenSidebarSectionAction";
import { AwakenStatusPillDemo } from "../src/components/AwakenStatusPill";
import { AwakenTaskWelcomeDemo } from "../src/components/AwakenTaskWelcome";
import { AwakenThinkingBlockDemo } from "../src/components/AwakenThinkingBlock";
import { AwakenThoughtTraceDemo } from "../src/components/AwakenThoughtTrace";
import { AwakenToastDemo } from "../src/components/AwakenToast";
import { AwakenToolActionRowDemo } from "../src/components/AwakenToolActionRow";
import { AwakenTooltipDemo } from "../src/components/AwakenTooltip";
import { AwakenToolTraceGroupDemo } from "../src/components/AwakenToolTraceGroup";
import { AwakenToolTraceLineDemo } from "../src/components/AwakenToolTraceLine";
import { AwakenTopBarDemo } from "../src/components/AwakenTopBar";
import { AwakenUserPromptBlockDemo } from "../src/components/AwakenUserPromptBlock";
import { AwakenWindowTitleBarDemo } from "../src/components/AwakenWindowTitleBar";
import { AwakenWorkbenchChromeDemo } from "../src/components/AwakenWorkbenchChrome";
import { AwakenWorkbenchShellDemo } from "../src/components/AwakenWorkbenchShell";
import { AwakenWorkbenchSidebarFrameDemo } from "../src/components/AwakenWorkbenchSidebarFrame";

export type AwakenGalleryEntry = {
  id: string;
  name: string;
  displayName: string;
  demo: ComponentType;
};

export const awakenGalleryEntries: AwakenGalleryEntry[] = [
  { id: "awaken-activity-line", name: "AwakenActivityLine", displayName: "活动状态行", demo: AwakenActivityLineDemo },
  { id: "awaken-approval-card", name: "AwakenApprovalCard", displayName: "审批卡片", demo: AwakenApprovalCardDemo },
  { id: "awaken-app-shell", name: "AwakenAppShell", displayName: "应用外壳", demo: AwakenAppShellDemo },
  { id: "awaken-artifact-card", name: "AwakenArtifactCard", displayName: "产物卡片", demo: AwakenArtifactCardDemo },
  { id: "awaken-artifact-grid", name: "AwakenArtifactGrid", displayName: "产物网格", demo: AwakenArtifactGridDemo },
  { id: "awaken-assistant-message", name: "AwakenAssistantMessage", displayName: "助手消息", demo: AwakenAssistantMessageDemo },
  { id: "awaken-avatar", name: "AwakenAvatar", displayName: "头像", demo: AwakenAvatarDemo },
  { id: "awaken-brand-row", name: "AwakenBrandRow", displayName: "品牌行", demo: AwakenBrandRowDemo },
  { id: "awaken-button", name: "AwakenButton", displayName: "按钮", demo: AwakenButtonDemo },
  { id: "awaken-center-stage", name: "AwakenCenterStage", displayName: "中央舞台", demo: AwakenCenterStageDemo },
  { id: "awaken-composer", name: "AwakenComposer", displayName: "指令编写器", demo: AwakenComposerDemo },
  { id: "awaken-composer-bar", name: "AwakenComposerBar", displayName: "输入栏", demo: AwakenComposerBarDemo },
  { id: "awaken-composer-dock", name: "AwakenComposerDock", displayName: "输入停靠栏", demo: AwakenComposerDockDemo },
  { id: "awaken-composer-input", name: "AwakenComposerInput", displayName: "输入框", demo: AwakenComposerInputDemo },
  { id: "awaken-context-button", name: "AwakenContextButton", displayName: "上下文按钮", demo: AwakenContextButtonDemo },
  { id: "awaken-conversation-list", name: "AwakenConversationList", displayName: "会话列表", demo: AwakenConversationListDemo },
  { id: "awaken-copy-button", name: "AwakenCopyButton", displayName: "复制按钮", demo: AwakenCopyButtonDemo },
  { id: "awaken-disclosure", name: "AwakenDisclosure", displayName: "展开折叠", demo: AwakenDisclosureDemo },
  { id: "awaken-divider", name: "AwakenDivider", displayName: "分隔线", demo: AwakenDividerDemo },
  { id: "awaken-dialog", name: "AwakenDialog", displayName: "弹窗", demo: AwakenDialogDemo },
  { id: "awaken-dropdown", name: "AwakenDropdown", displayName: "下拉菜单", demo: AwakenDropdownDemo },
  { id: "awaken-execution-feed", name: "AwakenExecutionFeed", displayName: "执行动态流", demo: AwakenExecutionFeedDemo },
  { id: "awaken-exploring-group", name: "AwakenExploringGroup", displayName: "探索分组", demo: AwakenExploringGroupDemo },
  { id: "awaken-file-change-summary", name: "AwakenFileChangeSummary", displayName: "文件变更摘要", demo: AwakenFileChangeSummaryDemo },
  { id: "awaken-icon", name: "AwakenIcon", displayName: "图标", demo: AwakenIconDemo },
  { id: "awaken-icon-button", name: "AwakenIconButton", displayName: "图标按钮", demo: AwakenIconButtonDemo },
  { id: "awaken-inspector-rail", name: "AwakenInspectorRail", displayName: "检查器侧轨", demo: AwakenInspectorRailDemo },
  { id: "awaken-inspector-tabs", name: "AwakenInspectorTabs", displayName: "检查器标签", demo: AwakenInspectorTabsDemo },
  { id: "awaken-kbd", name: "AwakenKbd", displayName: "快捷键标记", demo: AwakenKbdDemo },
  { id: "awaken-light-inspector-rail", name: "AwakenLightInspectorRail", displayName: "浅色检查器侧轨", demo: AwakenLightInspectorRailDemo },
  { id: "awaken-markdown-answer", name: "AwakenMarkdownAnswer", displayName: "Markdown 答案", demo: AwakenMarkdownAnswerDemo },
  { id: "awaken-model-selector", name: "AwakenModelSelector", displayName: "模型选择器", demo: AwakenModelSelectorDemo },
  { id: "awaken-mode-toggle", name: "AwakenModeToggle", displayName: "模式切换", demo: AwakenModeToggleDemo },
  { id: "awaken-narrative-feed", name: "AwakenNarrativeFeed", displayName: "叙事动态流", demo: AwakenNarrativeFeedDemo },
  { id: "awaken-new-task-button", name: "AwakenNewTaskButton", displayName: "新任务按钮", demo: AwakenNewTaskButtonDemo },
  { id: "awaken-original-thinking", name: "AwakenOriginalThinking", displayName: "原始思考动效", demo: AwakenOriginalThinkingDemo },
  { id: "awaken-pill", name: "AwakenPill", displayName: "胶囊标签", demo: AwakenPillDemo },
  { id: "awaken-platform-provider", name: "AwakenPlatformProvider", displayName: "平台提供器", demo: AwakenPlatformProviderDemo },
  { id: "awaken-popover", name: "AwakenPopover", displayName: "弹出层", demo: AwakenPopoverDemo },
  { id: "awaken-primary-nav", name: "AwakenPrimaryNav", displayName: "主导航", demo: AwakenPrimaryNavDemo },
  { id: "awaken-profile-button", name: "AwakenProfileButton", displayName: "个人入口", demo: AwakenProfileButtonDemo },
  { id: "awaken-project-tree", name: "AwakenProjectTree", displayName: "项目树", demo: AwakenProjectTreeDemo },
  { id: "awaken-prompt-card", name: "AwakenPromptCard", displayName: "提示卡片", demo: AwakenPromptCardDemo },
  { id: "awaken-quick-action-chip", name: "AwakenQuickActionChip", displayName: "快捷动作标签", demo: AwakenQuickActionChipDemo },
  { id: "awaken-quick-action-grid", name: "AwakenQuickActionGrid", displayName: "快捷动作网格", demo: AwakenQuickActionGridDemo },
  { id: "awaken-run-stage", name: "AwakenRunStage", displayName: "运行阶段", demo: AwakenRunStageDemo },
  { id: "awaken-run-step", name: "AwakenRunStep", displayName: "运行步骤", demo: AwakenRunStepDemo },
  { id: "awaken-run-timeline", name: "AwakenRunTimeline", displayName: "运行时间线", demo: AwakenRunTimelineDemo },
  { id: "awaken-send-button", name: "AwakenSendButton", displayName: "发送按钮", demo: AwakenSendButtonDemo },
  { id: "awaken-selectable-card", name: "AwakenSelectableCard", displayName: "选择卡片", demo: AwakenSelectableCardDemo },
  { id: "awaken-sidebar", name: "AwakenSidebar", displayName: "侧栏", demo: AwakenSidebarDemo },
  { id: "awaken-sidebar-section-action", name: "AwakenSidebarSectionAction", displayName: "侧栏区块动作", demo: AwakenSidebarSectionActionDemo },
  { id: "awaken-status-pill", name: "AwakenStatusPill", displayName: "状态胶囊", demo: AwakenStatusPillDemo },
  { id: "awaken-task-welcome", name: "AwakenTaskWelcome", displayName: "任务欢迎区", demo: AwakenTaskWelcomeDemo },
  { id: "awaken-thinking-block", name: "AwakenThinkingBlock", displayName: "思考块", demo: AwakenThinkingBlockDemo },
  { id: "awaken-thought-trace", name: "AwakenThoughtTrace", displayName: "思路轨迹", demo: AwakenThoughtTraceDemo },
  { id: "awaken-toast", name: "AwakenToast", displayName: "提示通知", demo: AwakenToastDemo },
  { id: "awaken-tool-action-row", name: "AwakenToolActionRow", displayName: "工具操作行", demo: AwakenToolActionRowDemo },
  { id: "awaken-tooltip", name: "AwakenTooltip", displayName: "工具提示", demo: AwakenTooltipDemo },
  { id: "awaken-tool-trace-group", name: "AwakenToolTraceGroup", displayName: "工具轨迹组", demo: AwakenToolTraceGroupDemo },
  { id: "awaken-tool-trace-line", name: "AwakenToolTraceLine", displayName: "工具轨迹行", demo: AwakenToolTraceLineDemo },
  { id: "awaken-top-bar", name: "AwakenTopBar", displayName: "顶栏", demo: AwakenTopBarDemo },
  { id: "awaken-user-prompt-block", name: "AwakenUserPromptBlock", displayName: "用户提示块", demo: AwakenUserPromptBlockDemo },
  { id: "awaken-window-title-bar", name: "AwakenWindowTitleBar", displayName: "窗口标题栏", demo: AwakenWindowTitleBarDemo },
  { id: "awaken-workbench-chrome", name: "AwakenWorkbenchChrome", displayName: "工作台框架", demo: AwakenWorkbenchChromeDemo },
  { id: "awaken-workbench-shell", name: "AwakenWorkbenchShell", displayName: "工作台外壳", demo: AwakenWorkbenchShellDemo },
  { id: "awaken-workbench-sidebar-frame", name: "AwakenWorkbenchSidebarFrame", displayName: "工作台侧栏框架", demo: AwakenWorkbenchSidebarFrameDemo },
];
