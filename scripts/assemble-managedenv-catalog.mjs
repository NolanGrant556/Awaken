import fs from "node:fs";
import path from "node:path";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const [directory, destination] = process.argv.slice(2);
const officeOnly = process.argv.includes("--office-only");
if (!directory || !destination) throw new Error("usage: node scripts/assemble-managedenv-catalog.mjs <native-assets-directory> <output-catalog>");
const catalog = JSON.parse(fs.readFileSync(path.join(root, "src/managedenv/assets/catalog.json"), "utf8"));
for (const filename of fs.readdirSync(directory).filter((name) => name.endsWith(".tar.gz.json"))) {
  const descriptor = JSON.parse(fs.readFileSync(path.join(directory, filename), "utf8"));
  if (officeOnly && descriptor.component !== "office-bundle") continue;
  const archive = path.join(directory, filename.slice(0, -5));
  const checksum = createHash("sha256");
  for await (const chunk of fs.createReadStream(archive)) checksum.update(chunk);
  const hash = checksum.digest("hex");
  if (hash !== descriptor.asset.sha256) throw new Error(`${filename}: archive hash mismatch`);
  if (descriptor.component === "office-bundle") {
    if (!catalog.office_bundle || descriptor.version !== catalog.office_bundle.version || descriptor.layout_version !== catalog.office_bundle.layout_version) throw new Error(`${filename}: Office identity mismatch`);
    catalog.office_bundle.assets[descriptor.target] = descriptor.asset;
    continue;
  }
  const existing = catalog.components[descriptor.component];
  if (!existing) throw new Error(`${filename}: unknown component`);
  if (existing.version && existing.version !== descriptor.version) throw new Error(`${filename}: component versions differ across targets`);
  Object.assign(existing, { version: descriptor.version, license: descriptor.license, source: descriptor.source, probe: descriptor.probe, probe_args: descriptor.probe_args });
  existing.assets[descriptor.target] = descriptor.asset;
}
fs.mkdirSync(path.dirname(path.resolve(destination)), { recursive: true });
fs.writeFileSync(destination, JSON.stringify(catalog, null, 2) + "\n");
