import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
  READY_MARKER,
  assessSmokeTranscript,
  isolatedLaunchEnvironment,
  packagedExecutableCandidates,
  resolvePackagedExecutable,
} from "../smoke-packaged-desktop.mjs";

const tempRoots = [];
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

function tempRoot() {
  const root = mkdtempSync(join(tmpdir(), "awaken-packaged-smoke-"));
  tempRoots.push(root);
  return root;
}

afterEach(() => {
  for (const root of tempRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true });
  }
});

describe("packaged desktop smoke", () => {
  it("keeps the ready marker on the packaged main-window log", () => {
    const source = readFileSync(join(repoRoot, "apps/desktop/src/main/index.ts"), "utf8");
    assert.match(source, new RegExp(`console\\.info\\("${READY_MARKER}"\\)`));
  });

  it("selects the host-native packaged executable", () => {
    assert.deepEqual(packagedExecutableCandidates("darwin", "arm64"), [
      "mac-arm64/Awaken.app/Contents/MacOS/Awaken",
    ]);
    assert.deepEqual(packagedExecutableCandidates("darwin", "x64"), [
      "mac/Awaken.app/Contents/MacOS/Awaken",
      "mac-x64/Awaken.app/Contents/MacOS/Awaken",
    ]);
    assert.deepEqual(packagedExecutableCandidates("win32", "x64"), ["win-unpacked/Awaken.exe"]);
    assert.throws(() => packagedExecutableCandidates("linux", "x64"), /does not support linux-x64/);
  });

  it("resolves the first executable that exists", () => {
    const root = tempRoot();
    const x64 = join(root, "mac/Awaken.app/Contents/MacOS/Awaken");
    const found = resolvePackagedExecutable(root, "darwin", "x64", (path) => path === x64);
    assert.equal(found, x64);
    assert.throws(
      () => resolvePackagedExecutable(root, "darwin", "arm64", () => false),
      /was not found/,
    );
  });

  it("isolates account paths and drops development acceptance overrides", () => {
    const root = tempRoot();
    const env = isolatedLaunchEnvironment(root, {
      AWAKEN_DESKTOP_ACCEPTANCE_ROOT: "/tmp/dev-acceptance",
      ELECTRON_RUN_AS_NODE: "1",
      PATH: "/usr/bin",
    });
    assert.equal(env.HOME, join(root, "home"));
    assert.equal(env.USERPROFILE, join(root, "home"));
    assert.equal(env.APPDATA, join(root, "home", "AppData", "Roaming"));
    assert.equal(env.PATH, "/usr/bin");
    assert.equal(env.ELECTRON_ENABLE_LOGGING, "1");
    assert.equal("AWAKEN_DESKTOP_ACCEPTANCE_ROOT" in env, false);
    assert.equal("ELECTRON_RUN_AS_NODE" in env, false);
  });

  it("accepts a transcript only after the main window exists and nothing crashed", () => {
    assert.equal(assessSmokeTranscript(`boot\n${READY_MARKER}\n`).ok, true);
    assert.match(assessSmokeTranscript("boot\n").reason, /main window/);
    assert.match(
      assessSmokeTranscript(`${READY_MARKER}\n[awaken-desktop] uncaughtException`).reason,
      /uncaught exception/,
    );
  });
});
