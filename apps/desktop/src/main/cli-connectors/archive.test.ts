import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { c as createTar } from "tar";
import { extractSingleExecutable, safeArchivePath } from "./archive.js";

const temporaryRoots: string[] = [];

afterEach(async () => {
  await Promise.all(temporaryRoots.splice(0).map((root) => fs.rm(root, { recursive: true, force: true })));
});

describe("CLI archive extraction", () => {
  it.each(["../dws", "bin/../dws", "/tmp/dws", "bin//dws", "bin/./dws", "bin\\..\\dws"])(
    "rejects unsafe archive path %s",
    (value) => {
      expect(() => safeArchivePath(value)).toThrow("CLI_ARCHIVE_UNSAFE_PATH");
    },
  );

  it("allows directory entries but extracts only the declared executable", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-archive-"));
    temporaryRoots.push(root);
    const source = path.join(root, "source");
    await fs.mkdir(path.join(source, "bin"), { recursive: true });
    await fs.writeFile(path.join(source, "bin", "dws"), "verified executable", { mode: 0o700 });
    await fs.writeFile(path.join(source, "README"), "not executable");
    const archive = path.join(root, "asset.tar.gz");
    await createTar({ cwd: source, file: archive, gzip: true }, ["bin", "README"]);
    const destination = path.join(root, "staging", "dws");

    await extractSingleExecutable(archive, "tar.gz", "bin/dws", destination);

    await expect(fs.readFile(destination, "utf8")).resolves.toBe("verified executable");
    await expect(fs.stat(path.join(root, "staging", "README"))).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("rejects symlink entries anywhere in a tar archive", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-symlink-"));
    temporaryRoots.push(root);
    const source = path.join(root, "source");
    await fs.mkdir(source);
    await fs.writeFile(path.join(source, "dws"), "verified executable", { mode: 0o700 });
    await fs.symlink("dws", path.join(source, "alias"));
    const archive = path.join(root, "asset.tar.gz");
    await createTar({ cwd: source, file: archive, gzip: true }, ["alias", "dws"]);

    await expect(
      extractSingleExecutable(archive, "tar.gz", "dws", path.join(root, "staging", "dws")),
    ).rejects.toThrow("CLI_ARCHIVE_UNSAFE_ENTRY");
  });
});
