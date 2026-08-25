/// <reference types="node" />

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

function cssRule(source: string, selector: string): string {
  const start = source.indexOf(`${selector} {`);
  if (start < 0) {
    throw new Error(`Missing CSS rule: ${selector}`);
  }
  const end = source.indexOf("}", start);
  if (end < 0) {
    throw new Error(`Unterminated CSS rule: ${selector}`);
  }
  return source.slice(start, end + 1);
}

describe("RunStage layout", () => {
  it("keeps the model trigger responsive and animates only its visible chevron", () => {
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );

    expect(designSystemStyles).toContain(
      "@container composer-bar-layout (max-width: 440px)",
    );
    expect(designSystemStyles).toMatch(
      /\.composer-bar \.composer-bar-model-trigger-label,\s*\.composer-bar \.composer-bar-model-trigger-chevron\s*\{\s*display: none;/,
    );

    expect(designSystemStyles).toMatch(
      /\.composer-bar-model-trigger-chevron\s*\{\s*display: inline-flex;[\s\S]*?transform: rotate\(0deg\);[\s\S]*?transition: transform 240ms[\s\S]*?\}/,
    );
    expect(designSystemStyles).toMatch(
      /\.composer-bar-model-trigger\[aria-expanded="true"\]\s*\.composer-bar-model-trigger-chevron\s*\{\s*transform: rotate\(180deg\);/,
    );
    expect(designSystemStyles).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.composer-bar-model-trigger-chevron\s*\{\s*transition: none !important;/,
    );
  });

  it("keeps the center-home spacing, project shelf, and sidebar typography aligned", () => {
    const workbenchStyles = readFileSync(
      new URL("./styles/workbench.css", import.meta.url),
      "utf8",
    );
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );
    const composerStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenComposerBar/AwakenComposerBar.css",
        import.meta.url,
      ),
      "utf8",
    );
    const newTaskSource = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenNewTaskButton/AwakenNewTaskButton.tsx",
        import.meta.url,
      ),
      "utf8",
    );
    const primaryNavSource = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenPrimaryNav/AwakenPrimaryNav.tsx",
        import.meta.url,
      ),
      "utf8",
    );

    expect(cssRule(workbenchStyles, ".run-stage-center-composer")).toContain(
      "gap: 16px;",
    );
    expect(cssRule(designSystemStyles, ".aw-task-welcome")).toContain("gap: 14px;");
    expect(cssRule(designSystemStyles, ".aw-task-welcome__message")).toContain(
      "font-size: var(--aw-font-size-lg);",
    );
    expect(cssRule(designSystemStyles, ".composer-bar textarea")).toContain(
      "font-size: var(--aw-font-size-md) !important;",
    );
    expect(cssRule(designSystemStyles, ".composer-bar textarea")).toContain(
      "line-height: var(--aw-line-height-md) !important;",
    );

    const composerToken = cssRule(composerStyles, ".composer-bar-token");
    expect(composerToken).toContain("vertical-align: middle;");
    expect(composerToken).toContain("gap: 5px;");
    expect(composerToken).toContain("min-height: var(--aw-line-height-md);");
    expect(composerToken).toContain("margin-inline-end: var(--aw-space-3);");
    expect(composerToken).toContain("font-weight: var(--aw-font-weight-medium);");

    const composerTokenIcon = cssRule(composerStyles, ".composer-bar-token-icon");
    expect(composerTokenIcon).toContain("width: var(--aw-icon-size-16);");
    expect(composerTokenIcon).toContain("height: var(--aw-icon-size-16);");
    expect(composerTokenIcon).not.toContain("transform:");

    const promptBubble = cssRule(designSystemStyles, ".execution-prompt-bubble");
    expect(promptBubble).toContain("--execution-prompt-collapsed-content-height: 60px;");
    expect(promptBubble).toContain("font-size: var(--aw-font-size-base);");
    expect(promptBubble).toContain("line-height: var(--aw-line-height-base);");

    const promptToken = cssRule(designSystemStyles, ".execution-prompt-bubble-token");
    expect(promptToken).toContain("vertical-align: middle;");
    expect(promptToken).toContain("gap: 5px;");
    expect(promptToken).toContain("margin-inline-end: var(--aw-space-3);");
    expect(promptToken).toContain("font-weight: var(--aw-font-weight-medium);");

    const promptTokenIcon = cssRule(
      designSystemStyles,
      ".execution-prompt-bubble-token-icon",
    );
    expect(promptTokenIcon).toContain("width: var(--aw-icon-size-16);");
    expect(promptTokenIcon).toContain("height: var(--aw-icon-size-16);");
    expect(promptTokenIcon).not.toContain("transform:");

    const projectShelf = cssRule(composerStyles, ".composer-bar-project-shelf");
    expect(projectShelf).toContain("width: 100%;");
    expect(projectShelf).toContain("margin: -28px 0 0;");
    expect(projectShelf).not.toContain("calc(100% - 28px)");

    const newTaskStyle = newTaskSource.slice(
      newTaskSource.indexOf("const buttonStyle"),
      newTaskSource.indexOf("const leftStyle"),
    );
    const primaryNavStyle = primaryNavSource.slice(
      primaryNavSource.indexOf("const rowBase"),
      primaryNavSource.indexOf("const iconWrapStyle"),
    );
    expect(newTaskStyle).toContain('fontWeight: "var(--aw-font-weight-medium)"');
    expect(primaryNavStyle).toContain('fontWeight: "var(--aw-font-weight-medium)"');
    expect(newTaskStyle).not.toContain("--aw-font-weight-semibold");
    expect(primaryNavStyle).not.toContain("--aw-font-weight-semibold");
  });

  it("adds breathing room only between adjacent turns", () => {
    const workbenchStyles = readFileSync(
      new URL("./styles/workbench.css", import.meta.url),
      "utf8",
    );
    const adjacentTurns = cssRule(
      workbenchStyles,
      ".execution-turn-section + .execution-turn-section",
    );

    expect(adjacentTurns).toContain("margin-top: var(--aw-space-9);");
    expect(cssRule(workbenchStyles, ".execution-turn-section")).not.toMatch(
      /^\s*margin-top:/m,
    );
  });

  it("reserves symmetric scrollbar space so centered turns do not shift", () => {
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );
    const galleryStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/gallery/styles.css",
        import.meta.url,
      ),
      "utf8",
    );
    const runStageScroll = cssRule(designSystemStyles, ".aw-run-stage-scroll");
    const galleryRunStageScroll = cssRule(
      galleryStyles,
      ".ads-workbench-scene .aw-run-stage-scroll",
    );

    expect(runStageScroll).toContain("overflow: auto;");
    expect(runStageScroll).toContain("scrollbar-gutter: stable both-edges;");
    expect(designSystemStyles).toMatch(
      /\.aw-run-stage-scroll:focus,\s*\.aw-run-stage-scroll:focus-visible\s*\{\s*outline:\s*none;/,
    );
    expect(designSystemStyles).toMatch(
      /:root\[data-aw-focus-modality="pointer"\] :focus-visible\s*\{\s*outline:\s*none !important;/,
    );
    expect(galleryRunStageScroll).toContain(
      "scrollbar-gutter: stable both-edges;",
    );
  });

  it("keeps subagent detail in a pointer-through overlay above the composer", () => {
    const workbenchStyles = readFileSync(
      new URL("./styles/workbench.css", import.meta.url),
      "utf8",
    );
    const overlay = cssRule(workbenchStyles, ".subagent-detail-overlay");
    const card = cssRule(
      workbenchStyles,
      ".subagent-detail-overlay > .aw-subagent-execution-card",
    );

    expect(overlay).toContain("position: absolute;");
    expect(overlay).toContain("pointer-events: none;");
    expect(card).toContain("pointer-events: auto;");
    expect(card).toContain("width: min(680px, 100%);");
    expect(card).toContain("max-height: min(560px, 100%);");
  });

  it("bounds the run-plan overlay between the stage top and measured composer", () => {
    const workbenchStyles = readFileSync(
      new URL("./styles/workbench.css", import.meta.url),
      "utf8",
    );
    const overlay = cssRule(workbenchStyles, ".run-plan-overlay");

    expect(overlay).toContain("position: absolute;");
    expect(overlay).toContain("top: 28px;");
    expect(overlay).toContain("pointer-events: none;");
  });

  it("makes plan, context, subagent, and media overlays mutually exclusive", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );
    const planToggleStart = appSource.indexOf("onToggle={() => {");
    const planToggleEnd = appSource.indexOf("}}", planToggleStart);
    const planToggle = appSource.slice(planToggleStart, planToggleEnd);

    expect(planToggleStart).toBeGreaterThan(-1);
    expect(planToggle).toContain("setContextUsagePanelOpen(false)");
    expect(planToggle).toContain("setOpenSubagentTaskId(undefined)");
    expect(appSource).toContain("runPlanEntry.closePanel(false);");
    expect(appSource).toContain(
      "bottom: runPlanPanelBottomOffset({\n                                  composerContentInset: composerContentInsetPx,",
    );
  });

  it("keeps the composer mounted while ask_user is active", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );

    expect(appSource).not.toContain("{activeUserQuestion ? null : (");
    expect(appSource).toContain("{renderUserQuestionPrompt()}");
    expect(appSource).toContain('<ComposerBar\n                                  value={composerValue}');
  });

  it("keeps the file-tab add button beside the scrollable tabs and outside fixed chrome actions", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );
    const projectFileStyles = readFileSync(
      new URL("./styles/project-files.css", import.meta.url),
      "utf8",
    );
    const railStart = appSource.indexOf(
      '<div className="project-files-tab-rail">',
    );
    const tabsStart = appSource.indexOf(
      '<div className="project-files-tabs"',
      railStart,
    );
    const addStart = appSource.indexOf('aria-label="新建文件标签"', tabsStart);
    const actionsStart = appSource.indexOf(
      '<div className="project-files-chrome-actions">',
      addStart,
    );

    expect(railStart).toBeGreaterThan(-1);
    expect(tabsStart).toBeGreaterThan(railStart);
    expect(addStart).toBeGreaterThan(tabsStart);
    expect(actionsStart).toBeGreaterThan(addStart);

    const rail = cssRule(projectFileStyles, ".project-files-tab-rail");
    const tabs = cssRule(projectFileStyles, ".project-files-tabs");
    const actions = cssRule(
      projectFileStyles,
      ".project-files-chrome-actions",
    );
    const addWrapper = cssRule(
      projectFileStyles,
      ".project-files-tab-rail > span",
    );

    expect(rail).toContain("flex: 1 1 auto;");
    expect(rail).toContain("overflow: hidden;");
    expect(tabs).toContain("flex: 0 1 auto;");
    expect(tabs).toContain("overflow-x: auto;");
    expect(addWrapper).toContain("flex: 0 0 auto;");
    expect(actions).toContain("flex: 0 0 auto;");
  });

  it("keeps the inspector toggle on the same right-side anchor when collapsed", () => {
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );
    const expandedChrome = cssRule(designSystemStyles, ".workbench-chrome-right");
    const collapsedChrome = cssRule(
      designSystemStyles,
      ".desktop-root-inspector-collapsed .workbench-chrome-right",
    );

    expect(expandedChrome).toContain("padding: 0 12px;");
    expect(collapsedChrome).toContain("padding: 0 12px 0 8px;");
  });

  it("renders provider reconnect as a narrative item instead of a timer chrome slot", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );

    expect(appSource).toContain('if (item.kind === "providerReconnect")');
    expect(appSource).toContain("<ReconnectStatus");
    expect(appSource).not.toContain("renderProviderReconnectStatus");
    expect(appSource).not.toContain("isLatestPrompt\n                                      ? renderProviderReconnectStatus()");
  });

  it("renders subagent detail through RunStage overlay instead of the narrative item", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );
    const subagentGroupStart = appSource.indexOf(
      'if (item.kind === "subagentGroup")',
    );
    const nextNarrativeKind = appSource.indexOf(
      'if (item.kind === "scheduledNotice")',
      subagentGroupStart,
    );
    const runStageOverlayStart = appSource.indexOf("overlay={");
    const runStageComposerStart = appSource.indexOf(
      "composer={",
      runStageOverlayStart,
    );

    expect(subagentGroupStart).toBeGreaterThan(-1);
    expect(nextNarrativeKind).toBeGreaterThan(subagentGroupStart);
    expect(
      appSource.slice(subagentGroupStart, nextNarrativeKind),
    ).not.toContain("SubagentExecutionCard");
    expect(runStageOverlayStart).toBeGreaterThan(-1);
    expect(runStageComposerStart).toBeGreaterThan(runStageOverlayStart);
    expect(
      appSource.slice(runStageOverlayStart, runStageComposerStart),
    ).toContain('className="subagent-detail-overlay"');
    expect(
      appSource.slice(runStageOverlayStart, runStageComposerStart),
    ).toContain("<SubagentExecutionCard");
  });

  it("keeps subagent identity and terminal status on one header line", () => {
    const cardSource = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenSubagentExecutionCard/AwakenSubagentExecutionCard.tsx",
        import.meta.url,
      ),
      "utf8",
    );
    const cardStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenSubagentExecutionCard/AwakenSubagentExecutionCard.css",
        import.meta.url,
      ),
      "utf8",
    );
    const headerStart = cardSource.indexOf(
      'className="aw-subagent-execution-card__header"',
    );
    const headerEnd = cardSource.indexOf("</header>", headerStart);
    const headerSource = cardSource.slice(headerStart, headerEnd);
    const card = cssRule(cardStyles, ".aw-subagent-execution-card");
    const body = cssRule(cardStyles, ".aw-subagent-execution-card__body");

    expect(headerStart).toBeGreaterThan(-1);
    expect(headerEnd).toBeGreaterThan(headerStart);
    expect(cardSource).toContain(
      'const headerMeta = [...metrics, awakenSubagentStatusLabels[status]].join(" · ");',
    );
    expect(headerSource).toContain("{agentLabel}：");
    expect(headerSource).toContain("<strong>{title}</strong>");
    expect(headerSource).toContain("{headerMeta}");
    expect(cardSource).toContain("{activity ? (");
    expect(card).toContain("background: var(--aw-surface-raised);");
    expect(card).toContain("color: var(--aw-text-primary);");
    expect(card).toContain("border: 1px solid var(--aw-border-main);");
    expect(cardStyles).not.toContain("--aw-color-");
    expect(cardStyles).not.toContain("background: #fff");
    expect(body).not.toContain("scrollbar-gutter");
    expect(body).toContain("scrollbar-width: thin;");
    expect(cardStyles).toMatch(
      /\.aw-subagent-execution-card__body::\-webkit-scrollbar-track,[\s\S]*?background:\s*transparent;/,
    );
  });

  it("does not expose a user follow-up input inside subagent details", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );
    expect(appSource).not.toContain('id="subagent-overlay-message"');
    expect(appSource).not.toContain('id="subagent-inspector-message"');
    expect(appSource).not.toContain("handleMessageSubagent");
    expect(appSource).not.toContain("subagentMessageDrafts");
    expect(appSource).not.toContain("发送给子任务");
  });

  it("keeps the right-rail subagent status pill on one line", () => {
    const appStyles = readFileSync(
      new URL("./styles.css", import.meta.url),
      "utf8",
    );
    const headerContent = cssRule(
      appStyles,
      ".subagent-inspector__header > div",
    );

    expect(headerContent).toContain("flex: 1;");
    expect(headerContent).toContain("min-width: 0;");
    expect(appStyles).toMatch(
      /\.subagent-inspector__status\s*\{[^}]*flex:\s*0 0 auto;[^}]*white-space:\s*nowrap;/,
    );
  });
});
