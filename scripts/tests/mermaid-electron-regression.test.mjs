import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
  assertLoopbackHttpOrigin,
  MERMAID_ELECTRON_FIXTURE,
  parseArgs,
  parseTranslate,
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
