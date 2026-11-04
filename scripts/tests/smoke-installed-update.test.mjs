import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, it } from "node:test";
import { createPackage, extractFile } from "@electron/asar";
import {
  assertBootTranscript,
  assertProductionGithubFeed,
  assertTargetVersion,
  assertUpgradeTranscript,
  buildLoopbackAppUpdateYaml,
  executeInstalledUpdateSmoke,
  materializeP12,
  findWindowsInstall,
  prepareFeedFiles,
  PREVIOUS_SMOKE_VERSION,
  replacePlistVersion,
  rewriteFeedForLoopback,
  rewriteInstalledApp,
  signDarwinApp,
  startLoopbackFeed,
  windowsSilentInstallArgs,
} from "../release/smoke-installed-update.mjs";

const directories = [];

function tempDir() {
  const directory = mkdtempSync(join(tmpdir(), "awaken-update-smoke-test-"));
  directories.push(directory);
  return directory;
}

afterEach(() => {
  while (directories.length > 0) {
    rmSync(directories.pop(), { recursive: true, force: true });
  }
});

describe("installed update smoke", () => {
  it("never falls back to an installation outside the smoke directory", () => {
    const root = tempDir();
    const production = join(root, "program-files", "Awaken");
    const isolated = join(root, "isolated-install");
    mkdirSync(production, { recursive: true });
    writeFileSync(join(production, "Awaken.exe"), "existing installation");
    const previous = process.env.ProgramFiles;
    process.env.ProgramFiles = join(root, "program-files");
    try {
      assert.equal(findWindowsInstall(isolated), "");
      mkdirSync(isolated, { recursive: true });
      writeFileSync(join(isolated, "Awaken.exe"), "smoke installation");
      assert.equal(findWindowsInstall(isolated), join(isolated, "Awaken.exe"));
    } finally {
      if (previous === undefined) delete process.env.ProgramFiles;
      else process.env.ProgramFiles = previous;
    }
  });

  it("requires a stable version newer than the manufactured previous install", () => {
    assert.equal(PREVIOUS_SMOKE_VERSION, "0.0.1");
    assert.doesNotThrow(() => assertTargetVersion("0.1.0"));
    assert.throws(() => assertTargetVersion("0.0.0"), /0\.0\.0/);
    assert.throws(() => assertTargetVersion("0.0.1"), /not newer/);
    assert.throws(() => assertTargetVersion("0.1.0-beta.1"), /SemVer/);
  });

  it("accepts only the public Awaken feed and keeps publisher names", () => {
    const parsed = assertProductionGithubFeed(
      "provider: github\nowner: NolanGrant556\nrepo: Awaken\npublisherName:\n  - CN=Awaken\n",
    );
    assert.deepEqual(parsed.publisherName, ["CN=Awaken"]);
    assert.throws(
      () => assertProductionGithubFeed("provider: github\nowner: other\nrepo: Awaken\n"),
      /Awaken/,
    );
    assert.throws(
      () =>
        assertProductionGithubFeed(
          "provider: github\nowner: NolanGrant556\nrepo: Awaken\ntoken: ghp_abcdefghijklmnopqrstuvwxyz\n",
        ),
      /credential/,
    );
  });

  it("rewrites feed urls onto the loopback server without wrapping checksums", () => {
    const rewritten = rewriteFeedForLoopback({
      version: "0.1.0",
      path: "https://github.com/NolanGrant556/Awaken/releases/download/v0.1.0/Awaken-0.1.0-arm64.zip",
      files: [
        {
          url: "https://github.com/NolanGrant556/Awaken/releases/download/v0.1.0/Awaken-0.1.0-arm64.zip",
          sha512: "abc",
          size: 4,
        },
      ],
    });
    assert.equal(rewritten.files[0].url, "Awaken-0.1.0-arm64.zip");
    assert.equal(rewritten.path, "Awaken-0.1.0-arm64.zip");
    const yaml = buildLoopbackAppUpdateYaml({
      url: "http://127.0.0.1:4312/",
      publisherName: ["CN=Awaken"],
    });
    assert.match(yaml, /^provider: generic$/m);
    assert.match(yaml, /^url: ["']?http:\/\/127\.0\.0\.1:4312\/["']?$/m);
    assert.throws(
      () => buildLoopbackAppUpdateYaml({ url: "https://example.com/", publisherName: [] }),
      /127\.0\.0\.1/,
    );
  });

  it("serves a range of the candidate payload from 127.0.0.1", async () => {
    const directory = tempDir();
    const payload = join(directory, "payload.bin");
    writeFileSync(payload, "abcdefghijklmnopqrstuvwxyz");
    const feed = await startLoopbackFeed(
      new Map([
        ["latest-mac.yml", { text: "version: 0.1.0\n" }],
        ["payload.bin", { path: payload }],
      ]),
    );
    try {
      assert.match(feed.url, /^http:\/\/127\.0\.0\.1:\d+\/$/);
      const response = await fetch(`${feed.url}payload.bin`, { headers: { Range: "bytes=0-3" } });
      assert.equal(response.status, 206);
      assert.equal(await response.text(), "abcd");
      const missing = await fetch(`${feed.url}missing.bin`);
      assert.equal(missing.status, 404);
    } finally {
      await feed.close();
    }
  });

  it("prepares a loopback feed only when the checksum matches", () => {
    const directory = tempDir();
    const payload = Buffer.from("candidate-zip");
    writeFileSync(join(directory, "Awaken-0.1.0-arm64.zip"), payload);
    const sha512 = createHash("sha512").update(payload).digest("base64");
    writeFileSync(
      join(directory, "latest-mac.yml"),
      `version: 0.1.0\nfiles:\n  - url: Awaken-0.1.0-arm64.zip\n    sha512: "${sha512}"\n    size: ${payload.length}\npath: Awaken-0.1.0-arm64.zip\nsha512: "${sha512}"\n`,
    );
    const files = prepareFeedFiles(directory, "latest-mac.yml", "0.1.0");
    assert.match(files.get("latest-mac.yml").text, /Awaken-0\.1\.0-arm64\.zip/);
    writeFileSync(join(directory, "Awaken-0.1.0-arm64.zip"), Buffer.from("tampered"));
    assert.throws(() => prepareFeedFiles(directory, "latest-mac.yml", "0.1.0"), /sha512 mismatch/);
  });

  it("rewrites the installed copy version and its feed without changing the payload hash input", async () => {
    const directory = tempDir();
    const source = join(directory, "source");
    const resources = join(directory, "Awaken.app", "Contents", "Resources");
    mkdirSync(source, { recursive: true });
    mkdirSync(resources, { recursive: true });
    writeFileSync(join(source, "package.json"), JSON.stringify({ name: "awaken", version: "0.1.0" }));
    await createPackage(source, join(resources, "app.asar"));
    writeFileSync(
      join(directory, "Awaken.app", "Contents", "Info.plist"),
      `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleShortVersionString</key><string>0.1.0</string>
<key>CFBundleVersion</key><string>0.1.0</string>
</dict></plist>`,
    );
    await rewriteInstalledApp({
      appRoot: join(directory, "Awaken.app"),
      version: PREVIOUS_SMOKE_VERSION,
      feedYaml: "provider: generic\nurl: http://127.0.0.1:9/\n",
    });
    const pkg = JSON.parse(extractFile(join(resources, "app.asar"), "package.json").toString("utf8"));
    assert.equal(pkg.version, "0.0.1");
    assert.equal(
      readFileSync(join(resources, "app-update.yml"), "utf8"),
      "provider: generic\nurl: http://127.0.0.1:9/\n",
    );
    const plist = readFileSync(join(directory, "Awaken.app", "Contents", "Info.plist"), "utf8");
    assert.equal(replacePlistVersion(plist, "0.0.1").includes("<string>0.1.0</string>"), false);
  });

  it("requires discover, download, defer, install, and a candidate boot", () => {
    const transcript = [
      "desktop_update_smoke_version 0.0.1",
      "desktop_update_smoke_available 0.1.0",
      "desktop_update_smoke_deferred 0.1.0",
      "desktop_update_install_launching",
    ].join("\n");
    assert.doesNotThrow(() => assertUpgradeTranscript(transcript, "0.0.1", "0.1.0"));
    assert.throws(() => assertUpgradeTranscript("desktop_update_smoke_version 0.0.1\n", "0.0.1", "0.1.0"), /discover/);
    assert.doesNotThrow(() =>
      assertBootTranscript(
        "desktop_update_smoke_version 0.1.0\ndesktop_main_window_created\n",
        "0.1.0",
      ),
    );
    assert.throws(
      () => assertBootTranscript("desktop_update_smoke_installing 0.1.0\n", "0.1.0"),
      /install again/,
    );
  });

  it("drives patch, signature, install, and boot before writing a receipt", async () => {
    const calls = [];
    let installed = "0.1.0";
    const receipt = await executeInstalledUpdateSmoke({
      targetVersion: "0.1.0",
      productionFeedText: "provider: github\nowner: NolanGrant556\nrepo: Awaken\n",
      files: new Map(),
      patchApp: async ({ version, feedYaml }) => {
        calls.push(["patch", version, feedYaml]);
        installed = version;
      },
      signApp: async () => {
        calls.push("sign");
      },
      launch: async ({ args }) => {
        calls.push(["launch", args.join(" ")]);
        if (args.includes("--awaken-update-smoke-install")) {
          installed = "0.1.0";
          return {
            output: [
              "desktop_update_smoke_version 0.0.1",
              "desktop_update_smoke_available 0.1.0",
              "desktop_update_smoke_deferred 0.1.0",
              "desktop_update_install_launching",
            ].join("\n"),
            exitCode: 0,
          };
        }
        return {
          output: "desktop_update_smoke_version 0.1.0\ndesktop_main_window_created\n",
          exitCode: null,
        };
      },
      readInstalledVersion: () => installed,
      sleep: async () => undefined,
      versionAttempts: 2,
    });
    assert.deepEqual(
      calls.map((call) => (Array.isArray(call) ? call[0] : call)),
      ["patch", "sign", "launch", "launch"],
    );
    assert.match(calls[0][2], /http:\/\/127\.0\.0\.1:\d+\//);
    assert.equal(calls[2][1].includes("--awaken-update-smoke-install"), true);
    assert.equal(receipt.ok, true);
    assert.deepEqual(receipt.phases, ["available", "deferred", "installing", "booted"]);
    assert.equal(receipt.feedHost, "127.0.0.1");
  });

  it("does not report success when the installed version never changes", async () => {
    await assert.rejects(
      () =>
        executeInstalledUpdateSmoke({
          targetVersion: "0.1.0",
          productionFeedText: "provider: github\nowner: NolanGrant556\nrepo: Awaken\n",
          files: new Map(),
          patchApp: async () => undefined,
          launch: async () => ({
            output: [
              "desktop_update_smoke_version 0.0.1",
              "desktop_update_smoke_available 0.1.0",
              "desktop_update_smoke_deferred 0.1.0",
              "desktop_update_install_launching",
            ].join("\n"),
            exitCode: 0,
          }),
          readInstalledVersion: () => "0.0.1",
          sleep: async () => undefined,
          versionAttempts: 2,
        }),
      /installed version stayed 0\.0\.1/,
    );
  });

  it("re-signs with the release identity and keeps the certificate password out of errors", () => {
    const directory = tempDir();
    const p12 = join(directory, "cert.p12");
    writeFileSync(p12, Buffer.from("certificate"));
    const calls = [];
    assert.throws(
      () =>
        signDarwinApp(
          join(directory, "Awaken.app"),
          {
            AWAKEN_CODESIGN_IDENTITY: "Developer ID Application: Test (TEAM)",
            CSC_LINK: p12,
            CSC_KEY_PASSWORD: "super-secret-password",
          },
          (command, args) => {
            calls.push(command);
            if (command === "codesign" && args.includes("--verify")) {
              throw new Error(`codesign failed super-secret-password keychain-secret`);
            }
            return "";
          },
          {
            keychainPath: join(directory, "signing.keychain-db"),
            keychainPassword: "keychain-secret",
            p12Path: join(directory, "decoded.p12"),
            exists: (path) => path === p12,
            writeFile: writeFileSync,
          },
        ),
      (error) => {
        assert.equal(error.message.includes("super-secret-password"), false);
        assert.equal(error.message.includes("keychain-secret"), false);
        return true;
      },
    );
    assert.equal(calls.includes("security"), true);
    assert.equal(calls.includes("codesign"), true);
    const decoded = materializeP12(Buffer.alloc(48, 7).toString("base64"), join(directory, "from-base64.p12"), {
      exists: () => false,
      writeFile: writeFileSync,
    });
    assert.equal(readFileSync(decoded).length >= 32, true);
    assert.throws(() => materializeP12("https://example.com/cert.p12", p12), /not a URL/);
    assert.deepEqual(windowsSilentInstallArgs("C:\\AwakenSmoke"), ["/S", "/D=C:\\AwakenSmoke"]);
  });
});
