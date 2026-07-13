import { EventEmitter } from "node:events";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PassThrough } from "node:stream";
import type { ChildProcess, SpawnOptions } from "node:child_process";
import { describe, expect, it, vi } from "vitest";
import { RuntimeSupervisor } from "./runtime-supervisor.js";

vi.mock("electron", () => ({
  app: {
    isPackaged: false,
    getPath: vi.fn(() => "/tmp/awaken-user-data"),
  },
}));

const gracefulSignal = process.platform === "win32" ? "SIGINT" : "SIGTERM";

class FakeChild extends EventEmitter {
  stdout = new PassThrough();
  stderr = new PassThrough();
  exitCode: number | null = null;
  signalCode: NodeJS.Signals | null = null;
  pid = 1234;
  killedWith: NodeJS.Signals | null = null;

  kill(signal?: NodeJS.Signals): boolean {
    this.killedWith = signal ?? "SIGTERM";
    setImmediate(() => {
      if (this.exitCode === null && this.signalCode === null) {
        this.exitCode = 0;
        this.emit("exit", 0, this.killedWith);
      }
    });
    return true;
  }
}

function asChild(child: FakeChild): ChildProcess {
  return child as unknown as ChildProcess;
}

function tick(): Promise<void> {
  return new Promise((resolve) => setImmediate(resolve));
}

describe("RuntimeSupervisor", () => {
  it("passes runtime env and reports running after health succeeds", async () => {
    let spawnOptions: SpawnOptions | undefined;
    const previousConfigDir = process.env.AWAKEN_CONFIG_DIR;
    const previousAPIKey = process.env.LLM_API_KEY;
    const previousProcessOnly = process.env.AWAKEN_PROCESS_ONLY_CANARY;
    process.env.AWAKEN_CONFIG_DIR = "/tmp/legacy-config";
    process.env.LLM_API_KEY = "legacy-key";
    process.env.AWAKEN_PROCESS_ONLY_CANARY = "must-not-flow-back";
    const supervisor = new RuntimeSupervisor(
      {
        defaultWorkspace: "/tmp/workspace",
      },
      {
        allocatePort: async () => 19001,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({
          PATH: "/login/bin:/usr/bin",
          HOME: "/Users/login-user",
          AWAKEN_LOGIN_ENV_CANARY: "login-value",
          AWAKEN_CONFIG_DIR: "/tmp/login-legacy-config",
          LLM_API_KEY: "login-legacy-key",
          LLM_BASE_URL: "https://legacy.example.invalid",
          LLM_MODEL_ID: "legacy-model",
          LLM_PROTOCOL: "legacy-protocol",
          LLM_AUTH_SCHEME: "legacy-auth",
        }),
        spawnProcess: (_command: string, _args: string[], options: SpawnOptions) => {
          spawnOptions = options;
          return asChild(new FakeChild());
        },
        waitForHealth: vi.fn(async () => {}),
      },
    );

    let info: Awaited<ReturnType<RuntimeSupervisor["start"]>>;
    try {
      info = await supervisor.start();
    } finally {
      if (previousConfigDir === undefined) {
        delete process.env.AWAKEN_CONFIG_DIR;
      } else {
        process.env.AWAKEN_CONFIG_DIR = previousConfigDir;
      }
      if (previousAPIKey === undefined) {
        delete process.env.LLM_API_KEY;
      } else {
        process.env.LLM_API_KEY = previousAPIKey;
      }
      if (previousProcessOnly === undefined) {
        delete process.env.AWAKEN_PROCESS_ONLY_CANARY;
      } else {
        process.env.AWAKEN_PROCESS_ONLY_CANARY = previousProcessOnly;
      }
    }

    expect(info).toEqual({
      status: "running",
      runtimeBaseUrl: "http://127.0.0.1:19001",
    });
    expect(spawnOptions?.env).toMatchObject({
      AWAKEN_CORE_ADDR: "127.0.0.1:19001",
      AWAKEN_DEFAULT_WORKSPACE: "/tmp/workspace",
      AWAKEN_USER_DATA_DIR: "/tmp/awaken-user-data",
      PATH: "/login/bin:/usr/bin",
      HOME: "/Users/login-user",
      AWAKEN_LOGIN_ENV_CANARY: "login-value",
    });
    expect(spawnOptions?.env).not.toHaveProperty("AWAKEN_CONFIG_DIR");
    expect(spawnOptions?.env).not.toHaveProperty("AWAKEN_RUNTIME_MANIFEST");
    expect(spawnOptions?.env).not.toHaveProperty("LLM_API_KEY");
    expect(spawnOptions?.env).not.toHaveProperty("LLM_BASE_URL");
    expect(spawnOptions?.env).not.toHaveProperty("LLM_MODEL_ID");
    expect(spawnOptions?.env).not.toHaveProperty("LLM_PROTOCOL");
    expect(spawnOptions?.env).not.toHaveProperty("LLM_AUTH_SCHEME");
    expect(spawnOptions?.env).not.toHaveProperty(
      "AWAKEN_PROCESS_ONLY_CANARY",
    );
  });

  it.skipIf(process.platform === "win32")(
    "loads the interactive login environment and preserves only values the shell keeps",
    async () => {
      let spawnOptions: SpawnOptions | undefined;
      const fixtureDir = await mkdtemp(
        join(tmpdir(), "awaken-login-environment-"),
      );
      const shellPath = join(fixtureDir, "login-shell");
      await writeFile(
        shellPath,
        [
          "#!/bin/sh",
          '[ "$1" = "-lic" ] || exit 64',
          'printf "profile stdout must stay out of supervisor logs\\n"',
          'export PATH="/login/from-shell:/usr/bin:/bin"',
          'export HOME="/Users/login-from-shell"',
          'export AWAKEN_LOGIN_ENV_CANARY="from-login-shell"',
          'export AWAKEN_LOGIN_MODE="$1"',
          'export LLM_API_KEY="must-be-scrubbed"',
          "unset AWAKEN_PROFILE_UNSET_CANARY",
          'exec /bin/sh -c "$2"',
          "",
        ].join("\n"),
        { mode: 0o700 },
      );

      const previousShell = process.env.SHELL;
      const previousPreserved =
        process.env.AWAKEN_ELECTRON_EXPORTED_CANARY;
      const previousUnset = process.env.AWAKEN_PROFILE_UNSET_CANARY;
      process.env.SHELL = shellPath;
      process.env.AWAKEN_ELECTRON_EXPORTED_CANARY = "preserved-by-shell";
      process.env.AWAKEN_PROFILE_UNSET_CANARY = "must-not-flow-back";
      const supervisor = new RuntimeSupervisor(
        {},
        {
          allocatePort: async () => 19001,
          loginEnvironmentTimeoutMs: 15_000,
          resolveBinaryPath: () => "/fake/awaken-core",
          spawnProcess: (
            _command: string,
            _args: string[],
            options: SpawnOptions,
          ) => {
            spawnOptions = options;
            return asChild(new FakeChild());
          },
          waitForHealth: vi.fn(async () => {}),
        },
      );

      try {
        await supervisor.start();
      } finally {
        if (previousShell === undefined) {
          delete process.env.SHELL;
        } else {
          process.env.SHELL = previousShell;
        }
        if (previousPreserved === undefined) {
          delete process.env.AWAKEN_ELECTRON_EXPORTED_CANARY;
        } else {
          process.env.AWAKEN_ELECTRON_EXPORTED_CANARY = previousPreserved;
        }
        if (previousUnset === undefined) {
          delete process.env.AWAKEN_PROFILE_UNSET_CANARY;
        } else {
          process.env.AWAKEN_PROFILE_UNSET_CANARY = previousUnset;
        }
        await rm(fixtureDir, { recursive: true, force: true });
      }

      expect(spawnOptions?.env).toMatchObject({
        PATH: "/login/from-shell:/usr/bin:/bin",
        HOME: "/Users/login-from-shell",
        AWAKEN_LOGIN_ENV_CANARY: "from-login-shell",
        AWAKEN_LOGIN_MODE: "-lic",
        AWAKEN_ELECTRON_EXPORTED_CANARY: "preserved-by-shell",
      });
      expect(spawnOptions?.env).not.toHaveProperty(
        "AWAKEN_PROFILE_UNSET_CANARY",
      );
      expect(spawnOptions?.env).not.toHaveProperty("LLM_API_KEY");
      expect(supervisor.getLogs()).toEqual([]);
    },
  );

  it.skipIf(process.platform === "win32")(
    "falls back to non-interactive login mode when interactive PATH is empty and HOME is missing",
    async () => {
      let spawnOptions: SpawnOptions | undefined;
      const fixtureDir = await mkdtemp(
        join(tmpdir(), "awaken-login-fallback-"),
      );
      const shellPath = join(fixtureDir, "login-shell");
      await writeFile(
        shellPath,
        [
          "#!/bin/sh",
          'if [ "$1" = "-lic" ]; then',
          '  export PATH=""',
          "  unset HOME",
          '  exec /bin/sh -c "$2"',
          "fi",
          '[ "$1" = "-lc" ] || exit 64',
          'export PATH="/fallback/login/bin:/usr/bin:/bin"',
          'export HOME="/Users/login-fallback"',
          'export AWAKEN_LOGIN_MODE="$1"',
          'exec /bin/sh -c "$2"',
          "",
        ].join("\n"),
        { mode: 0o700 },
      );

      const previousShell = process.env.SHELL;
      process.env.SHELL = shellPath;
      const supervisor = new RuntimeSupervisor(
        {},
        {
          allocatePort: async () => 19001,
          loginEnvironmentTimeoutMs: 15_000,
          resolveBinaryPath: () => "/fake/awaken-core",
          spawnProcess: (
            _command: string,
            _args: string[],
            options: SpawnOptions,
          ) => {
            spawnOptions = options;
            return asChild(new FakeChild());
          },
          waitForHealth: vi.fn(async () => {}),
        },
      );

      try {
        await supervisor.start();
      } finally {
        if (previousShell === undefined) {
          delete process.env.SHELL;
        } else {
          process.env.SHELL = previousShell;
        }
        await rm(fixtureDir, { recursive: true, force: true });
      }

      expect(spawnOptions?.env).toMatchObject({
        PATH: "/fallback/login/bin:/usr/bin:/bin",
        HOME: "/Users/login-fallback",
        AWAKEN_LOGIN_MODE: "-lc",
      });
      expect(supervisor.getLogs()).toEqual([]);
    },
  );

  it("falls back to the Electron process environment when login env resolution fails", async () => {
    let spawnOptions: SpawnOptions | undefined;
    const previousCanary = process.env.AWAKEN_LOGIN_ENV_FALLBACK_CANARY;
    const previousModel = process.env.LLM_MODEL_ID;
    process.env.AWAKEN_LOGIN_ENV_FALLBACK_CANARY = "process-value";
    process.env.LLM_MODEL_ID = "process-legacy-model";
    const supervisor = new RuntimeSupervisor(
      {},
      {
        allocatePort: async () => 19001,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => {
          throw new Error("login shell unavailable");
        },
        spawnProcess: (_command: string, _args: string[], options: SpawnOptions) => {
          spawnOptions = options;
          return asChild(new FakeChild());
        },
        waitForHealth: vi.fn(async () => {}),
      },
    );

    try {
      await supervisor.start();
    } finally {
      if (previousCanary === undefined) {
        delete process.env.AWAKEN_LOGIN_ENV_FALLBACK_CANARY;
      } else {
        process.env.AWAKEN_LOGIN_ENV_FALLBACK_CANARY = previousCanary;
      }
      if (previousModel === undefined) {
        delete process.env.LLM_MODEL_ID;
      } else {
        process.env.LLM_MODEL_ID = previousModel;
      }
    }

    expect(spawnOptions?.env).toMatchObject({
      AWAKEN_LOGIN_ENV_FALLBACK_CANARY: "process-value",
    });
    expect(spawnOptions?.env).not.toHaveProperty("LLM_MODEL_ID");
    expect(supervisor.getLogs()).toEqual([]);
  });

  it("retries with a new port when startup health fails", async () => {
    const children: FakeChild[] = [];
    const waitForHealth = vi
      .fn<() => Promise<void>>()
      .mockRejectedValueOnce(new Error("listen tcp bind failed"))
      .mockResolvedValueOnce(undefined);
    const allocatePort = vi
      .fn<() => Promise<number>>()
      .mockResolvedValueOnce(19001)
      .mockResolvedValueOnce(19002);
    const supervisor = new RuntimeSupervisor(
      {
        maxStartAttempts: 2,
      },
      {
        allocatePort,
        now: () => 1000,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess: () => {
          const child = new FakeChild();
          children.push(child);
          return asChild(child);
        },
        waitForHealth,
      },
    );

    const info = await supervisor.start();

    expect(info).toEqual({
      status: "running",
      runtimeBaseUrl: "http://127.0.0.1:19002",
    });
    expect(allocatePort).toHaveBeenCalledTimes(2);
    expect(waitForHealth).toHaveBeenCalledTimes(2);
    expect(children[0].killedWith).toBe(gracefulSignal);
    expect(supervisor.getLogs()).toEqual([
      {
        id: 0,
        timestamp: 1000,
        stream: "stderr",
        line: "startup attempt 1/2 failed: listen tcp bind failed",
      },
    ]);
  });

  it("reports crashed when the child cannot spawn", async () => {
    const supervisor = new RuntimeSupervisor(
      {
        maxStartAttempts: 1,
      },
      {
        allocatePort: async () => 19001,
        resolveBinaryPath: () => "/missing/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess: () => {
          const child = new FakeChild();
          setImmediate(() => child.emit("error", new Error("ENOENT")));
          return asChild(child);
        },
        waitForHealth: () => new Promise(() => {}),
      },
    );

    const info = await supervisor.start();

    expect(info.status).toBe("crashed");
    expect(info.exitCode).toBe(-1);
    expect(info.errorMessage).toContain("failed to spawn awaken-core: ENOENT");
  });

  it("clears the supervisor log buffer when restarting", async () => {
    const children: FakeChild[] = [];
    const supervisor = new RuntimeSupervisor(
      {
      },
      {
        allocatePort: vi
          .fn<() => Promise<number>>()
          .mockResolvedValueOnce(19001)
          .mockResolvedValueOnce(19002),
        now: () => 1000,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess: () => {
          const child = new FakeChild();
          children.push(child);
          return asChild(child);
        },
        waitForHealth: vi.fn(async () => {}),
      },
    );

    await supervisor.start();
    children[0].stdout.write("old log\n");
    await tick();
    expect(supervisor.getLogs()).toHaveLength(1);

    const info = await supervisor.restart();

    expect(info).toEqual({
      status: "running",
      runtimeBaseUrl: "http://127.0.0.1:19002",
    });
    expect(children).toHaveLength(2);
    expect(children[0].killedWith).toBe(gracefulSignal);
    expect(supervisor.getLogs()).toEqual([]);
  });

  it("transitions to crashed after a running child exits and can restart", async () => {
    const children: FakeChild[] = [];
    const supervisor = new RuntimeSupervisor(
      {
      },
      {
        allocatePort: vi
          .fn<() => Promise<number>>()
          .mockResolvedValueOnce(19001)
          .mockResolvedValueOnce(19002),
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess: () => {
          const child = new FakeChild();
          children.push(child);
          return asChild(child);
        },
        waitForHealth: vi.fn(async () => {}),
      },
    );

    await supervisor.start();
    children[0].exitCode = 9;
    children[0].emit("exit", 9, null);

    expect(supervisor.getStatus()).toMatchObject({
      status: "crashed",
      exitCode: 9,
    });

    const info = await supervisor.restart();

    expect(info).toEqual({
      status: "running",
      runtimeBaseUrl: "http://127.0.0.1:19002",
    });
  });

  it("stop() during starting cancels the in-flight start and does not spawn again", async () => {
    const spawnProcess = vi.fn(() => {
      const child = new FakeChild();
      return asChild(child);
    });
    // Health never resolves: start() is parked in waitForStartup.
    const pendingHealth = () => new Promise<void>(() => {});
    const supervisor = new RuntimeSupervisor(
      {
        maxStartAttempts: 3,
      },
      {
        allocatePort: async () => 19001,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess,
        waitForHealth: pendingHealth,
      },
    );

    // Kick off start() without awaiting; it will park in waitForStartup.
    void supervisor.start();
    await tick();

    expect(spawnProcess).toHaveBeenCalledTimes(1);
    expect(supervisor.getStatus().status).toBe("starting");

    await supervisor.stop();

    // No retry, no second spawn.
    expect(spawnProcess).toHaveBeenCalledTimes(1);
    expect(supervisor.getStatus().status).toBe("stopped");
    expect(supervisor.getStatus().runtimeBaseUrl).toBeUndefined();
  });

  it("restart() during starting cancels the old start and leaves a single new child", async () => {
    const children: FakeChild[] = [];
    // First start() parks on a never-resolving health check; restart()'s
    // internal start() uses a resolving health check.
    const waitForHealth = vi
      .fn<() => Promise<void>>()
      .mockImplementationOnce(() => new Promise<void>(() => {}))
      .mockResolvedValueOnce(undefined);
    const allocatePort = vi
      .fn<() => Promise<number>>()
      .mockResolvedValueOnce(19001)
      .mockResolvedValueOnce(19002);
    const supervisor = new RuntimeSupervisor(
      {
      },
      {
        allocatePort,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess: () => {
          const child = new FakeChild();
          children.push(child);
          return asChild(child);
        },
        waitForHealth,
      },
    );

    void supervisor.start();
    await tick();

    expect(children).toHaveLength(1);

    const info = await supervisor.restart();

    expect(info).toEqual({
      status: "running",
      runtimeBaseUrl: "http://127.0.0.1:19002",
    });
    // Only the original (cancelled) child and the restart child.
    expect(children).toHaveLength(2);
    expect(children[0].killedWith).toBe(gracefulSignal);
  });

  it("stale waitForHealth resolve does not overwrite stopped after stop()", async () => {
    const health = { resolve: null as (() => void) | null };
    const waitForHealth = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          health.resolve = resolve;
        }),
    );
    const supervisor = new RuntimeSupervisor(
      {
      },
      {
        allocatePort: async () => 19001,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess: () => asChild(new FakeChild()),
        waitForHealth,
      },
    );

    void supervisor.start();
    await tick();
    await supervisor.stop();

    expect(supervisor.getStatus().status).toBe("stopped");

    // Late health resolution from the cancelled start must not flip to running.
    health.resolve?.();
    await tick();

    expect(supervisor.getStatus().status).toBe("stopped");
    expect(supervisor.getStatus().runtimeBaseUrl).toBeUndefined();
  });

  it("stale waitForHealth reject does not trigger a retry after stop()", async () => {
    const health = { reject: null as ((err: Error) => void) | null };
    const waitForHealth = vi.fn(
      () =>
        new Promise<void>((_resolve, reject) => {
          health.reject = reject;
        }),
    );
    const spawnProcess = vi.fn(() => asChild(new FakeChild()));
    const supervisor = new RuntimeSupervisor(
      {
        maxStartAttempts: 3,
      },
      {
        allocatePort: async () => 19001,
        resolveBinaryPath: () => "/fake/awaken-core",
        resolveLoginUserEnvironment: async () => ({}),
        spawnProcess,
        waitForHealth,
      },
    );

    void supervisor.start();
    await tick();
    await supervisor.stop();

    expect(spawnProcess).toHaveBeenCalledTimes(1);

    // Late rejection from the cancelled start must not spawn a retry.
    health.reject?.(new Error("late bind failure"));
    await tick();

    expect(spawnProcess).toHaveBeenCalledTimes(1);
    expect(supervisor.getStatus().status).toBe("stopped");
  });
});
