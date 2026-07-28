import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { classHighlighter, highlightTree } from "@lezer/highlight";
import ReactMarkdown from "react-markdown";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import type { ProjectFilePreviewTextHandle } from "../../shared/runtime";
import { loadPreviewLanguage } from "./previewLanguage";

type HighlightSpan = { from: number; to: number; className: string };

function highlightedCode(source: string, spans: HighlightSpan[]): ReactNode[] {
  const result: ReactNode[] = [];
  let position = 0;
  for (const span of spans) {
    if (span.from > position) result.push(source.slice(position, span.from));
    result.push(
      <span key={`${span.from}-${span.to}`} className={span.className}>
        {source.slice(span.from, span.to)}
      </span>,
    );
    position = span.to;
  }
  if (position < source.length) result.push(source.slice(position));
  return result;
}

function MarkdownCodeBlock({
  className,
  children,
}: {
  className?: string;
  children?: ReactNode;
}) {
  const source = String(children ?? "").replace(/\n$/, "");
  const languageName = /^language-(.+)$/.exec(className ?? "")?.[1];
  const [spans, setSpans] = useState<HighlightSpan[]>([]);

  useEffect(() => {
    let cancelled = false;
    if (!languageName) {
      setSpans([]);
      return;
    }
    void loadPreviewLanguage(`snippet.${languageName}`, languageName).then(
      (language) => {
        if (cancelled || !language) return;
        const state = EditorState.create({ doc: source, extensions: [language] });
        const next: HighlightSpan[] = [];
        highlightTree(syntaxTree(state), classHighlighter, (from, to, classes) => {
          next.push({ from, to, className: classes });
        });
        setSpans(next);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [languageName, source]);

  if (!languageName && !source.includes("\n")) {
    return <code className={className}>{children}</code>;
  }
  return (
    <code className={className}>{highlightedCode(source, spans)}</code>
  );
}

export type MarkdownPreviewProps = {
  preview: ProjectFilePreviewTextHandle;
};

export default function MarkdownPreview({ preview }: MarkdownPreviewProps) {
  const sanitizeSchema = useMemo(
    () => ({
      ...defaultSchema,
      attributes: {
        ...defaultSchema.attributes,
        code: [
          ...(defaultSchema.attributes?.code ?? []),
          ["className", /^language-[A-Za-z0-9_+-]+$/],
        ],
      },
    }),
    [],
  );
  return (
    <article
      className="project-file-markdown-preview aw-text-selectable"
      data-preview-kind="markdown"
      aria-label={`${preview.displayName} Markdown 预览`}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[[rehypeSanitize, sanitizeSchema]]}
        components={{
          a: ({ children, ...props }) => (
            <a {...props} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
          code: MarkdownCodeBlock,
        }}
      >
        {preview.content}
      </ReactMarkdown>
    </article>
  );
}
