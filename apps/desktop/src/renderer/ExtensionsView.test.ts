import { describe, expect, it, vi } from "vitest";
import type { RuntimeSkill, RuntimeToolInfo } from "../shared/runtime";
import {
  loadMcpSettingsResources,
  loadSkillListsForRoots,
  isSkillPackageDeletable,
  skillDiagnosticText,
} from "./ExtensionsView";

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

describe("isSkillPackageDeletable", () => {
  it("only allows project and user .awaken/skills packages", () => {
    expect(isSkillPackageDeletable({ sourceOrigin: "project" })).toBe(true);
    expect(isSkillPackageDeletable({ sourceOrigin: "user" })).toBe(true);
    expect(isSkillPackageDeletable({ sourceOrigin: "mcp" })).toBe(false);
    expect(isSkillPackageDeletable({ sourceOrigin: "plugin" })).toBe(false);
  });
});
