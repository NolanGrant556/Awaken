import assert from "node:assert/strict";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
  isPackagedAwakenCommand,
  isUsableDevDesktopWindow,
  listPackagedAwakenProcesses,
  parsePsRows,
} from "../desktop-lifecycle-common.mjs";
import { cleanDesktopPackageOutputs } from "../clean-desktop-package.mjs";

const tempRoots = [];
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

afterEach(() => {
  for (const root of tempRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true });
  }
});

describe("desktop process ownership", () => {
  it("accepts a PID-scoped main window when macOS omits its title", () => {
    assert.equal(
      isUsableDevDesktopWindow({ name: "", width: 1440, height: 863, x: 36, y: 33 }),
      true,
    );
    assert.equal(
      isUsableDevDesktopWindow({
        name: "Developer Tools",
        width: 1440,
        height: 863,
        x: 36,
        y: 33,
      }),
      false,
    );
    assert.equal(
      isUsableDevDesktopWindow({ name: "", width: 1512, height: 33, x: 0, y: 0 }),
      false,
    );
  });

  it("detects only packaged Awaken main executables on macOS", () => {
    assert.equal(
      isPackagedAwakenCommand(
        "/Applications/Awaken.app/Contents/MacOS/Awaken",
        "darwin",
      ),
      true,
    );
    assert.equal(
      isPackagedAwakenCommand(
        "/repo/node_modules/electron/dist/Electron.app/Contents/MacOS/Electron",
        "darwin",
      ),
      false,
    );
    assert.equal(
      isPackagedAwakenCommand(
        "/Applications/Awaken.app/Contents/Frameworks/Awaken Helper.app/Contents/MacOS/Awaken Helper",
        "darwin",
      ),
      false,
    );
  });

  it("detects a Windows packaged main executable", () => {
    assert.equal(
      isPackagedAwakenCommand("C:\\Program Files\\Awaken\\Awaken.exe", "win32"),
      true,
    );
    assert.equal(
      isPackagedAwakenCommand("C:\\Awaken\\awaken-core.exe", "win32"),
      false,
    );
  });

  it("parses ps command rows and filters package instances", () => {
    const result = listPackagedAwakenProcesses({
      platform: "darwin",
      runCommand: () => ({
        ok: true,
        stdout: [
          "  41 /Applications/Awaken.app/Contents/MacOS/Awaken",
          "  42 /repo/node_modules/electron/dist/Electron.app/Contents/MacOS/Electron",
        ].join("\n"),
      }),
    });
    assert.deepEqual(parsePsRows("  7 /bin/zsh\n"), [
      { pid: 7, command: "/bin/zsh" },
    ]);
    assert.deepEqual(result, [
      { pid: 41, command: "/Applications/Awaken.app/Contents/MacOS/Awaken" },
    ]);
  });

  it("parses Windows process inventory from PowerShell", () => {
    const result = listPackagedAwakenProcesses({
      platform: "win32",
      runCommand: () => ({
        ok: true,
        stdout: JSON.stringify([
          { ProcessId: 51, ExecutablePath: "C:\\Program Files\\Awaken\\Awaken.exe" },
        ]),
      }),
    });
    assert.deepEqual(result, [
      { pid: 51, command: "C:\\Program Files\\Awaken\\Awaken.exe" },
    ]);
  });

});

describe("desktop package staging", () => {
  it("removes generated package outputs without creating runtime staging", () => {
    const root = mkdtempSync(join(tmpdir(), "awaken-desktop-clean-"));
    tempRoots.push(root);
    mkdirSync(join(root, "apps", "desktop", "dist-electron"), { recursive: true });
    mkdirSync(join(root, "apps", "desktop", "dist"), { recursive: true });
    mkdirSync(join(root, "apps", "desktop", "resources", "bin"), { recursive: true });
    writeFileSync(join(root, "package.json"), JSON.stringify({ name: "awaken" }));
    writeFileSync(
      join(root, "apps", "desktop", "package.json"),
      JSON.stringify({ name: "@awaken/desktop" }),
    );
    writeFileSync(join(root, "apps", "desktop", "dist", "stale.js"), "stale");
    writeFileSync(
      join(root, "apps", "desktop", "dist-electron", "stale.dmg"),
      "stale",
    );
    writeFileSync(join(root, "apps", "desktop", "resources", "bin", "stale"), "stale");
    cleanDesktopPackageOutputs(root);
    assert.throws(() =>
      readFileSync(join(root, "apps", "desktop", "dist", "stale.js")),
    );
    assert.equal(existsSync(join(root, "apps", "desktop", "resources", "runtime-archive")), false);
  });

  it("packages only the target architecture awaken-core", () => {
    const config = readFileSync(
      join(repoRoot, "apps", "desktop", "electron-builder.yml"),
      "utf8",
    );
    assert.match(config, /resources\/bin\/\$\{platform\}-\$\{arch\}/);
    assert.doesNotMatch(config, /^\s*- from: resources\/bin\s*$/m);
  });
});
