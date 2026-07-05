import "./AwakenUserPromptBlock.css";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { AwakenCopyButton } from "../AwakenCopyButton";

export type AwakenUserPromptBlockProps = {
  text: string;
  attachments?: Array<{
    id: string;
    src: string;
    alt: string;
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
  const isInteractive = text.trim().length > 0 || attachments.length > 0;
  const isExpanded = expanded ?? uncontrolledExpanded;

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

  return (
    <article
      ref={blockRef}
      className={`execution-prompt-block${
        canExpandText ? " execution-prompt-block-limited" : ""
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
            aria-label="Prompt images"
          >
            {attachments.map((attachment) => (
              <img
                key={attachment.id}
                className="execution-prompt-bubble-attachment"
                src={attachment.src}
                alt={attachment.alt}
              />
            ))}
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
