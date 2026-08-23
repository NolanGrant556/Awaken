// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeProject, RuntimeSkill } from "../shared/runtime";
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
  projectPath: projectRoot,
  context: "inline",
};

const runtimeProject: RuntimeProject = {
  projectId: "project-marketing",
  workspaceRoot: projectRoot,
  folders: [{ path: projectRoot, role: "user", isPrimary: true }],
  displayName: "营销项目",
  state: "active",
  createdAt: "2026-07-27T00:00:00Z",
  updatedAt: "2026-07-27T00:00:00Z",
  version: 1,
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

describe("ExtensionsView behavior", () => {
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

  it("clears visible loading after a silent refresh supersedes its data generation", async () => {
    let resolveInitialRuntimeInfo: ((value: { defaultWorkspace: string }) => void) | undefined;
    const initialRuntimeInfo = new Promise<{ defaultWorkspace: string }>((resolve) => {
      resolveInitialRuntimeInfo = resolve;
    });
    const getRuntimeInfo = vi
      .fn<() => Promise<{ defaultWorkspace: string }>>()
      .mockImplementationOnce(() => initialRuntimeInfo)
      .mockResolvedValue({ defaultWorkspace: projectRoot });
    const runtimeClient = {
      getRuntimeInfo,
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="connectors"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    const refreshButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="刷新"]',
    );
    expect(getRuntimeInfo).toHaveBeenCalledTimes(2);
    expect(refreshButton?.disabled).toBe(true);

    await act(async () => {
      resolveInitialRuntimeInfo?.({ defaultWorkspace: projectRoot });
      await initialRuntimeInfo;
    });
    await settle();

    expect(refreshButton?.disabled).toBe(false);
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
      listProjects: vi.fn(async () => [runtimeProject]),
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
      '[aria-label="打开营销项目 项目 Skills"]',
    );
    expect(folder).not.toBeNull();
    expect(folder?.dataset.size).toBe("dense");
    expect(folder?.getAttribute("title")).toBeNull();
    expect(document.querySelector(".skill-project-panel")).toBeNull();

    await act(async () => folder?.click());
    const projectDialog = document.querySelector('[role="dialog"]');
    expect(projectDialog?.textContent).toContain("营销项目");
    expect(projectDialog?.textContent).toContain("项目报告");
    expect(
      projectDialog?.querySelector(".aw-skill-card .aw-switch")?.getAttribute("data-size"),
    ).toBe("compact");
    expect(
      projectDialog?.querySelector(".aw-project-skills-dialog__heading strong"),
    ).toBeNull();
    const folderSummary = projectDialog?.querySelector<HTMLButtonElement>(
      '[aria-label="筛选 1 个项目文件夹"]',
    );
    expect(folderSummary?.textContent).toContain("1 个项目文件夹");
    expect(folderSummary?.textContent).toContain("主：marketing");
    expect(projectDialog?.textContent).not.toContain(projectRoot);
    await act(async () => folderSummary?.click());
    const folderPopover = document.querySelector<HTMLElement>(
      ".aw-project-skills-dialog__folder-popover",
    );
    expect(folderPopover?.textContent).toContain("marketing");
    expect(folderPopover?.textContent).not.toContain(projectRoot);
    expect(folderPopover?.querySelector(".aw-project-skills-dialog__path")).toBeNull();
    await act(async () => folderSummary?.click());
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

  it("shows project entities immediately and aggregates Skills from every attached folder", async () => {
    const secondaryRoot = "/workspace/shared-data";
    const emptyRoot = "/workspace/legal";
    const secondarySkill: RuntimeSkill = {
      ...projectSkill,
      id: "secondary-analysis",
      catalogEntryId: "catalog-secondary-analysis",
      name: "secondary-analysis",
      displayName: "次文件夹分析",
      description: "读取次文件夹数据",
      rootDir: `${secondaryRoot}/.awaken/skills/secondary-analysis`,
      skillFilePath: `${secondaryRoot}/.awaken/skills/secondary-analysis/SKILL.md`,
      projectPath: secondaryRoot,
    };
    const multiFolderProject: RuntimeProject = {
      ...runtimeProject,
      projectId: "project-quarterly",
      displayName: "季度经营分析",
      folders: [
        { path: projectRoot, role: "user", isPrimary: true },
        { path: secondaryRoot, role: "user", isPrimary: false },
        { path: emptyRoot, role: "user", isPrimary: false },
      ],
    };
    const folderlessProject: RuntimeProject = {
      ...runtimeProject,
      projectId: "project-empty",
      workspaceRoot: "",
      folders: [],
      displayName: "待规划项目",
    };
    const listSkills = vi.fn(async () => [projectSkill, secondarySkill]);
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [multiFolderProject, folderlessProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills,
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

    expect(listSkills).toHaveBeenCalledWith([projectRoot, secondaryRoot, emptyRoot]);
    await act(async () => buttonByText("项目 Skills")?.click());
    const projectFolder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开季度经营分析 项目 Skills"]',
    );
    const emptyProjectFolder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开待规划项目 项目 Skills"]',
    );
    expect(projectFolder?.textContent).toContain("2 Skill");
    expect(emptyProjectFolder?.textContent).toContain("0 Skill");

    await act(async () => projectFolder?.click());
    const projectDialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(projectDialog?.textContent).toContain("季度经营分析 Skills");
    const folderSummary = projectDialog?.querySelector<HTMLButtonElement>(
      '[aria-label="筛选 3 个项目文件夹"]',
    );
    expect(folderSummary?.textContent).toContain("3 个项目文件夹");
    expect(folderSummary?.textContent).toContain("主：marketing");
    expect(projectDialog?.textContent).not.toContain(projectRoot);
    expect(projectDialog?.textContent).not.toContain(secondaryRoot);
    expect(projectDialog?.textContent).toContain("项目报告marketing");
    expect(projectDialog?.textContent).toContain("次文件夹分析shared-data");
    expect(projectDialog?.textContent).not.toContain("主文件夹 · marketing");
    expect(projectDialog?.textContent).not.toContain("文件夹 · shared-data");

    await act(async () => folderSummary?.click());
    const folderPopover = document.querySelector<HTMLElement>(
      ".aw-project-skills-dialog__folder-popover",
    );
    const folderPopoverHeading = folderPopover?.querySelector(
      ".aw-project-skills-dialog__folder-popover-heading",
    );
    expect(folderPopoverHeading?.firstElementChild?.textContent).toBe("项目文件夹");
    expect(
      folderPopoverHeading?.querySelector(
        ".aw-project-skills-dialog__folder-popover-count",
      )?.textContent,
    ).toBe("3");
    expect(folderPopover?.textContent).toContain("marketing主文件夹");
    expect(folderPopover?.textContent).toContain("shared-data");
    expect(folderPopover?.textContent).toContain("legal");
    expect(folderPopover?.textContent).not.toContain(projectRoot);
    expect(folderPopover?.textContent).not.toContain(secondaryRoot);
    expect(folderPopover?.textContent).not.toContain(emptyRoot);

    const emptyFolderOption = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 legal 的 Skills"]',
    );
    expect(emptyFolderOption?.disabled).toBe(false);
    await act(async () => emptyFolderOption?.click());
    expect(folderSummary?.textContent).toContain("当前：legal");
    expect(projectDialog?.textContent).toContain("0 Skills");
    expect(projectDialog?.textContent).toContain("这个文件夹还没有 Skills");
    expect(projectDialog?.textContent).not.toContain("项目报告marketing");
    expect(projectDialog?.textContent).not.toContain("次文件夹分析shared-data");

    await act(async () => folderSummary?.click());
    const allFoldersOption = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看全部文件夹的 Skills"]',
    );
    await act(async () => allFoldersOption?.click());
    expect(folderSummary?.textContent).toContain("主：marketing");
    expect(projectDialog?.textContent).toContain("2 Skills");
    expect(projectDialog?.textContent).toContain("项目报告marketing");
    expect(projectDialog?.textContent).toContain("次文件夹分析shared-data");
  });

  it("shows all Agents Skills directly without collapsible source groups", async () => {
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => [globalAgentsSkill, projectAgentsSkill]),
      listPlugins: vi.fn(async () => []),
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
      listProjects: vi.fn(async () => [runtimeProject]),
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
      '[aria-label="打开营销项目 项目 Skills"]',
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
