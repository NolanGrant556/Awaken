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
  it("keeps global operation feedback floating at the centered top edge", () => {
    const workbenchStyles = readFileSync(
      new URL("./styles/workbench.css", import.meta.url),
      "utf8",
    );
    const toast = cssRule(workbenchStyles, ".desktop-toast");

    expect(toast).toContain("position: absolute;");
    expect(toast).toContain("top: 16px;");
    expect(toast).toContain("left: 50%;");
    expect(toast).toContain("transform: translateX(-50%);");
    expect(toast).toContain("pointer-events: none;");
    expect(toast).not.toContain("right:");
    expect(toast).not.toContain("bottom:");
  });

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

  it("keeps file tabs scrollable while panel controls stay in fixed chrome actions", () => {
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
      'className="project-files-tabs"',
      railStart,
    );
    const addStart = appSource.indexOf('aria-label="新建文件标签"', tabsStart);
    const actionsStart = appSource.indexOf(
      '<div className="project-files-chrome-actions">',
      addStart,
    );
    const expandToggleStart = appSource.indexOf(
      'className="workbench-chrome-icon workbench-chrome-expand-toggle"',
      actionsStart,
    );
    const inspectorToggleStart = appSource.indexOf(
      'className="workbench-chrome-icon workbench-chrome-panel-toggle"',
      expandToggleStart,
    );

    expect(railStart).toBeGreaterThan(-1);
    expect(tabsStart).toBeGreaterThan(railStart);
    expect(addStart).toBeGreaterThan(tabsStart);
    expect(actionsStart).toBeGreaterThan(addStart);
    expect(expandToggleStart).toBeGreaterThan(actionsStart);
    expect(inspectorToggleStart).toBeGreaterThan(expandToggleStart);

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
    const expandToggle = cssRule(
      projectFileStyles,
      ".workbench-chrome-expand-toggle",
    );

    expect(rail).toContain("flex: 1 1 auto;");
    expect(rail).toContain("overflow: hidden;");
    expect(tabs).toContain("flex: 0 1 auto;");
    expect(tabs).toContain("overflow-x: auto;");
    expect(addWrapper).toContain("flex: 0 0 auto;");
    expect(actions).toContain("flex: 0 0 auto;");
    expect(expandToggle).toContain("flex: 0 0 auto;");
  });

  it("keeps Artifact tabs mounted while project-file slots become active", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );

    expect(appSource).toContain(
      "artifactPreviewTabsState.tabs.map((artifactTab) =>",
    );
    expect(appSource).toContain(
      "artifactPreviewTabMediaIconConcept(\n                          artifactSelection.mediaKind,",
    );
    expect(appSource).toMatch(
      /<AwakenIcon\s+concept=\{mediaIconConcept\}\s+size=\{16\}/,
    );
    expect(appSource).toContain(
      "data-file-icon={mediaIconConcept ?? iconKind}",
    );
    expect(appSource).toContain(
      'dispatchArtifactPreviewTabs({ type: "deactivate" });\n    dispatchFileWorkspaceTabs({ type: "open", tab });',
    );
    expect(appSource).toContain("handleOpenDeliverablePreview(file, threadId)");
    expect(appSource).toContain("isMedia && threadId\n                  ? () => handleOpenDeliverablePreview");
    expect(appSource).not.toContain(
      'isMedia && typeof mediaSourceState === "string" && threadId',
    );
    expect(appSource).not.toContain("pinnedMedia");
    expect(appSource).not.toContain("MediaInspector");
    expect(appSource).not.toContain("setDeliverablePreview");
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
    expect(expandedChrome).not.toContain("transform");
    expect(designSystemStyles).not.toMatch(
      /\.workbench-chrome-icon:active\s*\{[^}]*transform\s*:/,
    );

    const panelToggle = cssRule(
      designSystemStyles,
      ".workbench-chrome-panel-toggle",
    );
    expect(panelToggle).toContain("position: absolute;");
    expect(panelToggle).toContain(
      "top: calc(50% - 11px + var(--aw-window-control-top-offset, -1px));",
    );
    expect(panelToggle).toContain("right: 12px;");
    expect(panelToggle).toContain("appearance: none;");
    expect(panelToggle).toContain("-webkit-appearance: none;");
    expect(panelToggle).not.toContain("transform");

    expect(designSystemStyles).toMatch(
      /\.desktop-root-inspector-expanded\s+\.workbench-chrome-right:has\(\.workbench-chrome-panel-toggle\)\s*\{\s*padding-right: 42px;/,
    );
    expect(designSystemStyles).toMatch(
      /\.workbench-chrome-panel-toggle:hover,\s*\.workbench-chrome-panel-toggle:focus-visible\s*\{\s*box-shadow: var\(--aw-shadow-control\);/,
    );
    expect(designSystemStyles).not.toMatch(
      /\.workbench-chrome-panel-toggle::(?:before|after)/,
    );

    const appSource = readFileSync(new URL("./App.tsx", import.meta.url), "utf8");
    expect(appSource).toContain("fileWorkspacePanelActionForState({");
    expect(appSource).toContain(
      'fileWorkspacePanelAction === "contract"\n                                ? handleContractFileWorkspace\n                                : handleExpandFileWorkspace',
    );
  });

  it("keeps chrome icons raster-stable at press and toggle time", () => {
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );

    // Fractional offsets park the 22px icon buttons off the device pixel
    // grid; any re-rasterization then shifts stroke anti-aliasing and reads
    // as a subtle jump on Retina displays.
    const icon = cssRule(designSystemStyles, ".workbench-chrome-icon");
    expect(icon).toContain("top: var(--aw-window-control-top-offset, -1px);");
    expect(designSystemStyles).not.toContain(
      "var(--aw-window-control-top-offset, -0.75px)",
    );

    // Idle dimming must stay on the status text: animating opacity on the
    // container that owns the toggle button promotes/demotes a compositor
    // layer around pointer interaction and re-rasterizes the icon.
    const statusContainer = cssRule(designSystemStyles, ".workbench-chrome-status");
    expect(statusContainer).not.toContain("opacity");
    const statusText = cssRule(designSystemStyles, ".workbench-chrome-state");
    expect(statusText).toContain("opacity: 0.56;");
    expect(statusText).toContain("transition: opacity 140ms ease;");
    expect(designSystemStyles).toContain(
      ".workbench-chrome-status:hover .workbench-chrome-state,",
    );
    expect(designSystemStyles).toContain(
      ".workbench-chrome-status:focus-within .workbench-chrome-state {",
    );
  });

  it("fades the inspector resize-handle line in only after the panel expansion arrives", () => {
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );

    expect(designSystemStyles).toMatch(
      /\.aw-workbench-inspector-resize-handle::before \{\s*opacity: 0;\s*animation: aw-inspector-handle-line-in 120ms ease 180ms forwards;\s*\}/,
    );
    expect(designSystemStyles).toContain("@keyframes aw-inspector-handle-line-in");
    expect(designSystemStyles).toMatch(
      /@media \(prefers-reduced-motion: reduce\) \{\s*\.aw-workbench-inspector-resize-handle::before \{\s*opacity: 1;\s*animation: none;/,
    );
  });

  it("uses semantic disclosure identities instead of mutable render positions", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );

    expect(appSource).toContain(
      "`${disclosureScope}:segment:${segment.id}`",
    );
    expect(appSource).toContain(
      "`${disclosureKey}:action:${action.id}`",
    );
    expect(appSource).toContain(
      '}:turn:${turn.id}`;',
    );
    expect(appSource).toContain(
      "`${turnDisclosureScope}:body:${segment.id}`",
    );
    expect(appSource).not.toContain(
      "`${disclosureScope}:segment:${segmentIndex}`",
    );
    expect(appSource).not.toContain(
      "`${disclosureKey}:action:${actionIndex}`",
    );
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

  it("fades collapsed prompt overflow instead of ellipsis, and keeps copy as a flat icon", () => {
    const designSystemStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ),
      "utf8",
    );
    const collapsedText = designSystemStyles.slice(
      designSystemStyles.indexOf(
        ".execution-prompt-block-limited:not(.execution-prompt-block-expanded)",
      ),
      designSystemStyles.indexOf(
        ".execution-prompt-block-expanded .execution-prompt-bubble {",
      ),
    );

    expect(collapsedText).toContain("padding-bottom: 0;");
    expect(collapsedText).toContain(
      "margin-bottom: calc(-1 * var(--execution-prompt-attachment-gap));",
    );
    expect(collapsedText).toContain("mask-image:");
    expect(collapsedText).toContain("-webkit-mask-image:");
    expect(collapsedText).toContain("calc(100% - 16px)");
    expect(collapsedText).toContain("mask-size: 100% 100%;");
    expect(collapsedText).toContain("overflow: hidden;");
    expect(collapsedText).not.toContain(".execution-prompt-bubble::after");
    expect(collapsedText).not.toContain(".execution-prompt-bubble-main::after");
    expect(collapsedText).not.toContain(".execution-prompt-bubble-text::after");
    expect(collapsedText).not.toContain("-webkit-line-clamp");
    expect(collapsedText).toContain("line-height: 16px;");

    const copyButton = cssRule(designSystemStyles, ".execution-prompt-copy-button");
    expect(copyButton).toContain("background: transparent !important;");
    expect(copyButton).toContain("border: 0 !important;");
    expect(copyButton).toContain("box-shadow: none !important;");

    const workbenchStyles = readFileSync(
      new URL("./styles/workbench.css", import.meta.url),
      "utf8",
    );
    expect(workbenchStyles).toContain(
      ".execution-prompt-block-limited:not(.execution-prompt-block-expanded)",
    );
    expect(workbenchStyles).toContain(
      "--execution-prompt-collapsed-content-height: 48px;",
    );
    expect(workbenchStyles).not.toContain(
      "--execution-prompt-collapsed-content-height: 50px;",
    );
    expect(workbenchStyles).toContain("padding: 10px 16px 0;");
    expect(workbenchStyles).toMatch(
      /\.execution-turn-header[\s\S]*?\.execution-prompt-block-limited:not\(\.execution-prompt-block-expanded\)[\s\S]*?\.execution-prompt-bubble \{[\s\S]*?padding: 10px 16px 0;/,
    );
  });

  it("collapses a hidden rewind dialog overlay instead of leaving a fullscreen cover", () => {
    const dialogStyles = readFileSync(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenDialog/AwakenDialog.css",
        import.meta.url,
      ),
      "utf8",
    );
    const hiddenBackdrop = cssRule(dialogStyles, ".aw-dialog-backdrop--hidden");
    expect(hiddenBackdrop).toContain("animation: none;");
    expect(hiddenBackdrop).toContain("inset: auto;");
    expect(hiddenBackdrop).toContain("width: 0;");
    expect(hiddenBackdrop).toContain("height: 0;");
    expect(hiddenBackdrop).toContain("visibility: hidden;");
    expect(hiddenBackdrop).toContain("pointer-events: none;");
    expect(dialogStyles).toContain("@keyframes aw-dialog-backdrop-in");
  });
});
