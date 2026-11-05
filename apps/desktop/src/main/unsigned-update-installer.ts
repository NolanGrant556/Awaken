import { execFile, spawn } from "node:child_process";
import { once } from "node:events";
import fs from "node:fs/promises";
import { dirname, join, resolve, sep } from "node:path";
import { promisify } from "node:util";
import yauzl from "yauzl";
import { sha256File } from "./unsigned-desktop-updater.js";

const exec = promisify(execFile);

// Arguments are passed separately, never interpolated into executable text.
// Stage beside the installed app so both renames remain on the same volume.
export const MAC_UNSIGNED_INSTALL_SCRIPT = `#!/bin/bash
set -eu
parent="$1"; target="$2"; staged="$3"; backup="$4"; log="$5"
for ((i=0;i<60;i++)); do
  if ! kill -0 "$parent" 2>/dev/null; then break; fi
  sleep 1
done
if kill -0 "$parent" 2>/dev/null; then echo 'parent_exit_timeout' >> "$log"; exit 1; fi
if ! /bin/mv "$target" "$backup"; then echo 'backup_failed' >> "$log"; exit 1; fi
if ! /bin/mv "$staged" "$target"; then
  /bin/mv "$backup" "$target"
  /usr/bin/open -n "$target"
  echo 'replace_failed_rolled_back' >> "$log"; exit 1
fi
"$target/Contents/MacOS/Awaken" > "$log.launch" 2>&1 &
child=$!
for ((i=0;i<60;i++)); do
  if /usr/bin/grep -q 'desktop_main_window_created' "$log.launch"; then
    echo 'install_succeeded' >> "$log"
    /bin/rm -rf "$backup"
    exit 0
  fi
  if ! kill -0 "$child" 2>/dev/null; then break; fi
  sleep 1
done
kill "$child" 2>/dev/null || true
for ((i=0;i<15;i++)); do
  if ! kill -0 "$child" 2>/dev/null; then break; fi
  sleep 1
done
if kill -0 "$child" 2>/dev/null; then echo 'rollback_blocked_child_running' >> "$log"; exit 1; fi
/bin/mv "$target" "$staged.failed"
/bin/mv "$backup" "$target"
/usr/bin/open -n "$target"
echo 'startup_failed_rolled_back' >> "$log"
`;

export function validateArchiveEntries(list: string): void {
  const names = list.split(/\r?\n/).filter(Boolean);
  if (!names.length || names.some(name =>
    !name.startsWith("Awaken.app/") || name.includes("\\") ||
    name.split("/").some(segment => segment === "..") || name.includes("\0"))) {
    throw new Error("update metadata archive paths invalid");
  }
}

export async function validateMacArchive(file: string): Promise<void> {
  await new Promise<void>((resolveArchive, reject) => {
    yauzl.open(file, { lazyEntries: true }, (error, zip) => {
      if (error || !zip) { reject(error ?? new Error("update metadata zip invalid")); return; }
      let bytes = 0; let entries = 0;
      const fail = (error: unknown) => { zip.close(); reject(error); };
      zip.on("error", fail); zip.on("end", resolveArchive);
      zip.on("entry", (entry: yauzl.Entry) => {
        try {
          validateArchiveEntries(entry.fileName);
          bytes += entry.uncompressedSize; entries += 1;
          if (bytes > 3 * 1024 ** 3 || entries > 100_000) throw new Error("update metadata archive size limit");
          const mode = entry.externalFileAttributes >>> 16;
          if ((mode & 0o170000) !== 0o120000) { zip.readEntry(); return; }
          if (entry.uncompressedSize > 4096) throw new Error("update metadata symlink too long");
          zip.openReadStream(entry, (error, stream) => {
            if (error || !stream) { fail(error); return; }
            const chunks: Buffer[] = [];
            stream.on("error", fail); stream.on("data", chunk => chunks.push(Buffer.from(chunk)));
            stream.on("end", () => {
              const link = Buffer.concat(chunks).toString("utf8");
              const root = resolve("/", "Awaken.app");
              const target = resolve("/", dirname(entry.fileName), link);
              if (!link || link.startsWith("/") || link.includes("\0") || !target.startsWith(`${root}${sep}`)) {
                fail(new Error("update metadata archive symlink escapes bundle")); return;
              }
              zip.readEntry();
            });
          });
        } catch (error) { fail(error); }
      });
      zip.readEntry();
    });
  });
}

export class UnsignedUpdateInstaller {
  private prepared?: { file: string; digest: string; stage?: string; target?: string; work?: string };
  constructor(private readonly options: {
    platform: string; arch: string; appPath: string; cacheRoot: string;
    quit: () => void; openInstaller: (file: string) => Promise<string>;
  }) {}

  async prepare(file: string, version: string): Promise<void> {
    if (this.prepared?.work) await fs.rm(this.prepared.work, { recursive: true, force: true });
    this.prepared = undefined;
    if (this.options.platform === "win32") {
      this.prepared = { file, digest: await sha256File(file) }; return;
    }
    const target = resolve(this.options.appPath, "../../..");
    if (!target.endsWith(`${sep}Awaken.app`) || (await fs.lstat(target)).isSymbolicLink()) {
      throw new Error("update install requires an installed Awaken.app");
    }
    // Refuse unwritable/system-owned installations before stopping the client.
    await fs.access(dirname(target), fs.constants.W_OK);
    const work = await fs.mkdtemp(join(dirname(target), ".awaken-update-"));
    const stage = join(work, "Awaken.app");
    try {
      await validateMacArchive(file);
      await exec("/usr/bin/ditto", ["-x", "-k", file, work]);
      const stageRoot = await fs.realpath(stage);
      // All symlinks must resolve inside this bundle (Framework symlinks are valid).
      const checkLinks = async (directory: string): Promise<void> => {
        for (const entry of await fs.readdir(directory, { withFileTypes: true })) {
          const path = join(directory, entry.name);
          if (entry.isSymbolicLink()) {
            const real = await fs.realpath(path);
            if (!real.startsWith(`${stageRoot}${sep}`)) throw new Error("update metadata archive symlink escapes bundle");
          } else if (entry.isDirectory()) await checkLinks(path);
        }
      };
      await checkLinks(stage);
      const plist = join(stage, "Contents", "Info.plist");
      const read = async (key: string) => (await exec("/usr/libexec/PlistBuddy", ["-c", `Print ${key}`, plist])).stdout.trim();
      if (await read("CFBundleIdentifier") !== "com.awaken.desktop" || await read("CFBundleShortVersionString") !== version) {
        throw new Error("update metadata bundle identity or version mismatch");
      }
      await exec("/usr/bin/lipo", ["-verify_arch", this.options.arch === "arm64" ? "arm64" : "x86_64", join(stage, "Contents", "MacOS", "Awaken")]);
      await exec("/usr/bin/codesign", ["--verify", "--deep", "--strict", stage]);
      this.prepared = { file, digest: await sha256File(file), stage, target, work };
    } catch (error) { await fs.rm(work, { recursive: true, force: true }); throw error; }
  }

  async install(): Promise<void> {
    const prepared = this.prepared;
    if (!prepared || await sha256File(prepared.file) !== prepared.digest) throw new Error("update checksum changed before install");
    if (this.options.platform === "win32") {
      const error = await this.options.openInstaller(prepared.file);
      if (error) throw new Error(`update install launch failed: ${error}`);
      this.options.quit(); return;
    }
    if (!prepared.work || !prepared.stage || !prepared.target) throw new Error("update install staging missing");
    const helper = join(prepared.work, "install.sh");
    await fs.writeFile(helper, MAC_UNSIGNED_INSTALL_SCRIPT, { mode: 0o700 });
    await fs.mkdir(this.options.cacheRoot, { recursive: true, mode: 0o700 });
    const log = join(this.options.cacheRoot, "install.log");
    const child = spawn("/bin/bash", [helper, String(process.pid), prepared.target, prepared.stage,
      join(prepared.work, "previous.app"), log], { detached: true, stdio: "ignore" });
    await once(child, "spawn");
    child.unref();
    this.options.quit();
  }
}
