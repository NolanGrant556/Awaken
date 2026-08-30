import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const markdownExtension = ".md";

function stripInlineCode(line) {
  let output = "";
  let cursor = 0;

  while (cursor < line.length) {
    if (line[cursor] !== "`") {
      output += line[cursor];
      cursor += 1;
      continue;
    }

    let markerEnd = cursor;
    while (line[markerEnd] === "`") markerEnd += 1;
    const marker = line.slice(cursor, markerEnd);
    const closing = line.indexOf(marker, markerEnd);
    if (closing < 0) {
      output += marker;
      cursor = markerEnd;
      continue;
    }

    output += " ".repeat(closing + marker.length - cursor);
    cursor = closing + marker.length;
  }

  return output;
}

export function stripMarkdownCode(markdown) {
  const lines = markdown.split("\n");
  let fence = null;

  return lines
    .map((line) => {
      const markerMatch = line.match(/^ {0,3}(`{3,}|~{3,})/);
      if (fence) {
        if (
          markerMatch &&
          markerMatch[1][0] === fence.character &&
          markerMatch[1].length >= fence.length &&
          line.slice(markerMatch[0].length).trim() === ""
        ) {
          fence = null;
        }
        return "";
      }

      if (markerMatch) {
        fence = {
          character: markerMatch[1][0],
          length: markerMatch[1].length,
        };
        return "";
      }

      return stripInlineCode(line);
    })
    .join("\n");
}

function unescapeDestination(destination) {
  return destination.replace(/\\([\\()<> ])/g, "$1");
}

function destinationFromInlineLink(line, start) {
  let cursor = start;
  while (/\s/.test(line[cursor] ?? "")) cursor += 1;

  if (line[cursor] === "<") {
    const closing = line.indexOf(">", cursor + 1);
    if (closing < 0) return null;
    return {
      destination: line.slice(cursor + 1, closing),
      end: closing + 1,
    };
  }

  let depth = 0;
  let destination = "";
  for (; cursor < line.length; cursor += 1) {
    const character = line[cursor];
    if (character === "\\" && cursor + 1 < line.length) {
      destination += character + line[cursor + 1];
      cursor += 1;
      continue;
    }
    if (character === "(" ) {
      depth += 1;
      destination += character;
      continue;
    }
    if (character === ")") {
      if (depth === 0) break;
      depth -= 1;
      destination += character;
      continue;
    }
    if (/\s/.test(character) && depth === 0) break;
    destination += character;
  }

  if (!destination) return null;
  return { destination: unescapeDestination(destination), end: cursor };
}

export function extractMarkdownDestinations(markdown) {
  const stripped = stripMarkdownCode(markdown);
  const destinations = [];

  stripped.split("\n").forEach((line, lineIndex) => {
    let cursor = 0;
    while (cursor < line.length) {
      const marker = line.indexOf("](", cursor);
      if (marker < 0) break;
      if (line.lastIndexOf("[", marker) < 0) {
        cursor = marker + 2;
        continue;
      }
      const parsed = destinationFromInlineLink(line, marker + 2);
      if (parsed) {
        destinations.push({ target: parsed.destination, line: lineIndex + 1 });
        cursor = Math.max(parsed.end, marker + 2);
      } else {
        cursor = marker + 2;
      }
    }

    const reference = line.match(/^ {0,3}\[[^\]]+\]:\s*(?:<([^>]+)>|(\S+))/);
    const referenceTarget = reference?.[1] ?? reference?.[2];
    if (referenceTarget) {
      destinations.push({ target: unescapeDestination(referenceTarget), line: lineIndex + 1 });
    }
  });

  return destinations;
}

function markdownFilesUnder(directory) {
  const files = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...markdownFilesUnder(path));
    } else if (entry.isFile() && entry.name.endsWith(markdownExtension)) {
      files.push(path);
    }
  }
  return files;
}

function localPathFromTarget(target) {
  const trimmed = target.trim();
  if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("//")) return null;
  if (/^[a-z][a-z\d+.-]*:/i.test(trimmed)) return null;

  const path = trimmed.split(/[?#]/, 1)[0];
  if (!path) return null;
  try {
    return decodeURI(path);
  } catch {
    return path;
  }
}

export function checkDocLinks({ rootDir, files } = {}) {
  const resolvedRoot = resolve(rootDir ?? process.cwd());
  const candidates = files ?? [
    join(resolvedRoot, "README.md"),
    ...markdownFilesUnder(join(resolvedRoot, "docs")),
  ];
  const missing = [];
  let checkedLinks = 0;

  for (const file of candidates) {
    const markdown = readFileSync(file, "utf8");
    for (const { target, line } of extractMarkdownDestinations(markdown)) {
      const localPath = localPathFromTarget(target);
      if (!localPath) continue;
      checkedLinks += 1;
      if (!existsSync(resolve(dirname(file), localPath))) {
        missing.push({
          file: relative(resolvedRoot, file),
          line,
          target,
        });
      }
    }
  }

  return {
    checkedFiles: candidates.length,
    checkedLinks,
    missing,
  };
}

function main() {
  const result = checkDocLinks({ rootDir: process.cwd() });
  if (result.missing.length > 0) {
    console.error(
      `文档链接检查失败：${result.missing.length} 个本地链接目标不存在（扫描 ${result.checkedFiles} 个文件）。`,
    );
    for (const missing of result.missing) {
      console.error(`${missing.file}:${missing.line} -> ${missing.target}`);
    }
    process.exitCode = 1;
    return;
  }

  console.log(
    `文档链接检查通过：${result.checkedFiles} 个文件，${result.checkedLinks} 个本地链接。`,
  );
}

if (resolve(process.argv[1] ?? "") === fileURLToPath(import.meta.url)) {
  main();
}
