type MarkdownAnswerStatus = "active" | "done" | "error";

type MarkdownAnswerProps = {
  markdown: string;
  status?: MarkdownAnswerStatus;
};

type MarkdownBlock =
  | { kind: "paragraph"; text: string }
  | { kind: "list"; ordered: boolean; items: string[] };

function renderInlineCode(text: string) {
  const parts = text.split(/(`[^`]+`)/g).filter(Boolean);
  return parts.map((part, index) => {
    if (part.startsWith("`") && part.endsWith("`") && part.length > 1) {
      return (
        <code
          key={`${part}:${index}`}
          style={{
            padding: "1px 5px",
            borderRadius: "var(--aw-radius-control)",
            background: "var(--aw-bg-fill-main)",
            color: "var(--aw-text-primary)",
            fontFamily: "var(--aw-font-mono)",
            fontSize: "0.92em",
          }}
        >
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

  const flushParagraph = () => {
    if (paragraph.length === 0) {
      return;
    }
    blocks.push({ kind: "paragraph", text: paragraph.join(" ") });
    paragraph = [];
  };

  const flushList = () => {
    if (!list) {
      return;
    }
    blocks.push({ kind: "list", ordered: list.ordered, items: list.items });
    list = undefined;
  };

  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line) {
      flushParagraph();
      flushList();
      continue;
    }

    const unordered = line.match(/^[-*]\s+(.+)$/);
    const ordered = line.match(/^\d+[.)]\s+(.+)$/);
    if (unordered || ordered) {
      flushParagraph();
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
    paragraph.push(line);
  }

  flushParagraph();
  flushList();
  return blocks;
}

const statusColors: Record<MarkdownAnswerStatus, string> = {
  active: "var(--aw-status-running)",
  done: "var(--aw-status-success)",
  error: "var(--aw-status-danger)",
};

export function MarkdownAnswer({
  markdown,
  status = "active",
}: MarkdownAnswerProps) {
  const blocks = parseMarkdown(markdown || "Answer is streaming");

  return (
    <article
      style={{
        display: "grid",
        gap: "12px",
        width: "100%",
        maxWidth: "100%",
        boxSizing: "border-box",
        minWidth: 0,
        padding: "16px",
        border: `1px solid ${
          status === "error" ? "var(--aw-error-soft-border)" : "var(--aw-border-main)"
        }`,
        borderRadius: "var(--aw-radius-panel)",
        background: status === "error" ? "var(--aw-error-soft-bg)" : "var(--aw-surface-raised)",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "12px",
          minWidth: 0,
        }}
      >
        <h2
          style={{
            margin: 0,
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-sm)",
            fontWeight: "var(--aw-font-weight-bold)",
            lineHeight: "var(--aw-line-height-sm)",
          }}
        >
          Answer
        </h2>
        <span
          style={{
            flex: "0 0 auto",
            color: statusColors[status],
            fontSize: "var(--aw-font-size-xs)",
            fontWeight: "var(--aw-font-weight-semibold)",
            lineHeight: "var(--aw-line-height-xs)",
            whiteSpace: "nowrap",
          }}
        >
          {status}
        </span>
      </div>
      <div
        style={{
          display: "grid",
          gap: "8px",
          maxHeight: "420px",
          overflow: "auto",
          color: "var(--aw-text-primary)",
          fontSize: "var(--aw-font-size-sm)",
          lineHeight: "var(--aw-line-height-base)",
          overflowWrap: "anywhere",
        }}
      >
        {blocks.map((block, index) => {
          if (block.kind === "list") {
            const ListTag = block.ordered ? "ol" : "ul";
            return (
              <ListTag
                key={`list:${index}`}
                style={{
                  display: "grid",
                  gap: "4px",
                  margin: 0,
                  paddingLeft: "20px",
                }}
              >
                {block.items.map((item, itemIndex) => (
                  <li key={`${item}:${itemIndex}`}>{renderInlineCode(item)}</li>
                ))}
              </ListTag>
            );
          }
          return (
            <p key={`paragraph:${index}`} style={{ margin: 0 }}>
              {renderInlineCode(block.text)}
            </p>
          );
        })}
      </div>
    </article>
  );
}
