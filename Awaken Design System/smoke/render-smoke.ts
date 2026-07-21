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

const richMarkdownState = AwakenDesignSystem.AwakenAssistantMessageStates.find(
  (state) => state.id === "rich-markdown",
);
const footerState = AwakenDesignSystem.AwakenAssistantMessageStates.find(
  (state) => state.id === "completed-answer-footer",
);

if (!richMarkdownState) {
  throw new Error("AwakenAssistantMessage rich-markdown state is missing");
}

if (!footerState) {
  throw new Error("AwakenAssistantMessage completed-answer-footer state is missing");
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
