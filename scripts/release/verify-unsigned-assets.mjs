#!/usr/bin/env node
import { createHash } from "node:crypto";
import { createReadStream, lstatSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export function unsignedReleaseFiles(version) {
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) throw new Error("version must be X.Y.Z");
  return [
    `Awaken-${version}-arm64.dmg`, `Awaken-${version}-arm64.zip`,
    `Awaken-${version}-x64.dmg`, `Awaken-${version}-x64.zip`,
    `Awaken-Setup-${version}-x64.exe`,
  ];
}

export async function verifyUnsignedAssets(dir, version) {
  const files = [];
  for (const name of unsignedReleaseFiles(version)) {
    const file = join(dir, name); const stat = lstatSync(file);
    if (!stat.isFile() || stat.size <= 0 || stat.size > 1024 ** 3) throw new Error(`invalid artifact: ${name}`);
    const hash = createHash("sha256");
    for await (const chunk of createReadStream(file)) hash.update(chunk);
    files.push({ name, size: stat.size, sha256: hash.digest("hex") });
  }
  writeFileSync(join(dir, "SHA256SUMS.txt"), files.map(file => `${file.sha256}  ${file.name}\n`).join(""));
  return { version, files };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [dir, version, receipt] = process.argv.slice(2);
  if (!dir || !version || !receipt) throw new Error("usage: verify-unsigned-assets.mjs DIR VERSION RECEIPT");
  const result = await verifyUnsignedAssets(dir, version);
  writeFileSync(receipt, JSON.stringify(result, null, 2) + "\n");
  console.log(`无签名通道发布资产检查通过：${result.files.length} 个安装/更新文件`);
}
