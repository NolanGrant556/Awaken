import "./AwakenSkillPreviewDialog.css";
import { useEffect, useRef } from "react";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenMarkdownRenderer } from "../AwakenMarkdownRenderer";

export type AwakenSkillPreviewDialogProps = {
  open: boolean;
  title?: string;
  contentKey?: string;
  frontmatterRaw?: string;
  markdown?: string;
  loading?: boolean;
  errorMessage?: string | null;
  onOpenChange: (open: boolean) => void;
};

export function AwakenSkillPreviewDialog({
  open,
  title = "SKILL.md 预览",
  contentKey,
  frontmatterRaw,
  markdown,
  loading = false,
  errorMessage,
  onOpenChange,
}: AwakenSkillPreviewDialogProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open) {
      scrollRef.current?.scrollTo({ top: 0 });
    }
  }, [contentKey, open]);

  return (
    <AwakenDialog
      open={open}
      title={title}
      titleVisuallyHidden
      size="large"
      className="aw-skill-preview"
      bodyClassName="aw-skill-preview__scroll"
      bodyRef={scrollRef}
      bodyTabIndex={0}
      initialFocusRef={scrollRef}
      onOpenChange={onOpenChange}
    >
      <div className="aw-skill-preview__content">
        {loading ? (
          <div className="aw-skill-preview__status" role="status" aria-live="polite">
            正在读取 SKILL.md…
          </div>
        ) : null}
        {errorMessage ? (
          <div className="aw-skill-preview__error" role="alert">
            <strong>无法预览 SKILL.md</strong>
            <span>{errorMessage}</span>
          </div>
        ) : null}
        {!loading && !errorMessage && frontmatterRaw ? (
          <pre className="aw-skill-preview__frontmatter" aria-label="YAML frontmatter">
            <code className="language-yaml">{frontmatterRaw}</code>
          </pre>
        ) : null}
        {!loading && !errorMessage && markdown ? (
          <AwakenMarkdownRenderer
            className="assistant-message assistant-message-markdown aw-skill-preview__markdown"
            markdown={markdown}
            fallback=""
            renderMermaid={false}
            renderImages={false}
            allowedLinkProtocols={["http", "https"]}
          />
        ) : null}
      </div>
    </AwakenDialog>
  );
}
