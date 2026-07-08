import {
  spawn,
  type ChildProcess,
  type SpawnOptions,
} from "node:child_process";
import { createServer } from "node:net";
import { createInterface } from "node:readline";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { app } from "electron";

const __dirname = dirname(fileURLToPath(import.meta.url));

export type SupervisorStatus =
  "stopped" | "starting" | "running" | "crashed" | "stopping";

export interface RuntimeLogEntry {
  id: number;
  timestamp: number;
  stream: "stdout" | "stderr";
  line: string;
}

export interface SupervisorStatusInfo {
  status: SupervisorStatus;
  runtimeBaseUrl?: string;
  exitCode?: number;
  errorMessage?: string;
}

export interface SupervisorOptions {
  defaultWorkspace?: string;
  maxStartAttempts?: number;
}

type StatusChangeCallback = (info: SupervisorStatusInfo) => void;
type LogsCallback = (entry: RuntimeLogEntry) => void;
type SpawnProcess = (
  command: string,
  args: string[],
  options: SpawnOptions,
) => ChildProcess;

export interface RuntimeSupervisorDeps {
  allocatePort?: () => Promise<number>;
  now?: () => number;
  resolveBinaryPath?: () => string;
  spawnProcess?: SpawnProcess;
  waitForHealth?: (baseUrl: string) => Promise<void>;
}

function resolveBinaryPath(): string {
  const platform = process.platform;
  const arch = process.arch;
  const binName = platform === "win32" ? "awaken-core.exe" : "awaken-core";
  const targetDir = `${platform}-${arch}`;

  if (app.isPackaged) {
    return join(process.resourcesPath, "bin", targetDir, binName);
  }
  return join(__dirname, "../../resources/bin", targetDir, binName);
}

function resolveRuntimeManifestPath(): string {
  if (app.isPackaged) {
    return join(process.resourcesPath, "runtime-manifest", "primary-runtime.json");
  }
  return join(__dirname, "../../resources/runtime-manifest/primary-runtime.json");
}

function allocatePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.unref();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      if (addr && typeof addr === "object") {
        const port = addr.port;
        server.close(() => resolve(port));
      } else {
        server.close();
        reject(new Error("failed to allocate port"));
      }
    });
  });
}

function sanitizedRuntimeEnv(): NodeJS.ProcessEnv {
  const env = { ...process.env };
  delete env.AWAKEN_CONFIG_DIR;
  delete env.LLM_API_KEY;
  delete env.LLM_BASE_URL;
  delete env.LLM_MODEL_ID;
  delete env.LLM_PROTOCOL;
  delete env.LLM_AUTH_SCHEME;
  return env;
}

async function waitForHealth(
  baseUrl: string,
  timeoutMs = 10000,
  intervalMs = 200,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(`${baseUrl}/health`);
      if (resp.ok) return;
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
  throw new Error(`runtime did not become healthy within ${timeoutMs}ms`);
}

export class RuntimeSupervisor {
  private options: SupervisorOptions;
  private deps: Required<RuntimeSupervisorDeps>;
  private child: ChildProcess | null = null;
  private runtimeBaseUrl: string | undefined;

  private status: SupervisorStatus = "stopped";
  private statusInfo: SupervisorStatusInfo = { status: "stopped" };
  private statusCallbacks = new Set<StatusChangeCallback>();

  // Monotonic token bumped on every stop()/start(). An in-flight start() loop
  // captures its generation and bails out at every await boundary when the
  // token changes, so stop()/restart() can reliably cancel a pending start and
  // guarantee no new child is spawned after we begin shutting down.
  private startGeneration = 0;

  private logs: RuntimeLogEntry[] = [];
  private readonly maxLogs = 1000;
  private logsCallbacks = new Set<LogsCallback>();
  private logSeq = 0;

  constructor(options: SupervisorOptions, deps: RuntimeSupervisorDeps = {}) {
    this.options = options;
    this.deps = {
      allocatePort: deps.allocatePort ?? allocatePort,
      now: deps.now ?? Date.now,
      resolveBinaryPath: deps.resolveBinaryPath ?? resolveBinaryPath,
      spawnProcess: deps.spawnProcess ?? spawn,
      waitForHealth:
        deps.waitForHealth ?? ((baseUrl) => waitForHealth(baseUrl)),
    };
  }

  async start(): Promise<SupervisorStatusInfo> {
    if (this.status !== "stopped" && this.status !== "crashed") {
      return this.getStatus();
    }

    const myGen = ++this.startGeneration;
    const maxAttempts = Math.max(1, this.options.maxStartAttempts ?? 3);
    let lastError: Error | undefined;

    for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
      // Bail out if stop()/restart() invalidated this start loop.
      if (this.startGeneration !== myGen) {
        break;
      }
      this.setStatus("starting");
      let child: ChildProcess | null = null;

      try {
        const port = await this.deps.allocatePort();
        // stop()/restart() may have run while we were waiting for a port.
        if (this.startGeneration !== myGen) {
          break;
        }
        const binaryPath = this.deps.resolveBinaryPath();
        const baseUrl = `http://127.0.0.1:${port}`;

        child = this.deps.spawnProcess(binaryPath, [], {
          env: {
            ...sanitizedRuntimeEnv(),
            AWAKEN_CORE_ADDR: `127.0.0.1:${port}`,
            AWAKEN_RUNTIME_MANIFEST: resolveRuntimeManifestPath(),
            AWAKEN_USER_DATA_DIR: app.getPath("userData"),
            ...(this.options.defaultWorkspace
              ? { AWAKEN_DEFAULT_WORKSPACE: this.options.defaultWorkspace }
              : {}),
          },
          stdio: ["ignore", "pipe", "pipe"],
        });

        this.child = child;
        this.attachChildHandlers(child);
        await this.waitForStartup(child, baseUrl);
        // A stale waitForStartup resolution must not flip us to running after
        // stop()/restart() took over.
        if (this.startGeneration !== myGen) {
          if (child === this.child) {
            this.child = null;
          }
          await this.terminateChild(child);
          break;
        }
        this.runtimeBaseUrl = baseUrl;
        this.setStatus("running");
        return this.getStatus();
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));

        if (child) {
          if (this.child === child) {
            this.child = null;
          }
          await this.terminateChild(child);
        }

        // Cancelled by stop()/restart(): do not retry, do not overwrite status.
        if (this.startGeneration !== myGen) {
          break;
        }

        if (attempt < maxAttempts) {
          this.appendLog({
            timestamp: this.deps.now(),
            stream: "stderr",
            line: `startup attempt ${attempt}/${maxAttempts} failed: ${lastError.message}`,
          });
          continue;
        }
      }
    }

    // Only the most recent start loop is allowed to report the crashed state.
    if (this.startGeneration === myGen) {
      this.runtimeBaseUrl = undefined;
      this.setStatus("crashed", {
        exitCode: -1,
        errorMessage: `failed to start awaken-core after ${maxAttempts} attempt${maxAttempts === 1 ? "" : "s"}: ${
          lastError?.message ?? "unknown error"
        }`,
      });
    }
    return this.getStatus();
  }

  async stop(): Promise<void> {
    // Invalidate any in-flight start() loop so it stops spawning/retrying and
    // does not overwrite the stopped state we are about to set.
    this.startGeneration++;
    if (
      !this.child ||
      this.child.exitCode !== null ||
      this.child.signalCode !== null
    ) {
      this.child = null;
      this.runtimeBaseUrl = undefined;
      this.setStatus("stopped");
      return;
    }
    this.setStatus("stopping");
    const child = this.child;
    this.child = null;
    this.runtimeBaseUrl = undefined;
    await this.terminateChild(child);
    this.setStatus("stopped");
  }

  async restart(): Promise<SupervisorStatusInfo> {
    await this.stop();
    this.clearLogs();
    return this.start();
  }

  getStatus(): SupervisorStatusInfo {
    return { ...this.statusInfo };
  }

  getLogs(): RuntimeLogEntry[] {
    return [...this.logs];
  }

  onLogs(callback: LogsCallback): () => void {
    this.logsCallbacks.add(callback);
    return () => {
      this.logsCallbacks.delete(callback);
    };
  }

  onStatusChange(callback: StatusChangeCallback): () => void {
    this.statusCallbacks.add(callback);
    return () => {
      this.statusCallbacks.delete(callback);
    };
  }

  private setStatus(
    status: SupervisorStatus,
    extra?: { exitCode?: number; errorMessage?: string },
  ): void {
    this.status = status;
    if (status === "running") {
      this.statusInfo = { status, runtimeBaseUrl: this.runtimeBaseUrl };
    } else {
      this.statusInfo = { status, ...extra };
    }
    for (const cb of this.statusCallbacks) {
      cb(this.statusInfo);
    }
  }

  private appendLog(entry: Omit<RuntimeLogEntry, "id">): void {
    const full: RuntimeLogEntry = { id: this.logSeq++, ...entry };
    this.logs.push(full);
    if (this.logs.length > this.maxLogs) {
      this.logs.shift();
    }
    for (const cb of this.logsCallbacks) {
      cb(full);
    }
  }

  private clearLogs(): void {
    this.logs = [];
  }

  private attachChildHandlers(child: ChildProcess): void {
    if (child.stdout) {
      const rl = createInterface({ input: child.stdout });
      rl.on("line", (line: string) => {
        this.appendLog({ timestamp: this.deps.now(), stream: "stdout", line });
      });
    }

    if (child.stderr) {
      const rl = createInterface({ input: child.stderr });
      rl.on("line", (line: string) => {
        this.appendLog({ timestamp: this.deps.now(), stream: "stderr", line });
      });
    }

    child.on("exit", (code, signal) => {
      if (this.child !== child) {
        return;
      }
      this.child = null;
      this.runtimeBaseUrl = undefined;
      if (this.status === "starting") {
        return;
      }
      this.setStatus("crashed", {
        exitCode: code ?? -1,
        errorMessage: `awaken-core exited with code ${code} signal ${signal}`,
      });
    });

    child.on("error", (err) => {
      if (this.child !== child || this.status === "starting") {
        return;
      }
      this.child = null;
      this.runtimeBaseUrl = undefined;
      this.setStatus("crashed", {
        errorMessage: `failed to spawn awaken-core: ${err.message}`,
      });
    });
  }

  private waitForStartup(child: ChildProcess, baseUrl: string): Promise<void> {
    let cleanup = (): void => {};
    const startupFailure = new Promise<never>((_resolve, reject) => {
      const onError = (err: Error) => {
        reject(new Error(`failed to spawn awaken-core: ${err.message}`));
      };
      const onExit = (code: number | null, signal: NodeJS.Signals | null) => {
        reject(
          new Error(
            `awaken-core exited before healthy with code ${code ?? -1} signal ${signal ?? "none"}`,
          ),
        );
      };

      child.once("error", onError);
      child.once("exit", onExit);
      cleanup = () => {
        child.off("error", onError);
        child.off("exit", onExit);
      };
    });

    return Promise.race([
      this.deps.waitForHealth(baseUrl),
      startupFailure,
    ]).finally(cleanup);
  }

  private terminateChild(child: ChildProcess): Promise<void> {
    if (child.exitCode !== null || child.signalCode !== null) {
      return Promise.resolve();
    }

    return new Promise((resolve) => {
      let settled = false;
      const done = () => {
        if (settled) {
          return;
        }
        settled = true;
        clearTimeout(killTimer);
        clearTimeout(resolveTimer);
        resolve();
      };

      const killTimer = setTimeout(() => {
        if (!this.killChild(child, true)) {
          done();
        }
      }, 3000);
      const resolveTimer = setTimeout(done, 3500);

      child.once("exit", done);
      if (!this.killChild(child, false)) {
        done();
      }
    });
  }

  private killChild(child: ChildProcess, force: boolean): boolean {
    try {
      if (process.platform === "win32" && child.pid) {
        // Signal the immediate process first (keeps tests observable and gives
        // the child a graceful chance before taskkill tears down the tree).
        child.kill(force ? "SIGKILL" : "SIGTERM");
        const args = ["/pid", String(child.pid), "/t"];
        if (force) {
          args.push("/f");
        }
        spawn("taskkill", args, { stdio: "ignore" });
        return true;
      }
      return child.kill(force ? "SIGKILL" : "SIGTERM");
    } catch {
      // ignore
      return false;
    }
  }
}
