import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { AwakenIcon } from "@awaken/design-system";
import { defaultKeymap } from "@codemirror/commands";
import {
  bracketMatching,
  defaultHighlightStyle,
  syntaxHighlighting,
} from "@codemirror/language";
import { Compartment, EditorState, RangeSet, StateEffect, StateField } from "@codemirror/state";
import {
  Decoration,
  type DecorationSet,
  EditorView,
  GutterMarker,
  gutterLineClass,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  WidgetType,
} from "@codemirror/view";
import type {
  FileLineComment,
  ProjectFilePreviewSaveResult,
  ProjectFilePreviewTextHandle,
} from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";
import { loadPreviewLanguage } from "./previewLanguage";

export type CodePreviewProps = {
  preview: ProjectFilePreviewTextHandle;
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
  fileComments?: FileLineComment[];
  onCreateComment?: (selection: CodePreviewLineSelection, comment: string) => void;
  onDeleteComment?: (commentId: string) => void;
  commentDisabled?: boolean;
  editable?: boolean;
  onSave?: (content: string) => Promise<ProjectFilePreviewSaveResult>;
  commentRequest?: { id: string; startLine: number; endLine: number };
  onCommentRequestHandled?: (id: string) => void;
  leadingFormatActions?: ReactNode;
};

export type CodePreviewLineSelection = {
  startLine: number;
  endLine: number;
  selectedText: string;
};

type CommentWidgetConfig = {
  comments: FileLineComment[];
  selection?: CodePreviewLineSelection;
  editing?: CodePreviewLineSelection;
  onSubmit: (selection: CodePreviewLineSelection, comment: string) => void;
  onCancel: () => void;
  onDelete?: (commentId: string) => void;
};

const setCommentWidgetConfig = StateEffect.define<CommentWidgetConfig>();

class FileLineCommentWidget extends WidgetType {
  constructor(
    private readonly line: number,
    private readonly comments: FileLineComment[],
    private readonly editing: CodePreviewLineSelection | undefined,
    private readonly callbacks: Pick<CommentWidgetConfig, "onSubmit" | "onCancel" | "onDelete">,
  ) {
    super();
  }

  eq(other: FileLineCommentWidget): boolean {
    return this.line === other.line &&
      this.editing?.startLine === other.editing?.startLine &&
      this.editing?.endLine === other.editing?.endLine &&
      this.comments.length === other.comments.length &&
      this.comments.every((comment, index) => {
        const candidate = other.comments[index];
        return candidate?.id === comment.id && candidate.comment === comment.comment &&
          candidate.validationState === comment.validationState;
      });
  }

  toDOM(): HTMLElement {
    const root = document.createElement("section");
    root.className = "project-file-line-comments";
    root.setAttribute("aria-label", `第 ${this.line} 行文件评论`);

    for (const comment of this.comments) {
      const card = document.createElement("article");
      card.className = `project-file-line-comment-card${
        comment.validationState === "stale" ? " is-stale" : ""
      }`;
      const header = document.createElement("header");
      const author = document.createElement("strong");
      author.textContent = "你";
      const location = document.createElement("span");
      location.textContent = lineRangeLabel(comment.startLine, comment.endLine);
      header.append(author, location);
      const body = document.createElement("p");
      body.textContent = comment.comment;
      card.append(header, body);
      if (comment.validationState === "stale") {
        const stale = document.createElement("span");
        stale.className = "project-file-line-comment-stale";
        stale.textContent = "文件已变化，需要重新选择";
        card.append(stale);
      }
      if (this.callbacks.onDelete) {
        const remove = document.createElement("button");
        remove.type = "button";
        remove.className = "project-file-line-comment-delete";
        remove.textContent = "删除";
        remove.setAttribute("aria-label", `删除 ${lineRangeLabel(comment.startLine, comment.endLine)} 评论`);
        remove.addEventListener("click", () => this.callbacks.onDelete?.(comment.id));
        card.append(remove);
      }
      root.append(card);
    }

    if (this.editing) {
      const form = document.createElement("div");
      form.className = "project-file-line-comment-editor";
      const header = document.createElement("header");
      const author = document.createElement("strong");
      author.textContent = "你";
      const location = document.createElement("span");
      location.textContent = lineRangeLabel(this.editing.startLine, this.editing.endLine);
      header.append(author, location);
      const textarea = document.createElement("textarea");
      textarea.placeholder = "添加评论";
      textarea.setAttribute("aria-label", `${lineRangeLabel(this.editing.startLine, this.editing.endLine)} 评论内容`);
      const error = document.createElement("span");
      error.className = "project-file-line-comment-error";
      error.setAttribute("role", "status");
      const actions = document.createElement("div");
      actions.className = "project-file-line-comment-actions";
      const cancel = document.createElement("button");
      cancel.type = "button";
      cancel.textContent = "取消";
      cancel.addEventListener("click", this.callbacks.onCancel);
      const submit = document.createElement("button");
      submit.type = "button";
      submit.textContent = "评论";
      submit.disabled = true;
      const refreshValidity = () => {
        const value = textarea.value.trim();
        const tooLarge = new TextEncoder().encode(value).byteLength > 4096;
        submit.disabled = value.length === 0 || tooLarge;
        error.textContent = tooLarge ? "评论不能超过 4 KiB" : "";
      };
      const submitComment = () => {
        refreshValidity();
        if (!submit.disabled && this.editing) {
          this.callbacks.onSubmit(this.editing, textarea.value.trim());
        }
      };
      textarea.addEventListener("input", refreshValidity);
      textarea.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          this.callbacks.onCancel();
        } else if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
          event.preventDefault();
          submitComment();
        }
      });
      submit.addEventListener("click", submitComment);
      actions.append(cancel, submit);
      form.append(header, textarea, error, actions);
      root.append(form);
      queueMicrotask(() => textarea.focus({ preventScroll: true }));
    }
    return root;
  }

  ignoreEvent(): boolean {
    return false;
  }
}

function lineRangeLabel(startLine: number, endLine: number): string {
  return startLine === endLine ? `第 ${startLine} 行` : `第 ${startLine}–${endLine} 行`;
}

function commentDecorations(state: EditorState, config: CommentWidgetConfig): DecorationSet {
  const ranges = [];
  if (config.selection) {
    for (let line = config.selection.startLine; line <= config.selection.endLine; line += 1) {
      if (line >= 1 && line <= state.doc.lines) {
        ranges.push(
          Decoration.line({ class: "project-file-line-comment-selection" }).range(
            state.doc.line(line).from,
          ),
        );
      }
    }
  }
  const byEndLine = new Map<number, FileLineComment[]>();
  for (const comment of config.comments) {
    const existing = byEndLine.get(comment.endLine) ?? [];
    existing.push(comment);
    byEndLine.set(comment.endLine, existing);
  }
  if (config.editing) {
    byEndLine.set(config.editing.endLine, byEndLine.get(config.editing.endLine) ?? []);
  }
  ranges.push(...[...byEndLine.entries()]
    .filter(([line]) => line >= 1 && line <= state.doc.lines)
    .sort(([a], [b]) => a - b)
    .map(([line, comments]) =>
      Decoration.widget({
        block: true,
        side: 1,
        widget: new FileLineCommentWidget(
          line,
          comments,
          config.editing?.endLine === line ? config.editing : undefined,
          config,
        ),
      }).range(state.doc.line(line).to),
    ));
  return Decoration.set(ranges, true);
}

class FileLineCommentSelectionGutterMarker extends GutterMarker {
  elementClass = "project-file-line-comment-selection-gutter";
}

const fileLineCommentSelectionGutterMarker = new FileLineCommentSelectionGutterMarker();

function commentGutterDecorations(
  state: EditorState,
  config: CommentWidgetConfig,
): RangeSet<GutterMarker> {
  if (!config.selection) return RangeSet.empty;
  const ranges = [];
  for (let line = config.selection.startLine; line <= config.selection.endLine; line += 1) {
    if (line >= 1 && line <= state.doc.lines) {
      ranges.push(fileLineCommentSelectionGutterMarker.range(state.doc.line(line).from));
    }
  }
  return RangeSet.of(ranges, true);
}

const fileCommentWidgetField = StateField.define<{
  config?: CommentWidgetConfig;
  decorations: DecorationSet;
  gutterDecorations: RangeSet<GutterMarker>;
}>({
  create: () => ({
    decorations: Decoration.none,
    gutterDecorations: RangeSet.empty,
  }),
  update: (value, transaction) => {
    let config = value.config;
    for (const effect of transaction.effects) {
      if (effect.is(setCommentWidgetConfig)) config = effect.value;
    }
    if (!config) return value;
    if (transaction.docChanged || config !== value.config) {
      return {
        config,
        decorations: commentDecorations(transaction.state, config),
        gutterDecorations: commentGutterDecorations(transaction.state, config),
      };
    }
    return value;
  },
  provide: (field) => [
    EditorView.decorations.from(field, (value) => value.decorations),
    gutterLineClass.from(field, (value) => value.gutterDecorations),
  ],
});

export function codePreviewLineSelection(state: EditorState): CodePreviewLineSelection {
  const selection = state.selection.main;
  const startLine = state.doc.lineAt(selection.from).number;
  let endPosition = selection.to;
  if (selection.to > selection.from && selection.to === state.doc.lineAt(selection.to).from) {
    endPosition -= 1;
  }
  const endLine = state.doc.lineAt(Math.max(selection.from, endPosition)).number;
  const selectedText: string[] = [];
  for (let line = startLine; line <= endLine; line += 1) {
    selectedText.push(state.doc.line(line).text);
  }
  return { startLine, endLine, selectedText: selectedText.join("\n") };
}

export function codePreviewCommentHighlightSelection(
  selection: CodePreviewLineSelection | undefined,
  editing: CodePreviewLineSelection | undefined,
): CodePreviewLineSelection | undefined {
  return editing ?? selection;
}

export default function CodePreview({
  preview,
  frameProps,
  fileComments = [],
  onCreateComment,
  onDeleteComment,
  commentDisabled = false,
  editable = false,
  onSave,
  commentRequest,
  onCommentRequestHandled,
  leadingFormatActions,
}: CodePreviewProps) {
  const editorHostRef = useRef<HTMLDivElement>(null);
  const editorShellRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<EditorView | null>(null);
  const wrapCompartmentRef = useRef(new Compartment());
  const [lineWrapping, setLineWrapping] = useState(true);
  const [lineSelection, setLineSelection] = useState<CodePreviewLineSelection>();
  const lineSelectionRef = useRef<CodePreviewLineSelection | undefined>(undefined);
  const [editingSelection, setEditingSelection] = useState<CodePreviewLineSelection>();
  const [plusTop, setPlusTop] = useState<number>();
  const onCreateCommentRef = useRef(onCreateComment);
  const onDeleteCommentRef = useRef(onDeleteComment);
  const onSaveRef = useRef(onSave);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const latestContentRef = useRef(preview.content);
  const [saveError, setSaveError] = useState<string>();
  const [editorVersion, setEditorVersion] = useState(0);
  onCreateCommentRef.current = onCreateComment;
  onDeleteCommentRef.current = onDeleteComment;
  onSaveRef.current = onSave;

  const saveContent = useCallback((content: string) => {
    latestContentRef.current = content;
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    if (!onSaveRef.current) return;
    saveTimerRef.current = setTimeout(() => {
      saveTimerRef.current = undefined;
      void onSaveRef.current?.(latestContentRef.current).then((result) => {
        setSaveError(result.ok ? undefined : result.message);
      }).catch(() => setSaveError("暂时无法自动保存这个文件。"));
    }, 450);
  }, []);

  const updateSelection = useCallback((view: EditorView, expose: boolean) => {
    if (!expose) return;
    const selection = codePreviewLineSelection(view.state);
    lineSelectionRef.current = selection;
    setLineSelection(selection);
    const coords = view.coordsAtPos(view.state.doc.line(selection.endLine).from);
    const hostRect = editorShellRef.current?.getBoundingClientRect();
    if (coords && hostRect) setPlusTop(coords.top - hostRect.top + (coords.bottom - coords.top - 24) / 2);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const host = editorHostRef.current;
    if (!host) return;
    void loadPreviewLanguage(preview.displayName, preview.language).then(
      (language) => {
        if (cancelled || !editorHostRef.current) return;
        const view = new EditorView({
          parent: editorHostRef.current,
          state: EditorState.create({
            doc: preview.content,
            extensions: [
              lineNumbers(),
              highlightSpecialChars(),
              bracketMatching(),
              syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
              keymap.of([
                {
                  key: "Mod-Shift-m",
                  run: (currentView) => {
                    if (commentDisabled || !onCreateCommentRef.current) return false;
                    const selection = codePreviewLineSelection(currentView.state);
                    lineSelectionRef.current = selection;
                    setLineSelection(selection);
                    if (selection.endLine - selection.startLine + 1 <= 200) {
                      setEditingSelection(selection);
                    }
                    return true;
                  },
                },
                ...defaultKeymap,
              ]),
              EditorState.readOnly.of(!editable),
              EditorView.editable.of(editable),
              EditorView.contentAttributes.of({
                "aria-label": editable
                  ? `${preview.displayName} 文本编辑器`
                  : `${preview.displayName} 只读代码预览`,
              }),
              wrapCompartmentRef.current.of(EditorView.lineWrapping),
              fileCommentWidgetField,
              EditorView.updateListener.of((update) => {
                if (update.docChanged && editable) saveContent(update.state.doc.toString());
                if (update.selectionSet) updateSelection(update.view, true);
                else if (update.viewportChanged && lineSelectionRef.current) updateSelection(update.view, true);
              }),
              EditorView.theme({
                "&": {
                  height: "100%",
                  backgroundColor: "transparent",
                  color: "var(--aw-text-primary)",
                },
                ".cm-scroller": {
                  fontFamily: "var(--aw-font-mono)",
                  fontSize: "13px",
                  lineHeight: "1.65",
                },
                ".cm-gutters": {
                  backgroundColor: "transparent",
                  color: "var(--aw-text-tertiary)",
                  borderRight: "none",
                },
                ".cm-content": { padding: "14px 0 40px" },
                ".cm-line": { padding: "0 18px" },
                ".cm-search": {
                  backgroundColor: "var(--aw-surface-panel)",
                  color: "var(--aw-text-primary)",
                  borderTop: "1px solid var(--aw-border-light)",
                },
              }),
              ...(language ? [language] : []),
            ],
          }),
        });
        editorRef.current = view;
        setEditorVersion((value) => value + 1);
        view.scrollDOM.addEventListener("scroll", () => updateSelection(view, Boolean(lineSelectionRef.current)), {
          passive: true,
        });
      },
    );
    return () => {
      cancelled = true;
      if (saveTimerRef.current) {
        clearTimeout(saveTimerRef.current);
        saveTimerRef.current = undefined;
        if (onSaveRef.current && latestContentRef.current !== preview.content) {
          void onSaveRef.current(latestContentRef.current);
        }
      }
      editorRef.current?.destroy();
      editorRef.current = null;
      host.replaceChildren();
    };
  }, [commentDisabled, editable, preview.displayName, preview.language, preview.previewId, saveContent, updateSelection]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor || !commentRequest) return;
    const startLine = Math.max(1, Math.min(commentRequest.startLine, editor.state.doc.lines));
    const endLine = Math.max(startLine, Math.min(commentRequest.endLine, editor.state.doc.lines));
    editor.dispatch({
      selection: {
        anchor: editor.state.doc.line(startLine).from,
        head: editor.state.doc.line(endLine).to,
      },
      scrollIntoView: true,
    });
    const selection = codePreviewLineSelection(editor.state);
    lineSelectionRef.current = selection;
    setLineSelection(selection);
    setEditingSelection(selection);
    onCommentRequestHandled?.(commentRequest.id);
  }, [commentRequest, editorVersion, onCommentRequestHandled]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    editor.dispatch({
      effects: setCommentWidgetConfig.of({
        comments: fileComments,
        selection: codePreviewCommentHighlightSelection(lineSelection, editingSelection),
        editing: editingSelection,
        onSubmit: (selection, comment) => {
          onCreateCommentRef.current?.(selection, comment);
          setEditingSelection(undefined);
        },
        onCancel: () => setEditingSelection(undefined),
        onDelete: (commentId) => onDeleteCommentRef.current?.(commentId),
      }),
    });
  }, [editingSelection, fileComments, lineSelection]);

  return (
    <PreviewFrame
      {...frameProps}
      formatLabel={preview.language ?? "纯文本"}
      formatActions={
        <>
          {leadingFormatActions}
          {saveError ? <span className="project-file-auto-save-error" role="alert">{saveError}</span> : null}
          <button
            type="button"
            aria-label="自动换行"
            title="自动换行"
            aria-pressed={lineWrapping}
            onClick={() => {
              const next = !lineWrapping;
              setLineWrapping(next);
              const editor = editorRef.current;
              if (!editor) return;
              editor.dispatch({
                effects: wrapCompartmentRef.current.reconfigure(
                  next ? EditorView.lineWrapping : [],
                ),
              });
            }}
          >
            <AwakenIcon concept="wrap-text" size={16} />
          </button>
        </>
      }
    >
      <div ref={editorShellRef} className="project-file-code-preview__editor-shell">
        <div ref={editorHostRef} className="project-file-code-preview__editor" />
        {lineSelection && plusTop !== undefined && onCreateComment && !commentDisabled ? (
          <button
            type="button"
            className="project-file-line-comment-add"
            style={{ top: `${plusTop}px` }}
            aria-label={`评论${lineRangeLabel(lineSelection.startLine, lineSelection.endLine)}`}
            title={lineSelection.endLine - lineSelection.startLine + 1 > 200
              ? "一次最多评论 200 行"
              : "添加评论"}
            disabled={lineSelection.endLine - lineSelection.startLine + 1 > 200}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => setEditingSelection(lineSelection)}
          >
            <AwakenIcon concept="add" size={16} />
          </button>
        ) : null}
      </div>
    </PreviewFrame>
  );
}
