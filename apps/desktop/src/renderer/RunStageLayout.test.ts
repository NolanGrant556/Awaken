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
