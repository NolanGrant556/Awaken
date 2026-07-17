import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { c as createTar } from "tar";
import { CliConnectorManager, readResponseBody, safeCliEnvironment } from "./manager.js";
import type { CliConnectorReadiness } from "../../shared/runtime.js";

const originalPath = process.env.PATH;

afterEach(() => {
  process.env.PATH = originalPath;
});

describe("CliConnectorManager", () => {
  it("passes only a non-secret environment allowlist to official CLI processes", () => {
    const previousSecret = process.env.OPENAI_API_KEY;
    process.env.OPENAI_API_KEY = "must-not-leak";
    try {
      const env = safeCliEnvironment();
      expect(env.OPENAI_API_KEY).toBeUndefined();
      expect(env.PATH).toBe(process.env.PATH);
    } finally {
      if (previousSecret === undefined) delete process.env.OPENAI_API_KEY;
      else process.env.OPENAI_API_KEY = previousSecret;
    }
  });

  it("rejects an unbounded response stream before retaining bytes past the limit", async () => {
    const response = new Response(new Uint8Array([1, 2, 3, 4, 5]));
    await expect(readResponseBody(response, 4)).rejects.toMatchObject({ code: "CLI_ARCHIVE_TOO_LARGE" });
  });

  it("installs a pinned verified archive and reports auth-required readiness", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-manager-"));
    const source = path.join(root, "source");
    await fs.mkdir(source);
    const executable = path.join(source, "dws");
    await fs.writeFile(executable, `#!/bin/sh
if [ "$1" = "version" ]; then
  printf '{"version":"v1.0.32"}\n'
elif [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  printf '{"success":true,"authenticated":false}\n'
else
  printf '{"ok":true}\n'
fi
`, { mode: 0o700 });
    const archive = path.join(root, "dws.tar.gz");
    await createTar({ cwd: source, file: archive, gzip: true }, ["dws"]);
    const archiveBytes = await fs.readFile(archive);
    const checksum = createHash("sha256").update(archiveBytes).digest("hex");
    const manifestPath = path.join(root, "manifest.json");
    await fs.writeFile(manifestPath, JSON.stringify({
      revision: "test.1",
      assets: [{
        providerId: "dingtalk-workspace",
        version: "1.0.32",
        platform: "darwin",
        architecture: "arm64",
        url: "https://github.com/example/dws.tar.gz",
        sha256: checksum,
        archiveType: "tar.gz",
        executableName: "dws",
        archiveExecutablePath: "dws",
      }],
    }));
    process.env.PATH = "";
    const published: CliConnectorReadiness[] = [];
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async () => new Response(archiveBytes, { status: 200 })),
      publishReadiness: vi.fn(async (readiness) => { published.push(readiness); }),
      onStateChanged: vi.fn(),
    });
    await manager.initialize();
    const installed = await manager.install("dingtalk-workspace");
    expect(installed.readiness).toMatchObject({
      install: "installed",
      executable: "available",
      authentication: "required",
      overall: "action_required",
      version: "1.0.32",
      ownership: "awaken-managed",
    });
    await expect(fs.stat(path.join(root, ".awaken", "runtime", "cli", "bin", "dws"))).resolves.toBeDefined();
    expect(published.some((entry) => entry.providerId === "dingtalk-workspace")).toBe(true);
    await manager.dispose();
  });


  it("loads DingTalk account profile from contact user get-self after auth", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-account-"));
    const source = path.join(root, "source");
    await fs.mkdir(source);
    const executable = path.join(source, "dws");
    await fs.writeFile(executable, `#!/bin/sh
if [ "$1" = "version" ]; then
  printf '{"version":"v1.0.32"}\n'
elif [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  printf '{"success":true,"authenticated":true,"corp_id":"dingxxxx"}\n'
elif [ "$1" = "contact" ] && [ "$2" = "user" ] && [ "$3" = "get-self" ]; then
  printf '{"success":true,"result":[{"orgEmployeeModel":{"orgUserName":"冯智森","orgName":"达利国际华南区"}}]}\n'
else
  printf '{"ok":true}\n'
fi
`, { mode: 0o700 });
    const archive = path.join(root, "dws.tar.gz");
    await createTar({ cwd: source, file: archive, gzip: true }, ["dws"]);
    const archiveBytes = await fs.readFile(archive);
    const checksum = createHash("sha256").update(archiveBytes).digest("hex");
    const manifestPath = path.join(root, "manifest.json");
    await fs.writeFile(manifestPath, JSON.stringify({
      revision: "test.1",
      assets: [{
        providerId: "dingtalk-workspace",
        version: "1.0.32",
        platform: "darwin",
        architecture: "arm64",
        url: "https://github.com/example/dws.tar.gz",
        sha256: checksum,
        archiveType: "tar.gz",
        executableName: "dws",
        archiveExecutablePath: "dws",
      }],
    }));
    process.env.PATH = "";
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async () => new Response(archiveBytes, { status: 200 })),
      publishReadiness: vi.fn(async () => undefined),
      onStateChanged: vi.fn(),
    });
    await manager.initialize();
    const installed = await manager.install("dingtalk-workspace");
    expect(installed.readiness).toMatchObject({
      authentication: "authorized",
      overall: "ready",
      account: {
        displayName: "冯智森",
        organization: "达利国际华南区",
      },
    });
    await manager.dispose();
  });

  it("fails closed when the platform has no manifest asset", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-unsupported-"));
    const manifestPath = path.join(root, "manifest.json");
    await fs.writeFile(manifestPath, JSON.stringify({ revision: "test.1", assets: [] }));
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "linux",
      architecture: "x64",
      manifestPath,
      fetchAsset: vi.fn(),
      publishReadiness: vi.fn(async () => undefined),
      onStateChanged: vi.fn(),
    });
    await manager.initialize();
    expect(manager.list().every((entry) => entry.readiness.install === "unsupported")).toBe(true);
    await expect(manager.install("lark-cli")).rejects.toMatchObject({ code: "CLI_UNSUPPORTED" });
    await manager.dispose();
  });
});
