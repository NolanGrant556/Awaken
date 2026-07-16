import fs from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import {
  resolveSystemSkillsSourceRoot,
  resolveSystemSkillsTargetRoot,
  syncSystemSkills,
  SystemSkillsSyncError,
  type SystemSkillsManifest,
} from "./system-skills-importer.js";

const tempRoots: string[] = [];

async function tempRoot(): Promise<string> {
  const root = await fs.mkdtemp(path.join(tmpdir(), "awaken-system-skills-"));
  tempRoots.push(root);
  return root;
}

async function writeFile(root: string, relativePath: string, content: string): Promise<void> {
  const target = path.join(root, ...relativePath.split("/"));
  await fs.mkdir(path.dirname(target), { recursive: true });
  await fs.writeFile(target, content, "utf8");
}

async function readManifest(targetRoot: string): Promise<SystemSkillsManifest> {
  return JSON.parse(
    await fs.readFile(path.join(targetRoot, ".manifest.json"), "utf8"),
  ) as SystemSkillsManifest;
}

afterEach(async () => {
  await Promise.all(
    tempRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

describe("system Skills paths", () => {
  it("resolves development, packaged, and fixed target roots", () => {
    expect(
      resolveSystemSkillsSourceRoot({
        packaged: false,
        resourcesPath: "/Applications/Awaken.app/Contents/Resources",
        appPath: "/repo/apps/desktop",
      }),
    ).toBe(path.join("/repo/apps/desktop", "resources", "system-skills"));
    expect(
      resolveSystemSkillsSourceRoot({
        packaged: true,
        resourcesPath: "/Applications/Awaken.app/Contents/Resources",
        appPath: "/repo/apps/desktop",
      }),
    ).toBe(
      path.join(
        "/Applications/Awaken.app/Contents/Resources",
        "system-skills",
      ),
    );
    expect(resolveSystemSkillsTargetRoot("/Users/example")).toBe(
      path.join("/Users/example", ".awaken", "skills", "system-skills"),
    );
  });
});

describe("syncSystemSkills", () => {
  it("installs a deterministic manifest and returns unchanged on a verified match", async () => {
    const root = await tempRoot();
    const sourceRoot = path.join(root, "source");
    const targetRoot = path.join(root, "home", ".awaken", "skills", "system-skills");
    await writeFile(sourceRoot, "word/SKILL.md", "word skill");
    await writeFile(sourceRoot, "word/references/guide.md", "guide");

    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" }),
    ).resolves.toMatchObject({ status: "installed", fileCount: 2 });
    expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
      .toBe("word skill");
    const manifest = await readManifest(targetRoot);
    expect(manifest.schemaVersion).toBe(1);
    expect(manifest.appVersion).toBe("1.0.0");
    expect(Object.keys(manifest.files)).toEqual([
      "word/SKILL.md",
      "word/references/guide.md",
    ]);

    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" }),
    ).resolves.toMatchObject({ status: "unchanged", fileCount: 2 });
  });

  it("updates for app version, source changes, and every target divergence", async () => {
    const root = await tempRoot();
    const sourceRoot = path.join(root, "source");
    const targetRoot = path.join(root, "target");
    await writeFile(sourceRoot, "README.md", "resource root");
    await writeFile(sourceRoot, "word/SKILL.md", "v1");
    await syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" });

    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
    ).resolves.toMatchObject({ status: "updated" });
    expect((await readManifest(targetRoot)).appVersion).toBe("2.0.0");

    await writeFile(sourceRoot, "word/SKILL.md", "v2");
    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
    ).resolves.toMatchObject({ status: "updated" });
    expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
      .toBe("v2");

    await writeFile(targetRoot, "word/SKILL.md", "tampered");
    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
    ).resolves.toMatchObject({ status: "updated" });
    expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
      .toBe("v2");

    await fs.rm(path.join(targetRoot, "word", "SKILL.md"));
    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
    ).resolves.toMatchObject({ status: "updated" });
    expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
      .toBe("v2");

    await writeFile(targetRoot, "extra.txt", "extra");
    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
    ).resolves.toMatchObject({ status: "updated" });
    await expect(fs.stat(path.join(targetRoot, "extra.txt"))).rejects.toMatchObject({
      code: "ENOENT",
    });
  });

  it("supports an empty packaged resource root", async () => {
    const root = await tempRoot();
    const sourceRoot = path.join(root, "source");
    const targetRoot = path.join(root, "target");
    await fs.mkdir(sourceRoot, { recursive: true });

    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" }),
    ).resolves.toMatchObject({ status: "installed", fileCount: 0 });
    expect(await fs.readdir(targetRoot)).toEqual([".manifest.json"]);
    expect((await readManifest(targetRoot)).files).toEqual({});
  });

  it("recovers an interrupted backup before comparing manifests", async () => {
    const root = await tempRoot();
    const sourceRoot = path.join(root, "source");
    const parent = path.join(root, "home", ".awaken", "skills");
    const targetRoot = path.join(parent, "system-skills");
    await writeFile(sourceRoot, "word/SKILL.md", "stable");
    await syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" });

    const backupRoot = path.join(parent, ".system-skills.backup-interrupted");
    const stagingRoot = path.join(parent, ".system-skills.stage-interrupted");
    await fs.rename(targetRoot, backupRoot);
    await writeFile(stagingRoot, "partial.txt", "partial");

    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" }),
    ).resolves.toMatchObject({ status: "unchanged" });
    expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
      .toBe("stable");
    await expect(fs.stat(stagingRoot)).rejects.toMatchObject({ code: "ENOENT" });
    await expect(fs.stat(backupRoot)).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("rolls back to the last complete target when directory promotion fails", async () => {
    const root = await tempRoot();
    const sourceRoot = path.join(root, "source");
    const targetRoot = path.join(root, "target");
    await writeFile(sourceRoot, "word/SKILL.md", "old");
    await syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" });
    await writeFile(sourceRoot, "word/SKILL.md", "new");

    await expect(
      syncSystemSkills({
        sourceRoot,
        targetRoot,
        appVersion: "2.0.0",
        deps: {
          rename: async (from, to) => {
            if (path.basename(from.toString()).startsWith(".system-skills.stage-")) {
              throw new Error("injected promotion failure");
            }
            await fs.rename(from, to);
          },
          uuid: (() => {
            let value = 0;
            return () => `test-${value++}`;
          })(),
        },
      }),
    ).rejects.toMatchObject({ code: "SYSTEM_SKILLS_SYNC_FAILED" });
    expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
      .toBe("old");
    expect((await readManifest(targetRoot)).appVersion).toBe("1.0.0");
  });

  it("preserves the existing target when the source is missing", async () => {
    const root = await tempRoot();
    const sourceRoot = path.join(root, "source");
    const targetRoot = path.join(root, "target");
    await writeFile(sourceRoot, "word/SKILL.md", "stable");
    await syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" });
    await fs.rm(sourceRoot, { recursive: true, force: true });

    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
    ).rejects.toBeInstanceOf(SystemSkillsSyncError);
    await expect(
      syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
    ).rejects.toMatchObject({ code: "SYSTEM_SKILLS_SOURCE_UNAVAILABLE" });
    expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
      .toBe("stable");
  });

  it.skipIf(process.platform === "win32")(
    "rejects source symlinks without replacing the existing target",
    async () => {
      const root = await tempRoot();
      const sourceRoot = path.join(root, "source");
      const targetRoot = path.join(root, "target");
      await writeFile(sourceRoot, "word/SKILL.md", "stable");
      await syncSystemSkills({ sourceRoot, targetRoot, appVersion: "1.0.0" });
      await fs.symlink(
        path.join(sourceRoot, "word", "SKILL.md"),
        path.join(sourceRoot, "word", "linked.md"),
      );

      await expect(
        syncSystemSkills({ sourceRoot, targetRoot, appVersion: "2.0.0" }),
      ).rejects.toMatchObject({ code: "SYSTEM_SKILLS_SOURCE_INVALID" });
      expect(await fs.readFile(path.join(targetRoot, "word", "SKILL.md"), "utf8"))
        .toBe("stable");
    },
  );
});
