import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { parse } from "yaml";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

function readWorkflow(name) {
  return parse(readFileSync(join(repoRoot, ".github/workflows", name), "utf8"));
}

function runs(job) {
  return (job?.steps ?? [])
    .map((step) => step.run ?? step.with?.script ?? "")
    .join("\n");
}

describe("GitHub CI", () => {
  const ci = readWorkflow("ci.yml");
  const release = readWorkflow("desktop-release.yml");
  const desktopPackage = JSON.parse(readFileSync(join(repoRoot, "apps/desktop/package.json"), "utf8"));

  it("checks main pushes and pull requests without using another releases repository", () => {
    assert.deepEqual(ci.on.push.branches, ["main"]);
    assert.deepEqual(ci.on.pull_request.types, [
      "opened",
      "synchronize",
      "reopened",
      "ready_for_review",
    ]);
    assert.equal(ci.on.push.tags, undefined);
    const text = JSON.stringify(ci);
    assert.doesNotMatch(text, /dist:desktop:release|AWAKEN_RELEASES_TOKEN|APPLE_API_KEY|Awaken-Releases/);
  });

  it("runs unit tests before acceptance and both client packages", () => {
    for (const name of ["unit", "acceptance", "package-macos", "package-windows", "ci-gate"]) {
      assert.ok(ci.jobs[name], `missing job ${name}`);
    }
    assert.deepEqual(ci.jobs.acceptance.needs, ["unit"]);
    assert.deepEqual(ci.jobs["package-macos"].needs, ["unit"]);
    assert.deepEqual(ci.jobs["package-windows"].needs, ["unit"]);
    assert.deepEqual(ci.jobs["ci-gate"].needs, [
      "unit",
      "acceptance",
      "package-macos",
      "package-windows",
      "publish",
    ]);
    assert.match(runs(ci.jobs["ci-gate"]), /result !== 'success'/);
    assert.match(runs(ci.jobs["ci-gate"]), /shouldPublish/);
  });

  it("keeps the unit job on repository tests rather than a packaged installer", () => {
    const script = runs(ci.jobs.unit);
    assert.match(script, /typecheck:desktop/);
    assert.match(script, /@awaken\/desktop run test/);
    assert.match(script, /xvfb xauth/);
    assert.match(script, /kernel\.apparmor_restrict_unprivileged_userns=0/);
    assert.match(script, /xvfb-run -a -s '-screen 0 1280x900x24' npm run test:scripts/);
    assert.match(script, /node node_modules\/electron\/install\.js/);
    assert.doesNotMatch(script, /chmod 4755/);
    const composer = readFileSync(join(repoRoot, "scripts/tests/composer-layout.test.mjs"), "utf8");
    assert.match(composer, /--no-sandbox/);
    assert.match(composer, /--disable-dev-shm-usage/);
    assert.match(composer, /--ozone-platform=x11/);
    assert.doesNotMatch(composer, /DBUS_SESSION_BUS_ADDRESS/);
    assert.match(script, /python -m pip install --disable-pip-version-check 'lxml==6\.1\.3' 'openpyxl==3\.1\.5'/);
    assert.match(script, /test:python/);
    assert.match(script, /check:builtin-plugins/);
    assert.match(script, /check-docs\.mjs/);
    assert.match(script, /check_provider_docs\.py/);
    assert.match(script, /prepare-cua-driver\.mjs/);
    assert.match(script, /go test -count=1 \.\/\.\.\./);
    assert.equal(ci.jobs.unit["runs-on"], "ubuntu-latest");
  });

  it("runs the Electron functional acceptance on macOS and Windows", () => {
    assert.deepEqual(ci.jobs.acceptance.strategy.matrix.os, ["macos-latest", "windows-latest"]);
    assert.match(runs(ci.jobs.acceptance), /test:mermaid-electron/);
  });

  it("builds ad-hoc macOS and unsigned Windows packages, then audits and smokes them", () => {
    for (const name of ["package-macos", "package-windows"]) {
      const script = runs(ci.jobs[name]);
      assert.match(script, /dist:desktop:adhoc/);
      assert.match(script, /audit-package-contents\.mjs/);
      assert.match(script, /smoke:packaged-desktop/);
      assert.equal(ci.jobs[name].env.CSC_IDENTITY_AUTO_DISCOVERY, "false");
    }
    assert.match(runs(ci.jobs["package-macos"]), /codesign --verify --deep --strict/);
    assert.equal(ci.jobs["package-macos"]["runs-on"], "macos-latest");
    assert.equal(ci.jobs["package-windows"]["runs-on"], "windows-latest");
    assert.match(desktopPackage.scripts["dist:adhoc"], /--publish never/);
    assert.match(desktopPackage.scripts.dist, /--publish never/);
  });

  it("publishes main installers to a prerelease in this repository", () => {
    const publish = ci.jobs.publish;
    assert.deepEqual(publish.needs, ["package-macos", "package-windows"]);
    assert.equal(publish.permissions.contents, "write");
    const script = runs(publish);
    const publishStep = publish.steps.find((step) => (step.run ?? "").includes("gh release create"));
    assert.match(script, /tag="ci-\$\{GITHUB_SHA\}"/);
    assert.match(script, /gh release create "\$tag"/);
    assert.match(script, /--prerelease/);
    assert.match(script, /--latest=false/);
    assert.equal(publishStep.env.GH_TOKEN, "${{ github.token }}");
  });

  it("still blocks the signed release workflow until the installed-app upgrade smoke exists", () => {
    const smoke = runs(release.jobs["upgrade-smoke"]);
    assert.match(smoke, /Refusing to publish/);
    assert.match(smoke, /exit 1/);
  });
});
