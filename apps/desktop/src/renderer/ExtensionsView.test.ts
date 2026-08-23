import { describe, expect, it, vi } from "vitest";
import type {
  RuntimeConnector,
  RuntimeMcpServer,
  RuntimeSkill,
  RuntimeToolInfo,
} from "../shared/runtime";
import {
  DEFAULT_SKILL_VIEW,
  SKILL_VIEW_TABS,
  canPreviewSkill,
  connectorOwnedMcpServerNames,
  dedupeSkills,
  findConnectorOwnedMcpServer,
  isUserInstalledMcpServer,
  pluginOwnerFromSkillId,
  skillSourceLabel,
  isSkillVisibleInView,
  loadMcpSettingsResources,
  loadSkillListsForRoots,
  loadSkillListsForProjects,
  nextMcpConfigTextForMode,
  skillDiagnosticText,
  withMcpServerToggleState,
} from "./ExtensionsView";

describe("MCP configuration modes", () => {
  it("switches untouched examples between stdio and Streamable HTTP", () => {
    const stdioExample = nextMcpConfigTextForMode("", "stdio");
    expect(stdioExample).toContain('"command": "npx"');

    const httpExample = nextMcpConfigTextForMode(stdioExample, "streamable-http");
    expect(httpExample).toContain('"type": "streamable-http"');
    expect(httpExample).toContain('"url": "https://mcp.example.test/mcp"');
  });

  it("does not discard pasted configuration when the user changes mode", () => {
    const pasted = '{"mcpServers":{"private":{"url":"https://private.example.test/mcp"}}}';
    expect(nextMcpConfigTextForMode(pasted, "streamable-http")).toBe(pasted);
  });
});

describe("MCP source grouping", () => {
  const connector = {
    id: "playwright",
    kind: "mcp",
    displayName: "Playwright",
    category: "code",
    state: "connected",
    sourceOrigin: "builtin",
    authType: "none",
    serverNames: ["playwright"],
  } satisfies RuntimeConnector;

  it("keeps connector-owned MCP snapshots out of the user-installed group", () => {
    const ownedNames = connectorOwnedMcpServerNames([connector]);
    const restoredBuiltin = {
      name: "playwright",
      state: "connected",
      transport: "stdio",
      scope: "user",
    } satisfies RuntimeMcpServer;
    const dynamicConnectorServer = {
      name: "browser-local",
      state: "connected",
      transport: "stdio",
      scope: "user",
      sourceConnectorId: "browser-local",
    } satisfies RuntimeMcpServer;

    expect(isUserInstalledMcpServer(restoredBuiltin, ownedNames)).toBe(false);
    expect(isUserInstalledMcpServer(dynamicConnectorServer, ownedNames)).toBe(false);
  });

  it("keeps standalone user or project MCP servers in the user-installed group", () => {
    const ownedNames = connectorOwnedMcpServerNames([{ ...connector, serverNames: [] }]);
    const standalone = {
      name: "playwright",
      state: "connected",
      transport: "stdio",
      scope: "project",
    } satisfies RuntimeMcpServer;

    expect(isUserInstalledMcpServer(standalone, ownedNames)).toBe(true);
  });

  it("labels plugin skills with the owning plugin name", () => {
    expect(pluginOwnerFromSkillId("plugin__acme-tools__lint")).toBe("acme-tools");
    expect(skillSourceLabel("plugin", "plugin__acme-tools__lint")).toBe("插件 · acme-tools");
    expect(skillSourceLabel("plugin")).toBe("插件");
  });

  it("keeps plugin-owned MCP servers out of the user-installed group", () => {
    const ownedNames = connectorOwnedMcpServerNames([]);
    const pluginServer = {
      name: "plugin__acme.tools__validator",
      state: "connected",
      transport: "stdio",
      scope: "plugin",
    } satisfies RuntimeMcpServer;
    expect(isUserInstalledMcpServer(pluginServer, ownedNames)).toBe(false);
  });

  it("resolves the single owned server used by an installed connector card", () => {
    const server = {
      name: "playwright",
      state: "disabled",
      transport: "stdio",
      scope: "user",
    } satisfies RuntimeMcpServer;

    expect(findConnectorOwnedMcpServer(connector, [server])).toBe(server);
    expect(
      findConnectorOwnedMcpServer(
        { serverNames: ["playwright", "playwright-helper"] },
        [server],
      ),
    ).toBeUndefined();
  });

  it("optimistically toggles a user MCP card without removing it from the list", () => {
    const servers = [
      {
        name: "ai",
        state: "connected",
        transport: "http",
        scope: "user",
      },
      {
        name: "keep",
        state: "connected",
        transport: "stdio",
        scope: "project",
      },
    ] satisfies RuntimeMcpServer[];

    const disabled = withMcpServerToggleState(servers, "ai", false);
    expect(disabled).toHaveLength(2);
    expect(disabled[0]).toMatchObject({ name: "ai", state: "disabled", errorReason: "" });
    expect(disabled[1]).toEqual(servers[1]);

    const enabled = withMcpServerToggleState(disabled, "ai", true);
    expect(enabled[0]).toMatchObject({ name: "ai", state: "pending", errorReason: "" });
  });
});

describe("loadSkillListsForRoots", () => {
  it("keeps loading other projects when one project root is invalid", async () => {
    const projectSkill: RuntimeSkill = {
      id: "project-skill",
      name: "project-skill",
      description: "Project skill",
      state: "loaded",
      sourceOrigin: "project",
      context: "inline",
    };
    const listSkills = vi.fn(async (root?: string) => {
      if (root === "/tmp/missing-workspace") {
        throw new Error("INVALID_PROJECT_ROOT");
      }
      return root === "/workspace" ? [projectSkill] : [];
    });

    await expect(
      loadSkillListsForRoots(["/workspace", "/tmp/missing-workspace"], listSkills),
    ).resolves.toEqual([
      {
        root: "/workspace",
        skills: [projectSkill],
      },
      {
        root: "/tmp/missing-workspace",
        skills: [],
        errorMessage: "INVALID_PROJECT_ROOT",
      },
    ]);
    expect(listSkills).toHaveBeenCalledTimes(2);
  });

  it("formats structured and plain Skill diagnostics for the management UI", () => {
    expect(skillDiagnosticText("frontmatter is invalid")).toBe(
      "frontmatter is invalid",
    );
    expect(
      skillDiagnosticText({
        code: "parse_failed",
        message: "frontmatter is invalid",
        severity: "error",
      }),
    ).toBe("parse_failed: frontmatter is invalid");
  });
});

describe("loadSkillListsForProjects", () => {
  it("requests primary-first folders as one project Catalog and keeps folder-less projects", async () => {
    const primary = "/workspace/primary";
    const secondary = "/workspace/secondary";
    const projects = [
      {
        projectId: "project-multi",
        workspaceRoot: primary,
        folders: [
          { path: secondary, role: "user" as const, isPrimary: false },
          { path: primary, role: "user" as const, isPrimary: true },
        ],
        displayName: "Multi",
        state: "active" as const,
        createdAt: "2026-07-27T00:00:00Z",
        updatedAt: "2026-07-27T00:00:00Z",
        version: 1,
      },
      {
        projectId: "project-empty",
        workspaceRoot: "",
        folders: [],
        displayName: "Empty",
        state: "active" as const,
        createdAt: "2026-07-27T00:00:00Z",
        updatedAt: "2026-07-27T00:00:00Z",
        version: 1,
      },
    ];
    const skill = {
      id: "secondary",
      name: "secondary",
      description: "Secondary",
      state: "loaded" as const,
      sourceOrigin: "project",
      context: "inline" as const,
    };
    const listSkills = vi.fn(async () => [skill]);

    await expect(loadSkillListsForProjects(projects, listSkills)).resolves.toEqual([
      { project: projects[0], skills: [skill] },
      { project: projects[1], skills: [] },
    ]);
    expect(listSkills).toHaveBeenCalledTimes(1);
    expect(listSkills).toHaveBeenCalledWith([primary, secondary]);
  });
});

describe("loadMcpSettingsResources", () => {
  it("keeps tools available and reports the config error independently", async () => {
    const tool: RuntimeToolInfo = {
      name: "mcp__playwright__browser_click",
      source: "mcp",
      description: "Click an element",
      readOnly: false,
    };
    const runtimeClient = {
      getMcpServerConfig: vi.fn(async () => {
        throw new Error("WORKSPACE_UNAVAILABLE");
      }),
      listMcpTools: vi.fn(async () => [tool]),
      getRuntimeLogs: vi.fn(async () => []),
    };

    await expect(
      loadMcpSettingsResources("playwright", runtimeClient),
    ).resolves.toEqual({
      config: undefined,
      configErrorMessage: "WORKSPACE_UNAVAILABLE",
      tools: [tool],
      logs: [],
    });
  });
});

describe("canPreviewSkill", () => {
  it("requires an opaque local catalog entry identity and a SKILL.md path", () => {
    expect(
      canPreviewSkill({
        catalogEntryId: "opaque-id",
        skillFilePath: "/workspace/.awaken/skills/demo/SKILL.md",
        sourceOrigin: "project",
      }),
    ).toBe(true);
    expect(
      canPreviewSkill({
        catalogEntryId: "opaque-id",
        skillFilePath: "/app/system-skills/excel/SKILL.md",
        sourceOrigin: "bundled",
      }),
    ).toBe(true);
    expect(
      canPreviewSkill({
        catalogEntryId: "opaque-id",
        skillFilePath: undefined,
        sourceOrigin: "mcp",
      }),
    ).toBe(false);
    expect(
      canPreviewSkill({
        catalogEntryId: undefined,
        skillFilePath: "/tmp/SKILL.md",
        sourceOrigin: "user",
      }),
    ).toBe(false);
  });
});

describe("Skill views", () => {
  it("defaults to Awaken and keeps the locked tab order", () => {
    expect(DEFAULT_SKILL_VIEW).toBe("awaken");
    expect(SKILL_VIEW_TABS).toEqual([
      { id: "awaken", label: "Awaken Skills" },
      { id: "project", label: "项目 Skills" },
      { id: "user", label: "全局 Skills" },
      { id: "agents", label: "Agents Skills" },
    ]);
  });

  it("routes bundled Skills only to Awaken without changing other source views", () => {
    expect(isSkillVisibleInView({ sourceOrigin: "bundled" }, "awaken")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "bundled" }, "user")).toBe(false);
    expect(isSkillVisibleInView({ sourceOrigin: "project" }, "project")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "user" }, "user")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "mcp" }, "user")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "plugin" }, "user")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "agents" }, "agents")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "agents" }, "user")).toBe(false);
  });

});

describe("dedupeSkills", () => {
  it("merges skills from multiple project roots and dedupes by id, source and root", () => {
    const globalUserSkill = {
      id: "user-skill",
      name: "user-skill",
      description: "global",
      state: "loaded" as const,
      sourceOrigin: "user",
      context: "inline" as const,
      rootDir: "/home/.awaken/skills/user-skill",
    };
    const projectAgentA = {
      id: "agent-skill",
      name: "agent-skill",
      description: "project a",
      state: "loaded" as const,
      sourceOrigin: "agents",
      context: "inline" as const,
      rootDir: "/project-a/.agents/skills/agent-skill",
    };
    const projectAgentB = {
      id: "agent-skill",
      name: "agent-skill",
      description: "project b",
      state: "loaded" as const,
      sourceOrigin: "agents",
      context: "inline" as const,
      rootDir: "/project-b/.agents/skills/agent-skill",
    };
    expect(dedupeSkills([globalUserSkill, projectAgentA, projectAgentB])).toEqual([
      globalUserSkill,
      projectAgentA,
      projectAgentB,
    ]);
  });

  it("dedupes the same global skill returned from multiple project roots", () => {
    const skill = {
      id: "shared",
      name: "shared",
      description: "global",
      state: "loaded" as const,
      sourceOrigin: "user",
      context: "inline" as const,
      rootDir: "/home/.awaken/skills/shared",
    };
    expect(dedupeSkills([skill, skill])).toEqual([skill]);
  });
});
