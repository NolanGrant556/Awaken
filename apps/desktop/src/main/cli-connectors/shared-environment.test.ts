import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { c as createTar } from "tar";
import { expect, it, vi } from "vitest";
import { CliConnectorManager } from "./manager.js";

it("leaves shared binaries with Core and rejects uninstall before changing an in-use installation", async () => {
  if (process.platform === "win32") return;
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-shared-cli-"));
  const homeAlias = `${root}-home-alias`;
  await fs.symlink(root, homeAlias, "dir");
  const executable = path.join(root, ".awaken/runtime/environments/components/component/instance/dws");
  await fs.mkdir(path.dirname(executable), { recursive: true });
  await fs.writeFile(executable, '#!/bin/sh\nif [ "$1" = "version" ]; then printf \'{"version":"v1.0.32"}\\n\'; else printf \'{"success":true,"authenticated":false}\\n\'; fi\n', { mode: 0o700 });
  const skill = path.join(root, "source/dws");
  await fs.mkdir(skill, { recursive: true });
  await fs.writeFile(path.join(skill, "SKILL.md"), "---\nname: dws\ndescription: fixture\n---\n# DWS\n");
  const archive = path.join(root, "skills.tar.gz");
  await createTar({ cwd: path.dirname(skill), file: archive, gzip: true }, ["dws"]);
  const bytes = await fs.readFile(archive);
  const manifestPath = path.join(root, "manifest.json");
  await fs.writeFile(manifestPath, JSON.stringify({ revision: "test", assets: [{ providerId: "dingtalk-workspace", version: "1.0.32", platform: "darwin", architecture: "arm64", url: "https://github.com/example/cli.tar.gz", sha256: "a".repeat(64), archiveType: "tar.gz", executableName: "dws", archiveExecutablePath: "dws" }], skillBundles: [{ providerId: "dingtalk-workspace", version: "1.0.32", url: "https://github.com/example/skills.tar.gz", sha256: createHash("sha256").update(bytes).digest("hex"), archiveType: "tar.gz", archiveRoot: "dws", destinationRoot: "dws", skillNames: ["dws"] }] }));
  const prepare = vi.fn(async () => executable);
  const remove = vi.fn(async () => { await fs.rm(executable); });
  let installed = false;
  const inspect = vi.fn(async () => installed ? { executablePath: executable, version: "1.0.32", healthy: true } : null);
  prepare.mockImplementation(async () => { installed = true; return executable; });
  remove.mockImplementation(async () => { await fs.rm(executable); installed = false; });
  const reserve = vi.fn(async (): Promise<{ token: string; release: () => Promise<void> }> => { throw new Error("environment_in_use"); });
  const fetchAsset = vi.fn(async (_url: string) => new Response(bytes));
  let liveUsages = 0;
  const hold = vi.fn(async (_executable: string, _token?: string) => { liveUsages += 1; return async () => { liveUsages -= 1; }; });
  const releaseReservation = vi.fn(async () => {});
  const options: ConstructorParameters<typeof CliConnectorManager>[0] = { homeDir: homeAlias, platform: "darwin", architecture: "arm64", manifestPath, prepareSharedComponent: prepare, removeSharedComponent: remove, inspectSharedComponent: inspect, reserveSharedComponentRemoval: reserve, holdSharedExecutable: hold, fetchAsset, publishReadiness: async () => {}, onStateChanged: () => {} };
  const manager = new CliConnectorManager(options);
  try {
    await manager.initialize();
    const view = await manager.install("dingtalk-workspace");
    expect(view.readiness).toMatchObject({ install: "installed", authentication: "required", ownership: "awaken-managed" });
    expect(prepare).toHaveBeenCalledTimes(1);
    expect(hold).toHaveBeenCalled();
    expect(liveUsages).toBe(0);
    const saved = JSON.parse(await fs.readFile(path.join(root, ".awaken/runtime/cli/connector-state.json"), "utf8"));
    expect(saved.version).toBe(2);
    expect(saved.records["dingtalk-workspace"]).not.toHaveProperty("ownership");
    expect(saved.records["dingtalk-workspace"]).not.toHaveProperty("executablePath");
    expect(saved.records["dingtalk-workspace"]).not.toHaveProperty("version");
    expect(fetchAsset.mock.calls.every(([url]) => String(url).endsWith("skills.tar.gz"))).toBe(true);
    await expect(fs.stat(path.join(root, ".awaken/runtime/cli/bin/dws"))).rejects.toMatchObject({ code: "ENOENT" });
    await expect(manager.uninstallManaged("dingtalk-workspace", true)).rejects.toThrow("environment_in_use");
    expect(remove).not.toHaveBeenCalled();
    await expect(fs.stat(executable)).resolves.toBeDefined();
    await expect(fs.stat(path.join(root, ".agents/skills/dws/SKILL.md"))).resolves.toBeDefined();
    reserve.mockClear();
    reserve.mockImplementation(async () => ({ token: "removal-owner", release: releaseReservation }));
    let releaseProbe!: () => void;
    let reportProbe!: () => void;
    const probeStarted = new Promise<void>((resolve) => { reportProbe = resolve; });
    const probeGate = new Promise<void>((resolve) => { releaseProbe = resolve; });
    hold.mockImplementationOnce(async () => {
      liveUsages += 1;
      reportProbe();
      await probeGate;
      return async () => { liveUsages -= 1; };
    });
    const refreshing = manager.refreshProvider("dingtalk-workspace");
    await probeStarted;
    const uninstalling = manager.uninstallManaged("dingtalk-workspace", true);
    await Promise.resolve();
    expect(reserve).not.toHaveBeenCalled();
    releaseProbe();
    await Promise.all([refreshing, uninstalling]);
    expect(remove).toHaveBeenCalledTimes(1);
    expect(remove).toHaveBeenCalledWith("dingtalk-workspace", "removal-owner");
    expect(hold).toHaveBeenCalledWith(executable, "removal-owner");
    expect(liveUsages).toBe(0);
    expect(releaseReservation).toHaveBeenCalledOnce();
    await expect(fs.stat(executable)).rejects.toMatchObject({ code: "ENOENT" });
  } finally {
    await manager.dispose();
    await fs.rm(homeAlias, { force: true });
    await fs.rm(root, { recursive: true, force: true });
  }
});

it("migrates Main metadata after Core is ready and recovers a pending uninstall after restart", async () => {
  if (process.platform === "win32") return;
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "awaken-cli-migration-"));
  const cliRoot = path.join(root, ".awaken/runtime/cli");
  const executable = path.join(cliRoot, "dingtalk-workspace/1.0.32/platform/dws");
  const logoutLog = path.join(root, "logout-count");
  await fs.mkdir(path.dirname(executable), { recursive: true });
  await fs.writeFile(executable, `#!/bin/sh\nif [ "$1" = "version" ]; then printf '{"version":"v1.0.32"}\\n'; elif [ "$2" = "logout" ]; then echo logout >> '${logoutLog}'; printf '{"success":true}\\n'; else printf '{"success":true,"authenticated":false}\\n'; fi\n`, { mode: 0o700 });
  const legacyPath = path.join(cliRoot, "installations.json");
  await fs.writeFile(legacyPath, JSON.stringify({ version: 1, records: { "dingtalk-workspace": { providerId: "dingtalk-workspace", executablePath: executable, version: "1.0.32", ownership: "awaken-managed", activation: "disabled", officialSkillVersion: "1.0.32", officialSkillNames: ["dws"], officialSkillDigests: { dws: "retained-proof" } } } }));
  const manifestPath = path.join(root, "manifest.json");
  await fs.writeFile(manifestPath, JSON.stringify({ revision: "test", assets: [{ providerId: "dingtalk-workspace", version: "1.0.32", platform: "darwin", architecture: "arm64", url: "https://github.com/example/cli.tar.gz", sha256: "a".repeat(64), archiveType: "tar.gz", executableName: "dws", archiveExecutablePath: "dws" }] }));
  let ready = false, installed = true;
  const inspect = vi.fn(async (id: string) => {
    if (!ready) throw new Error("Core starting");
    return id === "dingtalk-workspace" && installed ? { executablePath: executable, version: "1.0.32", healthy: true } : null;
  });
  const release = vi.fn(async () => {});
  const reserve = vi.fn(async () => ({ token: "reserved", release }));
  const remove = vi.fn(async () => {
    if (installed) {
      await fs.rm(executable); installed = false;
      throw new Error("response lost after Core deletion");
    }
  });
  const options: ConstructorParameters<typeof CliConnectorManager>[0] = {
    homeDir: root, platform: "darwin", architecture: "arm64", manifestPath,
    inspectSharedComponent: inspect, reserveSharedComponentRemoval: reserve, removeSharedComponent: remove,
    holdSharedExecutable: async () => async () => {},
    fetchAsset: vi.fn(async () => { throw new Error("migration must not download"); }),
    publishReadiness: async () => {}, onStateChanged: () => {},
  };
  let manager = new CliConnectorManager(options);
  try {
    await manager.initialize();
    expect(manager.list()[0].readiness.safeError?.code).toBe("CLI_COMPONENT_STATE_UNAVAILABLE");
    expect(manager.list()[0].canUninstallManaged).toBe(false);
    await expect(fs.stat(legacyPath)).resolves.toBeDefined();
    ready = true;
    await manager.refreshAll();
    expect(manager.list()[0].readiness).toMatchObject({ ownership: "awaken-managed", activation: "disabled", executablePath: executable });
    const statePath = path.join(cliRoot, "connector-state.json");
    const state = JSON.parse(await fs.readFile(statePath, "utf8"));
    expect(state.records["dingtalk-workspace"]).toEqual({ activation: "disabled", officialSkillVersion: "1.0.32", officialSkillNames: ["dws"], officialSkillDigests: { dws: "retained-proof" } });
    await expect(fs.stat(legacyPath)).rejects.toMatchObject({ code: "ENOENT" });
    await expect(fs.stat(`${legacyPath}.migrated`)).resolves.toBeDefined();
    const before = await fs.stat(statePath);
    await manager.refreshAll();
    expect((await fs.stat(statePath)).ino).toBe(before.ino);
    await manager.dispose();
    manager = new CliConnectorManager(options);
    await manager.initialize();
    expect(manager.list()[0].canUninstallManaged).toBe(true);
    await expect(manager.uninstallManaged("dingtalk-workspace")).rejects.toThrow("账号已退出登录");
    expect(JSON.parse(await fs.readFile(statePath, "utf8")).records["dingtalk-workspace"].pendingUninstall).toMatchObject({ removeOfficialSkills: false });
    await manager.dispose();
    manager = new CliConnectorManager(options);
    await manager.initialize();
    expect(manager.list()[0].canUninstallManaged).toBe(true);
    expect(manager.list()[0].readiness.safeError?.code).toBe("CLI_UNINSTALL_PENDING");
    await manager.uninstallManaged("dingtalk-workspace");
    expect(remove).toHaveBeenCalledTimes(2);
    expect((await fs.readFile(logoutLog, "utf8")).trim().split("\n")).toEqual(["logout"]);
    expect(manager.list()[0].canUninstallManaged).toBe(false);
    expect(JSON.parse(await fs.readFile(statePath, "utf8")).records["dingtalk-workspace"]).toBeUndefined();
    expect(options.fetchAsset).not.toHaveBeenCalled();
  } finally { await manager.dispose(); await fs.rm(root, { recursive: true, force: true }); }
});
