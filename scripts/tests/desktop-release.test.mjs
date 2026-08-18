import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { createPackage } from "@electron/asar";
import { parse as parseYaml } from "yaml";
import {
  auditInventory,
  buildInventory,
  compareInventories,
  extractAsar,
  inventoriesEqual,
} from "../release/audit-package-contents.mjs";
import {
  filterDmgArtifacts,
} from "../release/notarize-macos-artifacts.mjs";
import {
  isAdhocSignature,
  resolveAppPath,
} from "../release/staple-macos-app.mjs";
import {
  assertReleaseVersion,
  computeSha512Base64,
  isValidReleaseVersion,
  verifyUpdateArtifacts,
} from "../release/verify-update-artifacts.mjs";

const tempRoots = [];
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

function makeTempRoot(prefix) {
  const root = mkdtempSync(join(tmpdir(), prefix));
  tempRoots.push(root);
  return root;
}

afterEach(() => {
  for (const root of tempRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true });
  }
});

// --- fixture helpers -------------------------------------------------------

function writeFile(dir, name, content) {
  const filePath = join(dir, name);
  writeFileSync(filePath, content);
  return filePath;
}

function feedEntry(dir, name) {
  const filePath = join(dir, name);
  return {
    url: name,
    sha512: computeSha512Base64(filePath),
    size: readFileSync(filePath).length,
  };
}

function renderFeed(version, entries) {
  const lines = [`version: ${version}`, "files:"];
  for (const entry of entries) {
    lines.push(`  - url: ${entry.url}`);
    lines.push(`    sha512: ${entry.sha512}`);
    lines.push(`    size: ${entry.size}`);
  }
  lines.push("releaseDate: '2026-01-01T00:00:00.000Z'");
  return `${lines.join("\n")}\n`;
}

// Builds a complete, self-consistent artifact directory for `version`.
function makeArtifactDir(version) {
  const dir = makeTempRoot("awaken-update-artifacts-");
  const macZips = [`Awaken-${version}-arm64.zip`, `Awaken-${version}-x64.zip`];
  const winExe = `Awaken-Setup-${version}-x64.exe`;
  for (const name of macZips) {
    writeFile(dir, name, `fake zip payload ${name}`);
  }
  for (const arch of ["arm64", "x64"]) {
    writeFile(dir, `Awaken-${version}-${arch}.dmg`, `fake dmg payload ${arch}`);
  }
  writeFile(dir, winExe, `fake nsis payload ${version}`);
  writeFile(dir, `${winExe}.blockmap`, "fake blockmap");
  writeFile(dir, "latest-mac.yml", renderFeed(version, macZips.map((n) => feedEntry(dir, n))));
  writeFile(dir, "latest.yml", renderFeed(version, [feedEntry(dir, winExe)]));
  return { dir, macZips, winExe };
}

function makeTree(files) {
  const root = makeTempRoot("awaken-audit-tree-");
  for (const [path, content] of Object.entries(files)) {
    const fullPath = join(root, ...path.split("/"));
    mkdirSync(dirname(fullPath), { recursive: true });
    writeFileSync(fullPath, content);
  }
  return root;
}

// --- verify-update-artifacts ------------------------------------------------

describe("verify-update-artifacts", () => {
  it("accepts a consistent artifact directory and reports hashed assets", () => {
    const { dir, macZips, winExe } = makeArtifactDir("1.2.3");
    writeFile(dir, "builder-debug.yml", "electron-builder local diagnostics");
    writeFile(dir, "builder-effective-config.yaml", "appId: com.awaken.desktop\n");
    const receipt = verifyUpdateArtifacts({ dir, version: "1.2.3" });
    assert.equal(receipt.version, "1.2.3");
    assert.deepEqual(receipt.feeds, ["latest-mac.yml", "latest.yml"]);
    assert.deepEqual(
      receipt.assets.map((asset) => asset.name).sort(),
      [...macZips, winExe].sort(),
    );
    for (const asset of receipt.assets) {
      assert.match(asset.sha256, /^[a-f0-9]{64}$/);
      assert.equal(asset.size > 0, true);
    }
    assert.equal(typeof receipt.checkedAt, "string");
  });

  it("rejects a feed version that differs from the expected version", () => {
    const { dir } = makeArtifactDir("1.2.3");
    assert.throws(
      () => verifyUpdateArtifacts({ dir, version: "1.2.4" }),
      /does not match expected 1\.2\.4/,
    );
  });

  it("rejects 0.0.0 and non-SemVer versions", () => {
    const { dir } = makeArtifactDir("1.2.3");
    assert.throws(() => verifyUpdateArtifacts({ dir, version: "0.0.0" }), /0\.0\.0/);
    assert.throws(() => verifyUpdateArtifacts({ dir, version: "1.2" }), /SemVer/);
    assert.equal(isValidReleaseVersion("0.0.0"), false);
    assert.equal(isValidReleaseVersion("2.0.0"), true);
    assert.equal(isValidReleaseVersion("2.0.0-beta.1"), false);
    assert.throws(() => assertReleaseVersion("v1.2.3"), /SemVer/);
  });

  it("fails closed on a tampered SHA-512", () => {
    const { dir, macZips } = makeArtifactDir("1.2.3");
    // Keep the byte length identical so the SHA-512 check (not size) fires.
    writeFile(dir, macZips[0], "TAMPERED" + "x".repeat(31));
    assert.throws(
      () => verifyUpdateArtifacts({ dir, version: "1.2.3" }),
      /SHA-512 mismatch/,
    );
  });

  it("rejects duplicate file names inside a feed", () => {
    const { dir, macZips } = makeArtifactDir("1.2.3");
    const entry = feedEntry(dir, macZips[0]);
    writeFile(dir, "latest-mac.yml", renderFeed("1.2.3", [entry, entry]));
    assert.throws(
      () => verifyUpdateArtifacts({ dir, version: "1.2.3" }),
      /duplicate names/,
    );
  });

  it("rejects unexpected extra installer assets in the directory", () => {
    const { dir } = makeArtifactDir("1.2.3");
    writeFile(dir, "evil-notes.txt", "not part of the whitelist");
    assert.throws(
      () => verifyUpdateArtifacts({ dir, version: "1.2.3" }),
      /unexpected installer asset in directory: evil-notes\.txt/,
    );
  });

  it("rejects a mac feed that does not cover exactly arm64+x64 zips", () => {
    const { dir, macZips } = makeArtifactDir("1.2.3");
    writeFile(
      dir,
      "latest-mac.yml",
      renderFeed("1.2.3", [feedEntry(dir, macZips[0])]),
    );
    assert.throws(
      () => verifyUpdateArtifacts({ dir, version: "1.2.3" }),
      /must reference exactly/,
    );
  });
});

// --- audit-package-contents --------------------------------------------------

describe("audit-package-contents", () => {
  it("builds a sorted path/size/sha256 inventory", () => {
    const root = makeTree({
      "b/second.js": "two",
      "a/first.js": "one",
    });
    const inventory = buildInventory(root);
    assert.deepEqual(
      inventory.map((entry) => entry.path),
      ["a/first.js", "b/second.js"],
    );
    for (const entry of inventory) {
      assert.match(entry.sha256, /^[a-f0-9]{64}$/);
      assert.equal(entry.size > 0, true);
    }
  });

  it("passes a clean tree", () => {
    const root = makeTree({
      "dist/main/index.js": "console.log('ok')",
      "dist/renderer/assets/app.css": "body{}",
    });
    const inventory = buildInventory(root);
    const violations = auditInventory(inventory, (entry) =>
      readFileSync(join(root, ...entry.path.split("/"))),
    );
    assert.deepEqual(violations, []);
  });

  it("flags denied paths and secrets without echoing secret content", () => {
    const privateKey =
      "-----BEGIN PRIVATE KEY-----\nMIIEvgFAKEFAKEFAKE\n-----END PRIVATE KEY-----\n";
    const root = makeTree({
      "dist/main/index.js.map": "{}",
      "dist/main/util.test.js": "test('x')",
      "dist/main/__tests__/helper.js": "x",
      "dist/main/fixtures/sample.json": "{}",
      "config/.env": "SECRET=1",
      "repo/.git/config": "[core]",
      "certs/dev.p12": "binary",
      "dist/main/leak.js": `const key = \`${privateKey}\`;`,
      "dist/main/token.js": "const t = 'ghp_abcdefghijklmnopqrstuvwxyz0123456789';",
    });
    const inventory = buildInventory(root);
    const violations = auditInventory(inventory, (entry) =>
      readFileSync(join(root, ...entry.path.split("/"))),
    );
    const rulesByPath = new Map(violations.map((v) => [v.path, v.rule]));
    assert.equal(rulesByPath.get("dist/main/index.js.map"), "source-map");
    assert.equal(rulesByPath.get("dist/main/util.test.js"), "test-file");
    assert.equal(rulesByPath.get("dist/main/__tests__/helper.js"), "test-dir");
    assert.equal(rulesByPath.get("dist/main/fixtures/sample.json"), "fixture-dir");
    assert.equal(rulesByPath.get("config/.env"), "env-file");
    assert.equal(rulesByPath.get("repo/.git/config"), "git-metadata");
    assert.equal(rulesByPath.get("certs/dev.p12"), "signing-material");
    assert.equal(rulesByPath.get("dist/main/leak.js"), "private-key");
    assert.equal(rulesByPath.get("dist/main/token.js"), "github-token");
    // Reports carry path + rule id only, never the matched secret text.
    const report = JSON.stringify(violations);
    assert.equal(report.includes("MIIEvgFAKEFAKEFAKE"), false);
    assert.equal(report.includes("ghp_abcdefghijklmnopqrstuvwxyz0123456789"), false);
  });

  it("diffs inventories path by path", () => {
    const rootA = makeTree({ "keep.js": "same", "change.js": "old", "gone.js": "x" });
    const rootB = makeTree({ "keep.js": "same", "change.js": "new!", "added.js": "y" });
    const diff = compareInventories(buildInventory(rootA), buildInventory(rootB));
    assert.deepEqual(diff.onlyInA, ["gone.js"]);
    assert.deepEqual(diff.onlyInB, ["added.js"]);
    assert.deepEqual(diff.changed, ["change.js"]);
    assert.equal(inventoriesEqual(diff), false);
    assert.equal(
      inventoriesEqual(compareInventories(buildInventory(rootA), buildInventory(rootA))),
      true,
    );
  });

  it("round-trips app.asar extraction into an auditable inventory", async () => {
    const source = makeTree({ "index.js": "module.exports = 1;" });
    const work = makeTempRoot("awaken-asar-");
    const asarPath = join(work, "app.asar");
    const outDir = join(work, "extracted");
    await createPackage(source, asarPath);
    extractAsar(asarPath, outDir);
    const inventory = buildInventory(outDir);
    assert.deepEqual(
      inventory.map((entry) => entry.path),
      ["index.js"],
    );
    assert.equal(
      readFileSync(join(outDir, "index.js"), "utf8"),
      "module.exports = 1;",
    );
  });
});

// --- macOS hook helpers ------------------------------------------------------

describe("macOS release hooks", () => {
  it("resolves the single app bundle from the afterSign context", () => {
    const root = makeTempRoot("awaken-hook-context-");
    mkdirSync(join(root, "Awaken.app"), { recursive: true });
    const appPath = resolveAppPath({
      appOutDir: root,
      packager: { appInfo: { productFilename: "Awaken" } },
    });
    assert.equal(appPath, join(root, "Awaken.app"));
    assert.throws(
      () =>
        resolveAppPath({
          appOutDir: root,
          packager: { appInfo: { productFilename: "Missing" } },
        }),
      /does not exist/,
    );
  });

  it("classifies ad-hoc vs Developer ID codesign output", () => {
    assert.equal(
      isAdhocSignature("Executable=/x\nIdentifier=awaken\nSignature=adhoc\n"),
      true,
    );
    assert.equal(
      isAdhocSignature(
        "Signature=adhoc\nAuthority=Developer ID Application: Awaken (TEAM)\n",
      ),
      // Developer ID authority wins even if the output is unusual.
      false,
    );
    assert.equal(
      isAdhocSignature(
        "Executable=/x\nAuthority=Developer ID Application: Awaken (TEAM)\n",
      ),
      false,
    );
    assert.equal(isAdhocSignature("Executable=/x\n"), true);
  });

  it("filters DMG artifacts from the build result", () => {
    assert.deepEqual(
      filterDmgArtifacts([
        "/out/Awaken-1.2.3-arm64.dmg",
        "/out/Awaken-1.2.3-arm64.zip",
        "/out/Awaken-Setup-1.2.3-x64.exe",
        "/out/Awaken-1.2.3-x64.dmg",
      ]),
      ["/out/Awaken-1.2.3-arm64.dmg", "/out/Awaken-1.2.3-x64.dmg"],
    );
    assert.deepEqual(filterDmgArtifacts([]), []);
  });
});

// --- config contract ---------------------------------------------------------

describe("release config contract", () => {
  it("electron-builder.yml carries the update provider, targets, hooks and exclusions", () => {
    const config = parseYaml(
      readFileSync(join(repoRoot, "apps", "desktop", "electron-builder.yml"), "utf8"),
    );
    assert.deepEqual(config.publish, {
      provider: "github",
      owner: "NolanGrant556",
      repo: "Awaken-Releases",
      releaseType: "release",
    });
    assert.equal(config.electronUpdaterCompatibility, ">=2.16");
    assert.equal(config.extraMetadata.awakenUpdateChannel, "disabled");
    assert.equal(config.afterSign, "../../scripts/release/staple-macos-app.mjs");
    assert.equal(
      config.afterAllArtifactBuild,
      "../../scripts/release/notarize-macos-artifacts.mjs",
    );
    for (const hook of [config.afterSign, config.afterAllArtifactBuild]) {
      assert.equal(
        existsSync(join(repoRoot, "apps", "desktop", hook)),
        true,
        `hook exists: ${hook}`,
      );
    }

    assert.equal(config.mac.notarize, true);
    assert.equal(config.mac.artifactName, "Awaken-${version}-${arch}.${ext}");
    assert.deepEqual(config.mac.target, [
      { target: "dmg", arch: ["arm64", "x64"] },
      { target: "zip", arch: ["arm64", "x64"] },
    ]);
    assert.equal(config.win.artifactName, "Awaken-Setup-${version}-${arch}.${ext}");
    assert.deepEqual(config.win.target, [{ target: "nsis", arch: ["x64"] }]);

    for (const exclusion of [
      "!dist/**/*.test.js",
      "!dist/**/*.spec.js",
      "!dist/**/*.map",
      "!dist/**/__tests__/**",
      "!dist/**/fixtures/**",
    ]) {
      assert.equal(config.files.includes(exclusion), true, `files excludes ${exclusion}`);
    }

    const binaryResources = config.extraResources.find(
      (entry) => entry.from === "resources/bin/${platform}-${arch}",
    );
    for (const exclusion of [
      "!node/include/**/*",
      "!node/share/doc/**/*",
      "!node/share/man/**/*",
      "!node/README.md",
      "!node/CHANGELOG.md",
      "!**/*.map",
    ]) {
      assert.equal(binaryResources.filter.includes(exclusion), true);
    }
    for (const from of ["resources/cua-driver", "resources/cua-driver/node_modules"]) {
      const resource = config.extraResources.find((entry) => entry.from === from);
      assert.equal(resource.filter.includes("!**/*.map"), true);
      assert.equal(
        resource.filter.includes("!**/{test,tests,__tests__,example,examples}/**/*"),
        true,
      );
    }
  });

  it("desktop package declares a real release version and runtime electron-updater", () => {
    const pkg = JSON.parse(
      readFileSync(join(repoRoot, "apps", "desktop", "package.json"), "utf8"),
    );
    assert.match(pkg.version, /^\d+\.\d+\.\d+$/);
    assert.notEqual(pkg.version, "0.0.0");
    assert.equal(typeof pkg.dependencies["electron-updater"], "string");
    assert.equal(pkg.devDependencies["electron-updater"], undefined);
    assert.match(pkg.scripts.dist, /-c\.mac\.notarize=false/);
    assert.match(pkg.scripts["dist:adhoc"], /-c\.mac\.notarize=false/);
    assert.match(pkg.scripts["dist:release"], /-c\.forceCodeSigning=true/);
    assert.match(
      pkg.scripts["dist:release"],
      /-c\.extraMetadata\.awakenUpdateChannel=stable/,
    );
    assert.doesNotMatch(pkg.scripts["dist:release"], /notarize=false/);

    const lock = JSON.parse(readFileSync(join(repoRoot, "package-lock.json"), "utf8"));
    assert.equal(lock.packages["apps/desktop"].version, pkg.version);
    assert.equal(lock.packages["node_modules/electron-updater"].version, "6.8.9");
  });

  it("release workflow pins the commit output, uses immutable receipt names and blocks fake smoke", () => {
    const workflow = parseYaml(
      readFileSync(join(repoRoot, ".github", "workflows", "desktop-release.yml"), "utf8"),
    );
    assert.equal(
      workflow.jobs.prepare.outputs.sha,
      "${{ steps.commit.outputs.sha }}",
    );
    const receiptNames = [];
    for (const job of Object.values(workflow.jobs)) {
      for (const step of job.steps ?? []) {
        if (step.uses === "actions/upload-artifact@v4" && String(step.with?.name).startsWith("release-receipts")) {
          receiptNames.push(step.with.name);
        }
      }
    }
    assert.equal(new Set(receiptNames).size, receiptNames.length);
    const smokeScript = workflow.jobs["upgrade-smoke"].steps.map((step) => step.run ?? "").join("\n");
    assert.match(smokeScript, /Refusing to publish/);
    assert.match(smokeScript, /exit 1/);
    assert.doesNotMatch(smokeScript, /smoke driver placeholder/);
  });
});
