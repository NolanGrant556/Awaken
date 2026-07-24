import "./AwakenSkillPreviewDialog.css";
import { Trash2, Wrench } from "lucide-react";
import { useEffect, useRef, type ReactNode, type Ref } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenMarkdownRenderer } from "../AwakenMarkdownRenderer";
import { AwakenSwitch } from "../AwakenSwitch";

export type AwakenSkillPreviewMeta = {
  name: string;
  sourceLabel: string;
  description: string;
  enabled: boolean;
  toggleDisabled?: boolean;
  deleting?: boolean;
};

export type AwakenSkillPreviewSurfaceProps = AwakenSkillPreviewMeta & {
  children: ReactNode;
  documentRef?: Ref<HTMLDivElement>;
  onEnabledChange?: (enabled: boolean) => void;
};

export function AwakenSkillPreviewSurface({
  name,
  sourceLabel,
  description,
  enabled,
  toggleDisabled = false,
  deleting = false,
  children,
  documentRef,
  onEnabledChange,
}: AwakenSkillPreviewSurfaceProps) {
  return (
    <div className="aw-skill-preview-surface">
      <div className="aw-skill-preview-summary">
        <span className="aw-skill-preview-summary__icon" aria-hidden="true">
          <Wrench size={20} strokeWidth={1.8} />
        </span>
        <div className="aw-skill-preview-summary__copy">
          <div className="aw-skill-preview-summary__title">
            <strong>{name}</strong>
            <span>{sourceLabel}</span>
          </div>
          <p>{description}</p>
        </div>
        <AwakenSwitch
          checked={enabled}
          label={name}
          busy={deleting}
          disabled={toggleDisabled || deleting || !onEnabledChange}
          onCheckedChange={onEnabledChange}
        />
      </div>
      <div
        ref={documentRef}
        className="aw-skill-preview-document"
        tabIndex={0}
        aria-label="SKILL.md 内容"
      >
        {children}
      </div>
    </div>
  );
}

export type AwakenSkillPreviewContentProps = {
  frontmatterRaw?: string;
  markdown?: string;
  loading?: boolean;
  errorMessage?: string | null;
  skill?: AwakenSkillPreviewMeta;
  deleting?: boolean;
  documentRef?: Ref<HTMLDivElement>;
  onEnabledChange?: (enabled: boolean) => void;
};

export function AwakenSkillPreviewContent({
  frontmatterRaw,
  markdown,
  loading = false,
  errorMessage,
  skill,
  deleting = false,
  documentRef,
  onEnabledChange,
}: AwakenSkillPreviewContentProps) {
  const previewContent = (
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
  );

  return skill ? (
    <AwakenSkillPreviewSurface
      {...skill}
      deleting={deleting}
      documentRef={documentRef}
      onEnabledChange={onEnabledChange}
    >
      {previewContent}
    </AwakenSkillPreviewSurface>
  ) : (
    <div
      ref={documentRef}
      className="aw-skill-preview-document"
      tabIndex={0}
      aria-label="SKILL.md 内容"
    >
      {previewContent}
    </div>
  );
}

export function AwakenSkillPreviewDeleteButton({
  deleting = false,
  onDelete,
}: {
  deleting?: boolean;
  onDelete: () => void;
}) {
  return (
    <AwakenButton
      className="aw-skill-preview__delete"
      variant="default"
      size="md"
      loading={deleting}
      onClick={onDelete}
    >
      <Trash2 size={15} strokeWidth={1.9} />
      删除
    </AwakenButton>
  );
}

export type AwakenSkillPreviewDialogProps = {
  open: boolean;
  title?: string;
  contentKey?: string;
  frontmatterRaw?: string;
  markdown?: string;
  loading?: boolean;
  errorMessage?: string | null;
  skill?: AwakenSkillPreviewMeta;
  deletable?: boolean;
  deleting?: boolean;
  onEnabledChange?: (enabled: boolean) => void;
  onDelete?: () => void;
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
  skill,
  deletable = false,
  deleting = false,
  onEnabledChange,
  onDelete,
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
      bodyClassName="aw-skill-preview__body"
      initialFocusRef={scrollRef}
      footer={deletable && onDelete ? (
        <AwakenSkillPreviewDeleteButton deleting={deleting} onDelete={onDelete} />
      ) : undefined}
      onOpenChange={onOpenChange}
    >
      <AwakenSkillPreviewContent
        frontmatterRaw={frontmatterRaw}
        markdown={markdown}
        loading={loading}
        errorMessage={errorMessage}
        skill={skill}
        deleting={deleting}
        documentRef={scrollRef}
        onEnabledChange={onEnabledChange}
      />
    </AwakenDialog>
  );
}
