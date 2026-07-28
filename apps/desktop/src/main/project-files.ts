import fs from "node:fs/promises";
import path from "node:path";
import type {
  ProjectDirectoryEntry,
  ProjectDirectoryListRequest,
  ProjectDirectoryListResult,
  ProjectEntryActionErrorCode,
  ProjectEntryActionRequest,
  ProjectEntryKind,
  ProjectFilePreviewActionRequest,
  ProjectFilePreviewErrorCode,
  RuntimeProject,
} from "../shared/runtime.js";

export type ProjectDirectoryScope = Pick<RuntimeProject, "projectId" | "state" | "folders">;

function portableRelativePath(parts: string[]): string {
  return parts.join("/");
}

function relativePathParts(relativePath: string | undefined): string[] | null {
  const candidate = relativePath?.trim() ?? "";
  if (!candidate || candidate === ".") {
    return [];
  }
  if (candidate.includes("\0") || path.isAbsolute(candidate) || /^[A-Za-z]:[\\/]/.test(candidate)) {
    return null;
  }
  const parts = candidate.replaceAll("\\", "/").split("/");
  if (parts.some((part) => part === "..")) {
    return null;
  }
  return parts.filter((part) => part && part !== ".");
}

function isPathInside(root: string, candidate: string): boolean {
  const relative = path.relative(root, candidate);
  return (
    relative === "" ||
    (!relative.startsWith(`..${path.sep}`) && relative !== ".." && !path.isAbsolute(relative))
  );
}

function sameResolvedPath(left: string, right: string): boolean {
  return path.resolve(left) === path.resolve(right);
}

export type ResolvedProjectFile = {
  projectId: string;
  registeredRoot: string;
  realRoot: string;
  absolutePath: string;
  realPath: string;
  relativePath: string;
  displayName: string;
};

export type ResolvedProjectEntry = ResolvedProjectFile & {
  kind: ProjectEntryKind;
};

export type ResolveProjectEntryResult =
  | { ok: true; entry: ResolvedProjectEntry }
  | {
      ok: false;
      code: ProjectEntryActionErrorCode;
      message: string;
    };

export type ResolveProjectFileResult =
  | { ok: true; file: ResolvedProjectFile }
  | {
      ok: false;
      code: ProjectFilePreviewErrorCode;
      message: string;
    };

function previewErrorCodeFromEntryError(
  code: ProjectEntryActionErrorCode,
): ProjectFilePreviewErrorCode {
  switch (code) {
    case "ENTRY_NOT_FOUND":
    case "ENTRY_TYPE_MISMATCH":
      return "FILE_NOT_FOUND";
    case "ENTRY_UNREADABLE":
      return "FILE_UNREADABLE";
    case "INVALID_REQUEST":
    case "UNAUTHORIZED":
    case "PROJECT_NOT_FOUND":
    case "FOLDER_NOT_ATTACHED":
    case "PATH_OUTSIDE_PROJECT":
      return code;
    default:
      return "FILE_UNREADABLE";
  }
}

/**
 * Resolves one file or directory through the active-project attachment
 * authority. Renderer identity is only a request hint: Main rechecks lexical
 * containment, realpath containment, symlinks, existence, and the actual kind.
 */
export async function resolveProjectEntry(
  projects: ProjectDirectoryScope[],
  request: ProjectEntryActionRequest,
): Promise<ResolveProjectEntryResult> {
  if (
    !request ||
    typeof request.projectId !== "string" ||
    !request.projectId.trim() ||
    typeof request.folderPath !== "string" ||
    !request.folderPath.trim() ||
    typeof request.relativePath !== "string" ||
    !request.relativePath.trim() ||
    (request.kind !== "file" && request.kind !== "directory")
  ) {
    return {
      ok: false,
      code: "INVALID_REQUEST",
      message: "文件操作请求无效。",
    };
  }

  const project = projects.find(
    (candidate) => candidate.projectId === request.projectId && candidate.state === "active",
  );
  if (!project) {
    return {
      ok: false,
      code: "PROJECT_NOT_FOUND",
      message: "当前项目不存在或已被移除。",
    };
  }

  const folder = project.folders.find((candidate) =>
    sameResolvedPath(candidate.path, request.folderPath),
  );
  if (!folder) {
    return {
      ok: false,
      code: "FOLDER_NOT_ATTACHED",
      message: "这个文件夹不属于当前项目。",
    };
  }

  const parts = relativePathParts(request.relativePath);
  if (!parts || parts.length === 0) {
    return {
      ok: false,
      code: "PATH_OUTSIDE_PROJECT",
      message: "只能操作当前项目文件夹内的文件或目录。",
    };
  }

  try {
    const registeredRoot = path.resolve(folder.path);
    const absolutePath = path.resolve(registeredRoot, ...parts);
    if (!isPathInside(registeredRoot, absolutePath)) {
      return {
        ok: false,
        code: "PATH_OUTSIDE_PROJECT",
        message: "只能操作当前项目文件夹内的文件或目录。",
      };
    }

    const [realRoot, realPath, lexicalStat] = await Promise.all([
      fs.realpath(registeredRoot),
      fs.realpath(absolutePath),
      fs.lstat(absolutePath),
    ]);
    if (lexicalStat.isSymbolicLink() || !isPathInside(realRoot, realPath)) {
      return {
        ok: false,
        code: "PATH_OUTSIDE_PROJECT",
        message: "只能操作当前项目文件夹内的文件或目录。",
      };
    }

    const actualKind: ProjectEntryKind | undefined = lexicalStat.isDirectory()
      ? "directory"
      : lexicalStat.isFile()
        ? "file"
        : undefined;
    if (!actualKind) {
      return {
        ok: false,
        code: "ENTRY_NOT_FOUND",
        message: "所选项目文件或目录不存在。",
      };
    }
    if (actualKind !== request.kind) {
      return {
        ok: false,
        code: "ENTRY_TYPE_MISMATCH",
        message: "所选项目条目的类型已经发生变化，请刷新后重试。",
      };
    }

    return {
      ok: true,
      entry: {
        projectId: project.projectId,
        registeredRoot,
        realRoot,
        absolutePath,
        realPath,
        relativePath: portableRelativePath(parts),
        displayName: parts[parts.length - 1] ?? request.relativePath,
        kind: actualKind,
      },
    };
  } catch (error) {
    const missing =
      typeof error === "object" &&
      error !== null &&
      "code" in error &&
      (error.code === "ENOENT" || error.code === "ENOTDIR");
    return {
      ok: false,
      code: missing ? "ENTRY_NOT_FOUND" : "ENTRY_UNREADABLE",
      message: missing ? "所选项目文件或目录不存在。" : "这个项目文件或目录暂时无法读取。",
    };
  }
}

/**
 * Resolves one regular file through the same active-project and attached-folder
 * authority as the directory tree. A symlink is never a preview capability,
 * even when its current target happens to remain inside the project folder.
 */
export async function resolveProjectFile(
  projects: ProjectDirectoryScope[],
  request: ProjectFilePreviewActionRequest,
): Promise<ResolveProjectFileResult> {
  const result = await resolveProjectEntry(projects, {
    ...request,
    kind: "file",
  });
  if (!result.ok) {
    return {
      ok: false,
      code: previewErrorCodeFromEntryError(result.code),
      message: result.message,
    };
  }
  const { kind: _kind, ...file } = result.entry;
  return { ok: true, file };
}

function entryKind(entry: {
  isDirectory(): boolean;
  isSymbolicLink(): boolean;
}): ProjectDirectoryEntry["kind"] {
  if (entry.isSymbolicLink()) return "symlink";
  if (entry.isDirectory()) return "directory";
  return "file";
}

function sortEntries(left: ProjectDirectoryEntry, right: ProjectDirectoryEntry): number {
  if (left.kind === "directory" && right.kind !== "directory") return -1;
  if (left.kind !== "directory" && right.kind === "directory") return 1;
  return left.name.localeCompare(right.name, undefined, {
    numeric: true,
    sensitivity: "base",
  });
}

/**
 * Lists one directory inside a folder currently attached to an active project.
 * The registered folder and every requested descendant are revalidated in Main;
 * renderer-provided paths are never treated as authorization.
 */
export async function listProjectDirectory(
  projects: ProjectDirectoryScope[],
  request: ProjectDirectoryListRequest,
): Promise<ProjectDirectoryListResult> {
  if (
    !request ||
    typeof request.projectId !== "string" ||
    !request.projectId.trim() ||
    typeof request.folderPath !== "string" ||
    !request.folderPath.trim() ||
    (request.relativePath !== undefined && typeof request.relativePath !== "string")
  ) {
    return {
      ok: false,
      code: "INVALID_REQUEST",
      message: "文件目录请求无效。",
    };
  }

  const project = projects.find(
    (candidate) => candidate.projectId === request.projectId && candidate.state === "active",
  );
  if (!project) {
    return {
      ok: false,
      code: "PROJECT_NOT_FOUND",
      message: "当前项目不存在或已被移除。",
    };
  }

  const folder = project.folders.find((candidate) =>
    sameResolvedPath(candidate.path, request.folderPath),
  );
  if (!folder) {
    return {
      ok: false,
      code: "FOLDER_NOT_ATTACHED",
      message: "这个文件夹不属于当前项目。",
    };
  }

  const parts = relativePathParts(request.relativePath);
  if (!parts) {
    return {
      ok: false,
      code: "PATH_OUTSIDE_PROJECT",
      message: "只能查看当前项目文件夹内的目录。",
    };
  }

  try {
    const registeredRoot = path.resolve(folder.path);
    const lexicalTarget = path.resolve(registeredRoot, ...parts);
    if (!isPathInside(registeredRoot, lexicalTarget)) {
      return {
        ok: false,
        code: "PATH_OUTSIDE_PROJECT",
        message: "只能查看当前项目文件夹内的目录。",
      };
    }

    const [realRoot, realTarget] = await Promise.all([
      fs.realpath(registeredRoot),
      fs.realpath(lexicalTarget),
    ]);
    if (!isPathInside(realRoot, realTarget)) {
      return {
        ok: false,
        code: "PATH_OUTSIDE_PROJECT",
        message: "只能查看当前项目文件夹内的目录。",
      };
    }

    const directoryEntries = await fs.readdir(realTarget, {
      withFileTypes: true,
    });
    const relativePath = portableRelativePath(parts);
    const entries = directoryEntries
      .map<ProjectDirectoryEntry>((entry) => ({
        name: entry.name,
        relativePath: portableRelativePath([...parts, entry.name]),
        kind: entryKind(entry),
      }))
      .sort(sortEntries);

    return { ok: true, relativePath, entries };
  } catch {
    return {
      ok: false,
      code: "DIRECTORY_UNREADABLE",
      message: "这个目录暂时无法读取。",
    };
  }
}
