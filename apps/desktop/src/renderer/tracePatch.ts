import type {
  ToolTracePatchView,
  ToolTraceRecordView,
} from "./toolTracePresentation";

const cache = new WeakMap<
  ToolTraceRecordView,
  ToolTracePatchView | undefined
>();

/** Bounded LCS over the changed region. Unchanged outer lines never enter the matrix. */
function unified(
  path: string,
  before: string,
  after: string,
): { patch: string; coarse: boolean } {
  const lines = (text: string) => {
    const result = text.split("\n");
    if (text.endsWith("\n")) result.pop();
    return result;
  };
  const old = before ? lines(before) : [],
    next = after ? lines(after) : [];
  const oldHasNewline = (index: number) =>
    index < old.length - 1 || before.endsWith("\n");
  const newHasNewline = (index: number) =>
    index < next.length - 1 || after.endsWith("\n");
  const equal = (i: number, j: number) =>
    old[i] === next[j] && oldHasNewline(i) === newHasNewline(j);
  let prefix = 0,
    suffix = 0;
  while (prefix < old.length && prefix < next.length && equal(prefix, prefix))
    prefix++;
  while (
    suffix < old.length - prefix &&
    suffix < next.length - prefix &&
    equal(old.length - 1 - suffix, next.length - 1 - suffix)
  )
    suffix++;
  const a = old.slice(prefix, old.length - suffix),
    b = next.slice(prefix, next.length - suffix);
  const operations: string[] = [];
  function emit(sign: string, text: string, hasNewline: boolean) {
    operations.push(`${sign}${text}`);
    if (!hasNewline) operations.push("\\ No newline at end of file");
  }
  const coarse = a.length * b.length > 1_000_000;
  if (coarse) {
    a.forEach((line, i) => emit("-", line, oldHasNewline(prefix + i)));
    b.forEach((line, j) => emit("+", line, newHasNewline(prefix + j)));
  } else {
    const widths = b.length + 1,
      table = new Uint32Array((a.length + 1) * widths);
    for (let i = a.length - 1; i >= 0; i--)
      for (let j = b.length - 1; j >= 0; j--)
        table[i * widths + j] = equal(prefix + i, prefix + j)
          ? 1 + table[(i + 1) * widths + j + 1]
          : Math.max(table[(i + 1) * widths + j], table[i * widths + j + 1]);
    let i = 0,
      j = 0;
    while (i < a.length || j < b.length) {
      if (i < a.length && j < b.length && equal(prefix + i, prefix + j)) {
        emit(" ", a[i], oldHasNewline(prefix + i));
        i++;
        j++;
      } else if (
        i < a.length &&
        (j === b.length ||
          table[(i + 1) * widths + j] >= table[i * widths + j + 1])
      ) {
        emit("-", a[i], oldHasNewline(prefix + i));
        i++;
      } else {
        emit("+", b[j], newHasNewline(prefix + j));
        j++;
      }
    }
  }
  const start = Math.max(0, prefix - 3),
    tail = Math.min(suffix, 3);
  const header = `--- ${before ? `a/${path}` : "/dev/null"}\n+++ b/${path}\n@@ -${old.length ? start + 1 : 0},${prefix - start + a.length + tail} +${next.length ? start + 1 : 0},${prefix - start + b.length + tail} @@`;
  return {
    patch: [
      header,
      ...old.slice(start, prefix).map((line) => ` ${line}`),
      ...operations,
      ...old
        .slice(old.length - suffix, old.length - suffix + tail)
        .flatMap((line, index) =>
          oldHasNewline(old.length - suffix + index)
            ? [` ${line}`]
            : [` ${line}`, "\\ No newline at end of file"],
        ),
    ].join("\n"),
    coarse,
  };
}

export function toolResultPatch(
  tool: ToolTraceRecordView | undefined,
): ToolTracePatchView | undefined {
  if (!tool || tool.status !== "done") return undefined;
  if (cache.has(tool)) return cache.get(tool);
  let patch: ToolTracePatchView | undefined;
  try {
    const result = JSON.parse(tool.content || "null");
    if (
      tool.title?.toLowerCase() === "edit" &&
      typeof result?.file_path === "string" &&
      Array.isArray(result.hunks)
    ) {
      const hunks = result.hunks.filter(
        (hunk: unknown): hunk is { old_text: string; new_text: string } =>
          Boolean(
            hunk &&
            typeof hunk === "object" &&
            "old_text" in hunk &&
            "new_text" in hunk &&
            typeof hunk.old_text === "string" &&
            typeof hunk.new_text === "string",
          ),
      );
      if (hunks.length === result.hunks.length && hunks.length) {
        const previews = hunks.map(
          (hunk: { old_text: string; new_text: string }) =>
            unified(result.file_path, hunk.old_text, hunk.new_text),
        );
        patch = {
          patch: previews
            .map((preview: { patch: string }) => preview.patch)
            .join("\n"),
          affectedPaths: [result.file_path],
          truncated:
            previews.some((preview: { coarse: boolean }) => preview.coarse) ||
            hunks.some((hunk: { old_text: string; new_text: string }) =>
              /\n\[truncated\]/.test(hunk.old_text + hunk.new_text),
            ),
        };
      }
    } else if (
      tool.title?.toLowerCase() === "write" &&
      result?.action === "created" &&
      typeof result.file_path === "string"
    ) {
      const args = JSON.parse(tool.args || "null");
      if (typeof args?.content === "string")
        patch = {
          patch: unified(result.file_path, "", args.content).patch,
          affectedPaths: [result.file_path],
        };
    }
  } catch {
    /* Old or truncated results remain readable without invented changes. */
  }
  cache.set(tool, patch);
  return patch;
}
