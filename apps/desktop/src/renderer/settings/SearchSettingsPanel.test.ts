import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { RuntimeClient } from "../runtime/client";
import { SearchSettingsPanel } from "./SearchSettingsPanel";

describe("SearchSettingsPanel", () => {
  it("renders Exa and Tavily channel switches without a global switch or API key input", () => {
    const html = renderToString(
      React.createElement(SearchSettingsPanel, {
        runtimeClient: {} as RuntimeClient,
      }),
    );

    expect(html).toContain("Exa");
    expect(html).toContain("Tavily");
    expect(html).toContain("aw-switch");
    expect(html.match(/测试连接/g)).toHaveLength(2);
    expect(html).toContain("免 key");
    expect(html).not.toContain("API key");
    expect(html).not.toContain('type="password"');
  });

  it("keeps each provider's normal content to a metadata row and a description row", () => {
    const html = renderToString(
      React.createElement(SearchSettingsPanel, {
        runtimeClient: {} as RuntimeClient,
      }),
    );

    expect(html.match(/search-channel-title-row/g)).toHaveLength(2);
    expect(
      html.match(
        /search-channel-title-row[\s\S]*?search-channel-meta[\s\S]*?<\/div><p>/g,
      ),
    ).toHaveLength(2);
  });
});
