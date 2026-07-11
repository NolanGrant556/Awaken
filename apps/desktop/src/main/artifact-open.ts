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
]);

const terminalStatuses = new Set(["finished", "error", "cancelled"]);

function failed(
  code: Exclude<ArtifactOpenResult, { ok: true }>["code"],
  message: string,
): ArtifactOpenResult {
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

function isPathWithin(root: string, target: string): boolean {
  const relative = path.relative(root, target);
  return (
    relative === "" ||
    (!path.isAbsolute(relative) &&
      relative !== ".." &&
      !relative.startsWith(`..${path.sep}`))
  );
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
  const event = foldedArtifactEvent(runEntry.events, request.artifactId);
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

  const configuredRoots = transcript.thread.workspace.outputRoots?.length
    ? transcript.thread.workspace.outputRoots
    : [workspaceRoot];
  const realRoots = (
    await Promise.all(
      configuredRoots.map(async (root) => {
        try {
          return await fs.realpath(path.resolve(root));
        } catch {
          return undefined;
        }
      }),
    )
  ).filter((root): root is string => Boolean(root));
  if (!realRoots.some((root) => isPathWithin(root, realTarget))) {
    return {
      ok: false,
      result: failed("outside_output_roots", "文件不在当前任务允许的输出目录中。"),
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
