import { readFile } from "node:fs/promises";
import React, { type ComponentType } from "react";
import { renderToString } from "react-dom/server";
import type { AwakenAssistantMessageProps } from "../src/index";

type AwakenDesignSystemModule = typeof import("../src/index");
type AwakenGalleryDemosModule = typeof import("../gallery/demos");

function requireAssistantMessageProps(
  stateId: string,
  props: Partial<AwakenAssistantMessageProps>,
): AwakenAssistantMessageProps {
  if (typeof props.markdown !== "string") {
    throw new Error(`AwakenAssistantMessage state ${stateId} is missing markdown`);
  }
  return { ...props, markdown: props.markdown };
}

const AwakenDesignSystem = (await import(
  new URL("../dist/index.js", import.meta.url).href
)) as AwakenDesignSystemModule;
const AwakenGalleryDemos = (await import(
  new URL("../dist/gallery.js", import.meta.url).href
)) as AwakenGalleryDemosModule;

const productionDemoExports = Object.keys(AwakenDesignSystem).filter((name) => name.endsWith("Demo"));

if (productionDemoExports.length > 0) {
  throw new Error(`Production bundle must not export Gallery demos: ${productionDemoExports.join(", ")}`);
}

const demoOnlySelectors = [
  ".aw-account-demo",
  ".aw-connectors-demo",
  ".aw-project-skill-folder-demo",
  ".aw-settings-demo",
  ".aw-sidebar-demo",
  ".aw-skill-card-demo",
  ".aw-skills-demo",
];
const [productionCss, galleryCss] = await Promise.all([
  readFile(new URL("../dist/index.css", import.meta.url), "utf8"),
  readFile(new URL("../dist/gallery.css", import.meta.url), "utf8"),
]);

for (const selector of demoOnlySelectors) {
  if (productionCss.includes(selector)) {
    throw new Error(`Production CSS includes Gallery-only selector: ${selector}`);
  }
  if (!galleryCss.includes(selector)) {
    throw new Error(`Gallery CSS is missing expected demo selector: ${selector}`);
  }
}

if (!productionCss.includes(".aw-memory-light-flow-layout")) {
  throw new Error("Production CSS is missing the shared AwakenMemoryLightFlow layout");
}

if (!productionCss.includes(".aw-model-orbit")) {
  throw new Error("Production CSS is missing the shared model orbit header");
}

const modelOrbitKeyframes = [
  "aw-model-orbit-drift-cw",
  "aw-model-orbit-drift-ccw",
  "aw-model-orbit-counter-cw",
  "aw-model-orbit-counter-ccw",
];

if (
  modelOrbitKeyframes.some((keyframe) => !productionCss.includes(keyframe)) ||
  (productionCss.match(/360deg/g)?.length ?? 0) < modelOrbitKeyframes.length
) {
  throw new Error("Production CSS must preserve the complete 360-degree model orbit animations");
}

if (productionCss.includes("记忆控制")) {
  throw new Error("AwakenMemoryLightFlow layout must not inject a secondary memory-control heading");
}

if (productionCss.includes(".aw-memory-light-flow-demo__content")) {
  throw new Error("Production CSS must not include the retired Gallery-only memory layout");
}

const galleryExports: Record<string, unknown> = AwakenGalleryDemos;
const demoEntries = Object.entries(galleryExports)
  .filter(
    (entry): entry is [string, ComponentType] =>
      entry[0].startsWith("Awaken") && entry[0].endsWith("Demo") && typeof entry[1] === "function",
  )
  .sort(([a], [b]) => a.localeCompare(b));

if (demoEntries.length === 0) {
  throw new Error("No Awaken component demos were exported from the Gallery-only bundle");
}

for (const [name, Demo] of demoEntries) {
  const componentName = name.replace(/Demo$/, "");
  let html = "";
  try {
    html = renderToString(React.createElement(Demo));
  } catch (error) {
    throw new Error(`Gallery smoke failed for ${componentName}: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (!html.includes(componentName)) {
    throw new Error(`Gallery smoke missing component name for ${componentName}`);
  }
}

const expectedThinkingOrbStateIds = [
  "working",
  "searching",
  "solving",
  "listening",
  "composing",
  "shaping",
];
const thinkingOrbStateIds = AwakenDesignSystem.AwakenThinkingOrbStates.map((state) => state.id);

if (thinkingOrbStateIds.join(",") !== expectedThinkingOrbStateIds.join(",")) {
  throw new Error(`AwakenThinkingOrb states are incomplete or out of order: ${thinkingOrbStateIds.join(", ")}`);
}

const composingOrbState = AwakenDesignSystem.AwakenThinkingOrbStates.find(
  (state) => state.id === "composing",
);

if (composingOrbState?.props.size !== 64 || composingOrbState.props.speed !== 2.4) {
  throw new Error("AwakenThinkingOrb composing state must preserve the selected 64px / 2.4x configuration");
}

const thinkingOrbHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenThinkingOrb, {
    state: "composing",
    size: 64,
    speed: 2.4,
    "aria-label": "正在组织回答",
  }),
);

if (!thinkingOrbHtml.includes("<canvas") || !thinkingOrbHtml.includes('role="img"')) {
  throw new Error("AwakenThinkingOrb must render an accessible canvas image");
}

if (!thinkingOrbHtml.includes('aria-label="正在组织回答"')) {
  throw new Error("AwakenThinkingOrb must forward the consumer accessibility label");
}

const thinkingOrbStatusHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenThinkingOrbStatus),
);

if (!thinkingOrbStatusHtml.includes('role="status"') || !thinkingOrbStatusHtml.includes('aria-label="Thinking..."')) {
  throw new Error("AwakenThinkingOrbStatus must expose the Thinking line as an accessible status");
}

if (!thinkingOrbStatusHtml.includes('aria-hidden="true"')) {
  throw new Error("AwakenThinkingOrbStatus must hide its decorative canvas from assistive technology");
}

if (!productionCss.includes(".aw-thinking-orb-status")) {
  throw new Error("Production CSS is missing the shared AwakenThinkingOrbStatus surface");
}

if (
  productionCss.includes("--aw-thinking-orb-status-fill") ||
  productionCss.includes("--aw-thinking-orb-status-ring") ||
  productionCss.includes("width:270px")
) {
  throw new Error("AwakenThinkingOrbStatus must remain a transparent inline state without capsule styling");
}

const activeThoughtTraceHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenThoughtTrace, {
    text: "Inspecting the current implementation",
    status: "active",
    theme: "dark",
  }),
);

if (
  !activeThoughtTraceHtml.includes('aria-label="Thinking..."') ||
  !activeThoughtTraceHtml.includes("aw-thinking-orb-status") ||
  !activeThoughtTraceHtml.includes('data-size="20"') ||
  activeThoughtTraceHtml.includes('data-paused="true"')
) {
  throw new Error("Active AwakenThoughtTrace must use the compact animated Thinking line");
}

const completedThoughtTraceHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenThoughtTrace, {
    text: "Inspection complete",
    status: "done",
    theme: "light",
  }),
);

if (
  !completedThoughtTraceHtml.includes("Thought") ||
  !completedThoughtTraceHtml.includes("thought-trace-disclosure") ||
  !completedThoughtTraceHtml.includes('aria-hidden="true"') ||
  completedThoughtTraceHtml.includes("aw-thinking-orb-status") ||
  completedThoughtTraceHtml.includes("<canvas")
) {
  throw new Error("Completed AwakenThoughtTrace must replace the orb with a disclosure chevron");
}

const richMarkdownState = AwakenDesignSystem.AwakenAssistantMessageStates.find(
  (state) => state.id === "rich-markdown",
);
const articleMarkdownState = AwakenDesignSystem.AwakenAssistantMessageStates.find(
  (state) => state.id === "article-markdown",
);
const narrowChineseTableState = AwakenDesignSystem.AwakenAssistantMessageStates.find(
  (state) => state.id === "narrow-chinese-table",
);
const footerState = AwakenDesignSystem.AwakenAssistantMessageStates.find(
  (state) => state.id === "completed-answer-footer",
);
const completedStatesWithoutElapsed = AwakenDesignSystem.AwakenAssistantMessageStates.filter(
  (state) => state.props.status === "done" && typeof state.props.elapsedMs !== "number",
);

if (!richMarkdownState) {
  throw new Error("AwakenAssistantMessage rich-markdown state is missing");
}

if (!articleMarkdownState) {
  throw new Error("AwakenAssistantMessage article-markdown state is missing");
}

if (!narrowChineseTableState) {
  throw new Error("AwakenAssistantMessage narrow-chinese-table state is missing");
}

if (!footerState) {
  throw new Error("AwakenAssistantMessage completed-answer-footer state is missing");
}

if (completedStatesWithoutElapsed.length > 0) {
  throw new Error(
    `AwakenAssistantMessage completed states must show elapsed timing: ${completedStatesWithoutElapsed
      .map((state) => state.id)
      .join(", ")}`,
  );
}

const richMarkdownProps = requireAssistantMessageProps(richMarkdownState.id, richMarkdownState.props);
const articleMarkdownProps = requireAssistantMessageProps(
  articleMarkdownState.id,
  articleMarkdownState.props,
);
const narrowChineseTableProps = requireAssistantMessageProps(
  narrowChineseTableState.id,
  narrowChineseTableState.props,
);
const footerProps = requireAssistantMessageProps(footerState.id, footerState.props);

const richMarkdownHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, richMarkdownProps),
);

if (richMarkdownHtml.includes("**粗体**") || !richMarkdownHtml.includes("<strong>粗体</strong>")) {
  throw new Error("AwakenAssistantMessage did not render strong Markdown");
}

if (!richMarkdownHtml.includes("assistant-message-table-wrap")) {
  throw new Error("AwakenAssistantMessage did not render the GFM table wrapper");
}

if (!richMarkdownHtml.includes("assistant-mermaid-diagram")) {
  throw new Error("AwakenAssistantMessage did not route closed Mermaid blocks to the diagram component");
}

const articleMarkdownHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, articleMarkdownProps),
);

if (
  !articleMarkdownHtml.includes("assistant-message-code-frame-labeled") ||
  !articleMarkdownHtml.includes("assistant-message-code-language\">javascript")
) {
  throw new Error("AwakenAssistantMessage did not render the fenced-code language label");
}

if (
  !articleMarkdownHtml.includes("hljs-keyword") ||
  !articleMarkdownHtml.includes("hljs-title")
) {
  throw new Error("AwakenAssistantMessage did not render syntax-highlight token spans");
}

if (!articleMarkdownHtml.includes("assistant-message-table-wrap")) {
  throw new Error("AwakenAssistantMessage article state did not render the GFM table wrapper");
}

const narrowChineseTableHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, narrowChineseTableProps),
);

if (
  !narrowChineseTableHtml.includes("企业经营风险分析报告.docx") ||
  !narrowChineseTableHtml.includes("QA 状态") ||
  (narrowChineseTableHtml.match(/<th>/g)?.length ?? 0) !== 3
) {
  throw new Error("AwakenAssistantMessage narrow Chinese table fixture is incomplete");
}

const productionTableRule = productionCss.match(/\.assistant-message-table\{([^}]*)\}/)?.[1] ?? "";
const productionTableCellRule =
  productionCss.match(
    /\.assistant-message-table th,.assistant-message-table td\{([^}]*)\}/,
  )?.[1] ?? "";

if (
  !productionTableRule.includes("min-width:560px") ||
  !productionTableCellRule.includes("min-width:8rem") ||
  !productionTableCellRule.includes("overflow-wrap:normal") ||
  !productionTableCellRule.includes("word-break:normal")
) {
  throw new Error("AwakenAssistantMessage tables must preserve readable cells and horizontal overflow");
}

const footerHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, footerProps),
);

if (!footerHtml.includes("assistant-message-footer")) {
  throw new Error("AwakenAssistantMessage did not render the completed answer footer");
}

if (!footerHtml.includes("assistant-message-has-footer")) {
  throw new Error("AwakenAssistantMessage did not expose the completed footer spacing class");
}

if ((!footerHtml.includes("Agent ran") && !footerHtml.includes("执行时间")) || !footerHtml.includes("1m 05s")) {
  throw new Error("AwakenAssistantMessage did not render the elapsed runtime label");
}

if (!footerHtml.includes("Copy final answer")) {
  throw new Error("AwakenAssistantMessage did not render the final answer copy action");
}

if (!footerHtml.includes('<span class="assistant-message-footer-copy"><button')) {
  throw new Error("AwakenAssistantMessage should place the silent copy button directly in the footer layout");
}

if (!footerHtml.includes('aria-label="Copy final answer"')) {
  throw new Error("AwakenAssistantMessage should keep an accessible name for the silent copy button");
}

if (
  footerHtml.includes('title="Copy final answer"') ||
  footerHtml.includes(">Copy final answer</span>")
) {
  throw new Error("AwakenAssistantMessage should not render visible copy text or a native title");
}

const copyOnlyFooterHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, {
    markdown: "done without elapsed timing",
    status: "done",
  }),
);

if (!copyOnlyFooterHtml.includes("assistant-message-footer")) {
  throw new Error("AwakenAssistantMessage should keep the completed answer footer without elapsed timing");
}

if (!copyOnlyFooterHtml.includes("Copy final answer")) {
  throw new Error("AwakenAssistantMessage should keep the final answer copy action without elapsed timing");
}

if (!copyOnlyFooterHtml.includes('<span class="assistant-message-footer-copy"><button')) {
  throw new Error("AwakenAssistantMessage should keep the copy-only footer action right-aligned");
}

if (copyOnlyFooterHtml.includes("Agent ran")) {
  throw new Error("AwakenAssistantMessage should not show an elapsed runtime label without elapsed timing");
}

const activeFooterHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, {
    markdown: "still running",
    status: "active",
    elapsedMs: 12000,
  }),
);

if (activeFooterHtml.includes("assistant-message-footer")) {
  throw new Error("AwakenAssistantMessage should not render the footer while active");
}

const errorFooterHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, {
    markdown: "failed",
    status: "error",
    elapsedMs: 12000,
  }),
);

if (errorFooterHtml.includes("assistant-message-footer")) {
  throw new Error("AwakenAssistantMessage should not render the footer for error answers");
}

const streamableMcpDialogHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenMcpConfigDialog, {
    open: true,
    mode: "streamable-http",
    value: '{"mcpServers":{"remote":{"type":"streamable-http","url":"https://mcp.example.test/mcp"}}}',
    onOpenChange: () => undefined,
    onModeChange: () => undefined,
    onValueChange: () => undefined,
    onSubmit: () => undefined,
  }),
);

if (
  !streamableMcpDialogHtml.includes("本地进程（stdio）") ||
  !streamableMcpDialogHtml.includes("Streamable HTTP") ||
  !streamableMcpDialogHtml.includes('aria-selected="true"') ||
  !streamableMcpDialogHtml.includes("兼容")
) {
  throw new Error("AwakenMcpConfigDialog should expose distinct stdio and Streamable HTTP modes");
}

const repeatedConnectorDescriptionHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenConnectorCard, {
    item: {
      id: "playwright",
      variant: "catalog",
      displayName: "Playwright",
      description: "Playwright",
      state: "connected",
      stateLabel: "已连接",
      connected: true,
    },
  }),
);

if (repeatedConnectorDescriptionHtml.includes("aw-connector-card__description")) {
  throw new Error("AwakenConnectorCard should suppress a description that repeats its title");
}

const usefulConnectorDescriptionHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenConnectorCard, {
    item: {
      id: "filesystem",
      variant: "server",
      displayName: "filesystem",
      description: "stdio · user",
      state: "connected",
      stateLabel: "已连接",
      enabled: true,
    },
  }),
);

if (!usefulConnectorDescriptionHtml.includes("aw-connector-card__description")) {
  throw new Error("AwakenConnectorCard should retain useful supporting text");
}

const installedCatalogControlsHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenConnectorCard, {
    item: {
      id: "playwright",
      variant: "catalog",
      displayName: "Playwright",
      description: "Playwright",
      state: "connected",
      stateLabel: "已连接",
      connected: true,
      enabled: true,
      mcpServerName: "playwright",
    },
  }),
);

if (
  !installedCatalogControlsHtml.includes('role="switch"') ||
  !installedCatalogControlsHtml.includes("设置 Playwright") ||
  installedCatalogControlsHtml.includes("断开 Playwright")
) {
  throw new Error("Installed MCP catalog cards should render server switch and settings controls");
}

const unclosedMermaidHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, {
    markdown: "```mermaid\nflowchart LR\n  A --> B",
    status: "done",
  }),
);

if (unclosedMermaidHtml.includes("assistant-mermaid-diagram")) {
  throw new Error("AwakenAssistantMessage should keep unclosed Mermaid fences as code while streaming");
}

if (!unclosedMermaidHtml.includes("language-mermaid")) {
  throw new Error("AwakenAssistantMessage did not preserve unclosed Mermaid as a code block");
}

if (!unclosedMermaidHtml.includes("assistant-message-code-language\">mermaid")) {
  throw new Error("AwakenAssistantMessage did not label the unclosed Mermaid fallback");
}

const unknownLanguageHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenMarkdownRenderer, {
    markdown: "```awakenlang\nmake something_safe\n```",
  }),
);

if (
  !unknownLanguageHtml.includes("assistant-message-code-language\">awakenlang") ||
  !unknownLanguageHtml.includes("make something_safe")
) {
  throw new Error("AwakenMarkdownRenderer should retain unknown-language code and its label");
}

if (unknownLanguageHtml.includes("hljs-keyword")) {
  throw new Error("AwakenMarkdownRenderer should not guess syntax for unknown languages");
}

const unsafeMarkdownHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenMarkdownRenderer, {
    markdown: '<script data-awaken-test="unsafe">alert(1)</script>safe',
  }),
);

if (unsafeMarkdownHtml.includes("<script") || unsafeMarkdownHtml.includes("alert(1)")) {
  throw new Error("AwakenMarkdownRenderer should keep raw HTML inert after adding highlighting");
}

const scheduledTaskListHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenScheduledTaskList, {
    items: [
      {
        taskId: "sched_demo_running",
        name: "工作日科技新闻早报",
        kindLabel: "Agent",
        projectName: "Awaken",
        scheduleText: "工作日 09:30 · Asia/Shanghai",
        enabled: true,
        state: "running" as const,
        stateLabel: "运行中",
        nextRunText: "下次：明天 09:30",
        lastRunText: "最近：今天 09:30 成功",
        running: true,
      },
    ],
  }),
);

if (
  !scheduledTaskListHtml.includes('role="switch"') ||
  !scheduledTaskListHtml.includes("aw-scheduled-task-list__row") ||
  !scheduledTaskListHtml.includes("运行中") ||
  !scheduledTaskListHtml.includes("工作日 09:30 · Asia/Shanghai")
) {
  throw new Error("AwakenScheduledTaskList should render a running task row with switch and state badge");
}

const scheduledTaskEmptyListHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenScheduledTaskList, { items: [] }),
);

if (
  !scheduledTaskEmptyListHtml.includes("创建任务") ||
  !scheduledTaskEmptyListHtml.includes("从模板创建")
) {
  throw new Error("AwakenScheduledTaskList empty state must offer both create entries");
}

const scheduledTaskTemplateDialogHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenScheduledTaskTemplateDialog, {
    open: true,
    title: "从模板创建",
    description: "先选内置模板，再在编辑器里微调关键参数后保存",
    categories: [
      {
        id: "cron",
        label: "循环任务",
        templates: [
          {
            templateId: "daily_tech_news_brief",
            name: "工作日科技新闻早报",
            description: "每个工作日早上整理科技新闻",
            frequencyText: "工作日 09:30",
          },
        ],
      },
      {
        id: "calendar",
        label: "日程任务",
        templates: [
          {
            templateId: "once_text_birthday_reminder",
            name: "生日提醒",
            description: "下一个 1 月 1 日早上提醒",
            frequencyText: "下一个 1 月 1 日 09:00，不重复",
          },
        ],
      },
    ],
    onOpenChange: () => undefined,
    onSelect: () => undefined,
  }),
);

if (
  !scheduledTaskTemplateDialogHtml.includes("循环任务") ||
  !scheduledTaskTemplateDialogHtml.includes("日程任务") ||
  !scheduledTaskTemplateDialogHtml.includes("使用模板") ||
  !scheduledTaskTemplateDialogHtml.includes("先选内置模板")
) {
  throw new Error("AwakenScheduledTaskTemplateDialog should expose both categories and template cards");
}

const scheduledTaskEditorHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenScheduledTaskEditorDialog, {
    open: true,
    mode: "create",
    value: AwakenDesignSystem.createEmptyAwakenScheduledTaskDraft("Asia/Shanghai"),
    projects: [{ projectId: "project_demo", name: "Awaken" }],
    profiles: [{ profileId: "profile_demo", name: "Kimi for Coding" }],
    timezones: ["Asia/Shanghai"],
    errorMessage: "保存失败：请修正以下字段后重试。",
    fieldErrors: { name: "名称不能为空" },
    onOpenChange: () => undefined,
    onValueChange: () => undefined,
    onSubmit: () => undefined,
  }),
);

if (
  !scheduledTaskEditorHtml.includes("保存任务") ||
  !scheduledTaskEditorHtml.includes("每次运行时使用当前默认 profile") ||
  !scheduledTaskEditorHtml.includes("循环任务") ||
  !scheduledTaskEditorHtml.includes("日程任务") ||
  !scheduledTaskEditorHtml.includes("高级选项") ||
  !scheduledTaskEditorHtml.includes("名称不能为空") ||
  !scheduledTaskEditorHtml.includes('role="alert"')
) {
  throw new Error("AwakenScheduledTaskEditorDialog should render all sections and inline field errors");
}

const scheduledTaskHistoryHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenScheduledTaskHistoryDialog, {
    open: true,
    title: "执行记录",
    items: [
      {
        executionId: "exec_demo",
        triggerLabel: "计划触发",
        status: "error" as const,
        statusLabel: "失败",
        scheduledForText: "2026-07-26 15:00",
        startedAtText: "2026-07-26 15:00:01",
        finishedAtText: "2026-07-26 15:00:12",
        errorText: "MODEL_PROFILE_UNAVAILABLE：显式 profile 已被禁用",
        threadId: "thread_demo",
      },
    ],
    onOpenChange: () => undefined,
    onOpenThread: () => undefined,
  }),
);

if (
  !scheduledTaskHistoryHtml.includes("打开线程") ||
  !scheduledTaskHistoryHtml.includes("MODEL_PROFILE_UNAVAILABLE") ||
  !scheduledTaskHistoryHtml.includes("失败")
) {
  throw new Error("AwakenScheduledTaskHistoryDialog should render error rows with the thread action");
}

const scheduledTaskHistoryEmptyHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenScheduledTaskHistoryDialog, {
    open: true,
    title: "执行记录",
    items: [],
    onOpenChange: () => undefined,
  }),
);

if (!scheduledTaskHistoryEmptyHtml.includes("还没有执行记录")) {
  throw new Error("AwakenScheduledTaskHistoryDialog should render its empty state");
}

if (
  !productionCss.includes(".aw-scheduled-task-list__row") ||
  !productionCss.includes(".aw-scheduled-task-template-dialog") ||
  !productionCss.includes(".aw-scheduled-task-editor__section") ||
  !productionCss.includes(".aw-select[data-menu-placement=top] .aw-select__menu") ||
  !productionCss.includes(".aw-scheduled-task-state-badge")
) {
  throw new Error("Production CSS is missing scheduled task component surfaces");
}

const sidebarUpdateStateIds = AwakenDesignSystem.AwakenSidebarStates
  .map((state) => state.id)
  .filter((id) => id.startsWith("update-"));

if (
  sidebarUpdateStateIds.join(",") !==
  [
    "update-available",
    "update-downloading-indeterminate",
    "update-downloading-37",
    "update-downloaded",
    "update-error",
  ].join(",")
) {
  throw new Error(`AwakenSidebar update states are incomplete or out of order: ${sidebarUpdateStateIds.join(", ")}`);
}

const sidebarUpdateOnlyHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenSidebar, {
    profileUpdateAction: { state: "available", label: "更新到 v0.2.0", onClick: () => undefined },
  }),
);

if (
  !sidebarUpdateOnlyHtml.includes("aw-sidebar-profile") ||
  !sidebarUpdateOnlyHtml.includes("aw-sidebar-update") ||
  !sidebarUpdateOnlyHtml.includes('type="button"')
) {
  throw new Error("AwakenSidebar must render the update entry button even without profile fields");
}

const sidebarUpdateProgressState = AwakenDesignSystem.AwakenSidebarStates.find(
  (state) => state.id === "update-downloading-37",
);
const sidebarUpdateProgressHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenSidebar, sidebarUpdateProgressState?.props ?? {}),
);

if (
  !sidebarUpdateProgressHtml.includes('role="progressbar"') ||
  !sidebarUpdateProgressHtml.includes('aria-valuenow="37"') ||
  !sidebarUpdateProgressHtml.includes('aria-busy="true"')
) {
  throw new Error("AwakenSidebar update entry must expose a determinate progressbar while downloading");
}

const sidebarUpdateIndeterminateState = AwakenDesignSystem.AwakenSidebarStates.find(
  (state) => state.id === "update-downloading-indeterminate",
);
const sidebarUpdateIndeterminateHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenSidebar, sidebarUpdateIndeterminateState?.props ?? {}),
);

if (sidebarUpdateIndeterminateHtml.includes('role="progressbar"')) {
  throw new Error("AwakenSidebar update entry must not render a progressbar without a finite progress");
}

const sidebarUpdateIndex = sidebarUpdateProgressHtml.indexOf("aw-sidebar-update");
const sidebarProfileIndex = sidebarUpdateProgressHtml.indexOf('aria-label="用户 Nolan"');

if (sidebarProfileIndex === -1 || sidebarUpdateIndex === -1 || sidebarUpdateIndex > sidebarProfileIndex) {
  throw new Error("AwakenSidebar update entry must be a sibling above the profile button, not nested inside it");
}

const expectedRunPlanStateIds = [
  "in-progress",
  "first-step",
  "last-step",
  "all-completed",
  "with-explanation",
  "long-steps-overflow",
];
const runPlanStateIds = AwakenDesignSystem.AwakenRunPlanStates.map((state) => state.id);

if (runPlanStateIds.join(",") !== expectedRunPlanStateIds.join(",")) {
  throw new Error(`AwakenRunPlan states are incomplete or out of order: ${runPlanStateIds.join(", ")}`);
}

const runPlanSmokeItems = [
  {
    step: "梳理运行计划的 Core 快照",
    activeForm: "正在梳理运行计划的 Core 快照",
    status: "completed" as const,
  },
  {
    step: "实现运行计划协议",
    activeForm: "正在实现运行计划协议",
    status: "in_progress" as const,
  },
  {
    step: "补齐自动化与真实 Desktop 验收",
    activeForm: "正在补齐自动化与真实 Desktop 验收",
    status: "pending" as const,
  },
];

const runPlanTriggerHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenRunPlanTrigger, {
    items: runPlanSmokeItems,
    completedCount: 1,
    totalCount: 3,
    currentStep: 2,
    allCompleted: false,
    open: false,
    onToggle: () => undefined,
  }),
);

if (
  !runPlanTriggerHtml.includes("第 2/3 步 · 正在实现运行计划协议") ||
  !runPlanTriggerHtml.includes('aria-haspopup="dialog"') ||
  !runPlanTriggerHtml.includes('aria-expanded="false"') ||
  !runPlanTriggerHtml.includes("共 3 步") ||
  !runPlanTriggerHtml.includes("已完成 1 步") ||
  !runPlanTriggerHtml.includes("aw-run-plan-spin")
) {
  throw new Error("AwakenRunPlanTrigger must render the step summary, accessible name and spinning in-progress icon");
}

const runPlanCompletedTriggerHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenRunPlanTrigger, {
    items: runPlanSmokeItems.map((item) => ({ ...item, status: "completed" as const })),
    completedCount: 3,
    totalCount: 3,
    currentStep: 3,
    allCompleted: true,
    open: true,
    onToggle: () => undefined,
  }),
);

if (
  !runPlanCompletedTriggerHtml.includes("3/3 · 计划已完成") ||
  !runPlanCompletedTriggerHtml.includes("全部完成")
) {
  throw new Error("AwakenRunPlanTrigger must render the M/M completion summary when all steps are completed");
}

const runPlanPanelHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenRunPlanPanel, {
    items: runPlanSmokeItems,
    explanation: "调查后增加了一个必要步骤。",
    onClose: () => undefined,
  }),
);

if (
  !runPlanPanelHtml.includes('role="dialog"') ||
  !runPlanPanelHtml.includes("运行计划") ||
  !runPlanPanelHtml.includes("aw-run-plan-panel-wrap") ||
  !runPlanPanelHtml.includes("调查后增加了一个必要步骤。") ||
  !runPlanPanelHtml.includes("已完成") ||
  !runPlanPanelHtml.includes("进行中") ||
  !runPlanPanelHtml.includes("待处理")
) {
  throw new Error("AwakenRunPlanPanel must render the dialog, explanation and textual tri-state for every step");
}

const runPlanHistoricalHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenRunPlanHistorical, {
    items: runPlanSmokeItems,
    completedCount: 1,
    totalCount: 3,
  }),
);

if (
  !runPlanHistoricalHtml.includes("执行计划 · 1/3") ||
  runPlanHistoricalHtml.includes("aw-run-plan-spin")
) {
  throw new Error("AwakenRunPlanHistorical must render the terminal title without animated icons");
}

if (
  !productionCss.includes(".aw-run-plan-trigger") ||
  !productionCss.includes("min-height:44px") ||
  !productionCss.includes(".aw-run-plan-panel") ||
  !productionCss.includes(".aw-run-plan-historical") ||
  !productionCss.includes("aw-run-plan-spin")
) {
  throw new Error("Production CSS is missing AwakenRunPlan component surfaces");
}

console.log(`awaken design system render smoke passed: ${demoEntries.length} components`);
