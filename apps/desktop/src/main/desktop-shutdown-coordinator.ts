// DesktopShutdownCoordinator：从 index.ts 的 before-quit 抽取的可等待、幂等退出编排。
// 设计权威：docs/01_桌面工作台/桌面生命周期与发布/应用版本与自动更新设计契约.md §6、§9.2。
//
// - 同一 stop() promise 处理重入，不并发关闭两次。
// - "normal" 成功后由调用方 app.exit()；"update-install" 成功后只能交给
//   quitAndInstall(false, true)，不得被普通 app.exit() 截断。
// - 清理超时或失败时返回 { ok: false }，不强制退出、不丢弃任务或 userData。

export type ShutdownMode = "normal" | "update-install";

export type ShutdownResult = { ok: true } | { ok: false; reason: string };

export type DesktopShutdownCoordinatorDeps = {
  /** 同步 owner：worker 定时器、事件流、subscriptions、预览/媒体、网络观察等。 */
  stopSyncOwners: () => void;
  /** 异步 owner：supervisor/Core、CLI connector、pet/computer overlay。 */
  stopAsyncOwners: () => Promise<void>;
  /** 仅在全部异步 owner 成功停止后执行的不可恢复收尾（如销毁 updater listener）。 */
  finalizeOwners?: () => void;
  timeoutMs?: number;
  logger?: (event: string, fields: Record<string, unknown>) => void;
};

const DEFAULT_SHUTDOWN_TIMEOUT_MS = 15_000;

export class DesktopShutdownCoordinator {
  private readonly deps: DesktopShutdownCoordinatorDeps;
  private readonly timeoutMs: number;
  private readonly logger: (event: string, fields: Record<string, unknown>) => void;
  private inFlight: Promise<ShutdownResult> | null = null;
  private completedMode: ShutdownMode | null = null;

  constructor(deps: DesktopShutdownCoordinatorDeps) {
    this.deps = deps;
    this.timeoutMs = deps.timeoutMs ?? DEFAULT_SHUTDOWN_TIMEOUT_MS;
    this.logger =
      deps.logger ??
      ((event, fields) => {
        console.info(event, { component: "desktop-shutdown", ...fields });
      });
  }

  /** update-install 清理已完成，before-quit 必须放行让安装器接管，不再二次清理。 */
  get updateInstallReady(): boolean {
    return this.completedMode === "update-install";
  }

  get settled(): boolean {
    return this.completedMode !== null;
  }

  stop(mode: ShutdownMode): Promise<ShutdownResult> {
    if (this.completedMode) {
      return Promise.resolve({ ok: true });
    }
    if (this.inFlight) {
      return this.inFlight;
    }
    this.inFlight = this.run(mode);
    return this.inFlight;
  }

  private async run(mode: ShutdownMode): Promise<ShutdownResult> {
    this.logger("desktop_shutdown_started", { mode });
    try {
      this.deps.stopSyncOwners();
      await withTimeout(this.deps.stopAsyncOwners(), this.timeoutMs);
      this.deps.finalizeOwners?.();
      this.completedMode = mode;
      this.logger("desktop_shutdown_completed", { mode });
      return { ok: true };
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error);
      this.logger("desktop_shutdown_failed", { mode, reason });
      return { ok: false, reason };
    } finally {
      this.inFlight = null;
    }
  }
}

function withTimeout(promise: Promise<void>, timeoutMs: number): Promise<void> {
  let timer: NodeJS.Timeout | null = null;
  const timeout = new Promise<never>((_resolve, reject) => {
    timer = setTimeout(() => {
      reject(new Error(`shutdown cleanup timed out after ${timeoutMs}ms`));
    }, timeoutMs);
  });
  return Promise.race([promise, timeout]).finally(() => {
    if (timer) clearTimeout(timer);
  });
}
