import type { ComponentType } from "react";
import { AwakenAccountDemo } from "../src/components/AwakenAccount/AwakenAccount.demo";
import { AwakenAppShellDemo } from "../src/components/AwakenAppShell/AwakenAppShell.demo";
import { AwakenDeliverableListDemo } from "../src/components/AwakenDeliverableList/AwakenDeliverableList.demo";
import { AwakenAssistantMessageDemo } from "../src/components/AwakenAssistantMessage/AwakenAssistantMessage.demo";
import { AwakenAvatarDemo } from "../src/components/AwakenAvatar/AwakenAvatar.demo";
import { AwakenButtonDemo } from "../src/components/AwakenButton/AwakenButton.demo";
import { AwakenComposerBarDemo } from "../src/components/AwakenComposerBar/AwakenComposerBar.demo";
import { AwakenComposerInputDemo } from "../src/components/AwakenComposerInput/AwakenComposerInput.demo";
import { AwakenContextButtonDemo } from "../src/components/AwakenContextButton/AwakenContextButton.demo";
import { AwakenConnectorsDemo } from "../src/components/AwakenConnectors/AwakenConnectors.demo";
import { AwakenCopyButtonDemo } from "../src/components/AwakenCopyButton/AwakenCopyButton.demo";
import { AwakenDisclosureDemo } from "../src/components/AwakenDisclosure/AwakenDisclosure.demo";
import { AwakenDividerDemo } from "../src/components/AwakenDivider/AwakenDivider.demo";
import { AwakenDialogDemo } from "../src/components/AwakenDialog/AwakenDialog.demo";
import { AwakenDropdownDemo } from "../src/components/AwakenDropdown/AwakenDropdown.demo";
import { AwakenIconDemo } from "../src/components/AwakenIcon/AwakenIcon.demo";
import { AwakenIconButtonDemo } from "../src/components/AwakenIconButton/AwakenIconButton.demo";
import { AwakenInspectorRailDemo } from "../src/components/AwakenInspectorRail/AwakenInspectorRail.demo";
import { AwakenInspectorTabsDemo } from "../src/components/AwakenInspectorTabs/AwakenInspectorTabs.demo";
import { AwakenKbdDemo } from "../src/components/AwakenKbd/AwakenKbd.demo";
import { AwakenLightInspectorRailDemo } from "../src/components/AwakenLightInspectorRail/AwakenLightInspectorRail.demo";
import { AwakenModelSelectorDemo } from "../src/components/AwakenModelSelector/AwakenModelSelector.demo";
import { AwakenModeToggleDemo } from "../src/components/AwakenModeToggle/AwakenModeToggle.demo";
import { AwakenOriginalThinkingDemo } from "../src/components/AwakenOriginalThinking/AwakenOriginalThinking.demo";
import { AwakenPillDemo } from "../src/components/AwakenPill/AwakenPill.demo";
import { AwakenPlatformProviderDemo } from "../src/components/AwakenPlatformProvider/AwakenPlatformProvider.demo";
import { AwakenPopoverDemo } from "../src/components/AwakenPopover/AwakenPopover.demo";
import { AwakenProductLogoDemo } from "../src/components/AwakenProductLogo/AwakenProductLogo.demo";
import { AwakenQuickActionChipDemo } from "../src/components/AwakenQuickActionChip/AwakenQuickActionChip.demo";
import { AwakenRunStageDemo } from "../src/components/AwakenRunStage/AwakenRunStage.demo";
import { AwakenSendButtonDemo } from "../src/components/AwakenSendButton/AwakenSendButton.demo";
import { AwakenSelectableCardDemo } from "../src/components/AwakenSelectableCard/AwakenSelectableCard.demo";
import { AwakenSettingsDemo } from "../src/components/AwakenSettings/AwakenSettings.demo";
import { AwakenSidebarDemo } from "../src/components/AwakenSidebar/AwakenSidebar.demo";
import { AwakenSkillsDemo } from "../src/components/AwakenSkills/AwakenSkills.demo";
import { AwakenStatusPillDemo } from "../src/components/AwakenStatusPill/AwakenStatusPill.demo";
import { AwakenTaskWelcomeDemo } from "../src/components/AwakenTaskWelcome/AwakenTaskWelcome.demo";
import { AwakenThoughtTraceDemo } from "../src/components/AwakenThoughtTrace/AwakenThoughtTrace.demo";
import { AwakenToastDemo } from "../src/components/AwakenToast/AwakenToast.demo";
import { AwakenTooltipDemo } from "../src/components/AwakenTooltip/AwakenTooltip.demo";
import { AwakenToolTraceGroupDemo } from "../src/components/AwakenToolTraceGroup/AwakenToolTraceGroup.demo";
import { AwakenTopBarDemo } from "../src/components/AwakenTopBar/AwakenTopBar.demo";
import { AwakenWindowTitleBarDemo } from "../src/components/AwakenWindowTitleBar/AwakenWindowTitleBar.demo";
import { AwakenWorkbenchChromeDemo } from "../src/components/AwakenWorkbenchChrome/AwakenWorkbenchChrome.demo";
import { AwakenWorkbenchShellDemo } from "../src/components/AwakenWorkbenchShell/AwakenWorkbenchShell.demo";

export type AwakenGalleryEntry = {
  id: string;
  name: string;
  displayName: string;
  demo: ComponentType;
  category: AwakenGalleryCategory;
  aliases?: readonly string[];
};

export type AwakenGalleryCategory = "atomic" | "sidebar" | "workbench" | "composition";

export const awakenGalleryEntries: AwakenGalleryEntry[] = [
  { id: "awaken-product-logo", name: "AwakenProductLogo", displayName: "产品 LOGO", demo: AwakenProductLogoDemo, category: "atomic" },
  { id: "awaken-sidebar", name: "AwakenSidebar", displayName: "「左侧栏」组件库", demo: AwakenSidebarDemo, category: "sidebar" },
  { id: "awaken-skills", name: "AwakenSkills", displayName: "「技能」组件库", demo: AwakenSkillsDemo, category: "sidebar" },
  { id: "awaken-settings", name: "AwakenSettings", displayName: "「设置」组件库", demo: AwakenSettingsDemo, category: "sidebar" },
  { id: "awaken-account", name: "AwakenAccount", displayName: "「账户」组件库", demo: AwakenAccountDemo, category: "sidebar" },
  { id: "awaken-connectors", name: "AwakenConnectors", displayName: "「连接器」组件库", demo: AwakenConnectorsDemo, category: "sidebar" },
  { id: "awaken-app-shell", name: "AwakenAppShell", displayName: "应用外壳", demo: AwakenAppShellDemo, category: "composition" },
  {
    id: "awaken-deliverables",
    name: "AwakenDeliverableList",
    displayName: "「最终交付物」组件库",
    demo: AwakenDeliverableListDemo,
    category: "workbench",
    aliases: ["awaken-deliverable-card", "awaken-deliverable-list"],
  },
  { id: "awaken-assistant-message", name: "AwakenAssistantMessage", displayName: "「助手消息」组件库", demo: AwakenAssistantMessageDemo, category: "workbench" },
  { id: "awaken-avatar", name: "AwakenAvatar", displayName: "头像", demo: AwakenAvatarDemo, category: "atomic" },
  { id: "awaken-button", name: "AwakenButton", displayName: "按钮", demo: AwakenButtonDemo, category: "atomic" },
  { id: "awaken-composer-bar", name: "AwakenComposerBar", displayName: "「输入栏」组件库", demo: AwakenComposerBarDemo, category: "workbench", aliases: ["awaken-user-prompt-block", "awaken-composer-dock"] },
  { id: "awaken-composer-input", name: "AwakenComposerInput", displayName: "输入框", demo: AwakenComposerInputDemo, category: "atomic" },
  { id: "awaken-context-button", name: "AwakenContextButton", displayName: "上下文按钮", demo: AwakenContextButtonDemo, category: "atomic" },
  { id: "awaken-copy-button", name: "AwakenCopyButton", displayName: "复制按钮", demo: AwakenCopyButtonDemo, category: "atomic" },
  { id: "awaken-disclosure", name: "AwakenDisclosure", displayName: "展开折叠", demo: AwakenDisclosureDemo, category: "atomic" },
  { id: "awaken-divider", name: "AwakenDivider", displayName: "分隔线", demo: AwakenDividerDemo, category: "atomic" },
  { id: "awaken-dialog", name: "AwakenDialog", displayName: "弹窗", demo: AwakenDialogDemo, category: "atomic" },
  { id: "awaken-dropdown", name: "AwakenDropdown", displayName: "下拉菜单", demo: AwakenDropdownDemo, category: "atomic" },
  { id: "awaken-icon", name: "AwakenIcon", displayName: "图标", demo: AwakenIconDemo, category: "atomic" },
  { id: "awaken-icon-button", name: "AwakenIconButton", displayName: "图标按钮", demo: AwakenIconButtonDemo, category: "atomic" },
  { id: "awaken-inspector-rail", name: "AwakenInspectorRail", displayName: "检查器侧轨", demo: AwakenInspectorRailDemo, category: "composition" },
  { id: "awaken-inspector-tabs", name: "AwakenInspectorTabs", displayName: "检查器标签", demo: AwakenInspectorTabsDemo, category: "composition" },
  { id: "awaken-kbd", name: "AwakenKbd", displayName: "快捷键标记", demo: AwakenKbdDemo, category: "atomic" },
  { id: "awaken-light-inspector-rail", name: "AwakenLightInspectorRail", displayName: "浅色检查器侧轨", demo: AwakenLightInspectorRailDemo, category: "composition" },
  { id: "awaken-model-selector", name: "AwakenModelSelector", displayName: "模型选择器", demo: AwakenModelSelectorDemo, category: "atomic" },
  { id: "awaken-mode-toggle", name: "AwakenModeToggle", displayName: "模式切换", demo: AwakenModeToggleDemo, category: "atomic" },
  { id: "awaken-original-thinking", name: "AwakenOriginalThinking", displayName: "原始思考动效", demo: AwakenOriginalThinkingDemo, category: "atomic" },
  { id: "awaken-pill", name: "AwakenPill", displayName: "胶囊标签", demo: AwakenPillDemo, category: "atomic" },
  { id: "awaken-platform-provider", name: "AwakenPlatformProvider", displayName: "平台提供器", demo: AwakenPlatformProviderDemo, category: "composition" },
  { id: "awaken-popover", name: "AwakenPopover", displayName: "弹出层", demo: AwakenPopoverDemo, category: "atomic" },
  { id: "awaken-quick-action-chip", name: "AwakenQuickActionChip", displayName: "快捷动作标签", demo: AwakenQuickActionChipDemo, category: "atomic" },
  { id: "awaken-run-stage", name: "AwakenRunStage", displayName: "「运行阶段」组件库", demo: AwakenRunStageDemo, category: "workbench", aliases: ["awaken-execution-feed"] },
  { id: "awaken-send-button", name: "AwakenSendButton", displayName: "发送按钮", demo: AwakenSendButtonDemo, category: "atomic" },
  { id: "awaken-selectable-card", name: "AwakenSelectableCard", displayName: "选择卡片", demo: AwakenSelectableCardDemo, category: "atomic" },
  { id: "awaken-status-pill", name: "AwakenStatusPill", displayName: "状态胶囊", demo: AwakenStatusPillDemo, category: "atomic" },
  { id: "awaken-task-welcome", name: "AwakenTaskWelcome", displayName: "「任务欢迎区」组件库", demo: AwakenTaskWelcomeDemo, category: "workbench" },
  { id: "awaken-thought-trace", name: "AwakenThoughtTrace", displayName: "「思路轨迹」组件库", demo: AwakenThoughtTraceDemo, category: "workbench" },
  { id: "awaken-toast", name: "AwakenToast", displayName: "提示通知", demo: AwakenToastDemo, category: "atomic" },
  { id: "awaken-tooltip", name: "AwakenTooltip", displayName: "工具提示", demo: AwakenTooltipDemo, category: "atomic" },
  {
    id: "awaken-tool-trace",
    name: "AwakenToolTraceGroup",
    displayName: "「工具轨迹」组件库",
    demo: AwakenToolTraceGroupDemo,
    category: "workbench",
    aliases: ["awaken-tool-trace-group", "awaken-tool-trace-line"],
  },
  { id: "awaken-top-bar", name: "AwakenTopBar", displayName: "顶栏", demo: AwakenTopBarDemo, category: "composition" },
  { id: "awaken-window-title-bar", name: "AwakenWindowTitleBar", displayName: "窗口标题栏", demo: AwakenWindowTitleBarDemo, category: "composition" },
  { id: "awaken-workbench-chrome", name: "AwakenWorkbenchChrome", displayName: "工作台框架", demo: AwakenWorkbenchChromeDemo, category: "composition" },
  { id: "awaken-workbench-shell", name: "AwakenWorkbenchShell", displayName: "工作台外壳", demo: AwakenWorkbenchShellDemo, category: "composition" },
];

export const awakenGallerySections = [
  {
    id: "atomic",
    label: "原子组件",
    entries: awakenGalleryEntries.filter((entry) => entry.category === "atomic"),
  },
  {
    id: "sidebar",
    label: "左侧栏组件",
    entries: awakenGalleryEntries.filter((entry) => entry.category === "sidebar"),
  },
  {
    id: "workbench",
    label: "中间交互区组件",
    entries: awakenGalleryEntries.filter((entry) => entry.category === "workbench"),
  },
  {
    id: "composition",
    label: "布局 / 组合组件",
    entries: awakenGalleryEntries.filter((entry) => entry.category === "composition"),
  },
] as const;
