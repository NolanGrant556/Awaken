import { Fragment, useMemo, useState } from "react";

export type DiffRow = {
  text: string;
  kind: "add" | "remove" | "context" | "hunk";
  oldLine?: number;
  newLine?: number;
};
export type DiffFile = { name: string; rows: DiffRow[] };

export function parseTraceDiff(patch: string): DiffFile[] {
  const files: DiffFile[] = [];
  let file: DiffFile = { name: "变更", rows: [] };
  let oldLine = 0,
    newLine = 0;
  const flush = () => {
    if (file.rows.length) files.push(file);
  };
  const lines = patch.split("\n");
  for (let index = 0; index < lines.length; index++) {
    const text = lines[index];
    if (text.startsWith("diff --git ")) {
      flush();
      file = { name: text.slice(11), rows: [] };
      oldLine = newLine = 0;
      continue;
    }
    if (text.startsWith("--- ") && lines[index + 1]?.startsWith("+++ ")) {
      // Plain unified patches may concatenate files without a diff --git header.
      if (file.rows.length) {
        flush();
        file = { name: "变更", rows: [] };
        oldLine = newLine = 0;
      }
      const before = text.slice(4).replace(/^a\//, "");
      const after = lines[++index].slice(4).replace(/^b\//, "");
      file.name = after === "/dev/null" ? before : after;
      continue;
    }
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(text);
    if (hunk) {
      oldLine = Number(hunk[1]);
      newLine = Number(hunk[2]);
      file.rows.push({ text, kind: "hunk" });
      continue;
    }
    if (text.startsWith("+"))
      file.rows.push({
        text: text.slice(1),
        kind: "add",
        newLine: newLine ? newLine++ : undefined,
      });
    else if (text.startsWith("-"))
      file.rows.push({
        text: text.slice(1),
        kind: "remove",
        oldLine: oldLine ? oldLine++ : undefined,
      });
    else if (text.startsWith(" "))
      file.rows.push({
        text: text.slice(1),
        kind: "context",
        oldLine: oldLine ? oldLine++ : undefined,
        newLine: newLine ? newLine++ : undefined,
      });
    else if (text) file.rows.push({ text, kind: "hunk" });
  }
  flush();
  return files;
}

function CodeText({ text }: { text: string }) {
  // A small presentation lexer, never HTML injection or executable source.
  const tokens = text.split(
    /("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\/\/.*$|\b(?:const|let|var|function|return|if|else|for|func|type|struct|import|package|export|from|class|def|true|false|null|nil|async|await)\b|\b\d+(?:\.\d+)?\b)/g,
  );
  return (
    <>
      {tokens.map((token, index) =>
        index % 2 ? (
          <span
            key={index}
            className={
              token.startsWith("//")
                ? "trace-code-comment"
                : /^["']/.test(token)
                  ? "trace-code-string"
                  : /^\d/.test(token)
                    ? "trace-code-number"
                    : "trace-code-keyword"
            }
          >
            {token}
          </span>
        ) : (
          <Fragment key={index}>{token}</Fragment>
        ),
      )}
    </>
  );
}

export function TraceDiff({ patch }: { patch: string }) {
  const files = useMemo(() => parseTraceDiff(patch), [patch]);
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set());
  return (
    <div className="trace-diff">
      {files.map((file, index) => (
        <section className="trace-diff-file" key={`${index}:${file.name}`}>
          <div className="trace-diff-name" title={file.name}>
            {file.name.split(/[\\/]/).at(-1)}
          </div>
          <table aria-label={`${file.name} 的变更`}>
            <tbody>
              {(expanded.has(index) ? file.rows : file.rows.slice(0, 80)).map(
                (row, line) => (
                  <tr key={line} data-kind={row.kind}>
                    <td className="trace-diff-number">{row.oldLine}</td>
                    <td className="trace-diff-number">{row.newLine}</td>
                    <td className="trace-diff-sign">
                      {row.kind === "add"
                        ? "+"
                        : row.kind === "remove"
                          ? "−"
                          : ""}
                    </td>
                    <td>
                      <code>
                        <CodeText text={row.text} />
                      </code>
                    </td>
                  </tr>
                ),
              )}
            </tbody>
          </table>
          {file.rows.length > 80 ? (
            <button
              type="button"
              className="trace-diff-more"
              onClick={() =>
                setExpanded((previous) => {
                  const next = new Set(previous);
                  if (next.has(index)) next.delete(index);
                  else next.add(index);
                  return next;
                })
              }
            >
              {expanded.has(index)
                ? "收起长变更"
                : `展开已保留的全部 ${file.rows.length} 行`}
            </button>
          ) : null}
        </section>
      ))}
    </div>
  );
}
