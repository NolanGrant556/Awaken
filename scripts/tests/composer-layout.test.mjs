import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { build } from "vite";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const require = createRequire(import.meta.url);

test("Composer text, restart notice and clipped previews in isolated Electron", { timeout: 60000 }, async (t) => {
  const temp = await mkdtemp(join(tmpdir(), "awaken-composer-layout-"));
  let child;
  try {
    const baseline = process.env.AWAKEN_LAYOUT_BASELINE;
    await build({
      configFile: false,
      root: repo,
      logLevel: "error",
      define: { "process.env.NODE_ENV": JSON.stringify("production") },
      plugins: baseline ? [{
        name: "review-baseline",
        enforce: "pre",
        load(id) {
          if (!id.startsWith(repo + "/") || !/\.(tsx|css)$/.test(id)) return;
          const relative = id.slice(repo.length + 1);
          if (!relative.startsWith("Awaken Design System/") && !relative.startsWith("apps/desktop/")) return;
          try { return execFileSync("git", ["show", `${baseline}:${relative}`], { cwd: repo, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }); }
          catch { return; }
        },
      }] : [],
      build: {
        outDir: temp,
        emptyOutDir: false,
        lib: { entry: join(repo, "scripts/test-fixtures/composer-layout/fixture.tsx"), name: "ComposerLayoutFixture", formats: ["iife"], fileName: () => "fixture.js", cssFileName: "fixture" },
      },
      esbuild: { jsx: "automatic" },
    });
    await writeFile(join(temp, "index.html"), '<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="fixture.css"></head><body><div id="root"></div><script src="fixture.js"></script></body></html>');
    // A separate process and temporary userData prevent contact with real Desktop/Core data.
    await writeFile(join(temp, "main.cjs"), `
const { app, BrowserWindow } = require('electron');
const fs = require('node:fs');
app.setPath('userData', ${JSON.stringify(join(temp, "userData"))});
app.whenReady().then(async () => {
  const win = new BrowserWindow({ show: false, width: 1000, height: 760, webPreferences: { backgroundThrottling: false, sandbox: true, contextIsolation: true, nodeIntegration: false } });
  win.webContents.on('console-message', (_event, details) => console.error(details.message));
  await win.loadFile(${JSON.stringify(join(temp, "index.html"))});
  const result = await win.webContents.executeJavaScript('window.runComposerLayoutRegression()');
  fs.writeFileSync(${JSON.stringify(join(temp, "results.json"))}, JSON.stringify(result, null, 2));
  app.exit(0);
}).catch(error => { console.error(error); app.exit(1); });
`);
    const env = { ...process.env };
    delete env.ELECTRON_RUN_AS_NODE;
    child = spawn(require("electron"), [join(temp, "main.cjs")], { env, stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    child.stdout.on("data", chunk => { output += chunk; });
    child.stderr.on("data", chunk => { output += chunk; });
    const timer = setTimeout(() => child.kill(), 30000);
    const [code] = await once(child, "exit");
    clearTimeout(timer);
    assert.equal(code, 0, output);
    const results = JSON.parse(await readFile(join(temp, "results.json"), "utf8"));
    for (const result of results) {
      await t.test(result.name, () => assert.equal(result.passed, true, result.error));
    }
    if (process.env.AWAKEN_LAYOUT_RESULTS) {
      await writeFile(process.env.AWAKEN_LAYOUT_RESULTS, JSON.stringify(results, null, 2) + "\n");
    }
  } finally {
    if (child && child.exitCode === null) child.kill();
    await rm(temp, { recursive: true, force: true });
  }
});
