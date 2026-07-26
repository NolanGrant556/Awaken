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
const footerState = AwakenDesignSystem.AwakenAssistantMessageStates.find(
  (state) => state.id === "completed-answer-footer",
);
const completedStatesWithoutElapsed = AwakenDesignSystem.AwakenAssistantMessageStates.filter(
  (state) => state.props.status === "done" && typeof state.props.elapsedMs !== "number",
);

if (!richMarkdownState) {
  throw new Error("AwakenAssistantMessage rich-markdown state is missing");
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

if (!footerHtml.includes('<span class="assistant-message-footer-copy"><span')) {
  throw new Error("AwakenAssistantMessage should place the copy action wrapper directly in the footer layout");
}

if (footerHtml.includes(">Copy final answer</span>")) {
  throw new Error("AwakenAssistantMessage should render the final answer copy action as an icon-only control");
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

if (!copyOnlyFooterHtml.includes('<span class="assistant-message-footer-copy"><span')) {
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

console.log(`awaken design system render smoke passed: ${demoEntries.length} components`);
