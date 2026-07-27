import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import type { RuntimeProject } from "../shared/runtime.js";
import { listProjectDirectory } from "./project-files.js";

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
    temporaryRoots.splice(0).map((root) =>
      fs.rm(root, { recursive: true, force: true }),
    ),
  );
});

describe("project file directory listing", () => {
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
      entries: [
        { name: "index.ts", relativePath: "src/index.ts", kind: "file" },
      ],
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
});
