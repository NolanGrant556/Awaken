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
          runtimeClient={{ listProjectDirectory }}
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
    body!.getBoundingClientRect = () =>
      ({ left: 0, right: 720, top: 0, bottom: 600, width: 720, height: 600 }) as DOMRect;
    navigator!.getBoundingClientRect = () =>
      ({ left: 360, right: 720, top: 0, bottom: 600, width: 360, height: 600 }) as DOMRect;
    const toggle = container.querySelector<HTMLButtonElement>(
      '[aria-label="收起文件列表"]',
    );
    expect(toggle?.getAttribute("aria-expanded")).toBe("true");
    expect(navigator?.getAttribute("aria-hidden")).toBe("false");
    expect(container.textContent).toContain("README.md");

    await act(async () => {
      divider?.dispatchEvent(
        new MouseEvent("pointerdown", { bubbles: true, clientX: 360 }),
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

  it("only collapses after a forceful rightward fling from minimum width", async () => {
    const listProjectDirectory = vi.fn(async () => ({
      ok: true as const,
      relativePath: "",
      entries: [],
    }));

    await act(async () => {
      root.render(
        <ProjectFilesWorkspace
          runtimeClient={{ listProjectDirectory }}
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
      dispatchTimedPointerEvent(window, "pointermove", 510, 2_035);
      dispatchTimedPointerEvent(window, "pointermove", 550, 2_075);
    });
    expect(toggle?.getAttribute("aria-expanded")).toBe("false");
    expect(navigator?.getAttribute("aria-hidden")).toBe("true");

    await act(async () => toggle?.click());
    await act(async () => {
      dispatchTimedPointerEvent(divider!, "pointerdown", 480, 3_000);
      dispatchTimedPointerEvent(window, "pointermove", 520, 3_220);
      dispatchTimedPointerEvent(window, "pointermove", 560, 3_500);
      dispatchTimedPointerEvent(window, "pointerup", 560, 3_520);
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
          runtimeClient={{ listProjectDirectory }}
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
});
