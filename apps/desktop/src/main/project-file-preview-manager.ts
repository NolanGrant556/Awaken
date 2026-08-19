import { randomUUID } from "node:crypto";
import { createReadStream } from "node:fs";
import fs from "node:fs/promises";
import path from "node:path";
import { Readable } from "node:stream";
import type {
  DownloadItem,
  MouseInputEvent,
  Session,
  WebContents,
  WebPreferences,
} from "electron";
import type {
  ProjectFilePreviewInteraction,
} from "../shared/project-file-preview-interaction.js";
import type {
  ProjectFilePreviewActionRequest,
  ProjectFilePreviewActionResult,
  ProjectFilePreviewCloseRequest,
  ProjectFilePreviewErrorCode,
  ProjectFilePreviewHandle,
  ProjectFilePreviewKind,
  ProjectFilePreviewSaveRequest,
  ProjectFilePreviewSaveResult,
  ProjectFilePreviewStartRequest,
  ProjectFilePreviewStartResult,
  RuntimeProject,
} from "../shared/runtime.js";
import { OfficeCliError, OfficeCliRenderer, type OfficeCliRuntimePaths } from "./officecli.js";
import { resolveProjectFile, type ResolvedProjectFile } from "./project-files.js";
import {
  MAX_HTML_TEXT_RESOURCE_BYTES,
  MAX_IMAGE_PIXELS,
  MAX_PREVIEW_INFLIGHT_REQUESTS,
  MAX_PREVIEW_REQUESTS,
  MAX_PREVIEW_RESPONSE_BYTES,
  MAX_TEXT_PREVIEW_BYTES,
  classifyPreviewFile,
  decodeTextPreviewBuffer,
  languageNameForFile,
  mimeTypeForFile,
  parseSingleByteRange,
  previewByteLimit,
  rasterDimensions,
  safePreviewPathname,
} from "./project-file-preview-policy.js";

const PREVIEW_SCHEME = "awaken-preview";
const PREVIEW_TTL_MS = 30 * 60 * 1000;
const MAX_IMAGE_HEADER_BYTES = 1024 * 1024;
const PREVIEW_PRIMARY_CLICK_DISTANCE_PX = 6;
const PDF_CORS_HEADERS = {
  "Access-Control-Allow-Headers": "Range",
  "Access-Control-Allow-Methods": "GET, HEAD, OPTIONS",
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Expose-Headers": "Accept-Ranges, Content-Length, Content-Range",
};

type PreviewRecord = {
  previewId: string;
  kind: Exclude<ProjectFilePreviewKind, "markdown" | "code">;
  request: ProjectFilePreviewActionRequest;
  source: ResolvedProjectFile;
  sourceByteLength: number;
  servePath: string;
  src: string;
  partition?: string;
  officeOutputPath?: string;
  expiresAt: number;
  requestCount: number;
  inflightRequests: number;
  responseBytes: number;
  active: boolean;
  session?: Session;
};

type PreviewWebviewParameters = Record<string, string>;

export type ProjectFilePreviewManagerOptions = {
  getProjects: () => Promise<RuntimeProject[]>;
  runtimePaths: OfficeCliRuntimePaths;
  defaultSession: Session;
  sessionFromPartition: (partition: string) => Session;
  openExternal: (url: string) => Promise<void>;
  openPath: (filePath: string) => Promise<string>;
  showItemInFolder: (filePath: string) => void;
  savePdfAs: (defaultPath: string) => Promise<string | undefined>;
  onPreviewInteraction: (interaction: ProjectFilePreviewInteraction) => void;
};

type PreviewPrimaryPointerGesture = {
  startX: number;
  startY: number;
  moved: boolean;
};

function updatePreviewPrimaryPointerGesture(
  gesture: PreviewPrimaryPointerGesture | undefined,
  mouse: MouseInputEvent,
): {
  gesture: PreviewPrimaryPointerGesture | undefined;
  primaryClick: boolean;
} {
  if (mouse.type === "mouseDown" && mouse.button === "left") {
    return {
      gesture: { startX: mouse.x, startY: mouse.y, moved: false },
      primaryClick: false,
    };
  }
  if (!gesture) return { gesture: undefined, primaryClick: false };
  if (mouse.type === "mouseMove") {
    const distanceX = mouse.x - gesture.startX;
    const distanceY = mouse.y - gesture.startY;
    return {
      gesture: {
        ...gesture,
        moved:
          gesture.moved ||
          distanceX * distanceX + distanceY * distanceY >
            PREVIEW_PRIMARY_CLICK_DISTANCE_PX *
              PREVIEW_PRIMARY_CLICK_DISTANCE_PX,
      },
      primaryClick: false,
    };
  }
  if (mouse.type === "mouseUp") {
    return {
      gesture: undefined,
      primaryClick:
        mouse.button !== "right" &&
        mouse.button !== "middle" &&
        !gesture.moved,
    };
  }
  if (mouse.type === "mouseLeave" || mouse.type === "contextMenu") {
    return { gesture: undefined, primaryClick: false };
  }
  return { gesture, primaryClick: false };
}

function encodedPreviewPath(relativePath: string): string {
  return relativePath.split("/").map(encodeURIComponent).join("/");
}

function previewError(
  code: ProjectFilePreviewErrorCode,
  message: string,
  details: { byteLength?: number; limitBytes?: number } = {},
): ProjectFilePreviewStartResult {
  return { ok: false, code, message, ...details };
}

function saveError(
  code: ProjectFilePreviewErrorCode,
  message: string,
  details: { byteLength?: number; limitBytes?: number } = {},
): ProjectFilePreviewSaveResult {
  return { ok: false, code, message, ...details };
}

function nodeErrorCode(error: unknown): string {
  return typeof error === "object" && error !== null && "code" in error
    ? String(error.code)
    : "";
}

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

function isAllowedGuestInternalUrl(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === `${PREVIEW_SCHEME}:` || url.protocol === "data:" || url.protocol === "blob:";
  } catch {
    return value === "about:blank";
  }
}

function responseBodyForFile(
  filePath: string,
  start?: number,
  end?: number,
) {
  const nodeStream = createReadStream(filePath, {
    ...(start === undefined ? {} : { start }),
    ...(end === undefined ? {} : { end }),
  });
  return Readable.toWeb(nodeStream);
}

async function readImageHeader(filePath: string): Promise<Buffer> {
  const file = await fs.open(filePath, "r");
  try {
    const stat = await file.stat();
    const length = Math.min(stat.size, MAX_IMAGE_HEADER_BYTES);
    const buffer = Buffer.alloc(length);
    const { bytesRead } = await file.read(buffer, 0, length, 0);
    return buffer.subarray(0, bytesRead);
  } finally {
    await file.close();
  }
}

async function readFilePrefix(filePath: string, maxBytes: number): Promise<Buffer> {
  const file = await fs.open(filePath, "r");
  try {
    const buffer = Buffer.alloc(maxBytes);
    let offset = 0;
    while (offset < buffer.length) {
      const { bytesRead } = await file.read(
        buffer,
        offset,
        buffer.length - offset,
        offset,
      );
      if (bytesRead === 0) break;
      offset += bytesRead;
    }
    return buffer.subarray(0, offset);
  } finally {
    await file.close();
  }
}

export class ProjectFilePreviewManager {
  private readonly previews = new Map<string, PreviewRecord>();
  private readonly pendingRequests = new Map<string, AbortController>();
  private readonly officeRenderer: OfficeCliRenderer;
  private defaultProtocolRegistered = false;

  constructor(private readonly options: ProjectFilePreviewManagerOptions) {
    this.officeRenderer = new OfficeCliRenderer(options.runtimePaths);
  }

  initialize(): void {
    if (this.defaultProtocolRegistered) return;
    this.options.defaultSession.protocol.handle(PREVIEW_SCHEME, (request) =>
      this.handleProtocolRequest(request),
    );
    this.defaultProtocolRegistered = true;
  }

  async start(
    request: ProjectFilePreviewStartRequest,
  ): Promise<ProjectFilePreviewStartResult> {
    if (
      !request ||
      typeof request.requestId !== "string" ||
      !request.requestId.trim() ||
      request.requestId.length > 128
    ) {
      return previewError("INVALID_REQUEST", "文件预览请求无效。");
    }

    this.cancel({ requestId: request.requestId });
    const controller = new AbortController();
    this.pendingRequests.set(request.requestId, controller);
    try {
      const projects = await this.options.getProjects();
      if (controller.signal.aborted) {
        return previewError("PREVIEW_CANCELLED", "已取消文件预览。");
      }
      const resolved = await resolveProjectFile(projects, request);
      if (!resolved.ok) return previewError(resolved.code, resolved.message);
      let classification = classifyPreviewFile(resolved.file.displayName);
      const stat = await fs.stat(resolved.file.realPath);
      let textBuffer: Buffer | undefined;
      let textContent: string | undefined;
      if (!classification) {
        if (stat.size > MAX_TEXT_PREVIEW_BYTES) {
          return previewError(
            "FILE_TOO_LARGE",
            `文件大小超过当前文本预览上限（${Math.round(MAX_TEXT_PREVIEW_BYTES / 1024 / 1024)} MiB）。`,
            { byteLength: stat.size, limitBytes: MAX_TEXT_PREVIEW_BYTES },
          );
        }
        textBuffer = await readFilePrefix(
          resolved.file.realPath,
          MAX_TEXT_PREVIEW_BYTES + 1,
        );
        if (textBuffer.length > MAX_TEXT_PREVIEW_BYTES) {
          return previewError(
            "FILE_TOO_LARGE",
            `文件大小超过当前文本预览上限（${Math.round(MAX_TEXT_PREVIEW_BYTES / 1024 / 1024)} MiB）。`,
            {
              byteLength: Math.max(stat.size, textBuffer.length),
              limitBytes: MAX_TEXT_PREVIEW_BYTES,
            },
          );
        }
        textContent = decodeTextPreviewBuffer(textBuffer);
        if (textContent === undefined) {
          return previewError(
            "UNSUPPORTED_FILE_TYPE",
            "Awaken 暂不支持预览这种文件格式。",
          );
        }
        classification = {
          kind: "code",
          extension: path.extname(resolved.file.displayName).slice(1).toLocaleLowerCase(),
          language: languageNameForFile(resolved.file.displayName),
        };
      }

      const limit = previewByteLimit(classification.kind);
      if (stat.size > limit) {
        return previewError(
          "FILE_TOO_LARGE",
          `文件大小超过当前预览上限（${Math.round(limit / 1024 / 1024)} MiB）。`,
          { byteLength: stat.size, limitBytes: limit },
        );
      }

      if (classification.kind === "markdown" || classification.kind === "code") {
        const buffer = textBuffer ?? await readFilePrefix(resolved.file.realPath, limit + 1);
        if (buffer.length > limit) {
          return previewError(
            "FILE_TOO_LARGE",
            `文件大小超过当前预览上限（${Math.round(limit / 1024 / 1024)} MiB）。`,
            {
              byteLength: Math.max(stat.size, buffer.length),
              limitBytes: limit,
            },
          );
        }
        const content = textContent ?? decodeTextPreviewBuffer(buffer);
        if (content === undefined) {
          return previewError(
            "TEXT_BINARY_CONTENT",
            "这个文件包含二进制内容，无法作为文本预览。",
          );
        }
        const handle: ProjectFilePreviewHandle = {
          previewId: randomUUID(),
          requestId: request.requestId,
          kind: classification.kind,
          transport: "text",
          displayName: resolved.file.displayName,
          relativePath: resolved.file.relativePath,
          byteLength: stat.size,
          content,
          language: classification.language,
        };
        return { ok: true, preview: handle };
      }

      if (classification.kind === "image" && classification.extension !== "svg") {
        const dimensions = rasterDimensions(
          classification.extension,
          await readImageHeader(resolved.file.realPath),
        );
        if (
          dimensions &&
          dimensions.width > 0 &&
          dimensions.height > 0 &&
          dimensions.width * dimensions.height > MAX_IMAGE_PIXELS
        ) {
          return previewError(
            "IMAGE_TOO_LARGE",
            "图片像素尺寸超过当前预览上限。",
          );
        }
      }

      let officeOutputPath: string | undefined;
      if (classification.kind === "office") {
        officeOutputPath = await this.officeRenderer.render(
          resolved.file.realPath,
          controller.signal,
        );
      }
      if (controller.signal.aborted) {
        return previewError("PREVIEW_CANCELLED", "已取消文件预览。");
      }

      const previewId = randomUUID();
      const partition =
        classification.kind === "image" || classification.kind === "pdf"
          ? undefined
          : `awaken-preview-${previewId}`;
      const servePath =
        classification.kind === "office"
          ? "index.html"
          : classification.kind === "pdf"
            ? "document.pdf"
            : classification.kind === "image"
              ? `image.${classification.extension}`
              : resolved.file.relativePath;
      const src = `${PREVIEW_SCHEME}://${previewId}/${encodedPreviewPath(servePath)}`;
      const record: PreviewRecord = {
        previewId,
        kind: classification.kind,
        request: {
          projectId: request.projectId,
          folderPath: request.folderPath,
          relativePath: request.relativePath,
        },
        source: resolved.file,
        sourceByteLength: stat.size,
        servePath,
        src,
        partition,
        officeOutputPath,
        expiresAt: Date.now() + PREVIEW_TTL_MS,
        requestCount: 0,
        inflightRequests: 0,
        responseBytes: 0,
        active: true,
      };
      this.previews.set(previewId, record);
      if (partition) this.configurePreviewSession(record, partition);

      return {
        ok: true,
        preview: {
          previewId,
          requestId: request.requestId,
          kind: classification.kind,
          transport: "url",
          displayName: resolved.file.displayName,
          relativePath: resolved.file.relativePath,
          byteLength: stat.size,
          src,
          partition,
        },
      };
    } catch (error) {
      if (error instanceof OfficeCliError) {
        return previewError(error.code, error.message);
      }
      return previewError(
        "PREVIEW_UNAVAILABLE",
        "文件预览暂时不可用，请重试或使用系统默认应用打开。",
      );
    } finally {
      if (this.pendingRequests.get(request.requestId) === controller) {
        this.pendingRequests.delete(request.requestId);
      }
    }
  }

  cancel(request: ProjectFilePreviewCloseRequest): void {
    if (request.requestId) {
      this.pendingRequests.get(request.requestId)?.abort();
      this.pendingRequests.delete(request.requestId);
    }
    if (request.previewId) this.release(request.previewId);
  }

  release(previewId: string): void {
    const record = this.previews.get(previewId);
    if (!record) return;
    record.active = false;
    this.previews.delete(previewId);
    if (record.session && record.partition) {
      if (record.session.protocol.isProtocolHandled(PREVIEW_SCHEME)) {
        record.session.protocol.unhandle(PREVIEW_SCHEME);
      }
      record.session.setPermissionCheckHandler(null);
      record.session.setPermissionRequestHandler(null);
      void record.session.clearStorageData();
    }
  }

  dispose(): void {
    for (const controller of this.pendingRequests.values()) controller.abort();
    this.pendingRequests.clear();
    this.releaseAll();
    if (
      this.defaultProtocolRegistered &&
      this.options.defaultSession.protocol.isProtocolHandled(PREVIEW_SCHEME)
    ) {
      this.options.defaultSession.protocol.unhandle(PREVIEW_SCHEME);
    }
    this.defaultProtocolRegistered = false;
  }

  releaseAll(): void {
    for (const previewId of [...this.previews.keys()]) this.release(previewId);
  }

  authorizeWebview(
    webPreferences: WebPreferences,
    params: PreviewWebviewParameters,
  ): boolean {
    const record = this.recordFromUrl(params.src);
    if (
      !record ||
      !record.partition ||
      params.partition !== record.partition ||
      !record.active
    ) {
      return false;
    }
    delete webPreferences.preload;
    webPreferences.nodeIntegration = false;
    webPreferences.contextIsolation = true;
    webPreferences.sandbox = true;
    webPreferences.webSecurity = true;
    webPreferences.plugins = false;
    return true;
  }

  attachGuest(guest: WebContents): void {
    const record =
      this.recordFromUrl(guest.getURL()) ??
      [...this.previews.values()].find(
        (candidate) => candidate.active && candidate.session === guest.session,
      );
    if (!record) {
      guest.close({ waitForBeforeUnload: false });
      return;
    }
    guest.setWindowOpenHandler(({ url }) => {
      if (isHttpUrl(url)) void this.options.openExternal(url);
      return { action: "deny" };
    });
    const handleNavigation = (event: Electron.Event, url: string) => {
      if (isAllowedGuestInternalUrl(url)) return;
      event.preventDefault();
      if (isHttpUrl(url)) void this.options.openExternal(url);
    };
    guest.on("will-navigate", handleNavigation);
    guest.on("will-redirect", handleNavigation);
    let primaryPointerGesture: PreviewPrimaryPointerGesture | undefined;
    guest.on("before-mouse-event", (_event, mouse) => {
      const next = updatePreviewPrimaryPointerGesture(
        primaryPointerGesture,
        mouse,
      );
      primaryPointerGesture = next.gesture;
      if (next.primaryClick) {
        this.options.onPreviewInteraction({
          previewId: record.previewId,
          type: "primary-click",
        });
      }
    });
    guest.on("destroyed", () => {
      this.release(record.previewId);
    });
  }

  async openExternally(
    request: ProjectFilePreviewActionRequest,
  ): Promise<ProjectFilePreviewActionResult> {
    try {
      const resolved = await resolveProjectFile(await this.options.getProjects(), request);
      if (!resolved.ok) return resolved;
      const message = await this.options.openPath(resolved.file.realPath);
      return message
        ? { ok: false, code: "PREVIEW_UNAVAILABLE", message }
        : { ok: true };
    } catch {
      return {
        ok: false,
        code: "PREVIEW_UNAVAILABLE",
        message: "暂时无法使用系统默认应用打开这个文件。",
      };
    }
  }

  async reveal(
    request: ProjectFilePreviewActionRequest,
  ): Promise<ProjectFilePreviewActionResult> {
    try {
      const resolved = await resolveProjectFile(await this.options.getProjects(), request);
      if (!resolved.ok) return resolved;
      this.options.showItemInFolder(resolved.file.realPath);
      return { ok: true };
    } catch {
      return {
        ok: false,
        code: "PREVIEW_UNAVAILABLE",
        message: "暂时无法在文件管理器中显示这个文件。",
      };
    }
  }

  async saveMarkdown(
    request: ProjectFilePreviewSaveRequest,
  ): Promise<ProjectFilePreviewSaveResult> {
    if (typeof request?.content !== "string") {
      return saveError("INVALID_REQUEST", "Markdown 保存请求无效。");
    }
    if (request.content.length > MAX_TEXT_PREVIEW_BYTES) {
      return saveError(
        "FILE_TOO_LARGE",
        `文件大小超过当前文本预览上限（${Math.round(MAX_TEXT_PREVIEW_BYTES / 1024 / 1024)} MiB）。`,
        {
          byteLength: request.content.length,
          limitBytes: MAX_TEXT_PREVIEW_BYTES,
        },
      );
    }

    try {
      const resolved = await resolveProjectFile(await this.options.getProjects(), request);
      if (!resolved.ok) return resolved;
      const classification = classifyPreviewFile(resolved.file.displayName);
      if (classification?.kind !== "markdown") {
        return saveError("UNSUPPORTED_FILE_TYPE", "当前只支持保存 Markdown 文件。");
      }

      const nextBuffer = Buffer.from(request.content, "utf8");
      if (nextBuffer.length > MAX_TEXT_PREVIEW_BYTES) {
        return saveError(
          "FILE_TOO_LARGE",
          `文件大小超过当前文本预览上限（${Math.round(MAX_TEXT_PREVIEW_BYTES / 1024 / 1024)} MiB）。`,
          {
            byteLength: nextBuffer.length,
            limitBytes: MAX_TEXT_PREVIEW_BYTES,
          },
        );
      }

      const current = await fs.readFile(resolved.file.realPath);
      if (!current.equals(nextBuffer)) {
        await fs.writeFile(resolved.file.realPath, nextBuffer);
      }
      return { ok: true, byteLength: nextBuffer.length };
    } catch (error) {
      const code = nodeErrorCode(error);
      if (code === "ENOENT" || code === "ENOTDIR") {
        return saveError("FILE_NOT_FOUND", "所选项目文件不存在。");
      }
      if (code === "EACCES" || code === "EPERM" || code === "EROFS") {
        return saveError("FILE_UNWRITABLE", "这个 Markdown 文件当前不可写入。");
      }
      return saveError("FILE_UNWRITABLE", "暂时无法保存这个 Markdown 文件。");
    }
  }

  private configurePreviewSession(record: PreviewRecord, partition: string): void {
    const previewSession = this.options.sessionFromPartition(partition);
    record.session = previewSession;
    previewSession.setPermissionCheckHandler(() => false);
    previewSession.setPermissionRequestHandler((_webContents, _permission, callback) => {
      callback(false);
    });
    previewSession.webRequest.onBeforeRequest((details, callback) => {
      callback({ cancel: !isAllowedGuestInternalUrl(details.url) });
    });
    previewSession.on("will-download", (event, item) => {
      event.preventDefault();
      item.cancel();
      void this.handleControlledDownload(record, item);
    });
    previewSession.protocol.handle(PREVIEW_SCHEME, (request) =>
      this.handleProtocolRequest(request, record.previewId),
    );
  }

  private async handleControlledDownload(
    record: PreviewRecord,
    item: DownloadItem,
  ): Promise<void> {
    if (record.kind !== "pdf" || item.getURL() !== record.src) return;
    const refreshed = await resolveProjectFile(await this.options.getProjects(), record.request);
    if (!refreshed.ok) return;
    const destination = await this.options.savePdfAs(refreshed.file.displayName);
    if (!destination) return;
    await fs.copyFile(refreshed.file.realPath, destination);
  }

  private recordFromUrl(value: string): PreviewRecord | undefined {
    try {
      const url = new URL(value);
      if (url.protocol !== `${PREVIEW_SCHEME}:`) return undefined;
      return this.previews.get(url.hostname);
    } catch {
      return undefined;
    }
  }

  private invalidateForLimit(record: PreviewRecord): Response {
    record.active = false;
    return new Response("Preview resource limit exceeded", {
      status: 429,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }

  private async handleProtocolRequest(
    request: Request,
    expectedPreviewId?: string,
  ): Promise<Response> {
    let url: URL;
    try {
      url = new URL(request.url);
    } catch {
      return new Response("Bad preview URL", { status: 400 });
    }
    const record = this.previews.get(url.hostname);
    if (
      !record ||
      (expectedPreviewId !== undefined && record.previewId !== expectedPreviewId) ||
      !record.active ||
      Date.now() > record.expiresAt
    ) {
      return new Response("Preview expired", { status: 410 });
    }
    const parts = safePreviewPathname(url.pathname);
    if (!parts) return new Response("Invalid preview path", { status: 400 });
    if (record.kind === "pdf" && request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: PDF_CORS_HEADERS });
    }
    if (request.method !== "GET" && request.method !== "HEAD") {
      return new Response("Method not allowed", { status: 405 });
    }
    if (record.requestCount + 1 > MAX_PREVIEW_REQUESTS) {
      return this.invalidateForLimit(record);
    }
    if (record.inflightRequests + 1 > MAX_PREVIEW_INFLIGHT_REQUESTS) {
      return this.invalidateForLimit(record);
    }
    record.requestCount += 1;
    record.inflightRequests += 1;
    record.expiresAt = Date.now() + PREVIEW_TTL_MS;
    try {
      const resource = await this.resolveResource(record, parts.join("/"));
      if (!resource) return new Response("Preview resource not found", { status: 404 });
      const stat = await fs.stat(resource.filePath);
      const range = parseSingleByteRange(request.headers.get("range"), stat.size);
      if (range === "unsatisfiable") {
        return new Response(null, {
          status: 416,
          headers: {
            ...(record.kind === "pdf" ? PDF_CORS_HEADERS : {}),
            "Content-Range": `bytes */${stat.size}`,
          },
        });
      }
      const responseLength = range ? range.end - range.start + 1 : stat.size;
      if (record.responseBytes + responseLength > MAX_PREVIEW_RESPONSE_BYTES) {
        return this.invalidateForLimit(record);
      }
      record.responseBytes += responseLength;
      const headers = new Headers({
        ...(record.kind === "pdf" ? PDF_CORS_HEADERS : {}),
        "Accept-Ranges": "bytes",
        "Cache-Control": "no-store",
        "Content-Length": String(responseLength),
        "Content-Type": resource.mimeType,
        "X-Content-Type-Options": "nosniff",
      });
      if (range) {
        headers.set("Content-Range", `bytes ${range.start}-${range.end}/${stat.size}`);
      }
      return new Response(
        request.method === "HEAD"
          ? null
          : responseBodyForFile(resource.filePath, range?.start, range?.end),
        { status: range ? 206 : 200, headers },
      );
    } catch {
      return new Response("Preview resource unavailable", { status: 404 });
    } finally {
      record.inflightRequests = Math.max(0, record.inflightRequests - 1);
    }
  }

  private async resolveResource(
    record: PreviewRecord,
    requestedPath: string,
  ): Promise<{ filePath: string; mimeType: string } | undefined> {
    const refreshed = await resolveProjectFile(await this.options.getProjects(), record.request);
    if (!refreshed.ok) {
      record.active = false;
      return undefined;
    }
    if (record.kind === "office") {
      if (requestedPath !== "index.html" || !record.officeOutputPath) return undefined;
      return { filePath: record.officeOutputPath, mimeType: "text/html; charset=utf-8" };
    }
    if (record.kind === "pdf" || record.kind === "image") {
      if (requestedPath !== record.servePath) return undefined;
      return {
        filePath: refreshed.file.realPath,
        mimeType: mimeTypeForFile(refreshed.file.displayName),
      };
    }
    if (record.kind !== "html" || !requestedPath) return undefined;
    const child = await resolveProjectFile(await this.options.getProjects(), {
      projectId: record.request.projectId,
      folderPath: record.request.folderPath,
      relativePath: requestedPath,
    });
    if (!child.ok) return undefined;
    const stat = await fs.stat(child.file.realPath);
    const mimeType = mimeTypeForFile(child.file.displayName);
    const textResource = /^(text\/|application\/(javascript|json|xml))/.test(mimeType);
    const limit = textResource ? MAX_HTML_TEXT_RESOURCE_BYTES : previewByteLimit("image");
    if (stat.size > limit) return undefined;
    return { filePath: child.file.realPath, mimeType };
  }
}
