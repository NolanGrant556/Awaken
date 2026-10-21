// electron-builder captures DMG hashes before afterAllArtifactBuild alters them.
// Recompute feed hashes only after the build and notarization hooks finish.
import { readFileSync, writeFileSync, statSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { resolve, join } from 'node:path';
import { parse, stringify } from 'yaml';
if (process.platform === 'darwin') {
  const root = resolve(import.meta.dirname, '../..');
  const dir = join(root, 'apps/desktop/dist-electron');
  const version = JSON.parse(readFileSync(join(root, 'apps/desktop/package.json'), 'utf8')).version;
  const path = join(dir, 'latest-mac.yml');
  const feed = parse(readFileSync(path, 'utf8'));
  if (feed.version !== version) throw new Error('Unexpected feed version');
  const expected = ['arm64', 'x64'].flatMap(arch => ['zip', 'dmg'].map(ext => `Awaken-${version}-${arch}.${ext}`));
  if (!Array.isArray(feed.files) || feed.files.length !== expected.length || new Set(feed.files.map(file => file.url)).size !== expected.length) throw new Error('Unexpected feed files');
  for (const file of feed.files) {
    if (!expected.includes(file.url)) throw new Error('Unexpected feed URL');
    const artifact = join(dir, file.url);
    file.sha512 = createHash('sha512').update(readFileSync(artifact)).digest('base64');
    file.size = statSync(artifact).size;
  }
  const primary = feed.files.find(file => file.url === feed.path);
  if (!primary || !primary.url.endsWith('.zip')) throw new Error('Expected primary ZIP');
  feed.sha512 = primary.sha512;
  writeFileSync(path, stringify(feed));
  console.log('[finalize-macos-feed] hashes finalized after notarization');
}
