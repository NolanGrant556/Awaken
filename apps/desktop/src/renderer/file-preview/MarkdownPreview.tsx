import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type {
  ClipboardEvent as ReactClipboardEvent,
  KeyboardEvent as ReactKeyboardEvent,
  ReactNode,
} from "react";
import { syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { classHighlighter, highlightTree } from "@lezer/highlight";
import { Eye, Pencil, Save } from "@awaken/design-system";
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

function markdownText(value: string): string {
  return value
    .replaceAll("\u00a0", " ")
    .replace(/\\/g, "\\\\")
    .replace(/([*_\[\]`])/g, "\\$1");
}

function inlineMarkdown(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) {
    return markdownText(node.textContent ?? "");
  }
  if (!(node instanceof HTMLElement)) return "";
  const children = () => [...node.childNodes].map(inlineMarkdown).join("");
  switch (node.tagName.toLocaleLowerCase()) {
    case "br":
      return "  \n";
    case "strong":
    case "b":
      return `**${children()}**`;
    case "em":
    case "i":
      return `*${children()}*`;
    case "del":
    case "s":
      return `~~${children()}~~`;
    case "code": {
      const value = node.textContent ?? "";
      const fence = value.includes("`") ? "``" : "`";
      return `${fence}${value}${fence}`;
    }
    case "a": {
      const label = children() || markdownText(node.getAttribute("href") ?? "");
      const href = node.getAttribute("href") ?? "";
      return href ? `[${label}](${href})` : label;
    }
    case "img": {
      const alt = markdownText(node.getAttribute("alt") ?? "");
      const src = node.getAttribute("src") ?? "";
      return src ? `![${alt}](${src})` : alt;
    }
    case "input":
      return node.getAttribute("type") === "checkbox"
        ? `${(node as HTMLInputElement).checked ? "[x]" : "[ ]"} `
        : "";
    default:
      return children();
  }
}

function listMarkdown(list: HTMLElement, depth = 0): string {
  const ordered = list.tagName.toLocaleLowerCase() === "ol";
  const start = Number(list.getAttribute("start") || "1");
  return [...list.children]
    .filter((child): child is HTMLElement => child instanceof HTMLElement && child.tagName === "LI")
    .map((item, index) => {
      const nestedLists = [...item.children].filter(
        (child): child is HTMLElement =>
          child instanceof HTMLElement && (child.tagName === "UL" || child.tagName === "OL"),
      );
      const content = [...item.childNodes]
        .filter((child) => !nestedLists.includes(child as HTMLElement))
        .map((child) =>
          child instanceof HTMLElement && ["P", "DIV"].includes(child.tagName)
            ? [...child.childNodes].map(inlineMarkdown).join("")
            : inlineMarkdown(child),
        )
        .join("")
        .trim();
      const marker = ordered ? `${start + index}. ` : "- ";
      const indent = "  ".repeat(depth);
      const nested = nestedLists
        .map((child) => listMarkdown(child, depth + 1))
        .filter(Boolean)
        .join("\n");
      return `${indent}${marker}${content}${nested ? `\n${nested}` : ""}`;
    })
    .join("\n");
}

function tableMarkdown(table: HTMLElement): string {
  const rows = [...table.querySelectorAll("tr")].map((row) =>
    [...row.querySelectorAll(":scope > th, :scope > td")].map((cell) =>
      [...cell.childNodes]
        .map(inlineMarkdown)
        .join("")
        .trim()
        .replaceAll("|", "\\|"),
    ),
  );
  if (rows.length === 0) return "";
  const width = Math.max(...rows.map((row) => row.length));
  const normalized = rows.map((row) => [
    ...row,
    ...Array.from({ length: width - row.length }, () => ""),
  ]);
  const line = (cells: string[]) => `| ${cells.join(" | ")} |`;
  return [
    line(normalized[0]),
    line(Array.from({ length: width }, () => "---")),
    ...normalized.slice(1).map(line),
  ].join("\n");
}

function blockMarkdown(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return markdownText(node.textContent ?? "").trim();
  if (!(node instanceof HTMLElement)) return "";
  const tag = node.tagName.toLocaleLowerCase();
  if (/^h[1-6]$/.test(tag)) {
    return `${"#".repeat(Number(tag[1]))} ${[...node.childNodes].map(inlineMarkdown).join("").trim()}`;
  }
  if (tag === "p") return [...node.childNodes].map(inlineMarkdown).join("").trim();
  if (tag === "ul" || tag === "ol") return listMarkdown(node);
  if (tag === "blockquote") {
    return [...node.childNodes]
      .map(blockMarkdown)
      .filter(Boolean)
      .join("\n\n")
      .split("\n")
      .map((line) => `> ${line}`.trimEnd())
      .join("\n");
  }
  if (tag === "pre") {
    const code = node.querySelector("code");
    const language = /^language-(.+)$/.exec(code?.className ?? "")?.[1] ?? "";
    const source = (code?.textContent ?? node.textContent ?? "").replace(/\n$/, "");
    const longestFence = Math.max(3, ...[...source.matchAll(/`+/g)].map((match) => match[0].length + 1));
    const fence = "`".repeat(longestFence);
    return `${fence}${language}\n${source}\n${fence}`;
  }
  if (tag === "table") return tableMarkdown(node);
  if (tag === "hr") return "---";
  if (tag === "div" || tag === "section" || tag === "article") {
    const hasBlockChildren = [...node.children].some((child) =>
      /^(H[1-6]|P|DIV|UL|OL|BLOCKQUOTE|PRE|TABLE|HR)$/.test(child.tagName),
    );
    return hasBlockChildren
      ? [...node.childNodes].map(blockMarkdown).filter(Boolean).join("\n\n")
      : [...node.childNodes].map(inlineMarkdown).join("").trim();
  }
  return [...node.childNodes].map(inlineMarkdown).join("").trim();
}

export function markdownFromEditable(root: HTMLElement): string {
  const content = [...root.childNodes]
    .map(blockMarkdown)
    .filter((part) => part.trim())
    .join("\n\n")
    .replace(/\n{3,}/g, "\n\n")
    .trimEnd();
  return content ? `${content}\n` : "";
}

export type MarkdownPreviewProps = {
  preview: ProjectFilePreviewTextHandle;
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
  onSave?: (content: string) => Promise<ProjectFilePreviewSaveResult>;
};

export default function MarkdownPreview({
  preview,
  frameProps,
  onSave,
}: MarkdownPreviewProps) {
  const [mode, setMode] = useState<MarkdownViewMode>("preview");
  const [renderedContent, setRenderedContent] = useState(preview.content);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string>();
	const [saveConflict, setSaveConflict] = useState<{
		currentContent: string;
		submittedContent: string;
		showingCurrent: boolean;
	}>();
  const previewRef = useRef<HTMLElement>(null);
  const editorRef = useRef<HTMLElement>(null);
  const editorTemplateRef = useRef<HTMLDivElement>(null);
  const scrollPositionRef = useRef<
    { previewId: string; top: number } | undefined
  >(undefined);
  const draftRef = useRef(preview.content);
  const savedRef = useRef(preview.content);
  const onSaveRef = useRef(onSave);

  onSaveRef.current = onSave;

  useEffect(() => {
    setMode("preview");
    setRenderedContent(preview.content);
    setDirty(false);
    setSaveError(undefined);
	setSaveConflict(undefined);
    setSaving(false);
    draftRef.current = preview.content;
    savedRef.current = preview.content;
  }, [preview.previewId, preview.content]);

  const persist = useCallback(async (): Promise<boolean> => {
    if (!onSaveRef.current) return true;
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
		if (result.code === "FILE_CHANGED" && result.currentContent !== undefined && result.submittedContent !== undefined) {
			setSaveConflict({
				currentContent: result.currentContent,
				submittedContent: result.submittedContent,
				showingCurrent: false,
			});
		}
        return false;
      }
      savedRef.current = content;
      setDirty(false);
		setSaveConflict(undefined);
      return true;
    } catch {
      setSaveError("暂时无法保存这个 Markdown 文件。");
      return false;
    } finally {
      setSaving(false);
    }
  }, []);

	function toggleConflictVersion(): void {
		if (!saveConflict) return;
		const nextContent = saveConflict.showingCurrent
			? saveConflict.submittedContent
			: saveConflict.currentContent;
		const showingCurrent = !saveConflict.showingCurrent;
		setRenderedContent(nextContent);
		draftRef.current = nextContent;
		if (showingCurrent) {
			savedRef.current = nextContent;
			setDirty(false);
		} else {
			setDirty(nextContent !== savedRef.current);
		}
		setSaveConflict({ ...saveConflict, showingCurrent });
		setSaveError(showingCurrent ? "已加载磁盘版本；本地草稿仍可恢复。" : "已恢复本地草稿；保存前请重新加载并合并外部修改。" );
	}

  useEffect(
    () => () => {
      if (!onSaveRef.current || draftRef.current === savedRef.current) return;
      void onSaveRef.current(draftRef.current);
    },
    [preview.previewId],
  );

  useLayoutEffect(() => {
    const target = mode === "edit" ? editorRef.current : previewRef.current;
    if (!target) return;
    if (mode === "edit") {
      if (!editorTemplateRef.current) return;
      target.innerHTML = editorTemplateRef.current.innerHTML;
      target.focus({ preventScroll: true });
    }
    const scrollPosition = scrollPositionRef.current;
    if (scrollPosition?.previewId === preview.previewId) {
      target.scrollTop = scrollPosition.top;
    }
  }, [mode, preview.previewId, renderedContent]);

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

  async function requestPreviewMode(): Promise<void> {
    if (await persist()) {
      rememberScrollPosition(editorRef.current);
      setRenderedContent(draftRef.current);
      setMode("preview");
    }
  }

  function enterEditMode(): void {
    rememberScrollPosition(previewRef.current);
    setRenderedContent(draftRef.current);
    setMode("edit");
  }

  function rememberScrollPosition(element: HTMLElement | null): void {
    if (!element) return;
    scrollPositionRef.current = {
      previewId: preview.previewId,
      top: element.scrollTop,
    };
  }

  function handleEditorInput(): void {
    const editor = editorRef.current;
    if (!editor) return;
    const content = markdownFromEditable(editor);
    draftRef.current = content;
    setDirty(content !== savedRef.current);
    setSaveError(undefined);
  }

  function handleEditorKeyDown(event: ReactKeyboardEvent<HTMLElement>): void {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
      event.preventDefault();
      void persist();
    }
  }

  function handleEditorPaste(event: ReactClipboardEvent<HTMLElement>): void {
    event.preventDefault();
    const selection = window.getSelection();
    if (!selection || selection.rangeCount === 0) return;
    const range = selection.getRangeAt(0);
    range.deleteContents();
    const fragment = document.createDocumentFragment();
    const lines = event.clipboardData.getData("text/plain").split(/\r?\n/);
    let finalNode: Node | undefined;
    lines.forEach((line, index) => {
      if (index > 0) {
        const breakNode = document.createElement("br");
        fragment.append(breakNode);
        finalNode = breakNode;
      }
      const textNode = document.createTextNode(line);
      fragment.append(textNode);
      finalNode = textNode;
    });
    range.insertNode(fragment);
    if (finalNode) {
      range.setStartAfter(finalNode);
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
    }
    handleEditorInput();
  }

  return (
    <PreviewFrame
      {...frameProps}
      formatLabel="Markdown"
      formatActions={onSave ? (
        <>
          <div
            className="project-file-markdown-mode-switch"
            role="group"
            aria-label="Markdown 显示模式"
          >
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
              onClick={enterEditMode}
            >
              <Pencil size={15} aria-hidden="true" />
            </button>
          </div>
          <div
            className={`project-file-markdown-save-state${
              saveError
                ? " is-error"
                : saving
                  ? " is-saving"
                  : dirty
                    ? " is-dirty"
                    : ""
            }`}
          >
            <button
              type="button"
              className="project-file-markdown-save-button"
              aria-label="保存 Markdown"
              title="保存"
              disabled={!dirty || saving}
              onClick={() => void persist()}
            >
              <Save size={15} aria-hidden="true" />
            </button>
			{saveConflict ? (
				<button
					type="button"
					className="project-file-markdown-conflict-button"
					onClick={toggleConflictVersion}
				>
					{saveConflict.showingCurrent ? "恢复本地草稿" : "重新加载磁盘版本"}
				</button>
			) : null}
            {saveError ? (
              <span
                className="project-file-markdown-preview__status"
                role="alert"
                title={saveError}
              >
                {saveError}
              </span>
            ) : saving ? (
              <span className="project-file-markdown-preview__status" role="status">
                保存中
              </span>
            ) : dirty ? (
              <span className="project-file-markdown-preview__status" role="status">
                未保存
              </span>
            ) : null}
          </div>
        </>
      ) : undefined}
    >
      {mode === "edit" ? (
        <>
          <div ref={editorTemplateRef} hidden aria-hidden="true">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[[rehypeSanitize, sanitizeSchema]]}
              components={{ code: MarkdownCodeBlock }}
            >
              {renderedContent}
            </ReactMarkdown>
          </div>
          <article
            ref={editorRef}
            className="project-file-markdown-preview project-file-markdown-editor aw-text-selectable"
            data-markdown-editor=""
            data-preview-kind="markdown"
            data-markdown-mode="edit"
            aria-label={`${preview.displayName} Markdown 编辑`}
            contentEditable
            suppressContentEditableWarning
            spellCheck
            onInput={handleEditorInput}
            onPaste={handleEditorPaste}
            onBlur={() => void persist()}
            onKeyDown={handleEditorKeyDown}
            onClick={(event) => {
              if (event.target instanceof HTMLAnchorElement) event.preventDefault();
            }}
          />
        </>
      ) : (
        <article
          ref={previewRef}
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
            {renderedContent}
          </ReactMarkdown>
        </article>
      )}
    </PreviewFrame>
  );
}
