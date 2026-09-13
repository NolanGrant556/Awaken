// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import FilePreviewHost from "./FilePreviewHost";

async function settleUntil(isReady: () => boolean): Promise<void> {
  await act(async () => {
    for (let attempt = 0; attempt < 50; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 20));
      if (isReady()) return;
    }
  });
}

describe("FilePreviewHost", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("centers the browser empty state with an address-only prompt", async () => {
    await act(async () => {
      root.render(
        <FilePreviewHost
          runtimeClient={{
            startProjectFilePreview: vi.fn(),
            closeProjectFilePreview: vi.fn(async () => undefined),
            openProjectFileExternally: vi.fn(async () => ({
              ok: true as const,
            })),
            revealProjectFile: vi.fn(async () => ({ ok: true as const })),
            saveProjectMarkdownFile: vi.fn(async () => ({
              ok: true as const,
              byteLength: 0,
            })),
          }}
          mode="browser"
          onModeChange={vi.fn()}
        />,
      );
    });

    const emptyState = container.querySelector(
      ".project-files-preview-empty.is-centered",
    );
    expect(emptyState).not.toBeNull();
    expect(emptyState?.querySelector("strong")?.textContent).toBe("浏览器");
    expect(emptyState?.querySelector("span")?.textContent).toBe("输入网址");
    expect(
      container.querySelector('input[aria-label="当前地址"]'),
    ).not.toBeNull();
  });

  it("loads a sanitized independent Markdown preview and updates the file label", async () => {
    const startProjectFilePreview = vi.fn(
      async (request: { requestId: string }) => ({
        ok: true as const,
        preview: {
          previewId: "preview-1",
          requestId: request.requestId,
          kind: "markdown" as const,
          transport: "text" as const,
          displayName: "README.md",
          relativePath: "README.md",
          byteLength: 42,
          content:
            "# Title\n\n- [x] done\n\n<script>window.bad = true</script>",
          language: "markdown",
        },
      }),
    );
    const closeProjectFilePreview = vi.fn(async () => undefined);
    const onModeChange = vi.fn();
    const onPreviewLabelChange = vi.fn();

    await act(async () => {
      root.render(
        <FilePreviewHost
          runtimeClient={{
            startProjectFilePreview,
            closeProjectFilePreview,
            openProjectFileExternally: vi.fn(async () => ({
              ok: true as const,
            })),
            revealProjectFile: vi.fn(async () => ({ ok: true as const })),
            saveProjectMarkdownFile: vi.fn(async () => ({
              ok: true as const,
              byteLength: 42,
            })),
          }}
          projectId="project-1"
          folderPath="/workspace"
          selection={{ name: "README.md", relativePath: "README.md" }}
          mode="files"
          onModeChange={onModeChange}
          onPreviewLabelChange={onPreviewLabelChange}
        />,
      );
    });
    await settleUntil(
      () => container.querySelector(".project-file-markdown-preview") !== null,
    );

    expect(startProjectFilePreview).toHaveBeenCalledWith(
      expect.objectContaining({
        projectId: "project-1",
        folderPath: "/workspace",
        relativePath: "README.md",
      }),
    );
    expect(onModeChange).toHaveBeenCalledWith("files");
    expect(onPreviewLabelChange).toHaveBeenCalledWith("README.md");
    expect(
      container.querySelector(".project-file-markdown-preview")?.textContent,
    ).toContain("Title");
    expect(container.querySelector("script")).toBeNull();
    expect(
      container.querySelectorAll(".project-file-preview-heading"),
    ).toHaveLength(1);
    expect(container.textContent).toContain("查看源代码");
    expect(container.querySelector('[aria-label="保存 Markdown"]')).toBeNull();
    expect(container.querySelector('[aria-label="重新载入文件预览"]')).toBeNull();
    expect(container.querySelector(".project-file-preview-toolbar")).toBeNull();
  });

  it("keeps Markdown source mode and the active preview across parent callback rerenders", async () => {
    const startProjectFilePreview = vi.fn(async (request) => ({
      ok: true as const,
      preview: {
        previewId: "preview-stable-markdown",
        requestId: request.requestId,
        kind: "markdown" as const,
        transport: "text" as const,
        displayName: "README.md",
        relativePath: "README.md",
        byteLength: 16,
        content: "# Stable\n",
        language: "markdown",
      },
    }));
    const runtimeClient = {
      startProjectFilePreview,
      closeProjectFilePreview: vi.fn(async () => undefined),
      openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
      revealProjectFile: vi.fn(async () => ({ ok: true as const })),
      saveProjectMarkdownFile: vi.fn(async () => ({
        ok: true as const,
        byteLength: 16,
      })),
    };
    const selection = { name: "README.md", relativePath: "README.md" };
    const render = (onModeChange: (mode: "files" | "browser") => void) => (
      <FilePreviewHost
        runtimeClient={runtimeClient}
        projectId="project-1"
        folderPath="/workspace"
        selection={selection}
        mode="files"
        onModeChange={onModeChange}
        onPreviewLabelChange={() => undefined}
      />
    );

    await act(async () => root.render(render(() => undefined)));
    await settleUntil(
      () => [...container.querySelectorAll("button")].some((button) => button.textContent === "查看源代码"),
    );
    await act(async () => {
      [...container.querySelectorAll("button")]
        .find((button) => button.textContent === "查看源代码")
        ?.click();
    });
    await settleUntil(() => container.querySelector(".cm-editor") !== null);
    expect(container.querySelector(".cm-editor")).not.toBeNull();

    await act(async () => root.render(render(() => undefined)));

    expect(startProjectFilePreview).toHaveBeenCalledTimes(1);
    expect(container.querySelector(".cm-editor")).not.toBeNull();
    expect(container.textContent).toContain("查看预览");
    expect(container.textContent).not.toContain("正在准备预览");
  });

  it("renders code controls in the shared single-line heading without search", async () => {
    const runtimeClient = {
      startProjectFilePreview: vi.fn(async (request) => ({
        ok: true as const,
        preview: {
          previewId: "preview-code",
          requestId: request.requestId,
          kind: "code" as const,
          transport: "text" as const,
          displayName: "agent.go",
          relativePath: "agent.go",
          byteLength: 32,
          content: "package agent\n\nfunc Run() {}",
          language: "go",
        },
      })),
      closeProjectFilePreview: vi.fn(async () => undefined),
      openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
      revealProjectFile: vi.fn(async () => ({ ok: true as const })),
      saveProjectMarkdownFile: vi.fn(async () => ({
        ok: true as const,
        byteLength: 0,
      })),
    };

    await act(async () => {
      root.render(
        <FilePreviewHost
          runtimeClient={runtimeClient}
          projectId="project-1"
          folderPath="/workspace"
          selection={{ name: "agent.go", relativePath: "agent.go" }}
          mode="files"
          onModeChange={vi.fn()}
        />,
      );
    });
    await settleUntil(
      () => container.querySelector('[aria-label="自动换行"]') !== null,
    );

    const heading = container.querySelector(".project-file-preview-heading");
    expect(heading?.textContent).toContain("agent.go");
    expect(heading?.textContent).toContain("go");
    expect(heading?.querySelector('[aria-label="自动换行"]')).not.toBeNull();
    expect(
      container.querySelectorAll(".project-file-preview-heading"),
    ).toHaveLength(1);
    expect(container.querySelector(".project-file-preview-toolbar")).toBeNull();
    expect(container.textContent).not.toContain("查看源代码");
    expect(container.textContent).not.toContain("搜索");
  });

  it("shows project HTML as editable source in the file tab", async () => {
    const onModeChange = vi.fn();
    await act(async () => {
      root.render(
        <FilePreviewHost
          runtimeClient={{
            startProjectFilePreview: vi.fn(async (request) => ({
              ok: true as const,
              preview: {
                previewId: "preview-html",
                requestId: request.requestId,
                kind: "code" as const,
                transport: "text" as const,
                displayName: "index.html",
                relativePath: "site/index.html",
                byteLength: 15,
                content: "<h1>Local</h1>\n",
                language: "html",
              },
            })),
            closeProjectFilePreview: vi.fn(async () => undefined),
            openProjectFileExternally: vi.fn(async () => ({
              ok: true as const,
            })),
            revealProjectFile: vi.fn(async () => ({ ok: true as const })),
            saveProjectMarkdownFile: vi.fn(async () => ({
              ok: true as const,
              byteLength: 0,
            })),
          }}
          projectId="project-1"
          folderPath="/workspace"
          selection={{ name: "index.html", relativePath: "site/index.html" }}
          mode="files"
          onModeChange={onModeChange}
        />,
      );
    });
    await settleUntil(
      () =>
        container.querySelector('[aria-label="index.html 文本编辑器"]') !== null,
    );

    expect(onModeChange).toHaveBeenCalledWith("files");
    expect(container.querySelector("webview")).toBeNull();
    expect(container.querySelector(".embedded-browser__toolbar")).toBeNull();
    expect(container.textContent).not.toContain("查看源代码");
    expect(container.querySelector(".project-file-preview-heading")?.textContent).toContain(
      "index.html",
    );
    expect(container.querySelector(".project-file-preview-heading")?.textContent).toContain(
      "html",
    );
    expect(container.querySelector('[aria-label="自动换行"]')).not.toBeNull();
  });

  it("navigates the address bar to https without stacking a second toolbar", async () => {
    const idleState = {
      url: "",
      title: "",
      canGoBack: false,
      canGoForward: false,
      loading: false,
      secure: false,
      hasPage: false,
    };
    const navigate = vi.fn(async (_ref: unknown, url: string) => ({
      ...idleState,
      url,
      hasPage: true,
      secure: url.startsWith("https:"),
    }));
    const embeddedBrowser = {
      navigate,
      back: vi.fn(async () => idleState),
      forward: vi.fn(async () => idleState),
      reload: vi.fn(async () => idleState),
      stop: vi.fn(async () => idleState),
      close: vi.fn(async () => undefined),
      getState: vi.fn(async () => idleState),
      setViewport: vi.fn(async () => undefined),
      setActive: vi.fn(async () => undefined),
      onStateChanged: vi.fn(() => () => {}),
      onGuestHttpNavigate: vi.fn(() => () => {}),
    };

    await act(async () => {
      root.render(
        <FilePreviewHost
          runtimeClient={{
            startProjectFilePreview: vi.fn(),
            closeProjectFilePreview: vi.fn(async () => undefined),
            openProjectFileExternally: vi.fn(async () => ({
              ok: true as const,
            })),
            revealProjectFile: vi.fn(async () => ({ ok: true as const })),
            saveProjectMarkdownFile: vi.fn(async () => ({
              ok: true as const,
              byteLength: 0,
            })),
          }}
          mode="browser"
          onModeChange={vi.fn()}
          threadId="thread-1"
          viewId="project-file-slot:1"
          embeddedBrowser={embeddedBrowser}
        />,
      );
    });

    const input = container.querySelector(
      'input[aria-label="当前地址"]',
    ) as HTMLInputElement;
    expect(input).not.toBeNull();
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        window.HTMLInputElement.prototype,
        "value",
      )?.set;
      setter?.call(input, "example.com");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      container
        .querySelector("form.embedded-browser__toolbar")
        ?.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });

    expect(navigate).toHaveBeenCalledWith(
      { threadId: "thread-1", viewId: "project-file-slot:1" },
      "https://example.com/",
    );
    expect(container.querySelectorAll(".embedded-browser__toolbar")).toHaveLength(
      1,
    );
    expect(container.querySelector(".project-file-preview-heading")).toBeNull();
  });

  it("activates a local view so the empty workbench can still show a page", async () => {
    const idleState = {
      url: "",
      title: "",
      canGoBack: false,
      canGoForward: false,
      loading: false,
      secure: false,
      hasPage: false,
    };
    let emitState:
      | ((payload: {
          viewId: string;
          threadId: string;
          state: typeof idleState & { url: string; hasPage: boolean };
        }) => void)
      | undefined;
    const setActive = vi.fn(async () => undefined);
    const navigate = vi.fn(async (ref: { threadId: string; viewId: string }, url: string) => {
      const next = { ...idleState, url, hasPage: true };
      emitState?.({ ...ref, state: next });
      return next;
    });
    const embeddedBrowser = {
      navigate,
      back: vi.fn(async () => idleState),
      forward: vi.fn(async () => idleState),
      reload: vi.fn(async () => idleState),
      stop: vi.fn(async () => idleState),
      close: vi.fn(async () => undefined),
      getState: vi.fn(async () => idleState),
      setViewport: vi.fn(async () => undefined),
      setActive,
      onStateChanged: vi.fn((handler) => {
        emitState = handler;
        return () => {
          emitState = undefined;
        };
      }),
      onGuestHttpNavigate: vi.fn(() => () => {}),
    };

    await act(async () => {
      root.render(
        <FilePreviewHost
          runtimeClient={{
            startProjectFilePreview: vi.fn(),
            closeProjectFilePreview: vi.fn(async () => undefined),
            openProjectFileExternally: vi.fn(async () => ({
              ok: true as const,
            })),
            revealProjectFile: vi.fn(async () => ({ ok: true as const })),
            saveProjectMarkdownFile: vi.fn(async () => ({
              ok: true as const,
              byteLength: 0,
            })),
          }}
          mode="browser"
          onModeChange={vi.fn()}
          viewId="project-file-slot:9"
          embeddedBrowser={embeddedBrowser}
        />,
      );
    });

    expect(setActive).toHaveBeenCalledWith({
      threadId: "local",
      viewId: "project-file-slot:9",
    });

    const input = container.querySelector(
      'input[aria-label="当前地址"]',
    ) as HTMLInputElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        window.HTMLInputElement.prototype,
        "value",
      )?.set;
      setter?.call(input, "http://127.0.0.1:8765/");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      container
        .querySelector("form.embedded-browser__toolbar")
        ?.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });

    expect(navigate).toHaveBeenCalledWith(
      { threadId: "local", viewId: "project-file-slot:9" },
      "http://127.0.0.1:8765/",
    );
    const page = container.querySelector(".embedded-browser__page");
    expect(page).not.toBeNull();
    expect(page?.querySelector(".project-files-preview-empty")).toBeNull();
    expect(
      container.querySelector(".project-files-preview-empty")?.closest("[hidden]"),
    ).not.toBeNull();
  });

  it("keeps an Office webview mounted while switching between browser and files", async () => {
    let interactionHandler:
      | ((interaction: { previewId: string; type: "primary-click" }) => void)
      | undefined;
    const unsubscribeInteraction = vi.fn();
    const runtimeClient = {
      startProjectFilePreview: vi.fn(async (request) => ({
        ok: true as const,
        preview: {
          previewId: "preview-office",
          requestId: request.requestId,
          kind: "office" as const,
          transport: "url" as const,
          displayName: "台账.xlsx",
          relativePath: "台账.xlsx",
          byteLength: 256,
          src: "awaken-preview://preview-office/index.html",
          partition: "awaken-preview-preview-office",
        },
      })),
      closeProjectFilePreview: vi.fn(async () => undefined),
      onProjectFilePreviewInteraction: vi.fn((handler) => {
        interactionHandler = handler;
        return unsubscribeInteraction;
      }),
      openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
      revealProjectFile: vi.fn(async () => ({ ok: true as const })),
      saveProjectMarkdownFile: vi.fn(async () => ({
        ok: true as const,
        byteLength: 0,
      })),
    };
    const onModeChange = vi.fn();
    const onPreviewPrimaryClick = vi.fn();
    const selection = { name: "台账.xlsx", relativePath: "台账.xlsx" };
    const render = (mode: "files" | "browser") => (
      <FilePreviewHost
        runtimeClient={runtimeClient}
        projectId="project-1"
        folderPath="/workspace"
        selection={selection}
        mode={mode}
        onModeChange={onModeChange}
        onPreviewPrimaryClick={onPreviewPrimaryClick}
      />
    );

    await act(async () => root.render(render("files")));
    await settleUntil(() => container.querySelector("webview") !== null);
    const webview = container.querySelector("webview");
    expect(
      runtimeClient.onProjectFilePreviewInteraction,
    ).toHaveBeenCalledOnce();

    await act(async () => {
      interactionHandler?.({
        previewId: "another-preview",
        type: "primary-click",
      });
      interactionHandler?.({
        previewId: "preview-office",
        type: "primary-click",
      });
    });
    expect(onPreviewPrimaryClick).toHaveBeenCalledOnce();

    await act(async () => root.render(render("browser")));
    expect(container.querySelector("webview")).toBe(webview);
    expect(
      webview?.closest(".project-file-preview-mode-stack__preview")?.classList,
    ).toContain("is-hidden");
    expect(
      container.querySelector(".project-files-preview-empty")?.textContent,
    ).toContain("输入网址");

    await act(async () => root.render(render("files")));
    expect(container.querySelector("webview")).toBe(webview);
    expect(
      webview?.closest(".project-file-preview-mode-stack__preview")?.classList,
    ).not.toContain("is-hidden");
    expect(runtimeClient.startProjectFilePreview).toHaveBeenCalledTimes(1);
  });
});
