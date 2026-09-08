import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const errors = [];
const files = [];
const features = [];
const expected = ['实现方案.md', '需求文档.md'].sort();
const docs = resolve(root, 'docs');
const blocks = readdirSync(docs, { withFileTypes: true });

for (const block of blocks) {
  const blockPath = resolve(docs, block.name);
  if (!block.isDirectory()) {
    errors.push(`docs 根目录不能放文件：${block.name}`);
    continue;
  }
  const children = readdirSync(blockPath, { withFileTypes: true });
  if (!children.length) errors.push(`空板块：${block.name}`);
  for (const feature of children) {
    const featurePath = resolve(blockPath, feature.name);
    if (!feature.isDirectory()) {
      errors.push(`板块下只能放功能目录：${relative(root, featurePath)}`);
      continue;
    }
    features.push(featurePath);
    const entries = readdirSync(featurePath, { withFileTypes: true });
    if (entries.some((entry) => !entry.isFile()) ||
        JSON.stringify(entries.map((entry) => entry.name).sort()) !== JSON.stringify(expected)) {
      errors.push(`功能目录必须只有需求文档.md 和实现方案.md：${relative(root, featurePath)}`);
    }
    for (const name of expected) {
      const path = resolve(featurePath, name);
      if (existsSync(path)) files.push(path);
    }
  }
}

function prose(text) {
  const lines = [];
  let fence;
  for (const line of text.split('\n')) {
    const marker = line.match(/^\s*(`{3,}|~{3,})/);
    if (marker) {
      if (!fence) fence = marker[1];
      else if (marker[1][0] === fence[0] && marker[1].length >= fence.length) fence = undefined;
      lines.push('');
    } else lines.push(fence ? '' : line);
  }
  return { text: lines.join('\n'), fence };
}

function slug(value) {
  return value.replace(/<[^>]+>/g, '').toLowerCase()
    .replace(/[^\p{L}\p{N}\p{M}_\-\s]/gu, '').replace(/ /g, '-');
}

const anchorCache = new Map();
function anchors(path) {
  if (anchorCache.has(path)) return anchorCache.get(path);
  const text = prose(readFileSync(path, 'utf8')).text;
  const result = new Set();
  const counts = new Map();
  for (const match of text.matchAll(/^#{1,6}\s+(.+)$/gm)) {
    const base = slug(match[1]);
    const count = counts.get(base) ?? 0;
    counts.set(base, count + 1);
    result.add(count ? `${base}-${count}` : base);
  }
  for (const match of text.matchAll(/\bid=["']([^"']+)["']/g)) result.add(match[1]);
  anchorCache.set(path, result);
  return result;
}

const readme = readFileSync(resolve(root, 'README.md'), 'utf8');
const agents = readFileSync(resolve(root, 'AGENTS.md'), 'utf8');
const counts = `${blocks.filter((block) => block.isDirectory()).length} 个板块、${features.length} 个功能、${files.length} 份文档`;
for (const [name, text] of [['README.md', readme], ['AGENTS.md', agents]]) {
  if (!text.includes(counts)) errors.push(`${name} 的目录数量应为：${counts}`);
}
for (const path of files) {
  const documentPath = relative(root, path).replaceAll('\\', '/');
  if (!readme.includes(documentPath)) errors.push(`README 缺少功能入口：${documentPath}`);
}

for (const path of [...files, resolve(root, 'README.md'), resolve(root, 'AGENTS.md')]) {
  const raw = readFileSync(path, 'utf8');
  const { text, fence } = prose(raw);
  const name = relative(root, path);
  if (fence) errors.push(`${name}：代码围栏未闭合`);
  if ((text.match(/^# /gm) ?? []).length !== 1) errors.push(`${name}：需要一个一级标题`);
  for (const match of text.matchAll(/\[[^\]\n]*\]\((<[^>]+>|[^)\n]+)\)/g)) {
    const url = match[1].replace(/^<|>$/g, '');
    if (/^(?:[a-z][a-z0-9+.-]*:|\/\/)/i.test(url)) continue;
    const hash = url.indexOf('#');
    const pathname = decodeURIComponent(hash < 0 ? url : url.slice(0, hash));
    const fragment = hash < 0 ? '' : decodeURIComponent(url.slice(hash + 1));
    const target = pathname ? resolve(dirname(path), pathname) : path;
    const line = text.slice(0, match.index).split('\n').length;
    if (!existsSync(target)) errors.push(`${name}:${line} 链接不存在：${url}`);
    else if (fragment && target.endsWith('.md') && statSync(target).isFile() && !anchors(target).has(fragment)) {
      errors.push(`${name}:${line} 章节不存在：${url}`);
    }
  }
}

if (errors.length) {
  console.error(errors.join('\n'));
  console.error(`文档检查失败：${errors.length} 项`);
  process.exitCode = 1;
} else console.log(`文档检查通过：${counts}；目录、导航、相对链接、章节锚点和代码围栏均有效。`);
