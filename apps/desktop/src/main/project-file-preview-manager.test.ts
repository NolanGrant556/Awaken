import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Session, WebContents, WebPreferences } from "electron";
import type { RuntimeProject, RuntimeThreadTranscript } from "../shared/runtime.js";
import { ProjectFilePreviewManager } from "./project-file-preview-manager.js";
import { MAX_TEXT_PREVIEW_BYTES } from "./project-file-preview-policy.js";

const temporaryRoots: string[] = [];

async function temporaryRoot(): Promise<string> {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-preview-manager-"));
  temporaryRoots.push(root);
  return root;
}

function project(folderPath: string): RuntimeProject {
  return {
    projectId: "project-1",
    workspaceRoot: folderPath,
    folders: [{ path: folderPath, role: "user", isPrimary: true }],
    displayName: "Project",
    state: "active",
    createdAt: "2026-07-28T00:00:00.000Z",
    updatedAt: "2026-07-28T00:00:00.000Z",
    version: 1,
  };
}

type FakeSession = {
  protocolHandler?: (request: Request) => Response | Promise<Response>;
  permissionCheck?: unknown;
  permissionRequest?: unknown;
  protocol: {
    handle: ReturnType<typeof vi.fn>;
    unhandle: ReturnType<typeof vi.fn>;
    isProtocolHandled: ReturnType<typeof vi.fn>;
  };
  webRequest: { onBeforeRequest: ReturnType<typeof vi.fn> };
  setPermissionCheckHandler: ReturnType<typeof vi.fn>;
  setPermissionRequestHandler: ReturnType<typeof vi.fn>;
  on: ReturnType<typeof vi.fn>;
  clearStorageData: ReturnType<typeof vi.fn>;
};

function fakeSession(): FakeSession {
  const fake = {
    protocol: {
      handle: vi.fn(),
      unhandle: vi.fn(),
      isProtocolHandled: vi.fn(() => true),
    },
    webRequest: { onBeforeRequest: vi.fn() },
    setPermissionCheckHandler: vi.fn(),
    setPermissionRequestHandler: vi.fn(),
    on: vi.fn(),
    clearStorageData: vi.fn(async () => undefined),
  } as FakeSession;
  fake.protocol.handle.mockImplementation(
    (_scheme: string, handler: (request: Request) => Response | Promise<Response>) => {
      fake.protocolHandler = handler;
    },
  );
  return fake;
}

function createManager(root: string, transcript?: RuntimeThreadTranscript) {
  const defaultSession = fakeSession();
  const previewSessions = new Map<string, FakeSession>();
  const openExternal = vi.fn(async () => undefined);
  const onPreviewInteraction = vi.fn();
  const manager = new ProjectFilePreviewManager({
    getProjects: async () => [project(root)],
    getTranscript: async () => {
      if (!transcript) throw new Error("missing transcript fixture");
      return transcript;
    },
    runtimePaths: {
      packaged: false,
      resourcesPath: root,
      appPath: root,
      cachePath: root,
    },
    defaultSession: defaultSession as unknown as Session,
    sessionFromPartition: (partition) => {
      const current = fakeSession();
      previewSessions.set(partition, current);
      return current as unknown as Session;
    },
    openExternal,
    openPath: vi.fn(async () => ""),
    showItemInFolder: vi.fn(),
    savePdfAs: vi.fn(async () => undefined),
    onPreviewInteraction,
  });
  manager.initialize();
  return {
    manager,
    defaultSession,
    previewSessions,
    openExternal,
    onPreviewInteraction,
  };
}

afterEach(async () => {
  await Promise.all(
    temporaryRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

describe("ProjectFilePreviewManager", () => {
  it("previews a recorded deliverable through its durable artifact identity", async () => {
    const root = await temporaryRoot();
    await fs.writeFile(path.join(root, "delivery.md"), "# Delivery\n");
    const transcript = {
      thread: {
        threadId: "thread-1",
        workspace: { workspaceRoot: root },
      },
      runs: [
        {
          run: { runId: "run-1", status: "finished" },
          events: [
            {
              type: "AWAKEN_ARTIFACT_CREATED",
              artifactId: "artifact-1",
              artifactPath: "delivery.md",
              artifactTitle: "Delivery",
              artifactKind: "document",
              artifactStatus: "ready",
              artifactRole: "deliverable",
              artifactContractVersion: 2,
              artifactProducer: "artifact_publish",
            },
          ],
        },
      ],
    } as unknown as RuntimeThreadTranscript;
    const { manager } = createManager(root, transcript);

    await expect(
      manager.startArtifact({
        requestId: "artifact-preview-1",
        threadId: "thread-1",
        runId: "run-1",
        artifactId: "artifact-1",
      }),
    ).resolves.toMatchObject({
      ok: true,
      preview: {
        kind: "markdown",
        transport: "text",
        content: "# Delivery\n",
      },
    });
  });

  it("returns bounded text through typed IPC and rejects a file just over the limit", async () => {
    const root = await temporaryRoot();
    await fs.writeFile(path.join(root, "README.md"), "# Project\n");
    await fs.writeFile(path.join(root, "large.txt"), "");
    await fs.truncate(path.join(root, "large.txt"), MAX_TEXT_PREVIEW_BYTES + 1);
    const { manager } = createManager(root);

    await expect(
      manager.start({
        requestId: "request-1",
        projectId: "project-1",
        folderPath: root,
        relativePath: "README.md",
      }),
    ).resolves.toMatchObject({
      ok: true,
      preview: { kind: "markdown", transport: "text", content: "# Project\n" },
    });
    await expect(
      manager.start({
        requestId: "request-2",
        projectId: "project-1",
        folderPath: root,
        relativePath: "large.txt",
      }),
    ).resolves.toMatchObject({
      ok: false,
      code: "FILE_TOO_LARGE",
      byteLength: MAX_TEXT_PREVIEW_BYTES + 1,
      limitBytes: MAX_TEXT_PREVIEW_BYTES,
    });
    manager.dispose();
  });

  it("falls back to a bounded code preview for safe text with unknown extensions", async () => {
    const root = await temporaryRoot();
    await fs.writeFile(path.join(root, "go.mod"), "module example.com/app\n\ngo 1.24\n");
    await fs.writeFile(path.join(root, "build.customlang"), "task build { run() }\n");
    await fs.writeFile(path.join(root, "binary.customlang"), Buffer.from([0x00, 0x01, 0x02]));
    const { manager } = createManager(root);

    await expect(
      manager.start({
        requestId: "request-go-mod",
        projectId: "project-1",
        folderPath: root,
        relativePath: "go.mod",
      }),
    ).resolves.toMatchObject({
      ok: true,
      preview: {
        kind: "code",
        transport: "text",
        language: "mod",
        content: "module example.com/app\n\ngo 1.24\n",
      },
    });
    await expect(
      manager.start({
        requestId: "request-custom-source",
        projectId: "project-1",
        folderPath: root,
        relativePath: "build.customlang",
      }),
    ).resolves.toMatchObject({
      ok: true,
      preview: {
        kind: "code",
        transport: "text",
        language: "customlang",
        content: "task build { run() }\n",
      },
    });
    await expect(
      manager.start({
        requestId: "request-custom-binary",
        projectId: "project-1",
        folderPath: root,
        relativePath: "binary.customlang",
      }),
    ).resolves.toMatchObject({
      ok: false,
      code: "UNSUPPORTED_FILE_TYPE",
    });
    manager.dispose();
  });

  it("streams CORS-enabled PDF ranges to the renderer-side PDF.js viewer", async () => {
    const root = await temporaryRoot();
    await fs.writeFile(path.join(root, "manual.pdf"), Buffer.from("0123456789"));
    const { manager, defaultSession } = createManager(root);
    const result = await manager.start({
      requestId: "request-pdf",
      projectId: "project-1",
      folderPath: root,
      relativePath: "manual.pdf",
    });
    expect(result.ok).toBe(true);
    if (!result.ok || result.preview.transport !== "url") return;
    expect(result.preview.partition).toBeUndefined();
    const handler = defaultSession.protocolHandler;
    expect(handler).toBeTypeOf("function");
    const response = await handler!(
      new Request(result.preview.src, { headers: { Range: "bytes=2-5" } }),
    );
    expect(response.status).toBe(206);
    expect(response.headers.get("content-range")).toBe("bytes 2-5/10");
    expect(response.headers.get("access-control-allow-origin")).toBe("*");
    expect(response.headers.get("access-control-expose-headers")).toContain(
      "Content-Range",
    );
    expect(Buffer.from(await response.arrayBuffer()).toString()).toBe("2345");

    const optionsResponse = await handler!(
      new Request(result.preview.src, { method: "OPTIONS" }),
    );
    expect(optionsResponse.status).toBe(204);
    expect(optionsResponse.headers.get("access-control-allow-headers")).toBe("Range");
    manager.dispose();
  });

  it("does not attach PDF handles as Electron webviews", async () => {
    const root = await temporaryRoot();
    await fs.writeFile(path.join(root, "manual.pdf"), Buffer.from("%PDF-1.4\n"));
    const { manager } = createManager(root);
    const result = await manager.start({
      requestId: "request-pdf-resources",
      projectId: "project-1",
      folderPath: root,
      relativePath: "manual.pdf",
    });
    expect(result.ok).toBe(true);
    if (!result.ok || result.preview.transport !== "url") return;

    const preferences = {} as WebPreferences;
    expect(
      manager.authorizeWebview(preferences, {
        src: result.preview.src,
        partition: "",
      }),
    ).toBe(false);
    expect(preferences).toEqual({});
    manager.dispose();
  });

  it("serves HTML relative resources while project membership is still valid", async () => {
    const root = await temporaryRoot();
    await fs.mkdir(path.join(root, "site"));
    await fs.writeFile(path.join(root, "site", "index.html"), '<link href="style.css">');
    await fs.writeFile(path.join(root, "site", "style.css"), "body { color: red; }");
    const { manager, previewSessions } = createManager(root);
    const result = await manager.start({
      requestId: "request-html",
      projectId: "project-1",
      folderPath: root,
      relativePath: "site/index.html",
    });
    expect(result.ok).toBe(true);
    if (!result.ok || result.preview.transport !== "url" || !result.preview.partition) return;
    const handler = previewSessions.get(result.preview.partition)?.protocolHandler;
    const response = await handler!(
      new Request(result.preview.src.replace("index.html", "style.css")),
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain("text/css");
    expect(await response.text()).toContain("color: red");
    manager.dispose();
  });

  it("authorizes only the signed preview partition and hardens its webview", async () => {
    const root = await temporaryRoot();
    await fs.mkdir(path.join(root, "site"));
    await fs.writeFile(path.join(root, "site", "index.html"), "<h1>Local</h1>");
    const {
      manager,
      previewSessions,
      openExternal,
      onPreviewInteraction,
    } = createManager(root);
    const result = await manager.start({
      requestId: "request-secure-html",
      projectId: "project-1",
      folderPath: root,
      relativePath: "site/index.html",
    });
    expect(result.ok).toBe(true);
    if (!result.ok || result.preview.transport !== "url" || !result.preview.partition) return;

    const preferences = { preload: "/untrusted/preload.js" } as WebPreferences;
    expect(
      manager.authorizeWebview(preferences, {
        src: result.preview.src,
        partition: result.preview.partition,
      }),
    ).toBe(true);
    expect(preferences).toMatchObject({
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: true,
      webSecurity: true,
    });
    expect(preferences.preload).toBeUndefined();
    expect(
      manager.authorizeWebview({} as WebPreferences, {
        src: result.preview.src,
        partition: "wrong-partition",
      }),
    ).toBe(false);

    const previewSession = previewSessions.get(result.preview.partition)!;
    const permissionCheck = previewSession.setPermissionCheckHandler.mock.calls[0]?.[0];
    expect(permissionCheck?.()).toBe(false);
    const beforeRequest = previewSession.webRequest.onBeforeRequest.mock.calls[0]?.[0];
    const callback = vi.fn();
    beforeRequest?.(
      { url: "https://example.com/", resourceType: "mainFrame" },
      callback,
    );
    expect(callback).toHaveBeenCalledWith({ cancel: true });
    expect(openExternal).not.toHaveBeenCalled();

    const close = vi.fn();
    const on = vi.fn();
    const guest = {
      session: previewSession,
      getURL: () => "about:blank",
      setWindowOpenHandler: vi.fn(),
      on,
      close,
    } as unknown as WebContents;
    manager.attachGuest(guest);
    expect(close).not.toHaveBeenCalled();
    expect(guest.setWindowOpenHandler).toHaveBeenCalledOnce();
    const willNavigate = on.mock.calls.find(
      ([eventName]) => eventName === "will-navigate",
    )?.[1] as ((event: { preventDefault: () => void }, url: string) => void) | undefined;
    const preventDefault = vi.fn();
    willNavigate?.({ preventDefault }, "https://example.com/");
    expect(preventDefault).toHaveBeenCalledOnce();
    expect(openExternal).toHaveBeenCalledWith("https://example.com/");

    const beforeMouseEvent = on.mock.calls.find(
      ([eventName]) => eventName === "before-mouse-event",
    )?.[1] as
      | ((
          event: unknown,
          mouse: {
            type: "mouseDown" | "mouseMove" | "mouseUp";
            button?: "left";
            x: number;
            y: number;
          },
        ) => void)
      | undefined;
    beforeMouseEvent?.({}, { type: "mouseDown", button: "left", x: 10, y: 10 });
    beforeMouseEvent?.({}, { type: "mouseMove", x: 30, y: 10 });
    beforeMouseEvent?.({}, { type: "mouseUp", button: "left", x: 30, y: 10 });
    expect(onPreviewInteraction).not.toHaveBeenCalled();

    beforeMouseEvent?.({}, { type: "mouseDown", button: "left", x: 10, y: 10 });
    beforeMouseEvent?.({}, { type: "mouseUp", button: "left", x: 12, y: 11 });
    expect(onPreviewInteraction).toHaveBeenCalledWith({
      previewId: result.preview.previewId,
      type: "primary-click",
    });
    manager.dispose();
  });

  it("saves editable text previews in place and rejects non-text preview kinds", async () => {
    const root = await temporaryRoot();
    await fs.writeFile(path.join(root, "README.md"), "# Project\n");
    await fs.writeFile(path.join(root, "notes.txt"), "plain\n");
    await fs.writeFile(path.join(root, "index.html"), "<p>hi</p>\n");
    const { manager } = createManager(root);
    const request = {
      projectId: "project-1",
      folderPath: root,
    };

    await expect(
      manager.saveMarkdown({
        ...request,
        relativePath: "README.md",
        content: "# Updated\n",
      }),
    ).resolves.toMatchObject({ ok: true, byteLength: Buffer.byteLength("# Updated\n") });
    await expect(fs.readFile(path.join(root, "README.md"), "utf8")).resolves.toBe(
      "# Updated\n",
    );
    await expect(
      manager.saveMarkdown({
        ...request,
        relativePath: "notes.txt",
        content: "changed\n",
      }),
    ).resolves.toMatchObject({ ok: true, byteLength: Buffer.byteLength("changed\n") });
    await expect(fs.readFile(path.join(root, "notes.txt"), "utf8")).resolves.toBe(
      "changed\n",
    );
    await expect(
      manager.saveMarkdown({
        ...request,
        relativePath: "index.html",
        content: "<p>no</p>\n",
      }),
    ).resolves.toMatchObject({
      ok: false,
      code: "UNSUPPORTED_FILE_TYPE",
    });
    await expect(
      manager.saveMarkdown({
        ...request,
        relativePath: "missing.md",
        content: "# New\n",
      }),
    ).resolves.toMatchObject({
      ok: false,
      code: "FILE_NOT_FOUND",
    });
    await expect(fs.access(path.join(root, "missing.md"))).rejects.toThrow();
    await expect(
      manager.saveMarkdown({
        ...request,
        relativePath: "README.md",
        content: "x".repeat(MAX_TEXT_PREVIEW_BYTES + 1),
      }),
    ).resolves.toMatchObject({
      ok: false,
      code: "FILE_TOO_LARGE",
      limitBytes: MAX_TEXT_PREVIEW_BYTES,
    });
    await expect(fs.readFile(path.join(root, "README.md"), "utf8")).resolves.toBe(
      "# Updated\n",
    );
    manager.dispose();
  });

	it("requires digest CAS for project AGENTS.md and preserves both conflict bodies", async () => {
		const root = await temporaryRoot();
		await fs.mkdir(path.join(root, ".awaken"));
		await fs.writeFile(path.join(root, ".awaken", "AGENTS.md"), "# Original\n");
		const { manager } = createManager(root);
		const preview = await manager.start({
			requestId: "preview-agents",
			projectId: "project-1",
			folderPath: root,
			relativePath: ".awaken/AGENTS.md",
		});
		if (!preview.ok || preview.preview.transport !== "text") throw new Error("expected text preview");

		await expect(manager.saveMarkdown({
			projectId: "project-1",
			folderPath: root,
			relativePath: ".awaken/AGENTS.md",
			content: "# Missing digest\n",
		})).resolves.toMatchObject({ ok: false, code: "INVALID_REQUEST" });

		await fs.writeFile(path.join(root, ".awaken", "AGENTS.md"), "# External\n");
		const conflict = await manager.saveMarkdown({
			projectId: "project-1",
			folderPath: root,
			relativePath: ".awaken/AGENTS.md",
			content: "# Local draft\n",
			expectedDigest: preview.preview.contentDigest,
		});
		expect(conflict).toMatchObject({
			ok: false,
			code: "FILE_CHANGED",
			currentContent: "# External\n",
			currentDigest: expect.any(String),
			submittedContent: "# Local draft\n",
		});
		if (conflict.ok || !conflict.currentDigest) throw new Error("expected conflict digest");
		await expect(manager.saveMarkdown({
			projectId: "project-1",
			folderPath: root,
			relativePath: ".awaken/AGENTS.md",
			content: "# Local draft\n",
			expectedDigest: conflict.currentDigest,
		})).resolves.toMatchObject({ ok: true, digest: expect.any(String) });
		await expect(fs.readFile(path.join(root, ".awaken", "AGENTS.md"), "utf8")).resolves.toBe("# Local draft\n");
	});
});
