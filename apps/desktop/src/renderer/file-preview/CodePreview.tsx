import { useEffect, useRef, useState } from "react";
import { defaultKeymap } from "@codemirror/commands";
import {
  bracketMatching,
  defaultHighlightStyle,
  syntaxHighlighting,
} from "@codemirror/language";
import { Compartment, EditorState } from "@codemirror/state";
import {
  EditorView,
  highlightSpecialChars,
  keymap,
  lineNumbers,
} from "@codemirror/view";
import { WrapText } from "lucide-react";
import type { ProjectFilePreviewTextHandle } from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";
import { loadPreviewLanguage } from "./previewLanguage";

export type CodePreviewProps = {
  preview: ProjectFilePreviewTextHandle;
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
};

export default function CodePreview({ preview, frameProps }: CodePreviewProps) {
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
              keymap.of(defaultKeymap),
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
    <PreviewFrame
      {...frameProps}
      formatLabel={preview.language ?? "纯文本"}
      formatActions={
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
          <WrapText size={15} aria-hidden="true" />
        </button>
      }
    >
      <div ref={editorHostRef} className="project-file-code-preview__editor" />
    </PreviewFrame>
  );
}
