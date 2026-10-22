import { createHash, randomUUID } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { constants as fsConstants } from "node:fs";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import type {
  CliAccountProfile,
  CliActivation,
  CliAuthSessionView,
  CliConnectorReadiness,
  CliConnectorView,
  CliInstallPhase,
  CliInstallProgress,
  CliInstallOwnership,
  CliProviderId,
} from "../../shared/runtime.js";
import { extractSkillBundle } from "./archive.js";

type ManifestAsset = {
  providerId: CliProviderId;
  version: string;
  platform: "darwin" | "win32";
  architecture: "arm64" | "x64";
  url: string;
  sha256: string;
  archiveType: "tar.gz" | "zip";
  executableName: string;
  archiveExecutablePath: string;
};

type ManifestSkillBundle = {
  providerId: CliProviderId;
  version: string;
  url: string;
  sha256: string;
  archiveType: "tar.gz" | "zip";
  archiveRoot: string;
  destinationRoot?: string;
  skillNames: string[];
};

type CliManifest = { revision: string; assets: ManifestAsset[]; skillBundles?: ManifestSkillBundle[] };

// In-memory join of Core installation facts and Main connector state. Only the
// explicit connector-state projection in persistInstallations is persisted.
type InstallationRecord = {
  providerId: CliProviderId;
  executablePath: string;
  version: string;
  platform: "darwin" | "win32";
  architecture: string;
  manifestRevision: string;
  assetSha256: string;
  ownership: CliInstallOwnership;
  installMethod: "managed-download" | "npm" | "homebrew" | "system" | "unknown";
  activation: CliActivation;
  lastVerifiedAt: string;
  officialSkillVersion?: string;
  officialSkillNames?: string[];
  officialSkillDigests?: Record<string, string>;
  pendingUninstall?: { directory: string; removedSkills: number; retainedSkills: string[]; removeOfficialSkills: boolean; skills?: Array<{ name: string; digest: string }>; skillsStaged?: boolean };
};

type StoredInstallations = { version: 1; records: Partial<Record<CliProviderId, InstallationRecord>> };

type AuthSessionInternal = {
  controller: AbortController;
  filesReady?: Promise<void>;
  view: CliAuthSessionView;
  directory: string;
  child?: ChildProcess;
  deviceCode?: string;
  openedVerificationUrl?: string;
};

type CommandResult = { exitCode: number; stdout: string; stderr: string };

type ManagerOptions = {
  homeDir: string;
  inspectSharedComponent?: (providerId: CliProviderId) => Promise<{ executablePath: string; version: string; healthy: boolean } | null>;
  removeSharedComponent?: (providerId: CliProviderId, token?: string) => Promise<void>;
  reserveSharedComponentRemoval?: (providerId: CliProviderId) => Promise<{ token: string; release: () => Promise<void> }>;
  holdSharedExecutable?: (executable: string, removalToken?: string) => Promise<() => Promise<void>>;
  prepareSharedComponent?: (providerId: CliProviderId, signal: AbortSignal, progress: (message: string) => void) => Promise<string>;
  platform: NodeJS.Platform;
  architecture: string;
  manifestPath: string;
  fetchAsset: (url: string, signal: AbortSignal) => Promise<Response>;
  openExternal?: (url: string) => Promise<void>;
  publishReadiness: (readiness: CliConnectorReadiness) => Promise<void>;
  onStateChanged: (view: CliConnectorView) => void;
  environment?: () => NodeJS.ProcessEnv;
  beforeNetworkOperation?: (surface: string) => Promise<void>;
  onTransportFailure?: (surface: string) => void;
};

const providers: CliProviderId[] = ["dingtalk-workspace", "lark-cli"];
const maxArchiveBytes = 128 * 1024 * 1024;
const commandOutputLimit = 1024 * 1024;
const larkVerificationHosts = new Set([
  "accounts.feishu.cn",
  "open.feishu.cn",
  "accounts.larksuite.com",
  "open.larksuite.com",
]);
const larkServiceEndpoints = new Set([
  "accounts.feishu.cn/oauth/v1/app/registration",
  "accounts.feishu.cn/oauth/v1/device_authorization",
  "accounts.feishu.cn/oauth/v1/revoke",
  "accounts.larksuite.com/oauth/v1/app/registration",
  "accounts.larksuite.com/oauth/v1/device_authorization",
  "accounts.larksuite.com/oauth/v1/revoke",
  "open.feishu.cn/open-apis/authen/v2/oauth/token",
  "open.larksuite.com/open-apis/authen/v2/oauth/token",
]);

export class CliConnectorError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly recovery?: string,
  ) {
    super(message);
  }
}

export class CliConnectorManager {
  private readonly root: string;
  private readonly installationsPath: string;
  private readonly authRoot: string;
  private manifest: CliManifest = { revision: "invalid", assets: [] };
  private installations: StoredInstallations = { version: 1, records: {} };
  private readonly views = new Map<CliProviderId, CliConnectorView>();
  private readonly installControllers = new Map<CliProviderId, AbortController>();
  private readonly installSettled = new Map<CliProviderId, Promise<void>>();
  private readonly installProgress = new Map<CliProviderId, CliInstallProgress>();
  private readonly authSessions = new Map<CliProviderId, AuthSessionInternal>();
  private readonly authEpochs = new Map<CliProviderId, number>();
  private disposed = false;
  private readonly uninstalling = new Set<CliProviderId>();
  private readonly ownershipHydrated = new Set<CliProviderId>();
  private stateWrites: Promise<void> = Promise.resolve();
  private savedState = "";
  private readonly backfillAttempts = new Set<CliProviderId>();
  private readonly removalTokens = new Map<CliProviderId, string>();
  private readonly pendingCliCommands = new Map<CliProviderId, Set<Promise<void>>>();

  constructor(private readonly options: ManagerOptions) {
    this.root = path.join(options.homeDir, ".awaken", "runtime", "cli");
    this.installationsPath = path.join(this.root, "connector-state.json");
    this.authRoot = path.join(options.homeDir, ".awaken", "runtime", "auth");
  }

  managedBinPath(): string {
    return path.join(this.root, "bin");
  }

  async initialize(): Promise<void> {
    await fs.mkdir(this.root, { recursive: true, mode: 0o700 });
    await fs.mkdir(this.managedBinPath(), { recursive: true, mode: 0o700 });
    await fs.chmod(this.root, 0o700);
    await fs.chmod(this.managedBinPath(), 0o700);
    await fs.rm(this.authRoot, { recursive: true, force: true });
    await this.ensureAuthRoot();
    this.manifest = await this.loadManifest();
    this.installations = await this.loadInstallations();
    if (await pathExists(this.installationsPath)) await fs.chmod(this.installationsPath, 0o600);
    for (const providerId of providers) await this.refresh(providerId);
    await this.backfillOfficialSkills();
  }

  list(): CliConnectorView[] {
    return providers.map((id) => structuredClone(this.views.get(id) ?? this.defaultView(id)));
  }

  async refreshAll(): Promise<CliConnectorView[]> {
    for (const providerId of providers) await this.refresh(providerId);
    await this.backfillOfficialSkills();
    return this.list();
  }

  async refreshProvider(providerId: CliProviderId): Promise<CliConnectorView> {
    this.assertProvider(providerId);
    if (this.installControllers.has(providerId)) return this.view(providerId);
    return this.refresh(providerId);
  }

  async install(providerId: CliProviderId): Promise<CliConnectorView> {
    this.assertProvider(providerId);
    if (this.installControllers.has(providerId)) return this.view(providerId);
    this.assertNotUninstalling(providerId);
    if (this.installations.records[providerId]?.pendingUninstall) throw new CliConnectorError("CLI_UNINSTALL_PENDING", "请先重试完成卸载，再重新安装");
    await this.prepareNetworkOperation("cli-install");
    this.assertNotUninstalling(providerId);
    const asset = this.assetFor(providerId);
    if (!asset) throw new CliConnectorError("CLI_UNSUPPORTED", "当前平台或架构没有受支持的安装资产");
    const controller = new AbortController();
    let resolveInstallSettled!: () => void;
    const settled = new Promise<void>((resolve) => {
      resolveInstallSettled = resolve;
    });
    this.installControllers.set(providerId, controller);
    this.installSettled.set(providerId, settled);
    this.setInstallProgress(providerId, "preparing", 2, "正在准备安装");
    this.setReadiness(providerId, {
      ...this.readiness(providerId),
      install: "installing",
      overall: "working",
      safeError: undefined,
      updatedAt: new Date().toISOString(),
    });
    try {
      if (!this.options.prepareSharedComponent || !this.options.inspectSharedComponent) throw new CliConnectorError("CLI_CORE_UNAVAILABLE", "共享环境服务尚未就绪，请稍后重试");
      const executablePath = await this.options.prepareSharedComponent(providerId, controller.signal, (message) => this.setInstallProgress(providerId, "preparing", 0, message));
      if (await this.probeVersion(providerId, executablePath) !== asset.version) throw new CliConnectorError("CLI_VERSION_MISMATCH", "共享 CLI 版本不匹配");
      const officialSkills = await this.installOfficialSkills(providerId, controller.signal);
      this.setInstallProgress(providerId, "finishing", 97, "正在完成安装");
      this.installations.records[providerId] = {
        providerId,
        executablePath,
        version: asset.version,
        platform: asset.platform,
        architecture: asset.architecture,
        manifestRevision: this.manifest.revision,
        assetSha256: asset.sha256,
        ownership: "awaken-managed",
        installMethod: "managed-download",
        activation: "enabled",
        lastVerifiedAt: new Date().toISOString(),
        officialSkillVersion: this.skillBundleFor(providerId)?.version,
        officialSkillNames: officialSkills.names,
        officialSkillDigests: officialSkills.digests,
      };
      await this.saveInstallations();
      this.installProgress.delete(providerId);
      return await this.refresh(providerId);
    } catch (error) {
      this.installProgress.delete(providerId);
      if (controller.signal.aborted) {
        return await this.refresh(providerId);
      }
      const mapped = mapError(error, controller.signal.aborted ? "CLI_INSTALL_CANCELLED" : "CLI_INSTALL_FAILED");
      this.setReadiness(providerId, {
        ...this.readiness(providerId),
        install: "failed",
        overall: "failed",
        safeError: { code: mapped.code, message: mapped.message, recovery: mapped.recovery },
        updatedAt: new Date().toISOString(),
      });
      throw mapped;
    } finally {
      this.installControllers.delete(providerId);
      this.installSettled.delete(providerId);
      resolveInstallSettled();
    }
  }

  async cancelInstall(providerId: CliProviderId): Promise<CliConnectorView> {
    const settled = this.installSettled.get(providerId);
    this.installControllers.get(providerId)?.abort();
    await settled;
    return this.view(providerId);
  }

  async startAuth(providerId: CliProviderId): Promise<CliConnectorView> {
    this.assertProvider(providerId);
    this.assertNotUninstalling(providerId);
    if (this.disposed) throw new CliConnectorError("CLI_MANAGER_CLOSED", "连接器管理器已关闭");
    if (this.authSessions.has(providerId)) return this.view(providerId);
    const executable = this.requireExecutable(providerId);
    const configurationOnly = providerId === "lark-cli" && this.readiness(providerId).configuration === "required";
    // Reserve ownership before the first await, including directory/network setup.
    const session = this.createAuthSession(providerId, configurationOnly
      ? "正在打开飞书应用配置页面…" : "正在打开官方授权页面…");
    this.setReadiness(providerId, { ...this.readiness(providerId), authentication: "in_progress",
      overall: "working", safeError: undefined, updatedAt: new Date().toISOString() });
    try {
      await this.prepareNetworkOperation("cli-auth");
      if (!this.ownsAuth(session)) return this.view(providerId);
      session.filesReady = (async () => {
        await this.ensureAuthRoot();
        if (this.ownsAuth(session)) await fs.mkdir(session.directory, { recursive: true, mode: 0o700 });
      })();
      await session.filesReady;
      if (!this.ownsAuth(session)) return this.view(providerId);
      if (configurationOnly || providerId === "dingtalk-workspace") {
        const args = configurationOnly ? ["config", "init", "--new"] : ["auth", "login", "--format", "json"];
        const child = await this.spawnLeased(executable, args, session.directory, session.controller.signal);
        session.child = child;
        this.monitorVerificationUrl(session, child, "请在已打开的官方页面完成操作");
        session.view.phase = "waiting_user";
        this.emit(providerId);
        void this.finishBlockingSession(session, child, configurationOnly, configurationOnly);
      } else {
        void this.startLarkSplitAuth(session, executable);
      }
    } catch (error) {
      await this.failSession(session, mapError(error, "CLI_AUTH_START_FAILED"), configurationOnly);
    }
    return this.view(providerId);
  }

  async cancelAuth(providerId: CliProviderId): Promise<CliConnectorView> {
    const session = this.authSessions.get(providerId);
    if (!session) return this.view(providerId);
    // Remove ownership before terminating the child so a late exit event cannot
    // turn this explicit cancellation into a failed authorization state.
    this.authSessions.delete(providerId);
    this.advanceAuthEpoch(providerId);
    session.controller.abort();
    session.child?.kill();
    session.view.phase = "cancelled";
    session.view.safeMessage = "授权已取消";
    delete session.deviceCode;
    this.setReadiness(providerId, {
      ...this.readiness(providerId),
      authentication: "required",
      overall: "action_required",
      safeError: undefined,
      updatedAt: new Date().toISOString(),
    });
    await this.cleanupSessionFiles(session);
    return this.view(providerId);
  }

  async retryAuth(providerId: CliProviderId): Promise<CliConnectorView> {
    await this.cancelAuth(providerId);
    return this.startAuth(providerId);
  }

  async setActivation(providerId: CliProviderId, activation: CliActivation): Promise<CliConnectorView> {
    const record = this.installations.records[providerId];
    if (record) {
      record.activation = activation;
      await this.saveInstallations();
    }
    return this.refresh(providerId);
  }

  async logout(providerId: CliProviderId): Promise<CliConnectorView> {
    this.assertNotUninstalling(providerId);
    const executable = this.requireExecutable(providerId);
    await this.ensureAuthRoot();
    const args = providerId === "dingtalk-workspace"
      ? ["auth", "logout", "--format", "json"]
      : ["auth", "logout", "--json"];
    const result = await this.runCommand(executable, args, this.authRoot, 60_000);
    if (result.exitCode !== 0) throw new CliConnectorError("CLI_LOGOUT_FAILED", "CLI 退出登录失败", "retry");
    return this.refresh(providerId);
  }

  private async logoutBeforeUninstall(providerId: CliProviderId, record: InstallationRecord): Promise<void> {
    const candidates = [record.executablePath];
    const canonicalRoot = await fs.realpath(path.join(this.options.homeDir, ".awaken", "runtime"));
    let executable: string | undefined;
    for (const candidate of candidates) {
      if (!(await isExecutableFile(candidate))) continue;
      const relative = path.relative(canonicalRoot, await fs.realpath(candidate));
      if (relative.startsWith("..") || path.isAbsolute(relative)) continue;
      try {
        if (await this.probeVersion(providerId, candidate) === record.version) { executable = candidate; break; }
      } catch { /* Try the other managed copy, never another product's executable. */ }
    }
    if (!executable) throw new CliConnectorError("CLI_UNINSTALL_LOGOUT_UNAVAILABLE", "运行组件缺失或损坏，无法退出登录；请先重新安装修复，再卸载", "retry");
    await this.ensureAuthRoot();
    try {
      const args = providerId === "dingtalk-workspace"
        ? ["auth", "logout", "--format", "json"] : ["auth", "logout", "--json"];
      const result = await this.runCommand(executable, args, this.authRoot, 60_000);
      if (result.exitCode !== 0) throw new Error("logout failed");
      const reply = parseJSONObject(result.stdout);
      if (reply?.ok === false || reply?.success === false || reply?.error) throw new Error("logout failed");
      // Lark explicitly reports these successful no-op cases before config loading.
      const alreadyAbsent = providerId === "lark-cli" && reply?.ok === true && reply.loggedOut === false &&
        (reply.reason === "not_configured" || reply.reason === "not_logged_in");
      if (!alreadyAbsent) {
        const statusArgs = providerId === "dingtalk-workspace"
          ? ["auth", "status", "--format", "json"] : ["auth", "status", "--json"];
        const status = await this.runCommand(executable, statusArgs, this.authRoot, 30_000);
        const envelope = parseJSONObject(status.stdout);
        // A failed probe or an unavailable verification service is not evidence of logout.
        if (status.exitCode !== 0 || !isUserLoggedOut(providerId, envelope)) throw new Error("logout not confirmed");
      }
    } catch {
      throw new CliConnectorError("CLI_UNINSTALL_LOGOUT_FAILED", "退出登录未完成，卸载已停止，运行组件和技能未删除，请重试", "retry");
    }
    this.setReadiness(providerId, {
      ...this.readiness(providerId), authentication: "required", account: undefined,
      overall: "action_required", safeError: undefined, updatedAt: new Date().toISOString(),
    });
  }

  private assertNotUninstalling(providerId: CliProviderId): void {
    if (this.uninstalling.has(providerId)) throw new CliConnectorError("CLI_UNINSTALL_BUSY", "正在卸载，请稍后重试");
  }

  async uninstallManaged(providerId: CliProviderId, removeOfficialSkills = false): Promise<CliConnectorView> {
    this.assertProvider(providerId);
    this.assertNotUninstalling(providerId);
    if (!this.options.inspectSharedComponent || !this.options.reserveSharedComponentRemoval || !this.options.removeSharedComponent) throw new CliConnectorError("CLI_UNINSTALL_CORE_UNAVAILABLE", "共享组件卸载服务不可用，请重试");
    const authPhase = this.authSessions.get(providerId)?.view.phase;
    if (this.installControllers.has(providerId) || (authPhase && !["failed", "cancelled", "authorized"].includes(authPhase))) {
      throw new CliConnectorError("CLI_UNINSTALL_BUSY", "请先完成或取消安装、授权，再卸载");
    }
    this.uninstalling.add(providerId);
    let record: InstallationRecord;
    try {
      if (!this.installations.records[providerId]?.pendingUninstall) await this.syncSharedComponent(providerId);
      const current = this.installations.records[providerId];
      if (!current || (current.ownership !== "awaken-managed" && !current.pendingUninstall)) throw new CliConnectorError("CLI_UNINSTALL_NOT_MANAGED", "只能卸载 Awaken 管理的运行组件");
      record = current;
    } catch (error) { this.uninstalling.delete(providerId); throw error; }
    // Stop new probes and wait for this Main's already-started probes before
    // asking Core for exclusive removal. Other processes still fail closed.
    this.authEpochs.set(providerId, (this.authEpochs.get(providerId) ?? 0) + 1);
    let reservation: { token: string; release: () => Promise<void> } | undefined;
    try {
      await Promise.all(this.pendingCliCommands.get(providerId) ?? []);
      reservation = await this.options.reserveSharedComponentRemoval(providerId);
    } catch (error) { this.uninstalling.delete(providerId); throw error; }
    if (reservation) this.removalTokens.set(providerId, reservation.token);
    // Invalidate probes started before removal, so they cannot publish an old installed state.
    this.authEpochs.set(providerId, (this.authEpochs.get(providerId) ?? 0) + 1);
    const originalRecord = structuredClone(record);
    const staging = record.pendingUninstall?.directory ?? path.join(this.root, ".uninstall", randomUUID());
    if (path.dirname(staging) !== path.join(this.root, ".uninstall")) {
      this.uninstalling.delete(providerId);
      this.removalTokens.delete(providerId);
      await reservation?.release();
      throw new CliConnectorError("CLI_UNINSTALL_PATH_INVALID", "卸载记录路径无效，请检查安装记录");
    }
    const retained: string[] = record.pendingUninstall?.retainedSkills ?? [];
    let removedSkills = record.pendingUninstall?.removedSkills ?? 0;
    removeOfficialSkills = record.pendingUninstall?.removeOfficialSkills ?? removeOfficialSkills;
    let committed = Boolean(record.pendingUninstall);
    let loggedOut = Boolean(record.pendingUninstall);
    try {
      if (!record.pendingUninstall) {
        await this.cancelAuth(providerId);
        await this.logoutBeforeUninstall(providerId, record);
        loggedOut = true;
        const skills: Array<{ name: string; digest: string }> = [];
        if (removeOfficialSkills) {
          for (const name of record.officialSkillNames ?? []) {
            if (!/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(name)) continue;
            const source = path.join(this.options.homeDir, ".agents", "skills", name);
            if (!(await pathExists(source))) continue;
            const expected = record.officialSkillDigests?.[name];
            if (!expected || await skillDirectoryDigest(source) !== expected) {
              retained.push(name);
              continue;
            }
            skills.push({ name, digest: expected });
          }
        }
        // Persist the complete intent before moving any Skill. A process exit
        // between renames can resume from the same named staging slots.
        record.pendingUninstall = { directory: staging, removedSkills: 0, retainedSkills: retained, removeOfficialSkills, skills, skillsStaged: false };
        await this.saveInstallations();
        committed = true;
      }
      const pending = record.pendingUninstall!;
      // Older receipts were written after all moves and have no plan to replay.
      if (pending.skillsStaged === false) {
        removedSkills = 0;
        for (const entry of pending.skills ?? []) {
          if (!/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(entry.name) || !/^[a-f0-9]{64}$/.test(entry.digest)) {
            throw new CliConnectorError("CLI_UNINSTALL_PATH_INVALID", "卸载技能记录无效，未删除，请检查后重试");
          }
          const source = path.join(this.options.homeDir, ".agents", "skills", entry.name);
          const backup = path.join(staging, entry.name);
          const hasBackup = await pathExists(backup);
          if (hasBackup) {
            if (await pathExists(source) || await skillDirectoryDigest(backup) !== entry.digest) {
              throw new CliConnectorError("CLI_UNINSTALL_SKILL_CHANGED", "技能在卸载期间发生变化，文件已保留，请检查后重试");
            }
            removedSkills++;
            continue;
          }
          if (!(await pathExists(source))) continue;
          if (await skillDirectoryDigest(source) !== entry.digest) {
            if (!retained.includes(entry.name)) retained.push(entry.name);
            continue;
          }
          if (await fs.realpath(path.dirname(source)) !== path.join(await fs.realpath(this.options.homeDir), path.relative(this.options.homeDir, path.dirname(source)))) {
            throw new CliConnectorError("CLI_UNINSTALL_PATH_INVALID", "安装目录已被重定向，未删除，请检查后重试");
          }
          await fs.mkdir(staging, { recursive: true, mode: 0o700 });
          if (await fs.realpath(staging) !== path.join(await fs.realpath(this.options.homeDir), path.relative(this.options.homeDir, staging))) {
            throw new CliConnectorError("CLI_UNINSTALL_PATH_INVALID", "卸载暂存目录已被重定向，未移动技能，请检查后重试");
          }
          await fs.rename(source, backup);
          if (await skillDirectoryDigest(backup) !== entry.digest) {
            throw new CliConnectorError("CLI_UNINSTALL_SKILL_CHANGED", "技能在卸载期间发生变化，文件已保留，请检查后重试");
          }
          removedSkills++;
        }
        pending.removedSkills = removedSkills;
        pending.skillsStaged = true;
        await this.saveInstallations();
      }
      // Save the retry receipt before asking Core to remove shared binaries. A
      // failed response never causes Main to pretend it restored Core-owned files.
      await this.options.removeSharedComponent(providerId, reservation.token);
      // Keep a durable retry record until both file cleanup and registry removal succeed.
      await fs.rm(staging, { recursive: true, force: true });
      delete this.installations.records[providerId];
      try { await this.saveInstallations(); }
      catch (error) { this.installations.records[providerId] = record; throw error; }
    } catch (error) {
      if (!committed) {
        this.installations.records[providerId] = originalRecord;

      }
      const failure = error instanceof CliConnectorError ? error : new CliConnectorError("CLI_UNINSTALL_FAILED", committed ? "卸载清理未完成，请重试卸载" : "卸载失败，未删除安装文件，请检查目录权限后重试");
      if (loggedOut) throw new CliConnectorError(failure.code, `账号已退出登录。${failure.message}`, failure.recovery);
      throw failure;
    } finally {
      this.uninstalling.delete(providerId);
      this.removalTokens.delete(providerId);
      await reservation?.release();
    }
    const next = await this.refresh(providerId);
    return {
      ...next,
      uninstallMessage: [
        "已退出登录并卸载 Awaken 管理的运行组件。",
        next.readiness.install === "installed" ? "检测到本机其他安装，已按实际状态显示。" : "",
        removeOfficialSkills ? `已移除 ${removedSkills} 项可确认归属的官方技能。` : "已保留官方技能。",
        retained.length ? `以下技能因缺少归属记录或内容已变化而保留：${retained.join("、")}。` : "",
        "其他产品的安装文件保持不变。",
      ].filter(Boolean).join(""),
    };
  }

  async dispose(): Promise<void> {
    this.disposed = true;
    const sessions = [...this.authSessions.values()];
    for (const controller of this.installControllers.values()) controller.abort();
    for (const session of sessions) { session.controller.abort(); session.child?.kill(); }
    this.installControllers.clear();
    this.authSessions.clear();
    await Promise.all(sessions.map((session) => this.cleanupSessionFiles(session)));
  }

  private async refresh(providerId: CliProviderId): Promise<CliConnectorView> {
    if (this.uninstalling.has(providerId)) return this.view(providerId);
    const epoch = this.authEpochs.get(providerId);
    const commit = (readiness: CliConnectorReadiness) =>
      this.disposed || this.authEpochs.get(providerId) !== epoch ? this.view(providerId) : this.setReadiness(providerId, readiness);
    if (this.options.inspectSharedComponent) {
      try { await this.syncSharedComponent(providerId); }
      catch {
        return commit({ ...this.defaultView(providerId).readiness, safeError: { code: "CLI_COMPONENT_STATE_UNAVAILABLE", message: "本地服务尚未就绪，暂时无法检查运行组件", recovery: "retry" } });
      }
    }
    if (this.installations.records[providerId]?.pendingUninstall) {
      return this.setReadiness(providerId, { ...this.defaultView(providerId).readiness,
        authentication: "required", overall: "action_required",
        safeError: { code: "CLI_UNINSTALL_PENDING", message: "账号已退出，卸载尚未完成，请重试卸载", recovery: "retry" },
      });
    }
    const asset = this.assetFor(providerId);
    if (!asset) {
      return commit({
        providerId,
        activation: "enabled",
        install: "unsupported",
        executable: "missing",
        configuration: "unknown",
        authentication: "unknown",
        overall: "not_ready",
        safeError: { code: "CLI_UNSUPPORTED", message: "当前平台或架构不受支持" },
        updatedAt: new Date().toISOString(),
      });
    }
    const record = this.installations.records[providerId];
    const candidates = await this.discoverCandidates(providerId, record);
    const officialPath = this.officialCommandPath(providerId);
    const officialUsable = await isExecutableFile(officialPath);
    const recordedUsable = record?.executablePath && await isExecutableFile(record.executablePath)
      ? record.executablePath
      : undefined;
    // Prefer the official user-bin command when it exists so plugin auth, PATH
    // lookup and Bash resolve the same file. A foreign file at that path wins
    // over the hashed managed copy; Awaken still does not overwrite it.
    const executablePath = officialUsable
      ? officialPath
      : recordedUsable
        ?? candidates.find((candidate) => ownershipForPath(candidate, this.options.homeDir) === "awaken-managed")
        ?? candidates[0];
    if (!executablePath) {
      return commit({
        providerId,
        activation: record?.activation ?? "enabled",
        install: "missing",
        executable: "missing",
        configuration: "unknown",
        authentication: "unknown",
        overall: "not_ready",
        updatedAt: new Date().toISOString(),
      });
    }
    try {
      const version = await this.probeVersion(providerId, executablePath);
      const managedPath = record?.ownership === "awaken-managed" ? record.executablePath : "";
      const sameAsManaged = Boolean(managedPath) && (
        executablePath === managedPath || await resolvesToSameFile(executablePath, managedPath)
      );
      const displayedOwnership = sameAsManaged
        ? "awaken-managed"
        : ownershipForPath(executablePath, this.options.homeDir);
      const ownership = record?.ownership === "awaken-managed" ? "awaken-managed" : displayedOwnership;
      const activation = record?.activation ?? "enabled";
      const status = await this.authStatus(providerId, executablePath);
      const readiness: CliConnectorReadiness = {
        providerId,
        activation,
        install: "installed",
        executable: "available",
        configuration: status.configuration,
        authentication: status.authentication,
        overall: activation === "disabled" ? "disabled" : status.overall,
        version,
        executablePath,
        ownership: displayedOwnership,
        installMethod: ownership === "awaken-managed"
          ? (record?.installMethod ?? "managed-download")
          : (record?.installMethod ?? installMethodForPath(executablePath)),
        account: status.account,
        safeError: status.safeError,
        updatedAt: new Date().toISOString(),
      };
      if (this.authEpochs.get(providerId) !== epoch) return this.view(providerId);
      if (!record || record.executablePath !== executablePath || record.ownership !== ownership) {
        this.installations.records[providerId] = {
          ...record,
          providerId,
          executablePath,
          version,
          platform: this.supportedPlatform(),
          architecture: this.options.architecture,
          manifestRevision: this.manifest.revision,
          assetSha256: "",
          ownership,
          installMethod: ownership === "awaken-managed"
            ? (record?.installMethod ?? "managed-download")
            : installMethodForPath(executablePath),
          activation,
          lastVerifiedAt: readiness.updatedAt,
        };
        await this.saveInstallations();
      }
      return commit(readiness);
    } catch (error) {
      const mapped = mapError(error, "CLI_EXECUTABLE_INVALID");
      return commit({
        providerId,
        activation: record?.activation ?? "enabled",
        install: "installed",
        executable: "invalid",
        configuration: "unknown",
        authentication: "unknown",
        overall: "failed",
        executablePath,
        safeError: { code: mapped.code, message: mapped.message, recovery: mapped.recovery },
        updatedAt: new Date().toISOString(),
      });
    }
  }

  private async authStatus(providerId: CliProviderId, executablePath: string): Promise<{
    configuration: "required" | "ready" | "failed";
    authentication: "required" | "authorized";
    overall: "action_required" | "ready";
    account?: CliAccountProfile;
    safeError?: { code: string; message: string; recovery?: string };
  }> {
    await this.ensureAuthRoot();
    const args = providerId === "dingtalk-workspace"
      ? ["auth", "status", "--format", "json"]
      : ["auth", "status", "--json", "--verify"];
    const result = await this.runCommand(executablePath, args, this.authRoot, 30_000);
    const envelope = parseJSONObject(result.stdout) ?? parseJSONObject(result.stderr);
    if (result.exitCode === 0) {
      const authenticated = providerId === "dingtalk-workspace"
        ? booleanField(envelope, ["authenticated"])
        : isLarkUserAuthorized(envelope);
      if (authenticated) {
        // Provider auth status is the readiness source of truth. For Lark,
        // top-level verified describes the effective identity and can remain
        // true for the bot after user logout, so only a verified, available
        // user identity proves the connector's user authorization.
        const account = await this.loadAccountProfile(providerId, executablePath, envelope ?? null);
        return {
          configuration: "ready",
          authentication: "authorized",
          overall: "ready",
          account,
        };
      }
    }
    const errorType = stringField(envelope, ["subtype", "type", "code"]).toLowerCase();
    const configurationRequired = providerId === "lark-cli" && (errorType.includes("config") || errorType.includes("app"));
    return {
      configuration: configurationRequired ? "required" : "ready",
      authentication: "required",
      overall: "action_required",
    };
  }

  private async loadAccountProfile(
    providerId: CliProviderId,
    executablePath: string,
    authEnvelope: Record<string, unknown> | null,
  ): Promise<CliAccountProfile | undefined> {
    if (providerId === "lark-cli") {
      const identities = authEnvelope && typeof authEnvelope.identities === "object"
        ? authEnvelope.identities as Record<string, unknown>
        : undefined;
      const user = identities && typeof identities.user === "object"
        ? identities.user as Record<string, unknown>
        : undefined;
      const displayName = typeof user?.userName === "string" ? user.userName.trim() : "";
      return displayName ? { displayName } : undefined;
    }

    // DingTalk: auth status only has corp_id; profile comes from get-self.
    const profile = await this.runCommand(
      executablePath,
      ["contact", "user", "get-self", "--format", "json"],
      this.authRoot,
      20_000,
    );
    const envelope = parseJSONObject(profile.stdout) ?? parseJSONObject(profile.stderr);
    if (!envelope || profile.exitCode !== 0) return undefined;
    const rows = Array.isArray(envelope.result) ? envelope.result : [];
    const first = rows[0] && typeof rows[0] === "object" ? rows[0] as Record<string, unknown> : undefined;
    const model = first?.orgEmployeeModel && typeof first.orgEmployeeModel === "object"
      ? first.orgEmployeeModel as Record<string, unknown>
      : undefined;
    const displayName = typeof model?.orgUserName === "string" ? model.orgUserName.trim() : "";
    const organization = typeof model?.orgName === "string" ? model.orgName.trim() : "";
    if (!displayName && !organization) return undefined;
    return {
      ...(displayName ? { displayName } : {}),
      ...(organization ? { organization } : {}),
    };
  }

  private async startLarkSplitAuth(session: AuthSessionInternal, executable: string): Promise<void> {
    try {
      const initiated = await this.runCommand(executable, ["auth", "login", "--recommend", "--no-wait", "--json"], session.directory, 30_000, session.controller.signal);
      if (!this.ownsAuth(session)) return;
      if (initiated.exitCode !== 0) throw new CliConnectorError("CLI_AUTH_START_FAILED", "无法发起飞书授权", "retry");
      const payload = parseJSONObject(initiated.stdout);
      const verificationUrl = stringField(payload, ["verification_url", "verification_uri_complete", "verification_uri"]);
      const deviceCode = stringField(payload, ["device_code"]);
      if (!verificationUrl || !deviceCode) throw new CliConnectorError("CLI_AUTH_RESPONSE_INVALID", "飞书授权响应缺少必要字段", "retry");
      session.deviceCode = deviceCode;
      session.view.phase = "waiting_user";
      session.view.safeMessage = "请在已打开的飞书页面完成授权";
      this.emit("lark-cli");
      await this.presentVerificationUrl(session, verificationUrl);
      if (!this.ownsAuth(session)) return;
      session.view.phase = "confirming";
      session.view.safeMessage = "等待飞书确认授权…";
      const child = await this.spawnLeased(executable, ["auth", "login", "--device-code", deviceCode, "--json"], session.directory, session.controller.signal);
      drainChildOutput(child);
      session.child = child;
      this.emit("lark-cli");
      await this.finishBlockingSession(session, child, false);
    } catch (error) {
      await this.failSession(session, mapError(error, "CLI_AUTH_FAILED"));
    }
  }

  private async finishBlockingSession(
    session: AuthSessionInternal, child: ChildProcess, configurationOnly: boolean, continueWithAuth = false,
  ): Promise<void> {
    const providerId = session.view.provider;
    try {
      const exitCode = await childExit(child);
      if (!this.ownsAuth(session) || session.child !== child) return;
      if (exitCode !== 0) throw new CliConnectorError(configurationOnly ? "CLI_CONFIGURE_FAILED" : "CLI_AUTH_FAILED",
        configurationOnly ? "CLI 配置未完成" : "CLI 授权未完成", "retry");
      delete session.deviceCode;
      const refreshed = await this.refresh(providerId);
      if (!this.ownsAuth(session)) return;
      if (configurationOnly ? refreshed.readiness.configuration !== "ready" : refreshed.readiness.authentication !== "authorized") {
        throw new CliConnectorError(configurationOnly ? "CLI_CONFIGURE_VERIFY_FAILED" : "CLI_AUTH_VERIFY_FAILED", "授权配置结果验证失败", "retry");
      }
      await this.cleanupSessionFiles(session);
      if (!this.ownsAuth(session)) return;
      session.view.phase = "authorized";
      session.view.safeMessage = "授权成功";
      this.authSessions.delete(providerId);
      this.advanceAuthEpoch(providerId);
      this.emit(providerId);
      if (configurationOnly && continueWithAuth) await this.startAuth(providerId);
    } catch (error) {
      await this.failSession(session, mapError(error, "CLI_AUTH_FAILED"), configurationOnly);
    }
  }

  private async failSession(session: AuthSessionInternal, error: CliConnectorError, configurationOnly = false): Promise<void> {
    if (!this.ownsAuth(session)) return;
    const providerId = session.view.provider;
    session.controller.abort();
    session.child?.kill();
    delete session.deviceCode;
    session.view.phase = "failed";
    session.view.safeMessage = error.message;
    try { await this.cleanupSessionFiles(session); } catch {
      error = new CliConnectorError("CLI_AUTH_CLEANUP_FAILED", "授权临时文件清理失败，请重试", "retry");
    }
    if (!this.ownsAuth(session)) return;
    this.setReadiness(providerId, {
      ...this.readiness(providerId),
      configuration: configurationOnly ? "failed" : this.readiness(providerId).configuration,
      authentication: configurationOnly ? this.readiness(providerId).authentication : "failed",
      overall: "failed", safeError: { code: error.code, message: error.message, recovery: error.recovery },
      updatedAt: new Date().toISOString(),
    });
  }

  private ownsAuth(session: AuthSessionInternal): boolean {
    return !this.disposed && this.authSessions.get(session.view.provider) === session;
  }

  private advanceAuthEpoch(providerId: CliProviderId): void {
    this.authEpochs.set(providerId, (this.authEpochs.get(providerId) ?? 0) + 1);
  }

  private createAuthSession(providerId: CliProviderId, message: string): AuthSessionInternal {
    const id = randomUUID();
    const session: AuthSessionInternal = {
      directory: path.join(this.authRoot, providerId, id), controller: new AbortController(),
      view: { id, provider: providerId, phase: "starting", safeMessage: message },
    };
    this.authSessions.set(providerId, session);
    this.advanceAuthEpoch(providerId);
    this.emit(providerId);
    return session;
  }

  private async cleanupSessionFiles(session: AuthSessionInternal): Promise<void> {
    await session.filesReady?.catch(() => undefined);
    await fs.rm(session.directory, { recursive: true, force: true });
  }

  private async ensureAuthRoot(): Promise<void> {
    await fs.mkdir(this.authRoot, { recursive: true, mode: 0o700 });
    await fs.chmod(this.authRoot, 0o700);
  }

  private monitorVerificationUrl(
    session: AuthSessionInternal,
    child: ChildProcess,
    safeMessage: string,
  ): void {
    let buffered = "";
    const onData = (chunk: Buffer | string) => {
      buffered = `${buffered}${chunk.toString()}`.slice(-commandOutputLimit);
      const verificationUrl = authVerificationUrl(buffered, session.view.provider);
      if (!verificationUrl || session.openedVerificationUrl === verificationUrl) return;
      session.view.safeMessage = safeMessage;
      void this.presentVerificationUrl(session, verificationUrl).catch((error) => {
        void this.failSession(session, mapError(error, "CLI_AUTH_PAGE_OPEN_FAILED"));
      });
    };
    child.stdout?.on("data", onData);
    child.stderr?.on("data", onData);
  }

  private async presentVerificationUrl(session: AuthSessionInternal, verificationUrl: string): Promise<void> {
    if (!this.ownsAuth(session)) return;
    const safeUrl = authVerificationUrl(verificationUrl, session.view.provider);
    if (!safeUrl || session.openedVerificationUrl === safeUrl) return;
    session.openedVerificationUrl = safeUrl;
    this.emit(session.view.provider);
    try {
      await this.options.openExternal?.(safeUrl);
    } catch {
      throw new CliConnectorError("CLI_AUTH_PAGE_OPEN_FAILED", "无法打开官方授权页面", "retry");
    }
  }

  private officialCommandPath(providerId: CliProviderId): string {
    const executableName = providerId === "dingtalk-workspace"
      ? (this.options.platform === "win32" ? "dws.exe" : "dws")
      : (this.options.platform === "win32" ? "lark-cli.exe" : "lark-cli");
    return path.join(this.options.homeDir, ".local", "bin", executableName);
  }

  private async discoverCandidates(providerId: CliProviderId, record?: InstallationRecord): Promise<string[]> {
    const executableName = providerId === "dingtalk-workspace"
      ? (this.options.platform === "win32" ? "dws.exe" : "dws")
      : (this.options.platform === "win32" ? "lark-cli.exe" : "lark-cli");
    const candidates = new Set<string>();
    if (record?.executablePath && await isExecutableFile(record.executablePath)) candidates.add(record.executablePath);
    const pathEntries = (process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
    for (const entry of pathEntries) {
      const candidate = path.join(entry, executableName);
      if (await isExecutableFile(candidate)) candidates.add(path.resolve(candidate));
    }
    const defaults = this.options.platform === "darwin"
      ? [this.officialCommandPath(providerId), path.join("/opt/homebrew/bin", executableName), path.join("/usr/local/bin", executableName)]
      : [this.officialCommandPath(providerId), path.join(process.env.LOCALAPPDATA ?? "", executableName), path.join(process.env.APPDATA ?? "", "npm", executableName)];
    for (const candidate of defaults) if (candidate && await isExecutableFile(candidate)) candidates.add(path.resolve(candidate));
    return [...candidates].sort();
  }

  private async probeVersion(providerId: CliProviderId, executablePath: string): Promise<string> {
    const args = providerId === "dingtalk-workspace" ? ["version", "--format", "json"] : ["--version"];
    const result = await this.runCommand(executablePath, args, os.tmpdir(), 15_000);
    if (result.exitCode !== 0) throw new CliConnectorError("CLI_VERSION_PROBE_FAILED", "CLI 版本探测失败", "select_executable");
    const payload = parseJSONObject(result.stdout);
    const raw = providerId === "dingtalk-workspace"
      ? stringField(payload, ["version"])
      : result.stdout.trim();
    const match = raw.match(/v?(\d+\.\d+\.\d+)/);
    if (!match) throw new CliConnectorError("CLI_VERSION_INVALID", "无法识别 CLI 版本", "select_executable");
    return match[1];
  }

  private async installOfficialSkills(
    providerId: CliProviderId,
    signal: AbortSignal,
    reportProgress = true,
  ): Promise<{ names: string[]; digests: Record<string, string> }> {
    const bundle = this.skillBundleFor(providerId);
    if (!bundle) throw new CliConnectorError("CLI_SKILL_BUNDLE_MISSING", "官方 Skill 安装资产缺失");
    const staging = path.join(this.root, ".staging", `${providerId}-skills-${randomUUID()}`);
    await fs.mkdir(staging, { recursive: true, mode: 0o700 });
    const archivePath = path.join(staging, `skills.${bundle.archiveType === "zip" ? "zip" : "tar.gz"}`);
    try {
      if (reportProgress) this.setInstallProgress(providerId, "downloading_skills", 62, "正在下载官方 Skills");
      const response = await this.fetchAsset(bundle.url, signal, "cli-skill-download");
      if (!response.ok) throw new CliConnectorError("CLI_SKILL_DOWNLOAD_FAILED", `官方 Skill 下载失败 (${response.status})`, "retry");
      const declaredLength = Number(response.headers.get("content-length") ?? 0);
      if (declaredLength > maxArchiveBytes) throw new CliConnectorError("CLI_SKILL_ARCHIVE_TOO_LARGE", "官方 Skill 包超过安全大小限制");
      const bytes = await readResponseBody(response, maxArchiveBytes, (received, total) => {
        const ratio = total > 0 ? Math.min(received / total, 1) : 0.5;
        if (reportProgress) this.setInstallProgress(providerId, "downloading_skills", 62 + Math.round(ratio * 24), "正在下载官方 Skills");
      });
      if (reportProgress) this.setInstallProgress(providerId, "verifying_skills", 88, "正在校验官方 Skills");
      if (sha256(bytes) !== bundle.sha256) throw new CliConnectorError("CLI_SKILL_CHECKSUM_MISMATCH", "官方 Skill 包校验失败", "retry");
      await fs.writeFile(archivePath, bytes, { mode: 0o600 });
      const payload = path.join(staging, "payload");
      await extractSkillBundle(archivePath, bundle.archiveType, bundle, payload);
      if (reportProgress) this.setInstallProgress(providerId, "installing_skills", 93, "正在安装官方 Skills");
      const digests: Record<string, string> = {};
      for (const name of bundle.skillNames) {
        const digest = await skillDirectoryDigest(path.join(payload, name));
        if (digest) digests[name] = digest;
      }
      await replaceSkillDirectoriesAtomically(payload, path.join(this.options.homeDir, ".agents", "skills"), bundle.skillNames);
      return { names: [...bundle.skillNames], digests };
    } catch (error) {
      if (error instanceof CliConnectorError) throw error;
      throw new CliConnectorError("CLI_SKILL_INSTALL_FAILED", "官方 Skill 安装失败", "retry");
    } finally {
      await fs.rm(staging, { recursive: true, force: true });
    }
  }

  private async backfillOfficialSkills(): Promise<void> {
    let changed = false;
    for (const providerId of providers) {
      const record = this.installations.records[providerId];
      const bundle = this.skillBundleFor(providerId);
      if (!record || record.ownership !== "awaken-managed" || record.pendingUninstall || !bundle || !this.ownershipHydrated.has(providerId) || this.backfillAttempts.has(providerId) || this.installControllers.has(providerId) || this.uninstalling.has(providerId)) continue;
      const skillFiles = bundle.skillNames.map((name) => path.join(this.options.homeDir, ".agents", "skills", name, "SKILL.md"));
      const presence = await Promise.all(skillFiles.map((file) => isRegularFile(file)));
      if (presence.every(Boolean)) continue;
      if (presence.some(Boolean)) continue; // Preserve a partial set owned by another installer or the user.
      this.backfillAttempts.add(providerId);
      const controller = new AbortController();
      let settle!: () => void;
      this.installControllers.set(providerId, controller);
      this.installSettled.set(providerId, new Promise<void>((resolve) => { settle = resolve; }));
      try {
        const installed = await this.installOfficialSkills(providerId, controller.signal, false);
        record.officialSkillNames = installed.names;
        record.officialSkillDigests = installed.digests;
        record.officialSkillVersion = bundle.version;
        changed = true;
      } catch {
        // Keep an already usable CLI available while retrying the migration on next launch.
      } finally {
        this.installControllers.delete(providerId);
        this.installSettled.delete(providerId);
        settle();
      }
    }
    if (changed) await this.saveInstallations();
  }

  private async loadManifest(): Promise<CliManifest> {
    const raw = JSON.parse(await fs.readFile(this.options.manifestPath, "utf8")) as CliManifest;
    if (!raw || typeof raw.revision !== "string" || !Array.isArray(raw.assets) || (raw.skillBundles !== undefined && !Array.isArray(raw.skillBundles))) throw new CliConnectorError("CLI_MANIFEST_INVALID", "CLI manifest 无效");
    const identities = new Set<string>();
    for (const asset of raw.assets) {
      if (!providers.includes(asset.providerId) || !["darwin", "win32"].includes(asset.platform) || !["arm64", "x64"].includes(asset.architecture)) throw new CliConnectorError("CLI_MANIFEST_INVALID", "CLI manifest 包含无效平台条目");
      if (!/^https:\/\/github\.com\//.test(asset.url) || !/^[0-9a-f]{64}$/.test(asset.sha256)) throw new CliConnectorError("CLI_MANIFEST_INVALID", "CLI manifest 资产来源或校验值无效");
      const identity = `${asset.providerId}:${asset.platform}:${asset.architecture}`;
      if (identities.has(identity)) throw new CliConnectorError("CLI_MANIFEST_INVALID", "CLI manifest 包含重复条目");
      identities.add(identity);
    }
    const skillProviders = new Set<string>();
    for (const bundle of raw.skillBundles ?? []) {
      if (!providers.includes(bundle.providerId)
        || !/^https:\/\/github\.com\//.test(bundle.url)
        || !/^[0-9a-f]{64}$/.test(bundle.sha256)
        || !["tar.gz", "zip"].includes(bundle.archiveType)
        || !Array.isArray(bundle.skillNames)
        || bundle.skillNames.length === 0
        || bundle.skillNames.some((name) => !/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(name))
        || (bundle.destinationRoot !== undefined && (bundle.skillNames.length !== 1 || bundle.skillNames[0] !== bundle.destinationRoot))) {
        throw new CliConnectorError("CLI_MANIFEST_INVALID", "CLI manifest 包含无效 Skill 资产");
      }
      if (skillProviders.has(bundle.providerId)) throw new CliConnectorError("CLI_MANIFEST_INVALID", "CLI manifest 包含重复 Skill 资产");
      skillProviders.add(bundle.providerId);
    }
    return raw;
  }

  private async syncSharedComponent(providerId: CliProviderId): Promise<void> {
    const installed = await this.options.inspectSharedComponent!(providerId);
    const previous = this.installations.records[providerId];
    this.ownershipHydrated.add(providerId);
    if (installed) {
      this.installations.records[providerId] = {
        ...previous,
        providerId, executablePath: installed.executablePath, version: installed.version,
        platform: this.options.platform === "win32" ? "win32" : "darwin", architecture: this.options.architecture,
        manifestRevision: this.manifest.revision, assetSha256: "", ownership: "awaken-managed", installMethod: "managed-download",
        activation: previous?.activation ?? "enabled", lastVerifiedAt: previous?.lastVerifiedAt ?? "",
      };
    } else if (previous?.ownership === "awaken-managed") {
      // Keep connector/Skill state, but never turn a stale Main receipt into
      // permission to delete a binary which Core does not own.
      previous.executablePath = "";
      previous.ownership = "user-managed";
    }
    await this.saveInstallations();
  }

  private async loadInstallations(): Promise<StoredInstallations> {
    try {
      let content: string;
      try { content = await fs.readFile(this.installationsPath, "utf8"); }
      catch (error) {
        if (!this.options.inspectSharedComponent || (error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
        content = await fs.readFile(path.join(this.root, "installations.json"), "utf8");
      }
      const parsed = JSON.parse(content);
      if (parsed?.version === 1 && parsed.records) {
        return parsed as StoredInstallations;
      }
      if (parsed?.version !== 2 || !parsed.records) throw new Error("unsupported connector state");
      const records: StoredInstallations["records"] = {};
      for (const providerId of providers) {
        const state = parsed.records[providerId];
        if (!state) continue;
        records[providerId] = {
          providerId, executablePath: state.externalExecutable ?? "", version: "",
          platform: this.options.platform === "win32" ? "win32" : "darwin", architecture: this.options.architecture,
          manifestRevision: "", assetSha256: "", ownership: "user-managed", installMethod: "unknown", lastVerifiedAt: "",
          activation: state.activation ?? "enabled", officialSkillVersion: state.officialSkillVersion,
          officialSkillNames: state.officialSkillNames, officialSkillDigests: state.officialSkillDigests,
          pendingUninstall: state.pendingUninstall,
        };
      }
      return { version: 1, records };
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ENOENT") return { version: 1, records: {} };
      throw new CliConnectorError("CLI_INSTALLATIONS_INVALID", "CLI 状态记录无法读取");
    }
  }

  private saveInstallations(): Promise<void> {
    const next = this.stateWrites.then(() => this.persistInstallations());
    this.stateWrites = next.catch(() => undefined);
    return next;
  }

  private async persistInstallations(): Promise<void> {
    await fs.mkdir(path.dirname(this.installationsPath), { recursive: true, mode: 0o700 });
    let stored: unknown;
    {
      const records = Object.fromEntries(Object.entries(this.installations.records).map(([id, record]) => [id, {
        activation: record.activation, officialSkillVersion: record.officialSkillVersion,
        officialSkillNames: record.officialSkillNames, officialSkillDigests: record.officialSkillDigests,
        pendingUninstall: record.pendingUninstall,
        externalExecutable: record.ownership !== "awaken-managed" ? record.executablePath || undefined : undefined,
      }]));
      stored = { version: 2, records };
    }
    const encoded = `${JSON.stringify(stored, null, 2)}\n`;
    if (encoded !== this.savedState) {
      const temporary = `${this.installationsPath}.${randomUUID()}.tmp`;
      try {
        await fs.writeFile(temporary, encoded, { mode: 0o600 });
        await fs.rename(temporary, this.installationsPath);
        this.savedState = encoded;
      } finally { await fs.rm(temporary, { force: true }); }
    }
    if (this.options.inspectSharedComponent && providers.every((id) => this.ownershipHydrated.has(id))) {
      const legacy = path.join(this.root, "installations.json");
      // Core has imported both providers and connector state is durable. The
      // archive is never read as an ownership source by either current owner.
      try { await fs.rename(legacy, `${legacy}.migrated`); }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }
    }
  }

  private assetFor(providerId: CliProviderId): ManifestAsset | undefined {
    if (this.options.platform !== "darwin" && this.options.platform !== "win32") return undefined;
    if (this.options.architecture !== "arm64" && this.options.architecture !== "x64") return undefined;
    return this.manifest.assets.find((asset) => asset.providerId === providerId && asset.platform === this.options.platform && asset.architecture === this.options.architecture);
  }

  private skillBundleFor(providerId: CliProviderId): ManifestSkillBundle | undefined {
    return this.manifest.skillBundles?.find((bundle) => bundle.providerId === providerId);
  }

  private supportedPlatform(): "darwin" | "win32" {
    if (this.options.platform !== "darwin" && this.options.platform !== "win32") throw new CliConnectorError("CLI_UNSUPPORTED", "当前平台不受支持");
    return this.options.platform;
  }

  private requireExecutable(providerId: CliProviderId): string {
    const executable = this.readiness(providerId).executablePath;
    if (!executable || this.readiness(providerId).executable !== "available") throw new CliConnectorError("CLI_EXECUTABLE_MISSING", "请先安装或选择 CLI");
    return executable;
  }

  private setReadiness(providerId: CliProviderId, readiness: CliConnectorReadiness): CliConnectorView {
    const view: CliConnectorView = {
      providerId,
      canUninstallManaged: Boolean(this.installations.records[providerId]?.pendingUninstall || (this.ownershipHydrated.has(providerId) && this.installations.records[providerId]?.ownership === "awaken-managed")),
      displayName: providerId === "dingtalk-workspace" ? "钉钉" : "飞书",
      readiness,
      authSession: this.authSessions.get(providerId)?.view,
      installProgress: this.installProgress.get(providerId),
    };
    this.views.set(providerId, view);
    this.emit(providerId);
    void this.options.publishReadiness(readiness).catch(() => undefined);
    return structuredClone(view);
  }

  private emit(providerId: CliProviderId): void {
    const current = this.views.get(providerId) ?? this.defaultView(providerId);
    const session = this.authSessions.get(providerId)?.view;
    const view = structuredClone({
      ...current,
      authSession: session,
      installProgress: this.installProgress.get(providerId),
    });
    this.views.set(providerId, view);
    this.options.onStateChanged(view);
  }

  private view(providerId: CliProviderId): CliConnectorView {
    this.assertProvider(providerId);
    const current = this.views.get(providerId) ?? this.defaultView(providerId);
    return structuredClone({ ...current, installProgress: this.installProgress.get(providerId) });
  }

  private setInstallProgress(
    providerId: CliProviderId,
    phase: CliInstallPhase,
    percent: number,
    label: string,
  ): void {
    const previous = this.installProgress.get(providerId)?.percent ?? 0;
    this.installProgress.set(providerId, {
      phase,
      // Shared preparation has independent component/download phases, not a
      // trustworthy whole-install denominator. Show its real phase only.
      percent: this.options.prepareSharedComponent ? undefined : Math.max(previous, Math.min(100, Math.round(percent))),
      label,
    });
    this.emit(providerId);
  }

  private readiness(providerId: CliProviderId): CliConnectorReadiness {
    return this.views.get(providerId)?.readiness ?? this.defaultView(providerId).readiness;
  }

  private defaultView(providerId: CliProviderId): CliConnectorView {
    return {
      providerId,
      displayName: providerId === "dingtalk-workspace" ? "钉钉" : "飞书",
      readiness: {
        providerId,
        activation: "enabled",
        install: "missing",
        executable: "unknown",
        configuration: "unknown",
        authentication: "unknown",
        overall: "not_ready",
        updatedAt: new Date().toISOString(),
      },
    };
  }

  private assertProvider(providerId: string): asserts providerId is CliProviderId {
    if (!providers.includes(providerId as CliProviderId)) throw new CliConnectorError("CLI_PROVIDER_UNKNOWN", "未知 CLI provider");
  }

  private commandEnvironment(): NodeJS.ProcessEnv {
    return this.options.environment?.() ?? process.env;
  }

  private spawnOptions(cwd: string) {
    return safeSpawnOptions(cwd, this.commandEnvironment());
  }

  private async holdExecutable(executable: string): Promise<() => Promise<void>> {
    const provider = providers.find((id) => this.installations.records[id]?.executablePath === executable);
    const token = provider ? this.removalTokens.get(provider) : undefined;
    if (this.options.holdSharedExecutable && provider && this.uninstalling.has(provider) && !token) this.assertNotUninstalling(provider);
    let settle!: () => void;
    const pending = new Promise<void>((resolve) => { settle = resolve; });
    const commands = provider ? this.pendingCliCommands.get(provider) ?? new Set<Promise<void>>() : undefined;
    if (provider && commands) { commands.add(pending); this.pendingCliCommands.set(provider, commands); }
    const finish = () => { commands?.delete(pending); settle(); };
    try {
      const release = await this.options.holdSharedExecutable?.(executable, token) ?? (async () => {});
      return async () => { try { await release(); } finally { finish(); } };
    } catch (error) { finish(); throw error; }
  }

  private async spawnLeased(executable: string, args: string[], cwd: string, signal: AbortSignal): Promise<ChildProcess> {
    const release = await this.holdExecutable(executable);
    try {
      signal.throwIfAborted();
      const child = spawn(executable, args, { ...this.spawnOptions(cwd), signal });
      child.once("close", () => { void release().catch((error) => console.warn("CLI runtime lease release failed", error)); });
      return child;
    } catch (error) { await release(); throw error; }
  }

  private async runCommand(executable: string, args: string[], cwd: string, timeoutMs: number, signal?: AbortSignal): Promise<CommandResult> {
    await this.prepareNetworkOperation("cli-command");
    signal?.throwIfAborted();
    const release = await this.holdExecutable(executable);
    try {
      const result = await runCliCommand(executable, args, cwd, timeoutMs, this.commandEnvironment(), signal);
      if (looksLikeNetworkTransportFailure(`${result.stderr}\n${result.stdout}`)) {
        this.options.onTransportFailure?.("cli-command");
      }
      return result;
    } catch (error) {
      if (looksLikeNetworkTransportFailure(error instanceof Error ? error.message : String(error))) {
        this.options.onTransportFailure?.("cli-command");
      }
      throw error;
    } finally {
      await release();
    }
  }

  private prepareNetworkOperation(surface: string): Promise<void> {
    return this.options.beforeNetworkOperation?.(surface) ?? Promise.resolve();
  }

  private async fetchAsset(url: string, signal: AbortSignal, surface: string): Promise<Response> {
    await this.prepareNetworkOperation(surface);
    try {
      return await this.options.fetchAsset(url, signal);
    } catch (error) {
      this.options.onTransportFailure?.(surface);
      throw error;
    }
  }
}

function safeSpawnOptions(cwd: string, environment: NodeJS.ProcessEnv = process.env) {
  return {
    cwd,
    shell: false,
    windowsHide: true,
    env: {
      ...safeCliEnvironment(environment),
      LARKSUITE_CLI_NO_UPDATE_NOTIFIER: "1",
      LARKSUITE_CLI_NO_SKILLS_NOTIFIER: "1",
    },
    stdio: ["ignore", "pipe", "pipe"] as ["ignore", "pipe", "pipe"],
  };
}

export function authVerificationUrl(raw: string, providerId: CliProviderId): string | undefined {
  const candidates: string[] = [];
  const parsed = parseJSONObject(raw);
  if (parsed) {
    for (const key of ["verification_url", "verification_uri_complete", "verification_uri", "url"]) {
      const value = stringField(parsed, [key]);
      if (value) candidates.push(value);
    }
  }
  const matches = raw.match(/https:\/\/[^\s"'<>\\]+/g) ?? [];
  candidates.push(...matches.map((value) => value.replace(/[),.;\]}]+$/, "")));
  for (const candidate of candidates) {
    try {
      const url = new URL(candidate);
      if (url.protocol !== "https:") continue;
      const allowed = providerId === "dingtalk-workspace"
        ? isDingTalkVerificationUrl(url)
        : isLarkVerificationUrl(url);
      if (allowed) return url.toString();
    } catch {
      // Ignore malformed or non-provider URLs from CLI output.
    }
  }
  return undefined;
}

function isLarkUserAuthorized(envelope: Record<string, unknown> | undefined): boolean {
  if (!envelope || typeof envelope.identities !== "object" || !envelope.identities) return false;
  const identities = envelope.identities as Record<string, unknown>;
  if (typeof identities.user !== "object" || !identities.user) return false;
  const user = identities.user as Record<string, unknown>;
  return user.available === true && user.verified === true;
}

function isDingTalkVerificationUrl(url: URL): boolean {
  const host = url.hostname.toLowerCase();
  const pathname = url.pathname.length > 1 ? url.pathname.replace(/\/+$/, "") : url.pathname;
  return host === "login.dingtalk.com"
    && (url.port === "" || url.port === "443")
    && pathname === "/oauth2/auth";
}

function isLarkVerificationUrl(url: URL): boolean {
  const host = url.hostname.toLowerCase();
  if (!larkVerificationHosts.has(host)) return false;

  const pathname = url.pathname.length > 1 ? url.pathname.replace(/\/+$/, "") : url.pathname;
  return !larkServiceEndpoints.has(`${host}${pathname}`);
}

export function safeCliEnvironment(source: NodeJS.ProcessEnv = process.env): NodeJS.ProcessEnv {
  const allowed = [
    "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PATH", "TMPDIR", "TEMP", "TMP",
    "SystemRoot", "ComSpec", "LANG", "LC_ALL", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
    "http_proxy", "https_proxy", "all_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
    "XDG_CONFIG_HOME", "XDG_DATA_HOME",
  ];
  const env: NodeJS.ProcessEnv = {};
  for (const key of allowed) if (source[key] !== undefined) env[key] = source[key];
  return env;
}

export async function readResponseBody(
  response: Response,
  limit: number,
  onProgress?: (receivedBytes: number, totalBytes: number) => void,
): Promise<Buffer> {
  if (!response.body) return Buffer.alloc(0);
  const declaredLength = Number(response.headers.get("content-length") ?? 0);
  const reader = response.body.getReader();
  const chunks: Buffer[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > limit) {
        await reader.cancel();
        throw new CliConnectorError("CLI_ARCHIVE_TOO_LARGE", "安装包超过安全大小限制");
      }
      chunks.push(Buffer.from(value));
      onProgress?.(size, Number.isFinite(declaredLength) ? declaredLength : 0);
    }
    return Buffer.concat(chunks, size);
  } finally {
    reader.releaseLock();
  }
}

async function replaceSkillDirectoriesAtomically(
  sourceRoot: string,
  targetRoot: string,
  skillNames: string[],
): Promise<void> {
  await fs.mkdir(targetRoot, { recursive: true, mode: 0o700 });
  const swaps: Array<{ target: string; backup?: string }> = [];
  try {
    for (const skillName of skillNames) {
      const source = path.join(sourceRoot, skillName);
      const target = path.join(targetRoot, skillName);
      if (!(await isRegularFile(path.join(source, "SKILL.md")))) {
        throw new Error("CLI_SKILL_BUNDLE_MISSING_SKILL");
      }
      let backup: string | undefined;
      if (await pathExists(target)) {
        const stat = await fs.lstat(target);
        if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error("CLI_SKILL_TARGET_UNSAFE");
        backup = path.join(targetRoot, `.${skillName}.${randomUUID()}.old`);
        await fs.rename(target, backup);
      }
      try {
        await fs.rename(source, target);
      } catch (error) {
        if (backup) await fs.rename(backup, target);
        throw error;
      }
      swaps.push({ target, backup });
    }
    await Promise.all(swaps.flatMap(({ backup }) => backup
      ? [fs.rm(backup, { recursive: true, force: true }).catch(() => undefined)]
      : []));
  } catch (error) {
    for (const { target, backup } of swaps.reverse()) {
      await fs.rm(target, { recursive: true, force: true });
      if (backup) await fs.rename(backup, target);
    }
    throw error;
  }
}

async function pathExists(value: string): Promise<boolean> {
  try {
    await fs.lstat(value);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return false;
    throw error;
  }
}

async function isRegularFile(value: string): Promise<boolean> {
  try {
    const stat = await fs.lstat(value);
    return stat.isFile() && !stat.isSymbolicLink();
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return false;
    throw error;
  }
}

export async function runCliCommand(
  executable: string,
  args: string[],
  cwd: string,
  timeoutMs: number,
  environment: NodeJS.ProcessEnv = process.env,
  signal?: AbortSignal,
): Promise<CommandResult> {
  signal?.throwIfAborted();
  const child = spawn(executable, args, { ...safeSpawnOptions(cwd, environment), signal });
  let stdout = "";
  let stderr = "";
  child.stdout?.on("data", (chunk: Buffer) => { if (stdout.length < commandOutputLimit) stdout += chunk.toString("utf8"); });
  child.stderr?.on("data", (chunk: Buffer) => { if (stderr.length < commandOutputLimit) stderr += chunk.toString("utf8"); });
  const timeout = setTimeout(() => child.kill(), timeoutMs);
  try {
    // Process exit can precede the last stdout/stderr chunks. Version and auth
    // JSON must be parsed only after the pipes close, while retaining the lease.
    return { exitCode: await childExit(child, "close"), stdout: stdout.slice(0, commandOutputLimit), stderr: stderr.slice(0, commandOutputLimit) };
  } finally {
    clearTimeout(timeout);
  }
}

function childExit(child: ChildProcess, event: "exit" | "close" = "exit"): Promise<number> {
  return new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once(event, (code) => resolve(code ?? 1));
  });
}

function parseJSONObject(raw: string): Record<string, unknown> | undefined {
  try {
    const value = JSON.parse(raw.trim()) as unknown;
    return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
  } catch {
    return undefined;
  }
}

function stringField(value: Record<string, unknown> | undefined, names: string[]): string {
  if (!value) return "";
  const queue: Record<string, unknown>[] = [value];
  while (queue.length > 0) {
    const item = queue.shift()!;
    for (const name of names) if (typeof item[name] === "string") return item[name] as string;
    for (const child of Object.values(item)) if (child && typeof child === "object" && !Array.isArray(child)) queue.push(child as Record<string, unknown>);
  }
  return "";
}

function booleanField(value: Record<string, unknown> | undefined, names: string[]): boolean {
  if (!value) return false;
  const queue: Record<string, unknown>[] = [value];
  while (queue.length > 0) {
    const item = queue.shift()!;
    for (const name of names) if (item[name] === true) return true;
    for (const child of Object.values(item)) if (child && typeof child === "object" && !Array.isArray(child)) queue.push(child as Record<string, unknown>);
  }
  return false;
}

function drainChildOutput(child: ChildProcess): void {
  child.stdout?.resume();
  child.stderr?.resume();
}

async function isExecutableFile(filePath: string, executable = true): Promise<boolean> {
  try {
    const stat = await fs.stat(filePath);
    if (!stat.isFile()) return false;
    if (executable && process.platform !== "win32") await fs.access(filePath, fsConstants.X_OK);
    return true;
  } catch {
    return false;
  }
}

function sha256(value: Buffer): string {
  return createHash("sha256").update(value).digest("hex");
}

function looksLikeNetworkTransportFailure(raw: string): boolean {
  return /(?:ECONNREFUSED|ECONNRESET|ENETUNREACH|EHOSTUNREACH|ETIMEDOUT|connection refused|network is unreachable|proxy connect|failed to fetch|dial tcp)/i.test(raw);
}

function ownershipForPath(filePath: string, homeDir: string): CliInstallOwnership {
  const relative = path.relative(homeDir, filePath);
  if (relative && !relative.startsWith("..") && !path.isAbsolute(relative)) return "user-managed";
  return "system-managed";
}

async function resolvesToSameFile(left: string, right: string): Promise<boolean> {
  try {
    const [a, b] = await Promise.all([fs.realpath(left), fs.realpath(right)]);
    return process.platform === "win32" ? a.toLowerCase() === b.toLowerCase() : a === b;
  } catch {
    const a = path.resolve(left);
    const b = path.resolve(right);
    return process.platform === "win32" ? a.toLowerCase() === b.toLowerCase() : a === b;
  }
}

function installMethodForPath(filePath: string): "npm" | "homebrew" | "system" | "unknown" {
  const normalized = filePath.toLowerCase();
  if (normalized.includes("node_modules") || normalized.includes(`${path.sep}npm${path.sep}`)) return "npm";
  if (normalized.includes("homebrew") || normalized.startsWith("/opt/homebrew/")) return "homebrew";
  if (normalized.startsWith("/usr/") || normalized.includes("program files")) return "system";
  return "unknown";
}

function mapError(error: unknown, fallbackCode: string): CliConnectorError {
  if (error instanceof CliConnectorError) return error;
  if ((error as NodeJS.ErrnoException)?.name === "AbortError") return new CliConnectorError("CLI_INSTALL_CANCELLED", "安装已取消", "retry");
  return new CliConnectorError(fallbackCode, "CLI 操作失败", "retry");
}

/** Fingerprint a complete regular-file skill tree; redirected or oversized trees are never owned for removal. */
async function skillDirectoryDigest(root: string): Promise<string | undefined> {
  try {
    if (!(await fs.lstat(root)).isDirectory()) return undefined;
    const hash = createHash("sha256");
    let bytes = 0;
    let entries = 0;
    const walk = async (directory: string): Promise<void> => {
      for (const name of (await fs.readdir(directory)).sort()) {
        if (++entries > 10000) throw new Error("too many entries");
        const file = path.join(directory, name);
        const stat = await fs.lstat(file);
        hash.update(JSON.stringify([path.relative(root, file), stat.mode & 0o777]));
        if (stat.isDirectory()) { hash.update("directory"); await walk(file); }
        else if (stat.isFile()) {
          bytes += stat.size;
          if (bytes > 64 * 1024 * 1024) throw new Error("too large");
          hash.update("file"); hash.update(createHash("sha256").update(await fs.readFile(file)).digest());
        } else throw new Error("not a regular file");
      }
    };
    await walk(root);
    return hash.digest("hex");
  } catch { return undefined; }
}

function isUserLoggedOut(providerId: CliProviderId, envelope: Record<string, unknown> | undefined): boolean {
  if (!envelope || envelope.ok === false || envelope.success === false || envelope.error) return false;
  if (providerId === "dingtalk-workspace") return envelope.authenticated === false;
  if (!envelope.identities || typeof envelope.identities !== "object") return false;
  const user = (envelope.identities as Record<string, unknown>).user;
  return !!user && typeof user === "object" && (user as Record<string, unknown>).available === false;
}
