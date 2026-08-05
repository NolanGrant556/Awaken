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
const desktopStyles = readFileSync(
  new URL("../styles.css", import.meta.url),
  "utf8",
);

describe("focused settings layout", () => {
  it("centers the three compact settings pages on one shared width", () => {
    expect(settingsStyles).toContain(
      "--aw-settings-focused-content-width: 820px;",
    );
    for (const styles of [petsStyles, computerStyles, desktopStyles]) {
      expect(styles).toContain(
        "width: min(100%, var(--aw-settings-focused-content-width, 820px));",
      );
      expect(styles).toContain("margin-inline: auto;");
    }

    expect(petsStyles).toContain("min-height: 56px;");
    expect(computerStyles).toMatch(
      /\.computer-settings-card\s*\{[\s\S]*?padding:\s*14px;/,
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
});
