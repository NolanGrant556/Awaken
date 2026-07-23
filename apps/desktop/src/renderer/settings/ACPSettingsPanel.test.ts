import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { RuntimeClient } from "../runtime/client";
import { ACPSettingsPanel } from "./ACPSettingsPanel";

describe("ACPSettingsPanel", () => {
  it("renders built-in runner cards with probe switches and no password input", () => {
    const html = renderToString(
      React.createElement(ACPSettingsPanel, {
        runtimeClient: {
          getACPRunners: () =>
            Promise.resolve({
              runners: [
                {
                  id: "codex",
                  enabled: false,
                  command: "npx",
                  args: ["-y", "@zed-industries/codex-acp"],
                  env: [{ key: "OPENAI_API_KEY", has_value: true }],
                  permission_mode: "ask",
                  startup_timeout_seconds: 15,
                  turn_timeout_seconds: 600,
                  max_frame_bytes: 8 * 1024 * 1024,
                },
              ],
            }),
        } as unknown as RuntimeClient,
      }),
    );

    expect(html).toContain("opencode");
    expect(html).toContain("qwen_code");
    expect(html).toContain("claude_code");
    expect(html).toContain("codex");
    expect(html).toContain("aw-switch");
    expect(html).toContain("探测");
    expect(html).not.toContain('type="password"');
  });
});
