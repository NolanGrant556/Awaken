import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const source = JSON.parse(fs.readFileSync(path.join(root, "src/managedenv/assets/catalog.json"), "utf8"));
function check(catalog, ...flags) {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "awaken-office-catalog-"));
  try {
    const file = path.join(temporary, "catalog.json");
    fs.writeFileSync(file, JSON.stringify(catalog));
    const result = spawnSync(process.execPath, [path.join(root, "scripts/check-managedenv-catalog.mjs"), file, ...flags], { encoding: "utf8" });
    assert.equal(result.signal, null);
    return { status: result.status, ...JSON.parse(result.stdout) };
  } finally { fs.rmSync(temporary, { recursive: true, force: true }); }
}

test("unpublished Office assets cannot pass the Office release gate", () => {
  const catalog = structuredClone(source);
  catalog.office_bundle.assets = {};
  assert.equal(check(catalog).status, 0);
  const result = check(catalog, "--office-release");
  assert.equal(result.status, 1);
  assert.deepEqual(result.officeUnavailable, ["darwin-arm64", "darwin-x64", "windows-x64"]);
});

test("incomplete routes and escaping bundle paths are rejected before publication", () => {
  const catalog = structuredClone(source);
  delete catalog.office_bundle.spec.python.openpyxl;
  catalog.office_bundle.assets["windows-x64"] = {
    url: "https://example.invalid/office.zip", sha256: "a".repeat(64), format: "zip",
    max_bytes: 1024, max_expanded_bytes: 4096,
    executables: { python: "C:/external/python.exe" }, resources: { fonts: "../fonts" },
  };
  const result = check(catalog);
  assert.equal(result.status, 1);
  assert.ok(result.errors.some(value => value.includes("openpyxl")));
  assert.ok(result.errors.some(value => value.includes("missing executable soffice")));
  assert.ok(result.errors.some(value => value.includes("missing executable pdffonts")));
  assert.ok(result.errors.some(value => value.includes("invalid bundle-relative path")));
});
