import "./AwakenUserPromptBlock.css";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { AwakenCopyButton } from "../AwakenCopyButton";

export type AwakenUserPromptBlockProps = {
  text: string;
  expanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
};

export function AwakenUserPromptBlock({
  text,
  expanded,
  onExpandedChange,
}: AwakenUserPromptBlockProps) {
  const [uncontrolledExpanded, setUncontrolledExpanded] = useState(false);
  const blockRef = useRef<HTMLElement | null>(null);
  const isExpandable = text.split(/\r\n|\r|\n/).length > 3 || text.trim().length > 160;
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
    if (!isExpandable) {
      return;
    }
    updateExpanded(true);
  }

  function handlePromptKeyDown(event: KeyboardEvent<HTMLDivElement>): void {
    if (!isExpandable) {
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
        isExpandable ? " execution-prompt-block-limited" : ""
      }${
        isExpanded ? " execution-prompt-block-expanded" : ""
      }`}
    >
      <div
        className="execution-prompt-bubble"
        role={isExpandable && !isExpanded ? "button" : undefined}
        tabIndex={isExpandable && !isExpanded ? 0 : undefined}
        aria-expanded={isExpandable && !isExpanded ? isExpanded : undefined}
        onClick={expandPrompt}
        onKeyDown={handlePromptKeyDown}
      >
        <p className="execution-prompt-bubble-text">{text}</p>
        {isExpanded ? (
          <div className="execution-prompt-bubble-toolbar" aria-label="Prompt actions">
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
