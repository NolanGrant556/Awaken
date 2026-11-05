#!/usr/bin/env node
// Validate the actual update ZIPs with the same installer used by Desktop.
// Does not quit, launch or replace an installed application.
import fs from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

if (process.platform !== "darwin") throw new Error("macOS update preparation requires macOS");
const dir = resolve(process.argv[2] ?? "apps/desktop/dist-electron");
const receipt = resolve(process.argv[3] ?? "smoke-evidence/unsigned-update-prepare.json");
const { version } = JSON.parse(await fs.readFile("apps/desktop/package.json", "utf8"));
const { UnsignedUpdateInstaller } = await import(pathToFileURL(resolve("apps/desktop/dist/main/unsigned-update-installer.js")).href);
const { sha256File } = await import(pathToFileURL(resolve("apps/desktop/dist/main/unsigned-desktop-updater.js")).href);
const root = await fs.mkdtemp(join(tmpdir(), "awaken-unsigned-prepare-"));
const results = [];
try {
  for (const arch of ["arm64", "x64"]) {
    const target = join(root, arch, "Awaken.app");
    await fs.mkdir(target, { recursive: true });
    const installer = new UnsignedUpdateInstaller({
      platform: "darwin", arch, appPath: join(target, "Contents/Resources/app.asar"),
      cacheRoot: join(root, "cache"),
      quit() { throw new Error("preflight must not quit"); },
      async openInstaller() { throw new Error("preflight must not install"); },
    });
    const zip = join(dir, `Awaken-${version}-${arch}.zip`);
    await installer.prepare(zip, version);
    results.push({ arch, file: basename(zip), sha256: await sha256File(zip), prepared: true });
    console.log(`desktop_unsigned_prepare_passed ${arch} ${version}`);
  }
  await fs.mkdir(dirname(receipt), { recursive: true });
  await fs.writeFile(receipt, JSON.stringify({ version, results }, null, 2) + "\n");
} finally { await fs.rm(root, { recursive: true, force: true }); }
