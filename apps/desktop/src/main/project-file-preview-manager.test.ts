import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Session, WebContents, WebPreferences } from "electron";
import type { RuntimeProject } from "../shared/runtime.js";
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

function createManager(root: string) {
  const defaultSession = fakeSession();
  const previewSessions = new Map<string, FakeSession>();
  const openExternal = vi.fn(async () => undefined);
  const manager = new ProjectFilePreviewManager({
    getProjects: async () => [project(root)],
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
  });
  manager.initialize();
  return { manager, defaultSession, previewSessions, openExternal };
}

afterEach(async () => {
  await Promise.all(
    temporaryRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

describe("ProjectFilePreviewManager", () => {
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
    const { manager, previewSessions, openExternal } = createManager(root);
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
    manager.dispose();
  });
});
