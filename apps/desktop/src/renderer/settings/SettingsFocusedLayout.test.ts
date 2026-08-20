/// <reference types="node" />

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const settingsStyles = readFileSync(
  new URL(
    "../../../../../Awaken Design System/src/components/AwakenSettings/AwakenSettings.css",
    import.meta.url,
  ),
  "utf8",
);
const petsStyles = readFileSync(
  new URL(
    "../../../../../Awaken Design System/src/components/AwakenPetsSettings/AwakenPetsSettings.css",
    import.meta.url,
  ),
  "utf8",
);
const computerStyles = readFileSync(
  new URL(
    "../../../../../Awaken Design System/src/components/AwakenComputerSettings/AwakenComputerSettings.css",
    import.meta.url,
  ),
  "utf8",
);
const searchStyles = readFileSync(
  new URL(
    "../../../../../Awaken Design System/src/components/AwakenSearchSettings/AwakenSearchSettings.css",
    import.meta.url,
  ),
  "utf8",
);
const desktopStyles = readFileSync(
  new URL("../styles.css", import.meta.url),
  "utf8",
);
const globalAgentsStyles = readFileSync(
  new URL("./GlobalAgentsSettingsPanel.css", import.meta.url),
  "utf8",
);
const globalAgentsComponent = readFileSync(
  new URL("./GlobalAgentsSettingsPanel.tsx", import.meta.url),
  "utf8",
);

describe("focused settings layout", () => {
  it("centers the compact settings pages on one shared width", () => {
    expect(settingsStyles).toContain(
      "--aw-settings-focused-content-width: 820px;",
    );
    for (const styles of [
      petsStyles,
      computerStyles,
      searchStyles,
      desktopStyles,
    ]) {
      expect(styles).toContain(
        "width: min(100%, var(--aw-settings-focused-content-width, 820px));",
      );
      expect(styles).toContain("margin-inline: auto;");
    }

    expect(petsStyles).toContain("min-height: 56px;");
    expect(computerStyles).toMatch(
      /\.computer-settings-card\s*\{[\s\S]*?padding:\s*14px;/,
    );
    expect(searchStyles).toMatch(
      /\.search-channel-card\s*\{[\s\S]*?min-height:\s*76px;[\s\S]*?padding:\s*12px 14px;/,
    );
    expect(desktopStyles).toContain("min-height: 76px;");
  });

  it("uses a bounded centered ACP dialog instead of the full-height drawer", () => {
    expect(desktopStyles).toContain("width: min(680px, calc(100vw - 56px));");
    expect(desktopStyles).toContain(
      "max-height: min(720px, calc(100vh - 56px));",
    );
    expect(desktopStyles).not.toContain(".acp-runner-drawer");
  });

  it("keeps the full-bleed model page as the vertical scroll owner", () => {
    expect(settingsStyles).toMatch(
      /\.aw-settings-main--full-bleed > \.aw-model-settings-page\s*\{[\s\S]*?height:\s*100%;[\s\S]*?overflow-x:\s*hidden;[\s\S]*?overflow-y:\s*auto;[\s\S]*?overscroll-behavior:\s*contain;/,
    );
    expect(settingsStyles).toMatch(
      /\.aw-settings-main\s*\{[\s\S]*?min-height:\s*0;[\s\S]*?overflow:\s*hidden;/,
    );
    expect(settingsStyles).toMatch(
      /\.aw-settings-main--full-bleed > \.aw-model-settings-page\s*\{[\s\S]*?scrollbar-width:\s*none;/,
    );
  });

  it("uses the shared lightweight scrollbar and keeps global rules inside its editor", () => {
    expect(settingsStyles).toMatch(
      /\.aw-settings-scroll\s*\{[\s\S]*?scrollbar-color:\s*color-mix\(in srgb, var\(--aw-text-tertiary\) 28%, transparent\) transparent;/,
    );
    expect(settingsStyles).toMatch(
      /\.aw-settings-scroll::-webkit-scrollbar-thumb\s*\{[\s\S]*?min-height:\s*44px;[\s\S]*?border-radius:\s*var\(--aw-radius-pill\);/,
    );
    expect(settingsStyles).toMatch(
      /\.aw-settings-scroll::-webkit-scrollbar-button\s*\{[\s\S]*?display:\s*none;/,
    );
    expect(globalAgentsStyles).toMatch(
      /\.aw-agents-settings textarea\s*\{[\s\S]*?height:\s*clamp\(300px, 42vh, 400px\);[\s\S]*?max-height:\s*400px;[\s\S]*?resize:\s*none;[\s\S]*?overflow-y:\s*auto;/,
    );
    expect(globalAgentsStyles).toMatch(
      /\.aw-agents-settings__card\s*\{[\s\S]*?padding:\s*4px 0 0;/,
    );
    expect(globalAgentsStyles).toMatch(
      /\.aw-agents-settings__editor:focus-within\s*\{[\s\S]*?box-shadow:\s*0 0 0 3px/,
    );
    expect(globalAgentsStyles).toMatch(
      /\.aw-agents-settings__identity\s*\{[\s\S]*?gap:\s*10px;/,
    );
    expect(globalAgentsStyles).not.toContain("aw-agents-settings__metadata");
    expect(globalAgentsStyles).not.toContain("aw-agents-settings__document-path");
    expect(globalAgentsComponent).not.toContain("~/.awaken/AGENTS.md");
    expect(globalAgentsStyles).toMatch(
      /\.aw-agents-settings textarea::-webkit-scrollbar-thumb\s*\{[\s\S]*?background:\s*color-mix\(in srgb, var\(--aw-text-tertiary\) 28%, transparent\);/,
    );
  });

});
