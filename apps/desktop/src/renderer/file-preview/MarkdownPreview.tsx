import { syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { classHighlighter, highlightTree } from "@lezer/highlight";
import type {
  ReactNode
} from "react";
import {
  createElement,
  useEffect,
  useMemo,
  useRef,
  useState
} from "react";
import ReactMarkdown from "react-markdown";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import type {
  FileLineComment,
  ProjectFilePreviewSaveResult,
  ProjectFilePreviewTextHandle,
} from "../../shared/runtime";
import CodePreview, { type CodePreviewLineSelection } from "./CodePreview";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";
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
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
  onSave?: (content: string) => Promise<ProjectFilePreviewSaveResult>;
  fileComments?: FileLineComment[];
  onCreateComment?: (selection: CodePreviewLineSelection, comment: string) => void;
  onAddTextSelection?: (selection: CodePreviewLineSelection) => void;
  onDeleteComment?: (commentId: string) => void;
  commentDisabled?: boolean;
  lineRequest?: { id: string; startLine: number; endLine?: number };
};

type MarkdownPreviewSelection = CodePreviewLineSelection & {
  top: number;
  left: number;
};

function positionedMarkdownElement(tagName: string) {
  return function PositionedMarkdownElement({ node, children, ...props }: any) {
    const position = node?.position;
    return createElement(tagName, {
      ...props,
      ...(position?.start?.line ? { "data-source-start-line": position.start.line } : {}),
      ...(position?.end?.line ? { "data-source-end-line": position.end.line } : {}),
    }, children);
  };
}

const positionedMarkdownComponents = {
  p: positionedMarkdownElement("p"),
  h1: positionedMarkdownElement("h1"),
  h2: positionedMarkdownElement("h2"),
  h3: positionedMarkdownElement("h3"),
  h4: positionedMarkdownElement("h4"),
  h5: positionedMarkdownElement("h5"),
  h6: positionedMarkdownElement("h6"),
  li: positionedMarkdownElement("li"),
  blockquote: positionedMarkdownElement("blockquote"),
  pre: positionedMarkdownElement("pre"),
  table: positionedMarkdownElement("table"),
  code: MarkdownCodeBlock,
};

function sourceLineElement(node: Node | null, root: HTMLElement): HTMLElement | null {
  const element = node instanceof HTMLElement ? node : node?.parentElement;
  const positioned = element?.closest<HTMLElement>("[data-source-start-line][data-source-end-line]");
  return positioned && root.contains(positioned) ? positioned : null;
}

export default function MarkdownPreview({
  preview,
  frameProps,
  onSave,
  fileComments = [],
  onCreateComment,
  onAddTextSelection,
  onDeleteComment,
  commentDisabled = false,
  lineRequest,
}: MarkdownPreviewProps) {
  const [mode, setMode] = useState<"preview" | "source">("preview");
  const [content, setContent] = useState(preview.content);
  const [selection, setSelection] = useState<MarkdownPreviewSelection>();
  const [commentRequest, setCommentRequest] = useState<{
    id: string;
    startLine: number;
    endLine: number;
  }>();
  const previewRef = useRef<HTMLElement>(null);

  useEffect(() => {
    setMode("preview");
    setContent(preview.content);
    setSelection(undefined);
    setCommentRequest(undefined);
  }, [preview.previewId, preview.content]);

  useEffect(() => { if (lineRequest) setMode("source"); }, [lineRequest?.id, preview.previewId]);

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

  const sourceToggle = (
    <button
      type="button"
      className="project-file-markdown-source-toggle"
      onClick={() => {
        setSelection(undefined);
        setMode(mode === "preview" ? "source" : "preview");
      }}
    >
      {mode === "preview" ? "查看源代码" : "查看预览"}
    </button>
  );

  if (mode === "source") {
    return (
      <CodePreview
        lineRequest={lineRequest}
        preview={{ ...preview, content }}
        frameProps={frameProps}
        editable={Boolean(onSave)}
        onSave={async (nextContent) => {
          const result = onSave
            ? await onSave(nextContent)
            : { ok: false as const, code: "INVALID_REQUEST" as const, message: "无法保存此文件。" };
          if (result.ok) setContent(nextContent);
          return result;
        }}
        leadingFormatActions={sourceToggle}
        fileComments={fileComments.filter((item) => item.kind !== "selection")}
        onCreateComment={onCreateComment}
        onDeleteComment={onDeleteComment}
        commentDisabled={commentDisabled}
        commentRequest={commentRequest}
        onCommentRequestHandled={(id) => {
          setCommentRequest((current) => current?.id === id ? undefined : current);
        }}
      />
    );
  }

  const updateSelection = () => {
    const root = previewRef.current;
    const browserSelection = window.getSelection();
    if (!root || !browserSelection || browserSelection.rangeCount === 0 || browserSelection.isCollapsed) {
      setSelection(undefined);
      return;
    }
    const range = browserSelection.getRangeAt(0);
    if (!root.contains(range.commonAncestorContainer)) {
      setSelection(undefined);
      return;
    }
    const startElement = sourceLineElement(range.startContainer, root);
    const endElement = sourceLineElement(range.endContainer, root);
    const selectedText = browserSelection.toString().trim();
    const startLine = Number(startElement?.dataset.sourceStartLine);
    const endLine = Number(endElement?.dataset.sourceEndLine);
    if (!selectedText || !Number.isInteger(startLine) || !Number.isInteger(endLine)) {
      setSelection(undefined);
      return;
    }
    const rect = range.getBoundingClientRect();
    const rootRect = root.getBoundingClientRect();
    setSelection({
      startLine: Math.min(startLine, endLine),
      endLine: Math.max(startLine, endLine),
      selectedText: content.split(/\r?\n/).slice(Math.min(startLine, endLine) - 1, Math.max(startLine, endLine)).join("\n"),
      top: rect.bottom - rootRect.top + root.scrollTop + 8,
      left: Math.max(8, Math.min(rect.left - rootRect.left, root.clientWidth - 240)),
    });
  };

  return (
    <PreviewFrame {...frameProps} formatLabel="Markdown" formatActions={sourceToggle}>
      <article
        ref={previewRef}
        className="project-file-markdown-preview project-file-markdown-selectable aw-text-selectable"
        data-preview-kind="markdown"
        data-markdown-mode="preview"
        aria-label={`${preview.displayName} Markdown 预览`}
        onMouseUp={() => queueMicrotask(updateSelection)}
        onKeyUp={updateSelection}
      >
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          rehypePlugins={[[rehypeSanitize, sanitizeSchema]]}
          components={{
            ...positionedMarkdownComponents,
            a: ({ children, node, ...props }: any) => (
              <a
                {...props}
                data-source-start-line={node?.position?.start?.line}
                data-source-end-line={node?.position?.end?.line}
                target="_blank"
                rel="noreferrer noopener"
              >
                {children}
              </a>
            ),
          }}
        >
          {content}
        </ReactMarkdown>
        {selection ? (
          <span
            className="project-file-markdown-selection-actions"
            style={{ top: selection.top, left: selection.left }}
            onMouseDown={(event) => event.preventDefault()}
          >
            <button type="button" onClick={() => onAddTextSelection?.(selection)}>添加到对话</button>
            <button
              type="button"
              disabled={commentDisabled || !onCreateComment}
              onClick={() => {
                setCommentRequest({
                  id: globalThis.crypto?.randomUUID?.() ?? `markdown-comment-${Date.now()}`,
                  startLine: selection.startLine,
                  endLine: selection.endLine,
                });
                setMode("source");
              }}
            >
              评论
            </button>
          </span>
        ) : null}
      </article>
    </PreviewFrame>
  );
}
