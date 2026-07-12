import { describe, expect, it, vi } from "vitest";
import type { RuntimeSkill } from "../shared/runtime";
import { loadSkillListsForRoots, skillDiagnosticText } from "./ExtensionsView";

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
