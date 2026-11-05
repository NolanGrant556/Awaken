import { afterEach, describe, expect, it, vi } from "vitest";
import { createHash } from "node:crypto";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { UnsignedDesktopUpdater, parseReleaseCandidate, releaseChecksum, unsignedAssetName } from "./unsigned-desktop-updater.js";
import { isUnsignedDesktopUpdateIdentity } from "../shared/desktop-update.js";

const roots: string[] = [];
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });
function release(version = "0.1.2") {
  const name = unsignedAssetName(version, "darwin-arm64");
  const asset = (name: string, size: number) => ({ name, size, browser_download_url: `https://github.com/NolanGrant556/Awaken/releases/download/v${version}/${name}` });
  return { draft: false, prerelease: false, tag_name: `v${version}`, name: `Awaken ${version}`,
    assets: [asset(name, 3), asset("SHA256SUMS.txt", 100)] };
}
async function fixture(body = "zip", checksum = createHash("sha256").update(body).digest("hex")) {
  const root = await mkdtemp(join(tmpdir(), "awaken-unsigned-test-")); roots.push(root);
  const prepare = vi.fn(async () => undefined); const install = vi.fn(async () => undefined);
  const fetch = vi.fn(async (url: string) => {
    if (url.endsWith("/latest")) return Response.json(release());
    if (url.endsWith("SHA256SUMS.txt")) return new Response(`${checksum}  Awaken-0.1.2-arm64.zip\n`);
    return new Response(body);
  });
  const updater = new UnsignedDesktopUpdater({ currentVersion: "0.1.1", target: "darwin-arm64", cacheRoot: root, fetch, prepare, install });
  return { updater, prepare, install, fetch, root };
}
describe("unsigned release contract", () => {
  it("requires explicit packaged unsigned identity without enabling old disabled clients", () => {
    const identity = { packaged: true, devServer: false, channel: "unsigned", version: "0.1.2", platform: "darwin", arch: "arm64" };
    expect(isUnsignedDesktopUpdateIdentity(identity)).toBe(true);
    for (const next of [{ channel: "disabled" }, { channel: "stable" }, { packaged: false }, { devServer: true }, { arch: "ia32" }]) {
      expect(isUnsignedDesktopUpdateIdentity({ ...identity, ...next })).toBe(false);
    }
  });
  it("rejects prereleases, foreign assets, duplicate files and ambiguous checksums", () => {
    expect(() => parseReleaseCandidate({ ...release(), prerelease: true }, "darwin-arm64")).toThrow();
    const foreign = release(); foreign.assets[0].browser_download_url = "https://evil.example/payload.zip";
    expect(() => parseReleaseCandidate(foreign, "darwin-arm64")).toThrow();
    const duplicate = release(); duplicate.assets.push(duplicate.assets[0]);
    expect(() => parseReleaseCandidate(duplicate, "darwin-arm64")).toThrow();
    expect(() => releaseChecksum(`${"a".repeat(64)}  x\n${"b".repeat(64)}  x`, "x")).toThrow();
  });
  it("emits available and downloaded only after integrity and staging succeed", async () => {
    const f = await fixture(); const events: string[] = [];
    f.updater.on("update-available", () => events.push("available"));
    f.updater.on("update-downloaded", () => events.push("downloaded"));
    await f.updater.checkForUpdates(); await f.updater.downloadUpdate();
    expect(events).toEqual(["available", "downloaded"]); expect(f.prepare).toHaveBeenCalledOnce();
    await f.updater.quitAndInstall(); expect(f.install).toHaveBeenCalledOnce();
  });
  it("never stages or installs corrupted bytes, or bytes changed after staging", async () => {
    const corrupt = await fixture("bad", "0".repeat(64)); await corrupt.updater.checkForUpdates();
    await expect(corrupt.updater.downloadUpdate()).rejects.toThrow(/checksum/);
    expect(corrupt.prepare).not.toHaveBeenCalled();
    await expect(corrupt.updater.quitAndInstall()).rejects.toThrow(); expect(corrupt.install).not.toHaveBeenCalled();
    const changed = await fixture(); await changed.updater.checkForUpdates(); await changed.updater.downloadUpdate();
    const file = changed.prepare.mock.calls[0] as unknown as [string];
    await writeFile(file[0], "tampered");
    await expect(changed.updater.quitAndInstall()).rejects.toThrow(/checksum/);
    expect(changed.install).not.toHaveBeenCalled();
  });
  it("does not send package downloads to a redirect outside GitHub", async () => {
    const f = await fixture(); await f.updater.checkForUpdates();
    f.fetch.mockImplementation(async () => new Response(null, { status: 302, headers: { location: "https://evil.example/package" } }));
    await expect(f.updater.downloadUpdate()).rejects.toThrow(/origin/);
    expect(f.prepare).not.toHaveBeenCalled();
  });
});
