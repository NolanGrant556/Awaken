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
import { extractSingleExecutable, extractSkillBundle } from "./archive.js";

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
};

type StoredInstallations = { version: 1; records: Partial<Record<CliProviderId, InstallationRecord>> };

type AuthSessionInternal = {
  view: CliAuthSessionView;
  directory: string;
  child?: ChildProcess;
  deviceCode?: string;
  openedVerificationUrl?: string;
};

type CommandResult = { exitCode: number; stdout: string; stderr: string };

type ManagerOptions = {
  homeDir: string;
  platform: NodeJS.Platform;
  architecture: string;
  manifestPath: string;
  fetchAsset: (url: string, signal: AbortSignal) => Promise<Response>;
  openExternal?: (url: string) => Promise<void>;
  publishReadiness: (readiness: CliConnectorReadiness) => Promise<void>;
  onStateChanged: (view: CliConnectorView) => void;
};

const providers: CliProviderId[] = ["dingtalk-workspace", "lark-cli"];
const maxArchiveBytes = 128 * 1024 * 1024;
const commandOutputLimit = 1024 * 1024;

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

  constructor(private readonly options: ManagerOptions) {
    this.root = path.join(options.homeDir, ".awaken", "runtime", "cli");
    this.installationsPath = path.join(this.root, "installations.json");
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
    await fs.mkdir(this.authRoot, { recursive: true, mode: 0o700 });
    await fs.chmod(this.authRoot, 0o700);
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
    return this.list();
  }

  async refreshProvider(providerId: CliProviderId): Promise<CliConnectorView> {
    this.assertProvider(providerId);
    return this.refresh(providerId);
  }

  async install(providerId: CliProviderId): Promise<CliConnectorView> {
    this.assertProvider(providerId);
    if (this.installControllers.has(providerId)) return this.view(providerId);
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
      const targetDirectory = path.join(
        this.root,
        providerId,
        asset.version,
        `${asset.platform}-${asset.architecture}`,
      );
      const executablePath = path.join(targetDirectory, asset.executableName);
      let existingVersion = "";
      if (await isExecutableFile(executablePath)) {
        try {
          existingVersion = await this.probeVersion(providerId, executablePath);
        } catch {
          existingVersion = "";
        }
      }
      if (existingVersion !== asset.version) {
        const staging = path.join(this.root, ".staging", `${providerId}-${randomUUID()}`);
        await fs.mkdir(staging, { recursive: true, mode: 0o700 });
        const archivePath = path.join(staging, `asset.${asset.archiveType === "zip" ? "zip" : "tar.gz"}`);
        try {
          this.setInstallProgress(providerId, "downloading_cli", 5, "正在下载官方 CLI");
          const response = await this.options.fetchAsset(asset.url, controller.signal);
          if (!response.ok) throw new CliConnectorError("CLI_DOWNLOAD_FAILED", `下载失败 (${response.status})`, "retry");
          const declaredLength = Number(response.headers.get("content-length") ?? 0);
          if (declaredLength > maxArchiveBytes) throw new CliConnectorError("CLI_ARCHIVE_TOO_LARGE", "安装包超过安全大小限制");
          const bytes = await readResponseBody(response, maxArchiveBytes, (received, total) => {
            const ratio = total > 0 ? Math.min(received / total, 1) : 0.5;
            this.setInstallProgress(providerId, "downloading_cli", 5 + Math.round(ratio * 38), "正在下载官方 CLI");
          });
          this.setInstallProgress(providerId, "verifying_cli", 46, "正在校验官方 CLI");
          if (sha256(bytes) !== asset.sha256) throw new CliConnectorError("CLI_CHECKSUM_MISMATCH", "安装包校验失败", "retry");
          await fs.writeFile(archivePath, bytes, { mode: 0o600 });
          const payloadDirectory = path.join(staging, "payload");
          await fs.mkdir(payloadDirectory, { recursive: true, mode: 0o700 });
          const extractedPath = path.join(payloadDirectory, asset.executableName);
          await extractSingleExecutable(archivePath, asset.archiveType, asset.archiveExecutablePath, extractedPath);
          await fs.chmod(extractedPath, 0o700);
          const version = await this.probeVersion(providerId, extractedPath);
          if (version !== asset.version) throw new CliConnectorError("CLI_VERSION_MISMATCH", "安装包版本与 manifest 不一致");
          this.setInstallProgress(providerId, "installing_cli", 58, "正在安装官方 CLI");
          await fs.mkdir(path.dirname(targetDirectory), { recursive: true, mode: 0o700 });
          await replaceDirectoryAtomically(payloadDirectory, targetDirectory);
        } finally {
          await fs.rm(staging, { recursive: true, force: true });
        }
      } else {
        this.setInstallProgress(providerId, "installing_cli", 58, "已验证本机 CLI");
      }
      const officialSkillNames = await this.installOfficialSkills(providerId, controller.signal);
      this.setInstallProgress(providerId, "finishing", 97, "正在完成安装");
      await this.activateManagedExecutable(asset, executablePath);
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
        officialSkillNames,
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

  private async configureLark(continueWithAuth = false): Promise<CliConnectorView> {
    const providerId: CliProviderId = "lark-cli";
    const executable = this.requireExecutable(providerId);
    if (this.authSessions.has(providerId)) return this.view(providerId);
    const session = await this.createAuthSession(providerId, "正在打开飞书应用配置页面…");
    const child = spawn(executable, ["config", "init", "--new"], safeSpawnOptions(session.directory));
    this.monitorVerificationUrl(session, child, "请在已打开的飞书页面完成应用配置");
    session.child = child;
    session.view.phase = "waiting_user";
    this.emit(providerId);
    void this.finishBlockingSession(providerId, child, true, continueWithAuth);
    return this.view(providerId);
  }

  async startAuth(providerId: CliProviderId): Promise<CliConnectorView> {
    this.assertProvider(providerId);
    if (this.authSessions.has(providerId)) return this.view(providerId);
    if (providerId === "lark-cli" && this.readiness(providerId).configuration === "required") {
      return this.configureLark(true);
    }
    const executable = this.requireExecutable(providerId);
    const session = await this.createAuthSession(
      providerId,
      providerId === "dingtalk-workspace" ? "请在已打开的浏览器中完成钉钉授权" : "正在打开飞书授权页面…",
    );
    this.setReadiness(providerId, {
      ...this.readiness(providerId),
      authentication: "in_progress",
      overall: "working",
      updatedAt: new Date().toISOString(),
    });
    if (providerId === "dingtalk-workspace") {
      const child = spawn(executable, ["auth", "login", "--format", "json"], safeSpawnOptions(session.directory));
      this.monitorVerificationUrl(session, child, "请在已打开的钉钉页面完成授权");
      session.child = child;
      session.view.phase = "waiting_user";
      this.emit(providerId);
      void this.finishBlockingSession(providerId, child, false);
      return this.view(providerId);
    }
    void this.startLarkSplitAuth(session, executable);
    return this.view(providerId);
  }

  async cancelAuth(providerId: CliProviderId): Promise<CliConnectorView> {
    const session = this.authSessions.get(providerId);
    if (!session) return this.view(providerId);
    session.child?.kill();
    session.view.phase = "cancelled";
    session.view.safeMessage = "授权已取消";
    delete session.deviceCode;
    await this.cleanupSessionFiles(session);
    this.setReadiness(providerId, {
      ...this.readiness(providerId),
      authentication: "required",
      overall: "action_required",
      updatedAt: new Date().toISOString(),
    });
    this.emit(providerId);
    this.authSessions.delete(providerId);
    this.emit(providerId);
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
    const executable = this.requireExecutable(providerId);
    const args = providerId === "dingtalk-workspace"
      ? ["auth", "logout", "--format", "json"]
      : ["auth", "logout", "--json"];
    const result = await runCommand(executable, args, this.authRoot, 60_000);
    if (result.exitCode !== 0) throw new CliConnectorError("CLI_LOGOUT_FAILED", "CLI 退出登录失败", "retry");
    return this.refresh(providerId);
  }

  async uninstallManaged(providerId: CliProviderId): Promise<CliConnectorView> {
    const record = this.installations.records[providerId];
    if (!record || record.ownership !== "awaken-managed") {
      throw new CliConnectorError("CLI_UNINSTALL_NOT_MANAGED", "只能卸载 Awaken 管理的 CLI");
    }
    await this.cancelAuth(providerId);
    const providerRoot = path.join(this.root, providerId);
    const relative = path.relative(providerRoot, record.executablePath);
    if (relative.startsWith("..") || path.isAbsolute(relative)) throw new CliConnectorError("CLI_UNINSTALL_PATH_INVALID", "受管安装路径无效");
    await fs.rm(path.dirname(record.executablePath), { recursive: true, force: true });
    await fs.rm(path.join(this.managedBinPath(), path.basename(record.executablePath)), { force: true });
    delete this.installations.records[providerId];
    await this.saveInstallations();
    return this.refresh(providerId);
  }

  async dispose(): Promise<void> {
    for (const controller of this.installControllers.values()) controller.abort();
    for (const session of this.authSessions.values()) session.child?.kill();
    this.installControllers.clear();
    this.authSessions.clear();
    await fs.rm(this.authRoot, { recursive: true, force: true });
  }

  private async refresh(providerId: CliProviderId): Promise<CliConnectorView> {
    const asset = this.assetFor(providerId);
    if (!asset) {
      return this.setReadiness(providerId, {
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
    // Prefer Awaken-managed or an already recorded path. Never ask the user to
    // pick an executable; multiple candidates fall back to the first verified one
    // or a fresh managed install.
    const executablePath = record?.executablePath && await isExecutableFile(record.executablePath)
      ? record.executablePath
      : candidates.find((candidate) => ownershipForPath(candidate, this.options.homeDir) === "awaken-managed")
        ?? candidates[0];
    if (!executablePath) {
      return this.setReadiness(providerId, {
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
      const ownership = record?.executablePath === executablePath
        ? record.ownership
        : ownershipForPath(executablePath, this.options.homeDir);
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
        ownership,
        installMethod: record?.installMethod ?? installMethodForPath(executablePath),
        account: status.account,
        safeError: status.safeError,
        updatedAt: new Date().toISOString(),
      };
      if (!record) {
        this.installations.records[providerId] = {
          providerId,
          executablePath,
          version,
          platform: this.supportedPlatform(),
          architecture: this.options.architecture,
          manifestRevision: this.manifest.revision,
          assetSha256: "",
          ownership,
          installMethod: installMethodForPath(executablePath),
          activation,
          lastVerifiedAt: readiness.updatedAt,
        };
        await this.saveInstallations();
      }
      return this.setReadiness(providerId, readiness);
    } catch (error) {
      const mapped = mapError(error, "CLI_EXECUTABLE_INVALID");
      return this.setReadiness(providerId, {
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
    const args = providerId === "dingtalk-workspace"
      ? ["auth", "status", "--format", "json"]
      : ["auth", "status", "--json", "--verify"];
    const result = await runCommand(executablePath, args, this.authRoot, 30_000);
    const envelope = parseJSONObject(result.stdout) ?? parseJSONObject(result.stderr);
    if (result.exitCode === 0) {
      const authenticated = providerId === "dingtalk-workspace"
        ? booleanField(envelope, ["authenticated"])
        : booleanField(envelope, ["ok", "verified"]);
      if (authenticated) {
        // auth status is the readiness source of truth. dws doctor is diagnostics
        // only and must not override a verified authorized login.
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
    const profile = await runCommand(
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
      const initiated = await runCommand(executable, ["auth", "login", "--recommend", "--no-wait", "--json"], session.directory, 30_000);
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
      session.view.phase = "confirming";
      session.view.safeMessage = "等待飞书确认授权…";
      const child = spawn(executable, ["auth", "login", "--device-code", deviceCode, "--json"], safeSpawnOptions(session.directory));
      drainChildOutput(child);
      session.child = child;
      this.emit("lark-cli");
      await this.finishBlockingSession("lark-cli", child, false);
    } catch (error) {
      await this.failSession("lark-cli", mapError(error, "CLI_AUTH_FAILED"));
    }
  }

  private async finishBlockingSession(
    providerId: CliProviderId,
    child: ChildProcess,
    configurationOnly: boolean,
    continueWithAuth = false,
  ): Promise<void> {
    const exitCode = await childExit(child);
    const session = this.authSessions.get(providerId);
    if (!session || session.child !== child) return;
    if (exitCode !== 0) {
      await this.failSession(providerId, new CliConnectorError(configurationOnly ? "CLI_CONFIGURE_FAILED" : "CLI_AUTH_FAILED", configurationOnly ? "CLI 配置未完成" : "CLI 授权未完成", "retry"), configurationOnly);
      return;
    }
    delete session.deviceCode;
    if (configurationOnly) {
      this.authSessions.delete(providerId);
      await this.cleanupSessionFiles(session);
      const refreshed = await this.refresh(providerId);
      if (continueWithAuth) {
        if (refreshed.readiness.configuration !== "ready") {
          this.setReadiness(providerId, {
            ...refreshed.readiness,
            configuration: "failed",
            overall: "failed",
            safeError: {
              code: "CLI_CONFIGURE_VERIFY_FAILED",
              message: "飞书应用配置结果验证失败",
              recovery: "retry",
            },
            updatedAt: new Date().toISOString(),
          });
          return;
        }
        await this.startAuth(providerId);
      }
      return;
    }
    const refreshed = await this.refresh(providerId);
    if (refreshed.readiness.authentication !== "authorized") {
      await this.failSession(providerId, new CliConnectorError("CLI_AUTH_VERIFY_FAILED", "授权结果验证失败", "retry"));
      return;
    }
    session.view.phase = "authorized";
    session.view.safeMessage = "授权成功";
    await this.cleanupSessionFiles(session);
    this.emit(providerId);
    this.authSessions.delete(providerId);
    this.emit(providerId);
  }

  private async failSession(providerId: CliProviderId, error: CliConnectorError, configurationOnly = false): Promise<void> {
    const session = this.authSessions.get(providerId);
    if (!session) return;
    session.child?.kill();
    delete session.deviceCode;
    session.view.phase = "failed";
    session.view.safeMessage = error.message;
    await this.cleanupSessionFiles(session);
    this.setReadiness(providerId, {
      ...this.readiness(providerId),
      configuration: configurationOnly ? "failed" : this.readiness(providerId).configuration,
      authentication: configurationOnly ? this.readiness(providerId).authentication : "failed",
      overall: "failed",
      safeError: { code: error.code, message: error.message, recovery: error.recovery },
      updatedAt: new Date().toISOString(),
    });
    this.emit(providerId);
  }

  private async createAuthSession(providerId: CliProviderId, message: string): Promise<AuthSessionInternal> {
    const id = randomUUID();
    const directory = path.join(this.authRoot, providerId, id);
    await fs.mkdir(directory, { recursive: true, mode: 0o700 });
    const session: AuthSessionInternal = {
      directory,
      view: { id, provider: providerId, phase: "starting", safeMessage: message },
    };
    this.authSessions.set(providerId, session);
    this.emit(providerId);
    return session;
  }

  private async cleanupSessionFiles(session: AuthSessionInternal): Promise<void> {
    await fs.rm(session.directory, { recursive: true, force: true });
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
        void this.failSession(session.view.provider, mapError(error, "CLI_AUTH_PAGE_OPEN_FAILED"));
      });
    };
    child.stdout?.on("data", onData);
    child.stderr?.on("data", onData);
  }

  private async presentVerificationUrl(session: AuthSessionInternal, verificationUrl: string): Promise<void> {
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
      ? [path.join(this.options.homeDir, ".local", "bin", executableName), path.join("/opt/homebrew/bin", executableName), path.join("/usr/local/bin", executableName)]
      : [path.join(process.env.LOCALAPPDATA ?? "", executableName), path.join(process.env.APPDATA ?? "", "npm", executableName)];
    for (const candidate of defaults) if (candidate && await isExecutableFile(candidate)) candidates.add(path.resolve(candidate));
    return [...candidates].sort();
  }

  private async probeVersion(providerId: CliProviderId, executablePath: string): Promise<string> {
    const args = providerId === "dingtalk-workspace" ? ["version", "--format", "json"] : ["--version"];
    const result = await runCommand(executablePath, args, os.tmpdir(), 15_000);
    if (result.exitCode !== 0) throw new CliConnectorError("CLI_VERSION_PROBE_FAILED", "CLI 版本探测失败", "select_executable");
    const payload = parseJSONObject(result.stdout);
    const raw = providerId === "dingtalk-workspace"
      ? stringField(payload, ["version"])
      : result.stdout.trim();
    const match = raw.match(/v?(\d+\.\d+\.\d+)/);
    if (!match) throw new CliConnectorError("CLI_VERSION_INVALID", "无法识别 CLI 版本", "select_executable");
    return match[1];
  }

  private async activateManagedExecutable(asset: ManifestAsset, executablePath: string): Promise<void> {
    const stablePath = path.join(this.managedBinPath(), asset.executableName);
    const temporary = `${stablePath}.${randomUUID()}.tmp`;
    await fs.copyFile(executablePath, temporary);
    await fs.chmod(temporary, 0o700);
    if (this.options.platform === "win32") {
      const backup = `${stablePath}.old`;
      await fs.rm(backup, { force: true });
      if (await isExecutableFile(stablePath)) await fs.rename(stablePath, backup);
      try {
        await fs.rename(temporary, stablePath);
        await fs.rm(backup, { force: true });
      } catch (error) {
        if (await isExecutableFile(backup)) await fs.rename(backup, stablePath);
        throw error;
      }
    } else {
      await fs.rename(temporary, stablePath);
    }
  }

  private async installOfficialSkills(
    providerId: CliProviderId,
    signal: AbortSignal,
    reportProgress = true,
  ): Promise<string[]> {
    const bundle = this.skillBundleFor(providerId);
    if (!bundle) throw new CliConnectorError("CLI_SKILL_BUNDLE_MISSING", "官方 Skill 安装资产缺失");
    const staging = path.join(this.root, ".staging", `${providerId}-skills-${randomUUID()}`);
    await fs.mkdir(staging, { recursive: true, mode: 0o700 });
    const archivePath = path.join(staging, `skills.${bundle.archiveType === "zip" ? "zip" : "tar.gz"}`);
    try {
      if (reportProgress) this.setInstallProgress(providerId, "downloading_skills", 62, "正在下载官方 Skills");
      const response = await this.options.fetchAsset(bundle.url, signal);
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
      await replaceSkillDirectoriesAtomically(payload, path.join(this.options.homeDir, ".agents", "skills"), bundle.skillNames);
      return [...bundle.skillNames];
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
      if (!record || record.ownership !== "awaken-managed" || !bundle) continue;
      const skillFiles = bundle.skillNames.map((name) => path.join(this.options.homeDir, ".agents", "skills", name, "SKILL.md"));
      const presence = await Promise.all(skillFiles.map((file) => isRegularFile(file)));
      if (presence.every(Boolean)) continue;
      if (presence.some(Boolean)) continue; // Preserve a partial set owned by another installer or the user.
      try {
        record.officialSkillNames = await this.installOfficialSkills(providerId, new AbortController().signal, false);
        record.officialSkillVersion = bundle.version;
        changed = true;
      } catch {
        // Keep an already usable CLI available while retrying the migration on next launch.
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

  private async loadInstallations(): Promise<StoredInstallations> {
    try {
      const parsed = JSON.parse(await fs.readFile(this.installationsPath, "utf8")) as StoredInstallations;
      return parsed?.version === 1 && parsed.records ? parsed : { version: 1, records: {} };
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ENOENT") return { version: 1, records: {} };
      throw new CliConnectorError("CLI_INSTALLATIONS_INVALID", "CLI 安装记录无法读取");
    }
  }

  private async saveInstallations(): Promise<void> {
    await fs.mkdir(path.dirname(this.installationsPath), { recursive: true, mode: 0o700 });
    const temporary = `${this.installationsPath}.${randomUUID()}.tmp`;
    await fs.writeFile(temporary, `${JSON.stringify(this.installations, null, 2)}\n`, { mode: 0o600 });
    await fs.rename(temporary, this.installationsPath);
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
      percent: Math.max(previous, Math.min(100, Math.round(percent))),
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
}

function safeSpawnOptions(cwd: string) {
  return {
    cwd,
    shell: false,
    windowsHide: true,
    env: {
      ...safeCliEnvironment(),
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
      const host = url.hostname.toLowerCase();
      const allowed = providerId === "dingtalk-workspace"
        ? host === "dingtalk.com" || host.endsWith(".dingtalk.com")
        : ["feishu.cn", "larksuite.com", "larkoffice.com"].some(
            (suffix) => host === suffix || host.endsWith(`.${suffix}`),
          );
      if (allowed) return url.toString();
    } catch {
      // Ignore malformed or non-provider URLs from CLI output.
    }
  }
  return undefined;
}

export function safeCliEnvironment(): NodeJS.ProcessEnv {
  const allowed = [
    "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PATH", "TMPDIR", "TEMP", "TMP",
    "SystemRoot", "ComSpec", "LANG", "LC_ALL", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
    "http_proxy", "https_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
    "XDG_CONFIG_HOME", "XDG_DATA_HOME",
  ];
  const env: NodeJS.ProcessEnv = {};
  for (const key of allowed) if (process.env[key] !== undefined) env[key] = process.env[key];
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

async function replaceDirectoryAtomically(source: string, target: string): Promise<void> {
  const backup = `${target}.${randomUUID()}.old`;
  let backedUp = false;
  try {
    if (await pathExists(target)) {
      await fs.rename(target, backup);
      backedUp = true;
    }
    await fs.rename(source, target);
    if (backedUp) await fs.rm(backup, { recursive: true, force: true });
  } catch (error) {
    await fs.rm(target, { recursive: true, force: true });
    if (backedUp) await fs.rename(backup, target);
    throw error;
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

async function runCommand(executable: string, args: string[], cwd: string, timeoutMs: number): Promise<CommandResult> {
  const child = spawn(executable, args, safeSpawnOptions(cwd));
  let stdout = "";
  let stderr = "";
  child.stdout?.on("data", (chunk: Buffer) => { if (stdout.length < commandOutputLimit) stdout += chunk.toString("utf8"); });
  child.stderr?.on("data", (chunk: Buffer) => { if (stderr.length < commandOutputLimit) stderr += chunk.toString("utf8"); });
  const timeout = setTimeout(() => child.kill(), timeoutMs);
  try {
    return { exitCode: await childExit(child), stdout: stdout.slice(0, commandOutputLimit), stderr: stderr.slice(0, commandOutputLimit) };
  } finally {
    clearTimeout(timeout);
  }
}

function childExit(child: ChildProcess): Promise<number> {
  return new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code) => resolve(code ?? 1));
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

function ownershipForPath(filePath: string, homeDir: string): CliInstallOwnership {
  const relative = path.relative(homeDir, filePath);
  if (relative && !relative.startsWith("..") && !path.isAbsolute(relative)) return "user-managed";
  return "system-managed";
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
