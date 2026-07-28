import fs from "node:fs/promises";
import path from "node:path";
import type {
  ProjectDirectoryEntry,
  ProjectDirectoryListRequest,
  ProjectDirectoryListResult,
  ProjectFilePreviewActionRequest,
  ProjectFilePreviewErrorCode,
  RuntimeProject,
} from "../shared/runtime.js";

export type ProjectDirectoryScope = Pick<
  RuntimeProject,
  "projectId" | "state" | "folders"
>;

function portableRelativePath(parts: string[]): string {
  return parts.join("/");
}

function relativePathParts(relativePath: string | undefined): string[] | null {
  const candidate = relativePath?.trim() ?? "";
  if (!candidate || candidate === ".") {
    return [];
  }
  if (
    candidate.includes("\0") ||
    path.isAbsolute(candidate) ||
    /^[A-Za-z]:[\\/]/.test(candidate)
  ) {
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
    (!relative.startsWith(`..${path.sep}`) &&
      relative !== ".." &&
      !path.isAbsolute(relative))
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

export type ResolveProjectFileResult =
  | { ok: true; file: ResolvedProjectFile }
  | {
      ok: false;
      code: ProjectFilePreviewErrorCode;
      message: string;
    };

/**
 * Resolves one regular file through the same active-project and attached-folder
 * authority as the directory tree. A symlink is never a preview capability,
 * even when its current target happens to remain inside the project folder.
 */
export async function resolveProjectFile(
  projects: ProjectDirectoryScope[],
  request: ProjectFilePreviewActionRequest,
): Promise<ResolveProjectFileResult> {
  if (
    !request ||
    typeof request.projectId !== "string" ||
    !request.projectId.trim() ||
    typeof request.folderPath !== "string" ||
    !request.folderPath.trim() ||
    typeof request.relativePath !== "string" ||
    !request.relativePath.trim()
  ) {
    return {
      ok: false,
      code: "INVALID_REQUEST",
      message: "文件预览请求无效。",
    };
  }

  const project = projects.find(
    (candidate) =>
      candidate.projectId === request.projectId && candidate.state === "active",
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
      message: "只能查看当前项目文件夹内的文件。",
    };
  }

  try {
    const registeredRoot = path.resolve(folder.path);
    const absolutePath = path.resolve(registeredRoot, ...parts);
    if (!isPathInside(registeredRoot, absolutePath)) {
      return {
        ok: false,
        code: "PATH_OUTSIDE_PROJECT",
        message: "只能查看当前项目文件夹内的文件。",
      };
    }

    const [realRoot, realPath, lexicalStat] = await Promise.all([
      fs.realpath(registeredRoot),
      fs.realpath(absolutePath),
      fs.lstat(absolutePath),
    ]);
    if (
      lexicalStat.isSymbolicLink() ||
      !lexicalStat.isFile() ||
      !isPathInside(realRoot, realPath)
    ) {
      return {
        ok: false,
        code: lexicalStat.isFile() ? "PATH_OUTSIDE_PROJECT" : "FILE_NOT_FOUND",
        message: lexicalStat.isFile()
          ? "只能查看当前项目文件夹内的文件。"
          : "所选项目文件不存在。",
      };
    }

    return {
      ok: true,
      file: {
        projectId: project.projectId,
        registeredRoot,
        realRoot,
        absolutePath,
        realPath,
        relativePath: portableRelativePath(parts),
        displayName: parts[parts.length - 1] ?? request.relativePath,
      },
    };
  } catch (error) {
    const code =
      typeof error === "object" &&
      error !== null &&
      "code" in error &&
      (error.code === "ENOENT" || error.code === "ENOTDIR")
        ? "FILE_NOT_FOUND"
        : "FILE_UNREADABLE";
    return {
      ok: false,
      code,
      message:
        code === "FILE_NOT_FOUND"
          ? "所选项目文件不存在。"
          : "这个文件暂时无法读取。",
    };
  }
}

function entryKind(entry: {
  isDirectory(): boolean;
  isSymbolicLink(): boolean;
}): ProjectDirectoryEntry["kind"] {
  if (entry.isSymbolicLink()) return "symlink";
  if (entry.isDirectory()) return "directory";
  return "file";
}

function sortEntries(
  left: ProjectDirectoryEntry,
  right: ProjectDirectoryEntry,
): number {
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
    (request.relativePath !== undefined &&
      typeof request.relativePath !== "string")
  ) {
    return {
      ok: false,
      code: "INVALID_REQUEST",
      message: "文件目录请求无效。",
    };
  }

  const project = projects.find(
    (candidate) =>
      candidate.projectId === request.projectId && candidate.state === "active",
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
