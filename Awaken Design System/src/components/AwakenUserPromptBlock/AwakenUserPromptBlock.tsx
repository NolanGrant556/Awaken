import "./AwakenUserPromptBlock.css";
import { composerTokenIconConcept } from "../../composerTokenIcon";
import { AwakenIcon } from "../AwakenIcon";
import {
  Fragment,
  useEffect,
  useRef,
  useState,
  type ClipboardEvent as ReactClipboardEvent,
  type KeyboardEvent,
  type MouseEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";

import { AwakenCopyButton } from "../AwakenCopyButton";

const collapsedAttachmentLimit = 3;
const expandDragThresholdPx = 4;
const PROMPT_TOKEN_SELECTED_ATTRIBUTE = "data-prompt-token-selected";

export function syncPromptTokenSelection(root: HTMLElement): void {
  const selection = root.ownerDocument.defaultView?.getSelection();
  const selectedTokens = new Set<HTMLElement>();
  const tokens = Array.from(
    root.querySelectorAll<HTMLElement>(".execution-prompt-bubble-token"),
  );
  if (selection && !selection.isCollapsed && selection.rangeCount > 0) {
    for (let rangeIndex = 0; rangeIndex < selection.rangeCount; rangeIndex += 1) {
      const range = selection.getRangeAt(rangeIndex);
      for (const token of tokens) {
        if (range.intersectsNode(token)) {
          selectedTokens.add(token);
        }
      }
    }
  }
  for (const token of tokens) {
    token.toggleAttribute(
      PROMPT_TOKEN_SELECTED_ATTRIBUTE,
      selectedTokens.has(token),
    );
  }
}

function appendPromptClipboardSegment(output: string, segment: string): string {
  if (!segment) {
    return output;
  }
  if (!output) {
    return segment;
  }
  if (/\s$/u.test(output) || /^\s/u.test(segment)) {
    return `${output}${segment}`;
  }
  return `${output} ${segment}`;
}

/** Serialize a selected prompt fragment from DOM text/token labels (no soft-wrap newlines). */
export function plainTextFromPromptClipboardRoot(root: Node): string {
  let output = "";
  let lastWasToken = false;

  function walk(node: Node): void {
    if (node.nodeType === Node.TEXT_NODE) {
      const value = node.nodeValue ?? "";
      if (
        lastWasToken &&
        output &&
        value &&
        !/\s$/u.test(output) &&
        !/^\s/u.test(value)
      ) {
        output += " ";
      }
      output += value;
      lastWasToken = false;
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) {
      Array.from(node.childNodes).forEach(walk);
      return;
    }

    const element = node as HTMLElement;
    if (
      element.getAttribute("aria-hidden") === "true" ||
      element.classList.contains("execution-prompt-bubble-token-icon") ||
      element.classList.contains("execution-prompt-bubble-file-icon") ||
      element.classList.contains("execution-prompt-bubble-file-extension") ||
      element.classList.contains("execution-prompt-bubble-attachment-more")
    ) {
      return;
    }

    if (element.classList.contains("execution-prompt-bubble-token")) {
      const label =
        element.querySelector(".execution-prompt-bubble-token-label")?.textContent ??
        element.textContent ??
        "";
      output = appendPromptClipboardSegment(output, label);
      lastWasToken = true;
      return;
    }

    if (element.classList.contains("execution-prompt-bubble-file-name")) {
      output = appendPromptClipboardSegment(output, element.textContent ?? "");
      lastWasToken = false;
      return;
    }

    if (element.classList.contains("execution-prompt-bubble-attachment-file")) {
      const label =
        element.querySelector(".execution-prompt-bubble-file-name")?.textContent ??
        element.getAttribute("aria-label") ??
        "";
      if (label) {
        output = output ? `${output}\n${label}` : label;
      }
      lastWasToken = false;
      return;
    }

    Array.from(element.childNodes).forEach(walk);
  }

  walk(root);
  return output;
}

export type AwakenUserPromptToken = {
  id: string;
  type?: string;
  label: string;
  leadingText?: string;
  title?: string;
};

export type AwakenUserPromptAttachment = {
  id: string;
  kind?: "image" | "file";
  src?: string;
  alt: string;
  label?: string;
  detail?: string;
  tooltip?: string;
  extension?: string;
};

export type AwakenUserPromptBlockProps = {
  text: string;
  copyText?: string;
  tokens?: AwakenUserPromptToken[];
  attachments?: AwakenUserPromptAttachment[];
  expanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
};

/** Visible prompt payload for copy: attachment names, then inline token labels + text. */
export function buildUserPromptCopyText(input: {
  text: string;
  tokens?: AwakenUserPromptToken[];
  attachments?: AwakenUserPromptAttachment[];
}): string {
  const attachmentNames = (input.attachments ?? [])
    .map((attachment) => (attachment.label ?? attachment.alt).trim())
    .filter((name) => name.length > 0);
  const tokens = input.tokens ?? [];
  let inlineText = "";
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index]!;
    inlineText += `${token.leadingText ?? ""}${token.label}`;
    const nextSegment =
      index + 1 < tokens.length ? (tokens[index + 1]?.leadingText ?? "") : input.text;
    if (
      nextSegment.length > 0 &&
      !/^\s/u.test(nextSegment) &&
      !/\s$/u.test(token.label)
    ) {
      // Match the visual gap after atomic tokens for plain-text clipboard output.
      inlineText += " ";
    }
  }
  inlineText += input.text;
  return [...attachmentNames, inlineText].filter((part) => part.length > 0).join("\n");
}

export function AwakenUserPromptBlock({
  text,
  copyText,
  tokens = [],
  attachments = [],
  expanded,
  onExpandedChange,
}: AwakenUserPromptBlockProps) {
  const [uncontrolledExpanded, setUncontrolledExpanded] = useState(false);
  const blockRef = useRef<HTMLElement | null>(null);
  const selectRootRef = useRef<HTMLDivElement | null>(null);
  const pointerGestureRef = useRef<{
    x: number;
    y: number;
    dragged: boolean;
  } | null>(null);
  const visiblePromptText = `${tokens
    .map((token) => `${token.leadingText ?? ""}${token.label}`)
    .join("")}${text}`;
  const resolvedCopyText = copyText ?? buildUserPromptCopyText({ text, tokens, attachments });
  const canExpandText =
    visiblePromptText.split(/\r\n|\r|\n/).length > 3 ||
    visiblePromptText.trim().length > 160;
  const hasHiddenAttachments = attachments.length > collapsedAttachmentLimit;
  const isInteractive =
    text.trim().length > 0 || tokens.length > 0 || attachments.length > 0;
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

    function handleDocumentPointerDown(event: globalThis.PointerEvent): void {
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

  useEffect(() => {
    const selectRoot = selectRootRef.current;
    if (!selectRoot) {
      return;
    }
    const ownerDocument = selectRoot.ownerDocument;
    const handleSelectionChange = () => {
      syncPromptTokenSelection(selectRoot);
    };
    handleSelectionChange();
    ownerDocument.addEventListener("selectionchange", handleSelectionChange);
    return () => {
      ownerDocument.removeEventListener("selectionchange", handleSelectionChange);
      for (const token of selectRoot.querySelectorAll<HTMLElement>(
        ".execution-prompt-bubble-token",
      )) {
        token.removeAttribute(PROMPT_TOKEN_SELECTED_ATTRIBUTE);
      }
    };
  }, []);

  function expandPrompt(): void {
    if (!isInteractive) {
      return;
    }
    updateExpanded(true);
  }

  function selectionBelongsToPrompt(): boolean {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
      return false;
    }
    const anchorNode = selection.anchorNode;
    return Boolean(anchorNode && blockRef.current?.contains(anchorNode));
  }

  function handlePromptPointerDown(event: ReactPointerEvent<HTMLDivElement>): void {
    if (event.button !== 0) {
      return;
    }
    pointerGestureRef.current = {
      x: event.clientX,
      y: event.clientY,
      dragged: false,
    };
  }

  function handlePromptPointerMove(event: ReactPointerEvent<HTMLDivElement>): void {
    const gesture = pointerGestureRef.current;
    if (!gesture || gesture.dragged) {
      return;
    }
    if (
      Math.abs(event.clientX - gesture.x) > expandDragThresholdPx ||
      Math.abs(event.clientY - gesture.y) > expandDragThresholdPx
    ) {
      gesture.dragged = true;
    }
  }

  function handlePromptClick(event: MouseEvent<HTMLDivElement>): void {
    if (!isInteractive || isExpanded) {
      pointerGestureRef.current = null;
      return;
    }
    if (event.detail > 1) {
      pointerGestureRef.current = null;
      return;
    }
    const dragged = pointerGestureRef.current?.dragged === true;
    pointerGestureRef.current = null;
    if (dragged || selectionBelongsToPrompt()) {
      return;
    }
    expandPrompt();
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

  function handlePromptCopy(event: ReactClipboardEvent<HTMLDivElement>): void {
    const selection = window.getSelection();
    const selectRoot = event.currentTarget;
    if (
      !selection ||
      selection.isCollapsed ||
      selection.rangeCount === 0 ||
      !selection.anchorNode ||
      !selectRoot.contains(selection.anchorNode)
    ) {
      return;
    }
    // Walk cloned DOM so soft-wrapped token/text lines do not become hard `\n`.
    const clipboardText = plainTextFromPromptClipboardRoot(
      selection.getRangeAt(0).cloneContents(),
    );
    if (!clipboardText) {
      return;
    }
    event.clipboardData.setData("text/plain", clipboardText);
    event.preventDefault();
  }

  function renderAttachment(attachment: AwakenUserPromptAttachment) {
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
            <AwakenIcon concept="file-text" size={20} className="execution-prompt-bubble-file-icon" />
            <span className="execution-prompt-bubble-file-name aw-text-selectable">
              {label}
            </span>
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

  function renderToken(token: AwakenUserPromptToken) {
    const tokenType = token.type ?? "token";
    const tokenConcept = composerTokenIconConcept(tokenType);
    return (
      <span
        className={`execution-prompt-bubble-token execution-prompt-bubble-token-${tokenType}`}
        title={token.title ?? token.label}
      >
        <AwakenIcon
          concept={tokenConcept}
          size={16}
          className="execution-prompt-bubble-token-icon"
        />
        <span className="execution-prompt-bubble-token-label aw-text-selectable">
          {token.label}
        </span>
      </span>
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
        onPointerDown={handlePromptPointerDown}
        onPointerMove={handlePromptPointerMove}
        onPointerCancel={() => {
          pointerGestureRef.current = null;
        }}
        onClick={handlePromptClick}
        onKeyDown={handlePromptKeyDown}
      >
        <div
          ref={selectRootRef}
          data-aw-select-all-root=""
          className="execution-prompt-bubble-select-root"
          onCopy={handlePromptCopy}
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
          {tokens.length > 0 || text.trim().length > 0 ? (
            <div className="execution-prompt-bubble-main">
              <p
                className="execution-prompt-bubble-text aw-text-selectable"
                data-aw-run-stage-scroll-boundary=""
              >
                {tokens.map((token) => (
                  <Fragment key={token.id}>
                    {token.leadingText ? (
                      <span className="execution-prompt-bubble-inline-text">
                        {token.leadingText}
                      </span>
                    ) : null}
                    {renderToken(token)}
                  </Fragment>
                ))}
                {text}
              </p>
            </div>
          ) : null}
        </div>
        {isExpanded ? (
          <div
            className="execution-prompt-bubble-toolbar"
            aria-label="Prompt actions"
          >
            <AwakenCopyButton
              text={resolvedCopyText}
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
                borderColor: "var(--aw-border-light)",
              }}
            />
          </div>
        ) : null}
      </div>
    </article>
  );
}
