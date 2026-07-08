import "./AwakenUserPromptBlock.css";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { FileText } from "lucide-react";
import { AwakenCopyButton } from "../AwakenCopyButton";

const collapsedAttachmentLimit = 3;

export type AwakenUserPromptBlockProps = {
  text: string;
  attachments?: Array<{
    id: string;
    kind?: "image" | "file";
    src?: string;
    alt: string;
    label?: string;
    detail?: string;
    tooltip?: string;
    extension?: string;
  }>;
  expanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
};

export function AwakenUserPromptBlock({
  text,
  attachments = [],
  expanded,
  onExpandedChange,
}: AwakenUserPromptBlockProps) {
  const [uncontrolledExpanded, setUncontrolledExpanded] = useState(false);
  const blockRef = useRef<HTMLElement | null>(null);
  const canExpandText =
    text.split(/\r\n|\r|\n/).length > 3 || text.trim().length > 160;
  const hasHiddenAttachments = attachments.length > collapsedAttachmentLimit;
  const isInteractive = text.trim().length > 0 || attachments.length > 0;
  const isExpanded = expanded ?? uncontrolledExpanded;
  const visibleAttachments = isExpanded
    ? attachments
    : attachments.slice(0, collapsedAttachmentLimit);
  const hiddenAttachmentCount = Math.max(
    0,
    attachments.length - visibleAttachments.length,
  );

  function updateExpanded(nextExpanded: boolean): void {
    if (expanded === undefined) {
      setUncontrolledExpanded(nextExpanded);
    }
    onExpandedChange?.(nextExpanded);
  }

  useEffect(() => {
    if (!isExpanded) {
      return;
    }

    function handleDocumentPointerDown(event: PointerEvent): void {
      const target = event.target;
      if (!(target instanceof Node) || blockRef.current?.contains(target)) {
        return;
      }
      updateExpanded(false);
    }

    document.addEventListener("pointerdown", handleDocumentPointerDown);
    return () => {
      document.removeEventListener("pointerdown", handleDocumentPointerDown);
    };
  }, [isExpanded, expanded, onExpandedChange]);

  function expandPrompt(): void {
    if (!isInteractive) {
      return;
    }
    updateExpanded(true);
  }

  function handlePromptKeyDown(event: KeyboardEvent<HTMLDivElement>): void {
    if (!isInteractive) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    expandPrompt();
  }

  function renderAttachment(attachment: NonNullable<AwakenUserPromptBlockProps["attachments"]>[number]) {
    const kind = attachment.kind ?? "image";
    const label = attachment.label ?? attachment.alt;
    const title = attachment.tooltip ?? [label, attachment.detail]
      .filter(Boolean)
      .join("\n");

    return (
      <div
        key={attachment.id}
        className={`execution-prompt-bubble-attachment execution-prompt-bubble-attachment-${kind}`}
        title={title || undefined}
      >
        {kind === "image" && attachment.src ? (
          <img
            className="execution-prompt-bubble-attachment-thumbnail"
            src={attachment.src}
            alt={attachment.alt}
          />
        ) : (
          <div className="execution-prompt-bubble-file-card" aria-label={label}>
            <FileText
              className="execution-prompt-bubble-file-icon"
              size={18}
              strokeWidth={1.85}
              aria-hidden="true"
            />
            <span className="execution-prompt-bubble-file-name">{label}</span>
            {attachment.extension ? (
              <span className="execution-prompt-bubble-file-extension">
                {attachment.extension}
              </span>
            ) : null}
          </div>
        )}
      </div>
    );
  }

  function renderAttachmentOverflow() {
    if (hiddenAttachmentCount <= 0) {
      return null;
    }
    const title = `还有 ${hiddenAttachmentCount} 个附件，点击消息展开查看`;
    return (
      <div
        className="execution-prompt-bubble-attachment execution-prompt-bubble-attachment-more"
        title={title}
        aria-label={title}
      >
        +{hiddenAttachmentCount}
      </div>
    );
  }

  return (
    <article
      ref={blockRef}
      className={`execution-prompt-block${
        canExpandText ? " execution-prompt-block-limited" : ""
      }${hasHiddenAttachments ? " execution-prompt-block-has-hidden-attachments" : ""
      }${isExpanded ? " execution-prompt-block-expanded" : ""}`}
    >
      <div
        className="execution-prompt-bubble"
        role={isInteractive && !isExpanded ? "button" : undefined}
        tabIndex={isInteractive && !isExpanded ? 0 : undefined}
        aria-expanded={isInteractive && !isExpanded ? isExpanded : undefined}
        onClick={expandPrompt}
        onKeyDown={handlePromptKeyDown}
      >
        {attachments.length > 0 ? (
          <div
            className="execution-prompt-bubble-attachments"
            aria-label="Prompt attachments"
          >
            {visibleAttachments.map((attachment) => renderAttachment(attachment))}
            {renderAttachmentOverflow()}
          </div>
        ) : null}
        {text.trim().length > 0 ? (
          <p className="execution-prompt-bubble-text">{text}</p>
        ) : null}
        {isExpanded ? (
          <div
            className="execution-prompt-bubble-toolbar"
            aria-label="Prompt actions"
          >
            <AwakenCopyButton
              text={text}
              label="Copy prompt"
              copiedLabel="Copied"
              errorLabel="Copy failed"
              iconOnly
              feedbackDurationMs={1000}
              onClick={(event) => {
                event.stopPropagation();
              }}
              style={{
                width: "24px",
                height: "24px",
                borderRadius: "8px",
                background: "var(--aw-bg-fill-light)",
                borderColor: "rgba(31, 31, 31, 0.08)",
              }}
            />
          </div>
        ) : null}
      </div>
    </article>
  );
}
