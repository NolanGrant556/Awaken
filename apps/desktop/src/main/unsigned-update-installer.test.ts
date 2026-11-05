import { describe, expect, it } from "vitest";
import { execFile } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { MAC_UNSIGNED_INSTALL_SCRIPT, validateArchiveEntries } from "./unsigned-update-installer.js";

describe("unsigned installer", () => {
  it("rejects absolute and traversal paths before extraction", () => {
    validateArchiveEntries("Awaken.app/\nAwaken.app/Contents/Info.plist\n");
    for (const name of ["/tmp/file", "Awaken.app/../../other", "Other.app/Contents/x", "Awaken.app/..\\x", ""]) {
      expect(() => validateArchiveEntries(name)).toThrow();
    }
  });
  it.skipIf(process.platform !== "darwin")("atomically swaps an app with spaces in its path and retains the successful client", async () => {
    const root = await mkdtemp(join(tmpdir(), "awaken install test "));
    try {
      const target = join(root, "Awaken.app"); const stage = join(root, "stage", "Awaken.app");
      await mkdir(target); await writeFile(join(target, "previous"), "old");
      await mkdir(join(stage, "Contents", "MacOS"), { recursive: true });
      await writeFile(join(stage, "Contents", "MacOS", "Awaken"), "#!/bin/sh\necho desktop_main_window_created\n", { mode: 0o700 });
      const helper = join(root, "install.sh"); await writeFile(helper, MAC_UNSIGNED_INSTALL_SCRIPT);
      const parent = execFile("/bin/sh", ["-c", "exit 0"]); await new Promise(resolve => parent.once("exit", resolve));
      const backup = join(root, "backup.app"); const log = join(root, "install.log");
      await promisify(execFile)("/bin/bash", [helper, String(parent.pid), target, stage, backup, log]);
      expect(await readFile(log, "utf8")).toContain("install_succeeded");
      expect(await readFile(join(target, "Contents", "MacOS", "Awaken"), "utf8")).toContain("desktop_main_window_created");
      await expect(readFile(join(backup, "previous"))).rejects.toThrow();
    } finally { await rm(root, { recursive: true, force: true }); }
  });
});
