const fenceOpenPattern = /^( {0,3})(`{3,}|~{3,})(.*)$/;
const delimiterCellPattern = /^\s*:?-{1,}:?\s*$/;

type FenceState = {
  marker: string;
};

function splitQuotePrefix(line: string): { prefix: string; body: string } {
  const match = /^(?: {0,3}>\s?)+/.exec(line);
  if (!match) {
    return { prefix: "", body: line };
  }
  return { prefix: match[0], body: line.slice(match[0].length) };
}

function isIndentedCode(body: string): boolean {
  return /^(?: {4,}|\t)/.test(body);
}

function nextFenceState(body: string, fence: FenceState | undefined): FenceState | undefined {
  const match = fenceOpenPattern.exec(body);
  if (!match) {
    return fence;
  }
  const marker = match[2] ?? "";
  if (!fence) {
    return { marker };
  }
  if (marker[0] === fence.marker[0] && marker.length >= fence.marker.length && !(match[3] ?? "").trim()) {
    return undefined;
  }
  return fence;
}

function splitTableCells(line: string): string[] {
  const cells: string[] = [];
  let current = "";
  let escaped = false;

  for (const character of line.trimEnd()) {
    if (escaped) {
      current += character;
      escaped = false;
      continue;
    }
    if (character === "\\") {
      current += character;
      escaped = true;
      continue;
    }
    if (character === "|") {
      cells.push(current);
      current = "";
      continue;
    }
    current += character;
  }
  cells.push(current);

  if (cells.length > 1 && cells[0]?.trim() === "") {
    cells.shift();
  }
  if (cells.length > 0 && cells[cells.length - 1]?.trim() === "") {
    cells.pop();
  }
  return cells;
}

function hasPipe(line: string): boolean {
  return line.includes("|");
}

function isDelimiterRow(body: string): boolean {
  if (!hasPipe(body) || isIndentedCode(body)) {
    return false;
  }
  const cells = splitTableCells(body);
  return cells.length > 0 && cells.every((cell) => delimiterCellPattern.test(cell));
}

function isTableRow(body: string): boolean {
  return hasPipe(body) && !isIndentedCode(body) && !isDelimiterRow(body) && splitTableCells(body).length > 0;
}

function usesOuterPipes(line: string): boolean {
  const trimmed = line.trim();
  return trimmed.startsWith("|") || trimmed.endsWith("|");
}

function padDelimiterCells(cells: string[], count: number): string[] {
  const next = cells.slice(0, count);
  const sample = cells.find((cell) => cell.trim().length > 0) ?? "---";
  const filler = sample.trim().replace(/[^-:]/g, "") || "---";
  const padded = sample.includes(" ") ? ` ${filler} ` : filler;
  while (next.length < count) {
    next.push(padded);
  }
  return next;
}

function formatDelimiterRow(cells: string[], withPipes: boolean): string {
  const inner = cells.join("|");
  return withPipes ? `|${inner}|` : inner;
}

/**
 * remark-gfm / micromark reject a table when the delimiter row cell count
 * does not match the header. Models such as MiniMax often emit
 * `| 类别 | 工具 |` followed by `|------|`. This rewrite only pads or trims
 * that delimiter row for display; it does not change stored Markdown.
 */
export function repairGfmTableDelimiters(markdown: string): string {
  if (!markdown.includes("|")) {
    return markdown;
  }

  const lines = markdown.split(/\r?\n/);
  let fence: FenceState | undefined;
  let changed = false;

  for (let index = 0; index < lines.length; index += 1) {
    const current = lines[index] ?? "";
    const currentParts = splitQuotePrefix(current);
    const wasInFence = fence !== undefined;
    fence = nextFenceState(currentParts.body, fence);

    if (wasInFence || index === 0) {
      continue;
    }

    const previous = lines[index - 1] ?? "";
    const previousParts = splitQuotePrefix(previous);
    if (currentParts.prefix !== previousParts.prefix) {
      continue;
    }
    if (!isTableRow(previousParts.body) || !isDelimiterRow(currentParts.body)) {
      continue;
    }

    const headerCount = splitTableCells(previousParts.body).length;
    const delimiterCells = splitTableCells(currentParts.body);
    if (headerCount === 0 || delimiterCells.length === headerCount) {
      continue;
    }

    lines[index] = `${currentParts.prefix}${formatDelimiterRow(
      padDelimiterCells(delimiterCells, headerCount),
      usesOuterPipes(currentParts.body) || usesOuterPipes(previousParts.body),
    )}`;
    changed = true;
  }

  return changed ? lines.join("\n") : markdown;
}
