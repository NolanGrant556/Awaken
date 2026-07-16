import { describe, expect, it, vi } from "vitest";
import type { RuntimeSkill, RuntimeToolInfo } from "../shared/runtime";
import {
  DEFAULT_SKILL_VIEW,
  SKILL_VIEW_TABS,
  canPreviewSkill,
  isSkillVisibleInView,
  loadMcpSettingsResources,
  loadSkillListsForRoots,
  isSkillPackageDeletable,
  shouldRenderSkillDeleteAction,
  skillDiagnosticText,
  skillSourceLabel,
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
    expect(isSkillPackageDeletable({ sourceOrigin: "bundled" })).toBe(false);
  });

  it("hides the delete action only for product-owned bundled Skills", () => {
    expect(shouldRenderSkillDeleteAction({ sourceOrigin: "bundled" })).toBe(false);
    expect(shouldRenderSkillDeleteAction({ sourceOrigin: "mcp" })).toBe(true);
    expect(shouldRenderSkillDeleteAction({ sourceOrigin: "plugin" })).toBe(true);
    expect(shouldRenderSkillDeleteAction({ sourceOrigin: "project" })).toBe(true);
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
    ).toBe(false);
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
    ]);
  });

  it("routes bundled Skills only to Awaken without changing other source views", () => {
    expect(isSkillVisibleInView({ sourceOrigin: "bundled" }, "awaken")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "bundled" }, "user")).toBe(false);
    expect(isSkillVisibleInView({ sourceOrigin: "project" }, "project")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "user" }, "user")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "mcp" }, "user")).toBe(true);
    expect(isSkillVisibleInView({ sourceOrigin: "plugin" }, "user")).toBe(true);
  });

  it("labels bundled cards as Awaken built-ins", () => {
    expect(skillSourceLabel("bundled")).toBe("Awaken 内置");
  });
});
