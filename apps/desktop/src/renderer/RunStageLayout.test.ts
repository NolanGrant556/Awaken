import postcss from "postcss";
/// <reference types="node" />

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { readStyleSource } from "./style-test-source";

function cssRule(source: string, selector: string): string {
  const declarations: string[] = [];
  postcss.parse(source).walkRules(selector, (rule) => {
    if (rule.parent?.type !== "root") return;
    rule.walkDecls((decl) => { declarations.push(decl.prop + ": " + decl.value + (decl.important ? " !important" : "") + ";"); });
  });
  if (!declarations.length) throw new Error("Missing CSS rule: " + selector);
  return selector + " {\n" + declarations.join("\n") + "\n}";
}

describe("RunStage layout", () => {
  it("keeps global operation feedback floating at the centered top edge", () => {
    const workbenchStyles = readStyleSource(new URL("./styles/workbench.css", import.meta.url));
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
    const designSystemStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ));

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
    const workbenchStyles = readStyleSource(new URL("./styles/workbench.css", import.meta.url));
    const designSystemStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ));
    const composerStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/components/AwakenComposerBar/AwakenComposerBar.css",
        import.meta.url,
      ));



    expect(cssRule(workbenchStyles, ".run-stage-center-composer")).toContain(
      "gap: 16px;",
    );
    expect(cssRule(designSystemStyles, ".aw-task-welcome")).toContain(
      "gap: 14px;",
    );
    expect(cssRule(designSystemStyles, ".aw-task-welcome__message")).toContain(
      "font-size: var(--aw-font-size-lg);",
    );
    expect(cssRule(designSystemStyles, ".composer-bar textarea")).toContain(
      "font-size: var(--aw-font-size-md);",
    );
    expect(cssRule(designSystemStyles, ".composer-bar textarea")).toContain(
      "line-height: var(--aw-line-height-md);",
    );

    // Text baselines are measured in scripts/tests/composer-layout.test.mjs.
    const composerToken = cssRule(composerStyles, ".composer-bar-token");
    expect(composerToken).toContain("gap: 5px;");
    expect(composerToken).toContain("min-height: var(--aw-line-height-md);");
    expect(composerToken).toContain("margin-inline-end: var(--aw-space-3);");
    expect(composerToken).toContain(
      "font-weight: var(--aw-font-weight-medium);",
    );

    const composerTokenIcon = cssRule(
      composerStyles,
      ".composer-bar-token-icon",
    );
    expect(composerTokenIcon).toContain("width: var(--aw-icon-size-16);");
    expect(composerTokenIcon).toContain("height: var(--aw-icon-size-16);");
    expect(composerTokenIcon).not.toContain("transform:");

    const promptBubble = cssRule(
      designSystemStyles,
      ".execution-prompt-bubble",
    );
    expect(promptBubble).toContain(
      "--execution-prompt-collapsed-content-height: 60px;",
    );
    expect(promptBubble).toContain("font-size: var(--aw-font-size-base);");
    expect(promptBubble).toContain("line-height: var(--aw-line-height-base);");

    const promptToken = cssRule(
      designSystemStyles,
      ".execution-prompt-bubble-token",
    );
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


  });

  it("adds breathing room only between adjacent turns", () => {
    const workbenchStyles = readStyleSource(new URL("./styles/workbench.css", import.meta.url));
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
    const designSystemStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ));
    const galleryStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/gallery/styles.css",
        import.meta.url,
      ));
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

  it("bounds child details inside the inspector with an independent scroll body", () => {
    const styles = readStyleSource(new URL("./styles.css", import.meta.url));
    expect(cssRule(styles, ".subagent-panel")).toContain("overflow: hidden;");
    expect(cssRule(styles, ".subagent-panel__body")).toContain("overflow: auto;");
    expect(cssRule(styles, ".subagent-panel__body")).toContain("overscroll-behavior: contain;");
    expect(styles).not.toContain(".subagent-detail-overlay");
    const shell = readStyleSource(new URL("../../../../Awaken Design System/src/components/AwakenWorkbenchShell/AwakenWorkbenchShell.css", import.meta.url));
    let narrowPosition: string | undefined;
    postcss.parse(shell).walkAtRules("media", media => {
      if (media.params !== "(max-width: 1180px)") return;
      media.walkRules(".aw-workbench-shell-inspector-split:not(.aw-workbench-shell-inspector-collapsed):not(.aw-workbench-shell-inspector-expanded) > .aw-workbench-inspector", rule => {
        rule.walkDecls("position", declaration => { narrowPosition = declaration.value; });
      });
    });
    expect(narrowPosition).toBe("relative");
  });

  it("bounds the run-plan overlay between the stage top and measured composer", () => {
    const workbenchStyles = readStyleSource(new URL("./styles/workbench.css", import.meta.url));
    const overlay = cssRule(workbenchStyles, ".run-plan-overlay");

    expect(overlay).toContain("position: absolute;");
    expect(overlay).toContain("top: 28px;");
    expect(overlay).toContain("pointer-events: none;");
  });

  it("keeps the composer mounted while ask_user is active", () => {
    const appSource = readFileSync(
      new URL("./App.tsx", import.meta.url),
      "utf8",
    );

    expect(appSource).not.toContain("{activeUserQuestion ? null : (");
    expect(appSource).toContain("{renderUserQuestionPrompt()}");
    expect(appSource).toMatch(/<ComposerSurface\s+variant="dock"/);
  });

  it("keeps file tabs scrollable while panel controls stay in fixed chrome actions", () => {
    const projectFileStyles = readStyleSource(new URL("./styles/project-files.css", import.meta.url));
    const rail = cssRule(projectFileStyles, ".project-files-tab-rail");
    const tabs = cssRule(projectFileStyles, ".project-files-tabs");
    const actions = cssRule(projectFileStyles, ".project-files-chrome-actions");
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

  it("keeps the inspector toggle on the same right-side anchor when collapsed", () => {
    const designSystemStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ));
    const expandedChrome = cssRule(
      designSystemStyles,
      ".workbench-chrome-right",
    );
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
  });

  it("keeps chrome icons raster-stable at press and toggle time", () => {
    const designSystemStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ));

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
    const statusContainer = cssRule(
      designSystemStyles,
      ".workbench-chrome-status",
    );
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
    const designSystemStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ));

    expect(designSystemStyles).toMatch(
      /\.aw-workbench-inspector-resize-handle::before \{\s*opacity: 0;\s*animation: aw-inspector-handle-line-in 120ms ease 180ms forwards;\s*\}/,
    );
    expect(designSystemStyles).toContain(
      "@keyframes aw-inspector-handle-line-in",
    );
    expect(designSystemStyles).toMatch(
      /@media \(prefers-reduced-motion: reduce\) \{\s*\.aw-workbench-inspector-resize-handle::before \{\s*opacity: 1;\s*animation: none;/,
    );
  });

  it("fades collapsed prompt overflow instead of ellipsis, and keeps copy as a flat icon", () => {
    const designSystemStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/tokens/components.css",
        import.meta.url,
      ));
    const collapsedRules: string[] = [];
    postcss.parse(designSystemStyles).walkRules((rule) => {
      if (rule.selector.startsWith(".execution-prompt-block-limited:not(.execution-prompt-block-expanded)")) {
        collapsedRules.push(rule.toString());
      }
    });
    const collapsedText = collapsedRules.join("\n");

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

    const copyButton = cssRule(
      designSystemStyles,
      ".execution-prompt-copy-button",
    );
    expect(copyButton).toContain("background: transparent !important;");
    expect(copyButton).toContain("border: 0 !important;");
    expect(copyButton).toContain("box-shadow: none !important;");

    const workbenchStyles = readStyleSource(new URL("./styles/workbench.css", import.meta.url));
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
    const dialogStyles = readStyleSource(new URL(
        "../../../../Awaken Design System/src/components/AwakenDialog/AwakenDialog.css",
        import.meta.url,
      ));
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

// This is an ownership assertion: the actual panel actions are covered by
// WorkbenchComposition.behavior.test.tsx.
it("keeps Subagent details in the inspector instead of the main narrative overlay", () => {
  const root = readFileSync(new URL("./App.tsx", import.meta.url), "utf8");
  const narrative = readFileSync(
    new URL("./useNarrativeRenderer.tsx", import.meta.url),
    "utf8",
  );
  const overlay = readFileSync(
    new URL("./WorkbenchOverlays.tsx", import.meta.url),
    "utf8",
  );
  expect(narrative).not.toContain("SubagentExecutionCard");
  expect(root).toMatch(/overlay=\{\s*<WorkbenchOverlays/);
  expect(overlay).not.toContain("Subagent");
  expect(root.slice(root.indexOf("inspector={"))).toContain("<SubagentPanel");
});
