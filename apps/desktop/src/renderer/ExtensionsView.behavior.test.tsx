// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeSkill } from "../shared/runtime";
import { ExtensionsView } from "./ExtensionsView";
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
    const listSkills = vi.fn(async () => (deleted ? [] : [projectSkill]));
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

    await act(async () => buttonByText("项目 Skills")?.click());
    const folder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开marketing 项目 Skills"]',
    );
    expect(folder).not.toBeNull();
    expect(folder?.dataset.size).toBe("dense");
    expect(document.querySelector(".skill-project-panel")).toBeNull();

    await act(async () => folder?.click());
    const projectDialog = document.querySelector('[role="dialog"]');
    expect(projectDialog?.textContent).toContain("marketing");
    expect(projectDialog?.textContent).toContain("项目报告");
    expect(
      projectDialog?.querySelector(".aw-project-skills-dialog__heading strong"),
    ).toBeNull();
    expect(
      projectDialog?.querySelector(".aw-project-skills-dialog__path")?.textContent,
    ).toBe(projectRoot);

    const previewButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 项目报告 的 SKILL.md"]',
    );
    await act(async () => previewButton?.click());
    await settle();
    expect(getSkillPreview).toHaveBeenCalledWith({
      catalogEntryId: "catalog-project-report",
      projectRoot,
    });
    expect(document.body.textContent).toContain("项目报告正文");

    const previewClose = document.querySelector<HTMLButtonElement>(
      '.skill-preview-dialog button[aria-label="关闭"]',
    );
    await act(async () => previewClose?.click());
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain("项目报告");

    const deleteButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="删除 项目报告"]',
    );
    await act(async () => deleteButton?.click());
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
});
