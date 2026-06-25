import React from "react";
import { renderToString } from "react-dom/server";
import * as AwakenDesignSystem from "../dist/index.js";

const demoEntries = Object.entries(AwakenDesignSystem)
  .filter(([name, value]) => name.startsWith("Awaken") && name.endsWith("Demo") && typeof value === "function")
  .sort(([a], [b]) => a.localeCompare(b));

if (demoEntries.length === 0) {
  throw new Error("No Awaken component demos were exported");
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

if (!richMarkdownState) {
  throw new Error("AwakenAssistantMessage rich-markdown state is missing");
}

const richMarkdownHtml = renderToString(
  React.createElement(AwakenDesignSystem.AwakenAssistantMessage, richMarkdownState.props),
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
