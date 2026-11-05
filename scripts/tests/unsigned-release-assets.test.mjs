import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, readFileSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { unsignedReleaseFiles, verifyUnsignedAssets } from "../release/verify-unsigned-assets.mjs";

test("unsigned release requires both architectures and generates the runtime checksum format", async () => {
  const dir = mkdtempSync(join(tmpdir(), "awaken-unsigned-release-"));
  try {
    assert.throws(() => unsignedReleaseFiles("../0.1.2"));
    await assert.rejects(verifyUnsignedAssets(dir, "0.1.2"));
    for (const file of unsignedReleaseFiles("0.1.2")) writeFileSync(join(dir, file), file);
    const receipt = await verifyUnsignedAssets(dir, "0.1.2");
    assert.equal(receipt.files.length, 5);
    const lines = readFileSync(join(dir, "SHA256SUMS.txt"), "utf8").trim().split("\n");
    assert.equal(lines.length, 5);
    for (const line of lines) assert.match(line, /^[a-f0-9]{64}  Awaken-[A-Za-z0-9.-]+$/);
    if (process.platform !== "win32") {
      const name = unsignedReleaseFiles("0.1.2")[0]; rmSync(join(dir, name));
      symlinkSync(join(dir, unsignedReleaseFiles("0.1.2")[1]), join(dir, name));
      await assert.rejects(verifyUnsignedAssets(dir, "0.1.2"), /invalid artifact/);
    }
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
