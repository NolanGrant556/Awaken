// Explicit opt-in for installer builds marked awakenUpdateChannel=unsigned.
// HTTPS and release SHA-256 protect transport integrity, not publisher identity.
import { EventEmitter } from "node:events";
import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import fs from "node:fs/promises";
import { join } from "node:path";
import { compareStableSemver, isStableSemver, type DesktopUpdateTarget } from "../shared/desktop-update.js";

export const UNSIGNED_RELEASE_API = "https://api.github.com/repos/NolanGrant556/Awaken/releases/latest";
const RELEASE_ROOT = "https://github.com/NolanGrant556/Awaken/releases/download/";
const MAX_PACKAGE_BYTES = 1024 * 1024 * 1024;
type ReleaseAsset = { name: string; size: number; browser_download_url: string };
type Candidate = { version: string; releaseName: string; releaseDate: string; asset: ReleaseAsset; sha256: string };

export function unsignedAssetName(version: string, target: DesktopUpdateTarget): string {
  if (!isStableSemver(version)) throw new Error("update metadata version invalid");
  return target.startsWith("darwin-")
    ? `Awaken-${version}-${target.slice(7)}.zip`
    : `Awaken-Setup-${version}-x64.exe`;
}

export function parseReleaseCandidate(raw: unknown, target: DesktopUpdateTarget): Omit<Candidate, "sha256"> {
  const release = raw as Record<string, unknown> | null;
  if (!release || release.draft !== false || release.prerelease !== false || typeof release.tag_name !== "string") {
    throw new Error("update metadata must be a published release");
  }
  const version = release.tag_name.replace(/^v/, "");
  if (release.tag_name !== `v${version}` || !isStableSemver(version)) throw new Error("update metadata tag invalid");
  const name = unsignedAssetName(version, target);
  if (!Array.isArray(release.assets)) throw new Error("update metadata assets invalid");
  const assets = release.assets.filter((asset: ReleaseAsset) => asset?.name === name);
  const asset = assets[0] as ReleaseAsset | undefined;
  if (assets.length !== 1 || !asset || !Number.isSafeInteger(asset.size) || asset.size <= 0 || asset.size > MAX_PACKAGE_BYTES ||
      asset.browser_download_url !== `${RELEASE_ROOT}v${version}/${name}`) {
    throw new Error("update artifact missing or metadata invalid");
  }
  return {
    version, asset,
    releaseName: typeof release.name === "string" ? release.name : `Awaken ${version}`,
    releaseDate: typeof release.published_at === "string" ? release.published_at : "",
  };
}

export function releaseChecksum(text: string, filename: string): string {
  const lines = text.split(/\r?\n/).filter(Boolean);
  const matches: string[] = [];
  for (const line of lines) {
    const match = /^([a-fA-F0-9]{64}) [ *]([^\r\n]+)$/.exec(line);
    if (!match) throw new Error("update metadata checksum format invalid");
    if (match[2] === filename) matches.push(match[1].toLowerCase());
  }
  if (matches.length !== 1) throw new Error("update metadata checksum missing or duplicated");
  return matches[0];
}

export async function sha256File(file: string): Promise<string> {
  const hash = createHash("sha256");
  for await (const chunk of createReadStream(file)) hash.update(chunk);
  return hash.digest("hex");
}

type FetchResponse = Awaited<ReturnType<typeof fetch>>;
export type UnsignedUpdaterDeps = {
  currentVersion: string;
  target: DesktopUpdateTarget;
  cacheRoot: string;
  fetch: (url: string, init: Parameters<typeof fetch>[1]) => Promise<FetchResponse>;
  prepare: (file: string, version: string) => Promise<void>;
  install: () => Promise<void>;
};

export class UnsignedDesktopUpdater {
  private readonly events = new EventEmitter();
  autoDownload = false;
  autoInstallOnAppQuit = false;
  allowPrerelease = false;
  allowDowngrade = false;
  private candidate?: Candidate;
  private downloaded?: string;
  constructor(private readonly deps: UnsignedUpdaterDeps) {}
  on(event: string, listener: (...args: never[]) => void): this {
    this.events.on(event, listener as (...args: unknown[]) => void); return this;
  }
  removeListener(event: string, listener: (...args: never[]) => void): this {
    this.events.removeListener(event, listener as (...args: unknown[]) => void); return this;
  }
  private emit(event: string, ...args: unknown[]): void { this.events.emit(event, ...args); }

  private async request(url: string): Promise<FetchResponse> {
    // Follow only GitHub's release CDN redirects. Never forward credentials.
    for (let redirects = 0; redirects < 6; redirects += 1) {
      const parsed = new URL(url);
      if (parsed.protocol !== "https:" || parsed.username || parsed.password ||
          !["api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"].includes(parsed.hostname)) {
        throw new Error("update metadata download origin invalid");
      }
      const response = await this.deps.fetch(url, {
        redirect: "manual", signal: AbortSignal.timeout(10 * 60_000),
        headers: { "User-Agent": "Awaken-Desktop-Updater", Accept: parsed.hostname === "api.github.com" ? "application/vnd.github+json" : "application/octet-stream" },
      });
      if ([301, 302, 303, 307, 308].includes(response.status)) {
        const next = response.headers.get("location");
        await response.body?.cancel();
        if (!next) throw new Error("update metadata redirect invalid");
        url = new URL(next, url).href;
        continue;
      }
      if (!response.ok) throw new Error(`${[403, 429].includes(response.status) ? "rate limit" : "update network"} HTTP ${response.status}`);
      return response;
    }
    throw new Error("update metadata redirect limit");
  }

  private async readSmall(url: string, limit: number): Promise<string> {
    const response = await this.request(url);
    if (!response.body) throw new Error("update metadata body missing");
    const chunks: Uint8Array[] = []; let bytes = 0;
    for await (const chunk of response.body) {
      bytes += chunk.length;
      if (bytes > limit) throw new Error("update metadata size limit");
      chunks.push(chunk);
    }
    return Buffer.concat(chunks).toString("utf8");
  }

  async checkForUpdates(): Promise<void> {
    this.candidate = undefined;
    this.downloaded = undefined;
    const raw = JSON.parse(await this.readSmall(UNSIGNED_RELEASE_API, 1024 * 1024));
    const candidate = parseReleaseCandidate(raw, this.deps.target);
    if ((compareStableSemver(candidate.version, this.deps.currentVersion) ?? -1) <= 0) {
      this.emit("update-not-available"); return;
    }
    const checksumName = "SHA256SUMS.txt";
    const checksumAssets = raw.assets.filter((a: ReleaseAsset) => a?.name === checksumName);
    if (checksumAssets.length !== 1 || checksumAssets[0].browser_download_url !== `${RELEASE_ROOT}v${candidate.version}/${checksumName}`) {
      throw new Error("update metadata checksum asset invalid");
    }
    const checksums = await this.readSmall(checksumAssets[0].browser_download_url, 16 * 1024);
    this.candidate = { ...candidate, sha256: releaseChecksum(checksums, candidate.asset.name) };
    this.emit("update-available", candidate);
  }

  async downloadUpdate(): Promise<void> {
    const candidate = this.candidate;
    if (!candidate) throw new Error("update metadata check required");
    this.downloaded = undefined;
    await fs.mkdir(this.deps.cacheRoot, { recursive: true, mode: 0o700 });
    for (const entry of await fs.readdir(this.deps.cacheRoot)) {
      if (/^download-[A-Za-z0-9]+$/.test(entry)) await fs.rm(join(this.deps.cacheRoot, entry), { recursive: true, force: true });
    }
    const work = await fs.mkdtemp(join(this.deps.cacheRoot, "download-"));
    const file = join(work, candidate.asset.name);
    try {
      const response = await this.request(candidate.asset.browser_download_url);
      if (!response.body) throw new Error("update download body missing");
      const output = await fs.open(file, "wx", 0o600);
      const hash = createHash("sha256"); let transferred = 0; const started = Date.now();
      try {
        for await (const chunk of response.body) {
          transferred += chunk.length;
          if (transferred > candidate.asset.size) throw new Error("update checksum size mismatch");
          await output.writeFile(chunk); hash.update(chunk);
          this.emit("download-progress", { total: candidate.asset.size, transferred,
            percent: transferred / candidate.asset.size * 100,
            bytesPerSecond: transferred * 1000 / Math.max(1, Date.now() - started) });
        }
      } finally { await output.close(); }
      if (transferred !== candidate.asset.size || hash.digest("hex") !== candidate.sha256) {
        throw new Error("update checksum mismatch");
      }
      await this.deps.prepare(file, candidate.version);
      this.downloaded = file;
      this.emit("update-downloaded", candidate);
    } catch (error) {
      await fs.rm(work, { recursive: true, force: true });
      throw error;
    }
  }

  async quitAndInstall(): Promise<void> {
    if (!this.candidate || !this.downloaded || await sha256File(this.downloaded) !== this.candidate.sha256) {
      throw new Error("update checksum changed before install");
    }
    await this.deps.install();
  }
}
