import { spawn, type ChildProcess } from "node:child_process";

type SpawnObserverProcess = (
  executable: string,
  args: string[],
  options: { stdio: ["pipe", "pipe", "pipe"]; windowsHide: boolean },
) => ChildProcess;

type NetworkRouteObserverOptions = {
  platform: NodeJS.Platform;
  windowsHelperPath?: string;
  spawnProcess?: SpawnObserverProcess;
  restartDelayMs?: number;
  onChange: (reason: string) => void;
  onError?: (error: unknown) => void;
};

const macOSProxyWatchCommands = [
  "n.add State:/Network/Global/Proxies",
  "n.add Setup:/Network/Global/Proxies",
  "n.add State:/Network/Service/.*/Proxies pattern",
  "n.add Setup:/Network/Service/.*/Proxies pattern",
  "n.watch",
  "",
].join("\n");

export class NetworkRouteObserverError extends Error {
  constructor(readonly code: string, message: string) {
    super(message);
  }
}

export class NetworkRouteObserver {
  private child?: ChildProcess;
  private restartTimer?: ReturnType<typeof setTimeout>;
  private disposed = false;
  private generation = 0;

  constructor(private readonly options: NetworkRouteObserverOptions) {}

  start(): void {
    if (this.disposed || this.child || this.restartTimer) return;
    if (this.options.platform !== "darwin" && this.options.platform !== "win32") return;

    const executable = this.options.platform === "darwin"
      ? "/usr/sbin/scutil"
      : this.options.windowsHelperPath;
    if (!executable) {
      this.options.onError?.(new NetworkRouteObserverError(
        "NETWORK_PROXY_OBSERVER_UNAVAILABLE",
        "Windows 网络代理观察器不可用",
      ));
      return;
    }

    const generation = ++this.generation;
    try {
      const child = (this.options.spawnProcess ?? spawn)(executable, [], {
        stdio: ["pipe", "pipe", "pipe"],
        windowsHide: true,
      });
      this.child = child;
      let stdout = "";
      child.stdout?.on("data", (chunk: Buffer) => {
        stdout += chunk.toString("utf8");
        const lines = stdout.split(/\r?\n/);
        stdout = lines.pop() ?? "";
        for (const line of lines) {
          if (isProxyChangeLine(this.options.platform, line)) {
            this.options.onChange("native-proxy-change");
          }
        }
      });
      child.stderr?.on("data", () => {
        // scutil and the Windows helper may write platform diagnostics. They
        // are intentionally not forwarded because registry paths and local
        // network details do not belong in product logs.
      });
      child.once("error", (error) => {
        if (generation !== this.generation || this.disposed) return;
        this.options.onError?.(new NetworkRouteObserverError(
          "NETWORK_PROXY_OBSERVER_START_FAILED",
          error instanceof Error ? error.message : "网络代理观察器启动失败",
        ));
      });
      child.once("close", (code, signal) => {
        if (generation !== this.generation) return;
        this.child = undefined;
        if (this.disposed) return;
        this.options.onError?.(new NetworkRouteObserverError(
          "NETWORK_PROXY_OBSERVER_EXITED",
          `网络代理观察器已退出 (code=${code ?? "none"}, signal=${signal ?? "none"})`,
        ));
        this.scheduleRestart();
      });
      if (this.options.platform === "darwin") {
        child.stdin?.write(macOSProxyWatchCommands);
      }
    } catch (error) {
      this.child = undefined;
      this.options.onError?.(new NetworkRouteObserverError(
        "NETWORK_PROXY_OBSERVER_START_FAILED",
        error instanceof Error ? error.message : "网络代理观察器启动失败",
      ));
      this.scheduleRestart();
    }
  }

  dispose(): void {
    this.disposed = true;
    this.generation += 1;
    if (this.restartTimer) clearTimeout(this.restartTimer);
    this.restartTimer = undefined;
    const child = this.child;
    this.child = undefined;
    child?.removeAllListeners();
    child?.stdout?.removeAllListeners();
    child?.stderr?.removeAllListeners();
    if (child && !child.killed) child.kill();
  }

  private scheduleRestart(): void {
    if (this.disposed || this.restartTimer) return;
    this.restartTimer = setTimeout(() => {
      this.restartTimer = undefined;
      this.start();
    }, Math.max(250, this.options.restartDelayMs ?? 5_000));
    this.restartTimer.unref?.();
  }
}

export function isProxyChangeLine(platform: NodeJS.Platform, line: string): boolean {
  if (platform === "darwin") {
    return /changedKey\[\d+\]\s*=\s*(?:State|Setup):\/Network\/(?:Global|Service\/[^/]+)\/Proxies/.test(line);
  }
  if (platform === "win32") {
    return line.trim() === "proxy-change";
  }
  return false;
}
