// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeSkill } from "../shared/runtime";
import {
  ExtensionsView,
  SKILL_CATALOG_SYNC_INTERVAL_MS,
} from "./ExtensionsView";
import type { RuntimeClient } from "./runtime/client";

const projectRoot = "/workspace/marketing";

const projectSkill: RuntimeSkill = {
  id: "project-report",
  catalogEntryId: "catalog-project-report",
  name: "project-report",
  displayName: "项目报告",
  description: "生成项目报告",
  state: "loaded",
  sourceOrigin: "project",
  rootDir: `${projectRoot}/.awaken/skills/project-report`,
  skillFilePath: `${projectRoot}/.awaken/skills/project-report/SKILL.md`,
  context: "inline",
};

const bundledSkill: RuntimeSkill = {
  ...projectSkill,
  id: "bundled-report",
  catalogEntryId: "catalog-bundled-report",
  name: "bundled-report",
  displayName: "内置报告",
  sourceOrigin: "bundled",
  rootDir: "/runtime/system-skills/bundled-report",
  skillFilePath: "/runtime/system-skills/bundled-report/SKILL.md",
};

const globalAgentsSkill: RuntimeSkill = {
  ...projectSkill,
  id: "agents-global-dws",
  catalogEntryId: "catalog-agents-global-dws",
  name: "dws",
  displayName: "DWS",
  description: "管理钉钉产品能力",
  sourceOrigin: "agents",
  rootDir: "/Users/test/.agents/skills/dws",
  skillFilePath: "/Users/test/.agents/skills/dws/SKILL.md",
};

const projectAgentsSkill: RuntimeSkill = {
  ...projectSkill,
  id: "agents-project-review",
  catalogEntryId: "catalog-agents-project-review",
  name: "project-review",
  displayName: "项目代码复核",
  description: "按项目规范执行变更复核",
  sourceOrigin: "agents",
  rootDir: `${projectRoot}/.agents/skills/project-review`,
  skillFilePath: `${projectRoot}/.agents/skills/project-review/SKILL.md`,
};

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function buttonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.trim() === text,
  );
}

describe("ExtensionsView project Skills flow", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: vi.fn(),
    });
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("opens a project window, previews SKILL.md, and deletes through confirmation", async () => {
    let deleted = false;
    const listSkills = vi.fn(async () =>
      deleted ? [bundledSkill] : [projectSkill, bundledSkill],
    );
    const getSkillPreview = vi.fn(async () => ({
      frontmatterRaw: "---\nname: project-report\n---",
      bodyMarkdown: "# 项目报告正文",
    }));
    const deleteSkill = vi.fn(async () => {
      deleted = true;
      return {
        id: projectSkill.id,
        sourceOrigin: "project",
        originalPath: projectSkill.rootDir!,
        trashedPath: "/Users/test/.Trash/project-report",
        message: "deleted",
      };
    });
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => []),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills,
      getSkillPreview,
      deleteSkill,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    const bundledCard = document.querySelector<HTMLElement>(".aw-skill-card");
    expect(bundledCard?.textContent).toContain("内置报告");
    expect(bundledCard?.querySelector(".aw-switch")?.getAttribute("data-size")).toBe(
      "compact",
    );
    expect(bundledCard?.querySelector(".aw-skill-card__title-row")?.textContent).toContain(
      "内置报告Awaken 内置",
    );
    expect(bundledCard?.textContent).not.toContain("可预览 SKILL.md");
    expect(bundledCard?.querySelector('[aria-label^="删除"]')).toBeNull();
    expect(document.querySelector(".extension-skill-card")).toBeNull();

    await act(async () => buttonByText("项目 Skills")?.click());
    const folder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开marketing 项目 Skills"]',
    );
    expect(folder).not.toBeNull();
    expect(folder?.dataset.size).toBe("dense");
    expect(folder?.getAttribute("title")).toBeNull();
    expect(document.querySelector(".skill-project-panel")).toBeNull();

    await act(async () => folder?.click());
    const projectDialog = document.querySelector('[role="dialog"]');
    expect(projectDialog?.textContent).toContain("marketing");
    expect(projectDialog?.textContent).toContain("项目报告");
    expect(
      projectDialog?.querySelector(".aw-skill-card .aw-switch")?.getAttribute("data-size"),
    ).toBe("compact");
    expect(
      projectDialog?.querySelector(".aw-project-skills-dialog__heading strong"),
    ).toBeNull();
    expect(
      projectDialog?.querySelector(".aw-project-skills-dialog__path")?.textContent,
    ).toBe(projectRoot);
    const projectBackdrop = projectDialog?.closest(".aw-dialog-backdrop");

    const previewButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 项目报告 的 SKILL.md"]',
    );
    await act(async () => previewButton?.click());
    await settle();
    const previewDialog = document.querySelector<HTMLElement>(
      '.aw-project-skills-dialog--preview[role="dialog"]',
    );
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(previewDialog).toBe(projectDialog);
    expect(previewDialog?.closest(".aw-dialog-backdrop")).toBe(projectBackdrop);
    expect(document.querySelector(".aw-skill-preview")).toBeNull();
    expect(getSkillPreview).toHaveBeenCalledWith({
      catalogEntryId: "catalog-project-report",
      projectRoot,
    });
    expect(document.body.textContent).toContain("项目报告正文");
    expect(previewDialog?.querySelector(".aw-skill-preview-summary")?.textContent).toContain(
      "项目报告项目",
    );
    expect(previewDialog?.querySelector(".aw-skill-preview__delete")).not.toBeNull();

    const previewClose = document.querySelector<HTMLButtonElement>(
      '.aw-project-skills-dialog--preview button[aria-label="关闭"]',
    );
    await act(async () => previewClose?.click());
    const returnedProjectDialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(returnedProjectDialog).toBe(projectDialog);
    expect(returnedProjectDialog?.closest(".aw-dialog-backdrop")).toBe(projectBackdrop);
    expect(returnedProjectDialog?.textContent).toContain("项目报告");
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "查看 项目报告 的 SKILL.md",
    );

    expect(document.querySelector('[aria-label="删除 项目报告"]')).toBeNull();
    const reopenedPreviewButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 项目报告 的 SKILL.md"]',
    );
    await act(async () => reopenedPreviewButton?.click());
    await settle();
    const deleteButton = document.querySelector<HTMLButtonElement>(
      ".aw-skill-preview__delete",
    );
    await act(async () => deleteButton?.click());
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(document.querySelector('[role="dialog"]')).toBe(projectDialog);
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "确定要删除 Skill",
    );

    await act(async () => buttonByText("确认删除")?.click());
    await settle();
    expect(deleteSkill).toHaveBeenCalledWith({
      id: "project-report",
      sourceOrigin: "project",
      projectRoot,
    });
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "这个项目还没有 Skills",
    );
  });

  it("shows all Agents Skills directly without collapsible source groups", async () => {
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => []),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => [globalAgentsSkill, projectAgentsSkill]),
      getSkillPreview: vi.fn(async () => ({
        frontmatterRaw: "---\nname: project-review\n---",
        bodyMarkdown: "# 项目代码复核",
      })),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    await act(async () => buttonByText("Agents Skills")?.click());
    const agentsSection = document.querySelector<HTMLElement>(
      'section[aria-label="Agents Skills"]',
    );
    const cards = agentsSection?.querySelectorAll(".aw-skill-card");

    expect(cards).toHaveLength(2);
    expect(agentsSection?.textContent).toContain("DWS");
    expect(agentsSection?.textContent).toContain("项目代码复核");
    expect(agentsSection?.querySelector("[aria-expanded]")).toBeNull();
    expect(agentsSection?.querySelector(".skill-project-group")).toBeNull();
    expect(agentsSection?.textContent).not.toContain("全局 Agents Skills");
    expect(agentsSection?.textContent).not.toContain(projectRoot);
  });

  it("syncs an external project Skill deletion and closes its stale preview", async () => {
    let skillExistsOnDisk = true;
    const listSkills = vi.fn(async () =>
      skillExistsOnDisk ? [projectSkill] : [],
    );
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => []),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills,
      getSkillPreview: vi.fn(async () => ({
        frontmatterRaw: "---\nname: project-report\n---",
        bodyMarkdown: "# 项目报告正文",
      })),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    await act(async () => buttonByText("项目 Skills")?.click());
    const folder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开marketing 项目 Skills"]',
    );
    await act(async () => folder?.click());
    const projectDialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const previewButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 项目报告 的 SKILL.md"]',
    );
    await act(async () => previewButton?.click());
    await settle();
    expect(projectDialog?.textContent).toContain("项目报告正文");

    skillExistsOnDisk = false;
    await act(async () => {
      await new Promise((resolve) =>
        setTimeout(resolve, SKILL_CATALOG_SYNC_INTERVAL_MS + 100),
      );
    });
    await settle();

    expect(listSkills.mock.calls.length).toBeGreaterThan(1);
    expect(document.querySelector('[role="dialog"]')).toBe(projectDialog);
    expect(projectDialog?.textContent).not.toContain("项目报告正文");
    expect(projectDialog?.textContent).toContain("这个项目还没有 Skills");
  });
});
