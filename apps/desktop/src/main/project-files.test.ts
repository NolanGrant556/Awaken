import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import type { RuntimeProject } from "../shared/runtime.js";
import {
  listProjectDirectory,
  resolveProjectEntry,
  resolveProjectFile,
  sameProjectFolderPath,
} from "./project-files.js";

const temporaryRoots: string[] = [];

async function temporaryRoot(): Promise<string> {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-project-files-"));
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
    createdAt: "2026-07-27T00:00:00.000Z",
    updatedAt: "2026-07-27T00:00:00.000Z",
    version: 1,
  };
}

afterEach(async () => {
  await Promise.all(
    temporaryRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

describe("project file directory listing", () => {
  it("matches Windows drive and UNC paths with native case-insensitive semantics", () => {
    expect(
      sameProjectFolderPath(
        String.raw`C:\Users\Nolan\Awaken Project`,
        "c:/users/nolan/awaken project/",
        "win32",
      ),
    ).toBe(true);
    expect(
      sameProjectFolderPath(
        String.raw`\\Server\Team Share\Awaken`,
        `${String.raw`\\server\team share\awaken`}\\`,
        "win32",
      ),
    ).toBe(true);
    expect(
      sameProjectFolderPath(String.raw`C:\Awaken`, String.raw`D:\Awaken`, "win32"),
    ).toBe(false);
    expect(
      sameProjectFolderPath(
        String.raw`\\?\C:\Users\Nolan\Awaken`,
        String.raw`C:\Users\Nolan\Awaken`,
        "win32",
      ),
    ).toBe(true);
    expect(
      sameProjectFolderPath(
        String.raw`\\?\UNC\Server\Team Share\Awaken`,
        String.raw`\\server\team share\awaken`,
        "win32",
      ),
    ).toBe(true);
  });

  it("lists attached project folders with directories first", async () => {
    const root = await temporaryRoot();
    await fs.mkdir(path.join(root, "src"));
    await fs.writeFile(path.join(root, "README.md"), "# Project\n");

    const result = await listProjectDirectory([project(root)], {
      projectId: "project-1",
      folderPath: root,
    });

    expect(result).toEqual({
      ok: true,
      relativePath: "",
      entries: [
        { name: "src", relativePath: "src", kind: "directory" },
        { name: "README.md", relativePath: "README.md", kind: "file" },
      ],
    });
  });

  it("lists a nested directory using portable relative paths", async () => {
    const root = await temporaryRoot();
    await fs.mkdir(path.join(root, "src"));
    await fs.writeFile(path.join(root, "src", "index.ts"), "export {};\n");

    const result = await listProjectDirectory([project(root)], {
      projectId: "project-1",
      folderPath: root,
      relativePath: "src",
    });

    expect(result).toMatchObject({
      ok: true,
      relativePath: "src",
      entries: [{ name: "index.ts", relativePath: "src/index.ts", kind: "file" }],
    });
  });

  it("rejects traversal and unattached folder requests", async () => {
    const root = await temporaryRoot();
    const other = await temporaryRoot();

    await expect(
      listProjectDirectory([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "../outside",
      }),
    ).resolves.toMatchObject({ ok: false, code: "PATH_OUTSIDE_PROJECT" });

    await expect(
      listProjectDirectory([project(root)], {
        projectId: "project-1",
        folderPath: other,
      }),
    ).resolves.toMatchObject({ ok: false, code: "FOLDER_NOT_ATTACHED" });
  });

  it("does not follow a directory symlink outside the registered root", async () => {
    const root = await temporaryRoot();
    const outside = await temporaryRoot();
    await fs.writeFile(path.join(outside, "secret.txt"), "private\n");
    await fs.symlink(outside, path.join(root, "outside-link"));

    await expect(
      listProjectDirectory([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "outside-link",
      }),
    ).resolves.toMatchObject({ ok: false, code: "PATH_OUTSIDE_PROJECT" });
  });

  it("resolves only regular files inside the currently attached project folder", async () => {
    const root = await temporaryRoot();
    const outside = await temporaryRoot();
    await fs.writeFile(path.join(root, "inside.txt"), "inside\n");
    await fs.writeFile(path.join(outside, "secret.txt"), "secret\n");
    await fs.symlink(path.join(outside, "secret.txt"), path.join(root, "secret-link.txt"));

    await expect(
      resolveProjectFile([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "inside.txt",
      }),
    ).resolves.toMatchObject({
      ok: true,
      file: { relativePath: "inside.txt", displayName: "inside.txt" },
    });
    await expect(
      resolveProjectFile([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "../secret.txt",
      }),
    ).resolves.toMatchObject({ ok: false, code: "PATH_OUTSIDE_PROJECT" });
    await expect(
      resolveProjectFile([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "secret-link.txt",
      }),
    ).resolves.toMatchObject({ ok: false });
  });

  it("resolves validated file and directory references for project entry actions", async () => {
    const root = await temporaryRoot();
    await fs.mkdir(path.join(root, "docs"));
    await fs.writeFile(path.join(root, "docs", "brief.md"), "brief\n");

    await expect(
      resolveProjectEntry([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "docs",
        kind: "directory",
      }),
    ).resolves.toMatchObject({
      ok: true,
      entry: {
        absolutePath: path.join(root, "docs"),
        displayName: "docs",
        kind: "directory",
      },
    });
    await expect(
      resolveProjectEntry([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "docs/brief.md",
        kind: "file",
      }),
    ).resolves.toMatchObject({
      ok: true,
      entry: { displayName: "brief.md", kind: "file" },
    });
  });

  it("rejects entry action requests for mismatched kinds and symbolic links", async () => {
    const root = await temporaryRoot();
    const outside = await temporaryRoot();
    await fs.mkdir(path.join(root, "docs"));
    await fs.writeFile(path.join(outside, "secret.txt"), "secret\n");
    await fs.symlink(path.join(outside, "secret.txt"), path.join(root, "secret-link.txt"));

    await expect(
      resolveProjectEntry([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "docs",
        kind: "file",
      }),
    ).resolves.toMatchObject({ ok: false, code: "ENTRY_TYPE_MISMATCH" });
    await expect(
      resolveProjectEntry([project(root)], {
        projectId: "project-1",
        folderPath: root,
        relativePath: "secret-link.txt",
        kind: "file",
      }),
    ).resolves.toMatchObject({ ok: false });
  });
});
