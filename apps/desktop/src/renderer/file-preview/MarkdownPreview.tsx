import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode } from "react";
import { syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { classHighlighter, highlightTree } from "@lezer/highlight";
import { Eye, Pencil, Save } from "lucide-react";
import ReactMarkdown from "react-markdown";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import type {
  ProjectFilePreviewSaveResult,
  ProjectFilePreviewTextHandle,
} from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";
import { loadPreviewLanguage } from "./previewLanguage";

type HighlightSpan = { from: number; to: number; className: string };
type MarkdownViewMode = "preview" | "edit";

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
  onSave: (content: string) => Promise<ProjectFilePreviewSaveResult>;
};

export default function MarkdownPreview({
  preview,
  frameProps,
  onSave,
}: MarkdownPreviewProps) {
  const [mode, setMode] = useState<MarkdownViewMode>("preview");
  const [draft, setDraft] = useState(preview.content);
  const [savedContent, setSavedContent] = useState(preview.content);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string>();
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const draftRef = useRef(preview.content);
  const savedRef = useRef(preview.content);
  const onSaveRef = useRef(onSave);

  draftRef.current = draft;
  savedRef.current = savedContent;
  onSaveRef.current = onSave;

  useEffect(() => {
    setMode("preview");
    setDraft(preview.content);
    setSavedContent(preview.content);
    setSaveError(undefined);
    setSaving(false);
    draftRef.current = preview.content;
    savedRef.current = preview.content;
  }, [preview.previewId, preview.content]);

  const persist = useCallback(async (): Promise<boolean> => {
    const content = draftRef.current;
    if (content === savedRef.current) {
      setSaveError(undefined);
      return true;
    }
    setSaving(true);
    setSaveError(undefined);
    try {
      const result = await onSaveRef.current(content);
      if (!result.ok) {
        setSaveError(result.message);
        return false;
      }
      setSavedContent(content);
      savedRef.current = content;
      return true;
    } catch {
      setSaveError("暂时无法保存这个 Markdown 文件。");
      return false;
    } finally {
      setSaving(false);
    }
  }, []);

  useEffect(
    () => () => {
      if (draftRef.current === savedRef.current) return;
      void onSaveRef.current(draftRef.current);
    },
    [preview.previewId],
  );

  useEffect(() => {
    if (mode === "edit") editorRef.current?.focus();
  }, [mode]);

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

  const dirty = draft !== savedContent;

  async function requestPreviewMode(): Promise<void> {
    if (await persist()) setMode("preview");
  }

  function handleEditorKeyDown(event: ReactKeyboardEvent<HTMLTextAreaElement>): void {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
      event.preventDefault();
      void persist();
    }
  }

  return (
    <PreviewFrame
      {...frameProps}
      formatLabel="Markdown"
      formatActions={
        <>
          <button
            type="button"
            aria-label="预览 Markdown"
            title="预览"
            aria-pressed={mode === "preview"}
            onClick={() => void requestPreviewMode()}
          >
            <Eye size={15} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-label="编辑 Markdown"
            title="编辑"
            aria-pressed={mode === "edit"}
            onClick={() => setMode("edit")}
          >
            <Pencil size={15} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-label="保存 Markdown"
            title="保存"
            disabled={!dirty || saving}
            onClick={() => void persist()}
          >
            <Save size={15} aria-hidden="true" />
          </button>
          {saveError ? (
            <span
              className="project-file-markdown-preview__status is-error"
              role="alert"
              title={saveError}
            >
              {saveError}
            </span>
          ) : saving ? (
            <span className="project-file-markdown-preview__status">保存中</span>
          ) : dirty ? (
            <span className="project-file-markdown-preview__status">未保存</span>
          ) : null}
        </>
      }
    >
      {mode === "edit" ? (
        <textarea
          ref={editorRef}
          className="project-file-markdown-editor"
          data-markdown-editor=""
          aria-label={`${preview.displayName} Markdown 编辑`}
          spellCheck={false}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={() => void persist()}
          onKeyDown={handleEditorKeyDown}
        />
      ) : (
        <article
          className="project-file-markdown-preview aw-text-selectable"
          data-preview-kind="markdown"
          data-markdown-mode="preview"
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
            {draft}
          </ReactMarkdown>
        </article>
      )}
    </PreviewFrame>
  );
}
