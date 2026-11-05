import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import ts from "typescript";

const require = createRequire(import.meta.url);
test("native Electron exposes manual redirects without following or cancelling the caller", { timeout: 45_000 }, () => {
  const root = mkdtempSync(join(tmpdir(), "awaken-electron-update-fetch-"));
  try {
    const home = join(root, "home"); mkdirSync(home);
    const source = readFileSync(new URL("../../apps/desktop/src/main/electron-update-fetch.ts", import.meta.url), "utf8");
    writeFileSync(join(root, "fetch.cjs"), ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText);
    writeFileSync(join(root, "probe.cjs"), `
      const { app, net } = require('electron');
      const { createServer } = require('node:http');
      const assert = require('node:assert/strict');
      const { electronUpdateFetch } = require('./fetch.cjs');
      app.setPath('home', ${JSON.stringify(home)});
      app.setPath('userData', ${JSON.stringify(join(root, "userData"))});
      app.whenReady().then(async () => {
        let finalRequests = 0;
        const server = createServer((req, res) => {
          if (req.url === '/redirect') { res.writeHead(302, { location: '/final' }); res.end(); }
          else { finalRequests++; res.end('native-update-download'); }
        });
        await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
        const url = 'http://127.0.0.1:' + server.address().port;
        try {
          const fetch = (url, init) => electronUpdateFetch(options => net.request(options), url, init);
          const response = await fetch(url + '/redirect', { redirect: 'manual' });
          assert.equal(response.status, 302);
          assert.equal(finalRequests, 0);
          const followed = await fetch(new URL(response.headers.get('location'), url).href);
          assert.equal(await followed.text(), 'native-update-download');
          assert.equal(finalRequests, 1);
          const abort = new AbortController(); abort.abort(new Error('test abort'));
          await assert.rejects(fetch(url + '/final', { signal: abort.signal }), /test abort/);
          console.log('native_update_redirect_passed');
          server.closeAllConnections(); server.close(); app.exit(0);
        } catch (error) {
          console.error(error); server.closeAllConnections(); server.close(); app.exit(1);
        }
      }).catch(error => { console.error(error); app.exit(1); });
    `);
    const env = { ...process.env, HOME: home, CFFIXED_USER_HOME: home, APPDATA: join(home, "AppData"), USERPROFILE: home };
    delete env.ELECTRON_RUN_AS_NODE;
    const result = spawnSync(require("electron"), ["--no-sandbox", "--disable-gpu", join(root, "probe.cjs")], { env, encoding: "utf8", timeout: 35_000 });
    assert.equal(result.status, 0, result.stderr || result.error?.message);
    assert.match(result.stdout, /native_update_redirect_passed/);
  } finally { rmSync(root, { recursive: true, force: true }); }
});
