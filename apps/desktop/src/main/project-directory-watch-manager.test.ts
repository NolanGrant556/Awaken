import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RuntimeProject } from "../shared/runtime.js";
import {
  ProjectDirectoryWatchManager,
  type ProjectDirectoryWatchFactory,
} from "./project-directory-watch-manager.js";

const temporaryRoots: string[] = [];

async function temporaryRoot(): Promise<string> {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-project-watch-"));
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
    createdAt: "2026-08-14T00:00:00.000Z",
    updatedAt: "2026-08-14T00:00:00.000Z",
    version: 1,
  };
}

afterEach(async () => {
  vi.useRealTimers();
  await Promise.all(
    temporaryRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

describe("ProjectDirectoryWatchManager", () => {
  it("debounces project-folder changes and releases the watcher for its owner", async () => {
    vi.useFakeTimers();
    const root = await temporaryRoot();
    let emitChange = (): void => {};
    const close = vi.fn();
    const createWatcher: ProjectDirectoryWatchFactory = (_rootPath, onChange) => {
      emitChange = onChange;
      return { close };
    };
    const notify = vi.fn();
    const manager = new ProjectDirectoryWatchManager(
      notify,
      createWatcher,
      50,
      () => "watch-1",
    );

    await expect(
      manager.subscribe(
        [project(root)],
        { projectId: "project-1", folderPath: root },
        17,
      ),
    ).resolves.toEqual({ ok: true, subscriptionId: "watch-1" });

    emitChange();
    emitChange();
    await vi.advanceTimersByTimeAsync(49);
    expect(notify).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(notify).toHaveBeenCalledOnce();
    expect(notify).toHaveBeenCalledWith({
      webContentsId: 17,
      event: { subscriptionId: "watch-1" },
    });

    manager.unsubscribe("watch-1", 99);
    expect(close).not.toHaveBeenCalled();
    manager.releaseForWebContents(17);
    expect(close).toHaveBeenCalledOnce();
  });

  it("rejects folders that are not currently attached to the active project", async () => {
    const root = await temporaryRoot();
    const other = await temporaryRoot();
    const createWatcher = vi.fn<ProjectDirectoryWatchFactory>();
    const manager = new ProjectDirectoryWatchManager(vi.fn(), createWatcher);

    await expect(
      manager.subscribe(
        [project(root)],
        { projectId: "project-1", folderPath: other },
        17,
      ),
    ).resolves.toMatchObject({ ok: false, code: "FOLDER_NOT_ATTACHED" });
    expect(createWatcher).not.toHaveBeenCalled();
  });

  it("returns a stable typed error when the platform watcher cannot start", async () => {
    const root = await temporaryRoot();
    const manager = new ProjectDirectoryWatchManager(vi.fn(), () => {
      throw new Error("watch unavailable");
    });

    await expect(
      manager.subscribe(
        [project(root)],
        { projectId: "project-1", folderPath: root },
        17,
      ),
    ).resolves.toEqual({
      ok: false,
      code: "WATCH_UNAVAILABLE",
      message: "文件变化暂时无法自动同步。",
    });
  });
});
