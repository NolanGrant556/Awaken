import { readStyleSource } from "./style-test-source";
// @vitest-environment jsdom

import { resolve } from "node:path";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ProjectFilesWorkspace,
  projectFilesMenuViewportOffset,
  projectFilesSubmenuPlacement,
} from "./ProjectFilesWorkspace";

const projectFileStyles = readStyleSource(resolve(process.cwd(), "src/renderer/styles/project-files.css"));

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function settleUntil(isReady: () => boolean): Promise<void> {
  await act(async () => {
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 10));
      if (isReady()) return;
    }
  });
}

function dispatchTimedPointerEvent(
  target: EventTarget,
  type: "pointerdown" | "pointermove" | "pointerup",
  clientX: number,
  timeStamp: number,
): void {
  const event = new MouseEvent(type, { bubbles: true, clientX });
  Object.defineProperty(event, "timeStamp", { value: timeStamp });
  target.dispatchEvent(event);
}

function dispatchPreviewPointerEvent(
  target: EventTarget,
  type: "pointerdown" | "pointermove" | "pointerup",
  options: { clientX: number; clientY?: number; pointerId?: number },
): void {
  const event = new MouseEvent(type, {
    bubbles: true,
    button: 0,
    clientX: options.clientX,
    clientY: options.clientY ?? 0,
  });
  Object.defineProperty(event, "pointerId", {
    value: options.pointerId ?? 1,
  });
  target.dispatchEvent(event);
}

function previewRuntimeClient() {
  return {
    watchProjectDirectory: vi.fn(async () => ({
      ok: true as const,
      subscriptionId: "project-files-watch-1",
    })),
    unwatchProjectDirectory: vi.fn(async () => undefined),
    onProjectDirectoryChanged: vi.fn(() => () => {}),
    copyProjectEntryPath: vi.fn(async () => ({ ok: true as const })),
    revealProjectEntry: vi.fn(async () => ({ ok: true as const })),
    openProjectEntryWith: vi.fn(async () => ({ ok: true as const })),
    startArtifactPreview: vi.fn(async () => ({
      ok: false as const,
      code: "UNSUPPORTED_FILE_TYPE" as const,
      message: "unsupported",
    })),
    startProjectFilePreview: vi.fn(async () => ({
      ok: false as const,
      code: "UNSUPPORTED_FILE_TYPE" as const,
      message: "unsupported",
    })),
    closeProjectFilePreview: vi.fn(async () => undefined),
    openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
    revealProjectFile: vi.fn(async () => ({ ok: true as const })),
    saveProjectMarkdownFile: vi.fn(async () => ({
      ok: true as const,
      byteLength: 0,
    })),
    openArtifact: vi.fn(async () => ({ ok: true as const })),
    revealArtifact: vi.fn(async () => ({ ok: true as const })),
  };
}

describe("project file menu viewport placement", () => {
  it("moves a measured menu back inside the bottom and right viewport edges", () => {
    expect(
      projectFilesMenuViewportOffset(
        { left: 700, right: 924, top: 470, bottom: 610 },
        { width: 900, height: 600 },
      ),
    ).toEqual({ x: -32, y: -18 });
  });

  it("opens a bottom-right submenu to the left and shifts it upward", () => {
    expect(
      projectFilesSubmenuPlacement(
        { left: 700, right: 910, top: 500 },
        { width: 224, height: 218 },
        { width: 982, height: 569 },
      ),
    ).toEqual({ side: "left", top: -157 });
  });

  it("preserves the preferred right and row-aligned placement when space is available", () => {
    expect(
      projectFilesSubmenuPlacement(
        { left: 100, right: 310, top: 200 },
        { width: 224, height: 218 },
        { width: 1000, height: 800 },
      ),
    ).toEqual({ side: "right", top: -7 });
  });
});

describe("ProjectFilesWorkspace", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  it("switches the full-width file surface without an intermediate width animation", () => {
    expect(projectFileStyles).toMatch(
      /\.aw-workbench-shell:has\(> \.aw-workbench-inspector > \.inspector-surfaces > \.inspector-surface\)\s*\{\s*transition:\s*none;/,
    );
  });

  it("animates the file-list width, content, divider, and folder state with reduced-motion fallback", () => {
    expect(projectFileStyles).toContain(
      "transition: grid-template-columns 210ms cubic-bezier(0.2, 0.8, 0.2, 1);",
    );
    expect(projectFileStyles).toContain("transform: translateX(12px);");
    expect(projectFileStyles).toContain("visibility 0s linear 190ms;");
    expect(projectFileStyles).toContain(
      ".project-files-pathbar__folder-icon-state--expanded",
    );
    expect(projectFileStyles).toMatch(
      /@media \(prefers-reduced-motion: reduce\) \{[\s\S]*?\.project-files-pathbar__folder-icon-state \{\s*transition: none;/,
    );
  });

  it("keeps the file filter compact when the optional AGENTS creator is visible", () => {
    expect(projectFileStyles).toMatch(
      /\.project-files-navigator:has\(> \.project-files-create-agents\)\s*\{\s*grid-template-rows:\s*auto auto auto minmax\(0, 1fr\);/,
    );
    expect(projectFileStyles).toMatch(
      /\.project-files-create-agents\s*\{[^}]*margin:\s*9px 10px 6px;/s,
    );
    expect(projectFileStyles).toMatch(
      /\.project-files-filter\s*\{[^}]*height:\s*36px;[^}]*max-height:\s*36px;/s,
    );
  });

  it("labels the filter as loaded-tree filtering and clears it without changing layout", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [
        { name: "员工花名册.xlsx", relativePath: "员工花名册.xlsx", kind: "file" as const },
        { name: "README.md", relativePath: "README.md", kind: "file" as const },
      ],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),

            listProjectDirectory,
            ...previewRuntimeClient(),
          }}
          projectId="project-1"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
        />,
      );
    });
    await settleUntil(() => container.querySelectorAll('[role="treeitem"]').length === 2);

    const input = container.querySelector<HTMLInputElement>('[aria-label="筛选已加载的文件"]');
    expect(input?.placeholder).toBe("筛选已加载文件…");
    expect(input?.title).toBe("仅筛选当前已经加载的目录内容");
    expect(container.querySelector(".project-files-create-agents")).not.toBeNull();

    await act(async () => {
      if (!input) return;
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
      setter?.call(input, "员工");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(container.querySelectorAll('[role="treeitem"]')).toHaveLength(1);
    const clear = container.querySelector<HTMLButtonElement>('[aria-label="清除文件筛选"]');
    expect(clear).not.toBeNull();

    await act(async () => clear?.click());
    expect(input?.value).toBe("");
    expect(container.querySelectorAll('[role="treeitem"]')).toHaveLength(2);
  });

  it("renders a durable deliverable in the right file workspace", async () => {
    const startArtifactPreview = vi.fn(async () => ({
      ok: true as const,
      preview: {
        previewId: "artifact-preview-1",
        requestId: "artifact-request-1",
        kind: "markdown" as const,
        transport: "text" as const,
        displayName: "delivery.md",
        relativePath: "delivery.md",
        byteLength: 12,
        content: "# Delivery\n",
        language: "markdown",
      },
    }));
    const runtimeClient = {
      ...previewRuntimeClient(),
      createProjectAgentsFile: vi.fn(),
        listProjectDirectory: vi.fn(async () => ({
        ok: true as const,
        relativePath: "",
        entries: [],
      })),
      startArtifactPreview,
    };

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={runtimeClient}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          artifactPreview={{
            threadId: "thread-1",
            runId: "run-1",
            artifactId: "artifact-1",
            title: "交付报告.md",
          }}
        />,
      );
    });
    await settleUntil(
      () => container.querySelector("[data-preview-kind='markdown']") !== null,
    );

    expect(startArtifactPreview).toHaveBeenCalledWith(
      expect.objectContaining({
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    );
    expect(container.querySelector("[data-preview-kind='markdown']")?.textContent).toContain(
      "Delivery",
    );
  });

  it("renders a video Artifact from the media source and keeps its controls interactive", async () => {
    const onPreviewPrimaryClick = vi.fn();
    const startArtifactPreview = vi.fn();
    const runtimeClient = {
      ...previewRuntimeClient(),
      createProjectAgentsFile: vi.fn(),
        listProjectDirectory: vi.fn(async () => ({
        ok: true as const,
        relativePath: "",
        entries: [],
      })),
      startArtifactPreview,
    };

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={runtimeClient}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          artifactPreview={{
            threadId: "thread-1",
            runId: "run-1",
            artifactId: "video-1",
            title: "成片.mp4",
            mediaKind: "video",
          }}
          artifactMediaSource="awaken-artifact://media/video-1/clip.mp4"
          artifactMediaMetadata={{ width: 1920, height: 1080, durationSeconds: 12.4 }}
          onPreviewPrimaryClick={onPreviewPrimaryClick}
        />,
      );
    });

    const video = container.querySelector<HTMLVideoElement>("video");
    expect(video?.controls).toBe(true);
    expect(startArtifactPreview).not.toHaveBeenCalled();

    await act(async () => {
      if (!video) return;
      dispatchPreviewPointerEvent(video, "pointerdown", { clientX: 20 });
      dispatchPreviewPointerEvent(video, "pointerup", { clientX: 20 });
    });
    expect(onPreviewPrimaryClick).not.toHaveBeenCalled();
  });

  it("forwards an Artifact Office guest primary click to the shared composer collapse callback", async () => {
    let interactionHandler:
      | ((interaction: { previewId: string; type: "primary-click" }) => void)
      | undefined;
    const onPreviewPrimaryClick = vi.fn();
    const runtimeClient = {
      ...previewRuntimeClient(),
      createProjectAgentsFile: vi.fn(),
        listProjectDirectory: vi.fn(async () => ({
        ok: true as const,
        relativePath: "",
        entries: [],
      })),
      startArtifactPreview: vi.fn(async (request) => ({
        ok: true as const,
        preview: {
          previewId: "artifact-preview-office",
          requestId: request.requestId,
          kind: "office" as const,
          transport: "url" as const,
          displayName: "交付报告.docx",
          relativePath: "交付报告.docx",
          byteLength: 256,
          src: "awaken-preview://artifact-preview-office/index.html",
          partition: "awaken-preview-artifact-preview-office",
        },
      })),
      onProjectFilePreviewInteraction: vi.fn((handler) => {
        interactionHandler = handler;
        return vi.fn();
      }),
    };

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={runtimeClient}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          artifactPreview={{
            threadId: "thread-1",
            runId: "run-1",
            artifactId: "artifact-1",
            title: "交付报告.docx",
          }}
          onPreviewPrimaryClick={onPreviewPrimaryClick}
        />,
      );
    });
    await settleUntil(() => container.querySelector("webview") !== null);

    expect(runtimeClient.onProjectFilePreviewInteraction).toHaveBeenCalledOnce();
    await act(async () => {
      interactionHandler?.({
        previewId: "another-preview",
        type: "primary-click",
      });
      interactionHandler?.({
        previewId: "artifact-preview-office",
        type: "primary-click",
      });
    });
    expect(onPreviewPrimaryClick).toHaveBeenCalledOnce();
  });

  it("renders the browser empty state without a project or active file tab", async () => {
    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),

        listProjectDirectory: vi.fn(),
            ...previewRuntimeClient(),
          }}
          folders={[]}
          initialMode="browser"
        />,
      );
    });
    await settle();

    expect(container.textContent).toContain("浏览器");
    expect(container.textContent).toContain("预览 Html 文件");
    expect(container.querySelector(".project-files-preview-empty.is-centered")).not.toBeNull();
  });

  it("toggles the complete file list from the pathbar folder button", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [{ name: "README.md", relativePath: "README.md", kind: "file" as const }],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
        />,
      );
    });
    await settle();

    const navigator = container.querySelector<HTMLElement>("#awaken-project-files-navigator");
    const body = container.querySelector<HTMLElement>(".project-files-workspace__body");
    const divider = container.querySelector<HTMLElement>('[aria-label="调整文件列表宽度"]');
    expect(
      container.querySelector(".project-files-preview-empty.is-centered")?.textContent,
    ).toContain("打开文件");
    body!.getBoundingClientRect = () =>
      ({
        left: 0,
        right: 720,
        top: 0,
        bottom: 600,
        width: 720,
        height: 600,
      }) as DOMRect;
    navigator!.getBoundingClientRect = () =>
      ({
        left: 480,
        right: 720,
        top: 0,
        bottom: 600,
        width: 240,
        height: 600,
      }) as DOMRect;
    const toggle = container.querySelector<HTMLButtonElement>('[aria-label="收起文件列表"]');
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(toggle?.classList).toContain("is-expanded");
    expect(
      toggle?.querySelector(
        ".project-files-pathbar__folder-icon-state--collapsed .aw-icon--folder",
      ),
    ).not.toBeNull();
    expect(
      toggle?.querySelector(
        ".project-files-pathbar__folder-icon-state--expanded .aw-icon--folder-open",
      ),
    ).not.toBeNull();
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
    expect(divider?.getAttribute("aria-valuenow")).toBe("240");
    expect(container.textContent).toContain("README.md");

    await act(async () => {
      divider?.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 480 }));
      window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 300 }));
      window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, clientX: 300 }));
    });
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe("400px");

    await act(async () => {
      divider?.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, key: "ArrowRight" }));
    });
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe("388px");

    await act(async () => toggle?.click());
    expect(toggle?.getAttribute("aria-label")).toBe("展开文件列表");
    expect(toggle?.getAttribute("aria-expanded")).toBe("false");
    expect(toggle?.classList).not.toContain("is-expanded");
    expect(navigator?.getAttribute("aria-hidden")).toBe("true");
    expect(divider?.getAttribute("aria-hidden")).toBe("true");
    expect(divider?.getAttribute("tabindex")).toBe("-1");
    expect(container.querySelector(".project-files-workspace__body")?.classList).toContain(
      "is-file-list-collapsed",
    );

    await act(async () => toggle?.click());
    expect(toggle?.getAttribute("aria-label")).toBe("收起文件列表");
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(toggle?.classList).toContain("is-expanded");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
    expect(divider?.getAttribute("aria-hidden")).toBe("false");
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe("388px");
    expect(container.textContent).toContain("README.md");
  });

  it("keeps panel expansion out of the pathbar without replacing the file-list control", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
        />,
      );
    });
    await settle();

    expect(container.querySelector('[aria-label="展开面板"]')).toBeNull();
    expect(container.querySelector('[aria-label="收起文件列表"]')).not.toBeNull();
  });

  it("reports a primary click but preserves drag selection in the document preview", async () => {
    const onPreviewPrimaryClick = vi.fn();
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          onPreviewPrimaryClick={onPreviewPrimaryClick}
        />,
      );
    });
    await settle();

    const preview = container.querySelector<HTMLElement>(".project-files-preview");
    await act(async () => {
      if (!preview) return;
      dispatchPreviewPointerEvent(preview, "pointerdown", { clientX: 20 });
      dispatchPreviewPointerEvent(preview, "pointerup", { clientX: 22 });
    });
    expect(onPreviewPrimaryClick).toHaveBeenCalledTimes(1);

    await act(async () => {
      if (!preview) return;
      dispatchPreviewPointerEvent(preview, "pointerdown", { clientX: 20 });
      dispatchPreviewPointerEvent(preview, "pointermove", { clientX: 60 });
      dispatchPreviewPointerEvent(preview, "pointerup", { clientX: 60 });
    });
    expect(onPreviewPrimaryClick).toHaveBeenCalledTimes(1);

    await act(async () => {
      const button = container.querySelector<HTMLButtonElement>('[aria-label="收起文件列表"]');
      if (!button) return;
      dispatchPreviewPointerEvent(button, "pointerdown", { clientX: 20 });
      dispatchPreviewPointerEvent(button, "pointerup", { clientX: 20 });
    });
    expect(onPreviewPrimaryClick).toHaveBeenCalledTimes(1);
  });

  it("only collapses after a forceful rightward fling from minimum width", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
        />,
      );
    });
    await settle();

    const body = container.querySelector<HTMLElement>(".project-files-workspace__body");
    const navigator = container.querySelector<HTMLElement>("#awaken-project-files-navigator");
    const divider = container.querySelector<HTMLElement>('[aria-label="调整文件列表宽度"]');
    body!.getBoundingClientRect = () =>
      ({
        left: 0,
        right: 720,
        top: 0,
        bottom: 600,
        width: 720,
        height: 600,
      }) as DOMRect;
    navigator!.getBoundingClientRect = () =>
      ({
        left: 360,
        right: 720,
        top: 0,
        bottom: 600,
        width: 360,
        height: 600,
      }) as DOMRect;

    await act(async () => {
      dispatchTimedPointerEvent(divider!, "pointerdown", 360, 1_000);
      dispatchTimedPointerEvent(window, "pointermove", 362, 1_500);
      dispatchTimedPointerEvent(window, "pointermove", 430, 1_545);
      dispatchTimedPointerEvent(window, "pointermove", 500, 1_580);
      dispatchTimedPointerEvent(window, "pointerup", 500, 1_590);
    });

    const toggle = container.querySelector<HTMLButtonElement>('[aria-label="收起文件列表"]');
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe("240px");

    await act(async () => {
      dispatchTimedPointerEvent(divider!, "pointerdown", 480, 2_000);
      dispatchTimedPointerEvent(window, "pointermove", 530, 2_040);
      dispatchTimedPointerEvent(window, "pointermove", 570, 2_080);
      dispatchTimedPointerEvent(window, "pointerup", 570, 2_090);
    });
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");

    await act(async () => {
      dispatchTimedPointerEvent(divider!, "pointerdown", 480, 3_000);
      dispatchTimedPointerEvent(window, "pointermove", 540, 3_035);
      dispatchTimedPointerEvent(window, "pointermove", 610, 3_070);
    });
    expect(toggle?.getAttribute("aria-expanded")).toBe("false");
    expect(navigator?.getAttribute("aria-hidden")).toBe("true");

    await act(async () => toggle?.click());
    await act(async () => {
      dispatchTimedPointerEvent(divider!, "pointerdown", 480, 4_000);
      dispatchTimedPointerEvent(window, "pointermove", 550, 4_220);
      dispatchTimedPointerEvent(window, "pointermove", 620, 4_500);
      dispatchTimedPointerEvent(window, "pointerup", 620, 4_520);
    });
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("loads the primary folder, expands directories, and switches folder groups", async () => {
    const listProjectDirectory = vi.fn(
      async (request: { folderPath: string; relativePath?: string }) => {
        if (request.folderPath === "/workspace/docs") {
          return {
            ok: true as const,
            relativePath: request.relativePath ?? "",
            entries: [
              {
                name: "brief.md",
                relativePath: "brief.md",
                kind: "file" as const,
              },
            ],
          };
        }
        if (request.relativePath === "src") {
          return {
            ok: true as const,
            relativePath: "src",
            entries: [
              {
                name: "index.ts",
                relativePath: "src/index.ts",
                kind: "file" as const,
              },
            ],
          };
        }
        return {
          ok: true as const,
          relativePath: "",
          entries: [{ name: "src", relativePath: "src", kind: "directory" as const }],
        };
      },
    );

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[
            { path: "/workspace/app", label: "App", isPrimary: true },
            { path: "/workspace/docs", label: "Docs", isPrimary: false },
          ]}
        />,
      );
    });
    await settle();

    expect(listProjectDirectory).toHaveBeenCalledWith({
      projectId: "project-1",
      folderPath: "/workspace/app",
    });

    const srcRow = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "src",
    );
    expect(srcRow?.querySelector('[data-file-icon="folder"]')).not.toBeNull();
    await act(async () => srcRow?.click());
    await settle();
    expect(container.textContent).toContain("index.ts");
    expect(srcRow?.getAttribute("aria-expanded")).toBe("true");
    expect(srcRow?.querySelector('[data-file-icon="folder-open"]')).not.toBeNull();
    const indexRow = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "index.ts",
    );
    expect(indexRow?.classList).toContain("is-nested");
    expect(indexRow?.querySelector('[data-file-icon="code"]')).not.toBeNull();

    const folderTrigger =
      container.querySelector<HTMLButtonElement>('[aria-label="选择项目文件夹组"]');
    await act(async () => folderTrigger?.click());
    expect(document.querySelector(".project-files-folder-menu")?.textContent).not.toContain(
      "/workspace/",
    );
    const docsOption = Array.from(
      document.querySelectorAll<HTMLButtonElement>('[role="option"]'),
    ).find((button) => button.textContent?.includes("Docs"));
    await act(async () => docsOption?.click());
    await settle();

    expect(listProjectDirectory).toHaveBeenLastCalledWith({
      projectId: "project-1",
      folderPath: "/workspace/docs",
    });
    expect(container.textContent).toContain("brief.md");
  });

  it("refreshes loaded directory snapshots after files are added or deleted", async () => {
    let entries = [
      { name: "old.txt", relativePath: "old.txt", kind: "file" as const },
    ];
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries,
    }));
    let emitDirectoryChanged: ((event: { subscriptionId: string }) => void) | undefined;
    const unwatchProjectDirectory = vi.fn(async () => undefined);
    const runtimeActions = {
createProjectAgentsFile: vi.fn(),

      ...previewRuntimeClient(),
      listProjectDirectory,
      watchProjectDirectory: vi.fn(async () => ({
        ok: true as const,
        subscriptionId: "watch-1",
      })),
      unwatchProjectDirectory,
      onProjectDirectoryChanged: vi.fn(
        (handler: (event: { subscriptionId: string }) => void) => {
          emitDirectoryChanged = handler;
          return () => {};
        },
      ),
    };

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={runtimeActions}
          projectId="project-1"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
        />,
      );
    });
    await settle();
    expect(container.textContent).toContain("old.txt");

    entries = [
      { name: "new.txt", relativePath: "new.txt", kind: "file" as const },
    ];
    await act(async () => {
      emitDirectoryChanged?.({ subscriptionId: "watch-1" });
    });
    await settle();

    expect(container.textContent).toContain("new.txt");
    expect(container.textContent).not.toContain("old.txt");
    expect(listProjectDirectory).toHaveBeenLastCalledWith({
      projectId: "project-1",
      folderPath: "/workspace/app",
    });
  });

  it("refreshes expanded directories without collapsing surviving tree state", async () => {
    let nestedEntries = [
      { name: "old.md", relativePath: "docs/old.md", kind: "file" as const },
    ];
    const listProjectDirectory = vi.fn(
      async (request: { relativePath?: string }) =>
        request.relativePath === "docs"
          ? {
              ok: true as const,
              relativePath: "docs",
              entries: nestedEntries,
            }
          : {
              ok: true as const,
              relativePath: "",
              entries: [
                { name: "docs", relativePath: "docs", kind: "directory" as const },
              ],
            },
    );
    let emitDirectoryChanged: ((event: { subscriptionId: string }) => void) | undefined;
    const runtimeActions = {
createProjectAgentsFile: vi.fn(),

      ...previewRuntimeClient(),
      listProjectDirectory,
      watchProjectDirectory: vi.fn(async () => ({
        ok: true as const,
        subscriptionId: "watch-nested",
      })),
      onProjectDirectoryChanged: vi.fn(
        (handler: (event: { subscriptionId: string }) => void) => {
          emitDirectoryChanged = handler;
          return () => {};
        },
      ),
    };

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={runtimeActions}
          projectId="project-1"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
        />,
      );
    });
    await settle();
    const docsRow = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="treeitem"]'),
    ).find((row) => row.textContent?.includes("docs"));
    await act(async () => docsRow?.click());
    await settle();
    expect(container.textContent).toContain("old.md");

    nestedEntries = [
      { name: "new.md", relativePath: "docs/new.md", kind: "file" as const },
    ];
    await act(async () => {
      emitDirectoryChanged?.({ subscriptionId: "watch-nested" });
    });
    await settle();

    expect(docsRow?.getAttribute("aria-expanded")).toBe("true");
    expect(container.textContent).toContain("new.md");
    expect(container.textContent).not.toContain("old.md");
    expect(listProjectDirectory).toHaveBeenCalledWith({
      projectId: "project-1",
      folderPath: "/workspace/app",
      relativePath: "docs",
    });
  });

  it("reports file selections as project-scoped tabs with the matching mode", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [
        { name: "README.md", relativePath: "README.md", kind: "file" as const },
        {
          name: "index.html",
          relativePath: "site/index.html",
          kind: "file" as const,
        },
      ],
    }));
    const onOpenTab = vi.fn();
    const onWorkspaceModeChange = vi.fn();

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          onOpenTab={onOpenTab}
          onWorkspaceModeChange={onWorkspaceModeChange}
        />,
      );
    });
    await settle();

    const rows = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="treeitem"]'));
    await act(async () => rows.find((row) => row.textContent?.includes("README.md"))?.click());
    await act(async () => rows.find((row) => row.textContent?.includes("index.html"))?.click());

    expect(onOpenTab).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({
        projectId: "project-1",
        folderPath: "/workspace/app",
        name: "README.md",
        relativePath: "README.md",
        mode: "files",
      }),
    );
    expect(onOpenTab).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        projectId: "project-1",
        folderPath: "/workspace/app",
        name: "index.html",
        relativePath: "site/index.html",
        mode: "browser",
      }),
    );
    expect(onWorkspaceModeChange.mock.calls).toEqual([["files"], ["browser"]]);
  });

  it("offers only add, copy, and open-with actions for files", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [{ name: "README.md", relativePath: "README.md", kind: "file" as const }],
    }));
    const runtimeActions = previewRuntimeClient();
    const onAddTaskContext = vi.fn(async () => ({ ok: true as const }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...runtimeActions }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          onAddTaskContext={onAddTaskContext}
        />,
      );
    });
    await settle();

    const row = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="treeitem"]')).find(
      (button) => button.textContent?.includes("README.md"),
    );
    await act(async () => {
      row?.dispatchEvent(
        new MouseEvent("contextmenu", {
          bubbles: true,
          button: 2,
          clientX: 120,
          clientY: 80,
        }),
      );
    });

    const menu = container.querySelector<HTMLElement>(".project-files-context-menu");
    expect(menu?.textContent).toContain("添加任务");
    expect(menu?.textContent).toContain("复制路径");
    expect(menu?.textContent).toContain("打开方式");
    expect(menu?.querySelectorAll('[role="menuitem"]')).toHaveLength(3);

    const openWith = Array.from(
      menu?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [],
    ).find((button) => button.textContent?.includes("打开方式"));
    await act(async () => {
      openWith?.dispatchEvent(new MouseEvent("mouseover", { bubbles: true }));
      openWith?.click();
    });
    const submenu = menu?.querySelector<HTMLElement>(".project-files-context-submenu");
    expect(submenu?.getAttribute("aria-label")).toBe("选择打开方式");
    expect(submenu?.textContent).toContain("本地路径");
    expect(menu?.textContent).toContain("WPS");
    expect(menu?.textContent).toContain("Microsoft Office");
    expect(menu?.textContent).toContain("VS Code");
    expect(menu?.textContent).toContain("Cursor");
    expect(menu?.textContent).toContain("TRAE");
    expect(menu?.textContent).toContain("Xcode");
    const submenuButtons = Array.from(
      menu?.querySelectorAll<HTMLButtonElement>(".project-files-context-submenu [role=menuitem]") ??
        [],
    );
    expect(submenuButtons).toHaveLength(7);
    expect(submenuButtons[0]?.textContent?.trim()).toBe("本地路径");
    expect(submenuButtons[0]?.querySelector("svg.aw-icon--folder-search")).not.toBeNull();
    const applicationButtons = submenuButtons.slice(1);
    expect(applicationButtons).toHaveLength(6);
    expect(
      applicationButtons.every(
        (button) =>
          button.querySelector<HTMLImageElement>(
            ".project-files-context-submenu__application-icon",
          )?.getAttribute("src"),
      ),
    ).toBe(true);

    await act(async () => submenuButtons[0]?.click());
    await settle();
    expect(runtimeActions.revealProjectEntry).toHaveBeenCalledWith({
      projectId: "project-1",
      folderPath: "/workspace/app",
      relativePath: "README.md",
      kind: "file",
    });
    expect(container.querySelector(".project-files-context-status")).toBeNull();
    expect(container.textContent).not.toContain("已打开本地路径");

    await act(async () => {
      row?.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, button: 2 }));
    });
    const reopenedMenu = container.querySelector<HTMLElement>(".project-files-context-menu");
    const reopenedOpenWith = Array.from(
      reopenedMenu?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [],
    ).find((button) => button.textContent?.includes("打开方式"));
    await act(async () => reopenedOpenWith?.click());

    const cursor = Array.from(
      reopenedMenu?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [],
    ).find((button) => button.textContent?.trim() === "Cursor");
    await act(async () => cursor?.click());
    expect(runtimeActions.openProjectEntryWith).toHaveBeenCalledWith({
      projectId: "project-1",
      folderPath: "/workspace/app",
      relativePath: "README.md",
      kind: "file",
      application: "cursor",
    });

    await act(async () => {
      row?.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, button: 2 }));
    });
    const addTask = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".project-files-context-menu [role=menuitem]"),
    ).find((button) => button.textContent?.includes("添加任务"));
    await act(async () => addTask?.click());
    expect(onAddTaskContext).toHaveBeenCalledWith({
      projectId: "project-1",
      folderPath: "/workspace/app",
      relativePath: "README.md",
      kind: "file",
    });
  });

  it("keeps local-path success silent but still reports a Windows reveal failure", async () => {
    const windowsFolder = String.raw`C:\Users\Nolan\Awaken Project`;
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [{ name: "季度报告.pdf", relativePath: "季度报告.pdf", kind: "file" as const }],
    }));
    const runtimeActions = {
      ...previewRuntimeClient(),
      revealProjectEntry: vi.fn(async () => ({
        ok: false as const,
        code: "ENTRY_REVEAL_FAILED" as const,
        message: "暂时无法在文件管理器中显示这个文件。",
      })),
    };

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...runtimeActions }}
          projectId="project-1"
          folders={[{ path: windowsFolder, label: "Awaken Project", isPrimary: true }]}
        />,
      );
    });
    await settle();

    const row = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="treeitem"]')).find(
      (button) => button.textContent?.includes("季度报告.pdf"),
    );
    await act(async () => {
      row?.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, button: 2 }));
    });
    const openWith = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".project-files-context-menu [role=menuitem]"),
    ).find((button) => button.textContent?.includes("打开方式"));
    await act(async () => openWith?.click());
    const localPath = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".project-files-context-submenu [role=menuitem]"),
    ).find((button) => button.textContent?.trim() === "本地路径");
    await act(async () => localPath?.click());
    await settle();

    expect(runtimeActions.revealProjectEntry).toHaveBeenCalledWith({
      projectId: "project-1",
      folderPath: windowsFolder,
      relativePath: "季度报告.pdf",
      kind: "file",
    });
    const alert = container.querySelector<HTMLElement>(".project-files-context-status.is-error");
    expect(alert?.getAttribute("role")).toBe("alert");
    expect(alert?.textContent).toBe("暂时无法在文件管理器中显示这个文件。");
  });

  it("shows an alert when the selected application is not installed", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [{ name: "README.md", relativePath: "README.md", kind: "file" as const }],
    }));
    const runtimeActions = {
      ...previewRuntimeClient(),
      openProjectEntryWith: vi.fn(async () => ({
        ok: false as const,
        code: "APPLICATION_UNAVAILABLE" as const,
        message: "未找到 Cursor，请先安装后重试。",
      })),
    };

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...runtimeActions }}
          projectId="project-1"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
        />,
      );
    });
    await settle();

    const row = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="treeitem"]')).find(
      (button) => button.textContent?.includes("README.md"),
    );
    await act(async () => {
      row?.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, button: 2 }));
    });
    const openWith = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".project-files-context-menu [role=menuitem]"),
    ).find((button) => button.textContent?.includes("打开方式"));
    await act(async () => openWith?.click());
    const cursor = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".project-files-context-submenu [role=menuitem]"),
    ).find((button) => button.textContent?.trim() === "Cursor");
    await act(async () => cursor?.click());
    await settle();

    const alert = container.querySelector<HTMLElement>(".project-files-context-status.is-error");
    expect(alert?.getAttribute("role")).toBe("alert");
    expect(alert?.textContent).toBe("未找到 Cursor，请先安装后重试。");
    expect(alert?.parentElement?.classList).toContain("project-files-preview");
  });

  it("keeps office actions disabled for folders and ignores symbolic links", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [
        { name: "docs", relativePath: "docs", kind: "directory" as const },
        { name: "link", relativePath: "link", kind: "symlink" as const },
      ],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
createProjectAgentsFile: vi.fn(),
 listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          onAddTaskContext={vi.fn(async () => ({ ok: true as const }))}
        />,
      );
    });
    await settle();

    const rows = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="treeitem"]'));
    const docs = rows.find((button) => button.textContent?.includes("docs"));
    await act(async () => {
      docs?.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, button: 2 }));
    });
    const openWith = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".project-files-context-menu [role=menuitem]"),
    ).find((button) => button.textContent?.includes("打开方式"));
    await act(async () => openWith?.click());
    const applicationButtons = Array.from(
      container.querySelectorAll<HTMLButtonElement>(
        ".project-files-context-submenu [role=menuitem]",
      ),
    );
    expect(container.querySelector(".project-files-context-submenu")).not.toBeNull();
    expect(
      applicationButtons.find((button) => button.textContent?.trim() === "WPS Office")?.disabled,
    ).toBe(true);
    expect(
      applicationButtons.find((button) => button.textContent?.trim() === "Microsoft Office")
        ?.disabled,
    ).toBe(true);
    expect(
      applicationButtons.find((button) => button.textContent?.trim() === "Cursor")?.disabled,
    ).toBe(false);

    await act(async () => {
      document.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
    });
    const link = rows.find((button) => button.textContent?.includes("link"));
    await act(async () => {
      link?.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, button: 2 }));
    });
    expect(container.querySelector(".project-files-context-menu")).toBeNull();
  });
});
