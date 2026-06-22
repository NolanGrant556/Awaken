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
import { AwakenSidebarDemo } from "../src/components/AwakenSidebar";
import { AwakenStatusPillDemo } from "../src/components/AwakenStatusPill";
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
  demo: ComponentType;
};

export const awakenGalleryEntries: AwakenGalleryEntry[] = [
  { id: "awaken-activity-line", name: "AwakenActivityLine", demo: AwakenActivityLineDemo },
  { id: "awaken-approval-card", name: "AwakenApprovalCard", demo: AwakenApprovalCardDemo },
  { id: "awaken-app-shell", name: "AwakenAppShell", demo: AwakenAppShellDemo },
  { id: "awaken-artifact-card", name: "AwakenArtifactCard", demo: AwakenArtifactCardDemo },
  { id: "awaken-artifact-grid", name: "AwakenArtifactGrid", demo: AwakenArtifactGridDemo },
  { id: "awaken-assistant-message", name: "AwakenAssistantMessage", demo: AwakenAssistantMessageDemo },
  { id: "awaken-avatar", name: "AwakenAvatar", demo: AwakenAvatarDemo },
  { id: "awaken-brand-row", name: "AwakenBrandRow", demo: AwakenBrandRowDemo },
  { id: "awaken-button", name: "AwakenButton", demo: AwakenButtonDemo },
  { id: "awaken-center-stage", name: "AwakenCenterStage", demo: AwakenCenterStageDemo },
  { id: "awaken-composer", name: "AwakenComposer", demo: AwakenComposerDemo },
  { id: "awaken-composer-bar", name: "AwakenComposerBar", demo: AwakenComposerBarDemo },
  { id: "awaken-composer-dock", name: "AwakenComposerDock", demo: AwakenComposerDockDemo },
  { id: "awaken-composer-input", name: "AwakenComposerInput", demo: AwakenComposerInputDemo },
  { id: "awaken-context-button", name: "AwakenContextButton", demo: AwakenContextButtonDemo },
  { id: "awaken-conversation-list", name: "AwakenConversationList", demo: AwakenConversationListDemo },
  { id: "awaken-copy-button", name: "AwakenCopyButton", demo: AwakenCopyButtonDemo },
  { id: "awaken-disclosure", name: "AwakenDisclosure", demo: AwakenDisclosureDemo },
  { id: "awaken-divider", name: "AwakenDivider", demo: AwakenDividerDemo },
  { id: "awaken-dropdown", name: "AwakenDropdown", demo: AwakenDropdownDemo },
  { id: "awaken-execution-feed", name: "AwakenExecutionFeed", demo: AwakenExecutionFeedDemo },
  { id: "awaken-exploring-group", name: "AwakenExploringGroup", demo: AwakenExploringGroupDemo },
  { id: "awaken-file-change-summary", name: "AwakenFileChangeSummary", demo: AwakenFileChangeSummaryDemo },
  { id: "awaken-icon", name: "AwakenIcon", demo: AwakenIconDemo },
  { id: "awaken-icon-button", name: "AwakenIconButton", demo: AwakenIconButtonDemo },
  { id: "awaken-inspector-rail", name: "AwakenInspectorRail", demo: AwakenInspectorRailDemo },
  { id: "awaken-inspector-tabs", name: "AwakenInspectorTabs", demo: AwakenInspectorTabsDemo },
  { id: "awaken-kbd", name: "AwakenKbd", demo: AwakenKbdDemo },
  { id: "awaken-light-inspector-rail", name: "AwakenLightInspectorRail", demo: AwakenLightInspectorRailDemo },
  { id: "awaken-markdown-answer", name: "AwakenMarkdownAnswer", demo: AwakenMarkdownAnswerDemo },
  { id: "awaken-model-selector", name: "AwakenModelSelector", demo: AwakenModelSelectorDemo },
  { id: "awaken-mode-toggle", name: "AwakenModeToggle", demo: AwakenModeToggleDemo },
  { id: "awaken-narrative-feed", name: "AwakenNarrativeFeed", demo: AwakenNarrativeFeedDemo },
  { id: "awaken-new-task-button", name: "AwakenNewTaskButton", demo: AwakenNewTaskButtonDemo },
  { id: "awaken-pill", name: "AwakenPill", demo: AwakenPillDemo },
  { id: "awaken-platform-provider", name: "AwakenPlatformProvider", demo: AwakenPlatformProviderDemo },
  { id: "awaken-popover", name: "AwakenPopover", demo: AwakenPopoverDemo },
  { id: "awaken-primary-nav", name: "AwakenPrimaryNav", demo: AwakenPrimaryNavDemo },
  { id: "awaken-profile-button", name: "AwakenProfileButton", demo: AwakenProfileButtonDemo },
  { id: "awaken-project-tree", name: "AwakenProjectTree", demo: AwakenProjectTreeDemo },
  { id: "awaken-prompt-card", name: "AwakenPromptCard", demo: AwakenPromptCardDemo },
  { id: "awaken-quick-action-chip", name: "AwakenQuickActionChip", demo: AwakenQuickActionChipDemo },
  { id: "awaken-quick-action-grid", name: "AwakenQuickActionGrid", demo: AwakenQuickActionGridDemo },
  { id: "awaken-run-stage", name: "AwakenRunStage", demo: AwakenRunStageDemo },
  { id: "awaken-run-step", name: "AwakenRunStep", demo: AwakenRunStepDemo },
  { id: "awaken-run-timeline", name: "AwakenRunTimeline", demo: AwakenRunTimelineDemo },
  { id: "awaken-send-button", name: "AwakenSendButton", demo: AwakenSendButtonDemo },
  { id: "awaken-sidebar", name: "AwakenSidebar", demo: AwakenSidebarDemo },
  { id: "awaken-status-pill", name: "AwakenStatusPill", demo: AwakenStatusPillDemo },
  { id: "awaken-thinking-block", name: "AwakenThinkingBlock", demo: AwakenThinkingBlockDemo },
  { id: "awaken-thought-trace", name: "AwakenThoughtTrace", demo: AwakenThoughtTraceDemo },
  { id: "awaken-toast", name: "AwakenToast", demo: AwakenToastDemo },
  { id: "awaken-tool-action-row", name: "AwakenToolActionRow", demo: AwakenToolActionRowDemo },
  { id: "awaken-tooltip", name: "AwakenTooltip", demo: AwakenTooltipDemo },
  { id: "awaken-tool-trace-group", name: "AwakenToolTraceGroup", demo: AwakenToolTraceGroupDemo },
  { id: "awaken-tool-trace-line", name: "AwakenToolTraceLine", demo: AwakenToolTraceLineDemo },
  { id: "awaken-top-bar", name: "AwakenTopBar", demo: AwakenTopBarDemo },
  { id: "awaken-user-prompt-block", name: "AwakenUserPromptBlock", demo: AwakenUserPromptBlockDemo },
  { id: "awaken-window-title-bar", name: "AwakenWindowTitleBar", demo: AwakenWindowTitleBarDemo },
  { id: "awaken-workbench-chrome", name: "AwakenWorkbenchChrome", demo: AwakenWorkbenchChromeDemo },
  { id: "awaken-workbench-shell", name: "AwakenWorkbenchShell", demo: AwakenWorkbenchShellDemo },
  { id: "awaken-workbench-sidebar-frame", name: "AwakenWorkbenchSidebarFrame", demo: AwakenWorkbenchSidebarFrameDemo },
];
