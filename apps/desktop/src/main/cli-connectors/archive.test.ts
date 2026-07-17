import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { c as createTar } from "tar";
import { extractSingleExecutable, extractSkillBundle, safeArchivePath } from "./archive.js";

const flatSkillZip = "UEsDBAoAAAAAAC6b8VwAAAAAAAAAAAAAAAALABwAcmVmZXJlbmNlcy9VVAkAAygRWmooEVpqdXgLAAEE9QEAAAQAAAAAUEsDBAoAAAAAAC6b8VwE5Me2CgAAAAoAAAATABwAcmVmZXJlbmNlcy9ndWlkZS5tZFVUCQADKBFaaigRWmp1eAsAAQT1AQAABAAAAAByZWZlcmVuY2UKUEsDBAoAAAAAAC6b8VxinFJxGAAAABgAAAAIABwAU0tJTEwubWRVVAkAAygRWmooEVpqdXgLAAEE9QEAAAQAAAAALS0tCm5hbWU6IGR3cwotLS0KIyBEV1MKUEsBAh4DCgAAAAAALpvxXAAAAAAAAAAAAAAAAAsAGAAAAAAAAAAQAO1BAAAAAHJlZmVyZW5jZXMvVVQFAAMoEVpqdXgLAAEE9QEAAAQAAAAAUEsBAh4DCgAAAAAALpvxXATkx7YKAAAACgAAABMAGAAAAAAAAQAAAKSBRQAAAHJlZmVyZW5jZXMvZ3VpZGUubWRVVAUAAygRWmp1eAsAAQT1AQAABAAAAABQSwECHgMKAAAAAAAum/FcYpxScRgAAAAYAAAACAAYAAAAAAABAAAApIGcAAAAU0tJTEwubWRVVAUAAygRWmp1eAsAAQT1AQAABAAAAABQSwUGAAAAAAMAAwD4AAAA9gAAAAAA";

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

  it("extracts a flat official zip into one declared global skill", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-skill-zip-"));
    temporaryRoots.push(root);
    const archive = path.join(root, "skills.zip");
    await fs.writeFile(archive, Buffer.from(flatSkillZip, "base64"));

    await extractSkillBundle(archive, "zip", {
      archiveRoot: "",
      destinationRoot: "dws",
      skillNames: ["dws"],
    }, path.join(root, "payload"));

    await expect(fs.readFile(path.join(root, "payload", "dws", "SKILL.md"), "utf8"))
      .resolves.toContain("name: dws");
    await expect(fs.readFile(path.join(root, "payload", "dws", "references", "guide.md"), "utf8"))
      .resolves.toBe("reference\n");
  });

  it("extracts only the declared skills from a repository tag archive", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-skill-tar-"));
    temporaryRoots.push(root);
    const source = path.join(root, "source");
    await fs.mkdir(path.join(source, "cli-1.0.71", "skills", "lark-im"), { recursive: true });
    await fs.mkdir(path.join(source, "cli-1.0.71", "internal", "testdata", "lark-demo"), { recursive: true });
    await fs.writeFile(path.join(source, "cli-1.0.71", "skills", "lark-im", "SKILL.md"), "# Lark IM\n");
    await fs.writeFile(path.join(source, "cli-1.0.71", "internal", "testdata", "lark-demo", "SKILL.md"), "# Ignore\n");
    const archive = path.join(root, "skills.tar.gz");
    await createTar({ cwd: source, file: archive, gzip: true }, ["cli-1.0.71"]);

    await extractSkillBundle(archive, "tar.gz", {
      archiveRoot: "cli-1.0.71/skills",
      skillNames: ["lark-im"],
    }, path.join(root, "payload"));

    await expect(fs.readFile(path.join(root, "payload", "lark-im", "SKILL.md"), "utf8"))
      .resolves.toBe("# Lark IM\n");
    await expect(fs.stat(path.join(root, "payload", "lark-demo"))).rejects.toMatchObject({ code: "ENOENT" });
  });
});
