import {
  AwakenConnectorCard,
  AwakenConnectors,
  AwakenSkillCard,
} from "@awaken/design-system";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

describe("connector and skill icon consumption", () => {
  it("uses the official MCP mark in the MCP tab and MCP cards, plus a terminal in the CLI tab", () => {
    const html = renderToString(
      <AwakenConnectors
        activeKind="mcp"
        searchValue=""
        builtInMcpConnectors={[
          {
            id: "playwright",
            variant: "catalog",
            displayName: "Playwright",
            description: "浏览器自动化",
          },
        ]}
        userMcpConnectors={[]}
        cliConnectors={[]}
        onKindChange={vi.fn()}
        onSearchChange={vi.fn()}
        onRefresh={vi.fn()}
        onConfigureMcp={vi.fn()}
        onConnectorAction={vi.fn()}
        onMcpServerEnabledChange={vi.fn()}
        onMcpServerSettings={vi.fn()}
      />,
    );

    expect(html.match(/aw-icon--mcp-mark/g)?.length).toBeGreaterThanOrEqual(2);
    expect(html).toContain("aw-icon--terminal");
    expect(html.match(/<svg[^>]*width="16"[^>]*aw-icon--mcp-mark[^>]*>/)?.[0]).toBeDefined();
    expect(html.match(/<svg[^>]*width="16"[^>]*aw-icon--terminal[^>]*>/)?.[0]).toBeDefined();
  });

  it("uses the official MCP mark for standalone user MCP cards", () => {
    const html = renderToString(
      <AwakenConnectorCard
        item={{
          id: "user-server",
          variant: "server",
          displayName: "User MCP",
          description: "stdio · user",
        }}
      />,
    );

    expect(html).toContain("aw-icon--mcp-mark");
    expect(html).not.toContain("aw-icon--server");
  });

  it("uses the open-book icon for Skill cards", () => {
    const html = renderToString(
      <AwakenSkillCard
        skill={{
          id: "research",
          name: "Research",
          description: "调研与归纳",
          enabled: true,
          scope: "global",
        }}
      />,
    );

    expect(html).toContain("aw-icon--book-open");
  });
});
