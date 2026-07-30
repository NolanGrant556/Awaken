import { randomUUID } from "node:crypto";
import { createReadStream } from "node:fs";
import fs from "node:fs/promises";
import { Readable } from "node:stream";
import type { Session } from "electron";
import type {
  ArtifactMediaSourceResult,
  ArtifactOpenRequest,
  RuntimeThreadTranscript,
} from "../shared/runtime.js";
import { resolveArtifactMediaTarget } from "./artifact-open.js";
import { parseSingleByteRange, safePreviewPathname } from "./project-file-preview-policy.js";

const ARTIFACT_MEDIA_SCHEME = "awaken-artifact";
const ARTIFACT_MEDIA_TTL_MS = 30 * 60 * 1000;
const MAX_ARTIFACT_MEDIA_RECORDS = 128;

type ArtifactMediaRecord = {
  token: string;
  identity: string;
  path: string;
  fileName: string;
  mimeType: string;
  byteLength: number;
  expiresAt: number;
};

export type ArtifactMediaManagerOptions = {
  defaultSession: Session;
  getTranscript: (threadId: string) => Promise<RuntimeThreadTranscript>;
};

function responseBodyForFile(filePath: string, start?: number, end?: number) {
  return Readable.toWeb(
    createReadStream(filePath, {
      ...(start === undefined ? {} : { start }),
      ...(end === undefined ? {} : { end }),
    }),
  );
}

export class ArtifactMediaManager {
  private readonly records = new Map<string, ArtifactMediaRecord>();
  private readonly tokensByIdentity = new Map<string, string>();
  private initialized = false;

  constructor(private readonly options: ArtifactMediaManagerOptions) {}

  initialize(): void {
    if (this.initialized) return;
    this.options.defaultSession.protocol.handle(ARTIFACT_MEDIA_SCHEME, (request) =>
      this.handleRequest(request),
    );
    this.initialized = true;
  }

  async source(request: ArtifactOpenRequest): Promise<ArtifactMediaSourceResult> {
    let transcript: RuntimeThreadTranscript;
    try {
      transcript = await this.options.getTranscript(request.threadId);
    } catch {
      return {
        ok: false,
        code: "runtime_unavailable",
        message: "暂时无法读取媒体交付记录，请稍后重试。",
      };
    }
    const resolved = await resolveArtifactMediaTarget(transcript, request);
    if (!resolved.ok) return resolved.result;

    this.pruneExpired();
    const identity = `${request.threadId}:${request.runId}:${request.artifactId}`;
    const existingToken = this.tokensByIdentity.get(identity);
    const existing = existingToken ? this.records.get(existingToken) : undefined;
    const token = existing?.token ?? randomUUID();
    const record: ArtifactMediaRecord = {
      token,
      identity,
      path: resolved.path,
      fileName: resolved.fileName,
      mimeType: resolved.mimeType,
      byteLength: resolved.byteLength,
      expiresAt: Date.now() + ARTIFACT_MEDIA_TTL_MS,
    };
    this.records.set(token, record);
    this.tokensByIdentity.set(identity, token);
    this.pruneOverflow();
    return {
      ok: true,
      src: `${ARTIFACT_MEDIA_SCHEME}://media/${encodeURIComponent(token)}/${encodeURIComponent(record.fileName)}`,
      mimeType: record.mimeType,
      byteLength: record.byteLength,
    };
  }

  dispose(): void {
    this.records.clear();
    this.tokensByIdentity.clear();
  }

  private async handleRequest(request: Request): Promise<Response> {
    let url: URL;
    try {
      url = new URL(request.url);
    } catch {
      return new Response("Bad media URL", { status: 400 });
    }
    if (url.hostname !== "media") return new Response("Media not found", { status: 404 });
    const parts = safePreviewPathname(url.pathname);
    if (!parts || parts.length !== 2) return new Response("Invalid media path", { status: 400 });
    const [token] = parts;
    const record = this.records.get(token);
    if (!record || Date.now() > record.expiresAt) {
      return new Response("Media source expired", { status: 410 });
    }
    if (request.method === "OPTIONS") {
      return new Response(null, {
        status: 204,
        headers: {
          "Access-Control-Allow-Headers": "Range",
          "Access-Control-Allow-Methods": "GET, HEAD, OPTIONS",
          "Access-Control-Allow-Origin": "*",
        },
      });
    }
    if (request.method !== "GET" && request.method !== "HEAD") {
      return new Response("Method not allowed", { status: 405 });
    }
    try {
      const [currentRealPath, stat] = await Promise.all([
        fs.realpath(record.path),
        fs.stat(record.path),
      ]);
      if (
        currentRealPath !== record.path ||
        !stat.isFile() ||
        stat.size !== record.byteLength
      ) {
        return new Response("Media source changed", { status: 410 });
      }
    } catch {
      return new Response("Media source unavailable", { status: 410 });
    }
    record.expiresAt = Date.now() + ARTIFACT_MEDIA_TTL_MS;
    const range = parseSingleByteRange(request.headers.get("range"), record.byteLength);
    if (range === "unsatisfiable") {
      return new Response(null, {
        status: 416,
        headers: { "Content-Range": `bytes */${record.byteLength}` },
      });
    }
    const responseLength = range ? range.end - range.start + 1 : record.byteLength;
    const headers = new Headers({
      "Accept-Ranges": "bytes",
      "Access-Control-Allow-Headers": "Range",
      "Access-Control-Allow-Methods": "GET, HEAD, OPTIONS",
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Expose-Headers": "Accept-Ranges, Content-Length, Content-Range",
      "Cache-Control": "no-store",
      "Content-Length": String(responseLength),
      "Content-Type": record.mimeType,
      "X-Content-Type-Options": "nosniff",
    });
    if (range) {
      headers.set("Content-Range", `bytes ${range.start}-${range.end}/${record.byteLength}`);
    }
    return new Response(
      request.method === "HEAD"
        ? null
        : responseBodyForFile(record.path, range?.start, range?.end),
      { status: range ? 206 : 200, headers },
    );
  }

  private pruneExpired(): void {
    const now = Date.now();
    for (const [token, record] of this.records) {
      if (record.expiresAt > now) continue;
      this.records.delete(token);
      if (this.tokensByIdentity.get(record.identity) === token) {
        this.tokensByIdentity.delete(record.identity);
      }
    }
  }

  private pruneOverflow(): void {
    while (this.records.size > MAX_ARTIFACT_MEDIA_RECORDS) {
      const oldest = this.records.entries().next().value as
        | [string, ArtifactMediaRecord]
        | undefined;
      if (!oldest) return;
      this.records.delete(oldest[0]);
      if (this.tokensByIdentity.get(oldest[1].identity) === oldest[0]) {
        this.tokensByIdentity.delete(oldest[1].identity);
      }
    }
  }
}
