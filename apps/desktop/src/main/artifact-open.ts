import fs from "node:fs/promises";
import path from "node:path";
import type { AguiEvent, AguiArtifactRole } from "../shared/agui.js";
import type {
  ArtifactOpenRequest,
  ArtifactOpenResult,
  RuntimeThreadTranscript,
} from "../shared/runtime.js";

const supportedExtensions = new Set([
  ".doc",
  ".docx",
  ".md",
  ".xls",
  ".xlsx",
  ".csv",
  ".tsv",
  ".pdf",
  ".ppt",
  ".pptx",
  ".png",
  ".jpg",
  ".jpeg",
  ".webp",
  ".mp4",
]);

const mediaExtensionTypes = new Map<string, { kind: "image" | "video"; mimeType: string }>([
  [".png", { kind: "image", mimeType: "image/png" }],
  [".jpg", { kind: "image", mimeType: "image/jpeg" }],
  [".jpeg", { kind: "image", mimeType: "image/jpeg" }],
  [".webp", { kind: "image", mimeType: "image/webp" }],
  [".mp4", { kind: "video", mimeType: "video/mp4" }],
]);

const terminalStatuses = new Set(["finished", "error", "cancelled"]);

function failed(
  code: Exclude<ArtifactOpenResult, { ok: true }>["code"],
  message: string,
): Exclude<ArtifactOpenResult, { ok: true }> {
  return { ok: false, code, message };
}

function legacyRole(event: AguiEvent): AguiArtifactRole {
  const producer = (event.artifactProducer || event.toolCallName || "").toLowerCase();
  const extension = path.extname(event.artifactPath || "").toLowerCase();
  if (
    producer === "presentation_render" ||
    producer === "artifact_render" ||
    producer.endsWith("_render")
  ) {
    return "preview";
  }
  if (producer === "presentation_create" || producer === "presentation_edit") {
    return extension === ".ppt" || extension === ".pptx"
      ? "deliverable"
      : "preview";
  }
  if (producer === "document_create" || producer === "document_edit") {
    return extension === ".doc" || extension === ".docx" || extension === ".md"
      ? "deliverable"
      : "support";
  }
  if (producer === "spreadsheet_create" || producer === "spreadsheet_edit") {
    return [".xls", ".xlsx", ".csv", ".tsv"].includes(extension)
      ? "deliverable"
      : "support";
  }
  if (
    producer === "artifact_export" ||
    producer === "artifact_publish" ||
    producer === "write_file" ||
    (producer === "text_file_create" && extension === ".md")
  ) {
    return supportedExtensions.has(extension) ? "deliverable" : "support";
  }
  return event.artifactKind === "image" ? "preview" : "support";
}

function foldedArtifactEvent(
  events: AguiEvent[],
  artifactId: string,
): AguiEvent | undefined {
  let folded: AguiEvent | undefined;
  for (const event of events) {
    if (
      (event.type === "AWAKEN_ARTIFACT_CREATED" ||
        event.type === "AWAKEN_ARTIFACT_UPDATED") &&
      event.artifactId === artifactId
    ) {
      folded = {
        ...folded,
        ...event,
        artifactId,
        artifactTitle: event.artifactTitle ?? folded?.artifactTitle,
        artifactDescription:
          event.artifactDescription ?? folded?.artifactDescription,
        artifactKind: event.artifactKind ?? folded?.artifactKind,
        artifactPath: event.artifactPath ?? folded?.artifactPath,
        artifactContent: event.artifactContent ?? folded?.artifactContent,
        artifactStatus: event.artifactStatus ?? folded?.artifactStatus,
        artifactRole: event.artifactRole ?? folded?.artifactRole,
        artifactContractVersion:
          event.artifactContractVersion ?? folded?.artifactContractVersion,
        artifactProducer: event.artifactProducer ?? folded?.artifactProducer,
        toolCallName: event.toolCallName ?? folded?.toolCallName,
      };
    }
  }
  return folded;
}

function mediaArtifactEventFromToolResult(
  events: AguiEvent[],
  artifactPath?: string,
): AguiEvent | undefined {
  const requestedPath = artifactPath?.trim();
  if (!requestedPath) return undefined;

  for (const event of events) {
    if (
      event.type !== "TOOL_CALL_RESULT" ||
      (event.toolCallName !== "image_generate" && event.toolCallName !== "video_generate")
    ) {
      continue;
    }
    let payload: {
      status?: unknown;
      generation_status?: unknown;
      paths?: unknown;
      items?: unknown;
    };
    try {
      payload = JSON.parse(event.content ?? "{}") as typeof payload;
    } catch {
      continue;
    }
    const status = payload.generation_status ?? payload.status;
    if (status !== "succeeded" && status !== "partial") continue;

    const readyPaths = new Set<string>();
    if (Array.isArray(payload.items)) {
      for (const candidate of payload.items) {
        if (!candidate || typeof candidate !== "object") continue;
        const item = candidate as { status?: unknown; path?: unknown };
        if (item.status === "ready" && typeof item.path === "string" && item.path.trim()) {
          readyPaths.add(item.path.trim());
        }
      }
    }
    if (Array.isArray(payload.paths)) {
      for (const candidate of payload.paths) {
        if (typeof candidate === "string" && candidate.trim()) readyPaths.add(candidate.trim());
      }
    }
    if (!readyPaths.has(requestedPath)) continue;

    const kind = event.toolCallName === "video_generate" ? "video" : "image";
    return {
      type: "AWAKEN_ARTIFACT_CREATED",
      artifactId: `legacy-media:${requestedPath}`,
      artifactPath: requestedPath,
      artifactTitle: path.basename(requestedPath),
      artifactKind: kind,
      artifactStatus: "ready",
      artifactRole: "deliverable",
      artifactContractVersion: 2,
      artifactProducer: event.toolCallName,
      toolCallName: event.toolCallName,
    };
  }
  return undefined;
}

function recordedArtifactEvent(
  events: AguiEvent[],
  request: ArtifactOpenRequest,
): AguiEvent | undefined {
  return (
    foldedArtifactEvent(events, request.artifactId) ??
    mediaArtifactEventFromToolResult(events, request.artifactPath)
  );
}

export async function resolveArtifactTarget(
  transcript: RuntimeThreadTranscript,
  request: ArtifactOpenRequest,
): Promise<{ ok: true; path: string } | { ok: false; result: ArtifactOpenResult }> {
  if (transcript.thread.threadId !== request.threadId) {
    return {
      ok: false,
      result: failed("artifact_not_recorded", "没有找到这个交付文件记录。"),
    };
  }
  const runEntry = transcript.runs.find((entry) => entry.run.runId === request.runId);
  if (!runEntry) {
    return {
      ok: false,
      result: failed("artifact_not_recorded", "没有找到这个任务的交付文件记录。"),
    };
  }
  if (!terminalStatuses.has(runEntry.run.status)) {
    return {
      ok: false,
      result: failed("run_not_terminal", "任务尚未结束，暂时不能打开交付文件。"),
    };
  }
  const event = recordedArtifactEvent(runEntry.events, request);
  if (!event || !event.artifactPath) {
    return {
      ok: false,
      result: failed("artifact_not_recorded", "没有找到这个交付文件记录。"),
    };
  }
  const role = event.artifactRole || legacyRole(event);
  if (role !== "deliverable" || event.artifactStatus !== "ready") {
    return {
      ok: false,
      result: failed("artifact_not_deliverable", "该文件不是已就绪的最终交付物。"),
    };
  }
  const extension = path.extname(event.artifactPath).toLowerCase();
  if (!supportedExtensions.has(extension)) {
    return {
      ok: false,
      result: failed("unsupported_type", "暂不支持直接打开这种文件格式。"),
    };
  }

  const workspaceRoot = transcript.thread.workspace.workspaceRoot;
  const candidate = path.isAbsolute(event.artifactPath)
    ? path.resolve(event.artifactPath)
    : path.resolve(workspaceRoot, event.artifactPath);
  let realTarget: string;
  try {
    realTarget = await fs.realpath(candidate);
  } catch {
    return {
      ok: false,
      result: failed("file_not_found", "文件已移动、删除或当前无法访问。"),
    };
  }
  const realExtension = path.extname(realTarget).toLowerCase();
  if (
    !supportedExtensions.has(realExtension) ||
    realExtension !== extension
  ) {
    return {
      ok: false,
      result: failed("unsupported_type", "文件的真实格式与交付记录不一致。"),
    };
  }

  try {
    const stat = await fs.stat(realTarget);
    if (!stat.isFile()) {
      return {
        ok: false,
        result: failed("not_a_file", "交付物路径不是普通文件。"),
      };
    }
  } catch {
    return {
      ok: false,
      result: failed("file_not_found", "文件已移动、删除或当前无法访问。"),
    };
  }
  return { ok: true, path: realTarget };
}

export async function resolveArtifactMediaTarget(
  transcript: RuntimeThreadTranscript,
  request: ArtifactOpenRequest,
): Promise<
  | { ok: true; path: string; mimeType: string; byteLength: number; fileName: string }
  | { ok: false; result: Exclude<ArtifactOpenResult, { ok: true }> }
> {
  if (transcript.thread.threadId !== request.threadId) {
    return { ok: false, result: failed("artifact_not_recorded", "没有找到这个媒体交付物记录。") };
  }
  const runEntry = transcript.runs.find((entry) => entry.run.runId === request.runId);
  if (!runEntry) {
    return { ok: false, result: failed("artifact_not_recorded", "没有找到这个任务的媒体交付物记录。") };
  }
  const event = recordedArtifactEvent(runEntry.events, request);
  if (!event || !event.artifactPath) {
    return { ok: false, result: failed("artifact_not_recorded", "没有找到这个媒体交付物记录。") };
  }
  const role = event.artifactRole || legacyRole(event);
  if (role !== "deliverable" || event.artifactStatus !== "ready") {
    return { ok: false, result: failed("artifact_not_deliverable", "该媒体不是已就绪的最终交付物。") };
  }
  const extension = path.extname(event.artifactPath).toLowerCase();
  const mediaType = mediaExtensionTypes.get(extension);
  if (!mediaType || event.artifactKind !== mediaType.kind) {
    return { ok: false, result: failed("unsupported_type", "暂不支持预览这种媒体格式。") };
  }

  const workspaceRoot = transcript.thread.workspace.workspaceRoot;
  const candidate = path.isAbsolute(event.artifactPath)
    ? path.resolve(event.artifactPath)
    : path.resolve(workspaceRoot, event.artifactPath);
  let realTarget: string;
  try {
    realTarget = await fs.realpath(candidate);
  } catch {
    return { ok: false, result: failed("file_not_found", "媒体文件已移动、删除或当前无法访问。") };
  }
  if (path.extname(realTarget).toLowerCase() !== extension) {
    return { ok: false, result: failed("unsupported_type", "媒体文件格式与交付记录不一致。") };
  }
  try {
    const stat = await fs.stat(realTarget);
    if (!stat.isFile()) {
      return { ok: false, result: failed("not_a_file", "媒体交付物路径不是普通文件。") };
    }
    return {
      ok: true,
      path: realTarget,
      mimeType: mediaType.mimeType,
      byteLength: stat.size,
      fileName: path.basename(realTarget),
    };
  } catch {
    return { ok: false, result: failed("file_not_found", "媒体文件已移动、删除或当前无法访问。") };
  }
}

export async function openRecordedArtifact(
  transcript: RuntimeThreadTranscript,
  request: ArtifactOpenRequest,
  openPath: (target: string) => Promise<string>,
): Promise<ArtifactOpenResult> {
  const resolved = await resolveArtifactTarget(transcript, request);
  if (!resolved.ok) {
    return resolved.result;
  }
  try {
    const error = await openPath(resolved.path);
    if (error) {
      const normalized = error.toLowerCase();
      return normalized.includes("application") || normalized.includes("association")
        ? failed("no_default_application", "系统没有为该文件格式设置默认打开应用。")
        : failed("open_failed", "文件打开失败，请稍后重试。" );
    }
    return { ok: true };
  } catch {
    return failed("open_failed", "文件打开失败，请稍后重试。" );
  }
}

export async function revealRecordedArtifact(
  transcript: RuntimeThreadTranscript,
  request: ArtifactOpenRequest,
  reveal: (target: string) => void,
): Promise<ArtifactOpenResult> {
  const resolved = await resolveArtifactTarget(transcript, request);
  if (!resolved.ok) {
    return resolved.result;
  }
  try {
    reveal(resolved.path);
    return { ok: true };
  } catch {
    return failed("open_failed", "无法在文件夹中显示该文件。" );
  }
}
