import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { c as createTar } from "tar";
import {
  authVerificationUrl,
  readResponseBody,
  safeCliEnvironment,
} from "./manager.js";
import { CliConnectorManager } from "./core.test-helper.js";
import type { CliConnectorReadiness } from "../../shared/runtime.js";

const originalPath = process.env.PATH;

afterEach(() => {
  process.env.PATH = originalPath;
});

describe("CliConnectorManager", () => {
  it("only accepts provider-owned user-facing HTTPS authorization URLs", () => {
    expect(authVerificationUrl(
      '{"verification_url":"https://login.dingtalk.com/oauth2/auth?state=1"}',
      "dingtalk-workspace",
    )).toBe("https://login.dingtalk.com/oauth2/auth?state=1");
    expect(authVerificationUrl(
      "GET https://mcp.dingtalk.com/cli/clientId returned 200",
      "dingtalk-workspace",
    )).toBeUndefined();
    expect(authVerificationUrl(
      "Open https://login.dingtalk.com/oauth2/logout to continue",
      "dingtalk-workspace",
    )).toBeUndefined();
    expect(authVerificationUrl(
      "GET https://mcp.dingtalk.com/cli/clientId succeeded; open https://login.dingtalk.com/oauth2/auth?state=1",
      "dingtalk-workspace",
    )).toBe("https://login.dingtalk.com/oauth2/auth?state=1");
    expect(authVerificationUrl(
      "Open https://evil.login.dingtalk.com/oauth2/auth?state=1",
      "dingtalk-workspace",
    )).toBeUndefined();
    expect(authVerificationUrl(
      "Open https://open.feishu.cn/page/cli?user_code=abc to continue",
      "lark-cli",
    )).toBe("https://open.feishu.cn/page/cli?user_code=abc");
    expect(authVerificationUrl(
      "Open https://accounts.feishu.cn/device?code=abc to continue",
      "lark-cli",
    )).toBe("https://accounts.feishu.cn/device?code=abc");
    expect(authVerificationUrl(
      '{"verification_uri_complete":"https://open.larksuite.com/page/cli?user_code=abc"}',
      "lark-cli",
    )).toBe("https://open.larksuite.com/page/cli?user_code=abc");
    expect(authVerificationUrl(
      "POST https://accounts.feishu.cn/oauth/v1/app/registration failed: EOF",
      "lark-cli",
    )).toBeUndefined();
    expect(authVerificationUrl(
      "POST https://accounts.feishu.cn/oauth/v1/device_authorization failed: EOF",
      "lark-cli",
    )).toBeUndefined();
    expect(authVerificationUrl(
      "POST https://open.feishu.cn/open-apis/authen/v2/oauth/token failed: EOF",
      "lark-cli",
    )).toBeUndefined();
    expect(authVerificationUrl(
      "POST https://accounts.larksuite.com/oauth/v1/app/registration failed: EOF",
      "lark-cli",
    )).toBeUndefined();
    expect(authVerificationUrl("https://evil.example/auth", "lark-cli")).toBeUndefined();
    expect(authVerificationUrl("http://open.feishu.cn/auth", "lark-cli")).toBeUndefined();
  });

  it("passes only a non-secret environment allowlist to official CLI processes", () => {
    const env = safeCliEnvironment({
      PATH: "/test/bin",
      ALL_PROXY: "socks5://127.0.0.1:7897",
      all_proxy: "socks5://127.0.0.1:7897",
      OPENAI_API_KEY: "must-not-leak",
    });
    expect(env.OPENAI_API_KEY).toBeUndefined();
    expect(env.PATH).toBe("/test/bin");
    expect(env.ALL_PROXY).toBe("socks5://127.0.0.1:7897");
    expect(env.all_proxy).toBe("socks5://127.0.0.1:7897");
  });

  it("rejects an unbounded response stream before retaining bytes past the limit", async () => {
    const response = new Response(new Uint8Array([1, 2, 3, 4, 5]));
    await expect(readResponseBody(response, 4)).rejects.toMatchObject({ code: "CLI_ARCHIVE_TOO_LARGE" });
  });

  it.each(["remove", "keep", "modified", "legacy", "rollback", "external", "missing", "failed-auth", "symlink", "logout-failed", "logout-still-authorized", "logout-status-error", "both-missing", "cleanup-failed", "move-interrupted"])("installs pinned assets and safely uninstalls: %s", async (scenario) => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-manager-"));
    const source = path.join(root, "source");
    await fs.mkdir(source);
    const executable = path.join(source, "dws");
    await fs.writeFile(executable, `#!/bin/sh
state_dir="${root}"
if [ "$1" = "version" ]; then
  printf '{"version":"v1.0.32"}\n'
elif [ "$1" = "auth" ] && [ "$2" = "logout" ]; then
  echo logout >> "$state_dir/operations"
  if [ -f "$state_dir/logout-failed" ]; then exit 1; fi
  if [ ! -f "$state_dir/logout-still-authorized" ]; then /bin/rm -f "$state_dir/authorized"; fi
  printf '{"ok":true}\n'
elif [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  if [ -f "$state_dir/logout-status-error" ]; then printf '{}\n'; exit 1; fi
  if [ -f "$state_dir/authorized" ]; then printf '{"success":true,"authenticated":true}\n';
  else printf '{"success":true,"authenticated":false}\n'; fi
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
    const beforeNetworkOperation = vi.fn(async () => undefined);
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async (url) => new Response(url.includes("skills") ? skillArchiveBytes : archiveBytes, { status: 200 })),
      publishReadiness: vi.fn(async (readiness) => { published.push(readiness); }),
      onStateChanged: stateChanges,
      beforeNetworkOperation,
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
    await expect(fs.stat(path.join(root, ".awaken", "runtime", "cli", "bin", "dws"))).rejects.toMatchObject({ code: "ENOENT" });
    await expect(fs.readFile(path.join(root, ".agents", "skills", "dws", "SKILL.md"), "utf8"))
      .resolves.toContain("name: dws");
    expect(published.some((entry) => entry.providerId === "dingtalk-workspace")).toBe(true);
    const progressEvents = stateChanges.mock.calls
      .map(([view]) => view.installProgress?.percent)
      .filter((percent): percent is number => typeof percent === "number");
    expect(progressEvents).toEqual([]);
    expect(progressEvents).toEqual([...progressEvents].sort((a, b) => a - b));
    expect(stateChanges.mock.calls.some(([view]) => view.installProgress?.phase === "downloading_skills")).toBe(true);
    expect(beforeNetworkOperation).toHaveBeenCalledWith("cli-command");
    expect(beforeNetworkOperation).not.toHaveBeenCalledWith("cli-download");
    expect(beforeNetworkOperation).toHaveBeenCalledWith("cli-skill-download");

    const authRoot = path.join(root, ".awaken", "runtime", "auth");
    await fs.rm(authRoot, { recursive: true, force: true });
    const refreshedWithoutAuthRoot = await manager.refreshProvider("dingtalk-workspace");
    expect(refreshedWithoutAuthRoot.readiness).toMatchObject({
      install: "installed",
      executable: "available",
      authentication: "required",
      overall: "action_required",
    });
    expect(refreshedWithoutAuthRoot.readiness.safeError).toBeUndefined();
    await expect(fs.stat(authRoot)).resolves.toMatchObject({ mode: expect.any(Number) });

    await manager.dispose();
    await expect(fs.stat(authRoot)).resolves.toMatchObject({ mode: expect.any(Number) });

    await fs.rm(path.join(root, ".agents", "skills", "dws"), { recursive: true, force: true });
    const makeRestarted = () => new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(async (url) => new Response(url.includes("skills") ? skillArchiveBytes : archiveBytes, { status: 200 })),
      publishReadiness: vi.fn(async () => undefined),
      onStateChanged: vi.fn(),
    });
    let restarted = makeRestarted();
    await restarted.initialize();
    const skillFile = path.join(root, ".agents", "skills", "dws", "SKILL.md");
    await expect(fs.readFile(skillFile, "utf8")).resolves.toContain("name: dws");
    expect(restarted.list()[0].canUninstallManaged).toBe(true);
    const registryPath = path.join(root, ".awaken", "runtime", "cli", "connector-state.json");
    if (scenario === "modified") await fs.appendFile(skillFile, "user edits");
    if (scenario === "symlink") {
      const outside = path.join(root, "external-skill");
      await fs.rename(path.dirname(skillFile), outside);
      await fs.symlink(outside, path.dirname(skillFile));
    }
    if (scenario === "failed-auth") {
      await restarted.startAuth("dingtalk-workspace");
      await vi.waitFor(() => expect(restarted.list()[0].authSession?.phase).toBe("failed"));
    }
    if (scenario === "legacy") {
      const registry = JSON.parse(await fs.readFile(registryPath, "utf8"));
      delete registry.records["dingtalk-workspace"].officialSkillDigests;
      await fs.writeFile(registryPath, JSON.stringify(registry));
      await restarted.dispose(); restarted = makeRestarted(); await restarted.initialize();
    }
    if (scenario === "external") {
      await fs.mkdir(path.join(root, ".local", "bin"), { recursive: true });
      await fs.copyFile(executable, path.join(root, ".local", "bin", "dws"));
      await fs.chmod(path.join(root, ".local", "bin", "dws"), 0o700);
    }
    if (scenario === "missing") {
      await fs.rm(installed.readiness.executablePath!);
      await restarted.refreshProvider("dingtalk-workspace");
      expect(restarted.list()[0].canUninstallManaged).toBe(true);
      await expect(restarted.uninstallManaged("dingtalk-workspace", true)).rejects.toMatchObject({ code: "CLI_UNINSTALL_LOGOUT_UNAVAILABLE" });
      await fs.copyFile(executable, installed.readiness.executablePath!);
      await fs.chmod(installed.readiness.executablePath!, 0o700);
    }
    await fs.writeFile(path.join(root, "authorized"), "yes");
    if (scenario.startsWith("logout-")) {
      await fs.writeFile(path.join(root, scenario), "yes");
      await expect(restarted.uninstallManaged("dingtalk-workspace", true)).rejects.toMatchObject({ code: "CLI_UNINSTALL_LOGOUT_FAILED" });
      await expect(fs.stat(installed.readiness.executablePath!)).resolves.toBeDefined();
      await expect(fs.readFile(skillFile, "utf8")).resolves.toContain("name: dws");
      expect(JSON.parse(await fs.readFile(registryPath, "utf8")).records["dingtalk-workspace"]).toBeDefined();
      await fs.rm(path.join(root, scenario));
    }
    if (scenario === "both-missing") {
      await fs.rm(installed.readiness.executablePath!);
      await fs.rm(path.join(root, ".awaken", "runtime", "cli", "bin", "dws"), { force: true });
      await expect(restarted.uninstallManaged("dingtalk-workspace", true)).rejects.toMatchObject({ code: "CLI_UNINSTALL_LOGOUT_UNAVAILABLE" });
      await expect(fs.stat(skillFile)).resolves.toBeDefined();
      await restarted.dispose(); await fs.rm(root, { recursive: true, force: true }); return;
    }
    if (scenario === "rollback") {
      const rename = fs.rename.bind(fs);
      const failure = vi.spyOn(fs, "rename").mockImplementation(async (from, to) => {
        if (String(to) === registryPath) throw new Error("registry write failed");
        return rename(from, to);
      });
      await expect(restarted.uninstallManaged("dingtalk-workspace", true)).rejects.toMatchObject({ code: "CLI_UNINSTALL_FAILED" });
      failure.mockRestore();
      await expect(fs.readFile(skillFile, "utf8")).resolves.toContain("name: dws");
      await expect(fs.stat(installed.readiness.executablePath!)).resolves.toBeDefined();
      expect(JSON.parse(await fs.readFile(registryPath, "utf8")).records["dingtalk-workspace"]).toBeDefined();
    }
    if (scenario === "move-interrupted") {
      const rename = fs.rename.bind(fs);
      const failure = vi.spyOn(fs, "rename").mockImplementation(async (from, to) => {
        await rename(from, to);
        if (String(from) === path.dirname(skillFile)) throw new Error("process ended after first Skill move");
      });
      await expect(restarted.uninstallManaged("dingtalk-workspace", true)).rejects.toThrow("卸载清理未完成");
      failure.mockRestore();
      const pending = JSON.parse(await fs.readFile(registryPath, "utf8")).records["dingtalk-workspace"].pendingUninstall;
      expect(pending.skillsStaged).toBe(false);
      expect(pending.skills).toEqual([expect.objectContaining({ name: "dws" })]);
      await expect(fs.stat(path.join(pending.directory, "dws", "SKILL.md"))).resolves.toBeDefined();
      await restarted.dispose(); restarted = makeRestarted(); await restarted.initialize();
      expect(restarted.list()[0].canUninstallManaged).toBe(true);
      await expect(fs.stat(skillFile)).rejects.toMatchObject({ code: "ENOENT" });
    }
    if (scenario === "cleanup-failed") {
      const remove = fs.rm.bind(fs);
      const failure = vi.spyOn(fs, "rm").mockImplementation(async (target, options) => {
        if (String(target).includes(`${path.sep}.uninstall${path.sep}`)) throw new Error("cleanup denied");
        return remove(target, options);
      });
      await expect(restarted.uninstallManaged("dingtalk-workspace", true)).rejects.toThrow("账号已退出登录。卸载清理未完成");
      failure.mockRestore();
      await restarted.dispose(); restarted = makeRestarted(); await restarted.initialize();
      expect(restarted.list()[0].readiness.safeError?.code).toBe("CLI_UNINSTALL_PENDING");
      expect(restarted.list()[0].canUninstallManaged).toBe(true);
    }
    const removed = await restarted.uninstallManaged("dingtalk-workspace", scenario !== "keep");
    expect(removed.canUninstallManaged).toBe(false);
    await expect(fs.stat(path.join(root, "authorized"))).rejects.toMatchObject({ code: "ENOENT" });
    expect(await fs.readFile(path.join(root, "operations"), "utf8")).toContain("logout");
    await expect(fs.stat(path.join(root, ".awaken", "runtime", "cli", "bin", "dws"))).rejects.toMatchObject({ code: "ENOENT" });
    if (["keep", "modified", "legacy", "symlink"].includes(scenario)) {
      await expect(fs.readFile(skillFile, "utf8")).resolves.toContain("name: dws");
      expect(removed.uninstallMessage).toContain("保留");
    } else await expect(fs.stat(skillFile)).rejects.toMatchObject({ code: "ENOENT" });
    if (scenario === "external") {
      expect(removed.readiness).toMatchObject({ install: "installed", ownership: "user-managed" });
      expect(removed.uninstallMessage).toContain("本机其他安装");
      await expect(restarted.uninstallManaged("dingtalk-workspace", true)).rejects.toMatchObject({ code: "CLI_UNINSTALL_NOT_MANAGED" });
    } else {
      expect(removed.readiness.install).toBe("missing");
      expect(JSON.parse(await fs.readFile(registryPath, "utf8")).records["dingtalk-workspace"]).toBeUndefined();
      await restarted.dispose(); restarted = makeRestarted(); await restarted.initialize();
      expect(restarted.list()[0].readiness.install).toBe("missing");
    }
    await restarted.dispose();
    await fs.rm(root, { recursive: true, force: true });
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
      expect(stateChanges.mock.calls.some(([view]) => view.installProgress?.phase === "preparing" && view.installProgress?.label === "正在下载官方 CLI")).toBe(true);
    });

    const reopened = await manager.refreshProvider("dingtalk-workspace");
    expect(reopened.readiness).toMatchObject({ install: "installing", overall: "working" });
    expect(reopened.installProgress).toMatchObject({ phase: "preparing", label: "正在下载官方 CLI" });

    const duplicateStart = await manager.install("dingtalk-workspace");
    expect(duplicateStart.readiness).toMatchObject({ install: "installing", overall: "working" });
    expect(duplicateStart.installProgress).toMatchObject({ phase: "preparing", label: "正在下载官方 CLI" });

    await manager.cancelInstall("dingtalk-workspace");
    const cancelled = await installing;

    expect(cancelled.readiness).toMatchObject({ install: "missing", overall: "not_ready" });
    expect(cancelled.readiness.safeError).toBeUndefined();
    expect(cancelled.installProgress).toBeUndefined();
    await expect(fs.stat(path.join(root, ".awaken", "runtime", "cli", "bin", "dws"))).rejects.toMatchObject({ code: "ENOENT" });
    await manager.dispose();
  });

  it("cancels a waiting authorization without letting child exit overwrite the recovery state", async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-auth-cancel-"));
    const bin = path.join(root, "bin");
    const stateDir = path.join(root, "state");
    await fs.mkdir(bin, { recursive: true });
    await fs.mkdir(stateDir, { recursive: true });
    const executable = path.join(bin, "dws");
    await fs.writeFile(executable, `#!${process.execPath}
const fs = require("node:fs");
const path = require("node:path");
const args = process.argv.slice(2);
const stateDir = ${JSON.stringify(stateDir)};
if (args[0] === "version") {
  process.stdout.write('{"version":"v1.0.32"}\\n');
} else if (args[0] === "auth" && args[1] === "status") {
  process.stdout.write('{"success":true,"authenticated":false}\\n');
} else if (args[0] === "auth" && args[1] === "login") {
  process.stdout.write('GET https://mcp.dingtalk.com/cli/clientId succeeded\\n');
  process.stdout.write('Open https://login.dingtalk.com/oauth2/auth?state=cancel-test\\n');
  process.on("SIGTERM", () => {
    fs.writeFileSync(path.join(stateDir, "cancelled"), "ok");
    process.exit(0);
  });
  setInterval(() => undefined, 1000);
} else {
  process.stdout.write('{"ok":true}\\n');
}
`, { mode: 0o700 });
    const manifestPath = path.join(root, "manifest.json");
    await fs.writeFile(manifestPath, JSON.stringify({
      revision: "test.auth-cancel",
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
    }));
    process.env.PATH = bin;
    const openedUrls: string[] = [];
    const manager = new CliConnectorManager({
      homeDir: root,
      platform: "darwin",
      architecture: "arm64",
      manifestPath,
      fetchAsset: vi.fn(),
      openExternal: vi.fn(async (url) => { openedUrls.push(url); }),
      publishReadiness: vi.fn(async () => undefined),
      onStateChanged: vi.fn(),
    });
    await manager.initialize();
    const waiting = await manager.startAuth("dingtalk-workspace");
    expect(waiting.authSession?.phase).toBe("waiting_user");
    await vi.waitFor(() => {
      expect(openedUrls).toEqual(["https://login.dingtalk.com/oauth2/auth?state=cancel-test"]);
    });

    const cancelled = await manager.cancelAuth("dingtalk-workspace");
    expect(cancelled).toMatchObject({
      readiness: {
        install: "installed",
        executable: "available",
        authentication: "required",
        overall: "action_required",
      },
      authSession: undefined,
    });
    expect(cancelled.readiness.safeError).toBeUndefined();
    await vi.waitFor(async () => {
      await expect(fs.stat(path.join(stateDir, "cancelled"))).resolves.toBeDefined();
    });
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(manager.list().find((entry) => entry.providerId === "dingtalk-workspace")).toMatchObject({
      readiness: { authentication: "required", overall: "action_required" },
      authSession: undefined,
    });
    await expect(fs.readdir(path.join(root, ".awaken", "runtime", "auth", "dingtalk-workspace")))
      .resolves.toEqual([]);
    await manager.dispose();
  });

  it.each(["signed-in", "already-out", "logout-failed", "still-user", "bad-status"])("keeps Lark onboarding and verifies logout before uninstall: %s", async (scenario) => {
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
  printf 'POST https://accounts.feishu.cn/oauth/v1/app/registration failed once; retrying\\n' >&2
  printf 'Open https://open.feishu.cn/page/cli?user_code=test to continue\\n'
  while [ ! -f "$state_dir/config-approved" ]; do sleep 0.05; done
  touch "$state_dir/configured"
elif [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  if [ -f "$state_dir/bad-status" ]; then printf '{}'; exit 1; fi
  if [ ! -f "$state_dir/configured" ]; then
    printf '{"ok":false,"type":"config"}\\n' >&2
    exit 1
  fi
  if [ -f "$state_dir/authorized" ]; then
    printf '{"identity":"user","verified":true,"identities":{"bot":{"status":"ready","available":true,"verified":true},"user":{"status":"ready","available":true,"verified":true,"userName":"Nolan","tokenStatus":"valid"}}}\\n'
  else
    printf '{"identity":"bot","verified":true,"identities":{"bot":{"status":"ready","available":true,"verified":true},"user":{"status":"missing","available":false}}}\\n'
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
elif [ "$1" = "auth" ] && [ "$2" = "logout" ]; then
  if [ -f "$state_dir/logout-failed" ]; then exit 1; fi
  if [ ! -f "$state_dir/authorized" ]; then printf '{"ok":true,"loggedOut":false,"reason":"not_logged_in"}\\n'; exit 0; fi
  if [ ! -f "$state_dir/still-user" ]; then rm -f "$state_dir/authorized"; fi
  printf '{"loggedOut":true}\\n'
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
          path.join(stateDir, url.includes("/page/cli") ? "config-approved" : "auth-approved"),
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
      const entry = manager.list().find((entry) => entry.providerId === "lark-cli");
      if (entry?.readiness.overall === "ready" && !entry.authSession) break;
      await new Promise((resolve) => setTimeout(resolve, 25));
    }

    expect(openedUrls).toEqual([
      "https://open.feishu.cn/page/cli?user_code=test",
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

    const loggedOut = await manager.logout("lark-cli");
    expect(loggedOut.readiness).toMatchObject({
      configuration: "ready",
      authentication: "required",
      overall: "action_required",
    });
    expect(loggedOut.readiness.account).toBeUndefined();
    if (scenario !== "already-out") await fs.writeFile(path.join(stateDir, "authorized"), "yes");
    await manager.refreshProvider("lark-cli");
    if (["logout-failed", "still-user", "bad-status"].includes(scenario)) {
      await fs.writeFile(path.join(stateDir, scenario), "yes");
      await expect(manager.uninstallManaged("lark-cli", true)).rejects.toMatchObject({ code: "CLI_UNINSTALL_LOGOUT_FAILED" });
      await expect(fs.stat(installed.readiness.executablePath!)).resolves.toBeDefined();
      await expect(fs.stat(path.join(root, ".agents", "skills", "lark-im", "SKILL.md"))).resolves.toBeDefined();
      await fs.rm(path.join(stateDir, scenario));
    }
    const remove = fs.rm.bind(fs);
    let userRemovedBeforeFiles = false;
    const observe = vi.spyOn(fs, "rm").mockImplementation(async (target, options) => {
      if (String(target) === installed.readiness.executablePath!) {
        await expect(fs.stat(path.join(stateDir, "authorized"))).rejects.toMatchObject({ code: "ENOENT" });
        userRemovedBeforeFiles = true;
      }
      return remove(target, options);
    });
    const uninstalled = await manager.uninstallManaged("lark-cli");
    observe.mockRestore();
    expect(userRemovedBeforeFiles).toBe(true);
    expect(uninstalled.uninstallMessage).toContain("已退出登录并卸载");
    expect(uninstalled.readiness.install).toBe("missing");
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
