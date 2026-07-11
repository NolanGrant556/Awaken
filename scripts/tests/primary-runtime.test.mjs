import assert from "node:assert/strict";
import {
  chmodSync,
  copyFileSync,
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, it } from "node:test";
import {
  activateVersion,
  buildResolvedState,
  candidateSeedDirs,
  checkPrimaryRuntime,
  cleanupStaging,
  componentRelativePath,
  copyDirectoryContents,
  createBuildRevision,
  createStagingDir,
  inspectRuntimeProvenance,
  isForbiddenExternalPath,
  macOSRuntimeSigningExpectation,
  machODependencies,
  migrateOldAwakenCacheIfNeeded,
  pathInsideRoot,
  probeComponent,
  publishStaging,
  readCurrentJson,
  readMigrationMarker,
  resolveAwakenUserDataDir,
  resolveLibreOfficeInstallRoot,
  resolveManagedPath,
  resolvePrimaryRuntimeRoot,
  rollbackToPrevious,
  runtimeRequiresPreparation,
  sealMacOSRuntimeApplicationPermissions,
  setMacOSBackgroundOnlyApplication,
  signMacOSRuntimeApplication,
  versionKey,
  versionDir,
  verifyMacOSLibreOfficeReleaseSeed,
  writeCurrentJson,
  writeExecutable,
  writeExecWrapper,
  writeResolvedState,
} from "../primary-runtime-common.mjs";
import {
  createRuntimeArchiveInvocation,
  extractRuntimeArchiveInvocation,
  hasRuntimeArchiveIntegrity,
  runtimeArchiveIntegrityManifest,
  runtimeArchivePlatform,
  runtimeOfficeExecutableRelativePath,
} from "../runtime-archive-platform.mjs";

const temps = [];
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

function tempDir(prefix = "awaken-runtime-test-") {
  const dir = mkdtempSync(join(tmpdir(), prefix));
  temps.push(dir);
  return dir;
}

afterEach(() => {
  while (temps.length) {
    cleanupStaging(temps.pop());
  }
});

function fakeOkScript(message = "ok") {
  return `#!/usr/bin/env bash
set -euo pipefail
echo ${JSON.stringify(message)}
exit 0
`;
}

function fakeFailScript() {
  return `#!/usr/bin/env bash
set -euo pipefail
echo fail >&2
exit 2
`;
}

function writeMinimalMacOSApp(appRoot) {
  const executable = join(appRoot, "Contents", "MacOS", "test-app");
  mkdirSync(dirname(executable), { recursive: true });
  copyFileSync("/usr/bin/true", executable);
  chmodSync(executable, 0o755);
  writeFileSync(join(appRoot, "Contents", "Info.plist"), `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleExecutable</key><string>test-app</string>
  <key>CFBundleIdentifier</key><string>com.awaken.runtime-sign-test</string>
  <key>CFBundleName</key><string>Runtime Sign Test</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>CFBundleDocumentTypes</key><array><dict><key>CFBundleTypeName</key><string>Office</string></dict></array>
  <key>CFBundleURLTypes</key><array><dict><key>CFBundleURLSchemes</key><array><string>ms-word</string></array></dict></array>
  <key>NSServices</key><array><dict/></array>
  <key>UTExportedTypeDeclarations</key><array><dict/></array>
  <key>UTImportedTypeDeclarations</key><array><dict/></array>
  <key>LSUIElement</key><true/>
</dict></plist>
`);
  mkdirSync(join(appRoot, "Contents", "Library", "Spotlight", "OOoSpotlightImporter.mdimporter"), { recursive: true });
  mkdirSync(join(appRoot, "Contents", "PlugIns", "QuickLookPreview.appex"), { recursive: true });
  mkdirSync(join(appRoot, "Contents", "PlugIns", "QuickLookThumbnail.appex"), { recursive: true });
}

function baseManifest() {
  return {
    schemaVersion: 2,
    runtimeName: "primary-runtime",
    bundleVersion: "0.2.0",
    components: [
      { name: "python", kind: "executable", required: true, relativePath: "python/bin/python3", probe: { args: ["-c", "pass"], acceptedExitCodes: [0], requireManagedPrefix: false } },
      { name: "node", kind: "executable", required: true, relativePath: "node/bin/node", probe: { args: ["--version"], acceptedExitCodes: [0] } },
      { name: "libreoffice", kind: "application", required: true, relativePath: "libreoffice/LibreOffice.app", probe: { args: ["--version"], acceptedExitCodes: [0] } },
      { name: "poppler", kind: "directory", required: true, relativePath: "poppler/bin", probe: { tools: ["pdftoppm", "pdfinfo"] } },
      { name: "ffmpeg", kind: "executable", required: true, relativePath: "ffmpeg/bin/ffmpeg", probe: { args: ["-version"], acceptedExitCodes: [0] } },
      { name: "helper-packages", kind: "directory", required: true, relativePath: "packages", probe: { requireFile: "runtime.json" } },
    ],
  };
}

function writeFakeRuntimeTree(root, { failPython = false } = {}) {
  const versionRoot = join(root, "versions", "0.2.0", "darwin-arm64");
  const python = join(versionRoot, "python", "bin", "python3");
  const node = join(versionRoot, "node", "bin", "node");
  const soffice = join(versionRoot, "libreoffice", "LibreOffice.app", "Contents", "MacOS", "soffice");
  const pdftoppm = join(versionRoot, "poppler", "bin", "pdftoppm");
  const pdfinfo = join(versionRoot, "poppler", "bin", "pdfinfo");
  const ffmpeg = join(versionRoot, "ffmpeg", "bin", "ffmpeg");
  const packages = join(versionRoot, "packages");

  writeExecutable(python, failPython ? fakeFailScript() : `#!/usr/bin/env bash
set -euo pipefail
exit 0
`);
  writeExecutable(node, fakeOkScript("v20.0.0"));
  writeExecutable(soffice, fakeOkScript("LibreOffice 24.0"));
  writeExecutable(pdftoppm, `#!/usr/bin/env bash
echo Usage: pdftoppm >&2
exit 1
`);
  writeExecutable(pdfinfo, `#!/usr/bin/env bash
echo pdfinfo 24.0 >&2
exit 0
`);
  writeExecutable(ffmpeg, fakeOkScript("ffmpeg version"));
  mkdirSync(packages, { recursive: true });
  writeFileSync(join(packages, "runtime.json"), JSON.stringify({ ok: true }));

  const manifest = baseManifest();
  writeResolvedState(versionRoot, buildResolvedState({
    manifest,
    versionRootPath: versionRoot,
    platform: "darwin",
    arch: "arm64",
  }));
  writeCurrentJson(root, { active: "0.2.0/darwin-arm64", previous: "" });
  return { versionRoot, manifest, python, node };
}

describe("resolveAwakenUserDataDir", () => {
  it("prefers AWAKEN_USER_DATA_DIR", () => {
    const dir = tempDir();
    assert.equal(resolveAwakenUserDataDir({ AWAKEN_USER_DATA_DIR: dir }, "darwin"), dir);
  });

  it("uses macOS Application Support fixture", () => {
    const home = tempDir();
    assert.equal(
      resolveAwakenUserDataDir({ HOME: home }, "darwin"),
      join(home, "Library", "Application Support", "@awaken", "desktop"),
    );
  });

  it("uses Windows APPDATA fixture", () => {
    const appData = tempDir();
    assert.equal(
      resolveAwakenUserDataDir({ APPDATA: appData, HOME: tempDir() }, "win32"),
      join(appData, "@awaken", "desktop"),
    );
  });

  it("uses Linux XDG fixture", () => {
    const xdg = tempDir();
    assert.equal(
      resolveAwakenUserDataDir({ XDG_CONFIG_HOME: xdg, HOME: tempDir() }, "linux"),
      join(xdg, "@awaken", "desktop"),
    );
  });
});

describe("ownership and seeds", () => {
  it("does not include Codex cache in candidateSeedDirs", () => {
    const home = tempDir();
    const codex = join(home, ".cache", "codex-runtimes", "codex-primary-runtime", "dependencies");
    mkdirSync(codex, { recursive: true });
    const dirs = candidateSeedDirs({ HOME: home });
    assert.deepEqual(dirs, []);
  });

  it("keeps a full Awaken known-good runtime ready after the Codex cache is deleted", () => {
    const home = tempDir();
    const codex = join(home, ".cache", "codex-runtimes", "codex-primary-runtime", "dependencies");
    writeExecutable(join(codex, "bin", "python3"), fakeOkScript("codex-only"));

    const root = tempDir();
    const { manifest } = writeFakeRuntimeTree(root);
    const before = checkPrimaryRuntime({ root, platform: "darwin", arch: "arm64", manifest });
    assert.equal(before.ready, true);
    assert.ok(before.components.every((component) => pathInsideRoot(component.resolvedPath, root)));

    rmSync(join(home, ".cache", "codex-runtimes"), { recursive: true, force: true });
    assert.equal(existsSync(codex), false);

    const after = checkPrimaryRuntime({ root, platform: "darwin", arch: "arm64", manifest });
    assert.equal(after.ready, true);
    assert.deepEqual(after.missing, []);
    assert.equal(after.active, before.active);
    assert.ok(after.components.every((component) => component.ok && pathInsideRoot(component.resolvedPath, root)));
    assert.ok(after.components.every((component) => !String(component.resolvedPath).includes("codex-runtimes")));
  });

  it("only accepts explicit AWAKEN_PRIMARY_RUNTIME_SEED_DIR", () => {
    const seed = tempDir();
    assert.deepEqual(candidateSeedDirs({ AWAKEN_PRIMARY_RUNTIME_SEED_DIR: seed }), [seed]);
  });

  it("AWAKEN_PRIMARY_RUNTIME_ROOT overrides userData root", () => {
    const root = tempDir();
    assert.equal(resolvePrimaryRuntimeRoot({ AWAKEN_PRIMARY_RUNTIME_ROOT: root }, "darwin"), root);
  });

  it("prepares a new immutable revision when the manifest bundle version changes", () => {
    const ready = { ready: true, bundleVersion: "0.2.0" };
    assert.equal(runtimeRequiresPreparation(ready, { bundleVersion: "0.3.0" }), true);
    assert.equal(runtimeRequiresPreparation(ready, { bundleVersion: "0.2.0" }), false);
    assert.equal(runtimeRequiresPreparation(ready, { bundleVersion: "0.2.0" }, true), true);
    assert.equal(runtimeRequiresPreparation({ ready: false }, { bundleVersion: "0.2.0" }), true);
  });

  it("does not reuse an ad-hoc runtime for a release signing build", () => {
    const adHocExpectation = macOSRuntimeSigningExpectation({}, "darwin");
    const releaseExpectation = macOSRuntimeSigningExpectation({
      AWAKEN_RUNTIME_RELEASE_SIGNING: "1",
      AWAKEN_RUNTIME_CODESIGN_IDENTITY: "Developer ID Application: Awaken Test",
    }, "darwin");
    const report = {
      ready: true,
      bundleVersion: "0.3.0",
      security: {
        macOSRuntimeSigning: {
          mode: "ad-hoc",
          identity: "-",
          strictVerified: true,
          gatekeeperVerified: false,
          notarized: false,
          stapled: false,
        },
      },
    };
    assert.equal(runtimeRequiresPreparation(report, { bundleVersion: "0.3.0" }, false, {
      macOSRuntimeSigning: adHocExpectation,
    }), false);
    assert.equal(runtimeRequiresPreparation(report, { bundleVersion: "0.3.0" }, false, {
      macOSRuntimeSigning: releaseExpectation,
    }), true);
    report.security.macOSRuntimeSigning = { ...releaseExpectation };
    assert.equal(runtimeRequiresPreparation(report, { bundleVersion: "0.3.0" }, false, {
      macOSRuntimeSigning: releaseExpectation,
    }), false);
  });

  it("resolves platform-specific managed paths without venv or macOS leakage", () => {
    const manifest = JSON.parse(readFileSync(
      join(repoRoot, "apps/desktop/resources/runtime-manifest/primary-runtime.json"),
      "utf8",
    ));
    const python = manifest.components.find((component) => component.name === "python");
    const libreoffice = manifest.components.find((component) => component.name === "libreoffice");
    assert.equal(componentRelativePath(python, "win32"), "python/python.exe");
    assert.equal(componentRelativePath(python, "linux"), "python/bin/python3");
    assert.equal(componentRelativePath(libreoffice, "linux"), "libreoffice");
    assert.equal(manifest.bundleVersion, "0.3.0");
    assert.deepEqual(manifest.officeRenderWorker, {
      component: "libreoffice",
      defaultMaxConcurrency: 1,
      defaultTimeoutSeconds: 90,
      profileIsolation: "per-job",
      outputCommit: "atomic",
      macOSActivationPolicy: "background-only",
      macOSBundleIdentifier: "com.awaken.runtime.libreoffice-renderer",
      macOSLaunchServicesClaims: "none",
      macOSReleaseNotarization: "required",
    });
  });

  it("keeps Linux LibreOffice discovery bounded to the product root", () => {
    const fixture = tempDir();
    const productRoot = join(fixture, "usr", "lib", "libreoffice");
    const soffice = join(productRoot, "program", "soffice");
    writeExecutable(soffice, fakeOkScript());
    assert.equal(resolveLibreOfficeInstallRoot(soffice, "linux"), realpathSync(productRoot));

    const broadRoot = join(fixture, "broad", "usr", "lib");
    const unsafeSoffice = join(broadRoot, "program", "soffice");
    writeExecutable(unsafeSoffice, fakeOkScript());
    assert.equal(resolveLibreOfficeInstallRoot(unsafeSoffice, "linux"), "");
  });
});

describe("managed directory copying", () => {
  it("rejects a destination nested inside the source tree", () => {
    const source = tempDir();
    writeFileSync(join(source, "payload"), "ok");
    assert.throws(
      () => copyDirectoryContents(source, join(source, "copy")),
      /copy target must not be inside its source root/,
    );
  });

  it("preserves relative symlinks whose targets remain inside the source root", () => {
    const source = tempDir();
    const target = tempDir();
    mkdirSync(join(source, "Versions", "1"), { recursive: true });
    writeFileSync(join(source, "Versions", "1", "payload"), "ok");
    symlinkSync("1", join(source, "Versions", "Current"));
    symlinkSync("Versions/Current/payload", join(source, "payload-link"));

    copyDirectoryContents(source, join(target, "copy"));

    const copiedCurrent = join(target, "copy", "Versions", "Current");
    const copiedPayload = join(target, "copy", "payload-link");
    assert.equal(lstatSync(copiedCurrent).isSymbolicLink(), true);
    assert.equal(readlinkSync(copiedCurrent), "1");
    assert.equal(lstatSync(copiedPayload).isSymbolicLink(), true);
    assert.equal(readFileSync(copiedPayload, "utf8"), "ok");
  });

  it("rejects broken and source-root escaping symlinks", () => {
    const brokenSource = tempDir();
    symlinkSync("missing", join(brokenSource, "broken"));
    assert.throws(
      () => copyDirectoryContents(brokenSource, join(tempDir(), "copy")),
      /broken or cyclic symlink/,
    );

    const escapingSource = tempDir();
    const outside = join(tempDir(), "outside");
    writeFileSync(outside, "outside");
    symlinkSync(relative(escapingSource, outside), join(escapingSource, "escape"));
    assert.throws(
      () => copyDirectoryContents(escapingSource, join(tempDir(), "copy")),
      /external symlink is forbidden/,
    );
  });

  it("rejects absolute symlinks even when the target is inside the source root", () => {
    const source = tempDir();
    const payload = join(source, "payload");
    writeFileSync(payload, "ok");
    symlinkSync(payload, join(source, "absolute"));
    assert.throws(
      () => copyDirectoryContents(source, join(tempDir(), "copy")),
      /absolute symlink is forbidden/,
    );
  });
});

describe("macOS LibreOffice staging policy", () => {
  it("sets background-only and completes an ad-hoc inside-out signature", {
    skip: process.platform !== "darwin",
  }, () => {
    const stagingRoot = tempDir();
    const appRoot = join(stagingRoot, "LibreOffice.app");
    writeMinimalMacOSApp(appRoot);

    const background = setMacOSBackgroundOnlyApplication(appRoot, { ownershipRoot: stagingRoot });
    assert.equal(background.activationPolicy, "background-only");
    assert.equal(background.bundleIdentifier, "com.awaken.runtime.libreoffice-renderer");
    const plist = readFileSync(join(appRoot, "Contents", "Info.plist"), "utf8");
    assert.match(plist, /LSBackgroundOnly/);
    assert.match(plist, /com\.awaken\.runtime\.libreoffice-renderer/);
    assert.doesNotMatch(plist, /LSUIElement/);
    assert.doesNotMatch(plist, /CFBundleDocumentTypes|CFBundleURLTypes|NSServices|UTExportedTypeDeclarations|UTImportedTypeDeclarations/);
    assert.equal(existsSync(join(appRoot, "Contents", "PlugIns", "QuickLookPreview.appex")), false);
    assert.equal(existsSync(join(appRoot, "Contents", "PlugIns", "QuickLookThumbnail.appex")), false);
    assert.equal(existsSync(join(appRoot, "Contents", "Library", "Spotlight", "OOoSpotlightImporter.mdimporter")), false);

    const cacheDirectory = join(appRoot, "Contents", "Resources", "python", "__pycache__");
    mkdirSync(cacheDirectory, { recursive: true });
    const cacheFile = join(cacheDirectory, "runtime.pyc");
    writeFileSync(cacheFile, "sealed");
    const signing = signMacOSRuntimeApplication(appRoot, { ownershipRoot: stagingRoot });
    assert.equal(signing.mode, "ad-hoc");
    assert.equal(signing.strictVerified, true);
    assert.equal(signing.gatekeeperVerified, false);
    const immutable = sealMacOSRuntimeApplicationPermissions(appRoot, {
      ownershipRoot: stagingRoot,
    });
    assert.equal(immutable.sealed, true);
    assert.equal(lstatSync(cacheDirectory).mode & 0o222, 0);
    assert.equal(lstatSync(cacheFile).mode & 0o222, 0);
    assert.throws(
      () => verifyMacOSLibreOfficeReleaseSeed(appRoot),
      /unexpected bundle identifier/,
    );
  });

  it("requires an explicit non-ad-hoc identity for release signing", {
    skip: process.platform !== "darwin",
  }, () => {
    const stagingRoot = tempDir();
    const appRoot = join(stagingRoot, "LibreOffice.app");
    writeMinimalMacOSApp(appRoot);
    assert.throws(
      () => signMacOSRuntimeApplication(appRoot, {
        ownershipRoot: stagingRoot,
        release: true,
        identity: "-",
      }),
      /requires AWAKEN_RUNTIME_CODESIGN_IDENTITY/,
    );
  });

  it("refuses to mutate the source application itself", {
    skip: process.platform !== "darwin",
  }, () => {
    const appRoot = join(tempDir(), "LibreOffice.app");
    writeMinimalMacOSApp(appRoot);
    assert.throws(
      () => setMacOSBackgroundOnlyApplication(appRoot, {
        ownershipRoot: dirname(appRoot),
        sourceRoot: appRoot,
      }),
      /refusing to mutate the LibreOffice source application/,
    );
  });
});

describe("probe and readiness", () => {
  it("marks wrapper with deleted target as not ready", () => {
    const root = tempDir();
    const { versionRoot, manifest, python } = writeFakeRuntimeTree(root);
    const missingTarget = join(versionRoot, "gone-python");
    writeExecutable(missingTarget, fakeOkScript());
    writeExecWrapper(python, missingTarget, "python");
    rmSync(missingTarget, { force: true });
    const component = manifest.components.find((c) => c.name === "python");
    const status = probeComponent(component, python, root);
    assert.equal(status.ok, false);
    assert.match(status.detail, /wrapper target missing|path missing/);
  });

  it("marks the full active runtime not ready when a wrapper target becomes stale", () => {
    const root = tempDir();
    const { versionRoot, manifest, python } = writeFakeRuntimeTree(root);
    const target = join(versionRoot, "python", "bin", "python-real");
    writeExecutable(target, fakeOkScript("managed python"));
    writeExecWrapper(python, target, "python");
    rmSync(target, { force: true });

    const report = checkPrimaryRuntime({ root, platform: "darwin", arch: "arm64", manifest });
    assert.equal(report.ready, false);
    assert.ok(report.missing.includes("python"));
    assert.equal(report.active, "0.2.0/darwin-arm64");
    const pythonStatus = report.components.find((component) => component.name === "python");
    assert.equal(pythonStatus?.ok, false);
    assert.equal(pythonStatus?.detail, "wrapper target missing");
    assert.equal(pythonStatus?.resolvedPath, target);
  });

  it("marks executable exit nonzero as not ready", () => {
    const root = tempDir();
    const { python, manifest } = writeFakeRuntimeTree(root, { failPython: true });
    const component = manifest.components.find((c) => c.name === "python");
    const status = probeComponent(component, python, root);
    assert.equal(status.ok, false);
  });

  it("rejects wrapper that escapes awaken root", () => {
    const root = tempDir();
    const outside = tempDir();
    const outsideBin = join(outside, "python");
    writeExecutable(outsideBin, fakeOkScript());
    const { python, manifest } = writeFakeRuntimeTree(root);
    writeExecWrapper(python, outsideBin, "python");
    const component = manifest.components.find((c) => c.name === "python");
    const status = probeComponent(component, python, root);
    assert.equal(status.ok, false);
    assert.match(status.detail, /escapes awaken root/);
  });

  it("source deleted but managed copy still works", () => {
    const root = tempDir();
    const seed = tempDir();
    writeExecutable(join(seed, "python3"), fakeOkScript());
    const { versionRoot, manifest } = writeFakeRuntimeTree(root);
    const managedPython = join(versionRoot, "python", "bin", "python3");
    rmSync(seed, { recursive: true, force: true });
    const component = manifest.components.find((c) => c.name === "python");
    const status = probeComponent(component, managedPython, root);
    assert.equal(status.ok, true);
    assert.ok(pathInsideRoot(status.resolvedPath, root));
  });

  it("rejects a Python executable whose base_prefix escapes the runtime", () => {
    const root = tempDir();
    const python = join(root, "python", "bin", "python3");
    writeExecutable(python, `#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *"json,sys"* ]]; then
  echo '{"executable":"/outside/python","prefix":"/outside","base_prefix":"/outside"}'
fi
exit 0
`);
    const component = {
      name: "python",
      kind: "executable",
      required: true,
      probe: { args: ["-c", "pass"], acceptedExitCodes: [0] },
    };
    const status = probeComponent(component, python, root);
    assert.equal(status.ok, false);
    assert.match(status.detail, /base_prefix escapes/);
  });

  it("rejects pyvenv.cfg and known external build-machine prefixes", () => {
    const root = tempDir();
    mkdirSync(join(root, "python"), { recursive: true });
    writeFileSync(join(root, "python", "pyvenv.cfg"), "home = /opt/homebrew/bin\n");
    const provenance = inspectRuntimeProvenance(root, root, "darwin");
    assert.equal(provenance.ok, false);
    assert.ok(provenance.issues.some((issue) => issue.code === "python_venv_not_relocatable"));
    assert.equal(isForbiddenExternalPath("/opt/homebrew/lib/libfoo.dylib"), true);
    assert.equal(isForbiddenExternalPath("/usr/local/lib/libfoo.dylib"), true);
    assert.equal(isForbiddenExternalPath("/usr/lib/libSystem.B.dylib"), false);
  });

  it("detects a real Homebrew Mach-O external dependency on macOS", (t) => {
    if (process.platform !== "darwin") {
      t.skip("macOS-only provenance fixture");
      return;
    }
    const dependencies = machODependencies(process.execPath);
    if (!dependencies.some(isForbiddenExternalPath)) {
      t.skip("current Node distribution is already self-contained");
      return;
    }
    const root = tempDir();
    const copiedNode = join(root, "node", "bin", "node");
    mkdirSync(dirname(copiedNode), { recursive: true });
    cpSync(process.execPath, copiedNode);
    const provenance = inspectRuntimeProvenance(root, root, "darwin");
    assert.equal(provenance.ok, false);
    assert.ok(provenance.issues.some((issue) => issue.code === "forbidden_external_dependency"));
  });
});

describe("stage publish rollback", () => {
  it("prepare coordinator gates publish and activation behind all staging probes", () => {
    const source = readFileSync(join(repoRoot, "scripts/prepare-primary-runtime.mjs"), "utf8");
    const probeIndex = source.indexOf("const status = probeComponent(component, absolutePath, stagingDir)");
    const rejectIndex = source.indexOf("throw new Error(`staged probe failed:");
    const provenanceIndex = source.indexOf("inspectRuntimeProvenance(stagingDir, stagingDir");
    const publishIndex = source.indexOf("const versionRoot = publishStaging(root, stagingDir, key)");
    const activateIndex = source.indexOf("activateVersion(root, key)");
    assert.ok(probeIndex >= 0, "prepare must probe staged components");
    assert.ok(rejectIndex > probeIndex, "prepare must reject failed staged probes");
    assert.ok(provenanceIndex > rejectIndex, "provenance must be checked after component probes");
    assert.ok(publishIndex > provenanceIndex, "publish must occur only after all staging validation");
    assert.ok(activateIndex > publishIndex, "current activation must occur only after immutable publish");
  });

  it("stage mid-failure does not destroy current", () => {
    const root = tempDir();
    writeFakeRuntimeTree(root);
    const before = readCurrentJson(root);
    const staging = createStagingDir(root, "fail");
    writeFileSync(join(staging, "partial.txt"), "x");
    cleanupStaging(staging);
    const after = readCurrentJson(root);
    assert.equal(after.active, before.active);
    assert.equal(checkPrimaryRuntime({
      root,
      platform: "darwin",
      arch: "arm64",
      manifest: baseManifest(),
    }).ready, true);
  });

  it("failed full staging probe leaves current on the previous known-good runtime", () => {
    const root = tempDir();
    const { versionRoot, manifest } = writeFakeRuntimeTree(root);
    const beforeCurrent = readCurrentJson(root);
    const beforeReport = checkPrimaryRuntime({ root, platform: "darwin", arch: "arm64", manifest });
    assert.equal(beforeReport.ready, true);

    const staging = createStagingDir(root, "probe-failure");
    for (const name of ["python", "node", "libreoffice", "poppler", "ffmpeg", "packages"]) {
      cpSync(join(versionRoot, name), join(staging, name), { recursive: true });
    }
    writeExecutable(join(staging, "python", "bin", "python3"), fakeFailScript());

    const statuses = manifest.components.map((component) => probeComponent(
      component,
      join(staging, component.relativePath),
      staging,
    ));
    assert.equal(statuses.every((status) => status.ok), false);
    const pythonStatus = statuses.find((status) => status.name === "python");
    assert.equal(pythonStatus?.ok, false);
    assert.equal(pythonStatus?.detail, "python probe failed");

    // Publication/activation is gated on every required staging probe.
    cleanupStaging(staging);
    assert.equal(existsSync(staging), false);
    assert.deepEqual(readCurrentJson(root), beforeCurrent);
    const afterReport = checkPrimaryRuntime({ root, platform: "darwin", arch: "arm64", manifest });
    assert.equal(afterReport.ready, true);
    assert.equal(afterReport.active, beforeReport.active);
  });

  it("known-good rollback restores previous active", () => {
    const root = tempDir();
    writeFakeRuntimeTree(root);
    const good = "0.2.0/darwin-arm64";
    const staging = createStagingDir(root, "bad");
    const badKey = "0.2.1/darwin-arm64";
    mkdirSync(join(staging, "empty"), { recursive: true });
    publishStaging(root, staging, badKey);
    activateVersion(root, badKey);
    let current = readCurrentJson(root);
    assert.equal(current.active, badKey);
    assert.equal(current.previous, good);
    const rolled = rollbackToPrevious(root);
    assert.equal(rolled.ok, true);
    current = readCurrentJson(root);
    assert.equal(current.active, good);
  });

  it("publishes same-bundle repairs under immutable unique revisions", () => {
    const root = tempDir();
    const firstKey = versionKey("0.2.0", "darwin", "arm64", createBuildRevision(1000, "first"));
    const secondKey = versionKey("0.2.0", "darwin", "arm64", createBuildRevision(1001, "second"));
    assert.notEqual(firstKey, secondKey);

    const firstStage = createStagingDir(root, "first");
    writeFileSync(join(firstStage, "marker"), "first");
    publishStaging(root, firstStage, firstKey);
    activateVersion(root, firstKey);

    const secondStage = createStagingDir(root, "second");
    writeFileSync(join(secondStage, "marker"), "second");
    publishStaging(root, secondStage, secondKey);
    activateVersion(root, secondKey);

    let current = readCurrentJson(root);
    assert.equal(current.active, secondKey);
    assert.equal(current.previous, firstKey);
    assert.equal(readFileSync(join(versionDir(root, firstKey), "marker"), "utf8"), "first");
    assert.equal(readFileSync(join(versionDir(root, secondKey), "marker"), "utf8"), "second");
    assert.equal(rollbackToPrevious(root).ok, true);
    current = readCurrentJson(root);
    assert.equal(current.active, firstKey);
  });

  it("refuses to overwrite an existing immutable version key", () => {
    const root = tempDir();
    const key = versionKey("0.2.0", "darwin", "arm64", "fixed-revision");
    const first = createStagingDir(root, "first");
    writeFileSync(join(first, "marker"), "original");
    publishStaging(root, first, key);
    const second = createStagingDir(root, "second");
    writeFileSync(join(second, "marker"), "replacement");
    assert.throws(() => publishStaging(root, second, key), /already exists/);
    assert.equal(readFileSync(join(versionDir(root, key), "marker"), "utf8"), "original");
  });

  it("current.json uses atomic file not directory symlink", () => {
    const root = tempDir();
    writeCurrentJson(root, { active: "0.2.0/darwin-arm64", previous: "0.1.0/darwin-arm64" });
    const parsed = JSON.parse(readFileSync(join(root, "current.json"), "utf8"));
    assert.equal(parsed.schemaVersion, 2);
    assert.equal(parsed.active, "0.2.0/darwin-arm64");
    assert.equal(parsed.previous, "0.1.0/darwin-arm64");
    assert.equal(resolveManagedPath(join(root, "current"), root).ok, false);
  });
});

describe("release runtime archive contract", () => {
  it("packages only fully signed and notarized revisions with a digest manifest", () => {
    const source = readFileSync(
      join(repoRoot, "scripts/package-primary-runtime-archive.mjs"),
      "utf8",
    );
    const platformSource = readFileSync(
      join(repoRoot, "scripts/runtime-archive-platform.mjs"),
      "utf8",
    );
    assert.match(source, /mode !== "developer-id"/);
    assert.match(source, /notarized !== true/);
    assert.match(source, /stapled !== true/);
    assert.match(source, /codesign/);
    assert.match(source, /stapler/);
    assert.match(source, /spctl/);
    assert.match(source, /sha256/);
    assert.match(source, /designatedRequirement/);
    assert.match(source, /teamIdentifier/);
    assert.match(platformSource, /--keepParent/);
  });

  it("uses platform-native archive commands and a shared SHA-256 contract", () => {
    assert.equal(runtimeArchivePlatform("win32"), "windows");
    assert.equal(runtimeArchivePlatform("darwin"), "darwin");
    assert.equal(runtimeOfficeExecutableRelativePath("win32"), "program/soffice.com");
    assert.equal(runtimeOfficeExecutableRelativePath("darwin"), "Contents/MacOS/soffice");

    const macCreate = createRuntimeArchiveInvocation(
      "darwin",
      "/runtime/revision",
      "/output/runtime.zip",
    );
    assert.equal(macCreate.command, "ditto");
    assert.ok(macCreate.args.includes("--keepParent"));

    const windowsCreate = createRuntimeArchiveInvocation(
      "win32",
      "C:\\runtime\\revision",
      "C:\\output\\runtime.zip",
    );
    const windowsExtract = extractRuntimeArchiveInvocation(
      "win32",
      "C:\\output\\runtime.zip",
      "C:\\staging",
    );
    assert.equal(windowsCreate.command, "powershell.exe");
    assert.equal(windowsExtract.command, "powershell.exe");
    assert.match(windowsCreate.args.join(" "), /Compress-Archive/);
    assert.match(windowsCreate.args.join(" "), /hidden entries/);
    assert.match(windowsExtract.args.join(" "), /Expand-Archive/);
    const quoted = createRuntimeArchiveInvocation(
      "win32",
      "C:\\Nolan's runtime\\revision",
      "C:\\output\\runtime.zip",
    );
    assert.match(quoted.args.join(" "), /Nolan''s runtime/);
    assert.doesNotMatch(
      `${windowsCreate.command} ${windowsCreate.args.join(" ")} ${windowsExtract.command} ${windowsExtract.args.join(" ")}`,
      /codesign|xcrun|spctl|ditto/,
    );

    const integrity = runtimeArchiveIntegrityManifest();
    assert.equal(hasRuntimeArchiveIntegrity({ integrity }), true);
    assert.equal(hasRuntimeArchiveIntegrity({}), false);
  });

  it("installs without signing and activates only after archive and platform verification", () => {
    const source = readFileSync(
      join(repoRoot, "scripts/install-primary-runtime-archive.mjs"),
      "utf8",
    );
    assert.match(source, /SHA-256/);
    assert.match(source, /validateExtractedTree/);
    assert.match(source, /codesign/);
    assert.match(source, /stapler/);
    assert.match(source, /spctl/);
    assert.match(source, /expectedTeamIdentifier/);
    assert.match(source, /expectedPlatform === "darwin"/);
    assert.match(source, /expectedPlatform !== "darwin" && expectedPlatform !== "windows"/);
    assert.match(source, /com\.awaken\.runtime\.libreoffice-renderer/);
    assert.match(source, /renameSync\(extracted, target\)/);
    assert.match(source, /writeCurrentJson/);
    assert.doesNotMatch(source, /codesign[^\n]*--sign/);
    assert.ok(
      source.indexOf("renameSync(extracted, target)") >
        source.indexOf("runtime designated requirement does not match"),
      "immutable publish must happen after signature verification",
    );
  });

  it("ordinary Windows Desktop packages the managed runtime without using the macOS release path", () => {
    const source = readFileSync(
      join(repoRoot, "scripts/package-primary-runtime-for-desktop.mjs"),
      "utf8",
    );
    assert.match(source, /process\.platform !== "win32"/);
    assert.match(source, /package-primary-runtime-archive\.mjs/);
    assert.doesNotMatch(source, /codesign|xcrun|spctl|ditto/);
  });
});

describe("migration", () => {
  it("migrates old awaken cache successfully once", () => {
    const userData = tempDir();
    const home = tempDir();
    const oldCache = join(home, ".cache", "awaken-runtimes", "primary-runtime", "dependencies");
    const fakeRoot = tempDir();
    const { versionRoot } = writeFakeRuntimeTree(fakeRoot);
    mkdirSync(oldCache, { recursive: true });
    for (const name of ["python", "node", "libreoffice", "poppler", "ffmpeg", "packages"]) {
      cpSync(join(versionRoot, name), join(oldCache, name), { recursive: true });
    }

    const root = join(userData, "runtimes", "primary-runtime");
    const env = { AWAKEN_USER_DATA_DIR: userData, HOME: home };
    const manifest = JSON.parse(readFileSync(join(repoRoot, "apps/desktop/resources/runtime-manifest/primary-runtime.json"), "utf8"));
    // Use pass probe for python in migration tests (fake scripts)
    for (const component of manifest.components) {
      if (component.name === "python") {
        component.probe = { args: ["-c", "pass"], acceptedExitCodes: [0], requireManagedPrefix: false };
      }
    }
    const result = migrateOldAwakenCacheIfNeeded({
      env,
      platform: "darwin",
      arch: "arm64",
      root,
      manifest,
    });
    assert.equal(result.migrated, true);
    const marker = readMigrationMarker(env, "darwin");
    assert.equal(marker.status, "success");

    const again = migrateOldAwakenCacheIfNeeded({ env, platform: "darwin", arch: "arm64", root, manifest });
    assert.equal(again.skipped, true);
  });

  it("rejects bad wrapper during migration", () => {
    const userData = tempDir();
    const home = tempDir();
    const oldCache = join(home, ".cache", "awaken-runtimes", "primary-runtime", "dependencies");
    mkdirSync(join(oldCache, "python", "bin"), { recursive: true });
    const outside = join(home, "outside-python");
    writeExecutable(outside, fakeOkScript());
    writeExecWrapper(join(oldCache, "python", "bin", "python3"), outside, "python");
    const root = join(userData, "runtimes", "primary-runtime");
    const env = { AWAKEN_USER_DATA_DIR: userData, HOME: home };
    const manifest = {
      schemaVersion: 2,
      runtimeName: "primary-runtime",
      bundleVersion: "0.2.0",
      components: [
        { name: "python", kind: "executable", required: true, relativePath: "python/bin/python3", probe: { args: ["--version"], acceptedExitCodes: [0] } },
      ],
    };
    const result = migrateOldAwakenCacheIfNeeded({ env, platform: "darwin", arch: "arm64", root, manifest });
    assert.equal(result.migrated, false);
    assert.equal(result.failed, true);
    assert.ok(result.rejected?.some((item) => item.name === "python"));
    assert.equal(readMigrationMarker(env, "darwin").status, "failed");
  });

  it("does not migrate codex cache", () => {
    const userData = tempDir();
    const home = tempDir();
    const codex = join(home, ".cache", "codex-runtimes", "codex-primary-runtime", "dependencies");
    mkdirSync(codex, { recursive: true });
    writeExecutable(join(codex, "bin", "soffice"), fakeOkScript());
    const root = join(userData, "runtimes", "primary-runtime");
    const env = { AWAKEN_USER_DATA_DIR: userData, HOME: home };
    const result = migrateOldAwakenCacheIfNeeded({
      env,
      platform: "darwin",
      arch: "arm64",
      root,
      manifest: { schemaVersion: 2, bundleVersion: "0.2.0", components: [] },
    });
    assert.equal(result.migrated, false);
    assert.ok(result.skipped);
  });
});
