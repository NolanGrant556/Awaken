import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { EventEmitter, once } from "node:events";
import { readFileSync } from "node:fs";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
  assertLoopbackHttpOrigin,
  cleanupFixture,
  clickStateButton,
  MERMAID_ELECTRON_FIXTURE,
  parseArgs,
  parseTranslate,
  terminateChild,
  writeRegressionResult,
} from "../mermaid-electron-regression.mjs";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

describe("Mermaid Electron regression contract", () => {
  it("uses a maintained Gallery fixture instead of a personal thread title", () => {
    const statesSource = readFileSync(
      join(
        repoRoot,
        "Awaken Design System",
        "src",
        "components",
        "AwakenAssistantMessage",
        "AwakenAssistantMessage.states.ts",
      ),
      "utf8",
    );
    const regressionSource = readFileSync(
      join(repoRoot, "scripts", "mermaid-electron-regression.mjs"),
      "utf8",
    );

    assert.match(MERMAID_ELECTRON_FIXTURE.route, /^\/#\/components\//);
    assert.match(statesSource, new RegExp(MERMAID_ELECTRON_FIXTURE.wideStateLabel));
    assert.match(statesSource, new RegExp(MERMAID_ELECTRON_FIXTURE.largeStateLabel));
    assert.doesNotMatch(regressionSource, /项目工具关系图/);
  });

  it("exposes narrow root scripts for the contract and real Electron run", () => {
    const rootPackage = JSON.parse(readFileSync(join(repoRoot, "package.json"), "utf8"));
    assert.equal(
      rootPackage.scripts["test:mermaid-electron:contract"],
      "node --test scripts/tests/mermaid-electron-regression.test.mjs",
    );
    assert.equal(
      rootPackage.scripts["test:mermaid-electron"],
      "node scripts/mermaid-electron-regression.mjs",
    );
  });

  it("rejects non-loopback or non-http CDP origins", () => {
    assert.equal(assertLoopbackHttpOrigin("http://127.0.0.1:9222").port, "9222");
    assert.throws(() => assertLoopbackHttpOrigin("http://localhost:9222"), /loopback HTTP/);
    assert.throws(() => assertLoopbackHttpOrigin("https://127.0.0.1:9222"), /loopback HTTP/);
    assert.throws(() => assertLoopbackHttpOrigin("http://0.0.0.0:9222"), /loopback HTTP/);
  });

  it("parses exact pan transforms and command arguments", () => {
    assert.deepEqual(parseTranslate("translate(180px, -120px)"), { x: 180, y: -120 });
    assert.deepEqual(parseTranslate("translate(0.5px, -1.25px)"), { x: 0.5, y: -1.25 });
    assert.throws(() => parseTranslate("matrix(1, 0, 0, 1, 0, 0)"), /Unexpected/);
    assert.deepEqual(parseArgs(["--artifacts", "/tmp/evidence", "--timeout-ms", "7000"]), {
      artifactsDir: "/tmp/evidence",
      timeoutMs: 7000,
      help: false,
    });
    assert.throws(() => parseArgs(["--timeout-ms", "100"]), /at least 5000/);
  });

  it("forces the Electron fixture onto isolated userData", () => {
    const fixtureMain = readFileSync(
      join(repoRoot, "scripts", "mermaid-electron-fixture-main.cjs"),
      "utf8",
    );
    assert.match(fixtureMain, /app\.setPath\("userData", userDataDir\)/);
    assert.match(fixtureMain, /hostname !== "127\.0\.0\.1"/);
    assert.doesNotMatch(fixtureMain, /apps\/desktop\/src\/main/);
  });
});

describe("Mermaid CI readiness and cleanup", () => {
  it("waits beyond the old fixed delay and clicks only once the large diagram is ready", async () => {
    const pointerEvents = [];
    let polls = 0;
    const cdp = {
      evaluate: async () => {
        polls += 1;
        assert.equal(pointerEvents.length, 0);
        return polls < 5 ? null : { x: 200, y: 100, width: 32, height: 32 };
      },
      send: async (method, params) => pointerEvents.push({ method, ...params }),
    };
    await clickStateButton(cdp, MERMAID_ELECTRON_FIXTURE.largeStateLabel, "Expand diagram", 2_000);
    assert.equal(polls, 5);
    assert.deepEqual(pointerEvents.map((event) => event.type), ["mousePressed", "mouseReleased"]);
    assert.ok(pointerEvents.every((event) => event.x === 200 && event.y === 100));
  });

  it("times out with the missing button identity instead of clicking null or skipping the check", async () => {
    const cdp = {
      evaluate: async () => null,
      send: async () => assert.fail("must not click an unavailable button"),
    };
    await assert.rejects(
      clickStateButton(cdp, "大型 Mermaid 缩放", "Expand diagram", 20),
      /大型 Mermaid 缩放: Expand diagram did not become ready/,
    );
  });

  it("requests a graceful exit and waits for the isolated child to actually exit", async (t) => {
    const child = spawn(process.execPath, ["-e", `
      process.on('message', message => {
        if (message.type === 'awaken-mermaid-quit') setTimeout(() => process.exit(0), 100);
      });
      process.send('ready');
    `], { stdio: ["ignore", "ignore", "ignore", "ipc"] });
    t.after(() => { if (child.exitCode === null) child.kill(); });
    await once(child, "message");
    await terminateChild({ child });
    assert.equal(child.exitCode, 0);
  });

  it("waits for the Windows child exit after taskkill and rejects failed termination", async () => {
    const child = Object.assign(new EventEmitter(), { pid: 42, exitCode: null, signalCode: null });
    await terminateChild({ child }, {
      platform: "win32",
      killWindowsTree: (command, args) => {
        assert.equal(command, "taskkill");
        assert.deepEqual(args, ["/PID", "42", "/T", "/F"]);
        setTimeout(() => { child.exitCode = 1; child.emit("exit", 1); }, 30);
        return { status: 0 };
      },
    });
    assert.equal(child.exitCode, 1);
    child.exitCode = null;
    await assert.rejects(terminateChild({ child }, {
      platform: "win32", killWindowsTree: () => ({ status: 1 }),
    }), /taskkill exited 1/);
    await assert.rejects(terminateChild({ child }, {
      platform: "win32", killWindowsTree: () => ({ status: 0 }), exitTimeoutMs: 20,
    }), /did not exit after taskkill/);
  });

  it("continues cleanup after errors and saves the original interaction failure and cleanup failure", async (t) => {
    const dir = await mkdtemp(join(tmpdir(), "awaken-mermaid-report-test-"));
    t.after(() => rm(dir, { recursive: true, force: true }));
    const stages = [];
    const cleanup = await cleanupFixture({
      cdp: { close: () => stages.push("cdp") }, electron: "electron", vite: "vite", userDataDir: dir,
    }, {
      terminate: async (child) => {
        stages.push(child);
        if (child === "electron") throw new Error("stop failed");
      },
      remove: async (_path, options) => {
        stages.push("remove");
        assert.equal(options.maxRetries, 10);
        throw Object.assign(new Error("DIPS locked"), { code: "EBUSY" });
      },
    });
    assert.deepEqual(stages, ["cdp", "electron", "vite", "remove"]);
    const resultPath = join(dir, "result.json");
    await writeRegressionResult(resultPath, { isolation: {}, interaction: null }, new Error("expand failed"), cleanup);
    const report = JSON.parse(await readFile(resultPath, "utf8"));
    assert.equal(report.status, "failed");
    assert.equal(report.failure.message, "expand failed");
    assert.equal(report.isolation.temporaryUserDataRemoved, false);
    assert.deepEqual(report.cleanup.errors.map((error) => error.stage), ["stop-electron", "remove-user-data"]);
    assert.equal(report.cleanup.errors[1].code, "EBUSY");

    const cleanupOnly = await writeRegressionResult(resultPath, { isolation: {}, interaction: { checks: ["passed"] } }, null, cleanup);
    assert.equal(cleanupOnly.status, "failed");
    assert.equal(cleanupOnly.failure, null);
    assert.deepEqual(cleanupOnly.interaction.checks, ["passed"]);
  });

  it("removes an isolated profile and truthfully records successful cleanup", async (t) => {
    const dir = await mkdtemp(join(tmpdir(), "awaken-mermaid-cleanup-test-"));
    t.after(() => rm(dir, { recursive: true, force: true }));
    await writeFile(join(dir, "DIPS"), "fixture");
    const cleanup = await cleanupFixture({ userDataDir: dir });
    assert.deepEqual(cleanup, { status: "passed", temporaryUserDataRemoved: true, errors: [] });
    await assert.rejects(readFile(join(dir, "DIPS")), { code: "ENOENT" });
  });

  it("retries a real Windows file lock until the owner releases it", { skip: process.platform !== "win32", timeout: 15_000 }, async (t) => {
    const dir = await mkdtemp(join(tmpdir(), "awaken-mermaid-lock-test-"));
    const path = join(dir, "DIPS");
    await writeFile(path, "fixture");
    const child = spawn("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", `
      $stream = [System.IO.File]::Open($env:AWAKEN_LOCK_TEST_FILE, 'Open', 'ReadWrite', 'None');
      [Console]::Out.WriteLine('locked');
      [Console]::In.ReadLine() | Out-Null;
      $stream.Dispose();
    `], { env: { ...process.env, AWAKEN_LOCK_TEST_FILE: path }, stdio: ["pipe", "pipe", "pipe"] });
    t.after(async () => {
      if (child.exitCode === null) child.kill();
      await rm(dir, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    });
    const [ready] = await Promise.race([
      once(child.stdout, "data"),
      once(child, "exit").then(([code]) => { throw new Error(`Lock fixture exited early: ${code}`); }),
    ]);
    assert.match(String(ready), /locked/);
    await assert.rejects(rm(path), (error) => ["EBUSY", "EPERM"].includes(error.code));
    const release = setTimeout(() => child.stdin.end("release\n"), 250);
    t.after(() => clearTimeout(release));
    const cleanup = await cleanupFixture({ userDataDir: dir });
    assert.equal(cleanup.status, "passed");
    assert.equal(cleanup.temporaryUserDataRemoved, true);
  });
});
