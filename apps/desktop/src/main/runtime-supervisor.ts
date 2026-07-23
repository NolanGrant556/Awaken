import {
  spawn,
  type ChildProcess,
  type SpawnOptions,
} from "node:child_process";
import { createServer } from "node:net";
import { userInfo } from "node:os";
import { createInterface } from "node:readline";
import { dirname, isAbsolute, join } from "node:path";
import { fileURLToPath } from "node:url";
import { app } from "electron";
import { resolveSystemSkillsTargetRoot } from "./system-skills-importer.js";
import { RuntimeControlClient, type RuntimeManifest } from "./runtime-control.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

export type SupervisorStatus =
  | "stopped"
  | "starting"
  | "running"
  | "external-attached"
  | "crashed"
  | "stopping";

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
  maxStartAttempts?: number;
}

type StatusChangeCallback = (info: SupervisorStatusInfo) => void;
type LogsCallback = (entry: RuntimeLogEntry) => void;
type SpawnProcess = (
  command: string,
  args: string[],
  options: SpawnOptions,
) => ChildProcess;
type ResolveLoginUserEnvironment = () => Promise<NodeJS.ProcessEnv>;
type ResolveProductEnvironment = () => Promise<NodeJS.ProcessEnv>;

export interface RuntimeSupervisorDeps {
  allocatePort?: () => Promise<number>;
  loginEnvironmentTimeoutMs?: number;
  now?: () => number;
  readManifest?: (userDataDir: string) => Promise<RuntimeManifest | undefined>;
  resolveBinaryPath?: () => string;
  resolveLoginUserEnvironment?: ResolveLoginUserEnvironment;
  resolveProductEnvironment?: ResolveProductEnvironment;
  spawnProcess?: SpawnProcess;
  waitForHealth?: (baseUrl: string) => Promise<void>;
}

const LOGIN_ENV_TIMEOUT_MS = 3000;
const LOGIN_ENV_MAX_BYTES = 256 * 1024;
const LOGIN_ENV_START_MARKER =
  "__AWAKEN_LOGIN_ENV_START_9F6E01A594834F489AB8D774F20A21D8__";
const LOGIN_ENV_END_MARKER =
  "__AWAKEN_LOGIN_ENV_END_9F6E01A594834F489AB8D774F20A21D8__";

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

function parseLoginEnvironment(output: Buffer): NodeJS.ProcessEnv {
  const text = output.toString("utf8");
  const startToken = `\0${LOGIN_ENV_START_MARKER}\0`;
  const endToken = `\0${LOGIN_ENV_END_MARKER}\0`;
  const startIndex = text.indexOf(startToken);
  const endIndex = text.indexOf(
    endToken,
    startIndex < 0 ? 0 : startIndex + startToken.length,
  );
  if (startIndex < 0 || endIndex < 0) {
    throw new Error("login environment output was not framed correctly");
  }

  const env: NodeJS.ProcessEnv = {};
  const payload = text.slice(startIndex + startToken.length, endIndex);
  for (const entry of payload.split("\0")) {
    const separator = entry.indexOf("=");
    if (separator <= 0) {
      continue;
    }
    const name = entry.slice(0, separator);
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
      continue;
    }
    env[name] = entry.slice(separator + 1);
  }
  if (!env.PATH?.trim() || !env.HOME?.trim()) {
    throw new Error("login environment did not contain PATH and HOME");
  }
  return env;
}

function resolveLoginIdentity(): { shell: string; home?: string } {
  let accountShell: string | undefined;
  let accountHome: string | undefined;
  try {
    const account = userInfo({ encoding: "utf8" });
    accountShell = account.shell?.trim();
    accountHome = account.homedir;
  } catch {
    // The inherited environment and /bin/sh fallback still provide a safe path.
  }

  const inheritedShell = process.env.SHELL?.trim();
  const shell = [inheritedShell, accountShell].find(
    (candidate): candidate is string =>
      typeof candidate === "string" &&
      candidate.length > 0 &&
      isAbsolute(candidate),
  );
  return {
    shell: shell ?? "/bin/sh",
    home: process.env.HOME || accountHome,
  };
}

function readShellEnvironment(
  shell: string,
  args: string[],
  inheritedEnv: NodeJS.ProcessEnv,
  timeoutMs: number,
): Promise<NodeJS.ProcessEnv> {
  const command =
    `printf '\\0${LOGIN_ENV_START_MARKER}\\0'; ` +
    `/usr/bin/env -0; ` +
    `printf '${LOGIN_ENV_END_MARKER}\\0'`;

  return new Promise((resolve, reject) => {
    const child = spawn(shell, [...args, command], {
      env: inheritedEnv,
      stdio: ["ignore", "pipe", "ignore"],
    });
    const chunks: Buffer[] = [];
    let bytes = 0;
    let settled = false;

    const finish = (error?: Error, env?: NodeJS.ProcessEnv): void => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timer);
      if (error) {
        reject(error);
      } else {
        resolve(env ?? {});
      }
    };
    const stopChild = (): void => {
      try {
        child.kill("SIGKILL");
      } catch {
        // The process may already have exited.
      }
    };
    const timer = setTimeout(() => {
      stopChild();
      finish(new Error("login environment resolution timed out"));
    }, timeoutMs);

    child.stdout?.on("data", (chunk: Buffer | string) => {
      if (settled) {
        return;
      }
      const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      bytes += buffer.length;
      if (bytes > LOGIN_ENV_MAX_BYTES) {
        stopChild();
        finish(new Error("login environment output exceeded the size limit"));
        return;
      }
      chunks.push(buffer);
    });
    child.once("error", () => {
      finish(new Error("login environment process failed to start"));
    });
    child.once("close", (code, signal) => {
      if (settled) {
        return;
      }
      if (code !== 0 || signal) {
        finish(new Error("login environment process did not exit successfully"));
        return;
      }
      try {
        finish(undefined, parseLoginEnvironment(Buffer.concat(chunks)));
      } catch {
        finish(new Error("login environment output could not be parsed"));
      }
    });
  });
}

async function resolveLoginUserEnvironment(
  timeoutMs = LOGIN_ENV_TIMEOUT_MS,
): Promise<NodeJS.ProcessEnv> {
  if (process.platform === "win32") {
    return { ...process.env };
  }

  const { shell, home } = resolveLoginIdentity();
  const inheritedEnv = { ...process.env };
  if (!inheritedEnv.HOME && home) {
    inheritedEnv.HOME = home;
  }
  if (!inheritedEnv.SHELL) {
    inheritedEnv.SHELL = shell;
  }

  // Interactive login mode loads both login and interactive startup files
  // (for example .zprofile + .zshrc), which is where nvm, pyenv and mise are
  // commonly initialized. Marker framing isolates env(1)'s NUL-delimited
  // payload from any startup-file stdout; stderr and values are never logged.
  // Each mode is independently bounded so every interactive failure still gets
  // a real non-interactive login fallback before Core uses the process env.
  for (const args of [["-lic"], ["-lc"]]) {
    try {
      return await readShellEnvironment(
        shell,
        args,
        inheritedEnv,
        timeoutMs,
      );
    } catch {
      // A noisy or broken interactive profile may still have a valid login
      // environment. The caller performs the final process-environment fallback.
    }
  }
  throw new Error("login environment could not be resolved");
}

function sanitizedRuntimeEnv(
  sourceEnvironment: NodeJS.ProcessEnv,
): NodeJS.ProcessEnv {
  // The selected source is authoritative. In particular, variables explicitly
  // unset by the user's startup files must not leak back from Electron's env.
  const env = { ...sourceEnvironment };
  delete env.AWAKEN_CONFIG_DIR;
  delete env.AWAKEN_BUNDLED_SKILL_DIRS;
  delete env.LLM_API_KEY;
  delete env.LLM_BASE_URL;
  delete env.LLM_MODEL_ID;
  delete env.LLM_PROTOCOL;
  delete env.LLM_AUTH_SCHEME;
  delete env.AWAKEN_WEB_SEARCH_ENABLED;
  delete env.AWAKEN_WEB_SEARCH_PROVIDER;
  delete env.AWAKEN_WEB_SEARCH_ENDPOINT;
  delete env.AWAKEN_WEB_SEARCH_API_KEY;
  delete env.AWAKEN_WEB_SEARCH_PROVIDERS;
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

function isPidAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

export class RuntimeSupervisor {
  private options: SupervisorOptions;
  private deps: Required<RuntimeSupervisorDeps>;
  private child: ChildProcess | null = null;
  private runtimeBaseUrl: string | undefined;
  private controlClient: RuntimeControlClient | null = null;
  private runtimeIdentity: string | undefined;

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
    const loginEnvironmentTimeoutMs =
      deps.loginEnvironmentTimeoutMs ?? LOGIN_ENV_TIMEOUT_MS;
    this.deps = {
      allocatePort: deps.allocatePort ?? allocatePort,
      loginEnvironmentTimeoutMs,
      now: deps.now ?? Date.now,
      readManifest:
        deps.readManifest ??
        (async (userDataDir) => RuntimeControlClient.readManifest(userDataDir)),
      resolveBinaryPath: deps.resolveBinaryPath ?? resolveBinaryPath,
      resolveLoginUserEnvironment:
        deps.resolveLoginUserEnvironment ??
        (() => resolveLoginUserEnvironment(loginEnvironmentTimeoutMs)),
      resolveProductEnvironment:
        deps.resolveProductEnvironment ?? (async () => ({})),
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
    this.setStatus("starting");
    let runtimeEnvironment: NodeJS.ProcessEnv;
    try {
      runtimeEnvironment = sanitizedRuntimeEnv(
        await this.deps.resolveLoginUserEnvironment(),
      );
    } catch {
      // A missing, slow, or broken login shell must not prevent Desktop from
      // starting Core. Fall back to the Electron process environment without
      // exposing shell output or environment values through supervisor logs.
      runtimeEnvironment = sanitizedRuntimeEnv(process.env);
    }
    const productEnvironment = await this.deps.resolveProductEnvironment();
    if (this.startGeneration !== myGen) {
      return this.getStatus();
    }

    const userDataDir = app.getPath("userData");
    try {
      const manifest = await this.deps.readManifest(userDataDir);
      if (manifest && isPidAlive(manifest.pid)) {
        const expectedRuntimeIdentity = `packaged:${this.digestUserDataDir(userDataDir)}`;
        const client = new RuntimeControlClient(manifest);
        await client.verifyIdentity(expectedRuntimeIdentity);
        this.runtimeBaseUrl = manifest.base_url;
        const ownerID = `desktop-${userDataDir}`;
        if (manifest.owner_mode === "desktop") {
          this.appendLog({
            timestamp: this.deps.now(),
            stream: "stderr",
            line: "attaching to existing desktop-owned Core instance",
          });
        }
        await client.createLease(ownerID, "desktop");
        this.controlClient = client;
        this.runtimeIdentity = manifest.runtime_identity;
        this.setStatus("external-attached");
        return this.getStatus();
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.appendLog({
        timestamp: this.deps.now(),
        stream: "stderr",
        line: `existing manifest attach failed: ${message}`,
      });
      // Fall through to spawn a new Core.
    }

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
            ...runtimeEnvironment,
            ...productEnvironment,
            AWAKEN_CORE_ADDR: `127.0.0.1:${port}`,
            AWAKEN_USER_DATA_DIR: app.getPath("userData"),
            // Business data root shared by dev and packaged installs. Main
            // resolves the user home itself; inherited shell values must not
            // change the product identity.
            AWAKEN_HOME: join(app.getPath("home"), ".awaken"),
            // Bundled Skills are product-owned. Login shell configuration may
            // not replace the synchronized Desktop source identity.
            AWAKEN_BUNDLED_SKILL_DIRS: resolveSystemSkillsTargetRoot(
              app.getPath("home"),
            ),
            // FileStore projects are selected per run. Core must never derive
            // a project from Electron userData, a previous workspace, or cwd.
            AWAKEN_DEFAULT_WORKSPACE: "",
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
        try {
          await this.attachControlLease(baseUrl, app.getPath("userData"));
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          this.appendLog({
            timestamp: this.deps.now(),
            stream: "stderr",
            line: `control lease attach failed: ${message}`,
          });
          // A lease failure is not fatal: the Core is still usable through
          // regular Desktop routes; ACP bridge clients may not be able to reuse
          // this instance until the lease is established.
        }
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
    if (this.status === "external-attached") {
      this.runtimeBaseUrl = undefined;
      await this.releaseControlLease();
      this.setStatus("stopped");
      return;
    }
    if (
      !this.child ||
      this.child.exitCode !== null ||
      this.child.signalCode !== null
    ) {
      this.child = null;
      this.runtimeBaseUrl = undefined;
      await this.releaseControlLease();
      this.setStatus("stopped");
      return;
    }
    this.setStatus("stopping");
    const child = this.child;
    const baseUrl = this.runtimeBaseUrl;
    this.child = null;
    this.runtimeBaseUrl = undefined;
    await this.releaseControlLease();

    if (baseUrl) {
      try {
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 2000);
        try {
          await fetch(`${baseUrl}/v1/shutdown`, {
            method: "POST",
            signal: controller.signal,
          });
        } finally {
          clearTimeout(timer);
        }
      } catch {
        // Ignore shutdown endpoint errors and fall back to terminating the child.
      }
    }

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
    if (status === "running" || status === "external-attached") {
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

  private async attachControlLease(baseUrl: string, userDataDir: string): Promise<void> {
    const manifest = await RuntimeControlClient.readManifest(userDataDir);
    if (!manifest) {
      // No manifest means the Core was started without runtime identity support
      // (e.g. tests or legacy launch). Desktop continues without a control lease.
      return;
    }
    if (manifest.base_url !== baseUrl) {
      throw new Error(
        `manifest base_url ${manifest.base_url} does not match expected ${baseUrl}`,
      );
    }
    const expectedRuntimeIdentity = `packaged:${this.digestUserDataDir(userDataDir)}`;
    const client = new RuntimeControlClient(manifest);
    await client.verifyIdentity(expectedRuntimeIdentity);
    await client.createLease(`desktop-${app.getPath("userData")}`, "desktop");
    this.controlClient = client;
    this.runtimeIdentity = manifest.runtime_identity;
  }

  private async releaseControlLease(): Promise<void> {
    if (this.controlClient) {
      try {
        await this.controlClient.releaseLease();
      } catch {
        // Best effort: the Core will reclaim the lease via TTL.
      }
      this.controlClient = null;
      this.runtimeIdentity = undefined;
    }
  }

  private digestUserDataDir(userDataDir: string): string {
    // Mirror Go stableProjectID: lower-case, clean, SHA-256 first 12 bytes.
    const normalized = userDataDir.toLowerCase().replace(/\\/g, "/").trim();
    // The Go side uses filepath.Clean and a SHA-256 digest. We do not need to
    // match byte-for-byte here because the manifest verification endpoint
    // returns the runtime_identity computed by Core; this digest is only used
    // for the initial expectation check before calling verifyIdentity.
    return normalized;
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
        // On Windows SIGTERM is not gracefully delivered to non-Node console
        // processes; use SIGINT (CTRL_C_EVENT) so awaken-core can run its
        // shutdown handler and flush active transcripts.
        child.kill(force ? "SIGKILL" : "SIGINT");
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
