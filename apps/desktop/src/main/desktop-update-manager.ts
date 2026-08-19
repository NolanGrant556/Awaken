// DesktopUpdateManager：Electron Main 侧应用更新的唯一 owner。
// 设计权威：docs/01_桌面工作台/桌面生命周期与发布/应用版本与自动更新设计契约.md §3、§5、§9.2。
//
// 关键不变量：
// - autoDownload=false / autoInstallOnAppQuit=false：检查不代替用户点击，普通退出不意外更新。
// - 任一时刻最多一个 update operation；check/download/install 共用本类的单一 guard，
//   guard 在 finally 释放，异常不会把更新器永久卡在 busy。
// - 进度最快每 200ms 广播一帧，percent 单调不减并限制在 0–99；只有 updater 的
//   update-downloaded 事件（SHA-512/平台签名验证通过）才能发出 100 并进入 downloaded。
// - 重启安装前现查 Core 的只读 activity 快照；不可达或仍有 active/cancelling run 时
//   fail closed 为 active_runs，不相信 Renderer 传入的任何布尔值。

import { randomUUID } from "node:crypto";
import { readFileSync, writeFileSync, unlinkSync, mkdirSync } from "node:fs";
import { dirname } from "node:path";
import {
  classifyDesktopUpdateError,
  compareStableSemver,
  disabledDesktopUpdateSnapshot,
  evaluateDesktopUpdateReceipt,
  isDesktopRuntimeActivity,
  isStableSemver,
  resolveDesktopUpdateTarget,
  type DesktopRuntimeActivity,
  type DesktopUpdateErrorCode,
  type DesktopUpdateOperationStage,
  type DesktopUpdateReceipt,
  type DesktopUpdateSnapshot,
  type DesktopUpdateTarget,
} from "../shared/desktop-update.js";
import type { ShutdownMode, ShutdownResult } from "./desktop-shutdown-coordinator.js";

/** electron-updater 事件载荷的最小结构（与 electron-updater 解耦，便于测试替身）。 */
export type DesktopUpdateInfo = {
  version: string;
  releaseName?: string | null;
  releaseDate?: string;
};

export type DesktopUpdateProgress = {
  total?: number;
  transferred?: number;
  percent?: number;
  bytesPerSecond?: number;
};

type UpdaterListener = (...args: never[]) => void;

/** electron-updater autoUpdater 的受支持 API 子集；不读取其私有缓存布局。 */
export type DesktopUpdaterLike = {
  autoDownload: boolean;
  autoInstallOnAppQuit: boolean;
  allowPrerelease: boolean;
  allowDowngrade: boolean;
  on(event: string, listener: UpdaterListener): unknown;
  removeListener(event: string, listener: UpdaterListener): unknown;
  checkForUpdates(): Promise<unknown>;
  downloadUpdate(): Promise<unknown>;
  quitAndInstall(isSilent?: boolean, isForceRunAfter?: boolean): void;
};

export type DesktopUpdateCheckReason =
  | "startup"
  | "interval"
  | "resume"
  | "activate"
  | "retry";

export type DesktopUpdateManagerDeps = {
  updater: DesktopUpdaterLike;
  /** 只有 app.isPackaged 且为正式稳定版身份时才为 true；dev/ad-hoc/测试默认 false。 */
  enabled: boolean;
  currentVersion: string;
  platform: string;
  arch: string;
  /** receipt 位于 Electron userData 下，不含绝对缓存路径。 */
  receiptPath: string;
  /** 现查 Core 只读 activity 快照；返回 null 表示不可达（fail closed）。 */
  fetchActivity: () => Promise<DesktopRuntimeActivity | null>;
  /** 请求 shutdown coordinator 停止各 owner；成功后只能交给 quitAndInstall。 */
  requestShutdown: (mode: ShutdownMode) => Promise<ShutdownResult>;
  now?: () => number;
  logger?: (event: string, fields: Record<string, unknown>) => void;
};

const STARTUP_CHECK_BYPASS_THROTTLE: readonly DesktopUpdateCheckReason[] = [
  "startup",
  "retry",
];
const RESUME_CHECK_THROTTLE_MS = 30 * 60 * 1000;
const INTERVAL_CHECK_THROTTLE_MS = 6 * 60 * 60 * 1000;
const PROGRESS_EMIT_INTERVAL_MS = 200;
const PROGRESS_SPEED_WINDOW_MS = 5_000;

type ActiveOperation = {
  id: string;
  stage: DesktopUpdateOperationStage;
  silentFailureSnapshot?: DesktopUpdateSnapshot;
  failed?: boolean;
};

type SpeedSample = {
  at: number;
  transferred: number;
};

export class DesktopUpdateManager {
  private readonly deps: DesktopUpdateManagerDeps;
  private readonly updater: DesktopUpdaterLike;
  private readonly now: () => number;
  private readonly logger: (event: string, fields: Record<string, unknown>) => void;
  private readonly target: DesktopUpdateTarget | null;
  private snapshot: DesktopUpdateSnapshot;
  private readonly listeners = new Set<(snapshot: DesktopUpdateSnapshot) => void>();
  private readonly boundUpdaterListeners: Array<{ event: string; listener: UpdaterListener }> = [];
  private activeOperation: ActiveOperation | null = null;
  private lastCheckAttemptAt = 0;
  private lastProgressEmitAt = 0;
  private lastProgressLogAt = 0;
  private speedSamples: SpeedSample[] = [];
  private pendingReceiptVersion: string | null = null;
  private initialized = false;

  constructor(deps: DesktopUpdateManagerDeps) {
    this.deps = deps;
    this.updater = deps.updater;
    this.now = deps.now ?? (() => Date.now());
    this.logger =
      deps.logger ??
      ((event, fields) => {
        console.info(event, { component: "desktop-updater", ...fields });
      });
    this.target = resolveDesktopUpdateTarget(deps.platform, deps.arch);
    this.snapshot = deps.enabled
      ? {
          operationId: "boot",
          phase: "idle",
          currentVersion: deps.currentVersion,
          target: this.target ?? undefined,
        }
      : disabledDesktopUpdateSnapshot(deps.currentVersion);
  }

  initialize(): void {
    if (this.initialized) return;
    this.initialized = true;
    if (!this.deps.enabled || !isStableSemver(this.deps.currentVersion)) {
      if (this.snapshot.phase !== "disabled") {
        this.snapshot = disabledDesktopUpdateSnapshot(this.deps.currentVersion);
      }
      this.log("desktop_update_disabled", { reason: "identity" });
      return;
    }
    if (!this.target) {
      this.transition({ phase: "disabled" });
      this.log("desktop_update_disabled", { reason: "unsupported_platform" });
      return;
    }
    this.updater.autoDownload = false;
    this.updater.autoInstallOnAppQuit = false;
    this.updater.allowPrerelease = false;
    this.updater.allowDowngrade = false;
    this.bindUpdaterListener("checking-for-update", () => undefined);
    this.bindUpdaterListener("update-available", (info: DesktopUpdateInfo) =>
      this.handleUpdateAvailable(info),
    );
    this.bindUpdaterListener("update-not-available", () => this.handleUpdateNotAvailable());
    this.bindUpdaterListener("download-progress", (progress: DesktopUpdateProgress) =>
      this.handleDownloadProgress(progress),
    );
    this.bindUpdaterListener("update-downloaded", (info: DesktopUpdateInfo) =>
      this.handleUpdateDownloaded(info),
    );
    this.bindUpdaterListener("error", (error: Error) => this.handleUpdaterError(error));
    this.bindUpdaterListener("before-quit-for-update", () => {
      this.log("desktop_update_before_quit_for_update", {});
    });
    this.reconcileReceipt();
  }

  getSnapshot(): DesktopUpdateSnapshot {
    return { ...this.snapshot };
  }

  subscribe(listener: (snapshot: DesktopUpdateSnapshot) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  dispose(): void {
    for (const { event, listener } of this.boundUpdaterListeners) {
      this.updater.removeListener(event, listener);
    }
    this.boundUpdaterListeners.length = 0;
    this.listeners.clear();
    this.initialized = false;
  }

  async check(reason: DesktopUpdateCheckReason): Promise<DesktopUpdateSnapshot> {
    if (!this.deps.enabled || !this.target) {
      return this.getSnapshot();
    }
    if (this.activeOperation) {
      return this.getSnapshot();
    }
    if (!this.canCheckFromCurrentPhase()) {
      return this.getSnapshot();
    }
    if (!this.isCheckAllowed(reason)) {
      return this.getSnapshot();
    }
    const exposeFailure = reason === "retry" || this.snapshot.phase === "available";
    const operation = this.beginOperation(
      "check",
      exposeFailure ? undefined : this.getSnapshot(),
    );
    this.lastCheckAttemptAt = this.now();
    this.transition({
      phase: "checking",
      errorCode: undefined,
      errorMessage: undefined,
      retryFrom: undefined,
    });
    this.log("desktop_update_check_started", { reason });
    try {
      await this.updater.checkForUpdates();
      // 结果由 update-available / update-not-available / error 事件驱动。
      // 部分实现仅在 promise 中返回结果而不发事件；此时停留在 checking 不合法，
      // 若事件未改变 phase，则按“无新版本”收敛。
      if (this.snapshot.phase === "checking" && this.snapshot.operationId === operation.id) {
        this.transition({ phase: "current", checkedAt: new Date().toISOString() });
      }
    } catch (error) {
      this.failOperation(operation, "check", error);
    } finally {
      this.releaseOperation(operation);
    }
    return this.getSnapshot();
  }

  async download(): Promise<DesktopUpdateSnapshot> {
    if (!this.deps.enabled || !this.target) {
      return this.getSnapshot();
    }
    if (this.activeOperation) {
      return this.getSnapshot();
    }
    const canRetryDownload =
      this.snapshot.phase === "error" &&
      this.snapshot.retryFrom === "download" &&
      Boolean(this.snapshot.availableVersion);
    if (this.snapshot.phase !== "available" && !canRetryDownload) {
      return this.getSnapshot();
    }
    const operation = this.beginOperation("download");
    this.speedSamples = [];
    this.lastProgressEmitAt = 0;
    this.transition({
      phase: "downloading",
      percent: undefined,
      transferredBytes: 0,
      totalBytes: undefined,
      bytesPerSecond: undefined,
      errorCode: undefined,
      errorMessage: undefined,
      retryFrom: undefined,
    });
    this.log("desktop_update_download_started", {
      target_version: this.snapshot.availableVersion,
    });
    try {
      await this.updater.downloadUpdate();
      if (this.snapshot.phase === "downloading" && this.snapshot.operationId === operation.id) {
        this.failOperation(operation, "download", new Error("download completed without verification event"));
      }
    } catch (error) {
      this.failOperation(operation, "download", error);
    } finally {
      this.releaseOperation(operation);
    }
    return this.getSnapshot();
  }

  async restartAndInstall(): Promise<DesktopUpdateSnapshot> {
    if (!this.deps.enabled || !this.target) {
      return this.getSnapshot();
    }
    if (this.activeOperation) {
      return this.getSnapshot();
    }
    if (this.snapshot.phase !== "downloaded") {
      return this.getSnapshot();
    }
    const operation = this.beginOperation("install");
    this.transition({ phase: "installing", errorCode: undefined, errorMessage: undefined });
    try {
      // 现查只读 activity 快照；不可达或非零都 fail closed，不复用 Renderer 状态。
      const activity = await this.deps.fetchActivity();
      if (
        !isDesktopRuntimeActivity(activity) ||
        activity.active_run_count > 0 ||
        activity.cancelling_run_count > 0
      ) {
        this.log("desktop_update_install_blocked", {
          reason: "active_runs",
          active_run_count: activity?.active_run_count ?? null,
          cancelling_run_count: activity?.cancelling_run_count ?? null,
        });
        this.failInstall("active_runs", "仍有正在运行的任务，更新安装已被阻止。");
        return this.getSnapshot();
      }
      const shutdown = await this.deps.requestShutdown("update-install");
      if (!shutdown.ok) {
        this.log("desktop_update_shutdown_failed", { reason: shutdown.reason });
        this.failInstall("shutdown_failed", "停止运行环境失败，请重试。");
        return this.getSnapshot();
      }
      this.log("desktop_update_install_launching", {
        target_version: this.snapshot.availableVersion,
      });
      // 清理成功后只能交给安装器；该路径不得被普通 app.exit() 截断。
      this.updater.quitAndInstall(false, true);
    } catch (error) {
      this.failInstall(
        classifyDesktopUpdateError("install", error),
        "启动更新安装器失败，请重试。",
      );
    } finally {
      this.releaseOperation(operation);
    }
    return this.getSnapshot();
  }

  private handleUpdateAvailable(info: DesktopUpdateInfo): void {
    if (!this.isCurrentOperation("check")) return;
    const version = typeof info?.version === "string" ? info.version : "";
    const order = compareStableSemver(version, this.deps.currentVersion);
    // 禁止降级、同版本覆盖和 prerelease 泄漏；只接受严格高于当前的 stable 版本。
    if (!isStableSemver(version) || order === null || order <= 0) {
      this.log("desktop_update_version_rejected", {
        offered_version: version || null,
      });
      this.clearReceipt();
      this.transitionToCurrent();
      return;
    }
    if (this.pendingReceiptVersion && this.pendingReceiptVersion !== version) {
      this.clearReceipt();
    }
    this.transition({
      phase: "available",
      availableVersion: version,
      releaseName: typeof info.releaseName === "string" ? info.releaseName : undefined,
      releaseDate: typeof info.releaseDate === "string" ? info.releaseDate : undefined,
      percent: undefined,
      transferredBytes: undefined,
      totalBytes: undefined,
      bytesPerSecond: undefined,
      errorCode: undefined,
      errorMessage: undefined,
      retryFrom: undefined,
      checkedAt: new Date().toISOString(),
    });
    this.log("desktop_update_available", { target_version: version });
  }

  private handleUpdateNotAvailable(): void {
    if (!this.isCurrentOperation("check")) return;
    this.clearReceipt();
    this.transitionToCurrent();
  }

  private handleDownloadProgress(progress: DesktopUpdateProgress): void {
    if (!this.isCurrentOperation("download")) return;
    const at = this.now();
    if (at - this.lastProgressEmitAt < PROGRESS_EMIT_INTERVAL_MS) {
      return;
    }
    this.lastProgressEmitAt = at;
    const reportedTransferred =
      typeof progress?.transferred === "number" &&
      Number.isFinite(progress.transferred) &&
      progress.transferred >= 0
        ? progress.transferred
        : undefined;
    const transferred =
      reportedTransferred === undefined
        ? this.snapshot.transferredBytes
        : Math.max(this.snapshot.transferredBytes ?? 0, reportedTransferred);
    const total =
      typeof progress?.total === "number" && Number.isFinite(progress.total) && progress.total > 0
        ? progress.total
        : undefined;
    // 未知 totalBytes 时不伪造百分比；已知时单调不减并限制在 0–99，
    // 终帧 100 只能由 update-downloaded 事件发出。
    let percent = this.snapshot.percent;
    if (
      total !== undefined &&
      typeof progress?.percent === "number" &&
      Number.isFinite(progress.percent)
    ) {
      const next = Math.min(99, Math.max(0, Math.floor(progress.percent)));
      percent = percent === undefined ? next : Math.max(percent, next);
    }
    const bytesPerSecond = this.rollingBytesPerSecond(at, transferred);
    this.transition({
      percent,
      transferredBytes: transferred,
      totalBytes: total ?? this.snapshot.totalBytes,
      bytesPerSecond,
    });
    if (at - this.lastProgressLogAt >= 5_000) {
      this.lastProgressLogAt = at;
      this.log("desktop_update_download_progress", {
        percent: percent ?? null,
        bytes_per_second: bytesPerSecond ?? null,
      });
    }
  }

  private handleUpdateDownloaded(info: DesktopUpdateInfo): void {
    if (!this.isCurrentOperation("download")) return;
    const version =
      typeof info?.version === "string" && info.version
        ? info.version
        : this.snapshot.availableVersion;
    if (
      !version ||
      !isStableSemver(version) ||
      version !== this.snapshot.availableVersion ||
      (compareStableSemver(version, this.deps.currentVersion) ?? 0) <= 0
    ) {
      this.log("desktop_update_downloaded_version_rejected", {
        offered_version: version ?? null,
        expected_version: this.snapshot.availableVersion ?? null,
      });
      this.transition({
        phase: "error",
        errorCode: "metadata_invalid",
        errorMessage: undefined,
        retryFrom: "download",
      });
      return;
    }
    this.transition({
      phase: "downloaded",
      availableVersion: version,
      percent: 100,
    });
    // 版本来自 update info 事件，不从文件名反推。
    if (version && this.target) {
      this.writeReceipt({
        schemaVersion: 1,
        version,
        target: this.target,
        releaseTag: `v${version}`,
        downloadedAt: new Date().toISOString(),
      });
    }
    this.log("desktop_update_downloaded", { target_version: version ?? null });
  }

  private handleUpdaterError(error: Error): void {
    const operation = this.activeOperation;
    if (!operation) {
      this.log("desktop_update_background_error", {
        error_code: classifyDesktopUpdateError("check", error),
      });
      return;
    }
    this.failOperation(operation, operation.stage, error);
  }

  private failOperation(
    operation: ActiveOperation,
    stage: DesktopUpdateOperationStage,
    error: unknown,
  ): void {
    if (this.activeOperation?.id !== operation.id) return;
    if (operation.failed) return;
    operation.failed = true;
    if (stage === "check" && operation.silentFailureSnapshot) {
      const errorCode = classifyDesktopUpdateError(stage, error);
      this.log("desktop_update_background_check_failed", { error_code: errorCode });
      this.replaceSnapshot({
        ...operation.silentFailureSnapshot,
        operationId: operation.id,
        checkedAt: new Date().toISOString(),
      });
      return;
    }
    // updater 的 error 事件与 promise 拒绝可能双触发，同一 operation 只归类一次。
    const errorCode = classifyDesktopUpdateError(stage, error);
    this.log("desktop_update_operation_failed", {
      stage,
      error_code: errorCode,
    });
    this.transition({
      phase: "error",
      errorCode,
      errorMessage: undefined,
      retryFrom: stage === "install" ? undefined : stage,
    });
  }

  /** 安装失败（active runs / shutdown）恢复 downloaded 并告知用户重试，不丢弃已校验缓存。 */
  private failInstall(errorCode: DesktopUpdateErrorCode, message: string): void {
    this.transition({
      phase: "downloaded",
      percent: 100,
      errorCode,
      errorMessage: message,
    });
  }

  private isCheckAllowed(reason: DesktopUpdateCheckReason): boolean {
    if (STARTUP_CHECK_BYPASS_THROTTLE.includes(reason)) {
      return true;
    }
    const elapsed = this.now() - this.lastCheckAttemptAt;
    const throttle =
      reason === "interval" ? INTERVAL_CHECK_THROTTLE_MS : RESUME_CHECK_THROTTLE_MS;
    return this.lastCheckAttemptAt === 0 || elapsed >= throttle;
  }

  private canCheckFromCurrentPhase(): boolean {
    if (
      this.snapshot.phase === "downloaded" ||
      this.snapshot.phase === "downloading" ||
      this.snapshot.phase === "installing" ||
      this.snapshot.phase === "disabled"
    ) {
      return false;
    }
    return !(this.snapshot.phase === "error" && this.snapshot.retryFrom === "download");
  }

  private beginOperation(
    stage: DesktopUpdateOperationStage,
    silentFailureSnapshot?: DesktopUpdateSnapshot,
  ): ActiveOperation {
    const operation: ActiveOperation = { id: randomUUID(), stage, silentFailureSnapshot };
    this.activeOperation = operation;
    this.transition({ operationId: operation.id });
    return operation;
  }

  private releaseOperation(operation: ActiveOperation): void {
    if (this.activeOperation?.id === operation.id) {
      this.activeOperation = null;
    }
  }

  private isCurrentOperation(stage: DesktopUpdateOperationStage): boolean {
    return this.activeOperation?.stage === stage;
  }

  private rollingBytesPerSecond(at: number, transferred?: number): number | undefined {
    if (transferred === undefined) {
      return this.snapshot.bytesPerSecond;
    }
    this.speedSamples.push({ at, transferred });
    const cutoff = at - PROGRESS_SPEED_WINDOW_MS;
    while (this.speedSamples.length > 1 && this.speedSamples[0].at < cutoff) {
      this.speedSamples.shift();
    }
    const oldest = this.speedSamples[0];
    const spanMs = at - oldest.at;
    if (spanMs <= 0) {
      return this.snapshot.bytesPerSecond;
    }
    return Math.max(0, ((transferred - oldest.transferred) / spanMs) * 1000);
  }

  /** 启动对账：receipt 只描述"上次见过哪个版本"，不恢复 downloaded 状态。 */
  private reconcileReceipt(): void {
    if (!this.target) return;
    let raw: string;
    try {
      raw = readFileSync(this.deps.receiptPath, "utf8");
    } catch {
      return;
    }
    const verdict = evaluateDesktopUpdateReceipt(raw, this.deps.currentVersion, this.target);
    if (verdict === "discard") {
      try {
        unlinkSync(this.deps.receiptPath);
      } catch {
        // receipt 清理失败不阻塞启动。
      }
      return;
    }
    // 当前锁定的 electron-updater 不提供可测的"缓存再验证"公开 API，
    // 按设计契约 §5.1 降级：跨 App 重启不保留 downloaded 状态，
    // receipt 仅作为日志线索，随后由正常 check 重新发现并下载。
    try {
      const receipt = JSON.parse(raw) as DesktopUpdateReceipt;
      this.pendingReceiptVersion = receipt.version;
      this.log("desktop_update_receipt_noticed", {
        target_version: receipt.version,
        release_tag: receipt.releaseTag,
      });
    } catch {
      // evaluateDesktopUpdateReceipt 已通过解析，这里只是防御。
    }
  }

  private writeReceipt(receipt: DesktopUpdateReceipt): void {
    try {
      mkdirSync(dirname(this.deps.receiptPath), { recursive: true });
      writeFileSync(this.deps.receiptPath, JSON.stringify(receipt, null, 2), "utf8");
      this.pendingReceiptVersion = receipt.version;
    } catch (error) {
      this.log("desktop_update_receipt_write_failed", {
        error_name: error instanceof Error ? error.name : "unknown",
      });
    }
  }

  private clearReceipt(): void {
    this.pendingReceiptVersion = null;
    try {
      unlinkSync(this.deps.receiptPath);
    } catch {
      // 缺失或无法删除的提示 receipt 都不能阻塞检查；它从不作为安装凭证。
    }
  }

  private transitionToCurrent(): void {
    this.transition({
      phase: "current",
      availableVersion: undefined,
      releaseName: undefined,
      releaseDate: undefined,
      percent: undefined,
      transferredBytes: undefined,
      totalBytes: undefined,
      bytesPerSecond: undefined,
      errorCode: undefined,
      errorMessage: undefined,
      retryFrom: undefined,
      checkedAt: new Date().toISOString(),
    });
  }

  private transition(patch: Partial<DesktopUpdateSnapshot>): void {
    this.replaceSnapshot({ ...this.snapshot, ...patch });
  }

  private replaceSnapshot(snapshot: DesktopUpdateSnapshot): void {
    this.snapshot = Object.freeze(snapshot);
    const copy = this.getSnapshot();
    for (const listener of this.listeners) {
      listener(copy);
    }
  }

  private log(event: string, fields: Record<string, unknown>): void {
    this.logger(event, {
      operation_id: this.snapshot.operationId,
      phase: this.snapshot.phase,
      current_version: this.deps.currentVersion,
      target: this.target ?? "unsupported",
      ...fields,
    });
  }

  private bindUpdaterListener(event: string, listener: UpdaterListener): void {
    this.updater.on(event, listener);
    this.boundUpdaterListeners.push({ event, listener });
  }
}

export type { ShutdownMode, ShutdownResult };
