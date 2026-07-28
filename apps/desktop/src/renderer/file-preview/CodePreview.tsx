import { useEffect, useRef, useState } from "react";
import { defaultKeymap } from "@codemirror/commands";
import {
  bracketMatching,
  defaultHighlightStyle,
  syntaxHighlighting,
} from "@codemirror/language";
import { openSearchPanel, searchKeymap } from "@codemirror/search";
import { Compartment, EditorState } from "@codemirror/state";
import {
  EditorView,
  highlightSpecialChars,
  keymap,
  lineNumbers,
} from "@codemirror/view";
import { Search, WrapText } from "lucide-react";
import type { ProjectFilePreviewTextHandle } from "../../shared/runtime";
import { loadPreviewLanguage } from "./previewLanguage";

export type CodePreviewProps = {
  preview: ProjectFilePreviewTextHandle;
};

export default function CodePreview({ preview }: CodePreviewProps) {
  const editorHostRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<EditorView | null>(null);
  const wrapCompartmentRef = useRef(new Compartment());
  const [lineWrapping, setLineWrapping] = useState(true);

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
              keymap.of([...searchKeymap, ...defaultKeymap]),
              EditorState.readOnly.of(true),
              EditorView.editable.of(false),
              EditorView.contentAttributes.of({
                "aria-label": `${preview.displayName} 只读代码预览`,
              }),
              wrapCompartmentRef.current.of(EditorView.lineWrapping),
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
                  backgroundColor: "var(--aw-surface-panel-muted)",
                  color: "var(--aw-text-tertiary)",
                  borderRight: "1px solid var(--aw-border-light)",
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
      },
    );
    return () => {
      cancelled = true;
      editorRef.current?.destroy();
      editorRef.current = null;
      host.replaceChildren();
    };
  }, [preview.content, preview.displayName, preview.language]);

  return (
    <div className="project-file-code-preview" data-preview-kind="code">
      <div className="project-file-preview-toolbar">
        <span>{preview.language ?? "纯文本"}</span>
        <div className="project-file-preview-toolbar__actions">
          <button
            type="button"
            onClick={() => editorRef.current && openSearchPanel(editorRef.current)}
          >
            <Search size={15} aria-hidden="true" />
            搜索
          </button>
          <button
            type="button"
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
            <WrapText size={15} aria-hidden="true" />
            自动换行
          </button>
        </div>
      </div>
      <div ref={editorHostRef} className="project-file-code-preview__editor" />
    </div>
  );
}
