// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectFilesWorkspace } from "./ProjectFilesWorkspace";

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
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
    startProjectFilePreview: vi.fn(async () => ({
      ok: false as const,
      code: "UNSUPPORTED_FILE_TYPE" as const,
      message: "unsupported",
    })),
    closeProjectFilePreview: vi.fn(async () => undefined),
    openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
    revealProjectFile: vi.fn(async () => ({ ok: true as const })),
  };
}

describe("ProjectFilesWorkspace", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT =
      true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  it("renders the browser empty state without a project or active file tab", async () => {
    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{
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
    expect(
      container.querySelector(".project-files-preview-empty.is-centered"),
    ).not.toBeNull();
  });

  it("toggles the complete file list from the pathbar folder button", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [
        { name: "README.md", relativePath: "README.md", kind: "file" as const },
      ],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{ listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[
            { path: "/workspace/app", label: "App", isPrimary: true },
          ]}
        />,
      );
    });
    await settle();

    const navigator = container.querySelector<HTMLElement>(
      "#awaken-project-files-navigator",
    );
    const body = container.querySelector<HTMLElement>(
      ".project-files-workspace__body",
    );
    const divider = container.querySelector<HTMLElement>(
      '[aria-label="调整文件列表宽度"]',
    );
    expect(
      container.querySelector(".project-files-preview-empty.is-centered")?.textContent,
    ).toContain("打开文件");
    body!.getBoundingClientRect = () =>
      ({ left: 0, right: 720, top: 0, bottom: 600, width: 720, height: 600 }) as DOMRect;
    navigator!.getBoundingClientRect = () =>
      ({ left: 480, right: 720, top: 0, bottom: 600, width: 240, height: 600 }) as DOMRect;
    const toggle = container.querySelector<HTMLButtonElement>(
      '[aria-label="收起文件列表"]',
    );
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
    expect(divider?.getAttribute("aria-valuenow")).toBe("240");
    expect(container.textContent).toContain("README.md");

    await act(async () => {
      divider?.dispatchEvent(
        new MouseEvent("pointerdown", { bubbles: true, clientX: 480 }),
      );
      window.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 300 }),
      );
      window.dispatchEvent(
        new MouseEvent("pointerup", { bubbles: true, clientX: 300 }),
      );
    });
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe(
      "400px",
    );

    await act(async () => {
      divider?.dispatchEvent(
        new KeyboardEvent("keydown", { bubbles: true, key: "ArrowRight" }),
      );
    });
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe(
      "388px",
    );

    await act(async () => toggle?.click());
    expect(toggle?.getAttribute("aria-label")).toBe("展开文件列表");
    expect(toggle?.getAttribute("aria-expanded")).toBe("false");
    expect(navigator?.getAttribute("aria-hidden")).toBe("true");
    expect(divider?.getAttribute("aria-hidden")).toBe("true");
    expect(divider?.getAttribute("tabindex")).toBe("-1");
    expect(
      container.querySelector(".project-files-workspace__body")?.classList,
    ).toContain("is-file-list-collapsed");

    await act(async () => toggle?.click());
    expect(toggle?.getAttribute("aria-label")).toBe("收起文件列表");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
    expect(divider?.getAttribute("aria-hidden")).toBe("false");
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe(
      "388px",
    );
    expect(container.textContent).toContain("README.md");
  });

  it("requests the expanded panel from the pathbar without replacing the file-list control", async () => {
    const onExpand = vi.fn();
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{ listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          folders={[
            { path: "/workspace/app", label: "App", isPrimary: true },
          ]}
          onExpand={onExpand}
        />,
      );
    });
    await settle();

    const expand = container.querySelector<HTMLButtonElement>(
      '[aria-label="展开面板"]',
    );
    expect(expand).not.toBeNull();
    expect(
      container.querySelector('[aria-label="收起文件列表"]'),
    ).not.toBeNull();

    await act(async () => expand?.click());
    expect(onExpand).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{ listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          folders={[
            { path: "/workspace/app", label: "App", isPrimary: true },
          ]}
          expanded
          onExpand={onExpand}
        />,
      );
    });
    expect(
      container.querySelector('[aria-label="展开面板"]'),
    ).toBeNull();
    expect(
      container.querySelector('[aria-label="收起文件列表"]'),
    ).not.toBeNull();
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
          runtimeClient={{ listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          folders={[
            { path: "/workspace/app", label: "App", isPrimary: true },
          ]}
          expanded
          onPreviewPrimaryClick={onPreviewPrimaryClick}
        />,
      );
    });
    await settle();

    const preview = container.querySelector<HTMLElement>(
      ".project-files-preview",
    );
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
      const button = container.querySelector<HTMLButtonElement>(
        '[aria-label="收起文件列表"]',
      );
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
          runtimeClient={{ listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[
            { path: "/workspace/app", label: "App", isPrimary: true },
          ]}
        />,
      );
    });
    await settle();

    const body = container.querySelector<HTMLElement>(
      ".project-files-workspace__body",
    );
    const navigator = container.querySelector<HTMLElement>(
      "#awaken-project-files-navigator",
    );
    const divider = container.querySelector<HTMLElement>(
      '[aria-label="调整文件列表宽度"]',
    );
    body!.getBoundingClientRect = () =>
      ({ left: 0, right: 720, top: 0, bottom: 600, width: 720, height: 600 }) as DOMRect;
    navigator!.getBoundingClientRect = () =>
      ({ left: 360, right: 720, top: 0, bottom: 600, width: 360, height: 600 }) as DOMRect;

    await act(async () => {
      dispatchTimedPointerEvent(divider!, "pointerdown", 360, 1_000);
      dispatchTimedPointerEvent(window, "pointermove", 362, 1_500);
      dispatchTimedPointerEvent(window, "pointermove", 430, 1_545);
      dispatchTimedPointerEvent(window, "pointermove", 500, 1_580);
      dispatchTimedPointerEvent(window, "pointerup", 500, 1_590);
    });

    const toggle = container.querySelector<HTMLButtonElement>(
      '[aria-label="收起文件列表"]',
    );
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
    expect(body?.style.getPropertyValue("--project-file-list-width")).toBe(
      "240px",
    );

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
    const listProjectDirectory = vi.fn(async (request: {
      folderPath: string;
      relativePath?: string;
    }) => {
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
        entries: [
          { name: "src", relativePath: "src", kind: "directory" as const },
        ],
      };
    });

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{ listProjectDirectory, ...previewRuntimeClient() }}
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
    expect(
      srcRow?.querySelector('[data-file-icon="folder-open"]'),
    ).not.toBeNull();
    const indexRow = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "index.ts",
    );
    expect(indexRow?.classList).toContain("is-nested");
    expect(indexRow?.querySelector('[data-file-icon="code"]')).not.toBeNull();

    const folderTrigger = container.querySelector<HTMLButtonElement>(
      '[aria-label="选择项目文件夹组"]',
    );
    await act(async () => folderTrigger?.click());
    expect(
      document.querySelector(".project-files-folder-menu")?.textContent,
    ).not.toContain("/workspace/");
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

  it("reports file selections as project-scoped tabs with the matching mode", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [
        { name: "README.md", relativePath: "README.md", kind: "file" as const },
        { name: "index.html", relativePath: "site/index.html", kind: "file" as const },
      ],
    }));
    const onOpenTab = vi.fn();
    const onWorkspaceModeChange = vi.fn();

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{ listProjectDirectory, ...previewRuntimeClient() }}
          projectId="project-1"
          projectLabel="Awaken"
          folders={[{ path: "/workspace/app", label: "App", isPrimary: true }]}
          onOpenTab={onOpenTab}
          onWorkspaceModeChange={onWorkspaceModeChange}
        />,
      );
    });
    await settle();

    const rows = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="treeitem"]'),
    );
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
});
