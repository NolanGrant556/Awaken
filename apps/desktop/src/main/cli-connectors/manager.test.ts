import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { c as createTar } from "tar";
import {
  CliConnectorManager,
  authVerificationUrl,
  readResponseBody,
  safeCliEnvironment,
} from "./manager.js";
import type { CliConnectorReadiness } from "../../shared/runtime.js";

const originalPath = process.env.PATH;

afterEach(() => {
  process.env.PATH = originalPath;
});

describe("CliConnectorManager", () => {
  it("only accepts provider-owned HTTPS authorization URLs", () => {
    expect(authVerificationUrl(
      '{"verification_url":"https://login.dingtalk.com/oauth2/auth?state=1"}',
      "dingtalk-workspace",
    )).toBe("https://login.dingtalk.com/oauth2/auth?state=1");
    expect(authVerificationUrl(
      "Open https://accounts.feishu.cn/device?code=abc to continue",
      "lark-cli",
    )).toBe("https://accounts.feishu.cn/device?code=abc");
    expect(authVerificationUrl("https://evil.example/auth", "lark-cli")).toBeUndefined();
    expect(authVerificationUrl("http://open.feishu.cn/auth", "lark-cli")).toBeUndefined();
  });

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
    const skillSource = path.join(root, "skill-source");
    await fs.mkdir(path.join(skillSource, "official", "skills", "dws"), { recursive: true });
    await fs.writeFile(path.join(skillSource, "official", "skills", "dws", "SKILL.md"), "---\nname: dws\n---\n# DWS\n");
    const skillArchive = path.join(root, "dws-skills.tar.gz");
    await createTar({ cwd: skillSource, file: skillArchive, gzip: true }, ["official"]);
    const skillArchiveBytes = await fs.readFile(skillArchive);
    const skillChecksum = createHash("sha256").update(skillArchiveBytes).digest("hex");
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
      skillBundles: [{
        providerId: "dingtalk-workspace",
        version: "1.0.32",
        url: "https://github.com/example/dws-skills.tar.gz",
        sha256: skillChecksum,
        archiveType: "tar.gz",
        archiveRoot: "official/skills/dws",
        destinationRoot: "dws",
        skillNames: ["dws"],
      }],
    }));
    process.env.PATH = "";
    const published: CliConnectorReadiness[] = [];
    const stateChanges = vi.fn();
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async (url) => new Response(url.includes("skills") ? skillArchiveBytes : archiveBytes, { status: 200 })),
      publishReadiness: vi.fn(async (readiness) => { published.push(readiness); }),
      onStateChanged: stateChanges,
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
    await expect(fs.readFile(path.join(root, ".agents", "skills", "dws", "SKILL.md"), "utf8"))
      .resolves.toContain("name: dws");
    expect(published.some((entry) => entry.providerId === "dingtalk-workspace")).toBe(true);
    const progressEvents = stateChanges.mock.calls
      .map(([view]) => view.installProgress?.percent)
      .filter((percent): percent is number => typeof percent === "number");
    expect(progressEvents.length).toBeGreaterThan(3);
    expect(progressEvents).toEqual([...progressEvents].sort((a, b) => a - b));
    expect(stateChanges.mock.calls.some(([view]) => view.installProgress?.phase === "downloading_skills")).toBe(true);
    await manager.dispose();

    await fs.rm(path.join(root, ".agents", "skills", "dws"), { recursive: true, force: true });
    const restarted = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async (url) => new Response(url.includes("skills") ? skillArchiveBytes : archiveBytes, { status: 200 })),
      publishReadiness: vi.fn(async () => undefined),
      onStateChanged: vi.fn(),
    });
    await restarted.initialize();
    await expect(fs.readFile(path.join(root, ".agents", "skills", "dws", "SKILL.md"), "utf8"))
      .resolves.toContain("name: dws");
    await restarted.dispose();
  });

  it("cancels an in-flight install without leaving a failed or activated connector", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-cancel-"));
    const manifestPath = path.join(root, "manifest.json");
    await fs.writeFile(manifestPath, JSON.stringify({
      revision: "test.cancel",
      assets: [{
        providerId: "dingtalk-workspace",
        version: "1.0.32",
        platform: "darwin",
        architecture: "arm64",
        url: "https://github.com/example/dws.tar.gz",
        sha256: "0".repeat(64),
        archiveType: "tar.gz",
        executableName: "dws",
        archiveExecutablePath: "dws",
      }],
      skillBundles: [{
        providerId: "dingtalk-workspace",
        version: "1.0.32",
        url: "https://github.com/example/dws-skills.tar.gz",
        sha256: "0".repeat(64),
        archiveType: "tar.gz",
        archiveRoot: "official/skills/dws",
        destinationRoot: "dws",
        skillNames: ["dws"],
      }],
    }));
    process.env.PATH = "";
    const stateChanges = vi.fn();
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn((_url, signal) => new Promise<Response>((_resolve, reject) => {
        signal.addEventListener("abort", () => {
          const error = new Error("aborted");
          error.name = "AbortError";
          reject(error);
        }, { once: true });
      })),
      publishReadiness: vi.fn(async () => undefined),
      onStateChanged: stateChanges,
    });
    await manager.initialize();

    const installing = manager.install("dingtalk-workspace");
    await vi.waitFor(() => {
      expect(stateChanges.mock.calls.some(([view]) => view.installProgress?.phase === "downloading_cli")).toBe(true);
    });
    await manager.cancelInstall("dingtalk-workspace");
    const cancelled = await installing;

    expect(cancelled.readiness).toMatchObject({ install: "missing", overall: "not_ready" });
    expect(cancelled.readiness.safeError).toBeUndefined();
    expect(cancelled.installProgress).toBeUndefined();
    await expect(fs.stat(path.join(root, ".awaken", "runtime", "cli", "bin", "dws"))).rejects.toMatchObject({ code: "ENOENT" });
    await manager.dispose();
  });

  it("keeps Lark onboarding in Main, opens official pages, and chains config into login", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-lark-onboarding-"));
    const source = path.join(root, "source");
    await fs.mkdir(source);
    const executable = path.join(source, "lark-cli");
    await fs.writeFile(executable, `#!/bin/sh
state_dir="$(dirname "$0")/state"
mkdir -p "$state_dir"
if [ "$1" = "--version" ]; then
  printf 'lark-cli version 1.0.71\\n'
elif [ "$1" = "config" ] && [ "$2" = "init" ]; then
  printf 'Open https://open.feishu.cn/app/onboarding?flow=test to continue\\n'
  while [ ! -f "$state_dir/config-approved" ]; do sleep 0.05; done
  touch "$state_dir/configured"
elif [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  if [ ! -f "$state_dir/configured" ]; then
    printf '{"ok":false,"type":"config"}\\n' >&2
    exit 1
  fi
  if [ -f "$state_dir/authorized" ]; then
    printf '{"ok":true,"verified":true,"identities":{"user":{"userName":"Nolan"}}}\\n'
  else
    printf '{"ok":false,"type":"auth"}\\n' >&2
    exit 1
  fi
elif [ "$1" = "auth" ] && [ "$2" = "login" ]; then
  case " $* " in
    *" --no-wait "*)
      printf '{"verification_url":"https://accounts.feishu.cn/device?code=abc","device_code":"device-1"}\\n'
      ;;
    *" --device-code "*)
      while [ ! -f "$state_dir/auth-approved" ]; do sleep 0.05; done
      touch "$state_dir/authorized"
      ;;
  esac
else
  printf '{"ok":true}\\n'
fi
`, { mode: 0o700 });
    const archive = path.join(root, "lark-cli.tar.gz");
    await createTar({ cwd: source, file: archive, gzip: true }, ["lark-cli"]);
    const archiveBytes = await fs.readFile(archive);
    const checksum = createHash("sha256").update(archiveBytes).digest("hex");
    const larkSkillSource = path.join(root, "lark-skill-source");
    await fs.mkdir(path.join(larkSkillSource, "cli-1.0.71", "skills", "lark-im"), { recursive: true });
    await fs.writeFile(path.join(larkSkillSource, "cli-1.0.71", "skills", "lark-im", "SKILL.md"), "# Lark IM\n");
    const larkSkillArchive = path.join(root, "lark-skills.tar.gz");
    await createTar({ cwd: larkSkillSource, file: larkSkillArchive, gzip: true }, ["cli-1.0.71"]);
    const larkSkillBytes = await fs.readFile(larkSkillArchive);
    const larkSkillChecksum = createHash("sha256").update(larkSkillBytes).digest("hex");
    const manifestPath = path.join(root, "manifest.json");
    await fs.writeFile(manifestPath, JSON.stringify({
      revision: "test.1",
      assets: [{
        providerId: "lark-cli",
        version: "1.0.71",
        platform: "darwin",
        architecture: "arm64",
        url: "https://github.com/example/lark-cli.tar.gz",
        sha256: checksum,
        archiveType: "tar.gz",
        executableName: "lark-cli",
        archiveExecutablePath: "lark-cli",
      }],
      skillBundles: [{
        providerId: "lark-cli",
        version: "1.0.71",
        url: "https://github.com/example/lark-skills.tar.gz",
        sha256: larkSkillChecksum,
        archiveType: "tar.gz",
        archiveRoot: "cli-1.0.71/skills",
        skillNames: ["lark-im"],
      }],
    }));
    process.env.PATH = "/bin:/usr/bin";
    const openedUrls: string[] = [];
    const stateDir = path.join(
      root,
      ".awaken",
      "runtime",
      "cli",
      "lark-cli",
      "1.0.71",
      "darwin-arm64",
      "state",
    );
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async (url) => new Response(url.includes("skills") ? larkSkillBytes : archiveBytes, { status: 200 })),
      openExternal: vi.fn(async (url) => {
        openedUrls.push(url);
        await fs.mkdir(stateDir, { recursive: true });
        await fs.writeFile(
          path.join(stateDir, url.includes("onboarding") ? "config-approved" : "auth-approved"),
          "ok",
        );
      }),
      publishReadiness: vi.fn(async () => undefined),
      onStateChanged: vi.fn(),
    });
    await manager.initialize();
    const installed = await manager.install("lark-cli");
    expect(installed.readiness).toMatchObject({ configuration: "required", authentication: "required" });
    await expect(fs.readFile(path.join(root, ".agents", "skills", "lark-im", "SKILL.md"), "utf8"))
      .resolves.toBe("# Lark IM\n");

    const started = await manager.startAuth("lark-cli");
    expect(started.authSession).toMatchObject({ provider: "lark-cli", phase: "waiting_user" });
    for (let attempt = 0; attempt < 100; attempt += 1) {
      if (manager.list().find((entry) => entry.providerId === "lark-cli")?.readiness.overall === "ready") break;
      await new Promise((resolve) => setTimeout(resolve, 25));
    }

    expect(openedUrls).toEqual([
      "https://open.feishu.cn/app/onboarding?flow=test",
      "https://accounts.feishu.cn/device?code=abc",
    ]);
    expect(manager.list().find((entry) => entry.providerId === "lark-cli")).toMatchObject({
      readiness: {
        configuration: "ready",
        authentication: "authorized",
        overall: "ready",
        account: { displayName: "Nolan" },
      },
      authSession: undefined,
    });
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
    const accountSkillSource = path.join(root, "account-skill-source");
    await fs.mkdir(path.join(accountSkillSource, "dws"), { recursive: true });
    await fs.writeFile(path.join(accountSkillSource, "dws", "SKILL.md"), "# DWS\n");
    const accountSkillArchive = path.join(root, "account-skills.tar.gz");
    await createTar({ cwd: accountSkillSource, file: accountSkillArchive, gzip: true }, ["dws"]);
    const accountSkillBytes = await fs.readFile(accountSkillArchive);
    const accountSkillChecksum = createHash("sha256").update(accountSkillBytes).digest("hex");
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
      skillBundles: [{
        providerId: "dingtalk-workspace",
        version: "1.0.32",
        url: "https://github.com/example/account-skills.tar.gz",
        sha256: accountSkillChecksum,
        archiveType: "tar.gz",
        archiveRoot: "dws",
        destinationRoot: "dws",
        skillNames: ["dws"],
      }],
    }));
    process.env.PATH = "";
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async (url) => new Response(url.includes("skills") ? accountSkillBytes : archiveBytes, { status: 200 })),
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
