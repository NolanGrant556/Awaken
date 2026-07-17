import { promises as fs } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { cleanupLegacyPrimaryRuntime } from "./legacy-runtime-cleanup.js";

const roots: string[] = [];

async function userData(): Promise<string> {
  const root = await fs.mkdtemp(join(tmpdir(), "awaken-legacy-runtime-"));
  roots.push(root);
  return root;
}

afterEach(async () => {
  await Promise.all(roots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })));
});

describe("legacy primary runtime cleanup", () => {
  it("removes only the retired Awaken runtime and records an idempotent migration", async () => {
    const root = await userData();
    const legacy = join(root, "runtimes", "primary-runtime", "versions", "0.3.0");
    await fs.mkdir(legacy, { recursive: true });
    await fs.writeFile(join(legacy, "payload"), "legacy");
    await fs.mkdir(join(root, "runtimes", "keep-me"), { recursive: true });

    await expect(cleanupLegacyPrimaryRuntime(root)).resolves.toMatchObject({ status: "removed" });
    await expect(fs.stat(join(root, "runtimes", "primary-runtime"))).rejects.toMatchObject({ code: "ENOENT" });
    await expect(fs.stat(join(root, "runtimes", "keep-me"))).resolves.toBeTruthy();
    await expect(cleanupLegacyPrimaryRuntime(root)).resolves.toMatchObject({ status: "already_migrated" });
  });

  it("records an absent runtime without creating a replacement", async () => {
    const root = await userData();
    await expect(cleanupLegacyPrimaryRuntime(root)).resolves.toMatchObject({ status: "absent" });
    await expect(fs.stat(join(root, "runtimes", "primary-runtime"))).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("removes read-only directories from archived app bundles", async () => {
    const root = await userData();
    const contents = join(root, "runtimes", "primary-runtime", "LibreOffice.app", "Contents");
    await fs.mkdir(contents, { recursive: true });
    await fs.writeFile(join(contents, "Info.plist"), "fixture");
    await fs.chmod(contents, 0o555);

    await expect(cleanupLegacyPrimaryRuntime(root)).resolves.toMatchObject({ status: "removed" });
    await expect(fs.stat(join(root, "runtimes", "primary-runtime"))).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("refuses a symlink root instead of following it", async () => {
    const root = await userData();
    const external = await fs.mkdtemp(join(tmpdir(), "awaken-legacy-external-"));
    roots.push(external);
    await fs.writeFile(join(external, "keep"), "safe");
    await fs.mkdir(join(root, "runtimes"), { recursive: true });
    try {
      await fs.symlink(external, join(root, "runtimes", "primary-runtime"));
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "EPERM") {
        return;
      }
      throw error;
    }

    await expect(cleanupLegacyPrimaryRuntime(root)).resolves.toMatchObject({ status: "refused" });
    await expect(fs.readFile(join(external, "keep"), "utf8")).resolves.toBe("safe");
  });
});
