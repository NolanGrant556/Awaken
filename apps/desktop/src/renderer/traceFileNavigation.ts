import { normalizeTracePath } from "./toolTracePresentation";
import {
  projectFileTabId,
  type ProjectFileWorkspaceTab,
} from "./projectFileTabs";

/** Produces a project-scoped request, never an authorization or filesystem read. */
export function traceFileTarget(
  projectId: string | undefined,
  folders: Array<{ path: string; isPrimary?: boolean }>,
  path: string,
  startLine?: number,
  endLine?: number,
): ProjectFileWorkspaceTab | undefined {
  if (!projectId || !path.trim() || path.includes("\0")) return undefined;
  const normalized = normalizeTracePath(path);
  const absolute = /^(?:\/|[A-Za-z]:\/)/.test(normalized);
  if (!absolute && path.replace(/\\/g, "/").split("/").includes(".."))
    return undefined;
  const candidates = absolute
    ? folders
        .filter((folder) =>
          normalized.startsWith(
            `${normalizeTracePath(folder.path).replace(/\/$/, "")}/`,
          ),
        )
        .sort((a, b) => b.path.length - a.path.length)
    : folders.filter((folder) => folder.isPrimary);
  const folder = candidates[0];
  if (!folder) return undefined;
  const relativePath = absolute
    ? normalized.slice(
        normalizeTracePath(folder.path).replace(/\/$/, "").length + 1,
      )
    : normalized;
  if (!relativePath || relativePath.split("/").includes("..")) return undefined;
  return {
    id: projectFileTabId(projectId, folder.path, relativePath),
    projectId,
    folderPath: folder.path,
    relativePath,
    name: relativePath.split("/").at(-1)!,
    mode: "files",
    ...(Number.isInteger(startLine) && startLine! > 0
      ? {
          lineRequest: {
            id: crypto.randomUUID(),
            startLine: startLine!,
            ...(Number.isInteger(endLine) && endLine! >= startLine!
              ? { endLine }
              : {}),
          },
        }
      : {}),
  };
}
