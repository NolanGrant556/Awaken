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
import {
  officeCliBinaryName,
  officeCliTarget,
  readOfficeCliManifest,
} from "../prepare-officecli.mjs";

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

describe("desktop managed entrypoint", () => {
  it("does not run the foreground cleanup hook inside its own screen session", () => {
    const rootPackage = JSON.parse(
      readFileSync(join(repoRoot, "package.json"), "utf8"),
    );
    const openDesktopSource = readFileSync(
      join(repoRoot, "scripts", "open-desktop.mjs"),
      "utf8",
    );

    assert.equal(rootPackage.scripts["predev:desktop"], "npm run close:desktop");
    assert.equal(rootPackage.scripts["dev:desktop"], "npm run dev:desktop:managed");
    assert.equal(typeof rootPackage.scripts["dev:desktop:managed"], "string");
    assert.equal(rootPackage.scripts["predev:desktop:managed"], undefined);
    assert.match(
      openDesktopSource,
      /`npm run dev:desktop:managed > \$\{shellQuote\(logFile\)\} 2>&1`,/,
    );
    assert.doesNotMatch(openDesktopSource, /npm run close:desktop/);
  });
});

describe("desktop package staging", () => {
  it("removes generated package outputs without creating runtime staging", () => {
    const root = mkdtempSync(join(tmpdir(), "awaken-desktop-clean-"));
    tempRoots.push(root);
    mkdirSync(join(root, "apps", "desktop", "dist-electron"), { recursive: true });
    mkdirSync(join(root, "apps", "desktop", "dist"), { recursive: true });
    mkdirSync(join(root, "apps", "desktop", "resources", "bin"), { recursive: true });
    mkdirSync(join(root, "apps", "desktop", "resources", "system-skills"), { recursive: true });
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
    writeFileSync(
      join(root, "apps", "desktop", "resources", "system-skills", "README.md"),
      "owned source",
    );
    cleanDesktopPackageOutputs(root);
    assert.throws(() =>
      readFileSync(join(root, "apps", "desktop", "dist", "stale.js")),
    );
    assert.equal(existsSync(join(root, "apps", "desktop", "resources", "runtime-archive")), false);
    assert.equal(
      readFileSync(
        join(root, "apps", "desktop", "resources", "system-skills", "README.md"),
        "utf8",
      ),
      "owned source",
    );
  });

  it("packages only the target architecture runtime binaries", () => {
    const config = readFileSync(
      join(repoRoot, "apps", "desktop", "electron-builder.yml"),
      "utf8",
    );
    assert.match(config, /resources\/bin\/\$\{platform\}-\$\{arch\}/);
    assert.doesNotMatch(config, /^\s*- from: resources\/bin\s*$/m);
    assert.match(config, /^\s*- from: resources\/system-skills\s*$/m);
    assert.match(config, /^\s*- from: resources\/builtin-plugins\s*$/m);
    assert.match(config, /^\s*- from: resources\/officecli-release\.json\s*$/m);
    assert.match(config, /^\s*- from: resources\/licenses\s*$/m);
    const buildSource = readFileSync(join(repoRoot, "scripts", "build-binary.mjs"), "utf8");
    assert.match(buildSource, /awaken-network-watch\.exe/);
    assert.match(buildSource, /platform === "win32"/);
    assert.match(
      buildSource,
      /defaultMicrosoftOutlookClientId = "1bdaadb2-a9d3-44a9-99cf-806172ca83af"/,
    );
    assert.match(buildSource, /AWAKEN_MICROSOFT_CLIENT_ID/);
    const ensureSource = readFileSync(
      join(repoRoot, "scripts", "ensure-desktop-runtime.mjs"),
      "utf8",
    );
    assert.match(ensureSource, /companionBinaryPaths/);
    assert.match(ensureSource, /awaken-network-watch\.exe/);
  });

  it("ships the complete Outlook built-in Plugin package", () => {
    const pluginRoot = join(
      repoRoot,
      "apps",
      "desktop",
      "resources",
      "builtin-plugins",
      "awaken-outlook",
    );
    const metadata = JSON.parse(
      readFileSync(join(pluginRoot, ".awaken", "builtin.json"), "utf8"),
    );
    const manifest = JSON.parse(
      readFileSync(join(pluginRoot, "plugin.json"), "utf8"),
    );
    assert.deepEqual(metadata, {
      version: 1,
      displayName: "Outlook 邮箱",
      brandIcon: "outlook",
      order: 0,
    });
    assert.equal(manifest.name, "awaken-outlook");
    assert.equal(manifest.version, "1.1.0");
    assert.doesNotThrow(() =>
      JSON.parse(readFileSync(join(pluginRoot, ".awaken", "setup.json"), "utf8")),
    );
    for (const skill of [
      "inbox-triage",
      "outlook-email",
      "reply-drafting",
      "subscription-cleanup",
      "task-extraction",
      "shared-mailboxes",
      "attachment-management",
      "scheduled-mail",
      "mailbox-organization",
    ]) {
      const source = readFileSync(join(pluginRoot, "skills", skill, "SKILL.md"), "utf8");
      assert.match(source, /^---\nname: [a-z0-9-]+\ndescription: .+/);
      assert.match(source, /\nmetadata:\n\s+display_name: .+/);
    }
  });

  it("pins one verified OfficeCLI asset per supported product target", () => {
    const manifest = readOfficeCliManifest();
    assert.deepEqual(Object.keys(manifest.targets).sort(), [
      "darwin-arm64",
      "darwin-x64",
      "win32-arm64",
      "win32-x64",
    ]);
    for (const entry of Object.values(manifest.targets)) {
      assert.match(entry.sha256, /^[a-f0-9]{64}$/);
      assert.equal(typeof entry.asset, "string");
    }
    assert.equal(officeCliTarget("darwin", "arm64"), "darwin-arm64");
    assert.equal(officeCliTarget("win32", "x64"), "win32-x64");
    assert.equal(officeCliBinaryName("darwin-arm64"), "officecli");
    assert.equal(officeCliBinaryName("win32-x64"), "officecli.exe");

    const desktopPackage = JSON.parse(
      readFileSync(join(repoRoot, "apps", "desktop", "package.json"), "utf8"),
    );
    assert.match(
      desktopPackage.scripts.dist,
      /clean:package && npm run build:binary && npm run prepare:officecli && npm run prepare:runtime && npm run build/,
    );
    assert.match(desktopPackage.scripts["dist:adhoc"], /-c\.mac\.identity=-/);
    assert.match(desktopPackage.scripts["dist:adhoc"], /-c\.mac\.hardenedRuntime=false/);
    assert.match(desktopPackage.scripts["dist:release"], /-c\.forceCodeSigning=true/);
    assert.match(
      desktopPackage.scripts["dist:release"],
      /-c\.extraMetadata\.awakenUpdateChannel=stable/,
    );
    const rootPackage = JSON.parse(readFileSync(join(repoRoot, "package.json"), "utf8"));
    assert.match(
      rootPackage.scripts["dist:desktop:adhoc"],
      /@awaken\/desktop run dist:adhoc/,
    );
    const ensureSource = readFileSync(
      join(repoRoot, "scripts", "ensure-desktop-runtime.mjs"),
      "utf8",
    );
    assert.match(ensureSource, /scripts\/prepare-officecli\.mjs/);

    for (const name of ["LICENSE", "NOTICE", "THIRD-PARTY-NOTICES.txt"]) {
      assert.equal(
        existsSync(
          join(
            repoRoot,
            "apps",
            "desktop",
            "resources",
            "licenses",
            "officecli",
            manifest.version,
            name,
          ),
        ),
        true,
      );
    }
  });
});
