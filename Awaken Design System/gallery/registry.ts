import type { ComponentType } from "react";
import { AwakenAccountDemo } from "../src/components/AwakenAccount/AwakenAccount.demo";
import { AwakenDeliverableListDemo } from "../src/components/AwakenDeliverableList/AwakenDeliverableList.demo";
import { AwakenAssistantMessageDemo } from "../src/components/AwakenAssistantMessage/AwakenAssistantMessage.demo";
import { AwakenAvatarDemo } from "../src/components/AwakenAvatar/AwakenAvatar.demo";
import { AwakenAppearanceSettingsDemo } from "../src/components/AwakenAppearanceSettings/AwakenAppearanceSettings.demo";
import { AwakenComposerBarDemo } from "../src/components/AwakenComposerBar/AwakenComposerBar.demo";
import { AwakenFolderDemo } from "../src/components/AwakenFolder/AwakenFolder.demo";
import { AwakenConnectorsDemo } from "../src/components/AwakenConnectors/AwakenConnectors.demo";
import { AwakenMemorySettingsDemo } from "../src/components/AwakenMemorySettings/AwakenMemorySettings.demo";
import { AwakenModelSettingsDemo } from "../src/components/AwakenModelSettings/AwakenModelSettings.demo";
import { AwakenOriginalThinkingDemo } from "../src/components/AwakenOriginalThinking/AwakenOriginalThinking.demo";
import { AwakenProductLogoDemo } from "../src/components/AwakenProductLogo/AwakenProductLogo.demo";
import { AwakenRunStageDemo } from "../src/components/AwakenRunStage/AwakenRunStage.demo";
import { AwakenSettingsDemo } from "../src/components/AwakenSettings/AwakenSettings.demo";
import { AwakenSearchSettingsDemo } from "../src/components/AwakenSearchSettings/AwakenSearchSettings.demo";
import { AwakenPetsSettingsDemo } from "../src/components/AwakenPetsSettings/AwakenPetsSettings.demo";
import { AwakenSidebarDemo } from "../src/components/AwakenSidebar/AwakenSidebar.demo";
import { AwakenSkillsDemo } from "../src/components/AwakenSkills/AwakenSkills.demo";
import { AwakenSwitchDemo } from "../src/components/AwakenSwitch/AwakenSwitch.demo";
import { AwakenTaskWelcomeDemo } from "../src/components/AwakenTaskWelcome/AwakenTaskWelcome.demo";
import { AwakenDotmSquare3Demo } from "../src/components/AwakenDotmSquare3/AwakenDotmSquare3.demo";
import { AwakenThinkingOrbDemo } from "../src/components/AwakenThinkingOrb/AwakenThinkingOrb.demo";
import { AwakenThoughtTraceDemo } from "../src/components/AwakenThoughtTrace/AwakenThoughtTrace.demo";
import { AwakenToolTraceGroupDemo } from "../src/components/AwakenToolTraceGroup/AwakenToolTraceGroup.demo";
import { AwakenWorkbenchShellDemo } from "../src/components/AwakenWorkbenchShell/AwakenWorkbenchShell.demo";

export type AwakenGalleryEntry = {
  id: string;
  name: string;
  displayName: string;
  demo: ComponentType;
  category: AwakenGalleryCategory;
  aliases?: readonly string[];
};

export type AwakenGalleryCategory = "atomic" | "sidebar" | "settings" | "workbench" | "composition";

export const awakenGalleryEntries: AwakenGalleryEntry[] = [
  { id: "awaken-product-logo", name: "AwakenProductLogo", displayName: "产品 LOGO", demo: AwakenProductLogoDemo, category: "atomic" },
  { id: "awaken-switch", name: "AwakenSwitch", displayName: "开关", demo: AwakenSwitchDemo, category: "atomic", aliases: ["awaken-skill-toggle", "awaken-connector-toggle", "awaken-settings-switch"] },
  { id: "awaken-folder", name: "AwakenFolder", displayName: "文件夹", demo: AwakenFolderDemo, category: "atomic", aliases: ["awaken-project-skill-folder"] },
  { id: "awaken-sidebar", name: "AwakenSidebar", displayName: "「左侧栏」组件库", demo: AwakenSidebarDemo, category: "sidebar", aliases: ["awaken-dialog", "awaken-dropdown", "awaken-icon", "awaken-selectable-card"] },
  { id: "awaken-skills", name: "AwakenSkills", displayName: "「技能」组件库", demo: AwakenSkillsDemo, category: "sidebar" },
  { id: "awaken-account", name: "AwakenAccount", displayName: "「账户」组件库", demo: AwakenAccountDemo, category: "sidebar", aliases: ["awaken-popover"] },
  { id: "awaken-connectors", name: "AwakenConnectors", displayName: "「连接器」组件库", demo: AwakenConnectorsDemo, category: "sidebar" },
  { id: "awaken-settings", name: "AwakenSettings", displayName: "设置页框架", demo: AwakenSettingsDemo, category: "settings", aliases: ["awaken-button"] },
  { id: "awaken-model-settings", name: "AwakenModelSettings", displayName: "模型设置", demo: AwakenModelSettingsDemo, category: "settings" },
  { id: "awaken-search-settings", name: "AwakenSearchSettings", displayName: "搜索设置", demo: AwakenSearchSettingsDemo, category: "settings" },
  { id: "awaken-memory-settings", name: "AwakenMemorySettings", displayName: "记忆设置", demo: AwakenMemorySettingsDemo, category: "settings", aliases: ["awaken-memory-light-flow"] },
  { id: "awaken-appearance-settings", name: "AwakenAppearanceSettings", displayName: "外观设置", demo: AwakenAppearanceSettingsDemo, category: "settings", aliases: ["awaken-theme-choice-group"] },
  { id: "awaken-pets-settings", name: "AwakenPetsSettings", displayName: "宠物设置", demo: AwakenPetsSettingsDemo, category: "settings" },
  {
    id: "awaken-deliverables",
    name: "AwakenDeliverableList",
    displayName: "「最终交付物」组件库",
    demo: AwakenDeliverableListDemo,
    category: "workbench",
    aliases: ["awaken-deliverable-card", "awaken-deliverable-list"],
  },
  { id: "awaken-assistant-message", name: "AwakenAssistantMessage", displayName: "「助手消息」组件库", demo: AwakenAssistantMessageDemo, category: "workbench", aliases: ["awaken-copy-button"] },
  { id: "awaken-avatar", name: "AwakenAvatar", displayName: "头像", demo: AwakenAvatarDemo, category: "atomic" },
  { id: "awaken-composer-bar", name: "AwakenComposerBar", displayName: "「输入栏」组件库", demo: AwakenComposerBarDemo, category: "workbench", aliases: ["awaken-composer-input", "awaken-context-button", "awaken-user-prompt-block", "awaken-composer-dock", "awaken-icon-button", "awaken-send-button", "awaken-tooltip"] },
  { id: "awaken-original-thinking", name: "AwakenOriginalThinking", displayName: "原始思考动效", demo: AwakenOriginalThinkingDemo, category: "atomic" },
  { id: "awaken-thinking-orb", name: "AwakenThinkingOrb", displayName: "思考球", demo: AwakenThinkingOrbDemo, category: "atomic" },
  { id: "awaken-dotm-square3", name: "AwakenDotmSquare3", displayName: "矩阵方块", demo: AwakenDotmSquare3Demo, category: "atomic" },
  { id: "awaken-run-stage", name: "AwakenRunStage", displayName: "「运行阶段」组件库", demo: AwakenRunStageDemo, category: "workbench", aliases: ["awaken-execution-feed"] },
  { id: "awaken-task-welcome", name: "AwakenTaskWelcome", displayName: "「任务欢迎区」组件库", demo: AwakenTaskWelcomeDemo, category: "workbench" },
  { id: "awaken-thought-trace", name: "AwakenThoughtTrace", displayName: "「思路轨迹」组件库", demo: AwakenThoughtTraceDemo, category: "workbench" },
  {
    id: "awaken-tool-trace",
    name: "AwakenToolTraceGroup",
    displayName: "「工具轨迹」组件库",
    demo: AwakenToolTraceGroupDemo,
    category: "workbench",
    aliases: ["awaken-disclosure", "awaken-tool-trace-group", "awaken-tool-trace-line"],
  },
  {
    id: "awaken-workbench-shell",
    name: "AwakenWorkbenchShell",
    displayName: "「工作台框架」组件库",
    demo: AwakenWorkbenchShellDemo,
    category: "composition",
    aliases: [
      "awaken-app-shell",
      "awaken-inspector-rail",
      "awaken-inspector-tabs",
      "awaken-light-inspector-rail",
      "awaken-platform-provider",
      "awaken-top-bar",
      "awaken-window-title-bar",
      "awaken-workbench-chrome",
    ],
  },
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
    id: "settings",
    label: "设置页组件",
    entries: awakenGalleryEntries.filter((entry) => entry.category === "settings"),
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
