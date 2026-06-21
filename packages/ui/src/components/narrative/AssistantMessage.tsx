type AssistantMessageStatus = "active" | "done" | "error";

type AssistantMessageProps = {
  markdown: string;
  status?: AssistantMessageStatus;
};

type MarkdownBlock =
  | { kind: "paragraph"; text: string }
  | { kind: "list"; ordered: boolean; items: string[] }
  | { kind: "table"; headers: string[]; rows: string[][] };

function renderInlineCode(text: string) {
  const parts = text.split(/(`[^`]+`)/g).filter(Boolean);
  return parts.map((part, index) => {
    if (part.startsWith("`") && part.endsWith("`") && part.length > 1) {
      return (
        <code key={`${part}:${index}`} className="assistant-message-code">
          {part.slice(1, -1)}
        </code>
      );
    }
    return part;
  });
}

function parseMarkdown(markdown: string): MarkdownBlock[] {
  const lines = markdown.replace(/\r\n/g, "\n").split("\n");
  const blocks: MarkdownBlock[] = [];
  let paragraph: string[] = [];
  let list: { ordered: boolean; items: string[] } | undefined;
  let table: { headers: string[]; rows: string[][] } | undefined;

  const flushParagraph = () => {
    if (paragraph.length === 0) return;
    blocks.push({ kind: "paragraph", text: paragraph.join(" ") });
    paragraph = [];
  };

  const flushList = () => {
    if (!list) return;
    blocks.push({ kind: "list", ordered: list.ordered, items: list.items });
    list = undefined;
  };

  const flushTable = () => {
    if (!table) return;
    blocks.push({ kind: "table", headers: table.headers, rows: table.rows });
    table = undefined;
  };

  const parseTableRow = (line: string) =>
    line
      .replace(/^\|/, "")
      .replace(/\|$/, "")
      .split("|")
      .map((cell) => cell.trim());

  const isTableDivider = (line: string) =>
    /^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?$/.test(line);

  for (let index = 0; index < lines.length; index += 1) {
    const rawLine = lines[index];
    const line = rawLine.trim();
    if (!line) {
      flushParagraph();
      flushList();
      flushTable();
      continue;
    }

    if (
      line.includes("|") &&
      index + 1 < lines.length &&
      isTableDivider(lines[index + 1].trim())
    ) {
      flushParagraph();
      flushList();
      flushTable();
      const headers = parseTableRow(line);
      const rows: string[][] = [];
      index += 1;
      while (index + 1 < lines.length) {
        const nextLine = lines[index + 1].trim();
        if (!nextLine || !nextLine.includes("|") || isTableDivider(nextLine)) {
          break;
        }
        rows.push(parseTableRow(nextLine));
        index += 1;
      }
      table = { headers, rows };
      flushTable();
      continue;
    }

    const unordered = line.match(/^[-*]\s+(.+)$/);
    const ordered = line.match(/^\d+[.)]\s+(.+)$/);
    if (unordered || ordered) {
      flushParagraph();
      flushTable();
      const nextOrdered = Boolean(ordered);
      const item = (ordered?.[1] ?? unordered?.[1] ?? "").trim();
      if (!list || list.ordered !== nextOrdered) {
        flushList();
        list = { ordered: nextOrdered, items: [] };
      }
      list.items.push(item);
      continue;
    }

    flushList();
    flushTable();
    paragraph.push(line);
  }

  flushParagraph();
  flushList();
  flushTable();
  return blocks;
}

export function AssistantMessage({
  markdown,
  status = "active",
}: AssistantMessageProps) {
  const blocks = parseMarkdown(markdown || "Streaming response...");

  return (
    <article className={`assistant-message assistant-message-${status}`}>
      {blocks.map((block, index) => {
        if (block.kind === "list") {
          const ListTag = block.ordered ? "ol" : "ul";
          return (
            <ListTag key={`list:${index}`}>
              {block.items.map((item, itemIndex) => (
                <li key={`${item}:${itemIndex}`}>{renderInlineCode(item)}</li>
              ))}
            </ListTag>
          );
        }

        if (block.kind === "table") {
          return (
            <div key={`table:${index}`} className="assistant-message-table-wrap">
              <table className="assistant-message-table">
                <thead>
                  <tr>
                    {block.headers.map((header, headerIndex) => (
                      <th key={`${header}:${headerIndex}`}>{renderInlineCode(header)}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {block.rows.map((row, rowIndex) => (
                    <tr key={`row:${rowIndex}`}>
                      {block.headers.map((_, cellIndex) => (
                        <td key={`cell:${rowIndex}:${cellIndex}`}>
                          {renderInlineCode(row[cellIndex] ?? "")}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          );
        }

        return <p key={`paragraph:${index}`}>{renderInlineCode(block.text)}</p>;
      })}
    </article>
  );
}
